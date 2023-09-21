package watcher

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/cometbft/cometbft/types"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"gotest.tools/assert"
)

func TestBlockWatcher(t *testing.T) {
	var (
		kilnAddress = "3DC4DD610817606AD4A8F9D762A068A81E8741E2"
		kilnName    = "Kiln"
		chainID     = "chain-42"

		miscAddress = "1234567890ABCDEF10817606AD4A8FD7620A81E4"
	)

	blockWatcher := NewBlockWatcher(
		[]TrackedValidator{
			{
				Address: kilnAddress,
				Name:    kilnName,
			},
		},
		metrics.New("cosmos_validator_watcher"),
		&bytes.Buffer{},
	)

	MustParseAddress := func(address string) []byte {
		addr, err := hex.DecodeString(address)
		if err != nil {
			panic(err)
		}
		return addr
	}

	t.Run("Handle Blocks", func(t *testing.T) {
		blocks := []*types.Block{
			{
				Header: types.Header{ChainID: chainID, Height: 36},
				LastCommit: &types.Commit{
					Signatures: []types.CommitSig{
						{
							BlockIDFlag:      types.BlockIDFlagAbsent,
							ValidatorAddress: MustParseAddress("1234567890ABCDEF10817606AD4A8FD7620A81E4"),
						},
					},
				},
			},
			{
				Header: types.Header{ChainID: chainID, Height: 41},
				LastCommit: &types.Commit{
					Signatures: []types.CommitSig{
						{
							BlockIDFlag:      types.BlockIDFlagAbsent,
							ValidatorAddress: MustParseAddress(kilnAddress),
						},
					},
				},
			},
			{
				Header: types.Header{ChainID: chainID, Height: 42},
				LastCommit: &types.Commit{
					Signatures: []types.CommitSig{
						{
							BlockIDFlag:      types.BlockIDFlagAbsent,
							ValidatorAddress: MustParseAddress(miscAddress),
						},
						{
							BlockIDFlag:      types.BlockIDFlagCommit,
							ValidatorAddress: MustParseAddress(kilnAddress),
						},
					},
				},
			},
			{
				Header: types.Header{ChainID: chainID, Height: 43},
				LastCommit: &types.Commit{
					Signatures: []types.CommitSig{
						{
							BlockIDFlag:      types.BlockIDFlagCommit,
							ValidatorAddress: MustParseAddress(miscAddress),
						},
						{
							BlockIDFlag:      types.BlockIDFlagCommit,
							ValidatorAddress: MustParseAddress(kilnAddress),
						},
					},
				},
			},
		}

		for _, block := range blocks {
			blockWatcher.handleBlock(block)
		}

		assert.Equal(t,
			strings.Join([]string{
				`#35   0/1 validators ⚪️ Kiln`,
				`#40   0/1 validators ❌ Kiln`,
				`#41   1/2 validators ✅ Kiln`,
				`#42   2/2 validators ✅ Kiln`,
			}, "\n")+"\n",
			blockWatcher.writer.(*bytes.Buffer).String(),
		)

		assert.Equal(t, float64(43), testutil.ToFloat64(blockWatcher.metrics.BlockHeight.WithLabelValues(chainID)))
		assert.Equal(t, float64(2), testutil.ToFloat64(blockWatcher.metrics.ActiveSet.WithLabelValues(chainID)))
		assert.Equal(t, float64(4), testutil.ToFloat64(blockWatcher.metrics.TrackedBlocks.WithLabelValues(chainID)))
		assert.Equal(t, float64(5), testutil.ToFloat64(blockWatcher.metrics.SkippedBlocks.WithLabelValues(chainID)))

		assert.Equal(t, 1, testutil.CollectAndCount(blockWatcher.metrics.ValidatedBlocks))
		assert.Equal(t, 1, testutil.CollectAndCount(blockWatcher.metrics.MissedBlocks))
		assert.Equal(t, 1, testutil.CollectAndCount(blockWatcher.metrics.SoloMissedBlocks))
		assert.Equal(t, float64(2), testutil.ToFloat64(blockWatcher.metrics.ValidatedBlocks.WithLabelValues(chainID, kilnAddress, kilnName)))
		assert.Equal(t, float64(1), testutil.ToFloat64(blockWatcher.metrics.MissedBlocks.WithLabelValues(chainID, kilnAddress, kilnName)))
		assert.Equal(t, float64(0), testutil.ToFloat64(blockWatcher.metrics.SoloMissedBlocks.WithLabelValues(chainID, kilnAddress, kilnName)))
	})
}
