// Package commands contains all available commands.
package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/urfave/cli/v2"
)

const dirPerm = 0755

func buildZygote() error {
	err := utils.Run("go", "build", "./cmd/zygote")
	if err != nil {
		return fmt.Errorf("failed to build zygote: %w", err)
	}
	return nil
}

// SmokerCommand runs smoke tests.
func SmokerCommand() *cli.Command {
	return &cli.Command{
		Name:  "smoke",
		Usage: "Run smoke tests",
		Action: func(_ *cli.Context) error {
			err := buildZygote()
			if err != nil {
				return fmt.Errorf("failed to build zygote: %w", err)
			}

			zygotePath := filepath.Join(os.Getenv("PWD"), "zygote")

			// Create temporary directory and change to it
			tempDir := "/tmp/smoker"
			if utils.PathExists(tempDir) {
				err = os.RemoveAll(tempDir)
				if err != nil {
					return fmt.Errorf("failed to remove /tmp/smoker: %w", err)
				}
			}
			err = os.Mkdir(tempDir, dirPerm)
			if err != nil {
				return fmt.Errorf("failed to create /tmp/smoker: %w", err)
			}
			defer os.RemoveAll(tempDir) // Clean up at the end

			err = os.Chdir(tempDir)
			if err != nil {
				return fmt.Errorf("failed to change to /tmp/smoker: %w", err)
			}

			defer func() {
				// Change back to original directory
				err = os.Chdir(os.Getenv("PWD"))
				logger.FatalIfErr("Change back to original directory", err)
			}()

			defer func() {
				err = utils.Script([][]string{{"sudo", zygotePath, "deinit", "-v"}})
				logger.FatalIfErr("Deinit zygote", err)
			}()
			err = utils.Script([][]string{
				{"rm", "-rf", "./sqls"},
				{"sudo", zygotePath, "deinit", "-v"},
				{"sudo", zygotePath, "init"},
				{"sudo", "-K"},
				{zygotePath, "gen", "db", "--name=smokers"},
				{zygotePath, "gen", "table", "--name=users", "--db=smokers"},
				{zygotePath, "gen", "table", "--name=posts", "--db=smokers"},
				{zygotePath, "gen", "table", "--name=comments", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=users", "--name=name", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=users", "--name=email", "--type=string", "--db=smokers"},
				{zygotePath, "gen", "index", "--table=users", "--name=email", "--col=email", "-u", "--db=smokers"},
				{zygotePath, "gen", "index", "--table=users", "--name", "users_name", "--col", "email", "--col", "name", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=users", "--name=active", "--type=bool", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=users", "--name=pic", "--type=binary", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=users", "--name=age", "--type=double", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=posts", "--name=title", "--type=string", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=posts", "--name=uuid", "--type=uuid", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=posts", "--name=content", "--type=text", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=posts", "--name=views", "--type=integer", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=posts", "--name=tags", "--type=json", "--db=smokers"},
				{zygotePath, "gen", "prop", "--table=posts", "--name=tag", "--type=string", "--path=$.name", "--db=smokers"},
				{zygotePath, "gen", "index", "--table=posts", "--name=tags", "--col=tag", "--col=uuid", "--db=smokers"},
				{zygotePath, "gen", "index", "-t", "posts", "-n", "content", "--col=content", "--full-text", "--db=smokers"},
				{zygotePath, "migrate", "up"},
				{zygotePath, "migrate", "down"},
				{zygotePath, "migrate", "up"},
				{zygotePath, "migrate", "down"},
				{zygotePath, "migrate", "up"},
			})

			if err != nil {
				return fmt.Errorf("failed to run smoke tests: %w", err)
			}

			return nil
		},
	}
}
