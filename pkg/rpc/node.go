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
	"github.com/cometbft/cometbft/types"
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

type OnNodeEvent func(ctx context.Context, n *Node, event *ctypes.ResultEvent) error
type OnNodeStart func(ctx context.Context, n *Node) error
type OnNodeStatus func(ctx context.Context, n *Node, status *ctypes.ResultStatus) error

type NodeOption func(*Node)

type Node struct {
	Client *http.HTTP

	onStart  []OnNodeStart
	onStatus []OnNodeStatus
	onEvent  map[string][]OnNodeEvent

	chainID       string
	status        atomic.Value
	latestBlock   atomic.Value
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
		onEvent:       make(map[string][]OnNodeEvent),
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

func (n *Node) OnEvent(eventType string, callback OnNodeEvent) {
	n.onEvent[eventType] = append(n.onEvent[eventType], callback)
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

	return !status.SyncInfo.CatchingUp &&
		status.SyncInfo.LatestBlockTime.After(time.Now().Add(-120*time.Second))
}

func (n *Node) ChainID() string {
	return n.chainID
}

func (n *Node) Start(ctx context.Context) error {
	// Wait for the node to be ready
	initTicker := time.NewTicker(30 * time.Second)
	for {
		status, err := n.syncStatus(ctx)
		if err != nil {
			log.Error().Err(err).Msg("failed to sync status")
		}
		if status != nil && !status.SyncInfo.CatchingUp {
			break
		}

		select {
		case <-ctx.Done():
			return nil
		case <-initTicker.C:
			continue
		}
	}
	initTicker.Stop()

	// Start the websocket process
	if err := n.Client.Start(); err != nil {
		return fmt.Errorf("failed to start client: %w", err)
	}

	blocksEvents, err := n.Subscribe(ctx, EventNewBlock)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	// Subscribe to validator set changes
	validatorEvents, err := n.Subscribe(ctx, EventValidatorSetUpdates)
	if err != nil {
		return fmt.Errorf("failed to subscribe to events: %w", err)
	}

	// Call the start callbacks
	n.startedOnce.Do(func() {
		n.handleStart(ctx)
	})

	// Start the status loop
	statusTicker := time.NewTicker(30 * time.Second)
	blocksTicker := time.NewTicker(10 * time.Second)
	for {
		select {
		case <-ctx.Done():
			log.Debug().Err(ctx.Err()).Str("node", n.Client.Remote()).Msgf("stopping node status loop")
			return nil

		case evt := <-blocksEvents:
			log.Debug().Str("node", n.Client.Remote()).Msg("got new block event")
			n.saveLatestBlock(evt.Data.(types.EventDataNewBlock).Block)
			n.handleEvent(ctx, EventNewBlock, &evt)
			blocksTicker.Reset(10 * time.Second)

		case evt := <-validatorEvents:
			log.Debug().Str("node", n.Client.Remote()).Msg("got validator set update event")
			n.handleEvent(ctx, EventValidatorSetUpdates, &evt)

		case <-blocksTicker.C:
			log.Debug().Str("node", n.Client.Remote()).Msg("syncing latest blocks")
			n.syncBlocks(ctx)

		case <-statusTicker.C:
			log.Debug().Str("node", n.Client.Remote()).Msg("syncing status")
			n.syncStatus(ctx)
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

	for _, onStatus := range n.onStatus {
		if err := onStatus(ctx, n, status); err != nil {
			log.Error().Err(err).Msgf("failed to call status callback")
		}
	}

	return status, nil
}

func (n *Node) handleStart(ctx context.Context) {
	for _, onStart := range n.onStart {
		if err := onStart(ctx, n); err != nil {
			log.Error().Err(err).Msgf("failed to call start node callback")
			return
		}
	}

	// Mark the node as ready
	close(n.started)
}

func (n *Node) handleEvent(ctx context.Context, eventType string, event *ctypes.ResultEvent) {
	for _, onEvent := range n.onEvent[eventType] {
		if err := onEvent(ctx, n, event); err != nil {
			log.Error().Err(err).Msgf("failed to call event callback")
		}
	}
}

func (n *Node) getLatestBlock() *types.Block {
	block := n.latestBlock.Load()
	if block == nil {
		return nil
	}
	return block.(*types.Block)
}

func (n *Node) saveLatestBlock(block *types.Block) {
	n.latestBlock.Store(block)
}

func (n *Node) syncBlocks(ctx context.Context) {
	// Fetch latest block
	currentBlockResp, err := n.Client.Block(ctx, nil)
	if err != nil {
		log.Error().Err(err).Msgf("failed to sync with latest block")
		return
	}

	currentBlock := currentBlockResp.Block
	if currentBlock == nil {
		log.Error().Err(err).Msgf("no block returned when requesting latest block")
		return
	}

	// Check the latest known block height
	latestBlockHeight := int64(0)
	latestBlock := n.getLatestBlock()
	if latestBlock != nil {
		latestBlockHeight = latestBlock.Height
	}

	// Go back to a maximum of 20 blocks
	if currentBlock.Height-latestBlockHeight > 20 {
		latestBlockHeight = currentBlock.Height - 20
	}

	// Fetch all skipped blocks since latest known block
	for height := latestBlockHeight + 1; height < currentBlock.Height; height++ {
		blockResp, err := n.Client.Block(ctx, &height)
		if err != nil {
			log.Error().Err(err).Msgf("failed to sync with latest block")
			continue
		}

		n.handleEvent(ctx, EventNewBlock, &ctypes.ResultEvent{
			Query: "",
			Data: types.EventDataNewBlock{
				Block: blockResp.Block,
			},
			Events: make(map[string][]string),
		})
	}

	n.handleEvent(ctx, EventNewBlock, &ctypes.ResultEvent{
		Query: "",
		Data: types.EventDataNewBlock{
			Block: currentBlockResp.Block,
		},
		Events: make(map[string][]string),
	})

	n.saveLatestBlock(currentBlockResp.Block)
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
