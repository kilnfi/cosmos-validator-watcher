package watcher

import (
	"bytes"
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/webhook"
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
		webhook.New(url.URL{}),
		[]BlockWebhook{},
	)

	t.Run("Handle BlockInfo", func(t *testing.T) {
		blocks := []BlockInfo{
			{
				ChainID:          chainID,
				Height:           36,
				Transactions:     4,
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
				Transactions:     5,
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
				Transactions:     6,
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
				Transactions:     7,
				TotalValidators:  2,
				SignedValidators: 2,
				ProposerAddress:  kilnAddress,
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
				Height:           44,
				Transactions:     0,
				TotalValidators:  2,
				SignedValidators: 2,
				ProposerAddress:  kilnAddress,
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
				Height:           45,
				Transactions:     7,
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
			blockWatcher.handleBlockInfo(context.Background(), &block)
		}

		assert.Equal(t,
			strings.Join([]string{
				`#35   0/1 validators ⚪️ Kiln`,
				`#40   0/1 validators ❌ Kiln`,
				`#41   1/2 validators ✅ Kiln`,
				`#42   2/2 validators ✅ Kiln`,
				`#43   2/2 validators 👑 Kiln`,
				`#44   2/2 validators 🟡 Kiln`,
			}, "\n")+"\n",
			blockWatcher.writer.(*bytes.Buffer).String(),
		)

		assert.Equal(t, float64(45), testutil.ToFloat64(blockWatcher.metrics.BlockHeight.WithLabelValues(chainID)))
		assert.Equal(t, float64(29), testutil.ToFloat64(blockWatcher.metrics.Transactions.WithLabelValues(chainID)))
		assert.Equal(t, float64(2), testutil.ToFloat64(blockWatcher.metrics.ActiveSet.WithLabelValues(chainID)))
		assert.Equal(t, float64(6), testutil.ToFloat64(blockWatcher.metrics.TrackedBlocks.WithLabelValues(chainID)))
		assert.Equal(t, float64(5), testutil.ToFloat64(blockWatcher.metrics.SkippedBlocks.WithLabelValues(chainID)))

		assert.Equal(t, 1, testutil.CollectAndCount(blockWatcher.metrics.ValidatedBlocks))
		assert.Equal(t, 1, testutil.CollectAndCount(blockWatcher.metrics.MissedBlocks))
		assert.Equal(t, 1, testutil.CollectAndCount(blockWatcher.metrics.SoloMissedBlocks))
		assert.Equal(t, 1, testutil.CollectAndCount(blockWatcher.metrics.ConsecutiveMissedBlocks))
		assert.Equal(t, 1, testutil.CollectAndCount(blockWatcher.metrics.EmptyBlocks))
		assert.Equal(t, float64(2), testutil.ToFloat64(blockWatcher.metrics.ProposedBlocks.WithLabelValues(chainID, kilnAddress, kilnName)))
		assert.Equal(t, float64(4), testutil.ToFloat64(blockWatcher.metrics.ValidatedBlocks.WithLabelValues(chainID, kilnAddress, kilnName)))
		assert.Equal(t, float64(1), testutil.ToFloat64(blockWatcher.metrics.MissedBlocks.WithLabelValues(chainID, kilnAddress, kilnName)))
		assert.Equal(t, float64(0), testutil.ToFloat64(blockWatcher.metrics.SoloMissedBlocks.WithLabelValues(chainID, kilnAddress, kilnName)))
		assert.Equal(t, float64(0), testutil.ToFloat64(blockWatcher.metrics.ConsecutiveMissedBlocks.WithLabelValues(chainID, kilnAddress, kilnName)))
		assert.Equal(t, float64(1), testutil.ToFloat64(blockWatcher.metrics.EmptyBlocks.WithLabelValues(chainID, kilnAddress, kilnName)))
	})
}
