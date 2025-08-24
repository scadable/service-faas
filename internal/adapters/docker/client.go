package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"service-faas/internal/config"
	"strconv"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image" // Added import
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/rs/zerolog"
)

type Client struct {
	cli        *client.Client
	lg         zerolog.Logger
	cfg        config.Config
	authHeader string
}

type RunResult struct {
	ContainerID string
	HostPort    int
}

func New(cfg config.Config, lg zerolog.Logger) (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	c := &Client{cli: cli, cfg: cfg, lg: lg.With().Str("adapter", "docker").Logger()}

	if cfg.HarborUser != "" && cfg.HarborPass != "" {
		authConfig := registry.AuthConfig{
			Username:      cfg.HarborUser,
			Password:      cfg.HarborPass,
			ServerAddress: cfg.HarborURL,
		}
		encodedJSON, err := json.Marshal(authConfig)
		if err != nil {
			return nil, fmt.Errorf("marshal auth config: %w", err)
		}
		c.authHeader = base64.URLEncoding.EncodeToString(encodedJSON)
		c.lg.Info().Str("registry", cfg.HarborURL).Msg("configured Harbor registry authentication")
	}

	return c, nil
}

// RunWorker starts a new FaaS worker container.
func (c *Client) RunWorker(ctx context.Context, funcID, codePath, handlerPath string) (*RunResult, error) {
	name := "faas-worker-" + funcID

	// Ensure the image exists locally
	if err := c.ensureImage(ctx, c.cfg.WorkerImage); err != nil {
		return nil, err
	}

	// Ensure any old container with the same name is gone
	// ✅ FIX: Use container.RemoveOptions
	_ = c.cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})

	resp, err := c.cli.ContainerCreate(ctx,
		&container.Config{
			Image: c.cfg.WorkerImage,
			Env: []string{
				"HANDLER_FUNCTION=" + handlerPath,
			},
			ExposedPorts: nat.PortSet{"8000/tcp": struct{}{}},
		},
		&container.HostConfig{
			// Mount the Python code directory into the container
			Binds: []string{fmt.Sprintf("%s:/app/function", codePath)},
			// Publish port 8000 to a random available port on the host
			PortBindings: nat.PortMap{
				"8000/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: ""}},
			},
		},
		nil, nil, name,
	)
	if err != nil {
		return nil, fmt.Errorf("docker create: %w", err)
	}

	// ✅ FIX: Use container.StartOptions
	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("docker start: %w", err)
	}

	// Inspect the container to get the dynamically assigned host port
	inspect, err := c.cli.ContainerInspect(ctx, resp.ID)
	if err != nil {
		return nil, fmt.Errorf("docker inspect: %w", err)
	}
	hostPortStr := inspect.NetworkSettings.Ports["8000/tcp"][0].HostPort
	hostPort, _ := strconv.Atoi(hostPortStr)

	c.lg.Info().
		Str("container_id", resp.ID).
		Str("function_id", funcID).
		Int("host_port", hostPort).
		Msg("worker container started")

	return &RunResult{ContainerID: resp.ID, HostPort: hostPort}, nil
}

// StopAndRemoveContainer stops and removes a container by its ID.
func (c *Client) StopAndRemoveContainer(ctx context.Context, containerID string) error {
	if containerID == "" {
		return nil // Nothing to do
	}
	c.lg.Info().Str("container_id", containerID).Msg("stopping and removing container")
	// ✅ FIX: Use container.RemoveOptions
	err := c.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
	if err != nil && !client.IsErrNotFound(err) {
		return err
	}
	return nil
}

func (c *Client) ensureImage(ctx context.Context, img string) error {
	_, _, err := c.cli.ImageInspectWithRaw(ctx, img)
	if err == nil {
		return nil // Image exists
	}
	if !client.IsErrNotFound(err) {
		return fmt.Errorf("image inspect: %w", err) // Another error occurred
	}

	c.lg.Info().Str("image", img).Msg("pulling image from registry")
	// ✅ FIX: Use image.PullOptions
	rc, err := c.cli.ImagePull(ctx, img, image.PullOptions{RegistryAuth: c.authHeader})
	if err != nil {
		return fmt.Errorf("image pull: %w", err)
	}
	defer rc.Close()
	// You can optionally stream the pull output to logs
	_, _ = io.Copy(io.Discard, rc)

	return nil
}
