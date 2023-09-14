package exporter

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"cosmossdk.io/math"
	"github.com/cometbft/cometbft/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func TestExporter(t *testing.T) {
	var (
		kilnAddress = "3DC4DD610817606AD4A8F9D762A068A81E8741E2"
		kilnName    = "Kiln"

		miscAddress = "1234567890ABCDEF10817606AD4A8FD7620A81E4"
	)

	exporter := New(&Config{
		Writer:         &bytes.Buffer{},
		Metrics:        metrics.New("cosmos_validator_watcher"),
		BlockChan:      make(chan *types.Block),
		ValidatorsChan: make(chan []stakingtypes.Validator),
		TrackedValidators: []TrackedValidator{
			{
				Address: kilnAddress,
				Name:    kilnName,
			},
		},
	})

	MustParseAddress := func(address string) []byte {
		addr, err := hex.DecodeString(address)
		if err != nil {
			panic(err)
		}
		return addr
	}
	ReadMetric := func(counter prometheus.Counter) string {
		m := new(dto.Metric)
		_ = counter.Write(m)
		return m.String()
	}

	t.Run("Handle Blocks", func(t *testing.T) {
		blocks := []*types.Block{
			{
				Header: types.Header{Height: 35},
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
				Header: types.Header{Height: 40},
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
				Header: types.Header{Height: 41},
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
				Header: types.Header{Height: 42},
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
			exporter.handleBlock(block)
		}

		assert.Equal(t,
			strings.Join([]string{
				`#35   0/1 validators ⚪️ Kiln`,
				`#40   0/1 validators ❌ Kiln`,
				`#41   1/2 validators ✅ Kiln`,
				`#42   2/2 validators ✅ Kiln`,
			}, "\n")+"\n",
			exporter.cfg.Writer.(*bytes.Buffer).String(),
		)

		assert.Equal(t,
			`gauge:<value:42 > `,
			ReadMetric(exporter.cfg.Metrics.BlockHeight),
		)
		assert.Equal(t,
			`gauge:<value:2 > `,
			ReadMetric(exporter.cfg.Metrics.ActiveSet),
		)
		assert.Equal(t,
			`counter:<value:4 > `,
			ReadMetric(exporter.cfg.Metrics.TrackedBlocks),
		)
		assert.Equal(t,
			`counter:<value:5 > `,
			ReadMetric(exporter.cfg.Metrics.SkippedBlocks),
		)
		assert.Equal(t,
			`label:<name:"address" value:"3DC4DD610817606AD4A8F9D762A068A81E8741E2" > label:<name:"name" value:"Kiln" > counter:<value:2 > `,
			ReadMetric(exporter.cfg.Metrics.ValidatedBlocks.WithLabelValues(kilnAddress, kilnName)),
		)
		assert.Equal(t,
			`label:<name:"address" value:"3DC4DD610817606AD4A8F9D762A068A81E8741E2" > label:<name:"name" value:"Kiln" > counter:<value:1 > `,
			ReadMetric(exporter.cfg.Metrics.MissedBlocks.WithLabelValues(kilnAddress, kilnName)),
		)
	})

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

		validators := []stakingtypes.Validator{
			{
				OperatorAddress: "",
				ConsensusPubkey: createAddress("915dea44121fbceb01452f98ca005b457fe8360c5e191b6601ee01b8a8d407a0"), // 3DC4DD610817606AD4A8F9D762A068A81E8741E2
				Jailed:          false,
				Status:          stakingtypes.Bonded,
				Tokens:          math.NewInt(42000000),
			},
			{
				OperatorAddress: "",
				ConsensusPubkey: createAddress("0000000000000000000000000000000000000000000000000000000000000001"),
				Jailed:          false,
				Status:          stakingtypes.Bonded,
				Tokens:          math.NewInt(43000000),
			},
			{
				OperatorAddress: "",
				ConsensusPubkey: createAddress("0000000000000000000000000000000000000000000000000000000000000002"),
				Jailed:          false,
				Status:          stakingtypes.Unbonded,
				Tokens:          math.NewInt(1000000),
			},
			{
				OperatorAddress: "",
				ConsensusPubkey: createAddress("0000000000000000000000000000000000000000000000000000000000000003"),
				Jailed:          true,
				Status:          stakingtypes.Bonded,
				Tokens:          math.NewInt(99000000),
			},
		}

		exporter.handleValidators(validators)

		assert.Equal(t,
			`label:<name:"address" value:"3DC4DD610817606AD4A8F9D762A068A81E8741E2" > label:<name:"name" value:"Kiln" > gauge:<value:42 > `,
			ReadMetric(exporter.cfg.Metrics.Tokens.WithLabelValues(kilnAddress, kilnName)),
		)
		assert.Equal(t,
			`label:<name:"address" value:"3DC4DD610817606AD4A8F9D762A068A81E8741E2" > label:<name:"name" value:"Kiln" > gauge:<value:2 > `,
			ReadMetric(exporter.cfg.Metrics.Rank.WithLabelValues(kilnAddress, kilnName)),
		)
		assert.Equal(t,
			`label:<name:"address" value:"3DC4DD610817606AD4A8F9D762A068A81E8741E2" > label:<name:"name" value:"Kiln" > gauge:<value:1 > `,
			ReadMetric(exporter.cfg.Metrics.IsBonded.WithLabelValues(kilnAddress, kilnName)),
		)
		assert.Equal(t,
			`label:<name:"address" value:"3DC4DD610817606AD4A8F9D762A068A81E8741E2" > label:<name:"name" value:"Kiln" > gauge:<value:0 > `,
			ReadMetric(exporter.cfg.Metrics.IsJailed.WithLabelValues(kilnAddress, kilnName)),
		)
	})
}
