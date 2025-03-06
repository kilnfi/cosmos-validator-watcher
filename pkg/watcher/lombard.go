package watcher

import (
	"context"
	"fmt"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/rpc"
	notary "github.com/lombard-finance/ledger/x/notary/types"
	"github.com/rs/zerolog/log"
)

type LombardWatcher struct {
	trackedValidators []TrackedValidator
	pool              *rpc.Pool
	metrics           *metrics.Metrics

	latestNotarySessionId uint64
}

func NewLombardWatcher(validators []TrackedValidator, pool *rpc.Pool, metrics *metrics.Metrics) *LombardWatcher {
	return &LombardWatcher{
		trackedValidators: validators,
		pool:              pool,
		metrics:           metrics,
	}
}

func (w *LombardWatcher) Start(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		if err := w.checkNotarySignatures(ctx); err != nil {
			log.Error().Err(err).Msg("failed to check notary signatures")
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			continue
		}
	}
}

func (w *LombardWatcher) checkNotarySignatures(ctx context.Context) error {
	node := w.pool.GetSyncedNode()
	if node == nil {
		return fmt.Errorf("no node available to fetch lombard notary session")
	}

	clientCtx := (client.Context{}).WithClient(node.Client)
	queryClient := notary.NewQueryClient(clientCtx)

	listResp, err := queryClient.ListNotarySession(ctx, &notary.QueryListNotarySessionRequest{
		Pagination: &query.PageRequest{
			Limit:   10,
			Offset:  0,
			Reverse: true,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to list notary sessions: %w", err)
	}

	chainID := node.ChainID()

	for _, session := range listResp.NotarySessions {
		// Skip notary sessions that are not completed
		if session.State != notary.Completed {
			continue
		}
		// Skip notary sessions that have already been processed
		if session.Id <= w.latestNotarySessionId {
			continue
		}

		w.latestNotarySessionId = session.Id
		w.metrics.LombardNotarySessionId.WithLabelValues(chainID).Set(float64(session.Id))
		w.metrics.LombardEpoch.WithLabelValues(chainID).Set(float64(session.ValSet.Epoch))

		// Ensure the number of participants and signatures match
		if len(session.ValSet.Participants) != len(session.Signatures) {
			log.Warn().
				Int("participants", len(session.ValSet.Participants)).
				Int("signatures", len(session.Signatures)).
				Msg("participants and signatures mismatch")
		}

		for _, validator := range w.trackedValidators {
			labels := []string{chainID, validator.Address, validator.Name}
			validatorHasSigned := false

			for valIndex, members := range session.ValSet.Participants {
				if members.Operator != validator.OperatorAddress {
					continue
				}

				// Check if operator has signed
				if session.Signatures[valIndex] != nil {
					validatorHasSigned = true
				}
			}

			if validatorHasSigned {
				w.metrics.LombardNotaryProducedSignatures.WithLabelValues(labels...).Inc()
				w.metrics.LombardNotaryConsecutiveMissedSignatures.WithLabelValues(labels...).Set(0)
			} else {
				w.metrics.LombardNotaryMissedSignatures.WithLabelValues(labels...).Inc()
				w.metrics.LombardNotaryConsecutiveMissedSignatures.WithLabelValues(labels...).Inc()

				log.Warn().
					Uint64("session_id", session.Id).
					Str("validator", validator.Name).
					Str("operator", validator.OperatorAddress).
					Msg("notary has not signed")
			}
		}
	}

	return nil
}
