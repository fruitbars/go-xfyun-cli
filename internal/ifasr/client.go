package ifasr

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fruitbars/go-xfyun-cli/internal/auth"
	"github.com/fruitbars/go-xfyun-cli/internal/config"
	"github.com/fruitbars/go-xfyun-cli/internal/xfyun"
)

const Endpoint = "https://office-api-ist-dx.iflyaisol.com"

type Code string

func (c *Code) UnmarshalJSON(data []byte) error {
	*c = Code(strings.Trim(string(data), `"`))
	return nil
}

type OrderInfo struct {
	OrderID          string `json:"orderId"`
	FailType         int    `json:"failType"`
	Status           int    `json:"status"`
	OriginalDuration int64  `json:"originalDuration"`
	ExpireTime       int64  `json:"expireTime,omitempty"`
	Language         string `json:"language,omitempty"`
}

type Content struct {
	OrderID          string          `json:"orderId,omitempty"`
	TaskEstimateTime int64           `json:"taskEstimateTime,omitempty"`
	OrderInfo        OrderInfo       `json:"orderInfo,omitempty"`
	OrderResult      string          `json:"orderResult,omitempty"`
	TransResult      json.RawMessage `json:"transResult,omitempty"`
	PredictResult    json.RawMessage `json:"predictResult,omitempty"`
}

type Response struct {
	Code     Code    `json:"code"`
	DescInfo string  `json:"descInfo"`
	Content  Content `json:"content"`
}

type Options struct {
	Language        string
	DurationMS      int64
	Domain          string
	TrackMode       int
	CallbackURL     string
	RoleType        int
	RoleNum         int
	FeatureIDs      string
	Smooth          *bool
	Colloquial      *bool
	VADMode         int
	CantoneseScript *int
	Analysis        bool
	ResultType      string
	PollInterval    time.Duration
	MaxWait         time.Duration
	Extra           map[string]string
	NoWait          bool
	SignatureRand   string
}

var supportedDomains = map[string]struct{}{
	"court": {}, "finance": {}, "medical": {}, "tech": {},
	"sport": {}, "edu": {}, "isp": {}, "gov": {},
	"game": {}, "ecom": {}, "mil": {}, "com": {},
	"life": {}, "ent": {}, "culture": {}, "car": {},
}

var managedIFASRParameters = map[string]struct{}{
	"appId": {}, "accessKeyId": {}, "dateTime": {}, "signatureRandom": {},
	"fileSize": {}, "fileName": {}, "durationCheckDisable": {}, "duration": {},
	"language": {}, "pd": {}, "callbackUrl": {}, "roleType": {}, "roleNum": {},
	"featureIds": {}, "audioMode": {}, "audioUrl": {}, "eng_smoothproc": {},
	"eng_colloqproc": {}, "eng_vad_mdn": {}, "eng_rlang": {}, "analysis": {},
	"trackMode": {},
}

type Result struct {
	OrderID            string   `json:"order_id"`
	SignatureRand      string   `json:"signature_random"`
	Status             int      `json:"status"`
	Transcript         string   `json:"transcript,omitempty"`
	OriginalTranscript string   `json:"original_transcript,omitempty"`
	Response           Response `json:"response"`
}

type Client struct {
	Credentials config.Credentials
	HTTPClient  *http.Client
	Endpoint    string
	Now         func() time.Time
}

func (c *Client) Transcribe(ctx context.Context, audio io.Reader, fileName string, fileSize int64, opts Options) (Result, error) {
	var result Result
	if err := c.Credentials.ValidateSigned(); err != nil {
		return result, err
	}
	if fileSize <= 0 {
		return result, fmt.Errorf("audio file is empty")
	}
	if fileSize > MaxAudioBytes {
		return result, fmt.Errorf("audio file is %d bytes; API limit is 500 MiB", fileSize)
	}
	if err := validateFileName(fileName); err != nil {
		return result, err
	}
	if err := normalizeOptions(&opts); err != nil {
		return result, err
	}
	randomValue, err := signatureRandom(opts.SignatureRand)
	if err != nil {
		return result, err
	}
	uploadResponse, err := c.Upload(ctx, audio, fileName, fileSize, randomValue, opts)
	if err != nil {
		return result, err
	}
	result.OrderID = uploadResponse.Content.OrderID
	result.SignatureRand = randomValue
	result.Response = uploadResponse
	if opts.NoWait {
		return result, nil
	}
	return c.Wait(ctx, result.OrderID, randomValue, opts)
}

// TranscribeURL submits an externally hosted recording. Unlike local files,
// URL inputs cannot be split by this client, so callers must provide the
// remote byte size and keep the recording within one service order's limits.
func (c *Client) TranscribeURL(ctx context.Context, audioURL, fileName string, fileSize int64, opts Options) (Result, error) {
	var result Result
	if err := c.Credentials.ValidateSigned(); err != nil {
		return result, err
	}
	if err := validateAudioURL(audioURL); err != nil {
		return result, err
	}
	if fileSize <= 0 {
		return result, fmt.Errorf("audio URL file size must be positive")
	}
	if fileSize > MaxAudioBytes {
		return result, fmt.Errorf("audio URL file is %d bytes; use a local input for automatic splitting above 500 MiB", fileSize)
	}
	if err := validateFileName(fileName); err != nil {
		return result, err
	}
	if err := normalizeOptions(&opts); err != nil {
		return result, err
	}
	randomValue, err := signatureRandom(opts.SignatureRand)
	if err != nil {
		return result, err
	}
	uploadResponse, err := c.UploadURL(ctx, audioURL, fileName, fileSize, randomValue, opts)
	if err != nil {
		return result, err
	}
	result.OrderID = uploadResponse.Content.OrderID
	result.SignatureRand = randomValue
	result.Response = uploadResponse
	if opts.NoWait {
		return result, nil
	}
	return c.Wait(ctx, result.OrderID, randomValue, opts)
}

func (c *Client) Upload(ctx context.Context, audio io.Reader, fileName string, fileSize int64, signatureRandom string, opts Options) (Response, error) {
	if err := c.Credentials.ValidateSigned(); err != nil {
		return Response{}, err
	}
	params := c.uploadParams(fileName, fileSize, signatureRandom, opts)
	signature := auth.HMACSHA1(params, c.Credentials.APISecret)
	endpoint := strings.TrimRight(c.endpoint(), "/") + "/v2/upload?" + auth.Query(params)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, audio)
	if err != nil {
		return Response{}, fmt.Errorf("create IFASR upload request: %w", err)
	}
	req.ContentLength = fileSize
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("signature", signature)
	return c.do(req)
}

func (c *Client) UploadURL(ctx context.Context, audioURL, fileName string, fileSize int64, signatureRandom string, opts Options) (Response, error) {
	if err := c.Credentials.ValidateSigned(); err != nil {
		return Response{}, err
	}
	params := c.uploadParams(fileName, fileSize, signatureRandom, opts)
	params["audioMode"] = "urlLink"
	params["audioUrl"] = audioURL
	signature := auth.HMACSHA1(params, c.Credentials.APISecret)
	endpoint := strings.TrimRight(c.endpoint(), "/") + "/v2/upload?" + auth.Query(params)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, http.NoBody)
	if err != nil {
		return Response{}, fmt.Errorf("create IFASR URL upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("signature", signature)
	return c.do(req)
}

func (c *Client) uploadParams(fileName string, fileSize int64, signatureRandom string, opts Options) map[string]string {
	params := make(map[string]string, len(opts.Extra)+20)
	for key, value := range opts.Extra {
		params[key] = value
	}
	params["appId"] = c.Credentials.AppID
	params["accessKeyId"] = c.Credentials.APIKey
	params["dateTime"] = c.now().Format("2006-01-02T15:04:05-0700")
	params["signatureRandom"] = signatureRandom
	params["fileSize"] = strconv.FormatInt(fileSize, 10)
	params["fileName"] = filepath.Base(fileName)
	params["language"] = opts.Language
	if opts.DurationMS > 0 {
		params["duration"] = strconv.FormatInt(opts.DurationMS, 10)
		params["durationCheckDisable"] = "false"
	} else {
		params["durationCheckDisable"] = "true"
	}
	if opts.Domain != "" {
		params["pd"] = opts.Domain
	}
	if opts.TrackMode != 0 {
		params["trackMode"] = strconv.Itoa(opts.TrackMode)
	}
	if opts.CallbackURL != "" {
		params["callbackUrl"] = opts.CallbackURL
	}
	if opts.RoleType != 0 {
		params["roleType"] = strconv.Itoa(opts.RoleType)
	}
	if opts.RoleNum != 0 {
		params["roleNum"] = strconv.Itoa(opts.RoleNum)
	}
	if opts.FeatureIDs != "" {
		params["featureIds"] = opts.FeatureIDs
	}
	if opts.Smooth != nil {
		params["eng_smoothproc"] = strconv.FormatBool(*opts.Smooth)
	}
	if opts.Colloquial != nil {
		params["eng_colloqproc"] = strconv.FormatBool(*opts.Colloquial)
	}
	if opts.VADMode != 0 {
		params["eng_vad_mdn"] = strconv.Itoa(opts.VADMode)
	}
	if opts.CantoneseScript != nil {
		params["eng_rlang"] = strconv.Itoa(*opts.CantoneseScript)
	}
	if opts.Analysis {
		params["analysis"] = "1"
	}
	return params
}

func normalizeOptions(opts *Options) error {
	if opts.Language == "" {
		opts.Language = "autodialect"
	}
	if opts.ResultType == "" {
		opts.ResultType = "transfer"
	}
	if opts.PollInterval == 0 {
		opts.PollInterval = 2 * time.Second
	}
	if opts.MaxWait == 0 {
		opts.MaxWait = 30 * time.Minute
	}
	return validateOptions(*opts)
}

func signatureRandom(configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	return randomString(16)
}

func validateOptions(opts Options) error {
	if opts.Language != "" && opts.Language != "autodialect" && opts.Language != "autominor" {
		return fmt.Errorf("IFASR language must be autodialect or autominor")
	}
	if opts.RoleType != 0 && opts.RoleType != 1 && opts.RoleType != 3 {
		return fmt.Errorf("IFASR role_type must be 0, 1, or 3")
	}
	if opts.RoleNum < 0 || opts.RoleNum > 10 {
		return fmt.Errorf("IFASR role_num must be between 0 and 10")
	}
	if opts.RoleNum != 0 && opts.RoleType == 0 {
		return fmt.Errorf("IFASR role_num requires role_type 1 or 3")
	}
	if opts.FeatureIDs != "" && opts.RoleType != 3 {
		return fmt.Errorf("IFASR feature_ids require role_type=3")
	}
	if opts.RoleType == 3 && opts.FeatureIDs == "" {
		return fmt.Errorf("IFASR role_type=3 requires feature_ids")
	}
	if opts.FeatureIDs != "" {
		featureIDs := strings.Split(opts.FeatureIDs, ",")
		if len(featureIDs) > 64 {
			return fmt.Errorf("IFASR supports at most 64 feature_ids")
		}
		for _, featureID := range featureIDs {
			if strings.TrimSpace(featureID) == "" {
				return fmt.Errorf("IFASR feature_ids must not contain empty values")
			}
		}
	}
	if opts.TrackMode != 0 && opts.TrackMode != 1 && opts.TrackMode != 2 {
		return fmt.Errorf("IFASR track_mode must be 1 (mixed) or 2 (stereo tracks)")
	}
	if opts.TrackMode == 2 && opts.RoleType != 0 {
		return fmt.Errorf("IFASR track_mode=2 cannot be combined with role_type")
	}
	if opts.TrackMode == 2 && opts.Analysis {
		return fmt.Errorf("IFASR language analysis is unavailable with track_mode=2")
	}
	if opts.Domain != "" {
		if _, ok := supportedDomains[opts.Domain]; !ok {
			return fmt.Errorf("unsupported IFASR domain %q", opts.Domain)
		}
	}
	if opts.Analysis && opts.Language != "autominor" {
		return fmt.Errorf("IFASR language analysis requires language=autominor")
	}
	if opts.VADMode != 0 && opts.VADMode != 1 && opts.VADMode != 2 {
		return fmt.Errorf("IFASR VAD mode must be 1 (far field) or 2 (near field)")
	}
	if opts.CantoneseScript != nil && *opts.CantoneseScript != 0 && *opts.CantoneseScript != 1 {
		return fmt.Errorf("IFASR Cantonese script must be 0 (simplified) or 1 (traditional)")
	}
	if len(opts.CallbackURL) > 512 {
		return fmt.Errorf("IFASR callback URL exceeds 512 characters")
	}
	if opts.CallbackURL != "" {
		parsed, err := url.Parse(opts.CallbackURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("IFASR callback URL must be an absolute http or https URL")
		}
	}
	if opts.DurationMS < 0 || opts.DurationMS > MaxAudioDurationMS {
		return fmt.Errorf("IFASR duration must be between 0 and 18000000 milliseconds")
	}
	if opts.ResultType != "" && opts.ResultType != "transfer" && opts.ResultType != "analysis" && opts.ResultType != "transfer,analysis" {
		return fmt.Errorf("IFASR result_type must be transfer, analysis, or transfer,analysis")
	}
	for key := range opts.Extra {
		if _, managed := managedIFASRParameters[key]; managed {
			return fmt.Errorf("IFASR parameter %s has a first-class option and cannot be supplied through extra", key)
		}
	}
	return nil
}

func validateAudioURL(value string) error {
	if len(value) > 512 {
		return fmt.Errorf("IFASR audio URL exceeds 512 characters")
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("IFASR audio URL must be an absolute http or https URL")
	}
	return nil
}

func validateFileName(fileName string) error {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".mp3", ".wav", ".pcm", ".opus", ".flac", ".ogg", ".speex":
		return nil
	default:
		return fmt.Errorf("unsupported IFASR audio extension %q; use mp3, wav, pcm, opus, flac, ogg, or speex", ext)
	}
}

func (c *Client) Query(ctx context.Context, orderID, signatureRandom, resultType string) (Response, error) {
	if err := c.Credentials.ValidateSigned(); err != nil {
		return Response{}, err
	}
	if orderID == "" || signatureRandom == "" {
		return Response{}, fmt.Errorf("order ID and signature random are required")
	}
	if resultType == "" {
		resultType = "transfer"
	}
	if err := validateOptions(Options{ResultType: resultType}); err != nil {
		return Response{}, err
	}
	params := map[string]string{
		"accessKeyId":     c.Credentials.APIKey,
		"dateTime":        c.now().Format("2006-01-02T15:04:05-0700"),
		"signatureRandom": signatureRandom,
		"orderId":         orderID,
		"resultType":      resultType,
	}
	signature := auth.HMACSHA1(params, c.Credentials.APISecret)
	endpoint := strings.TrimRight(c.endpoint(), "/") + "/v2/getResult?" + auth.Query(params)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString("{}"))
	if err != nil {
		return Response{}, fmt.Errorf("create IFASR query request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("signature", signature)
	return c.do(req)
}

func (c *Client) Wait(ctx context.Context, orderID, signatureRandom string, opts Options) (Result, error) {
	result := Result{OrderID: orderID, SignatureRand: signatureRandom}
	if opts.PollInterval <= 0 {
		opts.PollInterval = 2 * time.Second
	}
	if opts.MaxWait <= 0 {
		opts.MaxWait = 30 * time.Minute
	}
	waitCtx, cancel := context.WithTimeout(ctx, opts.MaxWait)
	defer cancel()
	for {
		response, err := c.Query(waitCtx, orderID, signatureRandom, opts.ResultType)
		if err != nil {
			return result, err
		}
		result.Response = response
		result.Status = response.Content.OrderInfo.Status
		switch result.Status {
		case 4:
			transcript, original, err := ExtractTranscripts(response.Content.OrderResult)
			if err != nil {
				return result, err
			}
			result.Transcript = transcript
			result.OriginalTranscript = original
			return result, nil
		case -1:
			return result, &xfyun.APIError{Service: "IFASR", Code: strconv.Itoa(response.Content.OrderInfo.FailType), Message: "transcription order failed"}
		}
		timer := time.NewTimer(opts.PollInterval)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return result, fmt.Errorf("wait for IFASR order %s: %w", orderID, waitCtx.Err())
		case <-timer.C:
		}
	}
}

func (c *Client) do(req *http.Request) (Response, error) {
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Minute}
	}
	httpResp, err := client.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("IFASR request: %w", err)
	}
	defer httpResp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(httpResp.Body, 64*1024*1024))
	if err != nil {
		return Response{}, fmt.Errorf("read IFASR response: %w", err)
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("IFASR HTTP %s: %s", httpResp.Status, strings.TrimSpace(string(body)))
	}
	var response Response
	if err := json.Unmarshal(body, &response); err != nil {
		return Response{}, fmt.Errorf("decode IFASR response: %w", err)
	}
	if response.Code != "000000" && response.Code != "0" {
		return response, &xfyun.APIError{Service: "IFASR", Code: string(response.Code), Message: response.DescInfo}
	}
	return response, nil
}

func (c *Client) endpoint() string {
	if c.Endpoint != "" {
		return c.Endpoint
	}
	return Endpoint
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func ExtractTranscript(orderResult string) (string, error) {
	processed, _, err := ExtractTranscripts(orderResult)
	return processed, err
}

// ExtractTranscripts returns the processed transcript from lattice and, when
// smoothing or colloquial processing was enabled, the original transcript
// retained by the service in lattice2.
func ExtractTranscripts(orderResult string) (string, string, error) {
	if orderResult == "" {
		return "", "", nil
	}
	var outer struct {
		Lattice  []latticeItem `json:"lattice"`
		Lattice2 []latticeItem `json:"lattice2"`
	}
	if err := json.Unmarshal([]byte(orderResult), &outer); err != nil {
		return "", "", fmt.Errorf("decode IFASR order result: %w", err)
	}
	processed, err := extractLattice(outer.Lattice)
	if err != nil {
		return "", "", err
	}
	original, err := extractLattice(outer.Lattice2)
	if err != nil {
		return "", "", err
	}
	return processed, original, nil
}

type latticeItem struct {
	// The service returns json_1best inconsistently: older responses encode the
	// nested JSON as a string, while newer lattice2 responses may embed it as an
	// object. Keep the raw representation and accept both forms below.
	Best json.RawMessage `json:"json_1best"`
}

func extractLattice(latticeItems []latticeItem) (string, error) {
	var transcript strings.Builder
	for _, lattice := range latticeItems {
		bestJSON := bytes.TrimSpace(lattice.Best)
		if len(bestJSON) == 0 || bytes.Equal(bestJSON, []byte("null")) {
			continue
		}
		if bestJSON[0] == '"' {
			var encoded string
			if err := json.Unmarshal(bestJSON, &encoded); err != nil {
				return "", fmt.Errorf("decode IFASR segment string: %w", err)
			}
			bestJSON = []byte(encoded)
		}
		var best struct {
			ST struct {
				RT []struct {
					WS []struct {
						CW []struct {
							Word string `json:"w"`
						} `json:"cw"`
					} `json:"ws"`
				} `json:"rt"`
			} `json:"st"`
		}
		if err := json.Unmarshal(bestJSON, &best); err != nil {
			return "", fmt.Errorf("decode IFASR segment: %w", err)
		}
		for _, rt := range best.ST.RT {
			for _, ws := range rt.WS {
				if len(ws.CW) > 0 {
					transcript.WriteString(ws.CW[0].Word)
				}
			}
		}
	}
	return transcript.String(), nil
}

func randomString(length int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	randomBytes := make([]byte, length)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("generate signature random: %w", err)
	}
	for i := range randomBytes {
		randomBytes[i] = alphabet[int(randomBytes[i])%len(alphabet)]
	}
	return string(randomBytes), nil
}
