package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/evgnomon/zygote/internal/cert"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/acme/autocert"
)

func main() {
	cs, err := cert.Cert()
	if err != nil {
		log.Fatalf("Failed to load server certificate: %v", err)
	}
	domainName := "myservice.zygote.run"

	if os.Getenv("ZCORE_DOMAIN") != "" {
		domainName = os.Getenv("ZCORE_DOMAIN")
	}

	// Load client CA certificate for client authentication
	clientCACert, err := os.ReadFile(cs.CaCertFile())
	if err != nil {
		log.Fatalf("Failed to read client CA certificate: %v", err)
	}
	clientCAs := x509.NewCertPool()
	if ok := clientCAs.AppendCertsFromPEM(clientCACert); !ok {
		log.Fatalf("Failed to append client CA certificate")
	}

	// Base TLS configuration with client authentication
	tlsConfig := &tls.Config{
		ClientCAs:  clientCAs,
		ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS12,
	}

	// Configure certificate handling based on ACME flag
	useACME := strings.ToLower(os.Getenv("ACME")) == "true"
	if useACME {
		// Let's Encrypt configuration
		certManager := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(domainName),
			Cache:      autocert.DirCache(cs.FunctionsCertDir(domainName)),
		}
		tlsConfig.GetCertificate = certManager.GetCertificate

		// Start HTTP server for ACME challenges
		go func() {
			log.Printf("Starting HTTP server on :80 for Let's Encrypt challenges")
			err := http.ListenAndServe(":80", certManager.HTTPHandler(nil))
			if err != nil {
				log.Printf("HTTP server error: %v", err)
			}
		}()
	} else {
		// Use local certificates
		serverCert, err := tls.LoadX509KeyPair(cs.FunctionCertFile(domainName), cs.FunctionKeyFile(domainName))
		if err != nil {
			log.Fatalf("Failed to load server certificate: %v", err)
		}
		tlsConfig.Certificates = []tls.Certificate{serverCert}
	}

	// Create Echo instance
	e := echo.New()

	// Define authenticated endpoint
	e.GET("/", func(c echo.Context) error {
		clientCert := c.Request().TLS.PeerCertificates
		if len(clientCert) > 0 {
			return c.String(http.StatusOK, fmt.Sprintf("Hello, %s! You've been authenticated!\n", clientCert[0].Subject.CommonName))
		}
		return c.String(http.StatusUnauthorized, "Unauthorized")
	})

	port := os.Getenv("ZCORE_PORT")
	if port == "" {
		port = "8443"
	}

	// Create HTTPS server
	server := &http.Server{
		Addr:              fmt.Sprintf(":%s", port),
		Handler:           e,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Start the server
	log.Printf("Starting server on https://%s:%s\n", domainName, port)
	if useACME {
		err = server.ListenAndServeTLS("", "")
	} else {
		err = server.ListenAndServeTLS(cs.FunctionCertFile(domainName), cs.FunctionKeyFile(domainName))
	}
	if err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
