package exporter

import (
	"sort"

	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/watcher"
	"github.com/shopspring/decimal"
)

func (e *Exporter) handleValidators(evt watcher.NodeEvent[[]stakingtypes.Validator]) error {
	validators := evt.Data

	// Sort validators by tokens & status (bonded, unbonded, jailed)
	sort.Sort(RankedValidators(validators))

	seatPrice := decimal.Zero
	for _, val := range validators {
		tokens := decimal.NewFromBigInt(val.Tokens.BigInt(), -6)
		if val.Status == stakingtypes.Bonded && (seatPrice.IsZero() || seatPrice.GreaterThan(tokens)) {
			seatPrice = tokens
		}
		e.cfg.Metrics.SeatPrice.Set(seatPrice.InexactFloat64())
	}

	for _, tracked := range e.cfg.TrackedValidators {
		name := tracked.Name

		for i, val := range validators {
			pubkey := ed25519.PubKey{Key: val.ConsensusPubkey.Value[2:]}
			address := pubkey.Address().String()

			if tracked.Address == address {
				var (
					rank     = i + 1
					isBonded = val.Status == stakingtypes.Bonded
					isJailed = val.Jailed
					tokens   = decimal.NewFromBigInt(val.Tokens.BigInt(), -6)
				)

				e.cfg.Metrics.Rank.WithLabelValues(address, name).Set(float64(rank))
				e.cfg.Metrics.Tokens.WithLabelValues(address, name).Set(tokens.InexactFloat64())
				e.cfg.Metrics.IsBonded.WithLabelValues(address, name).Set(metrics.BoolToFloat64(isBonded))
				e.cfg.Metrics.IsJailed.WithLabelValues(address, name).Set(metrics.BoolToFloat64(isJailed))
				break
			}
		}
	}

	return nil
}

type RankedValidators []stakingtypes.Validator

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
	if s[i].Status == stakingtypes.Bonded && s[j].Status != stakingtypes.Bonded {
		return true
	} else if s[i].Status != stakingtypes.Bonded && s[j].Status == stakingtypes.Bonded {
		return false
	}

	return s[i].Tokens.BigInt().Cmp(s[j].Tokens.BigInt()) > 0
}
