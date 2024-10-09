package app

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/types/query"
	staking "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/fatih/color"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/crypto"
	_ "github.com/kilnfi/cosmos-validator-watcher/pkg/crypto"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/rpc"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/watcher"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/webhook"
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
		chainID             = cCtx.String("chain-id")
		httpAddr            = cCtx.String("http-addr")
		logLevel            = cCtx.String("log-level")
		namespace           = cCtx.String("namespace")
		noColor             = cCtx.Bool("no-color")
		nodes               = cCtx.StringSlice("node")
		noGov               = cCtx.Bool("no-gov")
		noStaking           = cCtx.Bool("no-staking")
		noUpgrade           = cCtx.Bool("no-upgrade")
		noCommission        = cCtx.Bool("no-commission")
		denom               = cCtx.String("denom")
		denomExpon          = cCtx.Uint("denom-exponent")
		startTimeout        = cCtx.Duration("start-timeout")
		stopTimeout         = cCtx.Duration("stop-timeout")
		validators          = cCtx.StringSlice("validator")
		webhookURL          = cCtx.String("webhook-url")
		webhookCustomBlocks = cCtx.StringSlice("webhook-custom-block")
		xGov                = cCtx.String("x-gov")
	)

	//
	// Setup
	//
	// Logger setup
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	zerolog.SetGlobalLevel(logLevelFromString(logLevel))

	// Disable colored output if requested
	color.NoColor = noColor

	// Handle signals via context
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create errgroup to manage all goroutines
	errg, ctx := errgroup.WithContext(ctx)

	// Timeout to wait for at least one node to be ready
	startCtx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()

	// Test connection to nodes
	pool, err := createNodePool(startCtx, nodes)
	if err != nil {
		return err
	}

	// Parse validators into name & address
	trackedValidators, err := createTrackedValidators(ctx, pool, validators, noStaking)
	if err != nil {
		return err
	}

	var wh *webhook.Webhook
	if webhookURL != "" {
		whURL, err := url.Parse(webhookURL)
		if err != nil {
			return fmt.Errorf("failed to parse webhook endpoint: %w", err)
		}
		wh = webhook.New(*whURL)
	}

	// Custom block webhooks
	blockWebhooks := []watcher.BlockWebhook{}
	for _, block := range webhookCustomBlocks {
		blockHeight, err := strconv.ParseInt(block, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse block height for custom webhook (%s): %w", block, err)
		}
		blockWebhooks = append(blockWebhooks, watcher.BlockWebhook{
			Height:   blockHeight,
			Metadata: map[string]string{},
		})
	}

	//
	// Node Watchers
	//
	metrics := metrics.New(namespace)
	metrics.Register()
	blockWatcher := watcher.NewBlockWatcher(trackedValidators, metrics, os.Stdout, wh, blockWebhooks)
	errg.Go(func() error {
		return blockWatcher.Start(ctx)
	})
	statusWatcher := watcher.NewStatusWatcher(chainID, metrics)
	errg.Go(func() error {
		return statusWatcher.Start(ctx)
	})
	if !noCommission {
		commissionWatcher := watcher.NewCommissionsWatcher(trackedValidators, metrics, pool)
		errg.Go(func() error {
			return commissionWatcher.Start(ctx)
		})
	}

	//
	// Pool watchers
	//
	if !noStaking {
		validatorsWatcher := watcher.NewValidatorsWatcher(trackedValidators, metrics, pool, watcher.ValidatorsWatcherOptions{
			Denom:         denom,
			DenomExponent: denomExpon,
		})
		errg.Go(func() error {
			return validatorsWatcher.Start(ctx)
		})
	}
	if xGov != "v1beta1" && xGov != "v1" {
		log.Warn().Msgf("unknown gov module version: %s (fallback to v1)", xGov)
		xGov = "v1"
	}
	if !noGov {
		votesWatcher := watcher.NewVotesWatcher(trackedValidators, metrics, pool, watcher.VotesWatcherOptions{
			GovModuleVersion: xGov,
		})
		errg.Go(func() error {
			return votesWatcher.Start(ctx)
		})
	}

	var upgradeWatcher *watcher.UpgradeWatcher
	if !noUpgrade {
		upgradeWatcher = watcher.NewUpgradeWatcher(metrics, pool, wh, watcher.UpgradeWatcherOptions{
			CheckPendingProposals: !noGov,
			GovModuleVersion:      xGov,
		})
		errg.Go(func() error {
			return upgradeWatcher.Start(ctx)
		})
	}

	//
	// Register watchers on nodes events
	//
	for _, node := range pool.Nodes {
		node.OnStart(blockWatcher.OnNodeStart)
		node.OnStatus(statusWatcher.OnNodeStatus)
		node.OnEvent(rpc.EventNewBlock, blockWatcher.OnNewBlock)

		if upgradeWatcher != nil {
			node.OnEvent(rpc.EventNewBlock, upgradeWatcher.OnNewBlock)
		}
	}

	//
	// Start Pool
	//
	errg.Go(func() error {
		return pool.Start(ctx)
	})

	//
	// HTTP server
	//
	log.Info().Msgf("starting HTTP server on %s", httpAddr)
	readyProbe := func() bool {
		// ready when at least one watcher is synced
		return pool.GetSyncedNode() != nil
	}
	httpServer := NewHTTPServer(
		httpAddr,
		WithReadyProbe(readyProbe),
		WithLiveProbe(upProbe),
		WithMetrics(metrics.Registry),
	)
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
	ctx, cancel = context.WithTimeout(context.Background(), stopTimeout)
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

func logLevelFromString(level string) zerolog.Level {
	switch level {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

func createNodePool(ctx context.Context, nodes []string) (*rpc.Pool, error) {
	rpcNodes := make([]*rpc.Node, len(nodes))
	for i, endpoint := range nodes {
		client, err := http.New(endpoint, "/websocket")
		if err != nil {
			return nil, fmt.Errorf("failed to create client: %w", err)
		}

		opts := []rpc.NodeOption{}

		// Check is query string websocket is present in the endpoint
		if u, err := url.Parse(endpoint); err == nil {
			if u.Query().Get("__websocket") == "0" {
				opts = append(opts, rpc.DisableWebsocket())
			}
		}

		rpcNodes[i] = rpc.NewNode(client, opts...)

		status, err := rpcNodes[i].Status(ctx)
		if err != nil {
			log.Error().Err(err).Msgf("failed to connect to %s", rpcNodes[i].Redacted())
			continue
		}

		chainID := status.NodeInfo.Network
		blockHeight := status.SyncInfo.LatestBlockHeight

		logger := log.With().Int64("height", blockHeight).Str("chainID", chainID).Logger()

		if rpcNodes[i].IsSynced() {
			logger.Info().Msgf("connected to %s", rpcNodes[i].Redacted())
		} else {
			logger.Warn().Msgf("connected to %s (but node is catching up)", rpcNodes[i].Redacted())
		}
	}

	var rpcNode *rpc.Node
	var chainID string
	for _, node := range rpcNodes {
		if chainID == "" {
			chainID = node.ChainID()
		} else if chainID != node.ChainID() && node.ChainID() != "" {
			return nil, fmt.Errorf("nodes are on different chains: %s != %s", chainID, node.ChainID())
		}
		if node.IsSynced() {
			rpcNode = node
		}
	}
	if rpcNode == nil {
		return nil, fmt.Errorf("no nodes synced")
	}

	return rpc.NewPool(chainID, rpcNodes), nil
}

func createTrackedValidators(ctx context.Context, pool *rpc.Pool, validators []string, noStaking bool) ([]watcher.TrackedValidator, error) {
	var stakingValidators []staking.Validator
	if !noStaking {
		node := pool.GetSyncedNode()
		clientCtx := (client.Context{}).WithClient(node.Client)
		queryClient := staking.NewQueryClient(clientCtx)

		resp, err := queryClient.Validators(ctx, &staking.QueryValidatorsRequest{
			Pagination: &query.PageRequest{
				Limit: 3000,
			},
		})
		if err != nil {
			return nil, err
		}
		stakingValidators = resp.Validators
	}

	trackedValidators := lo.Map(validators, func(v string, _ int) watcher.TrackedValidator {
		val := watcher.ParseValidator(v)

		for _, stakingVal := range stakingValidators {
			address := crypto.PubKeyAddress(stakingVal.ConsensusPubkey)
			if address == val.Address {
				val.Moniker = stakingVal.Description.Moniker
				val.OperatorAddress = stakingVal.OperatorAddress
			}
		}

		log.Info().
			Str("alias", val.Name).
			Str("moniker", val.Moniker).
			Msgf("tracking validator %s", val.Address)

		log.Debug().
			Str("account", val.AccountAddress()).
			Str("address", val.Address).
			Str("alias", val.Name).
			Str("moniker", val.Moniker).
			Str("operator", val.OperatorAddress).
			Msgf("validator info")

		return val
	})

	return trackedValidators, nil
}
