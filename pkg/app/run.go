package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/cometbft/cometbft/rpc/client/http"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cometbft/cometbft/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/fatih/color"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/exporter"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/watcher"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
)

func RunFunc(cCtx *cli.Context) error {
	var (
		ctx = cCtx.Context

		// Config flags
		chainID    = cCtx.String("chain-id")
		httpAddr   = cCtx.String("http-addr")
		namespace  = cCtx.String("namespace")
		noColor    = cCtx.Bool("no-color")
		nodes      = cCtx.StringSlice("node")
		validators = cCtx.StringSlice("validator")

		// Channels used to send data from watchers to the exporter
		blockChan      = make(chan watcher.NodeEvent[*types.Block], 10)
		statusChan     = make(chan watcher.NodeEvent[*ctypes.ResultStatus], 10)
		validatorsChan = make(chan watcher.NodeEvent[[]stakingtypes.Validator], 10)
	)

	//
	// Setup
	//
	// Logger setup
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Disable colored output if requested
	color.NoColor = noColor

	// Handle signals via context
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create errgroup to manage all goroutines
	errg, ctx := errgroup.WithContext(ctx)

	// Parse validators into name & address
	trackedValidators := []exporter.TrackedValidator{}
	for _, val := range validators {
		parts := strings.Split(val, ":")
		if len(parts) > 1 {
			log.Info().Msgf("tracking validator %s as %s", parts[0], parts[1])
			trackedValidators = append(trackedValidators, exporter.TrackedValidator{
				Address: parts[0],
				Name:    parts[1],
			})
		} else {
			log.Info().Msgf("tracking validator %s", parts[0])
			trackedValidators = append(trackedValidators, exporter.TrackedValidator{
				Address: parts[0],
				Name:    parts[0],
			})
		}
	}

	//
	// Start one watcher per node
	//
	watchers := make([]*watcher.Watcher, len(nodes))
	for i, endpoint := range nodes {
		i := i
		endpoint := endpoint
		errg.Go((func() error {
			rpcClient, err := http.New(endpoint, "/websocket")
			if err != nil {
				return fmt.Errorf("failed to create client: %w", err)
			}

			// First get node status or retry until successful
			status, err := retry.DoWithData(func() (*ctypes.ResultStatus, error) {
				return rpcClient.Status(ctx)
			},
				retry.Context(ctx),
				retry.Delay(1*time.Second),
				retry.MaxDelay(120*time.Second),
				retry.Attempts(0),
				retry.OnRetry(func(_ uint, err error) {
					log.Warn().Msgf("retrying connection to %s: %s", endpoint, err)
				}),
			)
			if err != nil {
				return fmt.Errorf("failed to get status: %w", err)
			}

			log.Info().Msgf("connected to %s", endpoint)
			statusChan <- watcher.NodeEvent[*ctypes.ResultStatus]{
				Endpoint: endpoint,
				Data:     status,
			}

			// Create & start the watcher
			watchers[i] = watcher.New(&watcher.Config{
				RpcClient:      rpcClient,
				BlockChan:      blockChan,
				StatusChan:     statusChan,
				ValidatorsChan: validatorsChan,
			})

			return watchers[i].Start(ctx)
		}))
	}

	//
	// Wait for 1 watcher to start and get the chain-id from it
	//
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timed out waiting for connection to nodes")
	case evt := <-statusChan:
		if chainID == "" {
			chainID = evt.Data.NodeInfo.Network
		}
		statusChan <- evt // put it back on the channel
	}

	//
	// Start exporter
	//
	exporter := exporter.New(&exporter.Config{
		TrackedValidators: trackedValidators,
		ChainID:           chainID,
		Metrics:           metrics.New(namespace, chainID),
		BlockChan:         blockChan,
		StatusChan:        statusChan,
		ValidatorsChan:    validatorsChan,
	})
	errg.Go(func() error {
		return exporter.Start(ctx)
	})

	//
	// Start HTTP server
	//
	log.Info().Msgf("starting HTTP server on %s", httpAddr)
	readyFn := func() bool {
		// ready when at least one watcher is ready
		for _, watcher := range watchers {
			if watcher.Ready() {
				return true
			}
		}
		return false
	}
	httpServer := NewHTTPServer(httpAddr, readyFn)
	errg.Go(func() error {
		return httpServer.Run()
	})

	//
	// Wait for context to be cancelled (via signals or error from errgroup)
	//
	<-ctx.Done()
	log.Info().Msg("shutting down")

	//
	// Stop all watchers and exporter
	//
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error().Err(fmt.Errorf("failed to stop http server: %w", err)).Msg("")
	}

	// Wait for all goroutines to finish
	return errg.Wait()
}
