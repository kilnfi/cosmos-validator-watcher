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

func WithOnStart(onStart OnNodeStart) NodeOption {
	return func(n *Node) {
		n.onStart = append(n.onStart, onStart)
	}
}

func WithOnStatus(onStatus OnNodeStatus) NodeOption {
	return func(n *Node) {
		n.onStatus = append(n.onStatus, onStatus)
	}
}

type Node struct {
	Client *http.HTTP

	onStart  []OnNodeStart
	onStatus []OnNodeStatus

	chainID       string
	isSynced      atomic.Bool
	started       chan struct{}
	startedOnce   sync.Once
	subscriptions map[string]<-chan ctypes.ResultEvent
}

func NewNode(client *http.HTTP, options ...NodeOption) *Node {
	node := &Node{
		Client:        client,
		isSynced:      atomic.Bool{},
		started:       make(chan struct{}),
		startedOnce:   sync.Once{},
		subscriptions: make(map[string]<-chan ctypes.ResultEvent),
	}

	for _, opt := range options {
		opt(node)
	}

	return node
}

func (n *Node) Started() chan struct{} {
	return n.started
}

func (n *Node) IsRunning() bool {
	return n.Client.IsRunning()
}

func (n *Node) IsSynced() bool {
	return n.isSynced.Load()
}

func (n *Node) ChainID() string {
	return n.chainID
}

func (n *Node) Start(ctx context.Context) {
	// Start the status loop
	ticker := time.NewTicker(30 * time.Second)

	for {
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

		for _, onStatus := range n.onStatus {
			if err := onStatus(ctx, n, status); err != nil {
				log.Error().Err(err).Msgf("failed to call status callback")
			}
		}

		if err != nil {
			// Failed to get status, assume we're not synced
			log.Error().Err(err).Msgf("failed to connect to %s", n.Client.Remote())
			n.isSynced.Store(false)
		} else if status.SyncInfo.CatchingUp {
			// We're catching up, not synced
			log.Warn().Msgf("node %s is catching up at block %d", n.Client.Remote(), status.SyncInfo.LatestBlockHeight)
			n.isSynced.Store(false)
		} else {
			// We're synced, set the ready flag
			n.isSynced.Store(true)
			n.startedOnce.Do(func() {
				n.chainID = status.NodeInfo.Network
				log.Info().Int64("height", status.SyncInfo.LatestBlockHeight).Msgf("connected to node %s", n.Client.Remote())
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
