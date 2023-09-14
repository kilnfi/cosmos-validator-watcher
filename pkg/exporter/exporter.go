package exporter

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/fatih/color"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

// TrackedValidator adds the ability to attach a custom name to a validator address
type TrackedValidator struct {
	Address string
	Name    string
}

type Config struct {
	TrackedValidators []TrackedValidator

	Writer         io.Writer
	Metrics        *metrics.Metrics
	BlockChan      <-chan *types.Block
	ValidatorsChan <-chan []stakingtypes.Validator
}

func (c *Config) SetDefault() *Config {
	if c.Writer == nil {
		c.Writer = os.Stdout
	}
	return c
}

type Exporter struct {
	cfg *Config

	latestBlockHeight int64
}

func New(config *Config) *Exporter {
	return &Exporter{
		cfg: config.SetDefault(),
	}
}

func (e *Exporter) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case block := <-e.cfg.BlockChan:
			e.handleBlock(block)
		case validators := <-e.cfg.ValidatorsChan:
			e.handleValidators(validators)
		}
	}
}

type BlockResult struct {
	Height           int64
	TotalValidators  int
	SignedValidators int
	ValidatorStatus  []ValidatorStatus
}

type ValidatorStatus struct {
	Address string
	Label   string
	Bonded  bool
	Signed  bool
	Rank    int
}

func (e *Exporter) handleBlock(block *types.Block) {
	if e.latestBlockHeight >= block.Header.Height {
		// Skip already processed blocks
		return
	}

	blockDiff := block.Header.Height - e.latestBlockHeight
	if e.latestBlockHeight > 0 && blockDiff > 1 {
		log.Warn().Msgf("skipped %d unknown blocks", blockDiff)
		e.cfg.Metrics.SkippedBlocks.Add(float64(blockDiff))
	}

	e.latestBlockHeight = block.Header.Height
	e.cfg.Metrics.BlockHeight.Set(float64(block.Header.Height))
	e.cfg.Metrics.ActiveSet.Set(float64(len(block.LastCommit.Signatures)))
	e.cfg.Metrics.TrackedBlocks.Inc()

	result := BlockResult{
		Height:          block.Header.Height,
		TotalValidators: len(block.LastCommit.Signatures),
	}

	// Compute total signed validators
	for _, sig := range block.LastCommit.Signatures {
		if !sig.Absent() {
			result.SignedValidators++
		}
	}

	// Check status of tracked validators
	for _, val := range e.cfg.TrackedValidators {
		bonded := false
		signed := false
		rank := 0
		for i, sig := range block.LastCommit.Signatures {
			if sig.ValidatorAddress.String() == "" {
				log.Warn().Msgf("empty validator address at pos %d", i)
			}
			if val.Address == sig.ValidatorAddress.String() {
				bonded = true
				signed = !sig.Absent()
				rank = i + 1
			}
			if signed {
				break
			}
		}
		result.ValidatorStatus = append(result.ValidatorStatus, ValidatorStatus{
			Address: val.Address,
			Label:   val.Name,
			Bonded:  bonded,
			Signed:  signed,
			Rank:    rank,
		})
	}

	// Print block result & update metrics
	validatorStatus := []string{}
	for _, res := range result.ValidatorStatus {
		icon := "⚪️"
		if res.Signed {
			icon = "✅"
			e.cfg.Metrics.ValidatedBlocks.WithLabelValues(res.Address, res.Label).Inc()
		} else if res.Bonded {
			icon = "❌"
			e.cfg.Metrics.MissedBlocks.WithLabelValues(res.Address, res.Label).Inc()
		}
		validatorStatus = append(validatorStatus, fmt.Sprintf("%s %s", icon, res.Label))
	}

	fmt.Fprintln(
		e.cfg.Writer,
		color.YellowString(fmt.Sprintf("#%d", result.Height)),
		color.CyanString(fmt.Sprintf("%3d/%d validators", result.SignedValidators, result.TotalValidators)),
		strings.Join(validatorStatus, " "),
	)
}

func (e *Exporter) handleValidators(validators []stakingtypes.Validator) {
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
