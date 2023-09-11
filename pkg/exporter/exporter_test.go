package exporter

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

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
		address = "3DC4DD610817606AD4A8F9D762A068A81E8741E2"
		name    = "Kiln"
	)

	exporter := New(&Config{
		Writer:         &bytes.Buffer{},
		Metrics:        metrics.New("cosmos_validator_watcher"),
		BlockChan:      make(chan *types.Block),
		ValidatorsChan: make(chan []stakingtypes.Validator),
		TrackedValidators: []TrackedValidator{
			{
				Address: address,
				Name:    name,
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
							ValidatorAddress: MustParseAddress(address),
						},
					},
				},
			},
			{
				Header: types.Header{Height: 41},
				LastCommit: &types.Commit{
					Signatures: []types.CommitSig{
						{
							BlockIDFlag:      types.BlockIDFlagCommit,
							ValidatorAddress: MustParseAddress(address),
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
							ValidatorAddress: MustParseAddress(address),
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
				`#41   1/1 validators ✅ Kiln`,
				`#42   1/1 validators ✅ Kiln`,
			}, "\n")+"\n",
			exporter.cfg.Writer.(*bytes.Buffer).String(),
		)

		assert.Equal(t,
			`gauge:<value:42 > `,
			ReadMetric(exporter.cfg.Metrics.BlockHeight),
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
			ReadMetric(exporter.cfg.Metrics.ValidatedBlocks.WithLabelValues(address, name)),
		)
		assert.Equal(t,
			`label:<name:"address" value:"3DC4DD610817606AD4A8F9D762A068A81E8741E2" > label:<name:"name" value:"Kiln" > counter:<value:1 > `,
			ReadMetric(exporter.cfg.Metrics.MissedBlocks.WithLabelValues(address, name)),
		)
	})

	t.Run("Handle Validators", func(t *testing.T) {
		prefix := "0000"
		pubkey := "915dea44121fbceb01452f98ca005b457fe8360c5e191b6601ee01b8a8d407a0"
		ba, err := hex.DecodeString(prefix + pubkey)
		require.NoError(t, err)

		addr := &codectypes.Any{
			TypeUrl: "/cosmos.crypto.ed25519.PubKey",
			Value:   ba,
		}

		validators := []stakingtypes.Validator{
			{
				OperatorAddress: "",
				ConsensusPubkey: addr,
				Jailed:          false,
				Status:          stakingtypes.Bonded,
			},
		}

		exporter.handleValidators(validators)

		assert.Equal(t,
			`label:<name:"address" value:"3DC4DD610817606AD4A8F9D762A068A81E8741E2" > label:<name:"name" value:"Kiln" > gauge:<value:1 > `,
			ReadMetric(exporter.cfg.Metrics.ValidatorBonded.WithLabelValues(address, name)),
		)
		assert.Equal(t,
			`label:<name:"address" value:"3DC4DD610817606AD4A8F9D762A068A81E8741E2" > label:<name:"name" value:"Kiln" > gauge:<value:0 > `,
			ReadMetric(exporter.cfg.Metrics.ValidatorJail.WithLabelValues(address, name)),
		)
	})
}
