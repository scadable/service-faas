package config

import (
	"os"
	"strings"
)

// DeploymentEnvType defines the allowed deployment environments.
type DeploymentEnvType string

const (
	EnvDocker     DeploymentEnvType = "docker"
	EnvKubernetes DeploymentEnvType = "kubernetes"
)

// Config holds all the configuration for the application.
type Config struct {
	ListenAddr         string
	DatabaseDSN        string
	HarborURL          string
	HarborUser         string
	HarborPass         string
	WorkerImage        string
	FunctionStorageDir string // Directory on the host to store uploaded Python code
	DeploymentEnv      DeploymentEnvType
}

// MustLoad loads configuration from environment variables.
func MustLoad() Config {
	env := getenv("DEPLOYMENT_ENV", "docker")
	var deploymentEnv DeploymentEnvType
	switch strings.ToLower(env) {
	case "kubernetes":
		deploymentEnv = EnvKubernetes
	default:
		deploymentEnv = EnvDocker
	}

	return Config{
		ListenAddr:         getenv("LISTEN_ADDR", ":8080"),
		DatabaseDSN:        getenv("DATABASE_DSN", "postgres://user:password@localhost:5432/faasdb?sslmode=disable"),
		HarborURL:          getenv("HARBOR_URL", "harbor.yourdomain.com"),
		HarborUser:         getenv("HARBOR_USER", "admin"),
		HarborPass:         getenv("HARBOR_PASS", "Harbor12345"),
		WorkerImage:        getenv("WORKER_IMAGE", "harbor.yourdomain.com/library/worker-faas:latest"),
		FunctionStorageDir: getenv("FUNCTION_STORAGE_DIR", "/tmp/faas_functions"),
		DeploymentEnv:      deploymentEnv,
	}
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
