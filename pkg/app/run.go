package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cometbft/cometbft/rpc/client/http"
	"github.com/fatih/color"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/rpc"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/watcher"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
)

func RunFunc(cCtx *cli.Context) error {
	var (
		ctx = cCtx.Context

		// Config flags
		chainID    = cCtx.String("chain-id")
		httpAddr   = cCtx.String("http-addr")
		logLevel   = cCtx.String("log-level")
		namespace  = cCtx.String("namespace")
		noColor    = cCtx.Bool("no-color")
		nodes      = cCtx.StringSlice("node")
		noStaking  = cCtx.Bool("no-staking")
		validators = cCtx.StringSlice("validator")
	)

	//
	// Setup
	//
	// Logger setup
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	switch logLevel {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		return fmt.Errorf("invalid log level: %s", logLevel)
	}

	// Disable colored output if requested
	color.NoColor = noColor

	// Handle signals via context
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create errgroup to manage all goroutines
	errg, ctx := errgroup.WithContext(ctx)

	// Parse validators into name & address
	trackedValidators := lo.Map(validators, func(v string, _ int) watcher.TrackedValidator {
		return watcher.ParseValidator(v)
	})
	for _, v := range trackedValidators {
		log.Info().Str("alias", v.Name).Msgf("tracking validator %s", v.Address)
	}

	//
	// Node Watchers
	//
	metrics := metrics.New(namespace)
	metrics.Register()
	blockWatcher := watcher.NewBlockWatcher(trackedValidators, metrics, os.Stdout)
	errg.Go(func() error {
		return blockWatcher.Start(ctx)
	})
	statusWatcher := watcher.NewStatusWatcher(chainID, metrics)
	errg.Go(func() error {
		return statusWatcher.Start(ctx)
	})

	//
	// Nodes
	//
	rpcNodes := make([]*rpc.Node, len(nodes))
	for i, endpoint := range nodes {
		client, err := http.New(endpoint, "/websocket")
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}
		rpcNodes[i] = rpc.NewNode(
			client,
			rpc.WithOnStart(blockWatcher.OnNodeStart),
			rpc.WithOnStatus(statusWatcher.OnNodeStatus),
		)
	}
	pool := rpc.NewPool(rpcNodes)
	errg.Go(func() error {
		return pool.Start(ctx)
	})

	//
	// Wait for connection
	//
	select {
	case <-ctx.Done():
		// cancelled before any nodes started
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timed out waiting for connection to nodes")
	case <-pool.Started():
		// at least one node started
	}

	//
	// Pool watchers
	//
	if !noStaking {
		validatorsWatcher := watcher.NewValidatorsWatcher(trackedValidators, metrics, pool)
		errg.Go(func() error {
			return validatorsWatcher.Start(ctx)
		})
	}

	//
	// HTTP server
	//
	log.Info().Msgf("starting HTTP server on %s", httpAddr)
	readyFn := func() bool {
		// ready when at least one watcher is ready
		for _, node := range pool.Nodes {
			if node.IsSynced() {
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

	if err := pool.Stop(ctx); err != nil {
		log.Error().Err(fmt.Errorf("failed to stop node pool: %w", err)).Msg("")
	}
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error().Err(fmt.Errorf("failed to stop http server: %w", err)).Msg("")
	}

	// Wait for all goroutines to finish
	return errg.Wait()
}
