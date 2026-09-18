package ocr

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/fruitbars/go-xfyun-cli/internal/auth"
	"github.com/fruitbars/go-xfyun-cli/internal/config"
	"github.com/fruitbars/go-xfyun-cli/internal/xfyun"
	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const Endpoint = "https://cbm01.cn-huabei-1.xf-yun.com/v1/private/se75ocrbm"

const (
	// MaxSourceImageBytes bounds local memory use before automatic compression.
	MaxSourceImageBytes = 32 * 1024 * 1024
	// MaxUploadImageBytes is the maximum compressed image payload sent to OCR.
	MaxUploadImageBytes = 4 * 1024 * 1024
	// MaxEncodedImageBytes is the maximum base64 payload sent to OCR.
	MaxEncodedImageBytes = 10 * 1024 * 1024
	// MaxOCRResultBytes bounds a decompressed result from one page. The HTTP
	// response itself is separately limited to 16 MiB.
	MaxOCRResultBytes = 64 * 1024 * 1024
	maxDecodedPixels  = 40_000_000
)

type Options struct {
	ResultOption          string
	ResultFormat          string
	OutputType            string
	ExifOption            string
	JSONElementOption     string
	MarkdownElementOption string
	SEDElementOption      string
	AlphaOption           string
	RotationMinAngle      float64
	Encoding              string
}

type Header struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	SID     string `json:"sid"`
	Status  int    `json:"status"`
}

type Result struct {
	Encoding string `json:"encoding"`
	Compress string `json:"compress"`
	Format   string `json:"format"`
	Status   int    `json:"status"`
	Seq      int    `json:"seq"`
	Text     string `json:"text"`
}

type Response struct {
	Header  Header `json:"header"`
	Payload struct {
		Result Result `json:"result"`
	} `json:"payload"`
}

type Client struct {
	Credentials config.Credentials
	HTTPClient  *http.Client
	Endpoint    string
	Now         func() time.Time
}

func (c *Client) Recognize(ctx context.Context, image []byte, opts Options) ([]byte, *Response, error) {
	if err := c.Credentials.ValidateSigned(); err != nil {
		return nil, nil, err
	}
	if len(image) == 0 {
		return nil, nil, fmt.Errorf("image is empty")
	}
	if len(image) > MaxSourceImageBytes {
		return nil, nil, fmt.Errorf("source image is %d bytes; local compression limit is %d MiB", len(image), MaxSourceImageBytes/(1024*1024))
	}
	if opts.Encoding == "" {
		var err error
		opts.Encoding, err = DetectEncoding(image)
		if err != nil {
			return nil, nil, err
		}
	}
	if opts.ResultOption == "" {
		opts.ResultOption = "normal"
	}
	if opts.ResultFormat == "" {
		opts.ResultFormat = "json,markdown"
	}
	if opts.OutputType == "" {
		opts.OutputType = "one_shot"
	}
	if opts.ExifOption == "" {
		opts.ExifOption = "0"
	}
	if opts.AlphaOption == "" {
		opts.AlphaOption = "0"
	}
	if err := validateOptions(opts); err != nil {
		return nil, nil, err
	}
	prepared, preparedEncoding, err := prepareImage(image, opts.Encoding, opts.AlphaOption == "1")
	if err != nil {
		return nil, nil, err
	}
	opts.Encoding = preparedEncoding
	encoded := base64.StdEncoding.EncodeToString(prepared)
	if len(prepared) > MaxUploadImageBytes || len(encoded) > MaxEncodedImageBytes {
		return nil, nil, fmt.Errorf("compressed OCR image exceeds upload limits: raw=%d bytes, base64=%d bytes", len(prepared), len(encoded))
	}
	body := map[string]any{
		"header": map[string]any{"app_id": c.Credentials.AppID, "status": 0},
		"parameter": map[string]any{"ocr": map[string]any{
			"result_option": opts.ResultOption, "result_format": opts.ResultFormat,
			"output_type": opts.OutputType, "exif_option": opts.ExifOption,
			"json_element_option":     opts.JSONElementOption,
			"markdown_element_option": opts.MarkdownElementOption,
			"sed_element_option":      opts.SEDElementOption,
			"alpha_option":            opts.AlphaOption, "rotation_min_angle": opts.RotationMinAngle,
			"result": map[string]any{"encoding": "utf8", "compress": "raw", "format": "plain"},
		}},
		"payload": map[string]any{"image": map[string]any{
			"encoding": opts.Encoding, "image": encoded, "status": 0, "seq": 0,
		}},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, nil, fmt.Errorf("encode request: %w", err)
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = Endpoint
	}
	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	signed, err := auth.SignedURL(endpoint, http.MethodPost, c.Credentials.APIKey, c.Credentials.APISecret, now)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, signed, bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	httpResp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("OCR request: %w", err)
	}
	defer httpResp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(httpResp.Body, 16*1024*1024))
	if err != nil {
		return nil, nil, fmt.Errorf("read OCR response: %w", err)
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("OCR HTTP %s: %s", httpResp.Status, strings.TrimSpace(string(responseBody)))
	}
	var response Response
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, nil, fmt.Errorf("decode OCR response: %w", err)
	}
	if response.Header.Code != 0 {
		return nil, &response, &xfyun.APIError{Service: "OCR", Code: fmt.Sprint(response.Header.Code), Message: response.Header.Message, SID: response.Header.SID}
	}
	decoded, err := base64.StdEncoding.DecodeString(response.Payload.Result.Text)
	if err != nil {
		return nil, &response, fmt.Errorf("decode OCR result base64: %w", err)
	}
	if response.Payload.Result.Compress == "gzip" {
		zr, err := gzip.NewReader(bytes.NewReader(decoded))
		if err != nil {
			return nil, &response, fmt.Errorf("open compressed OCR result: %w", err)
		}
		defer zr.Close()
		decoded, err = io.ReadAll(io.LimitReader(zr, MaxOCRResultBytes+1))
		if err != nil {
			return nil, &response, fmt.Errorf("decompress OCR result: %w", err)
		}
		if len(decoded) > MaxOCRResultBytes {
			return nil, &response, fmt.Errorf("decompressed OCR result exceeds the %d MiB per-page limit", MaxOCRResultBytes/(1024*1024))
		}
	}
	return decoded, &response, nil
}

func prepareImage(data []byte, encoding string, preserveAlpha bool) ([]byte, string, error) {
	if isAPIEncoding(encoding) && len(data) <= MaxUploadImageBytes && base64.StdEncoding.EncodedLen(len(data)) <= MaxEncodedImageBytes {
		return data, encoding, nil
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("decode oversized OCR image header: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > maxDecodedPixels {
		return nil, "", fmt.Errorf("OCR image dimensions %dx%d exceed the %d megapixel compression limit", config.Width, config.Height, maxDecodedPixels/1_000_000)
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("decode oversized OCR image: %w", err)
	}
	targetEncoding := "jpg"
	if preserveAlpha {
		targetEncoding = "png"
	}
	current := decoded
	for attempt := 0; attempt < 10; attempt++ {
		encoded, err := encodeCompressed(current, targetEncoding)
		if err != nil {
			return nil, "", err
		}
		if len(encoded) <= MaxUploadImageBytes && base64.StdEncoding.EncodedLen(len(encoded)) <= MaxEncodedImageBytes {
			return encoded, targetEncoding, nil
		}
		bounds := current.Bounds()
		ratio := math.Sqrt(float64(MaxUploadImageBytes)/float64(len(encoded))) * 0.92
		if ratio > 0.85 {
			ratio = 0.85
		}
		newWidth := max(1, int(float64(bounds.Dx())*ratio))
		newHeight := max(1, int(float64(bounds.Dy())*ratio))
		if newWidth == bounds.Dx() && newHeight == bounds.Dy() {
			break
		}
		resized := image.NewNRGBA(image.Rect(0, 0, newWidth, newHeight))
		draw.CatmullRom.Scale(resized, resized.Bounds(), current, bounds, draw.Over, nil)
		current = resized
	}
	return nil, "", fmt.Errorf("unable to compress OCR image below 4 MiB raw and 10 MiB base64")
}

func isAPIEncoding(encoding string) bool {
	return oneOf(encoding, "jpg", "jpeg", "png", "bmp")
}

// DetectEncoding inspects image content instead of trusting a file extension.
// Unsupported API formats may still be accepted when a registered pure-Go
// decoder can convert them to JPEG or PNG.
func DetectEncoding(data []byte) (string, error) {
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("unsupported or invalid image: %w", err)
	}
	if format == "jpeg" {
		return "jpg", nil
	}
	return strings.ToLower(format), nil
}

func encodeCompressed(source image.Image, encoding string) ([]byte, error) {
	var output bytes.Buffer
	switch encoding {
	case "png":
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		if err := encoder.Encode(&output, source); err != nil {
			return nil, fmt.Errorf("compress OCR image as PNG: %w", err)
		}
	default:
		if err := jpeg.Encode(&output, source, &jpeg.Options{Quality: 88}); err != nil {
			return nil, fmt.Errorf("compress OCR image as JPEG: %w", err)
		}
	}
	return output.Bytes(), nil
}

func validateOptions(opts Options) error {
	if !oneOf(opts.Encoding, "jpg", "jpeg", "png", "bmp", "gif", "webp", "tif", "tiff") {
		return fmt.Errorf("unsupported OCR image encoding %q", opts.Encoding)
	}
	if !oneOf(opts.ResultOption, "normal", "normal,char", "normal,no_line_position", "normal,char,no_line_position") {
		return fmt.Errorf("unsupported OCR result_option %q", opts.ResultOption)
	}
	if !oneOf(opts.ResultFormat, "json", "json,markdown", "json,sed", "json,markdown,sed") {
		return fmt.Errorf("unsupported OCR result_format %q", opts.ResultFormat)
	}
	if opts.OutputType != "one_shot" {
		return fmt.Errorf("unsupported OCR output_type %q; this client currently supports one_shot", opts.OutputType)
	}
	if !oneOf(opts.ExifOption, "0", "1") {
		return fmt.Errorf("OCR exif_option must be 0 or 1")
	}
	if !oneOf(opts.AlphaOption, "0", "1") {
		return fmt.Errorf("OCR alpha_option must be 0 or 1")
	}
	if opts.RotationMinAngle < 0 || opts.RotationMinAngle > 180 {
		return fmt.Errorf("OCR rotation_min_angle must be between 0 and 180")
	}
	for name, value := range map[string]string{
		"json_element_option":     opts.JSONElementOption,
		"markdown_element_option": opts.MarkdownElementOption,
		"sed_element_option":      opts.SEDElementOption,
	} {
		if len(value) > 1000 {
			return fmt.Errorf("OCR %s exceeds 1000 bytes", name)
		}
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
