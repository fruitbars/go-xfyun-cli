package tts

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestValidateOptions(t *testing.T) {
	valid := Options{
		Speed: 50, Volume: 50, Pitch: 50, Encoding: "lame", SampleRate: 24000,
		OralLevel: "mid", SparkAssist: 1, StopSplit: 0, Remain: 0,
		BackgroundSound: 0, EnglishReading: 2, NumberReading: 3,
		ReturnPronounce: 1, VisibleWatermark: 2, ImplicitWatermark: true,
	}
	if err := validateOptions(valid); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.Speed = 101
	if err := validateOptions(invalid); err == nil {
		t.Fatal("expected speed validation error")
	}
	invalid = valid
	invalid.Encoding = "raw"
	if err := validateOptions(invalid); err == nil {
		t.Fatal("expected implicit watermark encoding error")
	}
}

func TestSplitTextPreservesUTF8AndPrefersSentenceBoundaries(t *testing.T) {
	text := strings.Repeat("你好", 10) + "。" + strings.Repeat("世界", 10)
	parts := SplitText(text, 64)
	if len(parts) < 2 {
		t.Fatalf("parts = %d, want at least 2", len(parts))
	}
	if strings.Join(parts, "") != text {
		t.Fatal("split text did not preserve input")
	}
	for _, part := range parts {
		if len([]byte(part)) > 64 {
			t.Fatalf("part has %d bytes", len([]byte(part)))
		}
		if !utf8.ValidString(part) {
			t.Fatal("part is not valid UTF-8")
		}
	}
	if !strings.HasSuffix(parts[0], "。") {
		t.Fatalf("first part %q does not end at sentence boundary", parts[0])
	}
}

func TestSplitTextHandlesSingleRuneLimit(t *testing.T) {
	parts := SplitText("你好", 1)
	if strings.Join(parts, "") != "你好" || len(parts) != 2 {
		t.Fatalf("parts = %#v", parts)
	}
}
