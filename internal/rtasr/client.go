package rtasr

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/fruitbars/go-xfyun-cli/internal/auth"
	"github.com/fruitbars/go-xfyun-cli/internal/config"
	"github.com/fruitbars/go-xfyun-cli/internal/xfyun"
)

const Endpoint = "wss://office-api-ast-dx.iflyaisol.com/ast/communicate/v1"

type Options struct {
	Language           string
	RecognizedLanguage string
	AudioEncoding      string
	SampleRate         int
	ChunkSize          int
	Interval           time.Duration
	RoleType           int
	FeatureIDs         string
	Domain             string
	SpeakerMatch       bool
	KeepPunctuation    *bool
	VADMode            int
	Extra              map[string]string
}

type Result struct {
	Transcript string `json:"transcript"`
	SID        string `json:"sid,omitempty"`
	Segments   int    `json:"segments"`
}

type Client struct {
	Credentials config.Credentials
	Dialer      *websocket.Dialer
	Endpoint    string
	Now         func() time.Time
}

func (c *Client) Transcribe(ctx context.Context, input io.Reader, opts Options, onEvent func(json.RawMessage) error) (Result, error) {
	var result Result
	if err := c.Credentials.ValidateSigned(); err != nil {
		return result, err
	}
	if opts.Language == "" {
		opts.Language = "autodialect"
	}
	if opts.AudioEncoding == "" {
		opts.AudioEncoding = "pcm_s16le"
	}
	if opts.SampleRate == 0 {
		opts.SampleRate = 16000
	}
	if opts.ChunkSize == 0 {
		opts.ChunkSize = 1280
	}
	if opts.Interval == 0 {
		opts.Interval = 40 * time.Millisecond
	}
	if err := validateOptions(opts); err != nil {
		return result, err
	}

	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	requestID, err := randomUUID()
	if err != nil {
		return result, err
	}
	params := make(map[string]string, len(opts.Extra)+12)
	for key, value := range opts.Extra {
		params[key] = value
	}
	params["appId"] = c.Credentials.AppID
	params["accessKeyId"] = c.Credentials.APIKey
	params["uuid"] = strings.ReplaceAll(requestID, "-", "")
	params["utc"] = now.Format("2006-01-02T15:04:05-0700")
	params["lang"] = opts.Language
	params["audio_encode"] = opts.AudioEncoding
	if opts.AudioEncoding == "pcm_s16le" {
		params["samplerate"] = strconv.Itoa(opts.SampleRate)
	}
	if opts.RecognizedLanguage != "" {
		params["recognized_language"] = opts.RecognizedLanguage
	}
	if opts.RoleType != 0 {
		params["role_type"] = strconv.Itoa(opts.RoleType)
	}
	if opts.FeatureIDs != "" {
		params["feature_ids"] = opts.FeatureIDs
	}
	if opts.Domain != "" {
		params["pd"] = opts.Domain
	}
	if opts.SpeakerMatch {
		params["eng_spk_match"] = "1"
	}
	if opts.KeepPunctuation != nil && !*opts.KeepPunctuation {
		params["eng_punc"] = "0"
	}
	if opts.VADMode != 0 {
		params["eng_vad_mdn"] = strconv.Itoa(opts.VADMode)
	}
	params["signature"] = auth.HMACSHA1(params, c.Credentials.APISecret)
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = Endpoint
	}
	requestURL := endpoint + "?" + auth.Query(params)
	dialer := c.Dialer
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}
	conn, httpResp, err := dialer.DialContext(ctx, requestURL, nil)
	if err != nil {
		if httpResp != nil {
			return result, fmt.Errorf("RTASR websocket handshake failed: %s", httpResp.Status)
		}
		return result, fmt.Errorf("RTASR websocket connection: %w", err)
	}
	defer conn.Close()
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()

	// The service sends a started frame before accepting audio.
	_, first, err := conn.ReadMessage()
	if err != nil {
		return result, fmt.Errorf("read RTASR handshake: %w", err)
	}
	if err := handleEvent(first, &result, onEvent); err != nil {
		return result, err
	}
	if apiErr := eventError(first); apiErr != nil {
		return result, apiErr
	}

	writeErr := make(chan error, 1)
	writerCtx, cancelWriter := context.WithCancel(ctx)
	defer cancelWriter()
	finishWriter := func(err error) {
		writeErr <- err
		if err != nil {
			_ = conn.Close()
		}
	}
	go func() {
		buffer := make([]byte, opts.ChunkSize)
		for {
			n, readErr := input.Read(buffer)
			if n > 0 {
				if err := conn.WriteMessage(websocket.BinaryMessage, buffer[:n]); err != nil {
					finishWriter(fmt.Errorf("send RTASR audio: %w", err))
					return
				}
				if opts.Interval > 0 {
					select {
					case <-writerCtx.Done():
						finishWriter(writerCtx.Err())
						return
					case <-time.After(opts.Interval):
					}
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				finishWriter(fmt.Errorf("read audio input: %w", readErr))
				return
			}
		}
		if err := conn.WriteJSON(map[string]any{"end": true, "sessionId": requestID}); err != nil {
			finishWriter(fmt.Errorf("send RTASR end marker: %w", err))
			return
		}
		writeErr <- nil
	}()

	writerDone := false
	for {
		if !writerDone {
			select {
			case err := <-writeErr:
				writerDone = true
				if err != nil {
					return result, err
				}
			default:
			}
		}
		_, message, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			if !writerDone {
				select {
				case writeFailure := <-writeErr:
					writerDone = true
					if writeFailure != nil {
						return result, writeFailure
					}
				default:
				}
			}
			if writerDone && result.Segments > 0 {
				return result, nil
			}
			return result, fmt.Errorf("read RTASR response: %w", err)
		}
		if apiErr := eventError(message); apiErr != nil {
			return result, apiErr
		}
		if err := handleEvent(message, &result, onEvent); err != nil {
			return result, err
		}
		if eventIsFinal(message) {
			return result, nil
		}
	}
}

func validateOptions(opts Options) error {
	if opts.Language != "autodialect" && opts.Language != "autominor" {
		return fmt.Errorf("RTASR language must be autodialect or autominor")
	}
	if opts.RecognizedLanguage != "" && opts.Language != "autominor" {
		return fmt.Errorf("RTASR recognized_language is valid only when language=autominor")
	}
	if opts.AudioEncoding != "pcm_s16le" && opts.AudioEncoding != "opus-wb" && opts.AudioEncoding != "speex-7" && opts.AudioEncoding != "speex-10" {
		return fmt.Errorf("unsupported RTASR audio encoding %q", opts.AudioEncoding)
	}
	if opts.AudioEncoding == "pcm_s16le" && opts.SampleRate != 8000 && opts.SampleRate != 16000 {
		return fmt.Errorf("RTASR PCM sample rate must be 8000 or 16000")
	}
	if opts.RoleType != 0 && opts.RoleType != 2 {
		return fmt.Errorf("RTASR role_type must be 0 or 2")
	}
	if opts.FeatureIDs != "" && opts.RoleType != 2 {
		return fmt.Errorf("RTASR feature_ids require role_type=2")
	}
	if opts.SpeakerMatch && (opts.RoleType != 2 || opts.FeatureIDs == "") {
		return fmt.Errorf("RTASR speaker matching requires role_type=2 and feature_ids")
	}
	if opts.VADMode != 0 && opts.VADMode != 1 && opts.VADMode != 2 {
		return fmt.Errorf("RTASR VAD mode must be 1 (far field) or 2 (near field)")
	}
	if opts.ChunkSize <= 0 {
		return fmt.Errorf("RTASR chunk size must be positive")
	}
	if opts.Interval < 0 {
		return fmt.Errorf("RTASR frame interval cannot be negative")
	}
	return nil
}

func handleEvent(message []byte, result *Result, callback func(json.RawMessage) error) error {
	if callback != nil {
		copyOfMessage := append(json.RawMessage(nil), message...)
		if err := callback(copyOfMessage); err != nil {
			return err
		}
	}
	text, finalSegment, sid := eventText(message)
	if sid != "" {
		result.SID = sid
	}
	if finalSegment && text != "" {
		result.Transcript += text
		result.Segments++
	}
	return nil
}

func eventText(message []byte) (string, bool, string) {
	var envelope struct {
		SID  string          `json:"sid"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(message, &envelope) != nil || len(envelope.Data) == 0 {
		return "", false, envelope.SID
	}
	data := envelope.Data
	if len(data) > 0 && data[0] == '"' {
		var nested string
		if json.Unmarshal(data, &nested) == nil {
			data = json.RawMessage(nested)
		}
	}
	var parsed struct {
		CN struct {
			ST struct {
				Type any `json:"type"`
				RT   []struct {
					WS []struct {
						CW []struct {
							Word string `json:"w"`
						} `json:"cw"`
					} `json:"ws"`
				} `json:"rt"`
			} `json:"st"`
		} `json:"cn"`
	}
	if json.Unmarshal(data, &parsed) != nil {
		return "", false, envelope.SID
	}
	var b strings.Builder
	for _, rt := range parsed.CN.ST.RT {
		for _, ws := range rt.WS {
			if len(ws.CW) > 0 {
				b.WriteString(ws.CW[0].Word)
			}
		}
	}
	final := fmt.Sprint(parsed.CN.ST.Type) == "0"
	return b.String(), final, envelope.SID
}

func eventIsFinal(message []byte) bool {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(message, &envelope) != nil {
		return false
	}
	data := envelope.Data
	if len(data) > 0 && data[0] == '"' {
		var nested string
		if json.Unmarshal(data, &nested) == nil {
			data = json.RawMessage(nested)
		}
	}
	var status struct {
		Last bool `json:"ls"`
	}
	return json.Unmarshal(data, &status) == nil && status.Last
}

func eventError(message []byte) error {
	var event struct {
		Action  string `json:"action"`
		MsgType string `json:"msg_type"`
		Code    any    `json:"code"`
		Desc    string `json:"desc"`
		SID     string `json:"sid"`
		Data    struct {
			Normal *bool  `json:"normal"`
			Desc   string `json:"desc"`
		} `json:"data"`
	}
	if json.Unmarshal(message, &event) != nil {
		return nil
	}
	code := fmt.Sprint(event.Code)
	codeFailed := event.Code != nil && code != "" && code != "0" && code != "000000"
	if event.Action == "error" || codeFailed || (event.Data.Normal != nil && !*event.Data.Normal) {
		message := event.Desc
		if event.Data.Desc != "" {
			message = event.Data.Desc
		}
		return &xfyun.APIError{Service: "RTASR", Code: code, Message: message, SID: event.SID}
	}
	return nil
}

func randomUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate request ID: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(b)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}
