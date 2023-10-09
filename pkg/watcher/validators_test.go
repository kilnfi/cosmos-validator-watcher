package watcher

import (
	"encoding/hex"
	"testing"

	"cosmossdk.io/math"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	staking "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func TestValidatorsWatcher(t *testing.T) {
	var (
		kilnAddress = "3DC4DD610817606AD4A8F9D762A068A81E8741E2"
		kilnName    = "Kiln"
		chainID     = "chain-42"
	)

	validatorsWatcher := NewValidatorsWatcher(
		[]TrackedValidator{
			{
				Address: kilnAddress,
				Name:    kilnName,
			},
		},
		metrics.New("cosmos_validator_watcher"),
		nil,
		ValidatorsWatcherOptions{
			Denom:         "denom",
			DenomExponent: 6,
		},
	)

	t.Run("Handle Validators", func(t *testing.T) {
		createAddress := func(pubkey string) *codectypes.Any {
			prefix := "0000"
			ba, err := hex.DecodeString(prefix + pubkey)
			require.NoError(t, err)

			return &codectypes.Any{
				TypeUrl: "/cosmos.crypto.ed25519.PubKey",
				Value:   ba,
			}
		}

		validators := []staking.Validator{
			{
				OperatorAddress: "",
				ConsensusPubkey: createAddress("915dea44121fbceb01452f98ca005b457fe8360c5e191b6601ee01b8a8d407a0"), // 3DC4DD610817606AD4A8F9D762A068A81E8741E2
				Jailed:          false,
				Status:          staking.Bonded,
				Tokens:          math.NewInt(42000000),
			},
			{
				OperatorAddress: "",
				ConsensusPubkey: createAddress("0000000000000000000000000000000000000000000000000000000000000001"),
				Jailed:          false,
				Status:          staking.Bonded,
				Tokens:          math.NewInt(43000000),
			},
			{
				OperatorAddress: "",
				ConsensusPubkey: createAddress("0000000000000000000000000000000000000000000000000000000000000002"),
				Jailed:          false,
				Status:          staking.Unbonded,
				Tokens:          math.NewInt(1000000),
			},
			{
				OperatorAddress: "",
				ConsensusPubkey: createAddress("0000000000000000000000000000000000000000000000000000000000000003"),
				Jailed:          true,
				Status:          staking.Bonded,
				Tokens:          math.NewInt(99000000),
			},
		}

		validatorsWatcher.handleValidators(chainID, validators)

		assert.Equal(t, float64(42), testutil.ToFloat64(validatorsWatcher.metrics.Tokens.WithLabelValues(chainID, kilnAddress, kilnName, "denom")))
		assert.Equal(t, float64(2), testutil.ToFloat64(validatorsWatcher.metrics.Rank.WithLabelValues(chainID, kilnAddress, kilnName)))
		assert.Equal(t, float64(1), testutil.ToFloat64(validatorsWatcher.metrics.IsBonded.WithLabelValues(chainID, kilnAddress, kilnName)))
		assert.Equal(t, float64(0), testutil.ToFloat64(validatorsWatcher.metrics.IsJailed.WithLabelValues(chainID, kilnAddress, kilnName)))
	})
}
