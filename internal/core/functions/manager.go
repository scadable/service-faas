package functions

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"service-faas/internal/config"
	"service-faas/pkg/rand"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

type Manager struct {
	db           *gorm.DB
	orchestrator Orchestrator
	cfg          config.Config
	lg           zerolog.Logger
}

func NewManager(db *gorm.DB, orch Orchestrator, cfg config.Config, lg zerolog.Logger) *Manager {
	return &Manager{
		db:           db,
		orchestrator: orch,
		cfg:          cfg,
		lg:           lg.With().Str("component", "function-manager").Logger(),
	}
}

func (m *Manager) AddFunction(ctx context.Context, functionName string, code io.Reader) (*Function, error) {
	funcID := rand.ID16()
	codeDir := filepath.Join(m.cfg.FunctionStorageDir, funcID)
	if err := os.MkdirAll(codeDir, 0755); err != nil {
		return nil, fmt.Errorf("create function dir: %w", err)
	}

	codeFilePath := filepath.Join(codeDir, "handler.py")
	file, err := os.Create(codeFilePath)
	if err != nil {
		return nil, fmt.Errorf("create handler file: %w", err)
	}
	defer file.Close()
	if _, err := io.Copy(file, code); err != nil {
		return nil, fmt.Errorf("save handler code: %w", err)
	}

	fn := &Function{
		ID:            funcID,
		FunctionName:  functionName,
		HandlerPath:   fmt.Sprintf("function.handler.%s", functionName),
		CodePath:      codeDir,
		ContainerName: "faas-worker-" + funcID,
		Status:        "creating",
		CreatedAt:     time.Now().UTC(),
	}

	if err := m.db.Create(fn).Error; err != nil {
		return nil, fmt.Errorf("db create function record: %w", err)
	}

	runResult, err := m.orchestrator.RunWorker(ctx, fn.ID, fn.CodePath, fn.HandlerPath)
	if err != nil {
		m.lg.Error().Err(err).Str("function_id", fn.ID).Msg("failed to start container, rolling back")
		fn.Status = "error"
		m.db.Save(fn)
		return nil, fmt.Errorf("start worker container: %w", err)
	}

	fn.ContainerID = runResult.ContainerID
	fn.HostPort = runResult.HostPort
	fn.Status = "running"
	if err := m.db.Save(fn).Error; err != nil {
		m.lg.Error().Err(err).Str("function_id", fn.ID).Msg("failed to save container details to db")
		_ = m.orchestrator.StopAndRemoveContainer(ctx, fn.ContainerID)
		return nil, err
	}

	return fn, nil
}

func (m *Manager) ExecuteFunction(ctx context.Context, functionID, payload string) (json.RawMessage, error) {
	var fn Function
	if err := m.db.First(&fn, "id = ?", functionID).Error; err != nil {
		return nil, fmt.Errorf("function '%s' not found", functionID)
	}

	if fn.Status != "running" || fn.HostPort == 0 {
		return nil, fmt.Errorf("function '%s' is not in a running state", functionID)
	}

	workerURL := fmt.Sprintf("http://localhost:%d", fn.HostPort)
	reqBody := fmt.Sprintf(`{"payload": %q}`, payload)

	req, err := http.NewRequestWithContext(ctx, "POST", workerURL, strings.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request to worker: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read worker response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("worker returned non-200 status: %s - %s", resp.Status, string(bodyBytes))
	}

	var result struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("unmarshal worker response: %w", err)
	}

	return result.Result, nil
}

func (m *Manager) ListFunctions() ([]Function, error) {
	var functions []Function
	if err := m.db.Find(&functions).Error; err != nil {
		return nil, err
	}
	return functions, nil
}

func (m *Manager) RemoveFunction(ctx context.Context, functionID string) error {
	var fn Function
	if err := m.db.First(&fn, "id = ?", functionID).Error; err != nil {
		return fmt.Errorf("function '%s' not found", functionID)
	}

	if err := m.orchestrator.StopAndRemoveContainer(ctx, fn.ContainerID); err != nil {
		m.lg.Warn().Err(err).Str("function_id", functionID).Msg("failed to stop container, proceeding with cleanup")
	}

	if err := os.RemoveAll(fn.CodePath); err != nil {
		m.lg.Error().Err(err).Str("path", fn.CodePath).Msg("failed to delete function code directory")
	}

	if err := m.db.Delete(&fn).Error; err != nil {
		return fmt.Errorf("failed to delete function record from db: %w", err)
	}

	m.lg.Info().Str("function_id", functionID).Msg("function removed successfully")
	return nil
}

func (m *Manager) RestartRunningFunctions(ctx context.Context) error {
	m.lg.Info().Msg("restarting any previously running functions...")
	var runningFunctions []Function
	if err := m.db.Where("status = ?", "running").Find(&runningFunctions).Error; err != nil {
		return fmt.Errorf("could not query running functions: %w", err)
	}

	for _, fn := range runningFunctions {
		m.lg.Info().Str("function_id", fn.ID).Msg("restarting function")
		runResult, err := m.orchestrator.RunWorker(ctx, fn.ID, fn.CodePath, fn.HandlerPath)
		if err != nil {
			m.lg.Error().Err(err).Str("function_id", fn.ID).Msg("failed to restart function container")
			fn.Status = "stopped"
		} else {
			fn.ContainerID = runResult.ContainerID
			fn.HostPort = runResult.HostPort
		}
		if err := m.db.Save(&fn).Error; err != nil {
			m.lg.Error().Err(err).Str("function_id", fn.ID).Msg("failed to update function record on restart")
		}
	}
	return nil
}

func (m *Manager) CleanupAllFunctions(ctx context.Context) error {
	m.lg.Info().Msg("cleaning up all function containers")
	functions, err := m.ListFunctions()
	if err != nil {
		return fmt.Errorf("could not list functions for cleanup: %w", err)
	}

	for _, fn := range functions {
		if fn.Status == "running" {
			if err := m.orchestrator.StopAndRemoveContainer(ctx, fn.ContainerID); err != nil {
				m.lg.Error().Err(err).Str("function_id", fn.ID).Msg("failed during cleanup")
			}
		}
	}
	return nil
}
