package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/evgnomon/zygote/internal/cert"
	"github.com/gin-gonic/gin"
)

func main() {
	cs, err := cert.Cert()
	if err != nil {
		log.Fatalf("Failed to load server certificate: %v", err)
	}
	domainName := "localhost"
	serverCert, err := tls.LoadX509KeyPair(cs.FunctionCertFile(domainName), cs.FunctionKeyFile(domainName))
	if err != nil {
		log.Fatalf("Failed to load server certificate: %v", err)
	}

	// Load and trust the client CA certificate
	clientCACert, err := os.ReadFile(cs.CaCertFile())
	if err != nil {
		log.Fatalf("Failed to read client CA certificate: %v", err)
	}
	clientCAs := x509.NewCertPool()
	if ok := clientCAs.AppendCertsFromPEM(clientCACert); !ok {
		log.Fatalf("Failed to append client CA certificate")
	}

	// Configure TLS to require client certificates
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    clientCAs,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}

	// Create a new Gin router
	router := gin.Default()

	// Define a simple authenticated endpoint
	router.GET("/", func(c *gin.Context) {
		clientCert := c.Request.TLS.PeerCertificates
		if len(clientCert) > 0 {
			c.String(http.StatusOK, fmt.Sprintf("Hello, %s! You've been authenticated!\n", clientCert[0].Subject.CommonName))
		} else {
			c.String(http.StatusUnauthorized, "Unauthorized")
		}
	})

	// Create the HTTPS server with the TLS configuration
	server := &http.Server{
		Addr:              ":8443",
		Handler:           router,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Start the HTTPS server
	log.Printf("Starting server on https://%s:8443\n", domainName)
	err = server.ListenAndServeTLS(cs.FunctionCertFile(domainName), cs.FunctionKeyFile(domainName))
	if err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
