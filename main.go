package main

import (
	"context"
	"errors"
	"os"

	"github.com/kilnfi/cosmos-validator-watcher/pkg/app"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
)

var Version = "v0.0.0-dev" // generated at build time

func main() {
	app := &cli.App{
		Name:     "cosmos-validator-watcher",
		Usage:    "Real-time Cosmos-based chains monitoring tool",
		Flags:    app.Flags,
		Action:   app.RunFunc,
		Commands: app.Commands,
		Version:  Version,
	}

	if err := app.Run(os.Args); err != nil && !errors.Is(err, context.Canceled) {
		log.Error().Err(err).Msg("")
	}
}
