package main

import (
	"fmt"

	"github.com/evgnomon/zygote/internal/controller"
	"github.com/evgnomon/zygote/internal/server"
)

func main() {
	s, err := server.NewServer()
	if err != nil {
		panic(fmt.Errorf("failed to create server: %v", err))
	}
	dbC, err := controller.NewSQLQueryController()
	if err != nil {
		panic(fmt.Errorf("failed to create database controller: %v", err))
	}
	hw := controller.NewHelloWorldController()

	rc, err := controller.NewRedisQueryController(nil)
	if err != nil {
		panic(fmt.Errorf("failed to create redis controller: %v", err))
	}
	err = s.AddControllers([]controller.Controller{
		dbC,
		hw,
		rc,
	})
	if err != nil {
		panic(fmt.Errorf("failed to add controllers: %v", err))
	}
	err = s.Listen()
	if err != nil {
		panic(fmt.Errorf("failed to start server: %v", err))
	}
}
