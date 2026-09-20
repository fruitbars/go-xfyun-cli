package media

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNormalizeFormat(t *testing.T) {
	for input, want := range map[string]string{".wav": "wav", "mp3": "mp3", ".m4a": "ipod", ".oga": "ogg"} {
		if got := normalizeFormat(input); got != want {
			t.Fatalf("normalizeFormat(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestProbeAndConvertWAV(t *testing.T) {
	ffmpeg, err := ResolveFFmpeg()
	if err != nil {
		t.Skip(err)
	}
	if _, err := ResolveFFprobe(); err != nil {
		t.Skip(err)
	}
	directory := t.TempDir()
	input := filepath.Join(directory, "input.wav")
	output := filepath.Join(directory, "output.wav")
	if err := runFFmpegTestTone(context.Background(), ffmpeg, input); err != nil {
		t.Skipf("ffmpeg lacks test source: %v", err)
	}
	info, err := Probe(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if info.SampleRate != 16000 || info.Channels != 2 {
		t.Fatalf("input info = %+v", info)
	}
	if err := Convert(context.Background(), input, output, ConvertOptions{Channels: 1, SampleRate: 8000, Format: ".wav"}); err != nil {
		t.Fatal(err)
	}
	converted, err := Probe(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	if converted.SampleRate != 8000 || converted.Channels != 1 {
		t.Fatalf("converted info = %+v", converted)
	}
}

func TestProbeFallsBackToFFmpeg(t *testing.T) {
	ffmpeg, err := ResolveFFmpeg()
	if err != nil {
		t.Skip(err)
	}
	directory := t.TempDir()
	input := filepath.Join(directory, "input.wav")
	if err := runFFmpegTestTone(context.Background(), ffmpeg, input); err != nil {
		t.Skipf("ffmpeg lacks test source: %v", err)
	}
	t.Setenv("XFYUN_FFPROBE_PATH", filepath.Join(directory, "missing-ffprobe"))
	info, err := Probe(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if info.SampleRate != 16000 || info.Channels != 2 || info.DurationMS == 0 {
		t.Fatalf("fallback info = %+v", info)
	}
}

func runFFmpegTestTone(ctx context.Context, ffmpeg, output string) error {
	// Kept in the test instead of committing binary fixtures to the repository.
	cmd := commandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=frequency=1000:duration=0.1", "-ac", "2", "-ar", "16000", output)
	return cmd.Run()
}

// Small indirection keeps the test easy to compile on all supported systems.
var commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}
