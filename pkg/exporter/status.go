package exporter

import (
	"fmt"

	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/watcher"
)

func (e *Exporter) handleStatus(evt watcher.NodeEvent[*ctypes.ResultStatus]) error {
	endpoint := evt.Endpoint
	status := evt.Data

	if status == nil {
		e.cfg.Metrics.NodeSynced.WithLabelValues(endpoint).Set(0)
		return nil
	}

	// Check if node is on the good network
	if status.NodeInfo.Network != e.cfg.ChainID {
		return fmt.Errorf("node %s is on the wrong network: %s != %s", endpoint, status.NodeInfo.Network, e.cfg.ChainID)
	}

	e.cfg.Metrics.NodeSynced.WithLabelValues(endpoint).Set(metrics.BoolToFloat64(!status.SyncInfo.CatchingUp))

	return nil
}
