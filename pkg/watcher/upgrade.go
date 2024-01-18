package watcher

import (
	"context"
	"fmt"
	"time"

	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	comettypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/client"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	gov "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govbeta "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	"github.com/cosmos/cosmos-sdk/x/upgrade/types"
	upgrade "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	"github.com/gogo/protobuf/codec"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/rpc"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/webhook"
	"github.com/rs/zerolog/log"
)

type UpgradeWatcher struct {
	metrics *metrics.Metrics
	pool    *rpc.Pool
	webhook *webhook.Webhook
	options UpgradeWatcherOptions

	nextUpgradePlan   *upgrade.Plan // known upgrade plan
	latestBlockHeight int64         // latest block received
	latestWebhookSent int64         // latest block for which webhook has been sent
}

type UpgradeWatcherOptions struct {
	CheckPendingProposals bool
	GovModuleVersion      string
}

func NewUpgradeWatcher(metrics *metrics.Metrics, pool *rpc.Pool, webhook *webhook.Webhook, options UpgradeWatcherOptions) *UpgradeWatcher {
	return &UpgradeWatcher{
		metrics: metrics,
		pool:    pool,
		webhook: webhook,
		options: options,
	}
}

func (w *UpgradeWatcher) Start(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Minute)

	for {
		node := w.pool.GetSyncedNode()
		if node == nil {
			log.Warn().Msg("no node available to fetch upgrade plan")
		} else if err := w.fetchUpgrade(ctx, node); err != nil {
			log.Error().Err(err).Msg("failed to fetch upgrade plan")
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *UpgradeWatcher) OnNewBlock(ctx context.Context, node *rpc.Node, evt *ctypes.ResultEvent) error {
	// Ignore is webhook is not configured
	if w.webhook == nil {
		return nil
	}

	// Ignore if no upgrade plan
	if w.nextUpgradePlan == nil {
		return nil
	}

	// Ignore blocks if node is catching up
	if !node.IsSynced() {
		return nil
	}

	blockEvent := evt.Data.(comettypes.EventDataNewBlock)
	block := blockEvent.Block

	// Skip already processed blocks
	if w.latestBlockHeight >= block.Height {
		return nil
	}

	w.latestBlockHeight = block.Height

	// Ignore if upgrade plan is for a future block
	if block.Height < w.nextUpgradePlan.Height-1 {
		return nil
	}

	// Ignore if webhook has already been sent
	if w.latestWebhookSent >= w.nextUpgradePlan.Height {
		return nil
	}

	// Upgrade plan is for this block
	go w.triggerWebhook(ctx, node.ChainID(), *w.nextUpgradePlan)
	w.latestWebhookSent = w.nextUpgradePlan.Height
	w.nextUpgradePlan = nil

	return nil
}

func (w *UpgradeWatcher) triggerWebhook(ctx context.Context, chainID string, plan upgrade.Plan) {
	msg := struct {
		Type    string `json:"type"`
		Block   int64  `json:"block"`
		ChainID string `json:"chain_id"`
		Version string `json:"version"`
	}{
		Type:    "upgrade",
		Block:   plan.Height,
		ChainID: chainID,
		Version: plan.Name,
	}

	if err := w.webhook.Send(ctx, msg); err != nil {
		log.Error().Err(err).Msg("failed to send upgrade webhook")
	}
}

func (w *UpgradeWatcher) fetchUpgrade(ctx context.Context, node *rpc.Node) error {
	clientCtx := (client.Context{}).WithClient(node.Client)
	queryClient := upgrade.NewQueryClient(clientCtx)

	resp, err := queryClient.CurrentPlan(ctx, &upgrade.QueryCurrentPlanRequest{})
	if err != nil {
		return err
	}

	plan := resp.Plan

	if plan == nil && w.options.CheckPendingProposals {
		switch w.options.GovModuleVersion {
		case "v1beta1":
			plan, err = w.checkUpgradeProposalsV1Beta1(ctx, node)
		default: // v1
			plan, err = w.checkUpgradeProposalsV1(ctx, node)
		}
		if err != nil {
			log.Error().Err(err).Msg("failed to check upgrade proposals")
		}
	}

	w.handleUpgradePlan(node.ChainID(), plan)

	return nil
}

func (w *UpgradeWatcher) checkUpgradeProposalsV1(ctx context.Context, node *rpc.Node) (*upgrade.Plan, error) {
	clientCtx := (client.Context{}).WithClient(node.Client)
	queryClient := gov.NewQueryClient(clientCtx)

	// Fetch all proposals in voting period
	proposalsResp, err := queryClient.Proposals(ctx, &gov.QueryProposalsRequest{
		ProposalStatus: gov.StatusVotingPeriod,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get proposals: %w", err)
	}

	for _, proposal := range proposalsResp.GetProposals() {
		for _, message := range proposal.Messages {
			plan, err := extractUpgradePlan(message)
			if err != nil {
				return nil, fmt.Errorf("failed to extract upgrade plan: %w", err)
			}
			if plan != nil {
				return plan, nil
			}
		}
	}

	return nil, nil
}

func (w *UpgradeWatcher) checkUpgradeProposalsV1Beta1(ctx context.Context, node *rpc.Node) (*upgrade.Plan, error) {
	clientCtx := (client.Context{}).WithClient(node.Client)
	queryClient := govbeta.NewQueryClient(clientCtx)

	// Fetch all proposals in voting period
	proposalsResp, err := queryClient.Proposals(ctx, &govbeta.QueryProposalsRequest{
		ProposalStatus: govbeta.StatusVotingPeriod,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get proposals: %w", err)
	}

	for _, proposal := range proposalsResp.GetProposals() {
		plan, err := extractUpgradePlan(proposal.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to extract upgrade plan: %w", err)
		}
		if plan != nil {
			return plan, nil
		}
	}

	return nil, nil
}

func extractUpgradePlan(content *codectypes.Any) (*upgrade.Plan, error) {
	if content == nil {
		return nil, nil
	}

	cdc := codec.New(1)

	switch content.TypeUrl {
	case "/cosmos.upgrade.v1beta1.SoftwareUpgradeProposal":
		var upgrade types.SoftwareUpgradeProposal
		err := cdc.Unmarshal(content.Value, &upgrade)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal software upgrade proposal: %w", err)
		}
		return &upgrade.Plan, nil

	case "/cosmos.upgrade.v1beta1.MsgSoftwareUpgrade":
		var upgrade types.MsgSoftwareUpgrade
		err := cdc.Unmarshal(content.Value, &upgrade)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal software upgrade proposal: %w", err)
		}
		return &upgrade.Plan, nil
	}

	return nil, nil
}

func (w *UpgradeWatcher) handleUpgradePlan(chainID string, plan *upgrade.Plan) {
	if plan == nil {
		w.metrics.UpgradePlan.Reset()
		return
	}

	w.nextUpgradePlan = plan
	w.metrics.UpgradePlan.WithLabelValues(chainID, plan.Name).Set(float64(plan.Height))
}
