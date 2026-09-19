package ocr

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"testing"
)

func TestValidateOptions(t *testing.T) {
	valid := Options{
		Encoding: "png", ResultOption: "normal,char,no_line_position",
		ResultFormat: "json,markdown,sed", OutputType: "one_shot",
		ExifOption: "1", AlphaOption: "1", RotationMinAngle: 180,
	}
	if err := validateOptions(valid); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.ResultFormat = "markdown"
	if err := validateOptions(invalid); err == nil {
		t.Fatal("expected invalid result format error")
	}
	invalid = valid
	invalid.OutputType = "streaming_layout"
	if err := validateOptions(invalid); err == nil {
		t.Fatal("expected unsupported streaming output error")
	}
}

func TestResultAcceptsNumericAndQuotedSeq(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{input: `{"seq":3,"text":"numeric"}`, want: 3},
		{input: `{"seq":"4","text":"quoted"}`, want: 4},
	}
	for _, test := range tests {
		var response Response
		if err := json.Unmarshal([]byte(`{"payload":{"result":`+test.input+`}}`), &response); err != nil {
			t.Fatalf("decode %s: %v", test.input, err)
		}
		if response.Payload.Result.Seq != test.want {
			t.Fatalf("seq = %d, want %d for %s", response.Payload.Result.Seq, test.want, test.input)
		}
	}
}

func TestResponseAcceptsQuotedProtocolStatuses(t *testing.T) {
	var response Response
	input := `{"header":{"code":"0","status":"0","message":"ok","sid":"sid"},"payload":{"result":{"status":"0","seq":"3","text":""}}}`
	if err := json.Unmarshal([]byte(input), &response); err != nil {
		t.Fatal(err)
	}
	if response.Header.Code != 0 || response.Header.Status != 0 || response.Payload.Result.Status != 0 || response.Payload.Result.Seq != 3 {
		t.Fatalf("decoded response = %+v / %+v", response.Header, response.Payload.Result)
	}
}

func TestExtractResultFormats(t *testing.T) {
	formats := ExtractResultFormats([]byte(`{"document":[{"name":"markdown","value":"# 标题"},{"name":"sed","value":"<text>内容</text>"}],"image":[]}`))
	if formats.Markdown != "# 标题" || formats.SED != "<text>内容</text>" {
		t.Fatalf("formats = %+v", formats)
	}
	if got := ExtractResultFormats([]byte("plain text")); got != (ResultFormats{}) {
		t.Fatalf("plain response formats = %+v", got)
	}
}

func TestPrepareImageConvertsUnsupportedAPIFormat(t *testing.T) {
	img := image.NewPaletted(image.Rect(0, 0, 2, 2), []color.Color{color.White, color.Black})
	img.SetColorIndex(1, 1, 1)
	var source bytes.Buffer
	if err := gif.Encode(&source, img, nil); err != nil {
		t.Fatal(err)
	}
	prepared, encoding, err := prepareImage(source.Bytes(), "gif", false)
	if err != nil {
		t.Fatal(err)
	}
	if encoding != "jpg" || len(prepared) == 0 {
		t.Fatalf("encoding=%q size=%d", encoding, len(prepared))
	}
}

func TestPrepareImageCompressesOversizedSource(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1300, 1300))
	var state uint32 = 1
	for index := range img.Pix {
		state = state*1664525 + 1013904223
		img.Pix[index] = byte(state >> 24)
	}
	var source bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.NoCompression}
	if err := encoder.Encode(&source, img); err != nil {
		t.Fatal(err)
	}
	if source.Len() <= MaxUploadImageBytes {
		t.Fatalf("test source is only %d bytes", source.Len())
	}
	prepared, encoding, err := prepareImage(source.Bytes(), "png", false)
	if err != nil {
		t.Fatal(err)
	}
	if encoding != "jpg" {
		t.Fatalf("encoding = %q, want jpg", encoding)
	}
	if len(prepared) > MaxUploadImageBytes {
		t.Fatalf("compressed image = %d bytes", len(prepared))
	}
	if base64.StdEncoding.EncodedLen(len(prepared)) > MaxEncodedImageBytes {
		t.Fatal("base64 result exceeds limit")
	}
}
