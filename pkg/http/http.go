package http

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/evgnomon/zygote/pkg/cert"
	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/go-resty/resty/v2"
)

const httpClientTimeout = 10 * time.Second

var logger = utils.NewLogger()

type HTTPTransportConfig struct {
	caCertFile   string
	funcCertFile string
	funcKeyFile  string
}

func NewHTTPTransportConfig() *HTTPTransportConfig {
	cs, err := cert.Cert()
	logger.FatalIfErr("Create cert service", err)
	return &HTTPTransportConfig{
		caCertFile:   cs.CaCertFile(),
		funcCertFile: cs.FunctionCertFile(utils.User()),
		funcKeyFile:  cs.FunctionKeyFile(utils.User()),
	}
}

func NewHTTPTransportConfigForUserHost(userName, domain string) *HTTPTransportConfig {
	cs, err := cert.Cert()
	logger.FatalIfErr("Create cert service", err)
	return &HTTPTransportConfig{
		caCertFile:   cs.CaCertFileForDomain(domain),
		funcCertFile: cs.FunctionCertFile(userName),
		funcKeyFile:  cs.FunctionKeyFile(userName),
	}
}

func (s *HTTPTransportConfig) Client() (*resty.Client, error) {
	serverCACert, err := os.ReadFile(s.caCertFile) // The CA that signed the server's certificate
	if err != nil {
		return nil, err
	}
	systemCAs, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	serverCAs := systemCAs.Clone()
	if ok := serverCAs.AppendCertsFromPEM(serverCACert); !ok {
		return nil, fmt.Errorf("failed to append server CA cert")
	}

	clientCert, err := tls.LoadX509KeyPair(s.funcCertFile, s.funcKeyFile)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      serverCAs,
		MinVersion:   tls.VersionTLS12,
	}

	client := resty.New()
	client.SetTransport(&http.Transport{
		TLSClientConfig: tlsConfig,
	})

	client.SetTimeout(httpClientTimeout)
	return client, nil
}
