package watcher

import (
	"context"
	"fmt"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	gov "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govbeta "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/rpc"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type VotesWatcher struct {
	metrics    *metrics.Metrics
	validators []TrackedValidator
	pool       *rpc.Pool
	options    VotesWatcherOptions
}

type VotesWatcherOptions struct {
	GovModuleVersion string
}

func NewVotesWatcher(validators []TrackedValidator, metrics *metrics.Metrics, pool *rpc.Pool, options VotesWatcherOptions) *VotesWatcher {
	return &VotesWatcher{
		metrics:    metrics,
		validators: validators,
		pool:       pool,
		options:    options,
	}
}

func (w *VotesWatcher) Start(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Minute)

	for {
		node := w.pool.GetSyncedNode()
		if node == nil {
			log.Warn().Msg("no node available to fetch proposals")
		} else if err := w.fetchProposals(ctx, node); err != nil {
			log.Error().Err(err).
				Str("node", node.Redacted()).
				Msg("failed to fetch pending proposals")
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *VotesWatcher) fetchProposals(ctx context.Context, node *rpc.Node) error {
	var (
		votes map[uint64]map[TrackedValidator]bool
		err   error
	)

	switch w.options.GovModuleVersion {
	case "v1beta1":
		votes, err = w.fetchProposalsV1Beta1(ctx, node)
	default: // v1
		votes, err = w.fetchProposalsV1(ctx, node)
	}

	if err != nil {
		return err
	}

	w.metrics.Vote.Reset()
	for proposalId, votes := range votes {
		for validator, voted := range votes {
			w.metrics.Vote.
				WithLabelValues(node.ChainID(), validator.Address, validator.Name, fmt.Sprintf("%d", proposalId)).
				Set(metrics.BoolToFloat64(voted))
		}
	}

	return nil
}

func (w *VotesWatcher) fetchProposalsV1(ctx context.Context, node *rpc.Node) (map[uint64]map[TrackedValidator]bool, error) {
	votes := make(map[uint64]map[TrackedValidator]bool)

	clientCtx := (client.Context{}).WithClient(node.Client)
	queryClient := gov.NewQueryClient(clientCtx)

	// Fetch all proposals in voting period
	proposalsResp, err := queryClient.Proposals(ctx, &gov.QueryProposalsRequest{
		ProposalStatus: gov.StatusVotingPeriod,
	})
	if err != nil {
		return votes, fmt.Errorf("failed to fetch proposals in voting period: %w", err)
	}

	chainID := node.ChainID()

	// For each proposal, fetch validators vote
	for _, proposal := range proposalsResp.GetProposals() {
		votes[proposal.Id] = make(map[TrackedValidator]bool)
		w.metrics.ProposalEndTime.WithLabelValues(chainID, fmt.Sprintf("%d", proposal.Id)).Set(float64(proposal.VotingEndTime.Unix()))

		for _, validator := range w.validators {
			voter := validator.AccountAddress()
			if voter == "" {
				log.Warn().Str("validator", validator.Name).Msg("no account address for validator")
				continue
			}
			voteResp, err := queryClient.Vote(ctx, &gov.QueryVoteRequest{
				ProposalId: proposal.Id,
				Voter:      voter,
			})

			if isInvalidArgumentError(err) {
				votes[proposal.Id][validator] = false
			} else if err != nil {
				votes[proposal.Id][validator] = false
				log.Warn().
					Str("validator", validator.Name).
					Str("proposal", fmt.Sprintf("%d", proposal.Id)).
					Err(err).Msg("failed to get validator vote for proposal")
			} else {
				vote := voteResp.GetVote()
				voted := false
				for _, option := range vote.Options {
					if option.Option != gov.OptionEmpty {
						voted = true
						break
					}
				}
				votes[proposal.Id][validator] = voted
			}
		}
	}

	return votes, nil
}

func (w *VotesWatcher) fetchProposalsV1Beta1(ctx context.Context, node *rpc.Node) (map[uint64]map[TrackedValidator]bool, error) {
	votes := make(map[uint64]map[TrackedValidator]bool)

	clientCtx := (client.Context{}).WithClient(node.Client)
	queryClient := govbeta.NewQueryClient(clientCtx)

	// Fetch all proposals in voting period
	proposalsResp, err := queryClient.Proposals(ctx, &govbeta.QueryProposalsRequest{
		ProposalStatus: govbeta.StatusVotingPeriod,
	})
	if err != nil {
		return votes, fmt.Errorf("failed to fetch proposals in voting period: %w", err)
	}

	chainID := node.ChainID()

	// For each proposal, fetch validators vote
	for _, proposal := range proposalsResp.GetProposals() {
		votes[proposal.ProposalId] = make(map[TrackedValidator]bool)
		w.metrics.ProposalEndTime.WithLabelValues(chainID, fmt.Sprintf("%d", proposal.ProposalId)).Set(float64(proposal.VotingEndTime.Unix()))

		for _, validator := range w.validators {
			voter := validator.AccountAddress()
			if voter == "" {
				log.Warn().Str("validator", validator.Name).Msg("no account address for validator")
				continue
			}
			voteResp, err := queryClient.Vote(ctx, &govbeta.QueryVoteRequest{
				ProposalId: proposal.ProposalId,
				Voter:      voter,
			})

			if isInvalidArgumentError(err) {
				votes[proposal.ProposalId][validator] = false
			} else if err != nil {
				votes[proposal.ProposalId][validator] = false
				log.Warn().
					Str("validator", validator.Name).
					Str("proposal", fmt.Sprintf("%d", proposal.ProposalId)).
					Err(err).Msg("failed to get validator vote for proposal")
			} else {
				vote := voteResp.GetVote()
				voted := false
				for _, option := range vote.Options {
					if option.Option != govbeta.OptionEmpty {
						voted = true
						break
					}
				}
				votes[proposal.ProposalId][validator] = voted
			}
		}
	}

	return votes, nil
}

func (w *VotesWatcher) handleVoteV1Beta1(chainID string, validator TrackedValidator, proposalId uint64, votes []govbeta.WeightedVoteOption) {
	voted := false
	for _, option := range votes {
		if option.Option != govbeta.OptionEmpty {
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
