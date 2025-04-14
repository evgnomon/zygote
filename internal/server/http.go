package server

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/evgnomon/zygote/internal/cert"
	"github.com/evgnomon/zygote/internal/controller"
	"github.com/evgnomon/zygote/internal/util"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/acme/autocert"
)

var logger = util.NewLogger()

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
	s.domain = "myservice.zygote.run"
	if os.Getenv("ZCORE_DOMAIN") != "" {
		s.domain = os.Getenv("ZCORE_DOMAIN")
	}

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
			server := &http.Server{
				Addr:         ":80",
				Handler:      certManager.HTTPHandler(nil),
				ReadTimeout:  10 * time.Second, // Time limit for reading the entire request
				WriteTimeout: 10 * time.Second, // Time limit for writing the response
				IdleTimeout:  30 * time.Second, // Time limit for keep-alive connections
			}

			// Start the server
			err := server.ListenAndServe()
			if err != nil {
				// Handle error
				panic(err)
			}
			if err != nil {
				logger.Error("Failed to start HTTP server for ACME challenges", util.WrapError(err))
			}
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
	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", s.port),
		Handler:           s.e,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second, // Time limit for reading the entire request
		WriteTimeout:      30 * time.Second, // Time limit for writing the response
		IdleTimeout:       30 * time.Second, // Time limit for keep-alive connections
	}

	// Start the server
	logger.Info("Starting serverd", util.M{"port": s.port, "domain": s.domain})
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

func (s *Server) AddControllers(controllers []controller.Controller) error {
	for _, c := range controllers {
		err := c.AddEndpoint("", s.e)
		if err != nil {
			return fmt.Errorf("failed to add endpoint: %w", err)
		}
	}
	return nil
}
