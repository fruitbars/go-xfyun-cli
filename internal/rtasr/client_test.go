package rtasr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/fruitbars/go-xfyun-cli/internal/config"
)

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestInputFailureUnblocksResponseReader(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if err := conn.WriteJSON(map[string]any{"action": "started", "code": "0"}); err != nil {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	failure := errors.New("broken input")
	client := Client{Credentials: config.Credentials{AppID: "test", APIKey: "test", APISecret: "test"}, Endpoint: "ws" + strings.TrimPrefix(server.URL, "http")}
	start := time.Now()
	_, err := client.Transcribe(ctx, failingReader{failure}, Options{}, nil)
	if !errors.Is(err, failure) {
		t.Fatalf("error=%v, want input failure", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("input failure did not promptly unblock response reader")
	}
}

func TestEventTextFinalSegment(t *testing.T) {
	message := []byte(`{"sid":"session-1","data":{"seg_id":0,"cn":{"st":{"rt":[{"ws":[{"cw":[{"w":"你好","wp":"n"}]},{"cw":[{"w":"。","wp":"p"}]}]}],"type":"0"}},"ls":true}}`)
	text, final, sid := eventText(message)
	if text != "你好。" || !final || sid != "session-1" {
		t.Fatalf("eventText = (%q, %v, %q)", text, final, sid)
	}
	if !eventIsFinal(message) {
		t.Fatal("expected final event")
	}
}

func TestEventTextIgnoresIntermediate(t *testing.T) {
	message := []byte(`{"data":{"cn":{"st":{"rt":[{"ws":[{"cw":[{"w":"临时"}]}]}],"type":"1"}},"ls":false}}`)
	text, final, _ := eventText(message)
	if text != "临时" || final {
		t.Fatalf("eventText = (%q, %v)", text, final)
	}
}

func TestValidateAdvancedOptions(t *testing.T) {
	keepPunctuation := false
	valid := Options{
		Language: "autominor", RecognizedLanguage: "cn,en",
		AudioEncoding: "pcm_s16le", SampleRate: 16000,
		ChunkSize: 1280, RoleType: 2, FeatureIDs: "voice-1",
		SpeakerMatch: true, KeepPunctuation: &keepPunctuation, VADMode: 2,
	}
	if err := validateOptions(valid); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.RoleType = 0
	if err := validateOptions(invalid); err == nil {
		t.Fatal("expected voiceprint dependency error")
	}
}
