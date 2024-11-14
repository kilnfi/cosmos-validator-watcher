package watcher

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cometbft/cometbft/types"
	"github.com/fatih/color"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/rpc"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/webhook"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
)

type BlockWebhook struct {
	Height   int64             `json:"height"`
	Metadata map[string]string `json:"metadata"`
}

type BlockWatcher struct {
	trackedValidators   []TrackedValidator
	metrics             *metrics.Metrics
	writer              io.Writer
	blockChan           chan *BlockInfo
	validatorSet        atomic.Value // []*types.Validator
	latestBlockHeight   int64
	latestBlockProposer string
	webhook             *webhook.Webhook
	customWebhooks      []BlockWebhook
}

func NewBlockWatcher(validators []TrackedValidator, metrics *metrics.Metrics, writer io.Writer, webhook *webhook.Webhook, customWebhooks []BlockWebhook) *BlockWatcher {
	return &BlockWatcher{
		trackedValidators: validators,
		metrics:           metrics,
		writer:            writer,
		blockChan:         make(chan *BlockInfo),
		webhook:           webhook,
		customWebhooks:    customWebhooks,
	}
}

func (w *BlockWatcher) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case block := <-w.blockChan:
			w.handleBlockInfo(ctx, block)
		}
	}
}

func (w *BlockWatcher) OnNodeStart(ctx context.Context, node *rpc.Node) error {
	if err := w.syncValidatorSet(ctx, node); err != nil {
		return fmt.Errorf("failed to sync validator set: %w", err)
	}

	blockResp, err := node.Client.Block(ctx, nil)
	if err != nil {
		log.Warn().Err(err).
			Str("node", node.Redacted()).
			Msg("failed to get latest block")
	} else {
		w.handleNodeBlock(node, blockResp.Block)
	}

	// Ticker to sync validator set
	ticker := time.NewTicker(time.Minute)

	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Debug().Err(ctx.Err()).Str("node", node.Redacted()).Msgf("stopping block watcher loop")
				return
			case <-ticker.C:
				if err := w.syncValidatorSet(ctx, node); err != nil {
					log.Error().Err(err).Str("node", node.Redacted()).Msg("failed to sync validator set")
				}
			}
		}
	}()

	return nil
}

func (w *BlockWatcher) OnNewBlock(ctx context.Context, node *rpc.Node, evt *ctypes.ResultEvent) error {
	// Ignore blocks if node is catching up
	if !node.IsSynced() {
		return nil
	}

	blockEvent := evt.Data.(types.EventDataNewBlock)
	block := blockEvent.Block

	w.handleNodeBlock(node, block)

	return nil
}

func (w *BlockWatcher) OnValidatorSetUpdates(ctx context.Context, node *rpc.Node, evt *ctypes.ResultEvent) error {
	// Ignore blocks if node is catching up
	if !node.IsSynced() {
		return nil
	}

	w.syncValidatorSet(ctx, node)

	return nil
}

func (w *BlockWatcher) handleNodeBlock(node *rpc.Node, block *types.Block) {
	validatorSet := w.getValidatorSet()

	if len(validatorSet) != block.LastCommit.Size() {
		log.Warn().Msgf("validator set size mismatch: %d vs %d", len(validatorSet), block.LastCommit.Size())
	}

	// Set node block height
	w.metrics.NodeBlockHeight.WithLabelValues(node.ChainID(), node.Endpoint()).Set(float64(block.Height))

	// Extract block info
	w.blockChan <- NewBlockInfo(block, w.computeValidatorStatus(block))
}

func (w *BlockWatcher) getValidatorSet() []*types.Validator {
	validatorSet := w.validatorSet.Load()
	if validatorSet == nil {
		return nil
	}

	return validatorSet.([]*types.Validator)
}

func (w *BlockWatcher) syncValidatorSet(ctx context.Context, n *rpc.Node) error {
	validators := make([]*types.Validator, 0)

	for i := 0; i < 5; i++ {
		var (
			page    int = i + 1
			perPage int = 100
		)

		result, err := n.Client.Validators(ctx, nil, &page, &perPage)
		if err != nil {
			return fmt.Errorf("failed to get validators: %w", err)
		}
		validators = append(validators, result.Validators...)

		if len(validators) >= int(result.Total) {
			break
		}
	}

	log.Debug().
		Str("node", n.Redacted()).
		Int("validators", len(validators)).
		Msgf("validator set")

	w.validatorSet.Store(validators)

	return nil
}

func (w *BlockWatcher) handleBlockInfo(ctx context.Context, block *BlockInfo) {
	chainId := block.ChainID

	if w.latestBlockHeight >= block.Height {
		// Skip already processed blocks
		return
	}

	// Ensure to inititalize counters for each validator
	for _, val := range w.trackedValidators {
		w.metrics.ValidatedBlocks.WithLabelValues(chainId, val.Address, val.Name)
		w.metrics.MissedBlocks.WithLabelValues(chainId, val.Address, val.Name)
		w.metrics.SoloMissedBlocks.WithLabelValues(chainId, val.Address, val.Name)
		w.metrics.ConsecutiveMissedBlocks.WithLabelValues(chainId, val.Address, val.Name)
	}
	w.metrics.SkippedBlocks.WithLabelValues(chainId)

	blockDiff := block.Height - w.latestBlockHeight
	if w.latestBlockHeight > 0 && blockDiff > 1 {
		log.Warn().Msgf("skipped %d unknown blocks", blockDiff-1)
		w.metrics.SkippedBlocks.WithLabelValues(chainId).Add(float64(blockDiff))
	}

	w.metrics.BlockHeight.WithLabelValues(chainId).Set(float64(block.Height))
	w.metrics.ActiveSet.WithLabelValues(chainId).Set(float64(block.TotalValidators))
	w.metrics.TrackedBlocks.WithLabelValues(chainId).Inc()
	w.metrics.Transactions.WithLabelValues(chainId).Add(float64(block.Transactions))

	// Print block result & update metrics
	validatorStatus := []string{}
	for _, res := range block.ValidatorStatus {
		icon := "⚪️"
		if w.latestBlockProposer == res.Address {
			icon = "👑"
			w.metrics.ProposedBlocks.WithLabelValues(block.ChainID, res.Address, res.Label).Inc()
			w.metrics.ValidatedBlocks.WithLabelValues(block.ChainID, res.Address, res.Label).Inc()
			w.metrics.ConsecutiveMissedBlocks.WithLabelValues(block.ChainID, res.Address, res.Label).Set(0)
		} else if res.Signed {
			icon = "✅"
			w.metrics.ValidatedBlocks.WithLabelValues(block.ChainID, res.Address, res.Label).Inc()
			w.metrics.ConsecutiveMissedBlocks.WithLabelValues(block.ChainID, res.Address, res.Label).Set(0)
		} else if res.Bonded {
			icon = "❌"
			w.metrics.MissedBlocks.WithLabelValues(block.ChainID, res.Address, res.Label).Inc()
			w.metrics.ConsecutiveMissedBlocks.WithLabelValues(block.ChainID, res.Address, res.Label).Inc()

			// Check if solo missed block
			if block.SignedRatio().GreaterThan(decimal.NewFromFloat(0.66)) {
				w.metrics.SoloMissedBlocks.WithLabelValues(block.ChainID, res.Address, res.Label).Inc()
			}
		}
		validatorStatus = append(validatorStatus, fmt.Sprintf("%s %s", icon, res.Label))
	}

	fmt.Fprintln(
		w.writer,
		color.YellowString(fmt.Sprintf("#%d", block.Height-1)),
		color.CyanString(fmt.Sprintf("%3d/%d validators", block.SignedValidators, block.TotalValidators)),
		color.GreenString(fmt.Sprintf("txs: %d", block.Transactions)),
		strings.Join(validatorStatus, " "),
	)

	// Handle webhooks
	w.handleWebhooks(ctx, block)

	w.latestBlockHeight = block.Height
	w.latestBlockProposer = block.ProposerAddress
}

func (w *BlockWatcher) computeValidatorStatus(block *types.Block) []ValidatorStatus {
	validatorStatus := []ValidatorStatus{}

	for _, val := range w.trackedValidators {
		bonded := w.isValidatorActive(val.Address)
		signed := false
		rank := 0
		for i, sig := range block.LastCommit.Signatures {
			if val.Address == sig.ValidatorAddress.String() {
				bonded = true
				signed = (sig.BlockIDFlag == types.BlockIDFlagCommit)
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

func (w *BlockWatcher) isValidatorActive(address string) bool {
	for _, val := range w.getValidatorSet() {
		if val.Address.String() == address {
			return true
		}
	}
	return false
}

func (w *BlockWatcher) handleWebhooks(ctx context.Context, block *BlockInfo) {
	if len(w.customWebhooks) == 0 {
		return
	}

	newWebhooks := []BlockWebhook{}

	for _, webhook := range w.customWebhooks {
		// If webhook block height is passed
		if webhook.Height <= block.Height {
			w.triggerWebhook(ctx, block.ChainID, webhook)
		} else {
			newWebhooks = append(newWebhooks, webhook)
		}
	}

	w.customWebhooks = newWebhooks
}

func (w *BlockWatcher) triggerWebhook(ctx context.Context, chainID string, wh BlockWebhook) {
	msg := make(map[string]string)
	msg["type"] = "custom"
	msg["block"] = fmt.Sprintf("%d", wh.Height)
	msg["chain_id"] = chainID
	for k, v := range wh.Metadata {
		msg[k] = v
	}

	go func() {
		if err := w.webhook.Send(context.Background(), msg); err != nil {
			log.Error().Err(err).Msg("failed to send upgrade webhook")
		}
	}()
}
