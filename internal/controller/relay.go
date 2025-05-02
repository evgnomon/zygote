package controller

import (
	"fmt"
	nethttp "net/http"
	"net/http/httputil"
	"net/url"

	"github.com/evgnomon/zygote/pkg/http"
)

// RelayController gets HTTP requests and pass that to another server
type RelayController struct {
	// Add any necessary fields here
	proxy     *httputil.ReverseProxy
	targetURL *url.URL
}

// NewHelloWorldController creates a new instance of HelloWorldController
func NewRelayController() *RelayController {
	targetURL, _ := url.Parse("http://localhost:3000/")
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	// Optional: Modify proxy error handling
	proxy.ErrorHandler = func(w nethttp.ResponseWriter, _ *nethttp.Request, _ error) {
		nethttp.Error(w, "Error proxying request", nethttp.StatusBadGateway)
	}
	return &RelayController{proxy: proxy, targetURL: targetURL}
}

// AddEndpoint implements the Controller interface
func (c *RelayController) AddEndpoint(prefix string, e http.Router) error {
	err := e.Add(http.ANY, prefix+"/*", c.handleHello)
	logger.FatalIfErr("Add hello endpoint", err)
	return nil
}

// Close implements the Controller interface
func (c *RelayController) Close() error {
	// Add any cleanup logic here if needed
	return nil
}

// handleHello is the handler for the hello endpoint
func (c *RelayController) handleHello(ctx http.Context) error {
	u, err := ctx.GetUser()
	req := ctx.Request()
	req.Host = c.targetURL.Host
	req.URL.Scheme = c.targetURL.Scheme
	req.URL.Host = c.targetURL.Host
	req.URL.Path = ctx.Path()
	c.proxy.ServeHTTP(ctx.ResponseWriter(), req)
	if err != nil {
		return ctx.SendUnauthorizedError()
	}
	return ctx.SendString(fmt.Sprintf("Hello, %s! You've been authenticated!\n", u))
}
