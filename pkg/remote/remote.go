/*
Copyright (C) 2025- Hamed Ghasemzadeh. All rights reserved.
License: HGL General License <http://evgnomon.org/docs/hgl>
*/

package remote

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

const dirPermission = 0755

type SSHClient struct {
	client *ssh.Client
}

// NewSSHClient creates a new SSH client using key-based authentication
func NewSSHClient(user, host string) (*SSHClient, error) {
	// Try SSH agent first
	authMethods := []ssh.AuthMethod{}

	if sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		authMethods = append(authMethods, ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers))
	}

	// Fallback to default SSH key (~/.ssh/id_rsa) if no agent
	if len(authMethods) == 0 {
		key, err := os.ReadFile(os.Getenv("HOME") + "/.ssh/id_aurora")
		if err != nil {
			return nil, fmt.Errorf("failed to read private key: %v", err)
		}

		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %v", err)
		}

		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //#nosec
	}

	client, err := ssh.Dial("tcp", host+":22", config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %v", err)
	}

	return &SSHClient{client: client}, nil
}

// CopyFileToRemote copies a file with in-memory content to the specified remote path
func (c *SSHClient) CopyFileToRemote(content []byte, remotePath string) error {
	// Create a new SSH session
	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	// Create remote directory if it doesn't exist
	dir := filepath.Dir(remotePath)
	if err := c.CreateRemoteDir(dir); err != nil {
		return err
	}

	// Open SFTP connection
	goPipe, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %v", err)
	}
	defer goPipe.Close()

	// Send the command to write the file
	cmd := fmt.Sprintf("cat > %s", remotePath)
	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("failed to start command: %v", err)
	}

	// Write the content from memory to the remote file
	_, err = io.Copy(goPipe, bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("failed to write content: %v", err)
	}

	// Close the pipe and wait for the command to complete
	goPipe.Close()
	if err := session.Wait(); err != nil {
		return fmt.Errorf("command failed: %v", err)
	}

	return nil
}

// CreateRemoteDir creates a directory on the remote host if it doesn't exist
func (c *SSHClient) CreateRemoteDir(dir string) error {
	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session for mkdir: %v", err)
	}
	defer session.Close()

	cmd := fmt.Sprintf("mkdir -p %s", dir)
	if err := session.Run(cmd); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}
	return nil
}

// Close closes the SSH client connection
func (c *SSHClient) Close() {
	c.client.Close()
}

// CommandResult holds the results of a remote command execution
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// RunCommand executes a command on the remote host and returns stdout, stderr, and exit status
func (c *SSHClient) RunCommand(command string) (CommandResult, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return CommandResult{}, fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	err = session.Run(command)

	result := CommandResult{
		Stdout: stdoutBuf.String(),
		Stderr: stderrBuf.String(),
	}

	// Check if the command failed (non-zero exit status)
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			result.ExitCode = exitErr.ExitStatus()
			return result, fmt.Errorf("command '%s' failed with exit code %d: %s",
				command, result.ExitCode, stderrBuf.String())
		}
		return result, fmt.Errorf("failed to execute command '%s': %v", command, err)
	}

	result.ExitCode = 0
	return result, nil
}

// ReadFileFromRemote reads a file from the remote host and returns its content in memory
func (c *SSHClient) ReadFileFromRemote(remotePath string) ([]byte, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	var buf bytes.Buffer
	cmd := fmt.Sprintf("cat %s", remotePath)
	session.Stdout = &buf

	err = session.Run(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %v", remotePath, err)
	}

	return buf.Bytes(), nil
}

// ListRemoteDir lists the contents of a remote directory
func (c *SSHClient) ListRemoteDir(remotePath string) ([]string, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	var buf bytes.Buffer
	cmd := fmt.Sprintf("ls -1 %s", remotePath)
	session.Stdout = &buf

	err = session.Run(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory %s: %v", remotePath, err)
	}

	output := strings.TrimSpace(buf.String())
	if output == "" {
		return []string{}, nil
	}
	entries := strings.Split(output, "\n")

	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry != "" {
			result = append(result, entry)
		}
	}

	return result, nil
}

// CopyLocalToRemote copies a file from local host to remote host using streams
func (c *SSHClient) CopyLocalToRemote(localPath, remotePath string) error {
	// Open local file
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file %s: %v", localPath, err)
	}
	defer localFile.Close()

	// Create SSH session
	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	// Create remote directory
	dir := filepath.Dir(remotePath)
	if err := c.CreateRemoteDir(dir); err != nil {
		return err
	}

	// Get stdin pipe for streaming
	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %v", err)
	}
	defer stdin.Close()

	// Start remote command
	cmd := fmt.Sprintf("cat > %s", remotePath)
	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("failed to start command: %v", err)
	}

	// Stream the file content
	_, err = io.Copy(stdin, localFile)
	if err != nil {
		return fmt.Errorf("failed to stream content to remote: %v", err)
	}

	// Close stdin and wait for completion
	stdin.Close()
	if err := session.Wait(); err != nil {
		return fmt.Errorf("remote command failed: %v", err)
	}

	return nil
}

// CopyRemoteToLocal copies a file from remote host to local host using streams
func (c *SSHClient) CopyRemoteToLocal(remotePath, localPath string) error {
	// Create SSH session
	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	// Create local directory
	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, dirPermission); err != nil {
		return fmt.Errorf("failed to create local directory %s: %v", localDir, err)
	}

	// Create local file
	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file %s: %v", localPath, err)
	}
	defer localFile.Close()

	// Get stdout pipe for streaming
	stdout, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	// Start remote command
	cmd := fmt.Sprintf("cat %s", remotePath)
	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("failed to start command: %v", err)
	}

	// Stream the content to local file
	_, err = io.Copy(localFile, stdout)
	if err != nil {
		return fmt.Errorf("failed to stream content from remote: %v", err)
	}

	// Wait for completion
	if err := session.Wait(); err != nil {
		return fmt.Errorf("remote command failed: %v", err)
	}

	return nil
}
