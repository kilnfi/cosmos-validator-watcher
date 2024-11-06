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

	signedBlocksWindow         int64
	min_signed_per_window      float64
	downtime_jail_duration     float64
	slash_fraction_double_sign float64
	slash_fraction_downtime    float64
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
		return fmt.Errorf("failed to get signing infos: %w", err)
	}

	w.handleSlashingParams(node.ChainID(), sigininParams.Params)

	return nil

}

func (w *SlashingWatcher) handleSlashingParams(chainID string, params slashing.Params) {
	log.Info().
		Str("Slashing parameters for chain:", chainID).
		Str("Signed blocks window:", fmt.Sprint(params.SignedBlocksWindow)).
		Str("Min signed per window:", params.MinSignedPerWindow.String()).
		Str("Downtime jail duration:", params.DowntimeJailDuration.String()).
		Str("Slash fraction double sign:", params.SlashFractionDoubleSign.String()).
		Str("Slash fraction downtime:", params.SlashFractionDowntime.String()).
		Msgf("Updating slashing metrics for chain %s", chainID)

	w.signedBlocksWindow = params.SignedBlocksWindow
	w.min_signed_per_window, _ = params.MinSignedPerWindow.Float64()
	w.downtime_jail_duration = params.DowntimeJailDuration.Seconds()
	w.slash_fraction_double_sign, _ = params.SlashFractionDoubleSign.Float64()
	w.slash_fraction_downtime, _ = params.SlashFractionDowntime.Float64()

	w.metrics.SignedBlocksWindow.WithLabelValues(chainID).Set(float64(w.signedBlocksWindow))
	w.metrics.MinSignedBlocksPerWindow.WithLabelValues(chainID).Set(w.min_signed_per_window)
	w.metrics.DowntimeJailDuration.WithLabelValues(chainID).Set(w.downtime_jail_duration)
	w.metrics.SlashFractionDoubleSign.WithLabelValues(chainID).Set(w.slash_fraction_double_sign)
	w.metrics.SlashFractionDowntime.WithLabelValues(chainID).Set(w.slash_fraction_downtime)
}
