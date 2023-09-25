package watcher

import (
	"context"
	"fmt"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	upgrade "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/rpc"
	"github.com/rs/zerolog/log"
)

type UpgradeWatcher struct {
	metrics *metrics.Metrics
	pool    *rpc.Pool
}

func NewUpgradeWatcher(metrics *metrics.Metrics, pool *rpc.Pool) *UpgradeWatcher {
	return &UpgradeWatcher{
		metrics: metrics,
		pool:    pool,
	}
}

func (w *UpgradeWatcher) Start(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)

	for {
		node := w.pool.GetSyncedNode()
		if node == nil {
			log.Warn().Msg("no node available to fetch updates")
		} else if err := w.fetchUpdates(ctx, node); err != nil {
			log.Error().Err(err).Msg("failed to fetch pending updates")
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *UpgradeWatcher) fetchUpdates(ctx context.Context, node *rpc.Node) error {
	clientCtx := (client.Context{}).WithClient(node.Client)
	queryClient := upgrade.NewQueryClient(clientCtx)

	// Fetch all proposals in voting period
	planResp, err := queryClient.CurrentPlan(ctx, &upgrade.QueryCurrentPlanRequest{})
	if err != nil {
		return fmt.Errorf("failed to get plan: %w", err)
	}

	isPlan := false
	if planResp.Plan != nil {
		isPlan = true
	}

	w.metrics.UpgradePlan.
		WithLabelValues(node.ChainID()).
		Set(metrics.BoolToFloat64(isPlan))

	return nil
}
