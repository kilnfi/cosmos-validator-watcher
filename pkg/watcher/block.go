package watcher

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/avast/retry-go/v4"
	"github.com/cometbft/cometbft/types"
	"github.com/fatih/color"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/rpc"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

type BlockInfo struct {
	ChainID          string
	Height           int64
	TotalValidators  int
	SignedValidators int
	ValidatorStatus  []ValidatorStatus
}

func NewBlockInfo(block *types.Block) *BlockInfo {
	// Compute total signed validators
	signedValidators := 0
	for _, sig := range block.LastCommit.Signatures {
		if !sig.Absent() {
			signedValidators++
		}
	}

	return &BlockInfo{
		ChainID:          block.Header.ChainID,
		Height:           block.Header.Height - 1,
		TotalValidators:  len(block.LastCommit.Signatures),
		SignedValidators: signedValidators,
		ValidatorStatus:  []ValidatorStatus{},
	}
}

func (b *BlockInfo) SignedRatio() decimal.Decimal {
	if b.TotalValidators == 0 {
		return decimal.Zero
	}

	return decimal.NewFromInt(int64(b.SignedValidators)).
		Div(decimal.NewFromInt(int64(b.TotalValidators)))
}

type ValidatorStatus struct {
	Address string
	Label   string
	Bonded  bool
	Signed  bool
	Rank    int
}

type BlockWatcher struct {
	writer            io.Writer
	metrics           *metrics.Metrics
	validators        []TrackedValidator
	blockChan         chan *types.Block
	latestBlockHeight int64
}

func NewBlockWatcher(validators []TrackedValidator, metrics *metrics.Metrics, writer io.Writer) *BlockWatcher {
	return &BlockWatcher{
		validators:        validators,
		metrics:           metrics,
		writer:            writer,
		blockChan:         make(chan *types.Block),
		latestBlockHeight: 0,
	}
}

func (w *BlockWatcher) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case block := <-w.blockChan:
			w.handleBlock(block)
		}
	}
}

func (w *BlockWatcher) OnNodeStart(ctx context.Context, n *rpc.Node) error {
	blocksEvents, err := n.Subscribe(ctx, rpc.EventNewBlock)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	// Subscribe to validator set changes
	validatorEvents, err := n.Subscribe(ctx, rpc.EventValidatorSetUpdates)
	if err != nil {
		return fmt.Errorf("failed to subscribe to events: %w", err)
	}

	validators, err := retry.DoWithData(func() ([]*types.Validator, error) {
		return w.fetchValidators(ctx, n)
	}, retry.Attempts(2))
	if err != nil {
		return fmt.Errorf("failed to get active validator set: %w", err)
	}

	block, err := n.Client.Block(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get latest block: %w", err)
	}
	w.blockChan <- w.enchanceBlock(block.Block, validators)

	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Debug().Err(ctx.Err()).Str("node", n.Client.Remote()).Msgf("stopping block watcher loop")
				return
			case evt := <-blocksEvents:
				blockEvent := evt.Data.(types.EventDataNewBlock)
				w.metrics.NodeBlockHeight.WithLabelValues(n.ChainID(), n.Client.Remote()).Set(float64(blockEvent.Block.Height))
				w.blockChan <- w.enchanceBlock(blockEvent.Block, validators)
			case evt := <-validatorEvents:
				_ = evt.Data.(types.EventDataValidatorSetUpdates)
				validators, err = w.fetchValidators(ctx, n)
				if err != nil {
					log.Warn().Err(err).Msg("failed to update active validator set")
				}
			}
		}
	}()

	return nil
}

func (w *BlockWatcher) enchanceBlock(block *types.Block, validators []*types.Validator) *types.Block {
	if len(validators) != block.LastCommit.Size() {
		log.Warn().Msgf("validator set size mismatch: %d vs %d", len(validators), block.LastCommit.Size())
		return block
	}

	signatures := make([]types.CommitSig, len(block.LastCommit.Signatures))
	for i, sig := range block.LastCommit.Signatures {
		if len(sig.ValidatorAddress) == 0 {
			// Fill the validator address when it's empty (happens when validator miss the block)
			sig.ValidatorAddress = validators[i].Address
		} else if validators[i].Address.String() != sig.ValidatorAddress.String() {
			// Check that the validator address is correct compared to the active set
			log.Warn().Msgf("validator set mismatch pos %d: expected %s got %s", i, validators[i].Address, sig.ValidatorAddress.String())
		}
		signatures[i] = sig
	}

	block.LastCommit.Signatures = signatures

	return block
}

func (w *BlockWatcher) fetchValidators(ctx context.Context, n *rpc.Node) ([]*types.Validator, error) {
	validators := make([]*types.Validator, 0)

	for i := 0; i < 5; i++ {
		var (
			page    int = i + 1
			perPage int = 100
		)

		result, err := n.Client.Validators(ctx, nil, &page, &perPage)
		if err != nil {
			return nil, fmt.Errorf("failed to get validators: %w", err)
		}
		validators = append(validators, result.Validators...)

		if len(validators) >= int(result.Total) {
			break
		}
	}

	return validators, nil
}

func (w *BlockWatcher) handleBlock(block *types.Block) {
	chainId := block.Header.ChainID

	if w.latestBlockHeight >= block.Header.Height {
		// Skip already processed blocks
		return
	}

	// Ensure to inititalize counters for each validator
	for _, val := range w.validators {
		w.metrics.ValidatedBlocks.WithLabelValues(chainId, val.Address, val.Name)
		w.metrics.MissedBlocks.WithLabelValues(chainId, val.Address, val.Name)
		w.metrics.SoloMissedBlocks.WithLabelValues(chainId, val.Address, val.Name)
	}
	w.metrics.SkippedBlocks.WithLabelValues(chainId)

	blockDiff := block.Header.Height - w.latestBlockHeight
	if w.latestBlockHeight > 0 && blockDiff > 1 {
		log.Warn().Msgf("skipped %d unknown blocks", blockDiff)
		w.metrics.SkippedBlocks.WithLabelValues(chainId).Add(float64(blockDiff))
	}

	w.latestBlockHeight = block.Header.Height
	w.metrics.BlockHeight.WithLabelValues(chainId).Set(float64(block.Header.Height))
	w.metrics.ActiveSet.WithLabelValues(chainId).Set(float64(len(block.LastCommit.Signatures)))
	w.metrics.TrackedBlocks.WithLabelValues(chainId).Inc()

	info := NewBlockInfo(block)
	info.ValidatorStatus = w.computeValidatorStatus(block)

	w.handleBlockInfo(info)
}

func (w *BlockWatcher) computeValidatorStatus(block *types.Block) []ValidatorStatus {
	validatorStatus := []ValidatorStatus{}

	for _, val := range w.validators {
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
		validatorStatus = append(validatorStatus, ValidatorStatus{
			Address: val.Address,
			Label:   val.Name,
			Bonded:  bonded,
			Signed:  signed,
			Rank:    rank,
		})
	}

	return validatorStatus
}

func (w *BlockWatcher) handleBlockInfo(result *BlockInfo) {
	// Print block result & update metrics
	validatorStatus := []string{}
	for _, res := range result.ValidatorStatus {
		icon := "⚪️"
		if res.Signed {
			icon = "✅"
			w.metrics.ValidatedBlocks.WithLabelValues(result.ChainID, res.Address, res.Label).Inc()
		} else if res.Bonded {
			icon = "❌"
			w.metrics.MissedBlocks.WithLabelValues(result.ChainID, res.Address, res.Label).Inc()

			// Check if solo missed block
			if result.SignedRatio().GreaterThan(decimal.NewFromFloat(0.66)) {
				w.metrics.SoloMissedBlocks.WithLabelValues(result.ChainID, res.Address, res.Label).Inc()
			}
		}
		validatorStatus = append(validatorStatus, fmt.Sprintf("%s %s", icon, res.Label))
	}

	fmt.Fprintln(
		w.writer,
		color.YellowString(fmt.Sprintf("#%d", result.Height)),
		color.CyanString(fmt.Sprintf("%3d/%d validators", result.SignedValidators, result.TotalValidators)),
		strings.Join(validatorStatus, " "),
	)
}
