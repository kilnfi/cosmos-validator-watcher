package exporter

import (
	"fmt"
	"strings"

	"github.com/cometbft/cometbft/types"
	"github.com/fatih/color"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/watcher"
	"github.com/rs/zerolog/log"
)

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

func (e *Exporter) handleBlock(evt watcher.NodeEvent[*types.Block]) error {
	endpoint := evt.Endpoint
	block := evt.Data

	e.cfg.Metrics.NodeBlockHeight.WithLabelValues(endpoint).Set(float64(block.Header.Height))

	if e.latestBlockHeight >= block.Header.Height {
		// Skip already processed blocks
		return nil
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
		Height:          block.Header.Height - 1,
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

	return nil
}
