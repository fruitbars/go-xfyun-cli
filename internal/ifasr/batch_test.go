package ifasr

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/fruitbars/go-xfyun-cli/internal/config"
)

func TestNeedsSplitAtEitherLimit(t *testing.T) {
	if needsSplit(MaxAudioBytes, MaxAudioDurationMS) {
		t.Fatal("exact limits should be accepted")
	}
	if !needsSplit(MaxAudioBytes+1, 0) {
		t.Fatal("oversized file should be split")
	}
	if !needsSplit(1, MaxAudioDurationMS+1) {
		t.Fatal("overlong file should be split")
	}
}

func TestSegmentDurationAccountsForSizeAndDuration(t *testing.T) {
	if got := segmentDuration(100, 10*60*60*1000); got != targetPartMS {
		t.Fatalf("duration-limited segment = %d, want %d", got, targetPartMS)
	}
	got := segmentDuration(800*1024*1024, 4*60*60*1000)
	if got != 2*60*60*1000 {
		t.Fatalf("size-limited segment = %d, want %d", got, 2*60*60*1000)
	}
}

func TestParseFFmpegDuration(t *testing.T) {
	got, err := parseDuration([]byte("Duration: 05:12:03.45, start: 0.000000, bitrate: 128 kb/s"))
	if err != nil {
		t.Fatal(err)
	}
	want := int64((5*60*60+12*60+3)*1000 + 450)
	if got != want {
		t.Fatalf("duration = %d, want %d", got, want)
	}
}

func TestTranscribeFileClosesInputExactlyOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/upload" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":"000000","descInfo":"success","content":{"orderId":"test-order"}}`)
	}))
	defer server.Close()

	input := filepath.Join(t.TempDir(), "input.wav")
	if err := os.WriteFile(input, []byte("test audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := Client{
		Credentials: config.Credentials{AppID: "app", APIKey: "key", APISecret: "secret"},
		Endpoint:    server.URL,
	}
	result, err := client.TranscribeFile(context.Background(), input, Options{DurationMS: 1000, NoWait: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Parts) != 1 || result.Parts[0].Result.OrderID != "test-order" {
		t.Fatalf("result = %+v", result)
	}
}

func TestFFmpegSplitsIntoValidAudioParts(t *testing.T) {
	ffmpeg, err := resolveFFmpeg()
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	formats := map[string][]string{
		"wav":   {"-c:a", "pcm_s16le"},
		"mp3":   {"-c:a", "libmp3lame"},
		"flac":  {"-c:a", "flac"},
		"ogg":   {"-c:a", "libopus", "-f", "ogg"},
		"opus":  {"-c:a", "libopus", "-f", "opus"},
		"speex": {"-c:a", "libspeex", "-f", "ogg"},
		"pcm":   {"-f", "s16le", "-ar", "16000", "-ac", "1"},
	}
	for extension, outputArgs := range formats {
		t.Run(extension, func(t *testing.T) {
			directory := t.TempDir()
			input := filepath.Join(directory, "input."+extension)
			args := []string{
				"-nostdin", "-hide_banner", "-loglevel", "error", "-y",
				"-f", "lavfi", "-i", "sine=frequency=440:duration=3",
			}
			args = append(args, outputArgs...)
			args = append(args, input)
			if output, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
				t.Skipf("ffmpeg lacks %s test encoder: %v: %s", extension, err, output)
			}
			partsDirectory := filepath.Join(directory, "parts")
			if err := os.MkdirAll(partsDirectory, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := splitAudio(context.Background(), ffmpeg, input, partsDirectory, 1000); err != nil {
				t.Fatal(err)
			}
			parts, err := collectAudioParts(context.Background(), ffmpeg, partsDirectory, "."+extension)
			if err != nil {
				t.Fatal(err)
			}
			if len(parts) < 2 {
				t.Fatalf("parts = %d, want at least 2", len(parts))
			}
			for _, part := range parts {
				if part.size == 0 || part.durationMS == 0 {
					t.Fatalf("invalid part: %#v", part)
				}
			}
		})
	}
}
