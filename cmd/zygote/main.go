/*
Copyright (C) 2025- Hamed Ghasemzadeh. All rights reserved.
License: HGL General License <http://evgnomon.org/docs/hgl>
*/
package main

import (
	"os"

	"github.com/evgnomon/zygote/internal/commands"
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
			commands.BlueprintCommand(),
			commands.BuildCommand(),
			commands.CCommand(),
			commands.CallCommand(),
			commands.CertCommand(),
			commands.CheckCommand(),
			commands.DeinitCommand(),
			commands.GenerateCommand(),
			commands.InitCommand(),
			commands.JoinCommand(),
			commands.MemCommand(),
			commands.MigrateCommand(),
			commands.OpenActions(),
			commands.OpenDiffs(),
			commands.OpenPackages(),
			commands.QCommand(),
			commands.SQLCommand(),
			commands.SmokerCommand(),
			commands.VaultCommand(),
		},
	}
	if err := app.Run(os.Args); err != nil {
		logger.Error("error running command", err)
	}
}
