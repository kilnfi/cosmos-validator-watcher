package watcher

import (
	"testing"

	gov "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"gotest.tools/assert"
)

func TestVotesWatcher(t *testing.T) {
	var (
		kilnAddress = "3DC4DD610817606AD4A8F9D762A068A81E8741E2"
		kilnName    = "Kiln"
		chainID     = "chain-42"
		validators  = []TrackedValidator{
			{
				Address: kilnAddress,
				Name:    kilnName,
			},
		}
	)

	votesWatcher := NewVotesWatcher(
		validators,
		metrics.New("cosmos_validator_watcher"),
		nil,
		VotesWatcherOptions{
			GovModuleVersion: "v1beta1",
		},
	)

	t.Run("Handle Votes", func(t *testing.T) {
		votesWatcher.handleVoteV1Beta1(chainID, validators[0], 40, nil)
		votesWatcher.handleVoteV1Beta1(chainID, validators[0], 41, []gov.WeightedVoteOption{{Option: gov.OptionEmpty}})
		votesWatcher.handleVoteV1Beta1(chainID, validators[0], 42, []gov.WeightedVoteOption{{Option: gov.OptionYes}})

		assert.Equal(t, float64(0), testutil.ToFloat64(votesWatcher.metrics.Vote.WithLabelValues(chainID, kilnAddress, kilnName, "40")))
		assert.Equal(t, float64(0), testutil.ToFloat64(votesWatcher.metrics.Vote.WithLabelValues(chainID, kilnAddress, kilnName, "41")))
		assert.Equal(t, float64(1), testutil.ToFloat64(votesWatcher.metrics.Vote.WithLabelValues(chainID, kilnAddress, kilnName, "42")))
	})
}
