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
	if err != nil {
		logger.Fatal("Failed to create server", util.WrapError(err))
	}
	dbC, err := controller.NewSQLQueryController()
	if err != nil {
		logger.Fatal("Failed to create database controller", util.WrapError(err))
	}
	hw := controller.NewHelloWorldController()

	rc, err := controller.NewRedisQueryController(nil)
	if err != nil {
		logger.Fatal("Failed to create redis controller", util.WrapError(err))
	}
	err = s.AddControllers([]controller.Controller{
		dbC,
		hw,
		rc,
	})
	if err != nil {
		logger.Fatal("Failed to add controllers", util.WrapError(err))
	}
	err = s.Listen()
	if err != nil {
		logger.Fatal("Failed to start server", util.WrapError(err))
	}
}
