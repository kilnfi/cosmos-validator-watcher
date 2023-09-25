package rpc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/cometbft/cometbft/rpc/client/http"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/rs/zerolog/log"
)

const (
	EventCompleteProposal    = "CompleteProposal"
	EventNewBlock            = "NewBlock"
	EventNewRound            = "NewRound"
	EventRoundState          = "RoundState"
	EventValidatorSetUpdates = "ValidatorSetUpdates"
	EventVote                = "Vote"
)

type OnNodeStart func(ctx context.Context, n *Node) error
type OnNodeStatus func(ctx context.Context, n *Node, status *ctypes.ResultStatus) error

type NodeOption func(*Node)

type Node struct {
	Client *http.HTTP

	onStart  []OnNodeStart
	onStatus []OnNodeStatus

	chainID       string
	status        atomic.Value
	started       chan struct{}
	startedOnce   sync.Once
	subscriptions map[string]<-chan ctypes.ResultEvent
}

func NewNode(client *http.HTTP, options ...NodeOption) *Node {
	node := &Node{
		Client:        client,
		started:       make(chan struct{}),
		startedOnce:   sync.Once{},
		subscriptions: make(map[string]<-chan ctypes.ResultEvent),
	}

	for _, opt := range options {
		opt(node)
	}

	return node
}

func (n *Node) OnStart(callback OnNodeStart) {
	n.onStart = append(n.onStart, callback)
}

func (n *Node) OnStatus(callback OnNodeStatus) {
	n.onStatus = append(n.onStatus, callback)
}

func (n *Node) Started() chan struct{} {
	return n.started
}

func (n *Node) IsRunning() bool {
	return n.Client.IsRunning()
}

func (n *Node) IsSynced() bool {
	status := n.loadStatus()
	if status == nil {
		return false
	}

	return !status.SyncInfo.CatchingUp
}

func (n *Node) ChainID() string {
	return n.chainID
}

func (n *Node) Start(ctx context.Context) {
	// Start the status loop
	ticker := time.NewTicker(30 * time.Second)

	for {
		status, err := n.Status(ctx)
		if err != nil {
			log.Error().Err(err).Msgf("failed to get status of %s", n.Client.Remote())
		}

		for _, onStatus := range n.onStatus {
			if err := onStatus(ctx, n, status); err != nil {
				log.Error().Err(err).Msgf("failed to call status callback")
			}
		}

		if status != nil && !status.SyncInfo.CatchingUp {
			n.startedOnce.Do(func() {
				if err := n.handleStart(ctx); err != nil {
					log.Error().Err(err).Msgf("failed to start node")
				}
			})
		}

		select {
		case <-ctx.Done():
			log.Debug().Err(ctx.Err()).Str("node", n.Client.Remote()).Msgf("stopping node status loop")
			return
		case <-ticker.C:
		}
	}
}

func (n *Node) Status(ctx context.Context) (*ctypes.ResultStatus, error) {
	status := n.loadStatus()

	if status == nil || status.SyncInfo.LatestBlockTime.Before(time.Now().Add(-10*time.Second)) {
		return n.syncStatus(ctx)
	}

	return status, nil
}

func (n *Node) loadStatus() *ctypes.ResultStatus {
	status := n.status.Load()
	if status == nil {
		return nil
	}
	return status.(*ctypes.ResultStatus)
}

func (n *Node) syncStatus(ctx context.Context) (*ctypes.ResultStatus, error) {
	retryOpts := []retry.Option{
		retry.Context(ctx),
		retry.Delay(1 * time.Second),
		retry.Attempts(2),
		// retry.OnRetry(func(_ uint, err error) {
		// 	log.Warn().Err(err).Msgf("retrying status on %s", n.Client.Remote())
		// }),
	}

	status, err := retry.DoWithData(func() (*ctypes.ResultStatus, error) {
		return n.Client.Status(ctx)
	}, retryOpts...)

	n.status.Store(status)

	if err != nil {
		return status, fmt.Errorf("failed to get status of %s: %w", n.Client.Remote(), err)
	}

	if status.SyncInfo.CatchingUp {
		// We're catching up, not synced
		log.Warn().Msgf("node %s is catching up at block %d", n.Client.Remote(), status.SyncInfo.LatestBlockHeight)
		return status, nil
	}

	n.chainID = status.NodeInfo.Network

	return status, nil
}

func (n *Node) handleStart(ctx context.Context) error {
	// Start the websocket process
	if err := n.Client.Start(); err != nil {
		return fmt.Errorf("failed to start client: %w", err)
	}

	for _, onStart := range n.onStart {
		if err := onStart(ctx, n); err != nil {
			return fmt.Errorf("failed to call start callback: %w", err)
		}
	}

	// Mark the node as ready
	close(n.started)
	return nil
}

func (n *Node) Stop(ctx context.Context) error {
	if !n.IsRunning() {
		return nil
	}

	// if err := n.Client.UnsubscribeAll(ctx, "cosmos-validator-watcher"); err != nil {
	// 	return fmt.Errorf("failed to unsubscribe from all events: %w", err)
	// }

	// if err := n.Client.Stop(); err != nil {
	// 	return fmt.Errorf("failed to stop client: %w", err)
	// }

	return nil
}

func (n *Node) Subscribe(ctx context.Context, eventType string) (<-chan ctypes.ResultEvent, error) {
	if res, ok := n.subscriptions[eventType]; ok {
		return res, nil
	}

	out, err := n.Client.Subscribe(ctx, "cosmos-validator-watcher", fmt.Sprintf("tm.event='%s'", eventType), 10)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to %s: %w", eventType, err)
	}

	n.subscriptions[eventType] = out

	return n.subscriptions[eventType], nil
}
