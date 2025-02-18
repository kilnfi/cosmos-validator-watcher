package watcher

import (
	"testing"

	upgrade "cosmossdk.io/x/upgrade/types"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"gotest.tools/assert"
)

func TestUpgradeWatcher(t *testing.T) {
	var chainID = "chain-42"

	watcher := NewUpgradeWatcher(
		metrics.New("cosmos_validator_watcher"),
		nil,
		nil,
		UpgradeWatcherOptions{},
	)

	t.Run("Handle Upgrade Plan", func(t *testing.T) {
		var (
			version     = "v42.0.0"
			blockHeight = int64(123456789)
		)

		watcher.handleUpgradePlan(chainID, &upgrade.Plan{
			Name:   version,
			Height: blockHeight,
		})

		assert.Equal(t, float64(123456789), testutil.ToFloat64(watcher.metrics.UpgradePlan.WithLabelValues(chainID, version, "123456789")))
	})

	t.Run("Handle No Upgrade Plan", func(t *testing.T) {
		watcher.handleUpgradePlan(chainID, nil)

		assert.Equal(t, 0, testutil.CollectAndCount(watcher.metrics.UpgradePlan))
	})
}
