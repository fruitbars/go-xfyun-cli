package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/fruitbars/go-xfyun-cli/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerAdvertisesExpectedTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	server := New(config.Credentials{})
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.Run(ctx, serverTransport) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	want := []string{"xfyun_ifasr_result", "xfyun_ifasr_submit", "xfyun_media", "xfyun_ocr", "xfyun_rtasr", "xfyun_tts"}
	for i := range want {
		if i >= len(names) || names[i] != want[i] {
			t.Fatalf("tool names = %v, want %v", names, want)
		}
	}
	if len(names) != len(want) {
		t.Fatalf("tool names = %v, want %v", names, want)
	}
	for _, tool := range tools.Tools {
		schema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		if tool.Name == "xfyun_ifasr_result" && !strings.Contains(string(schema), `"orders"`) {
			t.Fatalf("IFASR result schema does not expose split orders: %s", schema)
		}
		if tool.Name == "xfyun_ifasr_result" && !strings.Contains(string(schema), `"task_file_path"`) {
			t.Fatalf("IFASR result schema does not expose task_file_path: %s", schema)
		}
		if tool.Name == "xfyun_ifasr_submit" {
			for _, field := range []string{`"variant"`, `"audio_url"`, `"file_name"`, `"file_size_bytes"`, `"track_mode"`, `"hot_word"`, `"language_type"`} {
				if !strings.Contains(string(schema), field) {
					t.Fatalf("IFASR submit schema does not expose %s: %s", field, schema)
				}
			}
		}
		if tool.Name == "xfyun_ifasr_result" {
			outputSchema, err := json.Marshal(tool.OutputSchema)
			if err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{`"expire_time"`, `"task_estimate_time_ms"`, `"raw_response"`, `"speakers"`, `"segments"`} {
				if !strings.Contains(string(outputSchema), field) {
					t.Fatalf("IFASR result output schema does not expose %s: %s", field, outputSchema)
				}
			}
		}
		if tool.Name == "xfyun_ocr" || tool.Name == "xfyun_tts" || tool.Name == "xfyun_rtasr" {
			outputSchema, err := json.Marshal(tool.OutputSchema)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(outputSchema), `"diagnostics"`) {
				t.Fatalf("%s output schema does not expose diagnostics: %s", tool.Name, outputSchema)
			}
		}
		if tool.Name == "xfyun_media" {
			for _, field := range []string{`"operation"`, `"input_path"`, `"output_path"`, `"sample_rate"`, `"channels"`, `"bitrate"`} {
				if !strings.Contains(string(schema), field) {
					t.Fatalf("media schema does not expose %s: %s", field, schema)
				}
			}
		}
		if tool.Name == "xfyun_ocr" {
			for _, field := range []string{`"include_raw"`, `"annotate"`, `"annotation_types"`, `"annotation_output_path"`} {
				if !strings.Contains(string(schema), field) {
					t.Fatalf("OCR schema does not expose %s: %s", field, schema)
				}
			}
		}
	}

	invalid, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "xfyun_ocr", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if !invalid.IsError {
		t.Fatal("invalid tool input should return a tool error")
	}
	invalidType, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "xfyun_ocr", Arguments: map[string]any{
		"input_path": "/does/not/matter.png", "annotate": true, "annotation_types": "not_a_layout_type",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !invalidType.IsError || !strings.Contains(invalidType.Content[0].(*mcp.TextContent).Text, "unsupported OCR annotation type") {
		t.Fatalf("invalid annotation type result = %+v", invalidType)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-serverErrors:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop")
	}
}

func TestSaveOCRAnnotationAcceptsExistingDottedDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "annotations.with-dot")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path, err := saveOCRAnnotation([]byte("png"), directory, 2, 3, false)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(directory, "page_002.png")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "png" {
		t.Fatalf("annotation data = %q", data)
	}
	if _, err := saveOCRAnnotation([]byte("new"), directory, 2, 3, false); err == nil {
		t.Fatal("expected existing annotation rejection")
	}
	if _, err := saveOCRAnnotation([]byte("new"), directory, 2, 3, true); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil || string(data) != "new" {
		t.Fatalf("forced annotation data = %q, err = %v", data, err)
	}
}

func TestPrepareAtomicOutputRejectsExistingFile(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "speech.mp3")
	if err := os.WriteFile(target, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepareAtomicOutput(target, false); err == nil {
		t.Fatal("expected existing-file error")
	}
	absolute, temporary, err := prepareAtomicOutput(target, true)
	if err != nil {
		t.Fatal(err)
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		t.Fatal(err)
	}
	if absolute != target {
		t.Fatalf("absolute path = %q, want %q", absolute, target)
	}
	if filepath.Dir(temporaryPath) != directory {
		t.Fatalf("temporary path %q is outside target directory", temporaryPath)
	}
	if err := os.Remove(temporaryPath); err != nil {
		t.Fatal(err)
	}
}

func TestResolveTextSupportsLiteralOrFileExclusively(t *testing.T) {
	if value, err := resolveText("直接文本", ""); err != nil || value != "直接文本" {
		t.Fatalf("literal text = %q, err = %v", value, err)
	}
	path := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(path, []byte("文件文本"), 0o600); err != nil {
		t.Fatal(err)
	}
	if value, err := resolveText("", path); err != nil || value != "文件文本" {
		t.Fatalf("file text = %q, err = %v", value, err)
	}
	if _, err := resolveText("", ""); err == nil {
		t.Fatal("expected missing text source error")
	}
	if _, err := resolveText("直接文本", path); err == nil {
		t.Fatal("expected mutually exclusive text source error")
	}
}

func TestAggregateIFASRStatusUsesMostActionableState(t *testing.T) {
	status := 4
	for _, next := range []int{0, 4, 3, 4} {
		status = aggregateIFASRStatus(status, next)
	}
	if status != 3 {
		t.Fatalf("status = %d, want processing", status)
	}
	if got := aggregateIFASRStatus(status, -1); got != -1 {
		t.Fatalf("failed status = %d", got)
	}
}
