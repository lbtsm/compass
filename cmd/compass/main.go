package main

import (
	log "github.com/ChainSafe/log15"
	"github.com/mapprotocol/compass/config"
	"github.com/urfave/cli/v2"
	"os"
)

var app = cli.NewApp()

var cliFlags = []cli.Flag{
	config.ConfigFileFlag,
	config.VerbosityFlag,
	config.KeystorePathFlag,
	config.KeyPathFlag,
	config.BlockstorePathFlag,
	config.FreshStartFlag,
	config.LatestBlockFlag,
	config.SkipErrorFlag,
	config.FilterFlag,
}

var (
	Version = "1.2.1"
)

// init initializes CLI
func init() {
	//app.Action = run
	app.Copyright = "Copyright 2021 MAP Protocol 2021 Authors"
	app.Name = "compass"
	app.Usage = "Compass"
	app.Authors = []*cli.Author{{Name: "MAP Protocol 2021"}}
	app.Version = Version
	app.EnableBashCompletion = true
	app.Commands = []*cli.Command{
		&accountCommand,
		&maintainerCommand,
		&messengerCommand,
		&oracleCommand,
		&apiCommand,
	}

	app.Flags = append(app.Flags, cliFlags...)
}

func main() {
	if err := app.Run(os.Args); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
}
