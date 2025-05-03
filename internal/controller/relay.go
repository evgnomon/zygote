package controller

import (
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
	base      string
}

// NewHelloWorldController creates a new instance of HelloWorldController
func NewRelayController(base, target string) *RelayController {
	targetURL, _ := url.Parse(target)
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	// Optional: Modify proxy error handling
	proxy.ErrorHandler = func(w nethttp.ResponseWriter, _ *nethttp.Request, _ error) {
		nethttp.Error(w, "Error proxying request", nethttp.StatusBadGateway)
	}
	return &RelayController{proxy: proxy, targetURL: targetURL, base: base}
}

// AddEndpoint implements the Controller interface
func (c *RelayController) AddEndpoint(prefix string, e http.Router) error {
	if c.base != "" {
		err := e.Add(http.ANY, prefix+"/"+c.base+"/*", c.handleRelay)
		logger.FatalIfErr("Add relay endpoint", err)
		return nil
	}
	err := e.Add(http.ANY, prefix+"/*", c.handleRelay)
	logger.FatalIfErr("Add relay endpoint", err)
	return nil
}

// Close implements the Controller interface
func (c *RelayController) Close() error {
	// Add any cleanup logic here if needed
	return nil
}

// handleRelay is the handler for the hello endpoint
func (c *RelayController) handleRelay(ctx http.Context) error {
	_, err := ctx.GetUser()
	if err != nil {
		return ctx.SendUnauthorizedError()
	}
	req := ctx.Request()
	req.Host = c.targetURL.Host
	req.URL.Scheme = c.targetURL.Scheme
	req.URL.Host = c.targetURL.Host
	req.URL.Path = ctx.Path()
	c.proxy.ServeHTTP(ctx.ResponseWriter(), req)
	return nil
}
