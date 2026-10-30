package watcher

import (
	"context"

	"github.com/cosmos/cosmos-sdk/types/query"
	slashing "github.com/cosmos/cosmos-sdk/x/slashing/types"
	staking "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// PageLimit is the page size used for paginated queries.
// Some chains reject larger pages (e.g. sei >= v6.7 caps untrusted queries at 1000).
const PageLimit = 1000

// paginate calls fetch with the next page key until the response has no next key.
func paginate[T any](fetch func(key []byte) ([]T, *query.PageResponse, error)) ([]T, error) {
	var (
		items []T
		key   []byte
	)
	for {
		page, resp, err := fetch(key)
		if err != nil {
			return nil, err
		}
		items = append(items, page...)
		if resp == nil || len(resp.NextKey) == 0 {
			return items, nil
		}
		key = resp.NextKey
	}
}

// FetchAllValidators returns all staking validators, following pagination.
func FetchAllValidators(ctx context.Context, queryClient staking.QueryClient) ([]staking.Validator, error) {
	return paginate(func(key []byte) ([]staking.Validator, *query.PageResponse, error) {
		resp, err := queryClient.Validators(ctx, &staking.QueryValidatorsRequest{
			Pagination: &query.PageRequest{
				Key:   key,
				Limit: PageLimit,
			},
		})
		if err != nil {
			return nil, nil, err
		}
		return resp.Validators, resp.Pagination, nil
	})
}

// FetchAllSigningInfos returns all validator signing infos, following pagination.
func FetchAllSigningInfos(ctx context.Context, queryClient slashing.QueryClient) ([]slashing.ValidatorSigningInfo, error) {
	return paginate(func(key []byte) ([]slashing.ValidatorSigningInfo, *query.PageResponse, error) {
		resp, err := queryClient.SigningInfos(ctx, &slashing.QuerySigningInfosRequest{
			Pagination: &query.PageRequest{
				Key:   key,
				Limit: PageLimit,
			},
		})
		if err != nil {
			return nil, nil, err
		}
		return resp.Info, resp.Pagination, nil
	})
}
