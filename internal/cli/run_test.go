package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestTTSFailurePreservesForcedOutput(t *testing.T) {
	t.Setenv("XFYUN_APP_ID", "test")
	t.Setenv("XFYUN_API_KEY", "test")
	t.Setenv("XFYUN_API_SECRET", "test")
	directory := t.TempDir()
	output := filepath.Join(directory, "speech.mp3")
	if err := os.WriteFile(output, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"tts", "--text", "test", "--output", output, "--force", "--speed", "101"}, bytes.NewReader(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected invalid speed error")
	}
	data, err := os.ReadFile(output)
	if err != nil || string(data) != "original" {
		t.Fatalf("original output lost: %q %v", data, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary file leaked: %v %v", entries, err)
	}
}
