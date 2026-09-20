package ifasr

import (
	"fmt"
	"strings"
)

// SupportedTranscriptFormats is the stable presentation vocabulary for IFASR.
var SupportedTranscriptFormats = []string{"dialogue", "text", "timeline", "speaker_grouped", "srt", "vtt"}

// FormatTranscript renders one of the human-facing transcript formats. The
// plain transcript remains available as the text format; dialogue is the
// default because speaker order is usually the most useful presentation.
func FormatTranscript(format, transcript string, utterances []Utterance, speakers []SpeakerTranscript) (string, error) {
	if format == "" {
		format = "dialogue"
	}
	switch format {
	case "text":
		return transcript, nil
	case "dialogue":
		return formatDialogue(utterances, transcript, false), nil
	case "timeline":
		return formatDialogue(utterances, transcript, true), nil
	case "speaker_grouped":
		return formatGrouped(speakers, transcript), nil
	case "srt", "vtt":
		return formatSubtitles(format, utterances, transcript), nil
	default:
		return "", fmt.Errorf("unsupported transcript format %q; use dialogue, text, timeline, speaker_grouped, srt, or vtt", format)
	}
}

func formatDialogue(utterances []Utterance, fallback string, timestamps bool) string {
	if len(utterances) == 0 {
		return fallback
	}
	hasSpeaker := false
	for _, utterance := range utterances {
		if utterance.Speaker != "" || utterance.Track != "" {
			hasSpeaker = true
			break
		}
	}
	if !hasSpeaker {
		return fallback
	}
	utterances = coalesceUtterances(utterances)
	var output strings.Builder
	for _, utterance := range utterances {
		if timestamps && (utterance.StartMS != 0 || utterance.EndMS != 0) {
			fmt.Fprintf(&output, "[%s --> %s] ", formatClock(utterance.StartMS), formatClock(utterance.EndMS))
		}
		if label := utteranceLabel(utterance); label != "" {
			output.WriteString(label)
			output.WriteString("：")
		}
		output.WriteString(utterance.Text)
		output.WriteByte('\n')
	}
	return strings.TrimSuffix(output.String(), "\n")
}

func formatGrouped(speakers []SpeakerTranscript, fallback string) string {
	if len(speakers) == 0 {
		return fallback
	}
	var output strings.Builder
	for _, speaker := range speakers {
		fmt.Fprintf(&output, "## %s\n%s\n\n", speakerLabel(speaker.Speaker, speaker.Track), speaker.Transcript)
	}
	return strings.TrimSpace(output.String())
}

func formatSubtitles(format string, utterances []Utterance, fallback string) string {
	if len(utterances) == 0 {
		return fallback
	}
	utterances = coalesceUtterances(utterances)
	var output strings.Builder
	if format == "vtt" {
		output.WriteString("WEBVTT\n\n")
	}
	for index, utterance := range utterances {
		if utterance.StartMS == 0 && utterance.EndMS == 0 {
			return formatDialogue(utterances, fallback, false)
		}
		if format == "srt" {
			fmt.Fprintf(&output, "%d\n", index+1)
		}
		fmt.Fprintf(&output, "%s --> %s\n%s：%s\n\n", formatSubtitleClock(utterance.StartMS, format), formatSubtitleClock(utterance.EndMS, format), utteranceLabel(utterance), utterance.Text)
	}
	return strings.TrimSpace(output.String())
}

func coalesceUtterances(input []Utterance) []Utterance {
	output := make([]Utterance, 0, len(input))
	for _, current := range input {
		if current.Text == "" {
			continue
		}
		if len(output) > 0 {
			previous := &output[len(output)-1]
			if previous.Speaker == current.Speaker && previous.Track == current.Track {
				previous.Text += current.Text
				if current.EndMS != 0 {
					previous.EndMS = current.EndMS
				}
				continue
			}
		}
		output = append(output, current)
	}
	return output
}

func utteranceLabel(utterance Utterance) string {
	return speakerLabel(utterance.Speaker, utterance.Track)
}

func speakerLabel(speaker, track string) string {
	if track == "L" {
		return "左声道"
	}
	if track == "R" {
		return "右声道"
	}
	if speaker == "" {
		return ""
	}
	return "发音人" + speaker
}

func formatClock(milliseconds int64) string {
	if milliseconds < 0 {
		milliseconds = 0
	}
	hours := milliseconds / (60 * 60 * 1000)
	minutes := (milliseconds / (60 * 1000)) % 60
	seconds := (milliseconds / 1000) % 60
	millis := milliseconds % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, millis)
}

func formatSubtitleClock(milliseconds int64, format string) string {
	value := formatClock(milliseconds)
	if format == "srt" {
		return strings.Replace(value, ".", ",", 1)
	}
	return value
}
