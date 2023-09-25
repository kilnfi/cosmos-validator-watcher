package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kilnfi/cosmos-validator-watcher/pkg/app"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
)

var data = []struct {
	node      string
	validator string
}{
	{"https://cosmos-rpc.publicnode.com:443", "3DC4DD610817606AD4A8F9D762A068A81E8741E2"},
	{"https://evmos-rpc.publicnode.com:443", "C28576ECA1802CD7046913848784D8147F6BAB22"},
	{"https://injective-rpc.publicnode.com:443", "37CE71AD8CD4DC01D8798E103A70978591708169"},
	{"https://juno-rpc.publicnode.com:443", "C55563E7FAD03561899AC6638E3E1E07DCB68D99"},
	{"https://kava-rpc.publicnode.com:443", "C669FD6A91DD9771BCEAE988E2C58356E8B33383"},
	// {"https://neutron-rpc.publicnode.com:443", "D2C7578217BA3ACEE64120FBCABD1B47EA51F9CE"},
	{"https://osmosis-rpc.publicnode.com:443", "17386B308EF9670CDD412FB2F3C09C5B875FB8B8"},
	{"https://persistence-rpc.publicnode.com:443", "C016A67FFD0E91FF9F0A81CF639F05B9161D90C8"},
	{"https://quicksilver-rpc.publicnode.com:443", "72A46AB01C191843223BF54B29201D87D7CEC44E"},
	{"https://stride-rpc.publicnode.com:443", "6F5D7AB6943BE2D01DD910CCD7B06D9E30A0FEFF"},
	{"https://rpc-sei-ia.cosmosia.notional.ventures:443", "908A25DA02880FAA7C1BE016A5ABFD165D9D9087"},
}

func main() {
	ctx := context.Background()

	app := &cli.App{
		Name:   "cosmos-validator-watcher",
		Flags:  app.Flags,
		Action: app.RunFunc,
	}

	for _, d := range data {
		args := []string{
			"cosmos-validator-watcher",
			"--node", d.node,
			"--validator", d.validator,
		}

		// Allow 3 seconds timeout
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		errg, ctx := errgroup.WithContext(ctx)
		errg.Go(func() error {
			return app.RunContext(ctx, args)
		})
		errg.Go(func() error {
			time.Sleep(1500 * time.Millisecond)
			return checkMetrics(ctx)
		})

		if err := errg.Wait(); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal().Err(err).Msg("")
		}
	}
}

// checkMetrics ensures if the given metric is present
func checkMetrics(ctx context.Context) error {
	metric := "cosmos_validator_watcher_block_height"

	req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost:8080/metrics", nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	client := http.DefaultClient
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %w", err)
	}

	if !strings.Contains(string(body), metric) {
		return fmt.Errorf("metric '%s' is not present", metric)
	}

	log.Info().Msg("metrics are present")

	return nil
}
