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
	want := []string{"xfyun_ifasr_result", "xfyun_ifasr_submit", "xfyun_ocr", "xfyun_rtasr", "xfyun_tts"}
	for i := range want {
		if i >= len(names) || names[i] != want[i] {
			t.Fatalf("tool names = %v, want %v", names, want)
		}
	}
	if len(names) != len(want) {
		t.Fatalf("tool names = %v, want %v", names, want)
	}
	for _, tool := range tools.Tools {
		if tool.Name != "xfyun_ifasr_result" {
			continue
		}
		schema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(schema), `"orders"`) {
			t.Fatalf("IFASR result schema does not expose split orders: %s", schema)
		}
	}

	invalid, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "xfyun_ocr", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if !invalid.IsError {
		t.Fatal("invalid tool input should return a tool error")
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
