package tts

import "testing"

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
