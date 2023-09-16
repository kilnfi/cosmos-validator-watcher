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
		Usage: "RPC node endpoint to connect to (speficied multiple nodes for high availability)",
		Value: cli.NewStringSlice("http://localhost:26657"),
	},
	&cli.StringSliceFlag{
		Name:  "validator",
		Usage: "validator address(es) to track (use :my-label to add a custom label in metrics & ouput)",
	},
}
