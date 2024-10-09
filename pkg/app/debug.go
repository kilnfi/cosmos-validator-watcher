package app

import (
	"fmt"

	"github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	bank "github.com/cosmos/cosmos-sdk/x/bank/types"
	staking "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/kilnfi/cosmos-validator-watcher/pkg/crypto"
	"github.com/urfave/cli/v2"
)

func DebugDenomRun(cCtx *cli.Context) error {
	var (
		ctx   = cCtx.Context
		nodes = cCtx.StringSlice("node")
	)

	if len(nodes) < 1 {
		return cli.Exit("at least one node must be specified", 1)
	}

	endpoint := nodes[0]

	rpcClient, err := http.New(endpoint, "/websocket")
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	clientCtx := (client.Context{}).WithClient(rpcClient)
	queryClient := bank.NewQueryClient(clientCtx)

	resp, err := queryClient.DenomsMetadata(ctx, &bank.QueryDenomsMetadataRequest{})
	if err != nil {
		return err
	}

	cdc := codec.NewLegacyAmino()
	j, err := cdc.MarshalJSONIndent(resp.Metadatas, "", "  ")
	fmt.Println(string(j))

	return nil
}

func DebugValidatorRun(cCtx *cli.Context) error {
	var (
		ctx   = cCtx.Context
		nodes = cCtx.StringSlice("node")
	)

	if cCtx.Args().Len() < 1 {
		return cli.Exit("validator address must be specified", 1)
	}
	if len(nodes) < 1 {
		return cli.Exit("at least one node must be specified", 1)
	}

	endpoint := nodes[0]
	validatorAddr := cCtx.Args().First()

	rpcClient, err := http.New(endpoint, "/websocket")
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	clientCtx := (client.Context{}).WithClient(rpcClient)
	queryClient := staking.NewQueryClient(clientCtx)

	resp, err := queryClient.Validator(ctx, &staking.QueryValidatorRequest{
		ValidatorAddr: validatorAddr,
	})
	if err != nil {
		return err
	}

	val := resp.Validator

	cdc := codec.NewLegacyAmino()
	j, err := cdc.MarshalJSONIndent(val, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(j))

	return nil
}

func DebugConsensusKeyRun(cCtx *cli.Context) error {
	var (
		ctx   = cCtx.Context
		nodes = cCtx.StringSlice("node")
	)

	if cCtx.Args().Len() < 1 {
		return cli.Exit("validator address must be specified", 1)
	}
	if len(nodes) < 1 {
		return cli.Exit("at least one node must be specified", 1)
	}

	endpoint := nodes[0]
	validatorAddr := cCtx.Args().First()

	rpcClient, err := http.New(endpoint, "/websocket")
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	clientCtx := (client.Context{}).WithClient(rpcClient)
	queryClient := staking.NewQueryClient(clientCtx)

	resp, err := queryClient.Validator(ctx, &staking.QueryValidatorRequest{
		ValidatorAddr: validatorAddr,
	})
	if err != nil {
		return err
	}

	val := resp.Validator
	address := crypto.PubKeyAddress(val.ConsensusPubkey)

	fmt.Println(address)

	return nil
}
