package main

import (
	"github.com/evgnomon/zygote/internal/controller"
	"github.com/evgnomon/zygote/internal/server"
	"github.com/evgnomon/zygote/internal/util"
)

var logger = util.NewLogger()

func main() {
	logger.Info("Starting Zygote API server...")
	s, err := server.NewServer()
	logger.FatalIfErr("Create server", err)
	dbC, err := controller.NewSQLQueryController()
	logger.FatalIfErr("Create database controller", err)
	hw := controller.NewHelloWorldController()
	rc, err := controller.NewRedisQueryController(nil)
	logger.FatalIfErr("Create redis controller", err)
	err = s.AddControllers([]controller.Controller{
		dbC,
		hw,
		rc,
	})
	logger.FatalIfErr("Add controllers", err)
	err = s.Listen()
	logger.FatalIfErr("Listen", err)
}
