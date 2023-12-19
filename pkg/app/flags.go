package app

import "github.com/urfave/cli/v2"

var Flags = []cli.Flag{
	&cli.StringFlag{
		Name:  "chain-id",
		Usage: "to ensure all nodes matches the specific network (dismiss to auto-detected)",
	},
	&cli.StringFlag{
		Name:  "http-addr",
		Usage: "http server address",
		Value: ":8080",
	},
	&cli.StringFlag{
		Name:  "log-level",
		Usage: "log level (debug, info, warn, error)",
		Value: "info",
	},
	&cli.StringFlag{
		Name:  "namespace",
		Usage: "namespace for Prometheus metrics",
		Value: "cosmos_validator_watcher",
	},
	&cli.BoolFlag{
		Name:  "no-color",
		Usage: "disable colored output",
	},
	&cli.StringSliceFlag{
		Name:  "node",
		Usage: "rpc node endpoint to connect to (specify multiple for high availability)",
		Value: cli.NewStringSlice("http://localhost:26657"),
	},
	&cli.BoolFlag{
		Name:  "no-gov",
		Usage: "disable calls to gov module (useful for consumer chains)",
	},
	&cli.BoolFlag{
		Name:  "no-staking",
		Usage: "disable calls to staking module (useful for consumer chains)",
	},
	&cli.StringFlag{
		Name:  "denom",
		Usage: "denom used in metrics label (eg. atom or uatom)",
	},
	&cli.UintFlag{
		Name:  "denom-exponent",
		Usage: "denom exponent (eg. 6 for atom, 1 for uatom)",
	},
	&cli.StringSliceFlag{
		Name:  "validator",
		Usage: "validator address(es) to track (use :my-label to add a custom label in metrics & ouput)",
	},
	&cli.StringFlag{
		Name:  "webhook-url",
		Usage: "endpoint where to send upgrade webhooks (experimental)",
	},
	&cli.StringFlag{
		Name:  "x-gov",
		Usage: "version of the gov module to use (v1|v1beta1)",
		Value: "v1beta1",
	},
}
