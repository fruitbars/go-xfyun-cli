package ifasr

import (
	"strings"
	"testing"
)

func TestFormatTranscriptDefaultsToInterleavedDialogue(t *testing.T) {
	utterances := []Utterance{
		{Speaker: "1", StartMS: 100, EndMS: 200, Text: "你好"},
		{Speaker: "1", StartMS: 200, EndMS: 300, Text: "，继续。"},
		{Speaker: "2", StartMS: 400, EndMS: 500, Text: "好的。"},
	}
	got, err := FormatTranscript("", "纯文本", utterances, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "发音人1：你好，继续。\n发音人2：好的。" {
		t.Fatalf("dialogue = %q", got)
	}
}

func TestFormatTranscriptTimelineAndSubtitles(t *testing.T) {
	utterances := []Utterance{{Speaker: "1", StartMS: 1250, EndMS: 3421, Text: "你好"}}
	timeline, err := FormatTranscript("timeline", "", utterances, nil)
	if err != nil || timeline != "[00:00:01.250 --> 00:00:03.421] 发音人1：你好" {
		t.Fatalf("timeline = %q, err=%v", timeline, err)
	}
	srt, err := FormatTranscript("srt", "", utterances, nil)
	if err != nil || !strings.Contains(srt, "00:00:01,250 --> 00:00:03,421") {
		t.Fatalf("srt = %q, err=%v", srt, err)
	}
}

func TestExtractUtterancesPreservesLatticeOrder(t *testing.T) {
	orderResult := `{"lattice":[{"json_1best":"{\"st\":{\"rl\":1,\"bg\":100,\"ed\":200,\"rt\":[{\"ws\":[{\"cw\":[{\"w\":\"甲\"}]}]}]}}"},{"json_1best":"{\"st\":{\"rl\":2,\"bg\":300,\"ed\":400,\"rt\":[{\"ws\":[{\"cw\":[{\"w\":\"乙\"}]}]}]}}"}],"label":{"rl_track":[{"rl":1,"track":"L"}]}}`
	utterances, err := ExtractUtterances(orderResult)
	if err != nil {
		t.Fatal(err)
	}
	if len(utterances) != 2 || utterances[0].Text != "甲" || utterances[1].Text != "乙" || utterances[0].Track != "L" {
		t.Fatalf("utterances = %+v", utterances)
	}
}
