package watcher

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	checkpointing "github.com/babylonlabs-io/babylon/x/checkpointing/types"
	epoching "github.com/babylonlabs-io/babylon/x/epoching/types"
	finality "github.com/babylonlabs-io/babylon/x/finality/types"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	tmtypes "github.com/cometbft/cometbft/proto/tendermint/types"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/std"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/fatih/color"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/rpc"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
)

type BabylonFinalityProvider struct {
	Address string
	Label   string
}

func ParseBabylonFinalityProvider(val string) BabylonFinalityProvider {
	parts := strings.Split(val, ":")
	if len(parts) > 1 {
		return BabylonFinalityProvider{
			Address: parts[0],
			Label:   parts[1],
		}
	}

	return BabylonFinalityProvider{
		Address: parts[0],
		Label:   parts[0],
	}
}

type BabylonWatcher struct {
	trackedValidators []TrackedValidator
	finalityProviders []BabylonFinalityProvider
	pool              *rpc.Pool
	metrics           *metrics.Metrics
	writer            io.Writer
	blockChan         chan *types.Block
	latestBlockHeight int64
	protoCodec        *codec.ProtoCodec
	epochInterval     int64
}

func NewBabylonWatcher(validators []TrackedValidator, finalityProviders []BabylonFinalityProvider, pool *rpc.Pool, metrics *metrics.Metrics, writer io.Writer) *BabylonWatcher {
	// Create a new Protobuf codec to decode babylon messages
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	std.RegisterInterfaces(interfaceRegistry)
	checkpointing.RegisterInterfaces(interfaceRegistry)
	finality.RegisterInterfaces(interfaceRegistry)
	protoCodec := codec.NewProtoCodec(interfaceRegistry)

	return &BabylonWatcher{
		trackedValidators: validators,
		finalityProviders: finalityProviders,
		pool:              pool,
		metrics:           metrics,
		writer:            writer,
		protoCodec:        protoCodec,
		blockChan:         make(chan *types.Block),
		epochInterval:     360,
	}
}

func (w *BabylonWatcher) Start(ctx context.Context) error {
	if err := w.syncEpochParams(ctx); err != nil {
		return err
	}

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case block := <-w.blockChan:
			w.handleBlock(block)
		case <-ticker.C:
			if err := w.syncEpochParams(ctx); err != nil {
				log.Error().Err(err).Msg("failed to sync epoch params")
			}
		}
	}
}

func (w *BabylonWatcher) OnNewBlock(ctx context.Context, node *rpc.Node, evt *ctypes.ResultEvent) error {
	// Ignore blocks if node is catching up
	if !node.IsSynced() {
		return nil
	}

	blockEvent := evt.Data.(types.EventDataNewBlock)
	w.blockChan <- blockEvent.Block

	return nil
}

func (w *BabylonWatcher) syncEpochParams(ctx context.Context) error {
	node := w.pool.GetSyncedNode()
	if node == nil {
		return fmt.Errorf("no node available to fetch upgrade plan")
	}

	chainID := node.ChainID()
	w.metrics.BabylonCheckpointVote.WithLabelValues(chainID).Add(0)
	w.metrics.BabylonFinalityVotes.WithLabelValues(chainID).Add(0)

	clientCtx := (client.Context{}).WithClient(node.Client)
	queryClient := epoching.NewQueryClient(clientCtx)
	resp, err := queryClient.EpochsInfo(ctx, &epoching.QueryEpochsInfoRequest{
		Pagination: &query.PageRequest{
			Limit:   1,
			Reverse: true,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to fetch epoch params: %w", err)
	}
	if len(resp.Epochs) == 0 {
		return fmt.Errorf("no epoch params found")
	}

	w.epochInterval = int64(resp.Epochs[0].CurrentEpochInterval)
	w.metrics.BabylonEpoch.WithLabelValues(chainID).Set(float64(resp.Epochs[0].EpochNumber))

	log.Debug().
		Uint64("epoch", resp.Epochs[0].EpochNumber).
		Int64("epoch-interval", w.epochInterval).
		Msg("babylon epoch interval")

	return nil
}

func (w *BabylonWatcher) handleBlock(block *types.Block) {
	var (
		// chainID = block.Header.ChainID
		blockHeight = block.Height
	)

	// Skip if the block is already known
	if w.latestBlockHeight >= blockHeight {
		return
	}
	w.latestBlockHeight = blockHeight

	chainID := block.Header.ChainID

	// Handle finality signature for each block
	msgs := w.findMsgAddFinalitySig(block.Txs)
	w.handleMsgAddFinalitySig(chainID, blockHeight, msgs)

	// Handle injectied checkpoint on the first block of an epoch
	if blockHeight%w.epochInterval == 1 {
		msg := w.findMsgInjectedCheckpoint(block.Txs)
		w.handleMsgInjectedCheckpoint(chainID, blockHeight, msg)
	}
}

func (w *BabylonWatcher) findMsgAddFinalitySig(txs types.Txs) []*finality.MsgAddFinalitySig {
	var msgs []*finality.MsgAddFinalitySig

	for _, txBytes := range txs {
		var tx tx.Tx

		// Unmarshal the transaction and ignore non-decodable txs
		if err := w.protoCodec.Unmarshal(txBytes, &tx); err != nil {
			continue
		}

		for _, msg := range tx.Body.Messages {
			if msg.TypeUrl != "/babylon.finality.v1.MsgAddFinalitySig" {
				continue
			}

			var cpTx finality.MsgAddFinalitySig
			if err := w.protoCodec.Unmarshal(msg.Value, &cpTx); err != nil {
				log.Warn().Msgf("failed to decode msg type MsgAddFinalitySig: %v", err)
				return nil
			}
			msgs = append(msgs, &cpTx)
		}
	}

	return msgs
}

func (w *BabylonWatcher) handleMsgAddFinalitySig(chainID string, blockHeight int64, msgs []*finality.MsgAddFinalitySig) {
	if len(msgs) == 0 {
		log.Warn().Int64("height", blockHeight).Msgf("babylon finality provider signature not found")
		return
	}

	w.metrics.BabylonFinalityVotes.WithLabelValues(chainID).Inc()

	validatorStatus := []string{}
	for _, fp := range w.finalityProviders {
		_, hasSigned := lo.Find(msgs, func(msg *finality.MsgAddFinalitySig) bool {
			return msg.Signer == fp.Address
		})
		icon := "??"
		if hasSigned {
			icon = "✅"
			w.metrics.BabylonCommittedFinalityVotes.WithLabelValues(chainID, fp.Address, fp.Label).Inc()
			w.metrics.BabylonConsecutiveMissedFinalityVotes.WithLabelValues(chainID, fp.Address, fp.Label).Set(0)
		} else {
			icon = "❌"
			w.metrics.BabylonMissedFinalityVotes.WithLabelValues(chainID, fp.Address, fp.Label).Inc()
			w.metrics.BabylonConsecutiveMissedFinalityVotes.WithLabelValues(chainID, fp.Address, fp.Label).Inc()
		}
		validatorStatus = append(validatorStatus, fmt.Sprintf("%s %s", icon, fp.Label))
	}

	fmt.Fprintln(
		w.writer,
		color.YellowString(fmt.Sprintf("#%d", blockHeight-1)),
		color.MagentaString(fmt.Sprintf("%3d finality prvds", len(msgs))),
		strings.Join(validatorStatus, " "),
	)
}

func (w *BabylonWatcher) findMsgInjectedCheckpoint(txs types.Txs) *checkpointing.MsgInjectedCheckpoint {
	for _, txBytes := range txs {
		var tx tx.Tx

		// Unmarshal the transaction and ignore non-decodable txs
		if err := w.protoCodec.Unmarshal(txBytes, &tx); err != nil {
			continue
		}

		for _, msg := range tx.Body.Messages {
			if msg.TypeUrl != "/babylon.checkpointing.v1.MsgInjectedCheckpoint" {
				continue
			}

			var cpTx checkpointing.MsgInjectedCheckpoint
			if err := w.protoCodec.Unmarshal(msg.Value, &cpTx); err != nil {
				log.Warn().Msgf("failed to decode msg type MsgInjectedCheckpoint: %v", err)
				return nil
			}
			return &cpTx
		}
	}

	return nil
}

func (w *BabylonWatcher) handleMsgInjectedCheckpoint(chainID string, blockHeight int64, msg *checkpointing.MsgInjectedCheckpoint) {
	if msg == nil {
		log.Warn().Int64("height", blockHeight).Msgf("babylon checkpoint not found on new epoch")
		return
	}

	var (
		epoch           = msg.Ckpt.Ckpt.EpochNum
		totalValidators = len(msg.ExtendedCommitInfo.GetVotes())
	)

	w.metrics.BabylonEpoch.WithLabelValues(chainID).Set(float64(epoch))
	w.metrics.BabylonCheckpointVote.WithLabelValues(chainID).Inc()

	validatorVotes := make(map[string]bool)
	for _, val := range w.trackedValidators {
		validatorVotes[val.Address] = false
	}

	// Check if validator has voted
	validatorStatus := []string{}
	for _, val := range w.trackedValidators {
		_, hasVoted := lo.Find(msg.ExtendedCommitInfo.Votes, func(vote abcitypes.ExtendedVoteInfo) bool {
			return strings.ToUpper(hex.EncodeToString(vote.Validator.Address)) == val.Address
		})
		icon := "??"
		if hasVoted {
			icon = "✅"
			w.metrics.BabylonCommittedCheckpointVote.WithLabelValues(chainID, val.Address, val.Name).Inc()
			w.metrics.BabylonConsecutiveMissedCheckpointVote.WithLabelValues(chainID, val.Address, val.Name).Set(0)
		} else {
			icon = "❌"
			w.metrics.BabylonMissedCheckpointVote.WithLabelValues(chainID, val.Address, val.Name).Inc()
			w.metrics.BabylonConsecutiveMissedCheckpointVote.WithLabelValues(chainID, val.Address, val.Name).Inc()
		}
		validatorStatus = append(validatorStatus, fmt.Sprintf("%s %s", icon, val.Name))
	}

	votedValidators := lo.CountBy(msg.ExtendedCommitInfo.Votes, func(vote abcitypes.ExtendedVoteInfo) bool {
		return vote.BlockIdFlag == tmtypes.BlockIDFlagCommit
	})

	fmt.Fprintln(
		w.writer,
		color.BlueString(fmt.Sprintf("Epoch #%d", epoch)),
		color.CyanString(fmt.Sprintf("%3d/%d validators", votedValidators, totalValidators)),
		strings.Join(validatorStatus, " "),
	)
}
