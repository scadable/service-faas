package functions

import "time"

// Function represents a single FaaS function instance.
type Function struct {
	ID            string    `gorm:"primaryKey" json:"id"`
	FunctionName  string    `json:"function_name"` // The name of the function in the .py file
	HandlerPath   string    `json:"handler_path"`  // e.g., handler.handle
	CodePath      string    `json:"-"`             // Host path to the .py file
	ContainerID   string    `json:"container_id"`
	ContainerName string    `json:"container_name"`
	HostPort      int       `json:"host_port"` // The port on the host mapped to the container
	Status        string    `json:"status"`    // e.g., "creating", "running", "stopped", "error"
	CreatedAt     time.Time `json:"created_at"`
}
