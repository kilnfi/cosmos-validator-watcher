package app

import "github.com/urfave/cli/v2"

var Commands = []*cli.Command{
	{
		Name:  "debug",
		Usage: "Debug utilities",
		Subcommands: []*cli.Command{
			{
				Name:   "consensus-key",
				Action: DebugConsensusKeyRun,
				Flags:  Flags,
			},
			{
				Name:   "denom",
				Action: DebugDenomRun,
				Flags:  Flags,
			},
			{
				Name:   "validator",
				Action: DebugValidatorRun,
				Flags:  Flags,
			},
		},
	},
}
