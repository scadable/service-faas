package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"service-faas/internal/config"
	"service-faas/internal/core/functions" // Import the functions package
	"strconv"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
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

// ✅ FIX: The local RunResult struct is removed.

func New(cfg config.Config, lg zerolog.Logger) (*Client, error) {
	// ... (constructor remains the same)
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

// ✅ FIX: The return type is changed to *functions.RunResult
func (c *Client) RunWorker(ctx context.Context, funcID, codePath, handlerPath string) (*functions.RunResult, error) {
	name := "faas-worker-" + funcID

	if err := c.ensureImage(ctx, c.cfg.WorkerImage); err != nil {
		return nil, err
	}

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
			Binds: []string{fmt.Sprintf("%s:/app/function", codePath)},
			PortBindings: nat.PortMap{
				"8000/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: ""}},
			},
		},
		nil, nil, name,
	)
	if err != nil {
		return nil, fmt.Errorf("docker create: %w", err)
	}

	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("docker start: %w", err)
	}

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

	// ✅ FIX: Return a *functions.RunResult struct
	return &functions.RunResult{ContainerID: resp.ID, HostPort: hostPort}, nil
}

// ... (StopAndRemoveContainer and ensureImage methods remain the same)
func (c *Client) StopAndRemoveContainer(ctx context.Context, containerID string) error {
	if containerID == "" {
		return nil
	}
	c.lg.Info().Str("container_id", containerID).Msg("stopping and removing container")
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
		return nil
	}
	if !client.IsErrNotFound(err) {
		return fmt.Errorf("image inspect: %w", err)
	}

	c.lg.Info().Str("image", img).Msg("pulling image from registry")
	rc, err := c.cli.ImagePull(ctx, img, image.PullOptions{RegistryAuth: c.authHeader})
	if err != nil {
		return fmt.Errorf("image pull: %w", err)
	}
	defer rc.Close()
	_, _ = io.Copy(io.Discard, rc)

	return nil
}
