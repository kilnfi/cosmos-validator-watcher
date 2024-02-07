package watcher

import (
	"context"
	"fmt"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/types"
	distribution "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/rpc"
	"github.com/rs/zerolog/log"
)

type CommissionWatcher struct {
	validators []TrackedValidator
	metrics    *metrics.Metrics
	pool       *rpc.Pool
}

func NewCommissionsWatcher(validators []TrackedValidator, metrics *metrics.Metrics, pool *rpc.Pool) *CommissionWatcher {
	return &CommissionWatcher{
		validators: validators,
		metrics:    metrics,
		pool:       pool,
	}
}

func (w *CommissionWatcher) Start(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Minute)

	for {
		node := w.pool.GetSyncedNode()
		if node == nil {
			log.Warn().Msg("no node available to fetch validators commissions")
		} else if err := w.fetchCommissions(ctx, node); err != nil {
			log.Error().Err(err).Msg("failed to fetch validators commissions")
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *CommissionWatcher) fetchCommissions(ctx context.Context, node *rpc.Node) error {
	for _, validator := range w.validators {
		if err := w.fetchValidatorCommission(ctx, node, validator); err != nil {
			log.Error().Err(err).Msgf("failed to fetch commission for validator %s", validator.OperatorAddress)
		}
	}
	return nil
}

func (w *CommissionWatcher) fetchValidatorCommission(ctx context.Context, node *rpc.Node, validator TrackedValidator) error {
	clientCtx := (client.Context{}).WithClient(node.Client)
	queryClient := distribution.NewQueryClient(clientCtx)

	commissionResq, err := queryClient.ValidatorCommission(ctx, &distribution.QueryValidatorCommissionRequest{
		ValidatorAddress: validator.OperatorAddress,
	})
	if err != nil {
		return fmt.Errorf("failed to get validators: %w", err)
	}

	w.handleValidatorCommission(node.ChainID(), validator, commissionResq.Commission.Commission)

	return nil
}

func (w *CommissionWatcher) handleValidatorCommission(chainID string, validator TrackedValidator, coins types.DecCoins) {
	for _, commission := range coins {
		w.metrics.Commission.
			WithLabelValues(chainID, validator.Address, validator.Name, commission.Denom).
			Set(commission.Amount.MustFloat64())
	}
}
