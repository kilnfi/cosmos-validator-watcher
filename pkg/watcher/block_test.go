package watcher

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"gotest.tools/assert"
)

func TestBlockWatcher(t *testing.T) {
	var (
		kilnAddress = "3DC4DD610817606AD4A8F9D762A068A81E8741E2"
		kilnName    = "Kiln"
		chainID     = "chain-42"
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

	t.Run("Handle BlockInfo", func(t *testing.T) {
		blocks := []BlockInfo{
			{
				ChainID:          chainID,
				Height:           36,
				TotalValidators:  1,
				SignedValidators: 0,
				ValidatorStatus: []ValidatorStatus{
					{
						Address: kilnAddress,
						Label:   kilnName,
						Bonded:  false,
						Signed:  false,
						Rank:    0,
					},
				},
			},
			{
				ChainID:          chainID,
				Height:           41,
				TotalValidators:  1,
				SignedValidators: 0,
				ValidatorStatus: []ValidatorStatus{
					{
						Address: kilnAddress,
						Label:   kilnName,
						Bonded:  true,
						Signed:  false,
						Rank:    1,
					},
				},
			},
			{
				ChainID:          chainID,
				Height:           42,
				TotalValidators:  2,
				SignedValidators: 1,
				ValidatorStatus: []ValidatorStatus{
					{
						Address: kilnAddress,
						Label:   kilnName,
						Bonded:  true,
						Signed:  true,
						Rank:    2,
					},
				},
			},
			{
				ChainID:          chainID,
				Height:           43,
				TotalValidators:  2,
				SignedValidators: 2,
				ValidatorStatus: []ValidatorStatus{
					{
						Address: kilnAddress,
						Label:   kilnName,
						Bonded:  true,
						Signed:  true,
						Rank:    2,
					},
				},
			},
		}

		for _, block := range blocks {
			blockWatcher.handleBlockInfo(&block)
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
