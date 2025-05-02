package main

import (
	"github.com/evgnomon/zygote/internal/controller"
	"github.com/evgnomon/zygote/internal/server"
	"github.com/evgnomon/zygote/pkg/http"
	"github.com/evgnomon/zygote/pkg/utils"
)

var logger = utils.NewLogger()

func main() {
	logger.Info("Starting Zygote API server...")
	s, err := server.NewServer()
	logger.FatalIfErr("Create server", err)
	dbC, err := controller.NewSQLQueryController()
	logger.FatalIfErr("Create database controller", err)
	hw := controller.NewHelloWorldController()
	rc, err := controller.NewRedisQueryController(nil)
	logger.FatalIfErr("Create redis controller", err)
	relayC := controller.NewRelayController()
	err = s.AddControllers([]http.Controller{
		dbC,
		hw,
		rc,
		relayC,
	})
	logger.FatalIfErr("Add controllers", err)
	err = s.Listen()
	logger.FatalIfErr("Listen", err)
}
