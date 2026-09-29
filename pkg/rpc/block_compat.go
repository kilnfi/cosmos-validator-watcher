package rpc

import (
	"context"
	"encoding/json"
	"fmt"

	cmtjson "github.com/cometbft/cometbft/libs/json"
	jsonrpcclient "github.com/cometbft/cometbft/rpc/jsonrpc/client"
	"github.com/cometbft/cometbft/types"
	"github.com/rs/zerolog/log"
)

// compatBlock mirrors coretypes.ResultBlock's block, but keeps Evidence out of
// the decode path.
//
// Tendermint 0.35 forks — sei-tendermint reports "0.35.0-unreleased" —
// serialise Block.Evidence as a bare array (EvidenceList), whereas CometBFT
// v0.38 expects the EvidenceData wrapper object. Decoding a full ResultBlock
// against such a node fails with
//
//	error unmarshalling result: json: cannot unmarshal array into Go value
//	of type map[string]json.RawMessage
//
// even though header, data and last_commit are byte-compatible. The watcher
// never reads evidence, so we decode around it and hand every remaining field
// to CometBFT's own decoder, preserving its encoding rules (int64 as string,
// hex addresses, BlockIDFlag).
type compatBlock struct {
	Block *struct {
		Header     json.RawMessage `json:"header"`
		Data       json.RawMessage `json:"data"`
		LastCommit json.RawMessage `json:"last_commit"`
	} `json:"block"`
}

// FetchBlock retrieves a block, preferring CometBFT's own client. A nil height
// requests the latest block.
//
// The compatibility path below is only ever reached after the standard call
// has already failed, so nodes that speak CometBFT's own format keep their
// existing behaviour byte for byte. Once a node is known to need the compat
// decode we go straight to it, to avoid paying for a failing request per block.
func (n *Node) FetchBlock(ctx context.Context, height *int64) (*types.Block, error) {
	if n.compatBlocks.Load() {
		return n.fetchBlockCompat(ctx, height)
	}

	resp, err := n.Client.Block(ctx, height)
	if err == nil {
		return resp.Block, nil
	}

	block, compatErr := n.fetchBlockCompat(ctx, height)
	if compatErr != nil {
		// The compat path is a fallback: report the original failure.
		return nil, err
	}

	if n.compatBlocks.CompareAndSwap(false, true) {
		log.Warn().
			Str("node", n.Redacted()).
			Err(err).
			Msgf("falling back to evidence-tolerant block decoding")
	}

	return block, nil
}

// fetchBlockCompat performs the "block" call through CometBFT's JSON-RPC
// client — reusing its remote-address parsing (tcp://, unix://, http(s)://),
// dialer, compression and proxy settings, request IDs and envelope handling —
// and only replaces the final result decoding.
func (n *Node) fetchBlockCompat(ctx context.Context, height *int64) (*types.Block, error) {
	caller, err := n.compatCaller()
	if err != nil {
		return nil, err
	}

	params := make(map[string]interface{})
	if height != nil {
		params["height"] = height
	}

	// json.RawMessage implements json.Unmarshaler, which cmtjson honours, so
	// this hands back the raw "result" payload untouched.
	var raw json.RawMessage
	if _, err := caller.Call(ctx, "block", params, &raw); err != nil {
		return nil, err
	}

	var res compatBlock
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("decoding block result: %w", err)
	}
	if res.Block == nil {
		return nil, nil
	}

	block := new(types.Block)
	if err := cmtjson.Unmarshal(res.Block.Header, &block.Header); err != nil {
		return nil, fmt.Errorf("decoding block header: %w", err)
	}
	if err := cmtjson.Unmarshal(res.Block.Data, &block.Data); err != nil {
		return nil, fmt.Errorf("decoding block data: %w", err)
	}
	// The genesis block carries no last_commit.
	if len(res.Block.LastCommit) > 0 && string(res.Block.LastCommit) != "null" {
		block.LastCommit = new(types.Commit)
		if err := cmtjson.Unmarshal(res.Block.LastCommit, block.LastCommit); err != nil {
			return nil, fmt.Errorf("decoding block last_commit: %w", err)
		}
	}

	return block, nil
}

func (n *Node) compatCaller() (*jsonrpcclient.Client, error) {
	n.compatOnce.Do(func() {
		n.compatClient, n.compatErr = jsonrpcclient.New(n.Client.Remote())
	})
	if n.compatErr != nil {
		return nil, n.compatErr
	}
	return n.compatClient, nil
}
