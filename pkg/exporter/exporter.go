package exporter

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/fatih/color"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/rs/zerolog/log"
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
		for i, sig := range block.LastCommit.Signatures {
			if sig.ValidatorAddress.String() == "" {
				log.Warn().Msgf("empty validator address at pos %d", i)
			}
			if val.Address == sig.ValidatorAddress.String() {
				bonded = true
				signed = !sig.Absent()
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
	for _, tracked := range e.cfg.TrackedValidators {
		name := tracked.Name

		for _, val := range validators {
			pubkey := ed25519.PubKey{Key: val.ConsensusPubkey.Value[2:]}
			address := pubkey.Address().String()

			if tracked.Address == address {
				e.cfg.Metrics.ValidatorBonded.WithLabelValues(address, name).Set(metrics.BoolToFloat64(val.Status == stakingtypes.Bonded))
				e.cfg.Metrics.ValidatorJail.WithLabelValues(address, name).Set(metrics.BoolToFloat64(val.Jailed))
				break
			}
		}
	}
}
