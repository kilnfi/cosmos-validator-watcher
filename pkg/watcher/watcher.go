package watcher

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/cometbft/cometbft/rpc/client/http"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/types/query"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/rs/zerolog/log"
)

type Config struct {
	RpcClient      *http.HTTP
	StatusChan     chan<- NodeEvent[*ctypes.ResultStatus]
	BlockChan      chan<- NodeEvent[*types.Block]
	ValidatorsChan chan<- NodeEvent[[]stakingtypes.Validator]
	DisableStaking bool
}

type Watcher struct {
	cfg *Config

	isSynced   atomic.Bool
	rpcClient  *http.HTTP
	validators []*types.Validator
}

func New(config *Config) *Watcher {
	return &Watcher{
		cfg:        config,
		rpcClient:  config.RpcClient,
		validators: make([]*types.Validator, 0),
	}
}

func (w *Watcher) Ready() bool {
	return w.isSynced.Load()
}

func (w *Watcher) Start(ctx context.Context) error {
	// Start the websocket process
	if err := w.rpcClient.Start(); err != nil {
		return fmt.Errorf("failed to start client: %w", err)
	}
	defer func() {
		if err := w.rpcClient.Stop(); err != nil {
			log.Warn().Err(err).Msg("failed to stop client")
		}
	}()

	// Sync the initial state
	if err := w.sync(ctx); err != nil {
		return fmt.Errorf("failed to init sync: %w", err)
	}

	// Subscribe to new blocks
	blockEvents, err := w.rpcClient.Subscribe(ctx, "cosmos-validator-watcher", "tm.event='NewBlock'")
	if err != nil {
		return fmt.Errorf("failed to subscribe to events: %w", err)
	}

	// Subscribe to validator set changes
	validatorEvents, err := w.rpcClient.Subscribe(ctx, "cosmos-validator-watcher", "tm.event='ValidatorSetUpdates'")
	if err != nil {
		return fmt.Errorf("failed to subscribe to events: %w", err)
	}

	// Start a ticker to sync validators every X minutes
	syncTicker := time.NewTicker(1 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			return nil

		case evt := <-blockEvents:
			blockEvent := evt.Data.(types.EventDataNewBlock)
			// w.cfg.Metrics.NodeBlockHeight.WithLabelValues(w.cfg.Endpoint).Set(float64(blockEvent.Block.Header.Height))
			w.cfg.BlockChan <- NodeEvent[*types.Block]{
				Endpoint: w.rpcClient.Remote(),
				Data:     w.enhanceBlock(blockEvent.Block),
			}

		case evt := <-validatorEvents:
			_ = evt.Data.(types.EventDataValidatorSetUpdates)
			if err := w.syncValidators(ctx); err != nil {
				log.Warn().Err(err).Msg("failed to get validators")
			}

		case <-syncTicker.C:
			if err := w.sync(ctx); err != nil {
				log.Warn().Err(err).Msg("")
			}
		}
	}
}

func (w *Watcher) sync(ctx context.Context) error {
	// Get node status
	if err := w.syncStatus(ctx); err != nil {
		return fmt.Errorf("failed to get node status: %w", err)
	}

	// Get active validator set
	if err := w.syncValidators(ctx); err != nil {
		return fmt.Errorf("failed to get current validator set: %w", err)
	}

	// Get x/staking validators status
	if err := w.syncStakingValidators(ctx); err != nil {
		return fmt.Errorf("failed to get validators status: %w", err)
	}

	return nil
}

// syncStatus fetches the current node status and updates the metrics
func (w *Watcher) syncStatus(ctx context.Context) error {
	retryOpts := []retry.Option{
		retry.Context(ctx),
		retry.Delay(1 * time.Second),
		retry.Attempts(3),
		retry.OnRetry(func(n uint, err error) {
			log.Warn().Msgf("retrying connection to %s: %s", w.rpcClient.Remote(), err)
		}),
	}
	status, err := retry.DoWithData(func() (*ctypes.ResultStatus, error) {
		return w.rpcClient.Status(ctx)
	}, retryOpts...)

	// Send the status to the exporter, even if it's nil
	w.cfg.StatusChan <- NodeEvent[*ctypes.ResultStatus]{
		Endpoint: w.rpcClient.Remote(),
		Data:     status,
	}

	if err != nil {
		w.isSynced.Store(false)
		// w.cfg.Metrics.NodeSynced.WithLabelValues(w.cfg.Endpoint).Set(0)
		return fmt.Errorf("failed to get node status: %w", err)
	}

	w.isSynced.Store(!status.SyncInfo.CatchingUp)

	return nil
}

// syncValidators fetches the current active validator set
func (w *Watcher) syncValidators(ctx context.Context) error {
	validators, err := w.getValidators(ctx)
	if err != nil {
		return err
	}

	// Check that all validators have a non-empty address
	for i, val := range validators {
		if val.Address.String() == "" {
			log.Warn().Msgf("empty validator address in active set at pos %d", i)
		}
	}
	w.validators = validators

	return nil
}

// enhanceBlock fills the missing signatures using the validator address from the known active set
func (w *Watcher) enhanceBlock(block *types.Block) *types.Block {
	if len(w.validators) != block.LastCommit.Size() {
		log.Warn().Msgf("validator set size mismatch: %d vs %d", len(w.validators), block.LastCommit.Size())
		return block
	}

	signatures := make([]types.CommitSig, len(block.LastCommit.Signatures))
	for i, sig := range block.LastCommit.Signatures {
		if len(sig.ValidatorAddress) == 0 {
			// Fill the validator address when it's empty (happens when validator miss the block)
			sig.ValidatorAddress = w.validators[i].Address
		} else if w.validators[i].Address.String() != sig.ValidatorAddress.String() {
			// Check that the validator address is correct compared to the active set
			log.Warn().Msgf("validator set mismatch pos %d: expected %s got %s", i, w.validators[i].Address, sig.ValidatorAddress.String())
		}
		signatures[i] = sig
	}

	block.LastCommit.Signatures = signatures

	return block
}

// getValidators returns all validators
func (w *Watcher) getValidators(ctx context.Context) ([]*types.Validator, error) {
	validators := make([]*types.Validator, 0)

	for i := 0; i < 5; i++ {
		var (
			page    int = i + 1
			perPage int = 100
		)

		result, err := w.rpcClient.Validators(ctx, nil, &page, &perPage)
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

func (w *Watcher) syncStakingValidators(ctx context.Context) error {
	if w.cfg.DisableStaking || !w.Ready() {
		return nil
	}

	clientCtx := (client.Context{}).WithClient(w.rpcClient)
	queryClient := stakingtypes.NewQueryClient(clientCtx)

	validators, err := queryClient.Validators(ctx, &stakingtypes.QueryValidatorsRequest{
		Pagination: &query.PageRequest{
			Limit: 3000,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to get validators: %w", err)
	}

	w.cfg.ValidatorsChan <- NodeEvent[[]stakingtypes.Validator]{
		Endpoint: w.rpcClient.Remote(),
		Data:     validators.Validators,
	}

	return nil
}
