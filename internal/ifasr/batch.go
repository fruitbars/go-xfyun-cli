package ifasr

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	MaxAudioBytes      int64 = 500 * 1024 * 1024
	MaxAudioDurationMS int64 = 5 * 60 * 60 * 1000
	targetPartBytes    int64 = 400 * 1024 * 1024
	targetPartMS       int64 = 4 * 60 * 60 * 1000
)

var durationPattern = regexp.MustCompile(`Duration:\s*(\d+):(\d+):(\d+(?:\.\d+)?)`)

type BatchPart struct {
	Index      int    `json:"index"`
	Size       int64  `json:"size_bytes"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	Result     Result `json:"result"`
}

type BatchResult struct {
	Split      bool                `json:"split"`
	Parts      []BatchPart         `json:"parts"`
	Transcript string              `json:"transcript,omitempty"`
	Speakers   []SpeakerTranscript `json:"speakers,omitempty"`
}

type audioPart struct {
	path       string
	size       int64
	durationMS int64
}

// TranscribeFile transparently divides over-limit audio into valid media files,
// submits every part, and combines completed transcripts in source order.
func (c *Client) TranscribeFile(ctx context.Context, inputPath string, opts Options) (BatchResult, error) {
	parts, split, cleanup, err := prepareAudioParts(ctx, inputPath, opts.DurationMS)
	if err != nil {
		return BatchResult{}, err
	}
	defer cleanup()

	batch := BatchResult{Split: split, Parts: make([]BatchPart, 0, len(parts))}
	for index, part := range parts {
		file, err := os.Open(part.path)
		if err != nil {
			return batch, fmt.Errorf("open IFASR part %d/%d: %w", index+1, len(parts), err)
		}
		partOptions := opts
		partOptions.DurationMS = part.durationMS
		partOptions.NoWait = true
		// net/http closes a request body when the request finishes. Hide the
		// file's Close method so this function retains ownership and can report
		// its own close failure exactly once.
		result, transcribeErr := c.Transcribe(ctx, io.LimitReader(file, part.size), filepath.Base(part.path), part.size, partOptions)
		closeErr := file.Close()
		if transcribeErr != nil {
			return batch, fmt.Errorf("transcribe IFASR part %d/%d: %w", index+1, len(parts), transcribeErr)
		}
		if closeErr != nil {
			return batch, fmt.Errorf("close IFASR part %d/%d: %w", index+1, len(parts), closeErr)
		}
		batch.Parts = append(batch.Parts, BatchPart{
			Index: index + 1, Size: part.size, DurationMS: part.durationMS, Result: result,
		})
	}
	if opts.NoWait {
		return batch, nil
	}

	transcripts := make([]string, 0, len(batch.Parts))
	var speakers []SpeakerTranscript
	for index := range batch.Parts {
		result, err := c.Wait(ctx, batch.Parts[index].Result.OrderID, batch.Parts[index].Result.SignatureRand, opts)
		if err != nil {
			return batch, fmt.Errorf("wait for IFASR part %d/%d: %w", index+1, len(batch.Parts), err)
		}
		batch.Parts[index].Result = result
		if result.Transcript != "" {
			transcripts = append(transcripts, result.Transcript)
		}
		speakers = mergeSpeakerTranscripts(speakers, result.Speakers)
	}
	batch.Transcript = strings.Join(transcripts, "\n")
	batch.Speakers = speakers
	return batch, nil
}

func mergeSpeakerTranscripts(current, next []SpeakerTranscript) []SpeakerTranscript {
	if len(next) == 0 {
		return current
	}
	indices := make(map[string]int, len(current))
	for index, speaker := range current {
		indices[speaker.Speaker+"\x00"+speaker.Track] = index
	}
	for _, speaker := range next {
		key := speaker.Speaker + "\x00" + speaker.Track
		index, ok := indices[key]
		if !ok {
			indices[key] = len(current)
			current = append(current, speaker)
			continue
		}
		if current[index].Transcript != "" && speaker.Transcript != "" {
			current[index].Transcript += "\n"
		}
		current[index].Transcript += speaker.Transcript
		current[index].Segments = append(current[index].Segments, speaker.Segments...)
	}
	return current
}

func prepareAudioParts(ctx context.Context, inputPath string, declaredDurationMS int64) ([]audioPart, bool, func(), error) {
	if err := validateFileName(inputPath); err != nil {
		return nil, false, func() {}, err
	}
	info, err := os.Stat(inputPath)
	if err != nil {
		return nil, false, func() {}, fmt.Errorf("stat audio input: %w", err)
	}
	if info.Size() <= 0 {
		return nil, false, func() {}, fmt.Errorf("audio file is empty")
	}
	durationMS := declaredDurationMS
	ffmpegPath, ffmpegErr := resolveFFmpeg()
	if durationMS == 0 && ffmpegErr == nil {
		durationMS, err = probeDuration(ctx, ffmpegPath, inputPath)
		if err != nil {
			durationMS = 0
		}
	}
	if !needsSplit(info.Size(), durationMS) {
		return []audioPart{{path: inputPath, size: info.Size(), durationMS: durationMS}}, false, func() {}, nil
	}
	if ffmpegErr != nil {
		return nil, false, func() {}, fmt.Errorf("audio exceeds the 5 hour or 500 MiB IFASR limit and the bundled splitter is unavailable: %w", ffmpegErr)
	}
	if durationMS == 0 {
		return nil, false, func() {}, fmt.Errorf("audio exceeds 500 MiB and its duration could not be determined")
	}

	temporary, err := os.MkdirTemp("", "xfyun-ifasr-parts-*")
	if err != nil {
		return nil, false, func() {}, fmt.Errorf("create IFASR split directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(temporary) }
	partMS := segmentDuration(info.Size(), durationMS)
	for attempt := 0; attempt < 4; attempt++ {
		if err := clearDirectory(temporary); err != nil {
			cleanup()
			return nil, false, func() {}, err
		}
		if err := splitAudio(ctx, ffmpegPath, inputPath, temporary, partMS); err != nil {
			cleanup()
			return nil, false, func() {}, err
		}
		parts, err := collectAudioParts(ctx, ffmpegPath, temporary, filepath.Ext(inputPath))
		if err != nil {
			cleanup()
			return nil, false, func() {}, err
		}
		valid := len(parts) > 1
		for _, part := range parts {
			if needsSplit(part.size, part.durationMS) {
				valid = false
				break
			}
		}
		if valid {
			return parts, true, cleanup, nil
		}
		partMS /= 2
		if partMS < 60*1000 {
			break
		}
	}
	cleanup()
	return nil, false, func() {}, fmt.Errorf("unable to split audio below the IFASR 5 hour and 500 MiB limits")
}

func needsSplit(size, durationMS int64) bool {
	return size > MaxAudioBytes || durationMS > MaxAudioDurationMS
}

func segmentDuration(size, durationMS int64) int64 {
	target := targetPartMS
	if size > targetPartBytes && durationMS > 0 {
		bySize := durationMS * targetPartBytes / size
		if bySize < target {
			target = bySize
		}
	}
	if target < 60*1000 {
		return 60 * 1000
	}
	return target
}

func resolveFFmpeg() (string, error) {
	if configured := os.Getenv("XFYUN_FFMPEG_PATH"); configured != "" {
		info, err := os.Stat(configured)
		if err != nil || info.IsDir() {
			return "", fmt.Errorf("XFYUN_FFMPEG_PATH does not name an executable file")
		}
		return configured, nil
	}
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("ffmpeg was not found")
	}
	return path, nil
}

func probeDuration(ctx context.Context, ffmpegPath, inputPath string) (int64, error) {
	args := []string{"-hide_banner"}
	if strings.EqualFold(filepath.Ext(inputPath), ".pcm") {
		args = append(args, "-f", "s16le", "-ar", "16000", "-ac", "1")
	}
	args = append(args, "-i", inputPath)
	output, _ := exec.CommandContext(ctx, ffmpegPath, args...).CombinedOutput()
	return parseDuration(output)
}

func parseDuration(output []byte) (int64, error) {
	match := durationPattern.FindSubmatch(output)
	if match == nil {
		return 0, fmt.Errorf("ffmpeg did not report audio duration")
	}
	hours, _ := strconv.ParseInt(string(match[1]), 10, 64)
	minutes, _ := strconv.ParseInt(string(match[2]), 10, 64)
	seconds, err := strconv.ParseFloat(string(match[3]), 64)
	if err != nil {
		return 0, fmt.Errorf("parse ffmpeg duration: %w", err)
	}
	return (hours*3600+minutes*60)*1000 + int64(seconds*1000), nil
}

func splitAudio(ctx context.Context, ffmpegPath, inputPath, directory string, partMS int64) error {
	extension := strings.ToLower(filepath.Ext(inputPath))
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y"}
	if extension == ".pcm" {
		args = append(args, "-f", "s16le", "-ar", "16000", "-ac", "1")
	}
	args = append(args, "-i", inputPath, "-map", "0:a:0", "-c", "copy", "-f", "segment",
		"-segment_time", strconv.FormatFloat(float64(partMS)/1000, 'f', 3, 64), "-reset_timestamps", "1")
	if format := segmentFormat(extension); format != "" {
		args = append(args, "-segment_format", format)
	}
	args = append(args, filepath.Join(directory, "part-%04d"+extension))
	if output, err := exec.CommandContext(ctx, ffmpegPath, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("split IFASR audio: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func segmentFormat(extension string) string {
	switch extension {
	case ".pcm":
		return "s16le"
	case ".opus":
		return "opus"
	case ".speex":
		return "spx"
	default:
		return ""
	}
}

func collectAudioParts(ctx context.Context, ffmpegPath, directory, extension string) ([]audioPart, error) {
	paths, err := filepath.Glob(filepath.Join(directory, "part-*"+strings.ToLower(extension)))
	if err != nil || len(paths) == 0 {
		return nil, fmt.Errorf("audio splitter produced no parts")
	}
	sort.Strings(paths)
	parts := make([]audioPart, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat split audio: %w", err)
		}
		durationMS, _ := probeDuration(ctx, ffmpegPath, path)
		if info.Size() == 0 || durationMS == 0 {
			continue
		}
		parts = append(parts, audioPart{path: path, size: info.Size(), durationMS: durationMS})
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("audio splitter produced no non-empty parts")
	}
	return parts, nil
}

func clearDirectory(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read IFASR split directory: %w", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(directory, entry.Name())); err != nil {
			return fmt.Errorf("clear IFASR split directory: %w", err)
		}
	}
	return nil
}
