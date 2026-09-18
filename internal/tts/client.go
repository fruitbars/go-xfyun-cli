package tts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/fruitbars/go-xfyun-cli/internal/auth"
	"github.com/fruitbars/go-xfyun-cli/internal/config"
	"github.com/fruitbars/go-xfyun-cli/internal/xfyun"
)

const Endpoint = "wss://cbm01.cn-huabei-1.xf-yun.com/v1/private/mcd9m97e6"

type Options struct {
	Voice             string
	Speed             int
	Volume            int
	Pitch             int
	Encoding          string
	SampleRate        int
	OralLevel         string
	SparkAssist       int
	StopSplit         int
	Remain            int
	BackgroundSound   int
	EnglishReading    int
	NumberReading     int
	ReturnPronounce   int
	VisibleWatermark  int
	ImplicitWatermark bool
}

type Metadata struct {
	SID           string `json:"sid"`
	Encoding      string `json:"encoding"`
	SampleRate    int    `json:"sample_rate"`
	Bytes         int64  `json:"bytes"`
	Pronunciation string `json:"pronunciation,omitempty"`
}

type Client struct {
	Credentials config.Credentials
	Dialer      *websocket.Dialer
	Endpoint    string
	Now         func() time.Time
}

type response struct {
	Header struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		SID     string `json:"sid"`
		Status  int    `json:"status"`
	} `json:"header"`
	Payload struct {
		Audio struct {
			Encoding   string `json:"encoding"`
			SampleRate int    `json:"sample_rate"`
			Status     int    `json:"status"`
			Audio      string `json:"audio"`
		} `json:"audio"`
		Pronunciation struct {
			Status int    `json:"status"`
			Text   string `json:"text"`
		} `json:"pybuf"`
	} `json:"payload"`
}

func (c *Client) Synthesize(ctx context.Context, text string, output io.Writer, opts Options) (Metadata, error) {
	var metadata Metadata
	if err := c.Credentials.ValidateTTS(); err != nil {
		return metadata, err
	}
	if text == "" {
		return metadata, fmt.Errorf("text is empty")
	}
	if len([]byte(text)) > 64*1024 {
		return metadata, fmt.Errorf("text is %d bytes; streaming-session text limit is 64 KiB", len([]byte(text)))
	}
	encodedText := base64.StdEncoding.EncodeToString([]byte(text))
	if opts.Voice == "" {
		opts.Voice = "x5_lingxiaoxuan_flow"
	}
	if opts.Encoding == "" {
		opts.Encoding = "lame"
	}
	if opts.SampleRate == 0 {
		opts.SampleRate = 24000
	}
	if opts.OralLevel == "" {
		opts.OralLevel = "mid"
	}
	if err := validateOptions(opts); err != nil {
		return metadata, err
	}

	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = Endpoint
	}
	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	signed, err := auth.SignedURL(endpoint, http.MethodGet, c.Credentials.APIKey, c.Credentials.APISecret, now)
	if err != nil {
		return metadata, err
	}
	dialer := c.Dialer
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}
	conn, httpResp, err := dialer.DialContext(ctx, signed, nil)
	if err != nil {
		if httpResp != nil {
			return metadata, fmt.Errorf("TTS websocket handshake failed: %s", httpResp.Status)
		}
		return metadata, fmt.Errorf("TTS websocket connection: %w", err)
	}
	defer conn.Close()
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()

	request := map[string]any{
		"header": map[string]any{"app_id": c.Credentials.AppID, "status": 2},
		"parameter": map[string]any{
			"oral": map[string]any{
				"oral_level": opts.OralLevel, "spark_assist": opts.SparkAssist,
				"stop_split": opts.StopSplit, "remain": opts.Remain,
			},
			"tts": map[string]any{
				"vcn": opts.Voice, "speed": opts.Speed, "volume": opts.Volume,
				"pitch": opts.Pitch, "bgs": opts.BackgroundSound,
				"reg": opts.EnglishReading, "rdn": opts.NumberReading,
				"rhy": opts.ReturnPronounce, "watermask": opts.VisibleWatermark,
				"implicit_watermark": opts.ImplicitWatermark,
				"audio": map[string]any{
					"encoding": opts.Encoding, "sample_rate": opts.SampleRate,
					"channels": 1, "bit_depth": 16, "frame_size": 0,
				},
			},
		},
		"payload": map[string]any{"text": map[string]any{
			"encoding": "utf8", "compress": "raw", "format": "plain",
			"status": 2, "seq": 0,
			"text": encodedText,
		}},
	}
	if err := conn.WriteJSON(request); err != nil {
		return metadata, fmt.Errorf("send TTS request: %w", err)
	}

	var pronunciation strings.Builder
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return metadata, fmt.Errorf("read TTS response: %w", err)
		}
		var resp response
		if err := json.Unmarshal(message, &resp); err != nil {
			return metadata, fmt.Errorf("decode TTS response: %w", err)
		}
		metadata.SID = resp.Header.SID
		if resp.Header.Code != 0 {
			return metadata, &xfyun.APIError{Service: "TTS", Code: fmt.Sprint(resp.Header.Code), Message: resp.Header.Message, SID: resp.Header.SID}
		}
		if resp.Payload.Audio.Encoding != "" {
			metadata.Encoding = resp.Payload.Audio.Encoding
			metadata.SampleRate = resp.Payload.Audio.SampleRate
		}
		if resp.Payload.Audio.Audio != "" {
			chunk, err := base64.StdEncoding.DecodeString(resp.Payload.Audio.Audio)
			if err != nil {
				return metadata, fmt.Errorf("decode TTS audio: %w", err)
			}
			n, err := output.Write(chunk)
			metadata.Bytes += int64(n)
			if err != nil {
				return metadata, fmt.Errorf("write TTS audio: %w", err)
			}
		}
		if resp.Payload.Pronunciation.Text != "" {
			chunk, err := base64.StdEncoding.DecodeString(resp.Payload.Pronunciation.Text)
			if err != nil {
				return metadata, fmt.Errorf("decode TTS pronunciation: %w", err)
			}
			pronunciation.Write(chunk)
		}
		if resp.Payload.Audio.Status == 2 || resp.Header.Status == 2 {
			metadata.Pronunciation = pronunciation.String()
			return metadata, nil
		}
	}
}

func validateOptions(opts Options) error {
	if opts.Speed < 0 || opts.Speed > 100 {
		return fmt.Errorf("TTS speed must be between 0 and 100")
	}
	if opts.Volume < 0 || opts.Volume > 100 {
		return fmt.Errorf("TTS volume must be between 0 and 100")
	}
	if opts.Pitch < 0 || opts.Pitch > 100 {
		return fmt.Errorf("TTS pitch must be between 0 and 100")
	}
	if !stringIn(opts.OralLevel, "low", "mid", "high") {
		return fmt.Errorf("TTS oral_level must be low, mid, or high")
	}
	if !intIn(opts.SparkAssist, 0, 1) || !intIn(opts.StopSplit, 0, 1) || !intIn(opts.Remain, 0, 1) {
		return fmt.Errorf("TTS spark_assist, stop_split, and remain must be 0 or 1")
	}
	if !intIn(opts.BackgroundSound, 0, 1) {
		return fmt.Errorf("TTS background sound must be 0 or 1")
	}
	if !intIn(opts.EnglishReading, 0, 1, 2) {
		return fmt.Errorf("TTS English reading mode must be 0, 1, or 2")
	}
	if !intIn(opts.NumberReading, 0, 1, 2, 3) {
		return fmt.Errorf("TTS number reading mode must be between 0 and 3")
	}
	if !intIn(opts.ReturnPronounce, 0, 1) {
		return fmt.Errorf("TTS pronunciation output must be 0 or 1")
	}
	if !intIn(opts.VisibleWatermark, 0, 1, 2) {
		return fmt.Errorf("TTS visible watermark must be 0, 1, or 2")
	}
	if !stringIn(opts.Encoding, "raw", "lame", "speex", "opus", "opus-wb", "opus-swb", "speex-wb") {
		return fmt.Errorf("unsupported TTS audio encoding %q", opts.Encoding)
	}
	if !intIn(opts.SampleRate, 8000, 16000, 24000) {
		return fmt.Errorf("TTS sample rate must be 8000, 16000, or 24000")
	}
	if opts.ImplicitWatermark && opts.Encoding != "lame" {
		return fmt.Errorf("TTS implicit watermark is supported only with lame/MP3 output")
	}
	return nil
}

func stringIn(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func intIn(value int, allowed ...int) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
