package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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
		httpAddr   = cCtx.String("http-addr")
		namespace  = cCtx.String("namespace")
		noColor    = cCtx.Bool("no-color")
		nodes      = cCtx.StringSlice("node")
		validators = cCtx.StringSlice("validator")
	)

	// Logger setup
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Disable colored output if requested
	color.NoColor = noColor

	// Handle signals via context
	ctx, stop := signal.NotifyContext(cCtx.Context, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	errg, ctx := errgroup.WithContext(ctx)

	// Channels used to send data from watchers to the exporter
	blockChan := make(chan *types.Block)
	validatorsChan := make(chan []stakingtypes.Validator)

	metrics := metrics.New(namespace)

	//
	// Start one watcher per node
	//
	watchers := make([]*watcher.Watcher, len(nodes))
	for i, endpoint := range nodes {
		log.Info().Msgf("connecting to node %s", endpoint)
		watcher, err := watcher.New(&watcher.Config{
			Endpoint:       endpoint,
			Metrics:        metrics,
			BlockChan:      blockChan,
			ValidatorsChan: validatorsChan,
		})
		if err != nil {
			return fmt.Errorf("failed to watch node %s: %w", endpoint, err)
		}
		i := i
		watchers[i] = watcher
		errg.Go(func() error {
			return watchers[i].Start(ctx)
		})
	}

	//
	// Start exporter
	//
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
	exporter := exporter.New(&exporter.Config{
		TrackedValidators: trackedValidators,
		Metrics:           metrics,
		BlockChan:         blockChan,
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

	close(blockChan)
	close(validatorsChan)

	// Wait for all goroutines to finish
	return errg.Wait()
}
