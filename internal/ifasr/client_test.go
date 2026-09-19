package ifasr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fruitbars/go-xfyun-cli/internal/config"
)

func TestExtractTranscript(t *testing.T) {
	orderResult := `{"lattice":[{"json_1best":"{\"st\":{\"rt\":[{\"ws\":[{\"cw\":[{\"w\":\"你好\"}]},{\"cw\":[{\"w\":\"，\"}]}]}]}}"},{"json_1best":"{\"st\":{\"rt\":[{\"ws\":[{\"cw\":[{\"w\":\"世界\"},{\"w\":\"视界\"}]},{\"cw\":[{\"w\":\"！\"}]}]}]}}"}]}`
	got, err := ExtractTranscript(orderResult)
	if err != nil {
		t.Fatal(err)
	}
	if want := "你好，世界！"; got != want {
		t.Fatalf("transcript = %q, want %q", got, want)
	}
}

func TestExtractTranscriptsIncludesOriginalLattice(t *testing.T) {
	orderResult := `{"lattice":[{"json_1best":"{\"st\":{\"rt\":[{\"ws\":[{\"cw\":[{\"w\":\"处理后\"}]}]}]}}"}],"lattice2":[{"json_1best":{"st":{"rt":[{"ws":[{"cw":[{"w":"原始"}]}]}]}}}]}`
	processed, original, err := ExtractTranscripts(orderResult)
	if err != nil {
		t.Fatal(err)
	}
	if processed != "处理后" || original != "原始" {
		t.Fatalf("processed=%q original=%q", processed, original)
	}
}

func TestExtractTranscriptsAcceptsObjectAndStringSegments(t *testing.T) {
	orderResult := `{"lattice":[{"json_1best":{"st":{"rt":[{"ws":[{"cw":[{"w":"对象"}]}]}]}}},{"json_1best":"{\"st\":{\"rt\":[{\"ws\":[{\"cw\":[{\"w\":\"字符串\"}]}]}]}}"}]}`
	processed, original, err := ExtractTranscripts(orderResult)
	if err != nil {
		t.Fatal(err)
	}
	if processed != "对象字符串" || original != "" {
		t.Fatalf("processed=%q original=%q", processed, original)
	}
}

func TestExtractTranscriptsSkipsMissingAndNullSegments(t *testing.T) {
	orderResult := `{"lattice":[{}, {"json_1best":null}, {"json_1best":{"st":{"rt":[{"ws":[{"cw":[]},{"cw":[{"w":"保留"}]}]}]}}}]}`
	processed, original, err := ExtractTranscripts(orderResult)
	if err != nil {
		t.Fatal(err)
	}
	if processed != "保留" || original != "" {
		t.Fatalf("processed=%q original=%q", processed, original)
	}
}

func TestExtractTranscriptsRejectsMalformedSegments(t *testing.T) {
	for _, orderResult := range []string{
		`{"lattice":[{"json_1best":"{not-json"}]}`,
		`{"lattice":[{"json_1best":42}]}`,
	} {
		if _, _, err := ExtractTranscripts(orderResult); err == nil || !strings.Contains(err.Error(), "decode IFASR segment") {
			t.Fatalf("ExtractTranscripts(%q) error = %v", orderResult, err)
		}
	}
}

func TestWaitParsesCompletedResponseWithObjectLattice(t *testing.T) {
	orderResult := `{"lattice":[{"json_1best":"{\"st\":{\"rt\":[{\"ws\":[{\"cw\":[{\"w\":\"处理后\"}]}]}]}}"}],"lattice2":[{"json_1best":{"st":{"rt":[{"ws":[{"cw":[{"w":"原始"}]}]}]}}}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/getResult" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("orderId"); got != "test-order" {
			t.Fatalf("orderId = %q", got)
		}
		if got := r.URL.Query().Get("signatureRandom"); got != "test-random" {
			t.Fatalf("signatureRandom = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(Response{
			Code: "000000",
			Content: Content{
				OrderInfo:   OrderInfo{OrderID: "test-order", Status: 4, OriginalDuration: 1234},
				OrderResult: orderResult,
			},
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	client := Client{
		Credentials: config.Credentials{AppID: "app", APIKey: "key", APISecret: "secret"},
		Endpoint:    server.URL,
	}
	result, err := client.Wait(context.Background(), "test-order", "test-random", Options{MaxWait: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != 4 || result.Transcript != "处理后" || result.OriginalTranscript != "原始" {
		t.Fatalf("result = %+v", result)
	}
}

func TestCodeAcceptsStringAndNumber(t *testing.T) {
	for _, input := range []string{`{"code":"000000"}`, `{"code":0}`} {
		var response Response
		if err := json.Unmarshal([]byte(input), &response); err != nil {
			t.Fatal(err)
		}
		if response.Code != "000000" && response.Code != "0" {
			t.Fatalf("unexpected code %q", response.Code)
		}
	}
}

func TestValidateAdvancedOptions(t *testing.T) {
	smooth := true
	colloquial := false
	script := 0
	valid := Options{
		Language: "autominor", RoleType: 3, RoleNum: 2,
		FeatureIDs: "voice-1,voice-2", Smooth: &smooth, Colloquial: &colloquial,
		VADMode: 2, CantoneseScript: &script, Analysis: true,
	}
	if err := validateOptions(valid); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.RoleType = 1
	if err := validateOptions(invalid); err == nil {
		t.Fatal("expected voiceprint dependency error")
	}
	invalid = valid
	invalid.DurationMS = 5*60*60*1000 + 1
	if err := validateOptions(invalid); err == nil {
		t.Fatal("expected duration limit error")
	}
	invalid = valid
	invalid.CallbackURL = "file:///tmp/result"
	if err := validateOptions(invalid); err == nil {
		t.Fatal("expected callback URL error")
	}
	invalid = valid
	invalid.ResultType = "raw"
	if err := validateOptions(invalid); err == nil {
		t.Fatal("expected result type error")
	}
}
