package watcher

import (
	"context"
	"fmt"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	slashing "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/rpc"
	"github.com/rs/zerolog/log"
)

type SlashingWatcher struct {
	metrics *metrics.Metrics
	pool    *rpc.Pool

	signedBlocksWindow      int64
	minSignedPerWindow      float64
	downtimeJailDuration    float64
	slashFractionDoubleSign float64
	slashFractionDowntime   float64
}

func NewSlashingWatcher(metrics *metrics.Metrics, pool *rpc.Pool) *SlashingWatcher {
	return &SlashingWatcher{
		metrics: metrics,
		pool:    pool,
	}
}

func (w *SlashingWatcher) Start(ctx context.Context) error {
	// update metrics every 30 minutes
	ticker := time.NewTicker(30 * time.Minute)

	for {
		node := w.pool.GetSyncedNode()
		if node == nil {
			log.Warn().Msg("no node available to fetch slashing parameters")
		} else if err := w.fetchSlashingParameters(ctx, node); err != nil {
			log.Error().Err(err).
				Str("node", node.Redacted()).
				Msg("failed to fetch slashing parameters")
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *SlashingWatcher) fetchSlashingParameters(ctx context.Context, node *rpc.Node) error {
	clientCtx := (client.Context{}).WithClient(node.Client)
	queryClient := slashing.NewQueryClient(clientCtx)
	sigininParams, err := queryClient.Params(ctx, &slashing.QueryParamsRequest{})
	if err != nil {
		return fmt.Errorf("failed to get slashing parameters: %w", err)
	}

	w.handleSlashingParams(node.ChainID(), sigininParams.Params)

	return nil

}

func (w *SlashingWatcher) handleSlashingParams(chainID string, params slashing.Params) {
	log.Debug().
		Str("chainID", chainID).
		Str("downtimeJailDuration", params.DowntimeJailDuration.String()).
		Str("minSignedPerWindow", fmt.Sprintf("%.2f", params.MinSignedPerWindow.MustFloat64())).
		Str("signedBlocksWindow", fmt.Sprint(params.SignedBlocksWindow)).
		Str("slashFractionDoubleSign", fmt.Sprintf("%.2f", params.SlashFractionDoubleSign.MustFloat64())).
		Str("slashFractionDowntime", fmt.Sprintf("%.2f", params.SlashFractionDowntime.MustFloat64())).
		Msgf("updating slashing metrics")

	w.signedBlocksWindow = params.SignedBlocksWindow
	w.minSignedPerWindow, _ = params.MinSignedPerWindow.Float64()
	w.downtimeJailDuration = params.DowntimeJailDuration.Seconds()
	w.slashFractionDoubleSign, _ = params.SlashFractionDoubleSign.Float64()
	w.slashFractionDowntime, _ = params.SlashFractionDowntime.Float64()

	w.metrics.SignedBlocksWindow.WithLabelValues(chainID).Set(float64(w.signedBlocksWindow))
	w.metrics.MinSignedBlocksPerWindow.WithLabelValues(chainID).Set(w.minSignedPerWindow)
	w.metrics.DowntimeJailDuration.WithLabelValues(chainID).Set(w.downtimeJailDuration)
	w.metrics.SlashFractionDoubleSign.WithLabelValues(chainID).Set(w.slashFractionDoubleSign)
	w.metrics.SlashFractionDowntime.WithLabelValues(chainID).Set(w.slashFractionDowntime)
}
