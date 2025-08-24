package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// ... (DeploymentEnvType constants remain the same) ...
type DeploymentEnvType string

const (
	EnvDocker     DeploymentEnvType = "docker"
	EnvKubernetes DeploymentEnvType = "kubernetes"
)

// Config holds all the configuration for the application.
type Config struct {
	ListenAddr         string
	DatabaseDSN        string // We will construct this from other vars
	HarborURL          string
	HarborUser         string
	HarborPass         string
	WorkerImage        string
	FunctionStorageDir string
	DeploymentEnv      DeploymentEnvType
	DBUser             string
	DBPassword         string
	DBHost             string
	DBName             string
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

	// Load individual database components
	dbUser := getenv("POSTGRES_USER", "user")
	dbPassword := getenv("POSTGRES_PASSWORD", "password")
	dbHost := getenv("POSTGRES_HOST", "localhost")
	dbName := getenv("POSTGRES_DB", "faasdb")
	dbPort := getenv("POSTGRES_PORT", "5432")

	// Construct the DSN string with URL encoding for credentials
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		url.QueryEscape(dbUser), url.QueryEscape(dbPassword), dbHost, dbPort, dbName,
	)

	return Config{
		ListenAddr:         getenv("LISTEN_ADDR", ":8080"),
		DatabaseDSN:        dsn, // Use the constructed DSN
		HarborURL:          getenv("HARBOR_URL", "harbor.yourdomain.com"),
		HarborUser:         getenv("HARBOR_USER", "admin"),
		HarborPass:         getenv("HARBOR_PASS", "Harbor12345"),
		WorkerImage:        getenv("WORKER_IMAGE", "harbor.yourdomain.com/library/worker-faas:latest"),
		FunctionStorageDir: getenv("FUNCTION_STORAGE_DIR", "/tmp/faas_functions"),
		DeploymentEnv:      deploymentEnv,
		DBUser:             dbUser,
		DBPassword:         dbPassword,
		DBHost:             dbHost,
		DBName:             dbName,
	}
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
