package ifasr

import (
	"context"
	"encoding/json"
	"io"
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

func TestExtractSpeakerTranscriptsGroupsRolesAndTracks(t *testing.T) {
	orderResult := `{"lattice":[{"json_1best":"{\"st\":{\"rl\":\"1\",\"rt\":[{\"ws\":[{\"cw\":[{\"w\":\"左声道\"}]}]}]}}"},{"json_1best":{"st":{"rl":"2","rt":[{"ws":[{"cw":[{"w":"右声道"}]}]}]}}},{"json_1best":"{\"st\":{\"rl\":\"1\",\"rt\":[{\"ws\":[{\"cw\":[{\"w\":\"继续\"}]}]}]}}"}],"label":{"rl_track":[{"rl":"1","track":"L"},{"rl":"2","track":"R"}]}}`
	speakers, err := ExtractSpeakerTranscripts(orderResult)
	if err != nil {
		t.Fatal(err)
	}
	if len(speakers) != 2 {
		t.Fatalf("speakers = %+v", speakers)
	}
	if speakers[0].Speaker != "1" || speakers[0].Track != "L" || speakers[0].Transcript != "左声道\n继续" {
		t.Fatalf("speaker 1 = %+v", speakers[0])
	}
	if speakers[1].Speaker != "2" || speakers[1].Track != "R" || speakers[1].Transcript != "右声道" {
		t.Fatalf("speaker 2 = %+v", speakers[1])
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
	orderResult := `{"lattice":[{"json_1best":"{\"st\":{\"rl\":\"1\",\"rt\":[{\"ws\":[{\"cw\":[{\"w\":\"处理后\"}]}]}]}}"}],"lattice2":[{"json_1best":{"st":{"rt":[{"ws":[{"cw":[{"w":"原始"}]}]}]}}}]}`
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
		if got := r.URL.Query().Get("accessKeyId"); got != "key" {
			t.Fatalf("accessKeyId = %q", got)
		}
		if got := r.URL.Query().Get("resultType"); got != "transfer" {
			t.Fatalf("resultType = %q", got)
		}
		if r.URL.Query().Get("dateTime") == "" {
			t.Fatal("dateTime is missing")
		}
		if r.URL.Query().Has("appId") {
			t.Fatal("getResult must not send undocumented appId")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != "{}" {
			t.Fatalf("body = %q, error = %v", body, err)
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
	if len(result.Speakers) != 1 || result.Speakers[0].Speaker != "1" || result.Speakers[0].Transcript != "处理后" {
		t.Fatalf("speakers = %+v", result.Speakers)
	}
}

func TestTranscribeURLSendsDocumentedURLParameters(t *testing.T) {
	smooth, colloquial := false, true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/upload" {
			t.Errorf("path = %q", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		if len(body) != 0 {
			t.Errorf("URL upload body has %d bytes", len(body))
		}
		query := r.URL.Query()
		want := map[string]string{
			"audioMode": "urlLink", "audioUrl": "https://media.example/meeting.wav?token=a+b",
			"fileName": "meeting.wav", "fileSize": "1234", "duration": "5678",
			"durationCheckDisable": "false", "language": "autodialect", "pd": "sport",
			"trackMode": "2", "eng_smoothproc": "false", "eng_colloqproc": "true",
		}
		for key, value := range want {
			if got := query.Get(key); got != value {
				t.Errorf("%s = %q, want %q", key, got, value)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"000000","descInfo":"success","content":{"orderId":"url-order"}}`))
	}))
	defer server.Close()

	client := Client{
		Credentials: config.Credentials{AppID: "app", APIKey: "key", APISecret: "secret"},
		Endpoint:    server.URL,
	}
	result, err := client.TranscribeURL(context.Background(), "https://media.example/meeting.wav?token=a+b", "meeting.wav", 1234, Options{
		DurationMS: 5678, Domain: "sport", TrackMode: 2,
		Smooth: &smooth, Colloquial: &colloquial, NoWait: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.OrderID != "url-order" || len(result.SignatureRand) != 16 {
		t.Fatalf("result = %+v", result)
	}
}

func TestUploadParamsMapsEverySupportedBusinessOption(t *testing.T) {
	smooth, colloquial, cantonese := true, true, 0
	client := Client{Credentials: config.Credentials{AppID: "app", APIKey: "key", APISecret: "secret"}}
	params := client.uploadParams("meeting.wav", 1234, "random", Options{
		Language: "autominor", DurationMS: 5678, Domain: "car", TrackMode: 1,
		CallbackURL: "https://callback.example/result", RoleType: 3, RoleNum: 2,
		FeatureIDs: "voice-1,voice-2", Smooth: &smooth, Colloquial: &colloquial,
		VADMode: 2, CantoneseScript: &cantonese, Analysis: true,
		Extra: map[string]string{
			"eng_max_clusters": "4", "eng_min_clusters": "1", "eng_dtd_thre": "2",
			"eng_control_spk": "1", "eng_combine_max": "3000",
		},
	})
	want := map[string]string{
		"appId": "app", "accessKeyId": "key", "signatureRandom": "random",
		"fileName": "meeting.wav", "fileSize": "1234", "duration": "5678",
		"durationCheckDisable": "false", "language": "autominor", "pd": "car",
		"trackMode": "1", "callbackUrl": "https://callback.example/result",
		"roleType": "3", "roleNum": "2", "featureIds": "voice-1,voice-2",
		"eng_smoothproc": "true", "eng_colloqproc": "true", "eng_vad_mdn": "2",
		"eng_rlang": "0", "analysis": "1",
	}
	for key, value := range want {
		if got := params[key]; got != value {
			t.Errorf("%s = %q, want %q", key, got, value)
		}
	}
	for key, value := range map[string]string{
		"eng_max_clusters": "4", "eng_min_clusters": "1", "eng_dtd_thre": "2",
		"eng_control_spk": "1", "eng_combine_max": "3000",
	} {
		if got := params[key]; got != value {
			t.Errorf("legacy %s = %q, want %q", key, got, value)
		}
	}
	if params["dateTime"] == "" {
		t.Fatal("dateTime was not generated")
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

func TestResponsePreservesReservedResultFields(t *testing.T) {
	input := `{"code":"000000","content":{"transResult":[{"segId":"1","dst":"translated"}],"predictResult":{"keywords":[{"word":"test"}]}}}`
	var response Response
	if err := json.Unmarshal([]byte(input), &response); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"transResult"`, `"predictResult"`, `"translated"`, `"keywords"`} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("encoded response is missing %s: %s", field, encoded)
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
	invalid = valid
	invalid.Language = "autodialect"
	if err := validateOptions(invalid); err == nil {
		t.Fatal("expected language analysis mode error")
	}
	invalid = Options{TrackMode: 2, RoleType: 1}
	if err := validateOptions(invalid); err == nil {
		t.Fatal("expected track and role conflict")
	}
	invalid = Options{TrackMode: 2, Analysis: true, Language: "autominor"}
	if err := validateOptions(invalid); err == nil {
		t.Fatal("expected track and analysis conflict")
	}
	invalid = Options{Domain: "unknown"}
	if err := validateOptions(invalid); err == nil {
		t.Fatal("expected unsupported domain error")
	}
	valid = Options{Extra: map[string]string{"eng_max_clusters": "2"}}
	if err := validateOptions(valid); err != nil {
		t.Fatalf("legacy parameter should be passed through: %v", err)
	}
	invalid = Options{RoleNum: 2}
	if err := validateOptions(invalid); err == nil {
		t.Fatal("expected role_num dependency error")
	}
}

func TestValidateEveryDocumentedDomain(t *testing.T) {
	for _, domain := range []string{
		"court", "finance", "medical", "tech", "sport", "edu", "isp", "gov",
		"game", "ecom", "mil", "com", "life", "ent", "culture", "car",
	} {
		if err := validateOptions(Options{Domain: domain}); err != nil {
			t.Errorf("domain %q: %v", domain, err)
		}
	}
}

func TestAcceptsEveryLegacyLFASRParameterForEnginePassthrough(t *testing.T) {
	for _, parameter := range []string{
		"eng_max_clusters", "eng_min_clusters", "eng_dtd_thre", "eng_control_spk", "eng_combine_max",
	} {
		if err := validateOptions(Options{Extra: map[string]string{parameter: "1"}}); err != nil {
			t.Errorf("parameter %q should pass through: %v", parameter, err)
		}
	}
}

func TestRejectsManagedParametersInExtra(t *testing.T) {
	for _, parameter := range []string{"appId", "audioMode", "audioUrl", "pd", "trackMode", "analysis"} {
		err := validateOptions(Options{Extra: map[string]string{parameter: "value"}})
		if err == nil || !strings.Contains(err.Error(), "first-class option") {
			t.Errorf("parameter %q error = %v", parameter, err)
		}
	}
}

func TestValidateAudioURL(t *testing.T) {
	if err := validateAudioURL("https://media.example/meeting.wav?token=abc"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "ftp://media.example/meeting.wav", "https:///meeting.wav"} {
		if err := validateAudioURL(value); err == nil {
			t.Errorf("validateAudioURL(%q) succeeded", value)
		}
	}
	tooLong := "https://media.example/" + strings.Repeat("a", 512)
	if err := validateAudioURL(tooLong); err == nil {
		t.Fatal("expected long audio URL error")
	}
}
