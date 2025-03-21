package rpc

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

type Pool struct {
	ChainID string
	Nodes   []*Node

	started     chan struct{}
	startedOnce sync.Once
}

func NewPool(chainID string, nodes []*Node) *Pool {
	return &Pool{
		ChainID:     chainID,
		Nodes:       nodes,
		started:     make(chan struct{}),
		startedOnce: sync.Once{},
	}
}

func (p *Pool) Start(ctx context.Context) error {
	errg, ctx := errgroup.WithContext(ctx)
	for _, node := range p.Nodes {
		node := node
		// Start node
		errg.Go(func() error {
			if err := node.Start(ctx); err != nil {
				log.Error().Err(err).Msg("node error")
			}
			return nil
		})

		// Mark pool as started when the first node is started
		go func() {
			select {
			case <-ctx.Done():
			case <-node.Started():
				// Mark the pool as started
				p.startedOnce.Do(func() {
					close(p.started)
				})
			}
		}()
	}
	return errg.Wait()
}

func (p *Pool) Stop(ctx context.Context) error {
	errg, ctx := errgroup.WithContext(ctx)

	for _, node := range p.Nodes {
		node := node
		errg.Go(func() error {
			if err := node.Stop(ctx); err != nil {
				return fmt.Errorf("failed to stop node: %w", err)
			}

			return nil
		})
	}

	return errg.Wait()
}

func (p *Pool) Started() chan struct{} {
	return p.started
}

func (p *Pool) GetSyncedNode() *Node {
	for _, node := range p.Nodes {
		if node.IsSynced() {
			return node
		}
	}
	return nil
}

func (p *Pool) OnNodeStart(callback OnNodeStart) {
	for _, n := range p.Nodes {
		n.onStart = append(n.onStart, callback)
	}
}

func (p *Pool) OnNodeStatus(callback OnNodeStatus) {
	for _, n := range p.Nodes {
		n.onStatus = append(n.onStatus, callback)
	}
}

func (p *Pool) OnNodeEvent(eventType string, callback OnNodeEvent) {
	for _, n := range p.Nodes {
		n.onEvent[eventType] = append(n.onEvent[eventType], callback)
	}
}
