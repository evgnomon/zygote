package container

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/evgnomon/zygote/pkg/utils"
)

const (
	HostNetworkName              = "host"
	defaultContainerStartTimeout = 120 * time.Second
	defaultNetworkName           = "mynet"
	dockerVersion                = "1.41"
	dockerDirPermissions         = 0700
	networkNameEnvVar            = "DOCKER_NETWORK_NAME"
)

var logger = utils.NewLogger()

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
		logger.Fatal("Unsupported OS", utils.M{"os": runtime.GOOS})
	}
	cli, err := client.NewClientWithOpts(client.WithVersion(dockerVersion), client.WithHost(dockerPath))
	if err != nil {
		return nil, err
	}
	return cli, nil
}

func Spawn(ctx context.Context,
	cli *client.Client,
	image string,
	cmd []string,
	portMap map[string]string,
	networkName string,
) {
	config := &containertypes.Config{
		Image:        image,
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
	logger.FatalIfErr("Create container", err, utils.M{"containerName": image})

	// Attach to STDOUT before starting
	attachOptions := containertypes.AttachOptions{
		Stream: true,
		Stdout: true,
		Stderr: true,
	}

	attachResponse, err := cli.ContainerAttach(ctx, resp.ID, attachOptions)
	logger.FatalIfErr("Attach to container", err, utils.M{"containerID": resp.ID})

	defer attachResponse.Close()

	err = cli.ContainerStart(ctx, resp.ID, containertypes.StartOptions{})
	logger.FatalIfErr("Start container", err, utils.M{"containerID": resp.ID})

	_, err = io.Copy(os.Stdout, attachResponse.Reader)
	logger.FatalIfErr("Copy output", err, utils.M{"containerID": resp.ID})
}

func SpawnAndWait(ctx context.Context,
	cli *client.Client,
	imageName, tenant string,
	cmd []string,
	portMap map[string]string,
	volumeMap map[string]string,
	networkName string,
) error {
	config := &containertypes.Config{
		Image:        imageName,
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
		Binds:        volumeBinding(ctx, cli, volumeMap),
	}
	if networkName != "" {
		hostConfig.NetworkMode = containertypes.NetworkMode(networkName)
	}

	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, fmt.Sprintf("%s-%d", tenant, time.Now().UnixNano()))
	logger.FatalIfErr("Create container", err, utils.M{"containerName": imageName})

	attachOptions := containertypes.AttachOptions{
		Stream: true,
		Stdout: true,
		Stderr: true,
	}
	attachResponse, err := cli.ContainerAttach(ctx, resp.ID, attachOptions)
	logger.FatalIfErr("Attach to container", err, utils.M{"containerID": resp.ID})
	defer attachResponse.Close()

	Pull(ctx, imageName)

	err = cli.ContainerStart(ctx, resp.ID, containertypes.StartOptions{})
	logger.FatalIfErr("Start container", err, utils.M{"containerID": resp.ID})

	// Stream output to stdout
	go func() {
		data, err := io.ReadAll(attachResponse.Reader)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error copying output: %v\n", err)
		}
		if len(data) != 0 {
			logger.Debug("Container output", utils.M{"message": data})
		}
	}()

	// Wait for the container to finish and get its exit code
	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, containertypes.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		logger.FatalIfErr("Wait for container", err, utils.M{"containerID": resp.ID})
	case status := <-statusCh:
		if status.StatusCode != 0 {
			logger.Debug("Container exited with non-zero status", utils.M{"image": imageName, "cmd": cmd})
			return fmt.Errorf("container exited with non-zero status: %d", status.StatusCode)
		}
	}
	return nil
}

func volumeBinding(ctx context.Context, cli *client.Client, volumeMap map[string]string) []string {
	var binds []string
	for volumeName, mountPath := range volumeMap {
		_, err := cli.VolumeInspect(ctx, volumeName)
		if err != nil {
			_, err := cli.VolumeCreate(ctx, volume.CreateOptions{Name: volumeName})
			logger.FatalIfErr("Create volume", err, utils.M{"volumeName": volumeName})
		}
		bind := volumeName + ":" + mountPath
		binds = append(binds, bind)
	}

	return binds
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
	logger.FatalIfErr("Create Docker client", err)

	_, err = cli.NetworkInspect(ctx, networkName, networktypes.InspectOptions{})
	if err != nil {
		_, err = cli.NetworkCreate(ctx, networkName, networktypes.CreateOptions{})
	}
	logger.FatalIfErr("Create network", err, utils.M{"networkName": networkName})

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
		Binds:        volumeBinding(ctx, cli, volumeMap),
	}
	if networkName != "" {
		hostConfig.NetworkMode = containertypes.NetworkMode(networkName)
	}

	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, fmt.Sprintf("%s-%d", tenant, time.Now().UnixNano()))
	logger.FatalIfErr("Create container", err, utils.M{"containerName": name})

	// Attach to STDIN, STDOUT, and STDERR before starting
	attachOptions := containertypes.AttachOptions{
		Stream: true,
		Stdin:  inputStr != "", // Enable STDIN attachment
		Stdout: false,
		Stderr: false,
	}

	attachResponse, err := cli.ContainerAttach(ctx, resp.ID, attachOptions)
	logger.FatalIfErr("Attach to container", err, utils.M{"containerID": resp.ID})

	defer attachResponse.Close()

	var wg sync.WaitGroup
	if inputStr != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Write your input string to the container's STDIN
			inputWithNewline := inputStr + "\n"
			n, err := attachResponse.Conn.Write([]byte(inputWithNewline))
			logger.FatalIfErr("Write to container's STDIN", err, utils.M{"input": inputWithNewline, "name": name})
			if n != len(inputWithNewline) {
				logger.Fatal("Failed to write to container's STDIN", utils.M{"expected": len(inputWithNewline), "actual": n})
			}
			time.Sleep(1 * time.Second)
			attachResponse.Close()
		}()

		wg.Wait()
	}

	err = cli.ContainerStart(ctx, resp.ID, containertypes.StartOptions{})
	logger.FatalIfErr("Start container", err, utils.M{"containerID": resp.ID})
	statusCh, errCh := cli.ContainerWait(ctx, resp.ID, containertypes.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		logger.FatalIfErr("Wait for container", err, utils.M{"containerID": resp.ID})

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
	logger.FatalIfErr("Create Docker client", err)

	opt := containertypes.ListOptions{All: true}
	for _, o := range opts {
		o(&opt)
	}

	containers, err := cli.ContainerList(context.Background(), opt)
	logger.FatalIfErr("List containers", err)

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
	logger.FatalIfErr("Create Docker client", err)

	removeOptions := containertypes.RemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	}

	err = cli.ContainerStop(ctx, containerID, containertypes.StopOptions{Signal: "SIGTERM"})
	logger.FatalIfErr("Stop container", err, utils.M{"containerID": containerID})

	if err := cli.ContainerRemove(ctx, containerID, removeOptions); err != nil {
		logger.FatalIfErr("Remove container", err, utils.M{"containerID": containerID})
	}
}

func RemoveVolumePrefix(volumePrefix string) {
	cli, err := CreateClinet()
	logger.FatalIfErr("Create Docker client", err)

	volumes, err := cli.VolumeList(context.Background(), volume.ListOptions{})
	logger.FatalIfErr("List volumes", err, utils.M{"volumePrefix": volumePrefix})

	for _, volume := range volumes.Volumes {
		if strings.HasPrefix(volume.Name, volumePrefix) {
			err := cli.VolumeRemove(context.Background(), volume.Name, false)
			logger.FatalIfErr("Remove volume", err, utils.M{"volumeName": volume.Name})
		}
	}
}

func WaitHealthy(namePrefix string, timeout time.Duration) bool {
	cli, err := CreateClinet()
	logger.FatalIfErr("Create Docker client", err)

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
		logger.FatalIfErr("Create Docker client", err)
	}
	steps := 200 * time.Millisecond
	go func() {
		defer close(healthChan)
		for count := int64(0); count < int64(timeout)/int64(steps); count++ {
			inspectData, err := cli.ContainerInspect(context.Background(), containerID)
			if err != nil {
				logger.Error("Error inspecting container", err)
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
	logger.FatalIfErr("Get current user", err)

	dockerConfigPath := usr.HomeDir + "/.docker/config.json"
	// Create the file if not exists
	err = os.MkdirAll(usr.HomeDir+"/.docker", dockerDirPermissions)
	logger.FatalIfErr("Create .docker directory", err)
	if _, err := os.Stat(dockerConfigPath); os.IsNotExist(err) {
		_, err = os.Create(dockerConfigPath)
		logger.FatalIfErr("Create Docker config file", err)
	}
	file, err := os.ReadFile(dockerConfigPath)
	logger.FatalIfErr("Read Docker config file", err)
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
	logger.FatalIfErr("Marshal auth config", err)

	authStr := string(encodedJSON)
	return authStr
}

func Pull(ctx context.Context, image string) {
	if !strings.Contains(image, ":") {
		image += ":latest"
	}
	cli, err := CreateClinet()
	logger.FatalIfErr("Create Docker client", err)

	images, err := cli.ImageList(ctx, imagetypes.ListOptions{})
	logger.FatalIfErr("List images", err)

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
	logger.FatalIfErr("Image pull", err, utils.M{"image": image})
	logger.Debug("Pulling image", utils.M{"image": image})
	_, err = io.ReadAll(reader)
	if err != nil {
		logger.Fatal("Image pull failed", utils.M{"error": err})
	}

	err = reader.Close()
	logger.Warning("Close reader", utils.M{"image": image, "error": err})
}

type NetworkConfig struct {
	Name string
}

func NewNetworkConfig(imageName string) *NetworkConfig {
	return &NetworkConfig{
		Name: imageName,
	}
}

func (n *NetworkConfig) Ensure(ctx context.Context) {
	cli, err := CreateClinet()
	logger.FatalIfErr("Create Docker client", err, utils.M{"networkName": n.Name})
	if n.Name != HostNetworkName {
		_, err := cli.NetworkInspect(ctx, n.Name, networktypes.InspectOptions{})
		if err != nil {
			_, err = cli.NetworkCreate(ctx, n.Name, networktypes.CreateOptions{})
			logger.FatalIfErr("Create network", err, utils.M{"networkName": n.Name})
		}
	}
}

// ContainerConfig holds configuration parameters for creating a container
type ContainerConfig struct {
	Name          string
	NetworkName   string
	Image         string
	HealthCommand []string
	Bindings      []string
	Caps          []string
	EnvVars       []string
	Cmd           []string
	Ports         map[int]int
}

// StartContainer creates and starts a container based on the configuration
func (c *ContainerConfig) StartContainer(ctx context.Context) error {
	cli, err := CreateClinet()
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}

	exposedPorts := nat.PortSet{}
	for _, target := range c.Ports {
		exposedPorts[nat.Port(fmt.Sprint(target))] = struct{}{}
	}

	config := &containertypes.Config{
		Image:        c.Image,
		Env:          c.EnvVars,
		ExposedPorts: exposedPorts,
		Healthcheck: &containertypes.HealthConfig{
			Test:     c.HealthCommand,
			Timeout:  40 * time.Second,
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

	Pull(ctx, c.Image)
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, c.Name)
	if err != nil {
		if errdefs.IsConflict(err) {
			logger.Warning("Container already exists: %s", utils.M{"containerName": c.Name})
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
