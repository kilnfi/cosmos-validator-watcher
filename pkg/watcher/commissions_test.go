package watcher

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"gotest.tools/assert"
)

func TestCommissionsWatcher(t *testing.T) {
	chainID := "chain-42"

	kilnValidator := TrackedValidator{
		Address:         "3DC4DD610817606AD4A8F9D762A068A81E8741E2",
		Name:            "Kiln",
		OperatorAddress: "cosmosvaloper1uxlf7mvr8nep3gm7udf2u9remms2jyjqvwdul2",
	}

	watcher := NewCommissionsWatcher(
		[]TrackedValidator{kilnValidator},
		metrics.New("cosmos_validator_watcher"),
		nil,
	)

	t.Run("Handle Commissions", func(t *testing.T) {
		watcher.handleValidatorCommission(chainID, kilnValidator, []types.DecCoin{
			{
				Denom:  "uatom",
				Amount: sdkmath.LegacyNewDec(123),
			},
			{
				Denom:  "ibc/0025F8A87464A471E66B234C4F93AEC5B4DA3D42D7986451A059273426290DD5",
				Amount: sdkmath.LegacyNewDec(42),
			},
		})

		assert.Equal(t, float64(123), testutil.ToFloat64(watcher.metrics.Commission.WithLabelValues(chainID, kilnValidator.Address, kilnValidator.Name, "uatom")))
		assert.Equal(t, float64(42), testutil.ToFloat64(watcher.metrics.Commission.WithLabelValues(chainID, kilnValidator.Address, kilnValidator.Name, "ibc/0025F8A87464A471E66B234C4F93AEC5B4DA3D42D7986451A059273426290DD5")))
	})
}
