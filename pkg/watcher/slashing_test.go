package watcher

import (
	"testing"
	"time"

	cosmossdk_io_math "cosmossdk.io/math"
	slashing "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"gotest.tools/assert"
)

func TestSlashingWatcher(t *testing.T) {
	var chainID = "test-chain"

	watcher := NewSlashingWatcher(
		metrics.New("cosmos_validator_watcher"),
		nil,
	)

	t.Run("Handle Slashing Parameters", func(t *testing.T) {

		minSignedPerWindow := cosmossdk_io_math.LegacyMustNewDecFromStr("0.1")
		slashFractionDoubleSign := cosmossdk_io_math.LegacyMustNewDecFromStr("0.01")
		slashFractionDowntime := cosmossdk_io_math.LegacyMustNewDecFromStr("0.001")

		params := slashing.Params{
			SignedBlocksWindow:      int64(1000),
			MinSignedPerWindow:      minSignedPerWindow,
			DowntimeJailDuration:    time.Duration(10) * time.Second,
			SlashFractionDoubleSign: slashFractionDoubleSign,
			SlashFractionDowntime:   slashFractionDowntime,
		}

		watcher.handleSlashingParams(chainID, params)

		assert.Equal(t, float64(1000), testutil.ToFloat64(watcher.metrics.SignedBlocksWindow.WithLabelValues(chainID)))
		assert.Equal(t, float64(0.1), testutil.ToFloat64(watcher.metrics.MinSignedBlocksPerWindow.WithLabelValues(chainID)))
		assert.Equal(t, float64(10), testutil.ToFloat64(watcher.metrics.DowntimeJailDuration.WithLabelValues(chainID)))
		assert.Equal(t, float64(0.01), testutil.ToFloat64(watcher.metrics.SlashFractionDoubleSign.WithLabelValues(chainID)))
		assert.Equal(t, float64(0.001), testutil.ToFloat64(watcher.metrics.SlashFractionDowntime.WithLabelValues(chainID)))
	})

}
