package controller

import (
	"fmt"

	"github.com/evgnomon/zygote/pkg/http"
)

// HelloWorldController struct to hold controller data
type HelloWorldController struct {
	// Add any necessary fields here
}

// NewHelloWorldController creates a new instance of HelloWorldController
func NewHelloWorldController() *HelloWorldController {
	return &HelloWorldController{}
}

// AddEndpoint implements the Controller interface
func (c *HelloWorldController) AddEndpoint(prefix string, e http.Router) error {
	err := e.Add(http.GET, prefix+"/hello", c.handleHello)
	logger.FatalIfErr("Add hello endpoint", err)
	return nil
}

// Close implements the Controller interface
func (c *HelloWorldController) Close() error {
	// Add any cleanup logic here if needed
	return nil
}

// handleHello is the handler for the hello endpoint
func (c *HelloWorldController) handleHello(ctx http.Context) error {
	u, err := ctx.GetUser()
	if err != nil {
		return ctx.SendUnauthorizedError()
	}
	return ctx.SendString(fmt.Sprintf("Hello, %s! You've been authenticated!\n", u))
}
