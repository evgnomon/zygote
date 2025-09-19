package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evgnomon/zygote/pkg/utils"
)

const certDirPermission = 0700
const int64Bits = 128

var logger = utils.NewLogger()

type CertService struct {
	ConfigHome string
}

func Cert() (*CertService, error) {
	if os.Getenv("ZYGOTE_CONFIG_HOME") != "" {
		cs := CertService{
			ConfigHome: os.Getenv("ZYGOTE_CONFIG_HOME"),
		}
		return &cs, nil
	}
	userDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	if strings.Contains(userDir, "root") {
		sudoUser := os.Getenv("SUDO_USER")
		if sudoUser != "" {
			userDir = "/home/" + sudoUser
		}
	}
	cs := CertService{
		ConfigHome: filepath.Join(userDir, ".config", "zygote"),
	}
	return &cs, nil
}

func (c *CertService) CaCertDir() string {
	p := filepath.Join(c.ConfigHome, "certs", "ca")
	if _, err := os.Stat(p); os.IsNotExist(err) {
		err = os.MkdirAll(p, certDirPermission)
		if err != nil {
			log.Fatal(err)
		}
	}
	return p
}

// Get ca cert file path
func (c *CertService) CaCertFile() string {
	return filepath.Join(c.CaCertDir(), "ca_cert.pem")
}

func (c *CertService) FunctionCert(name string) string {
	logger.Info("Get function cert", utils.M{"name": name})
	f := c.FunctionCertFileByContainer(name)
	c.EnsureFunctionCert(name)
	a, err := os.ReadFile(f)
	logger.FatalIfErr("Read function cert file", err, utils.M{"file": f})
	return string(a)
}

func (c *CertService) EnsureFunctionCert(name string) {
	f := c.FunctionCertFileByContainer(name)
	if _, err := os.Stat(f); os.IsNotExist(err) {
		logger.Info("Ensure function cert", utils.M{"name": name})
		err := c.Sign([]string{ContainerCertName(name)}, []string{"127.0.0.1"}, time.Now().AddDate(1, 0, 0), "")
		logger.FatalIfErr("Auto generate function cert", err, utils.M{"file": f})
	}
}

func (c *CertService) FunctionKey(name string) string {
	c.EnsureFunctionCert(name)
	f := c.FunctionKeyFileByContainer(name)
	a, err := os.ReadFile(f)
	logger.FatalIfErr("Read function key file", err, utils.M{"file": f})
	return string(a)
}

// Get ca cert file path
func (c *CertService) CaCertPublic() string {
	a, err := os.ReadFile(c.CaCertFile())
	logger.FatalIfErr("Read CA cert file", err)
	return string(a)
}

// Get ca cert file path
func (c *CertService) CaCertPathForDomain(domain string) string {
	f := filepath.Join(c.CaCertDir(), fmt.Sprintf("%s.pem", domain))
	if _, err := os.Stat(f); os.IsNotExist(err) {
		return c.CaCertFile()
	}
	return f
}

func (c *CertService) FunctionsCertDir(name string) string {
	p := filepath.Join(c.ConfigHome, "certs", "functions", name)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		err = os.MkdirAll(p, certDirPermission)
		if err != nil {
			log.Fatal(err)
		}
	}
	return p
}

func ContainerCertName(containerName string) string {
	if utils.DomainName() == "my.zygote.run" {
		logger.Info("Using container name as cert name for my.zygote.run", utils.M{"container": containerName})
		return containerName
	}
	domain := utils.DomainName()
	node_type := utils.NodeType()
	suffix := utils.NodeSuffix()
	if suffix == "" {
		if node_type == "" {
			return containerName + "." + domain
		}
		return fmt.Sprintf("%s-%s.%s", node_type, containerName, domain)
	}
	name := fmt.Sprintf("%s-%s-%s.%s", node_type, containerName, suffix, domain)
	return name
}

func (c *CertService) FunctionCertFileByContainer(containerName string) string {
	logger.Info("Get function cert by container", utils.M{"container": containerName})
	return c.FunctionCertPath(ContainerCertName(containerName))
}

func (c *CertService) FunctionCertFileByHost() string {
	domain := utils.DomainName()
	node_type := utils.NodeType()
	suffix := utils.NodeSuffix()
	if suffix == "" {
		if node_type == "" {
			return c.FunctionCertPath(domain)
		}
		return c.FunctionCertPath(node_type + "." + domain)
	}
	name := fmt.Sprintf("%s-%s.%s", node_type, suffix, domain)
	return c.FunctionCertPath(name)
}

func (c *CertService) FunctionCertPath(name string) string {
	return filepath.Join(c.FunctionsCertDir(name), fmt.Sprintf("%s_cert.pem", name))
}

func (c *CertService) FunctionKeyFileByContainer(containerName string) string {
	return c.FunctionKeyPath(ContainerCertName(containerName))
}

func (c *CertService) FunctionKeyFileByHost() string {
	domain := utils.DomainName()
	node_type := utils.NodeType()
	suffix := utils.NodeSuffix()
	if suffix == "" {
		if node_type == "" {
			return c.FunctionKeyPath(domain)
		}
		return c.FunctionKeyPath(node_type + "." + domain)
	}
	name := fmt.Sprintf("%s-%s.%s", node_type, suffix, domain)
	return c.FunctionKeyPath(name)
}

func (c *CertService) FunctionKeyPath(name string) string {
	return filepath.Join(c.FunctionsCertDir(name), fmt.Sprintf("%s_key.pem", name))
}

func (c *CertService) MakeRootCert(expiresAt time.Time) error {
	perFilePath := filepath.Join(c.CaCertDir(), "ca_key.pem")
	certExist := utils.PathExists(perFilePath)
	if certExist {
		logger.Info("Root cert already exists, skipping generation")
		return nil
	}

	caPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), int64Bits))
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %v", err)
	}

	caTemplate := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{CommonName: "Zygote Root CA"},
		NotBefore:             time.Now(),
		NotAfter:              expiresAt,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caPriv.PublicKey, caPriv)
	if err != nil {
		return err
	}

	caCertOut, err := os.Create(c.CaCertFile())

	if err != nil {
		return err
	}
	err = pem.Encode(caCertOut, &pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})
	if err != nil {
		return err
	}
	caCertOut.Close()

	caKeyOut, err := os.Create(perFilePath)
	if err != nil {
		return err
	}
	caPrivBytes, err := x509.MarshalECPrivateKey(caPriv)
	if err != nil {
		return err
	}
	err = pem.Encode(caKeyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: caPrivBytes})
	if err != nil {
		return err
	}
	caKeyOut.Close()

	return nil
}

func (c *CertService) Sign(domainName, ipAddresses []string, expiresAt time.Time, password string) error {
	logger.Info("Signing certificate", utils.M{"domain": domainName, "ip": ipAddresses})
	caKeyPEM, err := os.ReadFile(filepath.Join(c.CaCertDir(), "ca_key.pem"))
	if err != nil {
		return err
	}
	caKeyBlock, _ := pem.Decode(caKeyPEM)
	caPriv, err := x509.ParseECPrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return err
	}

	caCertPEM, err := os.ReadFile(filepath.Join(c.CaCertDir(), "ca_cert.pem"))
	if err != nil {
		return err
	}
	caCertBlock, _ := pem.Decode(caCertPEM)
	caCert, err := x509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return err
	}

	// Generate a private key for the server certificate
	serverPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), int64Bits))
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %v", err)
	}
	// Parse IP addresses
	var ips []net.IP
	for _, ipStr := range ipAddresses {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return fmt.Errorf("invalid IP address: %s", ipStr)
		}
		ips = append(ips, ip)
	}
	serverTemplate := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: domainName[0],
		},
		NotBefore:   time.Now(),
		NotAfter:    expiresAt,
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageCodeSigning},
		DNSNames:    domainName,
		IPAddresses: ips,
	}

	serverCertDER, err := x509.CreateCertificate(rand.Reader, &serverTemplate, caCert, &serverPriv.PublicKey, caPriv)
	if err != nil {
		return err
	}

	pubFilePath := filepath.Join(c.FunctionsCertDir(domainName[0]), fmt.Sprintf("%s_cert.pem", domainName[0]))
	serverCertOut, err := os.Create(pubFilePath)
	if err != nil {
		return err
	}
	err = pem.Encode(serverCertOut, &pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})
	if err != nil {
		return err
	}
	serverCertOut.Close()
	keyFilePath := filepath.Join(c.FunctionsCertDir(domainName[0]), fmt.Sprintf("%s_key.pem", domainName[0]))

	serverKeyOut, err := os.Create(keyFilePath)
	if err != nil {
		return err
	}
	serverPrivBytes, err := x509.MarshalECPrivateKey(serverPriv)
	if err != nil {
		return err
	}
	err = pem.Encode(serverKeyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: serverPrivBytes})
	if err != nil {
		return err
	}
	serverKeyOut.Close()

	p12FilePath := filepath.Join(c.FunctionsCertDir(domainName[0]), fmt.Sprintf("%s.p12", domainName[0]))
	if password == "" {
		err = utils.Run("openssl", "pkcs12", "-export", "-in", pubFilePath, "-inkey", keyFilePath, "-out", p12FilePath, "-passout", "pass:")
	} else {
		err = utils.Run("openssl", "pkcs12", "-export", "-in", pubFilePath, "-inkey", keyFilePath, "-out", p12FilePath)
	}
	if err != nil {
		return fmt.Errorf("failed to create p12 file: %v", err)
	}
	return nil
}

func TLSConfig(name string) *tls.Config {
	s, err := Cert()
	logger.FatalIfErr("Create cert service", err, utils.M{"name": name})
	caCert := s.CaCertPublic()
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM([]byte(caCert)) {
		logger.Fatal("Failed to append CA certificate", utils.M{"name": name})
	}
	s.EnsureFunctionCert(name)
	tlsConfig := &tls.Config{
		RootCAs:    caCertPool,
		MinVersion: tls.VersionTLS12,
		Certificates: func() []tls.Certificate {
			cert, err := tls.LoadX509KeyPair(
				s.FunctionCertPath(ContainerCertName(name)),
				s.FunctionKeyPath(ContainerCertName(name)),
			)
			logger.FatalIfErr("Load client certificate", err, utils.M{"name": name})
			return []tls.Certificate{cert}
		}(),
	}

	return tlsConfig
}
