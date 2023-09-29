package watcher

import (
	"context"
	"fmt"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	gov "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	"github.com/cosmos/cosmos-sdk/x/upgrade/types"
	upgrade "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	"github.com/gogo/protobuf/codec"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/rpc"
	"github.com/rs/zerolog/log"
)

type UpgradeWatcher struct {
	metrics *metrics.Metrics
	pool    *rpc.Pool
	options UpgradeWatcherOptions
}

type UpgradeWatcherOptions struct {
	CheckPendingProposals bool
}

func NewUpgradeWatcher(metrics *metrics.Metrics, pool *rpc.Pool, options UpgradeWatcherOptions) *UpgradeWatcher {
	return &UpgradeWatcher{
		metrics: metrics,
		pool:    pool,
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

func (w *UpgradeWatcher) fetchUpgrade(ctx context.Context, node *rpc.Node) error {
	clientCtx := (client.Context{}).WithClient(node.Client)
	queryClient := upgrade.NewQueryClient(clientCtx)

	resp, err := queryClient.CurrentPlan(ctx, &upgrade.QueryCurrentPlanRequest{})
	if err != nil {
		return err
	}

	plan := resp.Plan

	if plan == nil && w.options.CheckPendingProposals {
		plan, err = w.checkUpgradeProposals(ctx, node)
		if err != nil {
			log.Error().Err(err).Msg("failed to check upgrade proposals")
		}
	}

	w.handleUpgradePlan(node.ChainID(), plan)

	return nil
}

func (w *UpgradeWatcher) checkUpgradeProposals(ctx context.Context, node *rpc.Node) (*upgrade.Plan, error) {
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
		if proposal.Content == nil {
			continue
		}

		cdc := codec.New(1)

		switch proposal.Content.TypeUrl {
		case "/cosmos.upgrade.v1beta1.CancelSoftwareUpgradeProposal":
			var upgrade types.SoftwareUpgradeProposal
			err := cdc.Unmarshal(proposal.Content.Value, &upgrade)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal software upgrade proposal: %w", err)
			}
			return &upgrade.Plan, nil

		case "/cosmos.upgrade.v1beta1.MsgSoftwareUpgrade":
			var upgrade types.MsgSoftwareUpgrade
			err := cdc.Unmarshal(proposal.Content.Value, &upgrade)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal software upgrade proposal: %w", err)
			}
			return &upgrade.Plan, nil
		}
	}

	return nil, nil
}

func (w *UpgradeWatcher) handleUpgradePlan(chainID string, plan *upgrade.Plan) {
	if plan == nil {
		w.metrics.UpgradePlan.Reset()
		return
	}

	w.metrics.UpgradePlan.WithLabelValues(chainID, plan.Name).Set(float64(plan.Height))
}