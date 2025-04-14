package container

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/user"
	"runtime"
	"strings"
	"sync"
	"time"

	containertypes "github.com/docker/docker/api/types/container"
	imagetypes "github.com/docker/docker/api/types/image"
	networktypes "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
	"github.com/evgnomon/zygote/internal/util"
)

const (
	HostNetworkName              = "host"
	defaultContainerStartTimeout = 20 * time.Second
	defaultNetworkName           = "mynet"
	dockerVersion                = "1.41"
	networkNameEnvVar            = "DOCKER_NETWORK_NAME"
)

var logger = util.NewLogger()

func AppNetworkName() string {
	if os.Getenv(networkNameEnvVar) != "" {
		return os.Getenv(networkNameEnvVar)
	}
	return defaultNetworkName
}

func CreateClinet() (*client.Client, error) {
	var dockerPath string
	switch runtime.GOOS {
	case "windows":
		dockerPath = "npipe:////./pipe/docker_engine"
	case "linux":
		dockerPath = "unix:///var/run/docker.sock"
	case "darwin":
		homePath := os.Getenv("HOME")
		dockerPath = fmt.Sprintf("unix://%s/.docker/run/docker.sock", homePath)
	default:
		panic("Unsupported OS")
	}
	cli, err := client.NewClientWithOpts(client.WithVersion(dockerVersion), client.WithHost(dockerPath))
	if err != nil {
		return nil, err
	}
	return cli, nil
}

func Spawn(ctx context.Context,
	cli *client.Client,
	name string,
	cmd []string,
	portMap map[string]string,
	networkName string,
) {
	config := &containertypes.Config{
		Image:        name,
		Cmd:          cmd,
		AttachStdout: true,
	}
	portBindings := nat.PortMap{}

	for containerPort, hostPort := range portMap {
		portBindings[nat.Port(containerPort)] = []nat.PortBinding{
			{HostIP: "0.0.0.0", HostPort: hostPort},
		}
	}
	hostConfig := &containertypes.HostConfig{
		AutoRemove:   true,
		PortBindings: portBindings,
	}
	if networkName != "" {
		hostConfig.NetworkMode = containertypes.NetworkMode(networkName)
	}
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, "")
	if err != nil {
		panic(err)
	}

	// Attach to STDOUT before starting
	attachOptions := containertypes.AttachOptions{
		Stream: true,
		Stdout: true,
		Stderr: true,
	}

	attachResponse, err := cli.ContainerAttach(ctx, resp.ID, attachOptions)
	if err != nil {
		panic(err)
	}

	defer attachResponse.Close()

	if err := cli.ContainerStart(ctx, resp.ID, containertypes.StartOptions{}); err != nil {
		panic(err)
	}

	_, err = io.Copy(os.Stdout, attachResponse.Reader)
	if err != nil {
		panic(err)
	}
}

func SpawnAndWait(ctx context.Context,
	cli *client.Client,
	name, tenant string,
	cmd []string,
	portMap map[string]string,
	networkName string,
) error { // Return error instead of panicking
	config := &containertypes.Config{
		Image:        name,
		Cmd:          cmd,
		AttachStdout: true,
	}
	portBindings := nat.PortMap{}
	for containerPort, hostPort := range portMap {
		portBindings[nat.Port(containerPort)] = []nat.PortBinding{
			{HostIP: "0.0.0.0", HostPort: hostPort},
		}
	}
	hostConfig := &containertypes.HostConfig{
		AutoRemove:   true,
		PortBindings: portBindings,
	}
	if networkName != "" {
		hostConfig.NetworkMode = containertypes.NetworkMode(networkName)
	}

	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, "")
	if err != nil {
		return fmt.Errorf("failed to create container: %v", err)
	}

	attachOptions := containertypes.AttachOptions{
		Stream: true,
		Stdout: true,
		Stderr: true,
	}
	attachResponse, err := cli.ContainerAttach(ctx, resp.ID, attachOptions)
	if err != nil {
		return fmt.Errorf("failed to attach to container: %v", err)
	}
	defer attachResponse.Close()

	if err := cli.ContainerStart(ctx, resp.ID, containertypes.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %v", err)
	}

	// Stream output to stdout
	go func() {
		data, err := io.ReadAll(attachResponse.Reader)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error copying output: %v\n", err)
		}
		logger.Debug("Container output: %s", util.M{"message": data})
	}()

	// Wait for the container to finish and get its exit code
	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, containertypes.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("error waiting for container: %v", err)
		}
	case status := <-statusCh:
		if status.StatusCode != 0 {
			return fmt.Errorf("container exited with non-zero status: %d", status.StatusCode)
		}
	}

	return nil
}

func SpawnWithInput(
	name, tenant string,
	cmd []string,
	portMap map[string]string,
	volumeMap map[string]string,
	networkName string,
	inputStr string) {
	ctx := context.Background()

	cli, err := CreateClinet()
	if err != nil {
		panic(err)
	}

	var binds []string
	for volumeName, mountPath := range volumeMap {
		_, err := cli.VolumeInspect(ctx, volumeName)
		if err != nil {
			_, err := cli.VolumeCreate(ctx, volume.CreateOptions{Name: volumeName})
			if err != nil {
				panic(err)
			}
		}
		bind := volumeName + ":" + mountPath
		binds = append(binds, bind)
	}

	_, err = cli.NetworkInspect(ctx, networkName, networktypes.InspectOptions{})
	if err != nil {
		_, err = cli.NetworkCreate(ctx, networkName, networktypes.CreateOptions{})
	}
	if err != nil {
		panic(err)
	}

	config := &containertypes.Config{
		Image:        name,
		Cmd:          cmd,
		AttachStdout: false,
		AttachStdin:  true, // Enable STDIN attachment
		AttachStderr: false,
		// Tty:          true,
		OpenStdin:       true,
		StdinOnce:       true,
		NetworkDisabled: true,
	}
	portBindings := nat.PortMap{}

	for containerPort, hostPort := range portMap {
		portBindings[nat.Port(containerPort)] = []nat.PortBinding{
			{HostIP: "0.0.0.0", HostPort: hostPort},
		}
	}
	hostConfig := &containertypes.HostConfig{
		AutoRemove:   true,
		PortBindings: portBindings,
		Binds:        binds,
	}
	if networkName != "" {
		hostConfig.NetworkMode = containertypes.NetworkMode(networkName)
	}

	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, fmt.Sprintf("%s-%d", tenant, time.Now().UnixNano()))
	if err != nil {
		panic(err)
	}

	// Attach to STDIN, STDOUT, and STDERR before starting
	attachOptions := containertypes.AttachOptions{
		Stream: true,
		Stdin:  inputStr != "", // Enable STDIN attachment
		Stdout: false,
		Stderr: false,
	}

	attachResponse, err := cli.ContainerAttach(ctx, resp.ID, attachOptions)
	if err != nil {
		panic(err)
	}

	defer attachResponse.Close()

	var wg sync.WaitGroup
	if inputStr != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Write your input string to the container's STDIN
			inputWithNewline := inputStr + "\n"
			n, err := attachResponse.Conn.Write([]byte(inputWithNewline))
			if err != nil {
				logger.Fatal("Failed to write to container's STDIN", util.WrapError(err))
			}
			if n != len(inputWithNewline) {
				logger.Fatal("Failed to write to container's STDIN", util.M{"expected": len(inputWithNewline), "actual": n})
			}
			time.Sleep(1 * time.Second)
			attachResponse.Close()
		}()

		wg.Wait()
	}

	if err := cli.ContainerStart(ctx, resp.ID, containertypes.StartOptions{}); err != nil {
		panic(err)
	}

	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, containertypes.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			panic(err)
		}
	case <-statusCh:
		// Container has stopped
	}
}

// Ark runs a script inside a Docker container with the specified configuration.
//
// Parameters:
//   - script: The script to be executed inside the container.
//
// The function performs the following tasks:
//  1. Adds an 'exit 0' to the script to ensure graceful termination.
//  2. Sets the container image and other configurations.
//  3. Spawns the container with the provided input.
func Ark(tenant, script string) {
	scriptWithExit := fmt.Sprintf("%s \n exit 0;", script)

	imageName := "ghcr.io/evgnomon/ark:main"
	command := []string{"bash"}
	portMapping := map[string]string{}
	networkName := AppNetworkName()

	SpawnWithInput(imageName, tenant, command, portMapping, nil, networkName, scriptWithExit)
}

// Vol is a function that creates a Docker container from a specific image and runs a command on it.
// The command creates a file with some content in a specified directory.
// The function also maps a volume and a network for the container.

// Parameters:
// - srcContent (string): The content that will be written in the target file.
// - targetVolume (string): The name of the volume that will be mapped to the container.
// - targetDir (string): The directory inside the container where the target file will be created.
// - targetFile (string): The name of the file that will be created.
// - networkName (string): The name of the network that the container will be connected to.

// It uses a defer statement to recover from potential panics and log them.
// The Docker image used is ghcr.io/evgnomon/ark:main.
func Vol(tenant, srcContent, targetVolume, targetDir, targetFile, networkName string) {
	imageName := "ghcr.io/evgnomon/ark:main"
	Pull(context.Background(), imageName)
	command := []string{"bash", "-c", fmt.Sprintf("tee %s/%s", targetDir, targetFile)}
	portMapping := make(map[string]string)

	volMap := map[string]string{
		targetVolume: targetDir,
	}

	SpawnWithInput(imageName, tenant, command, portMapping, volMap, networkName, srcContent)
}

type Container struct {
	Name string
	ID   string
}

type ListOption func(*containertypes.ListOptions)

func ListRunningContainers(opt *containertypes.ListOptions) {
	opt.All = false
}

func List(opts ...ListOption) []*Container {
	cli, err := CreateClinet()
	if err != nil {
		panic(err)
	}

	opt := containertypes.ListOptions{All: true}
	for _, o := range opts {
		o(&opt)
	}

	containers, err := cli.ContainerList(context.Background(), opt)
	if err != nil {
		panic(err)
	}

	result := []*Container{}

	for i := range containers {
		result = append(result, &Container{
			Name: containers[i].Names[0],
			ID:   containers[i].ID,
		})
	}
	return result
}

func RemoveContainer(containerID string) {
	ctx := context.Background()
	cli, err := CreateClinet()
	if err != nil {
		panic(err)
	}

	removeOptions := containertypes.RemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	}

	err = cli.ContainerStop(ctx, containerID, containertypes.StopOptions{Signal: "SIGTERM"})
	if err != nil {
		panic(err)
	}

	if err := cli.ContainerRemove(ctx, containerID, removeOptions); err != nil {
		panic(err)
	}
}

func RemoveVolumePrefix(volumePrefix string) {
	cli, err := CreateClinet()
	if err != nil {
		panic(err)
	}

	volumes, err := cli.VolumeList(context.Background(), volume.ListOptions{})
	if err != nil {
		panic(err)
	}

	for _, volume := range volumes.Volumes {
		if strings.HasPrefix(volume.Name, volumePrefix) {
			err := cli.VolumeRemove(context.Background(), volume.Name, false)
			if err != nil {
				logger.Error("Error removing volume", util.WrapError(err))
			}
		}
	}
}

func WaitHealthy(namePrefix string, timeout time.Duration) bool {
	cli, err := CreateClinet()
	if err != nil {
		panic(err)
	}
	containers := List()
	healthChans := []<-chan bool{}
	for _, container := range containers {
		if !strings.HasPrefix(container.Name, fmt.Sprintf("/%s", namePrefix)) {
			continue
		}
		healthChans = append(healthChans, checkHealth(cli, container.ID, timeout))
	}
	for _, healthChan := range healthChans {
		if !<-healthChan {
			return false
		}
	}
	return true
}

func checkHealth(cli *client.Client, containerID string, timeout time.Duration) <-chan bool {
	healthChan := make(chan bool)
	cli, err := CreateClinet()
	if err != nil {
		panic(err)
	}
	steps := 200 * time.Millisecond
	go func() {
		defer close(healthChan)
		for count := int64(0); count < int64(timeout)/int64(steps); count++ {
			inspectData, err := cli.ContainerInspect(context.Background(), containerID)
			if err != nil {
				logger.Error("Error inspecting container", util.WrapError(err))
				healthChan <- false
				return
			}
			if inspectData.State.Health.Status == "healthy" {
				healthChan <- true
				return
			}
			time.Sleep(steps)
		}
		healthChan <- false
	}()
	return healthChan
}

type DockerConfig struct {
	Auths map[string]registry.AuthConfig `json:"auths"`
}

func GetAuthString(image string) string {
	// Read Docker config
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}
	dockerConfigPath := usr.HomeDir + "/.docker/config.json"
	file, err := os.ReadFile(dockerConfigPath)
	if err != nil {
		return ""
	}
	var dockerConfig DockerConfig
	err = json.Unmarshal(file, &dockerConfig)
	if err != nil {
		return ""
	}

	firstSlash := strings.Index(image, "/")
	authConfigKey := "https://index.docker.io/v1/"
	if firstSlash != -1 {
		authConfigKey = image[:firstSlash]
	}
	authConfig, exists := dockerConfig.Auths[authConfigKey]
	if !exists {
		return ""
	}

	// Pull image
	encodedJSON, err := json.Marshal(authConfig)
	if err != nil {
		panic(err)
	}
	authStr := string(encodedJSON)
	return authStr
}

func Pull(ctx context.Context, image string) {
	if !strings.Contains(image, ":") {
		image += ":latest"
	}
	cli, err := CreateClinet()
	if err != nil {
		log.Fatal(err)
	}

	images, err := cli.ImageList(ctx, imagetypes.ListOptions{})
	if err != nil {
		panic(err)
	}

	for i := range images {
		img := &images[i]
		for _, tag := range img.RepoTags {
			if tag == image {
				return
			}
		}
	}

	authStr := GetAuthString(image)

	imgFullName := image
	if !strings.Contains(image, "/") {
		imgFullName = "docker.io/library/" + image
	}

	reader, err := cli.ImagePull(ctx, imgFullName, imagetypes.PullOptions{RegistryAuth: authStr})
	if err != nil {
		panic(err)
	}
	defer reader.Close()
}

type NetworkConfig struct {
	Name string
}

func NewNetworkConfig(name string) *NetworkConfig {
	return &NetworkConfig{
		Name: name,
	}
}

func (n *NetworkConfig) Ensure(ctx context.Context) error {
	cli, err := CreateClinet()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	if n.Name != HostNetworkName {
		_, err := cli.NetworkInspect(ctx, n.Name, networktypes.InspectOptions{})
		if err != nil {
			_, err = cli.NetworkCreate(ctx, n.Name, networktypes.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create network: %w", err)
			}
		}
	}
	return nil
}

// ContainerConfig holds configuration parameters for creating a container
type ContainerConfig struct {
	Name          string
	NetworkName   string
	MysqlImage    string
	HealthCommand []string
	Bindings      []string
	Caps          []string
	EnvVars       []string
	Cmd           []string
	Ports         map[int]int
}

// Make creates and starts a container based on the configuration
func (c *ContainerConfig) Make(ctx context.Context) error {
	cli, err := CreateClinet()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}

	exposedPorts := nat.PortSet{}
	for _, target := range c.Ports {
		exposedPorts[nat.Port(fmt.Sprint(target))] = struct{}{}
	}

	config := &containertypes.Config{
		Image:        c.MysqlImage,
		Env:          c.EnvVars,
		ExposedPorts: exposedPorts,
		Healthcheck: &containertypes.HealthConfig{
			Test:     c.HealthCommand,
			Timeout:  20 * time.Second,
			Retries:  20,
			Interval: 1 * time.Second,
		},
		Cmd: c.Cmd,
	}

	natBindings := map[nat.Port][]nat.PortBinding{}
	for exposed, target := range c.Ports {
		natBindings[nat.Port(fmt.Sprint(target))] = []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: fmt.Sprintf("%d", exposed),
			},
		}
	}

	hostConfig := &containertypes.HostConfig{
		Binds:  c.Bindings,
		CapAdd: c.Caps,
		RestartPolicy: containertypes.RestartPolicy{
			Name: containertypes.RestartPolicyAlways,
		},
	}

	if c.NetworkName != HostNetworkName {
		hostConfig.PortBindings = natBindings
	}

	if c.NetworkName != "" {
		hostConfig.NetworkMode = containertypes.NetworkMode(c.NetworkName)
		if c.NetworkName == HostNetworkName {
			hostConfig.NetworkMode = HostNetworkName
		}
	}

	Pull(ctx, c.MysqlImage)
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, c.Name)
	if err != nil {
		if errdefs.IsConflict(err) {
			logger.Warning("Container already exists: %s", util.M{"containerName": c.Name})
			return nil
		}
		return fmt.Errorf("failed to create container: %w", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, containertypes.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	ok := WaitHealthy(c.Name, defaultContainerStartTimeout)
	if !ok {
		return fmt.Errorf("container %s is not healthy", c.Name)
	}
	return nil
}

func MapContainerName(name, tenant string, repIndex, shardIndex int) string {
	if shardIndex == 0 {
		return fmt.Sprintf("%s-%s-%s", tenant, name, string('a'+rune(repIndex)))
	}
	return fmt.Sprintf("%s-%s-%s-%d", tenant, name, string('a'+rune(repIndex)), shardIndex)
}
