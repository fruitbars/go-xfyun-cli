package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fruitbars/go-xfyun-cli/internal/ifasr"
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

func TestWriteSpeakerFilesWithTimestamps(t *testing.T) {
	directory := t.TempDir()
	speakers := []ifasr.SpeakerTranscript{{Speaker: "0", Track: "L", Transcript: "你好继续", Segments: []ifasr.SpeakerSegment{{StartMS: 1250, EndMS: 3421, Transcript: "你好"}, {StartMS: 4000, EndMS: 5000, Transcript: "继续"}}}}
	if err := writeSpeakerFiles(directory, "", speakers, true, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "speaker-0-L.txt"))
	if err != nil || !strings.Contains(string(data), "[00:00:01.250 --> 00:00:03.421] 你好") {
		t.Fatalf("speaker file = %q, err = %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(directory, "speakers.txt")); err != nil {
		t.Fatal(err)
	}
	if err := writeSpeakerFiles(directory, "", speakers, true, false); err == nil {
		t.Fatal("expected existing speaker output error")
	}
}

func TestIFASRURLRequiresRemoteMetadata(t *testing.T) {
	t.Setenv("XFYUN_APP_ID", "test")
	t.Setenv("XFYUN_API_KEY", "test")
	t.Setenv("XFYUN_API_SECRET", "test")
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"ifasr", "--audio-url", "https://media.example/meeting.wav"}, bytes.NewReader(nil), &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "--file-name") {
		t.Fatalf("error = %v", err)
	}
}
