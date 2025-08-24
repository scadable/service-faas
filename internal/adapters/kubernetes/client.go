package kubernetes

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"service-faas/internal/config"
	"service-faas/internal/core/functions" // Import the functions package

	"github.com/rs/zerolog"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	faasNamespace = "scadable-faas"
	appName       = "faas-worker"
)

type Client struct {
	clientset *kubernetes.Clientset
	lg        zerolog.Logger
	cfg       config.Config
}

// ✅ FIX: The local RunResult struct is removed.

func New(cfg config.Config, lg zerolog.Logger) (*Client, error) {
	// ... (constructor remains the same)
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}
	return &Client{
		clientset: clientset,
		lg:        lg.With().Str("adapter", "kubernetes").Logger(),
		cfg:       cfg,
	}, nil
}

// ✅ FIX: The return type is changed to *functions.RunResult
func (c *Client) RunWorker(ctx context.Context, funcID, codePath, handlerPath string) (*functions.RunResult, error) {
	deploymentName := appName + "-" + funcID
	labels := map[string]string{
		"app":  appName,
		"func": funcID,
	}

	// Read the actual Python code from the file
	handlerFilePath := filepath.Join(codePath, "handler.py")
	handlerFile, err := os.Open(handlerFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open handler file: %w", err)
	}
	defer handlerFile.Close()
	
	handlerCode, err := io.ReadAll(handlerFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read handler file: %w", err)
	}

	// Create a ConfigMap to store the handler code
	configMap := &apiv1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "handler-code-" + funcID,
			Namespace: faasNamespace,
		},
		Data: map[string]string{
			"handler.py": string(handlerCode), // Store the actual Python code content
		},
	}
	_, err := c.clientset.CoreV1().ConfigMaps(faasNamespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create configmap: %w", err)
	}

	// Create Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: faasNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: apiv1.PodSpec{
					ServiceAccountName: "faas-manager-sa",
					ImagePullSecrets: []apiv1.LocalObjectReference{
						{Name: "harbor-registry-secret"},
					},
					Containers: []apiv1.Container{
						{
							Name:  appName,
							Image: c.cfg.WorkerImage,
							Env: []apiv1.EnvVar{
								{
									Name:  "HANDLER_FUNCTION",
									Value: handlerPath,
								},
							},
							Ports: []apiv1.ContainerPort{
								{
									ContainerPort: 8000,
								},
							},
							VolumeMounts: []apiv1.VolumeMount{
								{
									Name:      "handler-volume",
									MountPath: "/app/function",
								},
							},
						},
					},
					Volumes: []apiv1.Volume{
						{
							Name: "handler-volume",
							VolumeSource: apiv1.VolumeSource{
								ConfigMap: &apiv1.ConfigMapVolumeSource{
									LocalObjectReference: apiv1.LocalObjectReference{
										Name: "handler-code-" + funcID,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err = c.clientset.AppsV1().Deployments(faasNamespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	// Create Service
	service := &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-" + funcID,
			Namespace: faasNamespace,
		},
		Spec: apiv1.ServiceSpec{
			Selector: labels,
			Type:     apiv1.ServiceTypeNodePort,
			Ports: []apiv1.ServicePort{
				{
					Port:       80,
					TargetPort: intstr.FromInt(8000),
				},
			},
		},
	}

	createdService, err := c.clientset.CoreV1().Services(faasNamespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}

	c.lg.Info().Str("deployment", deploymentName).Msg("created kubernetes deployment and service")

	// ✅ FIX: Return a *functions.RunResult struct
	return &functions.RunResult{
		ContainerID: deploymentName,
		HostPort:    int(createdService.Spec.Ports[0].NodePort),
	}, nil
}

// ... (StopAndRemoveContainer and int32Ptr methods remain the same) ...
func (c *Client) StopAndRemoveContainer(ctx context.Context, containerID string) error {
	deploymentName := containerID
	serviceName := "service-" + containerID[len(appName)+1:]
	configMapName := "handler-code-" + containerID[len(appName)+1:]

	// Delete Deployment
	deletePolicy := metav1.DeletePropagationForeground
	if err := c.clientset.AppsV1().Deployments(faasNamespace).Delete(ctx, deploymentName, metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil && !errors.IsNotFound(err) {
		return err
	}

	// Delete Service
	if err := c.clientset.CoreV1().Services(faasNamespace).Delete(ctx, serviceName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
		return err
	}

	// Delete ConfigMap
	if err := c.clientset.CoreV1().ConfigMaps(faasNamespace).Delete(ctx, configMapName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
		return err
	}

	c.lg.Info().Str("deployment", deploymentName).Msg("deleted kubernetes resources")
	return nil
}

func int32Ptr(i int32) *int32 { return &i }
