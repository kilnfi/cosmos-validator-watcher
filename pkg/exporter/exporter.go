package exporter

import (
	"context"
	"io"
	"os"

	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cometbft/cometbft/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/watcher"
)

// TrackedValidator adds the ability to attach a custom name to a validator address
type TrackedValidator struct {
	Address string
	Name    string
}

type Config struct {
	TrackedValidators []TrackedValidator

	ChainID        string
	Writer         io.Writer
	Metrics        *metrics.Metrics
	BlockChan      <-chan watcher.NodeEvent[*types.Block]
	StatusChan     <-chan watcher.NodeEvent[*ctypes.ResultStatus]
	ValidatorsChan <-chan watcher.NodeEvent[[]stakingtypes.Validator]
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
			if err := e.handleBlock(block); err != nil {
				return err
			}
		case status := <-e.cfg.StatusChan:
			if err := e.handleStatus(status); err != nil {
				return err
			}
		case validators := <-e.cfg.ValidatorsChan:
			if err := e.handleValidators(validators); err != nil {
				return err
			}
		}
	}
}
