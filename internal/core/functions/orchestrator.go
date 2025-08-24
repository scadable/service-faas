package functions

import "context"

// Orchestrator defines the interface for running and managing FaaS workers.
type Orchestrator interface {
	RunWorker(ctx context.Context, funcID, codePath, handlerPath string) (*RunResult, error)
	StopAndRemoveContainer(ctx context.Context, containerID string) error
}

// RunResult holds the outcome of running a worker.
type RunResult struct {
	ContainerID string
	HostPort    int
}
