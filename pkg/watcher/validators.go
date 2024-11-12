package watcher

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/types/query"
	slashing "github.com/cosmos/cosmos-sdk/x/slashing/types"
	staking "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/crypto"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/rpc"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

type ValidatorsWatcher struct {
	metrics    *metrics.Metrics
	validators []TrackedValidator
	pool       *rpc.Pool
	opts       ValidatorsWatcherOptions
}

type ValidatorsWatcherOptions struct {
	Denom         string
	DenomExponent uint
	NoSlashing    bool
}

func NewValidatorsWatcher(validators []TrackedValidator, metrics *metrics.Metrics, pool *rpc.Pool, opts ValidatorsWatcherOptions) *ValidatorsWatcher {
	return &ValidatorsWatcher{
		metrics:    metrics,
		validators: validators,
		pool:       pool,
		opts:       opts,
	}
}

func (w *ValidatorsWatcher) Start(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)

	for {
		node := w.pool.GetSyncedNode()
		if node == nil {
			log.Warn().Msg("no node available to fetch validators")
		} else if err := w.fetchValidators(ctx, node); err != nil {
			log.Error().Err(err).
				Str("node", node.Redacted()).
				Msg("failed to fetch staking validators")
		} else if err := w.fetchSigningInfos(ctx, node); err != nil {
			log.Error().Err(err).
				Str("node", node.Redacted()).
				Msg("failed to fetch signing infos")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *ValidatorsWatcher) fetchSigningInfos(ctx context.Context, node *rpc.Node) error {
	if !w.opts.NoSlashing {
		clientCtx := (client.Context{}).WithClient(node.Client)
		queryClient := slashing.NewQueryClient(clientCtx)
		signingInfos, err := queryClient.SigningInfos(ctx, &slashing.QuerySigningInfosRequest{
			Pagination: &query.PageRequest{
				Limit: 3000,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to get signing infos: %w", err)
		}

		w.handleSigningInfos(node.ChainID(), signingInfos.Info)

		return nil
	} else {
		return nil
	}

}

func (w *ValidatorsWatcher) fetchValidators(ctx context.Context, node *rpc.Node) error {
	clientCtx := (client.Context{}).WithClient(node.Client)
	queryClient := staking.NewQueryClient(clientCtx)

	validators, err := queryClient.Validators(ctx, &staking.QueryValidatorsRequest{
		Pagination: &query.PageRequest{
			Limit: 3000,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to get validators: %w", err)
	}

	w.handleValidators(node.ChainID(), validators.Validators)

	return nil
}

func (w *ValidatorsWatcher) handleSigningInfos(chainID string, signingInfos []slashing.ValidatorSigningInfo) {
	for _, tracked := range w.validators {

		for _, val := range signingInfos {

			if tracked.ConsensusAddress == val.Address {
				w.metrics.MissedBlocksWindow.WithLabelValues(chainID, tracked.Address, tracked.Name).Set(float64(val.MissedBlocksCounter))
				break
			}
		}

	}
}

func (w *ValidatorsWatcher) handleValidators(chainID string, validators []staking.Validator) {
	// Sort validators by tokens & status (bonded, unbonded, jailed)
	sort.Sort(RankedValidators(validators))

	denomExponent := w.opts.DenomExponent
	if denomExponent == 0 {
		denomExponent = 1
	}

	seatPrice := decimal.Zero
	for _, val := range validators {
		tokens := decimal.NewFromBigInt(val.Tokens.BigInt(), -int32(denomExponent))
		if val.Status == staking.Bonded && (seatPrice.IsZero() || seatPrice.GreaterThan(tokens)) {
			seatPrice = tokens
		}
		w.metrics.SeatPrice.WithLabelValues(chainID, w.opts.Denom).Set(seatPrice.InexactFloat64())
	}

	for _, tracked := range w.validators {
		name := tracked.Name

		for i, val := range validators {
			address := crypto.PubKeyAddress(val.ConsensusPubkey)
			if tracked.Address == address {
				var (
					rank     = i + 1
					isBonded = val.Status == staking.Bonded
					isJailed = val.Jailed
					tokens   = decimal.NewFromBigInt(val.Tokens.BigInt(), -int32(denomExponent))
				)

				w.metrics.Rank.WithLabelValues(chainID, address, name).Set(float64(rank))
				w.metrics.Tokens.WithLabelValues(chainID, address, name, w.opts.Denom).Set(tokens.InexactFloat64())
				w.metrics.IsBonded.WithLabelValues(chainID, address, name).Set(metrics.BoolToFloat64(isBonded))
				w.metrics.IsJailed.WithLabelValues(chainID, address, name).Set(metrics.BoolToFloat64(isJailed))
				break
			}
		}
	}
}

type RankedValidators []staking.Validator

func (p RankedValidators) Len() int      { return len(p) }
func (p RankedValidators) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (s RankedValidators) Less(i, j int) bool {
	// Jailed validators are always last
	if s[i].Jailed && !s[j].Jailed {
		return false
	} else if !s[i].Jailed && s[j].Jailed {
		return true
	}

	// Not bonded validators are after bonded validators
	if s[i].Status == staking.Bonded && s[j].Status != staking.Bonded {
		return true
	} else if s[i].Status != staking.Bonded && s[j].Status == staking.Bonded {
		return false
	}

	return s[i].Tokens.BigInt().Cmp(s[j].Tokens.BigInt()) > 0
}
