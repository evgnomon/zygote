/*
Copyright (C) 2025- Hamed Ghasemzadeh. All rights reserved.
License: HGL General License <http://evgnomon.org/docs/hgl>
*/
package main

import (
	"os"

	"github.com/evgnomon/zygote/cmd/zygote/commands"
	"github.com/evgnomon/zygote/pkg/utils"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	cli "github.com/urfave/cli/v2"
)

var logger = utils.NewLogger()

func main() {
	os.Setenv("EDITOR", "vi")
	err := utils.WriteScripts()
	logger.FatalIfErr("Write scripts", err)
	logger := utils.NewLogger()

	app := &cli.App{
		Action: func(_ *cli.Context) error {
			cc := commands.CheckCommand()
			return cc.Action(nil)
		},
		Commands: []*cli.Command{
			commands.VaultCommand(),
			commands.BlueprintCommand(),
			commands.BuildCommand(),
			commands.CheckCommand(),
			commands.MigrateCommand(),
			commands.InitCommand(),
			commands.DeinitCommand(),
			commands.CertCommand(),
			commands.CallCommand(),
			commands.QCommand(),
			commands.OpenDiffs(),
			commands.OpenActions(),
			commands.SQLCommand(),
			commands.GenerateCommand(),
			commands.SmokerCommand(),
			commands.JoinCommand(),
			commands.MemCommand(),
			commands.CCommand(),
		},
	}
	if err := app.Run(os.Args); err != nil {
		logger.Error("error running command", err)
	}
}
