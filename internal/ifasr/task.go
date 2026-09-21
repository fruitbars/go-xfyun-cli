package ifasr

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// TaskFile is a small, local, permission-restricted continuation record. It
// intentionally contains only the identifiers required by the result API and
// the non-sensitive request traces useful during diagnosis.
type TaskFile struct {
	Version   int            `json:"version"`
	CreatedAt string         `json:"created_at"`
	Variant   Variant        `json:"variant"`
	Parts     []TaskPart     `json:"parts"`
	Requests  []RequestTrace `json:"requests,omitempty"`
}

type TaskPart struct {
	Index           int    `json:"index"`
	OrderID         string `json:"order_id"`
	SignatureRandom string `json:"signature_random,omitempty"`
	SizeBytes       int64  `json:"size_bytes,omitempty"`
	DurationMS      int64  `json:"duration_ms,omitempty"`
}

func NewTaskFile(variant Variant, batch BatchResult) TaskFile {
	task := TaskFile{Version: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Variant: variant, Requests: batch.Requests}
	for _, part := range batch.Parts {
		task.Parts = append(task.Parts, TaskPart{Index: part.Index, OrderID: part.Result.OrderID, SignatureRandom: part.Result.SignatureRand, SizeBytes: part.Size, DurationMS: part.DurationMS})
	}
	return task
}

func LoadTask(path string) (TaskFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TaskFile{}, fmt.Errorf("read task file: %w", err)
	}
	var task TaskFile
	if err := json.Unmarshal(data, &task); err != nil {
		return TaskFile{}, fmt.Errorf("decode task file: %w", err)
	}
	if task.Version != 1 || task.Variant == "" || len(task.Parts) == 0 {
		return TaskFile{}, fmt.Errorf("invalid IFASR task file: expected version 1, variant, and at least one part")
	}
	for index, part := range task.Parts {
		if part.OrderID == "" {
			return TaskFile{}, fmt.Errorf("invalid IFASR task file: parts[%d] has no order_id", index)
		}
		if task.Variant == VariantLLM && part.SignatureRandom == "" {
			return TaskFile{}, fmt.Errorf("invalid IFASR task file: parts[%d] has no signature_random", index)
		}
	}
	return task, nil
}

// SaveTask writes with mode 0600 and refuses replacement unless force is true.
func SaveTask(path string, task TaskFile, force bool) error {
	if path == "" {
		return fmt.Errorf("task file path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve task file path: %w", err)
	}
	if !force {
		if _, err := os.Stat(abs); err == nil {
			return fmt.Errorf("task file already exists: %s", abs)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect task file: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return fmt.Errorf("create task file directory: %w", err)
	}
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return fmt.Errorf("encode task file: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(abs), ".xfyun-task-*")
	if err != nil {
		return fmt.Errorf("create temporary task file: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("protect task file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write task file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close task file: %w", err)
	}
	if force {
		_ = os.Remove(abs)
	}
	if err := os.Rename(temporaryPath, abs); err != nil {
		return fmt.Errorf("commit task file: %w", err)
	}
	committed = true
	return nil
}
