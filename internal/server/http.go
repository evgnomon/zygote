package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	nethttp "net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/evgnomon/zygote/internal/cert"
	"github.com/evgnomon/zygote/pkg/http"
	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/acme/autocert"
)

var logger = utils.NewLogger()

type Server struct {
	e       *echo.Echo
	cs      *cert.CertService
	useACME bool
	domain  string
	port    int
}

func NewServer() (*Server, error) {
	s := &Server{
		e: echo.New(),
	}
	cs, err := cert.Cert()
	if err != nil {
		return nil, fmt.Errorf("failed to create cert service: %w", err)
	}
	s.cs = cs
	s.domain = utils.HostName()

	// Configure certificate handling based on ACME flag
	s.useACME = strings.ToLower(os.Getenv("ACME")) == "true"
	port := os.Getenv("ZCORE_PORT")
	if port == "" {
		s.port = 8443
	} else {
		s.port, err = strconv.Atoi(port)
		if err != nil {
			return s, fmt.Errorf("failed to parse port: %w", err)
		}
	}

	return s, nil
}
func (s *Server) tlsConfig() (*tls.Config, error) {
	// Load client CA certificate for client authentication
	clientCACert, err := os.ReadFile(s.cs.CaCertFile())
	if err != nil {
		return nil, fmt.Errorf("failed to read client CA certificate: %w", err)
	}
	clientCAs := x509.NewCertPool()
	if ok := clientCAs.AppendCertsFromPEM(clientCACert); !ok {
		return nil, fmt.Errorf("failed to append client CA certificate")
	}

	// Base TLS configuration with client authentication
	tlsConfig := &tls.Config{
		ClientCAs:  clientCAs,
		ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS12,
	}

	if s.useACME {
		// Let's Encrypt configuration
		certManager := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(s.domain),
			Cache:      autocert.DirCache(s.cs.FunctionsCertDir(s.domain)),
		}
		tlsConfig.GetCertificate = certManager.GetCertificate

		// Start HTTP server for ACME challenges
		go func() {
			logger.Info("Starting HTTP server on :80 for ACME challenges")
			// Create a new HTTP server with timeouts
			server := &nethttp.Server{
				Addr:         ":80",
				Handler:      certManager.HTTPHandler(nil),
				ReadTimeout:  10 * time.Second, // Time limit for reading the entire request
				WriteTimeout: 10 * time.Second, // Time limit for writing the response
				IdleTimeout:  30 * time.Second, // Time limit for keep-alive connections
			}

			// Start the server
			err := server.ListenAndServe()
			logger.FatalIfErr("Failed to start HTTP server for ACME challenges", err)
		}()
	} else {
		// Use local certificates
		serverCert, err := tls.LoadX509KeyPair(
			s.cs.FunctionCertFile(s.domain),
			s.cs.FunctionKeyFile(s.domain),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load server certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{serverCert}
	}
	return tlsConfig, nil
}

func (s *Server) Listen() error {
	tlsConfig, err := s.tlsConfig()
	if err != nil {
		return fmt.Errorf("failed to create TLS config: %w", err)
	}

	// Create HTTPS httpServer
	httpServer := &nethttp.Server{
		Addr:              fmt.Sprintf(":%d", s.port),
		Handler:           s.e,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second, // Time limit for reading the entire request
		WriteTimeout:      30 * time.Second, // Time limit for writing the response
		IdleTimeout:       30 * time.Second, // Time limit for keep-alive connections
	}

	// Start the server
	logger.Info("Starting serverd", utils.M{"port": s.port, "domain": s.domain})
	if s.useACME {
		err = httpServer.ListenAndServeTLS("", "")
	} else {
		err = httpServer.ListenAndServeTLS(
			s.cs.FunctionCertFile(s.domain),
			s.cs.FunctionKeyFile(s.domain),
		)
	}
	if err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	return nil
}

type Context struct {
	echo.Context
}

// BindBody implements http.Context.
func (c *Context) BindBody(b any) error {
	err := c.Bind(b)
	if err != nil {
		return c.SendError("invalid request body")
	}
	return nil
}

// GetRequestContext implements http.Context.
func (c *Context) GetRequestContext() context.Context {
	return c.Request().Context()
}

// Send implements http.Context.
func (c *Context) Send(response any) error {
	return c.JSON(nethttp.StatusOK, response)
}

// SendError implements http.Context.
func (c *Context) SendError(msg string) error {
	return c.JSON(nethttp.StatusBadRequest, map[string]any{
		"error": msg,
	})
}

// SendInternalError implements http.Context.
func (c *Context) SendInternalError(msg string, err error) error {
	logger.Error(msg, err)
	return c.String(nethttp.StatusInternalServerError, "Internal Server Error")
}

// GetUser implements http.Context.
func (c *Context) GetUser() (string, error) {
	clientCert := c.Request().TLS.PeerCertificates
	if len(clientCert) > 0 {
		return clientCert[0].Subject.CommonName, nil
	}
	return "", fmt.Errorf("no client certificate found")
}

// SendString implements http.Context.
func (c *Context) SendString(response string) error {
	return c.String(nethttp.StatusOK, response)
}

// SendUnauthorizedError implements http.Context.
func (c *Context) SendUnauthorizedError() error {
	return c.String(nethttp.StatusUnauthorized, "Unauthorized")
}

func NewContext(c echo.Context) http.Context {
	return &Context{
		c,
	}
}

func (s *Server) Add(method http.Method, path string, handler func(http.Context) error, _ ...http.RouteOpt) error {
	switch method {
	case http.GET:
		s.e.GET(path, func(c echo.Context) error {
			return handler(NewContext(c))
		})
	case http.POST:
		s.e.POST(path, func(c echo.Context) error {
			return handler(NewContext(c))
		})
	case http.PUT:
		s.e.PUT(path, func(c echo.Context) error {
			return handler(NewContext(c))
		})
	case http.DELETE:
		s.e.DELETE(path, func(c echo.Context) error {
			return handler(NewContext(c))
		})
	case http.PATCH:
		s.e.PATCH(path, func(c echo.Context) error {
			return handler(NewContext(c))
		})
	}
	return fmt.Errorf("unsupported method")
}

func (s *Server) AddControllers(controllers []http.Controller) error {
	for _, c := range controllers {
		err := c.AddEndpoint("", s)
		if err != nil {
			return fmt.Errorf("failed to add endpoint: %w", err)
		}
	}
	return nil
}
