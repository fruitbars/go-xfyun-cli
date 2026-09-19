package ocr

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"strings"
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

func TestDefaultMarkdownElementOptionUsesHTMLTablesAndMathML(t *testing.T) {
	if !strings.Contains(DefaultMarkdownElementOption, "table_format=0") {
		t.Fatalf("default markdown options do not enable HTML tables: %q", DefaultMarkdownElementOption)
	}
	if !strings.Contains(DefaultMarkdownElementOption, "formula_format=0") {
		t.Fatalf("default markdown options do not keep HTML-table formulas as MathML: %q", DefaultMarkdownElementOption)
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
	formats := ExtractResultFormats([]byte(`{"document":[{"name":"markdown","value":"# 标题"},{"name":"sed","value":[{"type":"paragraph","text":["内容"]}]}],"image":[]}`))
	if formats.Markdown != "# 标题" || len(formats.SED) != 1 || formats.SED[0]["type"] != "paragraph" {
		t.Fatalf("formats = %+v", formats)
	}
	if got := ExtractResultFormats([]byte("plain text")); got.Markdown != "" || len(got.SED) != 0 {
		t.Fatalf("plain response formats = %+v", got)
	}
}

func TestAnnotateImageDrawsSelectedTypes(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 80))
	for index := range img.Pix {
		img.Pix[index] = 0xff
	}
	var source bytes.Buffer
	if err := png.Encode(&source, img); err != nil {
		t.Fatal(err)
	}
	response := []byte(`{"image":[{"width":100,"height":80,"content":[{"type":"paragraph","coord":[{"x":10,"y":10},{"x":60,"y":10},{"x":60,"y":30},{"x":10,"y":30}]},{"type":"textline","coord":[{"x":10,"y":40},{"x":60,"y":40},{"x":60,"y":50},{"x":10,"y":50}]}]}]}`)
	annotated, count, err := AnnotateImage(source.Bytes(), response, "paragraph")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("annotation count = %d, want 1", count)
	}
	decoded, _, err := image.Decode(bytes.NewReader(annotated))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.At(10, 10) == img.At(10, 10) {
		t.Fatal("annotation did not change the selected box")
	}
	if decoded.At(10, 40) != img.At(10, 40) {
		t.Fatal("unselected type was annotated")
	}
	path := filepath.Join(t.TempDir(), "annotation.png")
	if err := os.WriteFile(path, annotated, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := AnnotateImage(source.Bytes(), []byte("not JSON"), "paragraph"); err == nil {
		t.Fatal("expected invalid OCR result error")
	}
}

func TestParseAnnotationTypesSupportsDocumentedAndHelperTypes(t *testing.T) {
	types, err := ParseAnnotationTypes("page,information_bar,fingerprint,cell,item,textline")
	if err != nil {
		t.Fatal(err)
	}
	if len(types) != 6 {
		t.Fatalf("types = %v", types)
	}
	all, err := ParseAnnotationTypes("all")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(SupportedAnnotationTypes) {
		t.Fatalf("all type count = %d, want %d", len(all), len(SupportedAnnotationTypes))
	}
	if _, err := ParseAnnotationTypes("not_a_layout_type"); err == nil {
		t.Fatal("expected unsupported annotation type error")
	}
	if _, _, err := AnnotateImage([]byte("not an image"), []byte(`{}`), "paragraph"); err == nil {
		t.Fatal("expected invalid source image error")
	}
}

func TestAnnotateImageDrawsEverySupportedType(t *testing.T) {
	const (
		width  = 640
		height = 240
	)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for index := range img.Pix {
		img.Pix[index] = 0xff
	}
	var source bytes.Buffer
	if err := png.Encode(&source, img); err != nil {
		t.Fatal(err)
	}

	content := make([]map[string]any, 0, len(SupportedAnnotationTypes))
	for index, typeName := range SupportedAnnotationTypes {
		x := 10 + (index%7)*90
		y := 10 + (index/7)*55
		content = append(content, map[string]any{
			"type": typeName,
			"coord": []map[string]int{
				{"x": x, "y": y}, {"x": x + 70, "y": y},
				{"x": x + 70, "y": y + 35}, {"x": x, "y": y + 35},
			},
		})
	}
	response, err := json.Marshal(map[string]any{
		"image": []map[string]any{{"width": width, "height": height, "content": content}},
	})
	if err != nil {
		t.Fatal(err)
	}

	annotated, count, err := AnnotateImage(source.Bytes(), response, "all")
	if err != nil {
		t.Fatal(err)
	}
	if count != len(SupportedAnnotationTypes) {
		t.Fatalf("annotation count = %d, want %d", count, len(SupportedAnnotationTypes))
	}
	if _, _, err := image.Decode(bytes.NewReader(annotated)); err != nil {
		t.Fatalf("decode annotated image: %v", err)
	}
}

func TestTransformAnnotationPointRestoresQuarterTurn(t *testing.T) {
	tests := []struct {
		angle float64
		want  image.Point
	}{{0, image.Pt(10, 20)}, {90, image.Pt(20, 90)}, {180, image.Pt(70, 80)}, {270, image.Pt(60, 10)}}
	for _, test := range tests {
		point := transformAnnotationPoint(image.Pt(10, 20), annotationGeometry{Width: 80, Height: 100, Angle: test.angle}, 80, 100)
		if point != test.want {
			t.Fatalf("angle %.0f transformed point = %v, want %v", test.angle, point, test.want)
		}
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
