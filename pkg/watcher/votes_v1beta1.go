package watcher

import (
	"context"
	"fmt"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	gov "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/rpc"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type VotesV1Beta1Watcher struct {
	metrics    *metrics.Metrics
	validators []TrackedValidator
	pool       *rpc.Pool
}

func NewVotesV1Beta1Watcher(validators []TrackedValidator, metrics *metrics.Metrics, pool *rpc.Pool) *VotesV1Beta1Watcher {
	return &VotesV1Beta1Watcher{
		metrics:    metrics,
		validators: validators,
		pool:       pool,
	}
}

func (w *VotesV1Beta1Watcher) Start(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Minute)

	for {
		node := w.pool.GetSyncedNode()
		if node == nil {
			log.Warn().Msg("no node available to fetch proposals")
		} else if err := w.fetchProposals(ctx, node); err != nil {
			log.Error().Err(err).Msg("failed to fetch pending proposals")
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *VotesV1Beta1Watcher) fetchProposals(ctx context.Context, node *rpc.Node) error {
	clientCtx := (client.Context{}).WithClient(node.Client)
	queryClient := gov.NewQueryClient(clientCtx)

	// Fetch all proposals in voting period
	proposalsResp, err := queryClient.Proposals(ctx, &gov.QueryProposalsRequest{
		ProposalStatus: gov.StatusVotingPeriod,
	})
	if err != nil {
		return fmt.Errorf("failed to get proposals: %w", err)
	}

	chainID := node.ChainID()

	// For each proposal, fetch validators vote
	for _, proposal := range proposalsResp.GetProposals() {
		w.metrics.ProposalEndTime.WithLabelValues(chainID, fmt.Sprintf("%d", proposal.ProposalId)).Set(float64(proposal.VotingEndTime.Unix()))

		for _, validator := range w.validators {
			voter := validator.AccountAddress()
			if voter == "" {
				log.Warn().Str("validator", validator.Name).Msg("no account address for validator")
				continue
			}
			voteResp, err := queryClient.Vote(ctx, &gov.QueryVoteRequest{
				ProposalId: proposal.ProposalId,
				Voter:      voter,
			})

			w.metrics.Vote.Reset()
			if isInvalidArgumentError(err) {
				w.handleVote(chainID, validator, proposal.ProposalId, nil)
			} else if err != nil {
				return fmt.Errorf("failed to get validator vote for proposal %d: %w", proposal.ProposalId, err)
			} else {
				vote := voteResp.GetVote()
				w.handleVote(chainID, validator, proposal.ProposalId, vote.Options)
			}
		}
	}

	return nil
}

func (w *VotesV1Beta1Watcher) handleVote(chainID string, validator TrackedValidator, proposalId uint64, votes []gov.WeightedVoteOption) {
	voted := false
	for _, option := range votes {
		if option.Option != gov.OptionEmpty {
			voted = true
			break
		}
	}

	w.metrics.Vote.
		WithLabelValues(chainID, validator.Address, validator.Name, fmt.Sprintf("%d", proposalId)).
		Set(metrics.BoolToFloat64(voted))
}

func isInvalidArgumentError(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	return st.Code() == codes.InvalidArgument
}
