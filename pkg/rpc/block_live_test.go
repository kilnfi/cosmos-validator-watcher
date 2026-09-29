package rpc

import (
	"context"
	"os"
	"testing"
	"time"

	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFetchBlockLive exercises fetchBlock against a real node. It is skipped
// unless CVW_LIVE_NODE is set, e.g. against a port-forwarded Sei validator:
//
//	kubectl -n sei port-forward pod/mainnet-sei-validator-1-a-0 26657:26657
//	CVW_LIVE_NODE=http://localhost:26657 go test ./pkg/rpc/ -run Live -v
//
// This package pulls in neither babylon nor blst, so it builds without cgo.
func TestFetchBlockLive(t *testing.T) {
	endpoint := os.Getenv("CVW_LIVE_NODE")
	if endpoint == "" {
		t.Skip("set CVW_LIVE_NODE to run (e.g. http://localhost:26657)")
	}

	client, err := rpchttp.New(endpoint, "/websocket")
	require.NoError(t, err)
	node := NewNode(client)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Report whether the stock CometBFT decoder can read this node at all.
	if _, err := client.Block(ctx, nil); err != nil {
		t.Logf("stock Client.Block FAILED (this is the bug): %v", err)
	} else {
		t.Logf("stock Client.Block succeeded — node speaks CometBFT's format")
	}

	block, err := node.FetchBlock(ctx, nil)
	require.NoError(t, err, "fetchBlock must succeed regardless of node flavour")
	require.NotNil(t, block)

	t.Logf("chain_id=%s height=%d proposer=%s txs=%d signatures=%d compat=%v",
		block.Header.ChainID,
		block.Header.Height,
		block.Header.ProposerAddress,
		block.Txs.Len(),
		len(block.LastCommit.Signatures),
		node.compatBlocks.Load(),
	)

	assert.NotEmpty(t, block.Header.ChainID)
	assert.Greater(t, block.Header.Height, int64(0))
	assert.NotEmpty(t, block.Header.ProposerAddress)
	require.NotNil(t, block.LastCommit)
	assert.NotEmpty(t, block.LastCommit.Signatures, "commit must carry signatures")
	assert.Equal(t, block.Header.Height-1, block.LastCommit.Height)

	// Fetch an explicit earlier height, the way syncBlocks back-fills.
	previous := block.Header.Height - 1
	older, err := node.FetchBlock(ctx, &previous)
	require.NoError(t, err)
	require.NotNil(t, older)
	assert.Equal(t, previous, older.Header.Height)

	// Confirm blocks keep arriving, i.e. the gauge would actually advance.
	time.Sleep(2 * time.Second)
	later, err := node.FetchBlock(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, later)
	t.Logf("advanced %d blocks in 2s", later.Header.Height-block.Header.Height)
	assert.Greater(t, later.Header.Height, block.Header.Height, "chain should be advancing")
}
