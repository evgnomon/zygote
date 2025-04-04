package controller

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
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
func (c *HelloWorldController) AddEndpoint(prefix string, e *echo.Echo) error {
	// Register the hello world endpoint with the prefix
	e.GET(prefix+"/hello", c.handleHello)
	return nil
}

// Close implements the Controller interface
func (c *HelloWorldController) Close() error {
	// Add any cleanup logic here if needed
	return nil
}

// handleHello is the handler for the hello endpoint
func (c *HelloWorldController) handleHello(ctx echo.Context) error {
	clientCert := ctx.Request().TLS.PeerCertificates
	if len(clientCert) > 0 {
		return ctx.String(http.StatusOK, fmt.Sprintf("Hello, %s! You've been authenticated!\n", clientCert[0].Subject.CommonName))
	}
	return ctx.String(http.StatusUnauthorized, "Unauthorized")
}
