package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	cmtjson "github.com/cometbft/cometbft/libs/json"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Both fixtures are the same pacific-1 block (height 234731060), captured from
// a live node. They differ only in how Block.Evidence is serialised:
//
//	block_sei_tendermint.json  "evidence": []                  (Tendermint 0.35, EvidenceList)
//	block_cometbft.json        "evidence": {"evidence": []}    (CometBFT v0.38, EvidenceData)
const (
	fixtureSei      = "testdata/block_sei_tendermint.json"
	fixtureCometBFT = "testdata/block_cometbft.json"
)

func newFixtureNode(t *testing.T, fixture string) (*Node, *int) {
	t.Helper()

	payload, err := os.ReadFile(fixture)
	require.NoError(t, err)

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		// Echo back the request id so CometBFT's id verification passes.
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		var body map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(payload, &body))
		if len(req.ID) > 0 {
			body["id"] = req.ID
		}
		out, err := json.Marshal(body)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(out)
	}))
	t.Cleanup(server.Close)

	client, err := rpchttp.New(server.URL, "/websocket")
	require.NoError(t, err)

	return NewNode(client), &calls
}

// TestUpstreamDecodeFailsOnSeiEvidence documents the underlying defect: the
// stock CometBFT decoder cannot read a Tendermint 0.35 block at all, because
// evidence arrives as an array where EvidenceData (a struct) is expected.
func TestUpstreamDecodeFailsOnSeiEvidence(t *testing.T) {
	payload, err := os.ReadFile(fixtureSei)
	require.NoError(t, err)

	var env struct {
		Result json.RawMessage `json:"result"`
	}
	require.NoError(t, json.Unmarshal(payload, &env))

	err = cmtjson.Unmarshal(env.Result, new(ctypes.ResultBlock))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot unmarshal array into Go value of type map[string]json.RawMessage")

	// The CometBFT-shaped fixture decodes fine, proving evidence is the only
	// incompatible field.
	payload, err = os.ReadFile(fixtureCometBFT)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(payload, &env))
	require.NoError(t, cmtjson.Unmarshal(env.Result, new(ctypes.ResultBlock)))
}

// TestFetchBlockCometBFTFormat is the non-regression guard: against a node
// speaking CometBFT's own format, fetchBlock must go through Client.Block and
// never touch the compatibility path.
func TestFetchBlockCometBFTFormat(t *testing.T) {
	node, calls := newFixtureNode(t, fixtureCometBFT)

	block, err := node.FetchBlock(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, block)

	assertFixtureBlock(t, block)
	assert.False(t, node.compatBlocks.Load(), "compat path must not engage for CometBFT nodes")
	assert.Equal(t, 1, *calls, "expected exactly one RPC call")
}

// TestFetchBlockSeiTendermintFormat is the fix: a Tendermint 0.35 node decodes
// correctly via the fallback, and the node is then pinned to the compat path.
func TestFetchBlockSeiTendermintFormat(t *testing.T) {
	node, calls := newFixtureNode(t, fixtureSei)

	block, err := node.FetchBlock(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, block)

	assertFixtureBlock(t, block)
	assert.True(t, node.compatBlocks.Load(), "node should be pinned to the compat path")
	assert.Equal(t, 2, *calls, "first fetch tries the standard path, then falls back")

	// Subsequent fetches skip the failing standard call.
	*calls = 0
	block, err = node.FetchBlock(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, block)
	assertFixtureBlock(t, block)
	assert.Equal(t, 1, *calls, "compat path should be used directly afterwards")
}

// assertFixtureBlock checks every field the watcher actually consumes.
func assertFixtureBlock(t *testing.T, block *types.Block) {
	t.Helper()

	assert.Equal(t, "pacific-1", block.Header.ChainID)
	assert.Equal(t, int64(234731060), block.Header.Height)
	assert.Equal(t, "507A1930FE00C2B4D3DAE52C7463DCDA2468A5ED", block.Header.ProposerAddress.String())
	assert.Equal(t, 2, block.Txs.Len())

	require.NotNil(t, block.LastCommit)
	assert.Equal(t, int64(234731059), block.LastCommit.Height)
	assert.Equal(t, int32(0), block.LastCommit.Round)
	require.Equal(t, 40, block.LastCommit.Size())

	sig := block.LastCommit.Signatures[0]
	assert.Equal(t, types.BlockIDFlagCommit, sig.BlockIDFlag)
	assert.Equal(t, "7482D68A8C68FD7204273223D90864207B1558B5", sig.ValidatorAddress.String())

	// Every validator signed this block.
	for i, s := range block.LastCommit.Signatures {
		assert.NotEqual(t, types.BlockIDFlagAbsent, s.BlockIDFlag, "signature %d", i)
	}
}
