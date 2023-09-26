package watcher

import (
	"context"
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
	ticker := time.NewTicker(1 * time.Minute)

	for {
		node := w.pool.GetSyncedNode()
		if node == nil {
			log.Warn().Msg("no node available to fetch upgrade plan")
		} else if err := w.fetchUpgrade(ctx, node); err != nil {
			log.Error().Err(err).Msg("failed to fetch upgrade plan")
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *UpgradeWatcher) fetchUpgrade(ctx context.Context, node *rpc.Node) error {
	clientCtx := (client.Context{}).WithClient(node.Client)
	queryClient := upgrade.NewQueryClient(clientCtx)

	resp, err := queryClient.CurrentPlan(ctx, &upgrade.QueryCurrentPlanRequest{})
	if err != nil {
		return err
	}

	w.handleUpgradePlan(node.ChainID(), resp.Plan)

	return nil
}

func (w *UpgradeWatcher) handleUpgradePlan(chainID string, plan *upgrade.Plan) {
	if plan == nil {
		w.metrics.UpgradePlan.Reset()
		return
	}

	w.metrics.UpgradePlan.WithLabelValues(chainID, plan.Name).Set(float64(plan.Height))
}
