package watcher

import (
	"context"
	"fmt"

	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/rpc"
	"github.com/rs/zerolog/log"
)

type StatusWatcher struct {
	metrics    *metrics.Metrics
	chainID    string
	statusChan chan *ctypes.ResultStatus
}

func NewStatusWatcher(chainID string, metrics *metrics.Metrics) *StatusWatcher {
	return &StatusWatcher{
		metrics:    metrics,
		chainID:    chainID,
		statusChan: make(chan *ctypes.ResultStatus),
	}
}

func (w *StatusWatcher) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case status := <-w.statusChan:
			if status == nil {
				continue
			} else if w.chainID == "" {
				w.chainID = status.NodeInfo.Network
			} else if w.chainID != status.NodeInfo.Network {
				return fmt.Errorf("chain ID mismatch: %s != %s", w.chainID, status.NodeInfo.Network)
			}
		}
	}
}

func (w *StatusWatcher) OnNodeStatus(ctx context.Context, n *rpc.Node, status *ctypes.ResultStatus) error {
	synced := false
	blockHeight := int64(0)
	chainID := ""

	if status != nil {
		synced = !status.SyncInfo.CatchingUp
		blockHeight = status.SyncInfo.LatestBlockHeight
		chainID = status.NodeInfo.Network
	}

	log.Debug().
		Str("node", n.Client.Remote()).
		Int64("height", blockHeight).
		Bool("synced", synced).
		Msgf("node status")

	w.metrics.NodeSynced.WithLabelValues(chainID, n.Client.Remote()).Set(
		metrics.BoolToFloat64(synced),
	)

	w.statusChan <- status

	return nil
}
