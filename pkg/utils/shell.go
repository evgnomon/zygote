package utils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type VaultConfig struct {
	DefaultKey string `toml:"default_key"`
}

type ZygoteConfig struct {
	Vault VaultConfig
}

const (
	dirPerm = 0755
)

func ReadConfig() (*ZygoteConfig, error) {
	doc, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".config", "zygote", "config.toml"))
	if err != nil {
		return nil, err
	}
	config := ZygoteConfig{}
	err = toml.Unmarshal(doc, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

var (
	secretsDir = filepath.Join(os.Getenv("HOME"), ".blueprint", "secrets")
)

type RunOpts struct {
	SupressStderr bool
	SupressStdout bool
}

func RunWithOpts(opts RunOpts, argv ...string) error {
	qoutedArgv := make([]string, len(argv))
	for i, arg := range argv {
		qoutedArgv[i] = fmt.Sprintf("%q", arg)
	}
	cmd := exec.Command("/bin/sh", "-c", strings.Join(qoutedArgv, " ")) // #nosec
	if !opts.SupressStderr {
		cmd.Stderr = os.Stderr
	}
	if !opts.SupressStdout {
		cmd.Stdout = os.Stdout
	}
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("run %v failed: %v", argv, err)
	}
	return nil
}

func Run(argv ...string) error {
	return RunWithOpts(RunOpts{}, argv...)
}

func RunSilent(argv ...string) error {
	return RunWithOpts(RunOpts{
		SupressStderr: true,
		SupressStdout: true,
	}, argv...)
}

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("error getting path %s: %v", path, err)
	}
	return true, nil
}

func DirExists(path string) (bool, error) {
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("error getting path %s: %v", path, err)
	}
	return stat.IsDir(), nil
}

func Chdir(path string) error {
	return os.Chdir(path)
}

func Elevate() error {
	err := Run("sudo", "-v")
	return err
}

func UnElevate() error {
	err := Run("sudo", "-K")
	return err
}

func User() string {
	return os.Getenv("USER")
}

func UserHome() string {
	return os.Getenv("HOME")
}

func EncryptFile(filename, gpgKey string) error {
	if filename == "" {
		return fmt.Errorf("please provide a file to encrypt")
	}

	zc, err := ReadConfig()
	if err != nil {
		return fmt.Errorf("failed to read config: %v", err)
	}

	// Use default key if none provided
	if gpgKey == "" {
		gpgKey = zc.Vault.DefaultKey
	}

	// Create secrets directory if it doesn't exist
	err = os.MkdirAll(secretsDir, dirPerm)
	if err != nil {
		return fmt.Errorf("failed to create secrets directory: %v", err)
	}

	// Construct output filename
	outputFile := filepath.Join(secretsDir, filename+".asc")

	// Run gpg command
	cmd := exec.Command("gpg", "-e", "-r", gpgKey, "--armor", "-o", outputFile)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("encryption failed: %v", err)
	}

	return nil
}

func DecryptFile(filename string) error {
	if filename == "" {
		return fmt.Errorf("please provide a file to decrypt")
	}

	// Construct full path to encrypted file
	inputFile := filepath.Join(secretsDir, filename+".asc")

	// Run gpg command
	cmd := exec.Command("gpg", "--quiet", "-d", inputFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("decryption failed: %v", err)
	}

	return nil
}
