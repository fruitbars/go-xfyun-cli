package media

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Info describes the first audio stream in a media file.
type Info struct {
	Path          string `json:"path"`
	Format        string `json:"format,omitempty"`
	Codec         string `json:"codec,omitempty"`
	DurationMS    int64  `json:"duration_ms,omitempty"`
	SampleRate    int    `json:"sample_rate,omitempty"`
	Channels      int    `json:"channels,omitempty"`
	ChannelLayout string `json:"channel_layout,omitempty"`
	Bitrate       int64  `json:"bitrate,omitempty"`
	SizeBytes     int64  `json:"size_bytes,omitempty"`
}

// ConvertOptions controls audio stream conversion. Zero values preserve the
// source property where the selected output format permits it.
type ConvertOptions struct {
	Channels   int
	SampleRate int
	Bitrate    string
	Format     string
}

func ResolveFFmpeg() (string, error) {
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

func ResolveFFprobe() (string, error) {
	if configured := os.Getenv("XFYUN_FFPROBE_PATH"); configured != "" {
		info, err := os.Stat(configured)
		if err != nil || info.IsDir() {
			return "", fmt.Errorf("XFYUN_FFPROBE_PATH does not name an executable file")
		}
		return configured, nil
	}
	path, err := exec.LookPath("ffprobe")
	if err != nil {
		return "", fmt.Errorf("ffprobe was not found; install ffprobe or set XFYUN_FFPROBE_PATH")
	}
	return path, nil
}

func Probe(ctx context.Context, inputPath string) (Info, error) {
	absolute, err := filepath.Abs(inputPath)
	if err != nil {
		return Info{}, fmt.Errorf("resolve media input: %w", err)
	}
	stat, err := os.Stat(absolute)
	if err != nil {
		return Info{}, fmt.Errorf("stat media input: %w", err)
	}
	if stat.IsDir() || stat.Size() == 0 {
		return Info{}, fmt.Errorf("media input must be a non-empty file")
	}
	ffprobe, err := ResolveFFprobe()
	if err != nil {
		return probeWithFFmpeg(ctx, absolute, stat.Size(), err)
	}
	args := []string{"-v", "error", "-select_streams", "a:0", "-show_entries", "stream=codec_name,sample_rate,channels,channel_layout,bit_rate:format=format_name,duration,bit_rate", "-of", "json", absolute}
	output, err := exec.CommandContext(ctx, ffprobe, args...).CombinedOutput()
	if err != nil {
		return probeWithFFmpeg(ctx, absolute, stat.Size(), fmt.Errorf("probe media: %w: %s", err, strings.TrimSpace(string(output))))
	}
	var decoded struct {
		Streams []struct {
			Codec         string `json:"codec_name"`
			SampleRate    string `json:"sample_rate"`
			Channels      int    `json:"channels"`
			ChannelLayout string `json:"channel_layout"`
			Bitrate       string `json:"bit_rate"`
		} `json:"streams"`
		Format struct {
			Name     string `json:"format_name"`
			Duration string `json:"duration"`
			Bitrate  string `json:"bit_rate"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &decoded); err != nil {
		return Info{}, fmt.Errorf("decode media probe: %w", err)
	}
	if len(decoded.Streams) == 0 {
		return Info{}, fmt.Errorf("media contains no audio stream")
	}
	stream := decoded.Streams[0]
	info := Info{Path: absolute, Format: decoded.Format.Name, Codec: stream.Codec, Channels: stream.Channels, ChannelLayout: stream.ChannelLayout, SizeBytes: stat.Size()}
	info.SampleRate = int(parseInt(stream.SampleRate))
	info.Bitrate = parseInt(stream.Bitrate)
	if info.Bitrate == 0 {
		info.Bitrate = parseInt(decoded.Format.Bitrate)
	}
	if decoded.Format.Duration != "" {
		if seconds, parseErr := strconv.ParseFloat(decoded.Format.Duration, 64); parseErr == nil {
			info.DurationMS = int64(seconds * 1000)
		}
	}
	return info, nil
}

var (
	ffmpegDurationPattern = regexp.MustCompile(`Duration:\s*(\d+):(\d+):(\d+(?:\.\d+)?)`)
	ffmpegInputPattern    = regexp.MustCompile(`Input #\d+,\s*([^,\s]+)`)
	ffmpegAudioPattern    = regexp.MustCompile(`Audio:\s*([^,\s]+)(?:\s+\([^)]*\))?,\s*(\d+)\s*Hz,\s*([^,\s]+)`)
	ffmpegBitratePattern  = regexp.MustCompile(`(?:,\s*|\s)(\d+)\s*kb/s`)
)

func probeWithFFmpeg(ctx context.Context, inputPath string, size int64, probeErr error) (Info, error) {
	ffmpeg, err := ResolveFFmpeg()
	if err != nil {
		return Info{}, probeErr
	}
	output, _ := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-i", inputPath).CombinedOutput()
	text := string(output)
	audio := ffmpegAudioPattern.FindStringSubmatch(text)
	if audio == nil {
		return Info{}, probeErr
	}
	info := Info{Path: inputPath, SizeBytes: size, Codec: audio[1], SampleRate: int(parseInt(audio[2]))}
	channels := strings.TrimSpace(audio[3])
	switch channels {
	case "mono":
		info.Channels = 1
	case "stereo":
		info.Channels = 2
	default:
		if fields := strings.Fields(channels); len(fields) > 0 {
			info.Channels = int(parseInt(fields[0]))
		}
	}
	if format := ffmpegInputPattern.FindStringSubmatch(text); format != nil {
		info.Format = format[1]
	}
	if duration := ffmpegDurationPattern.FindStringSubmatch(text); duration != nil {
		hours := parseInt(duration[1])
		minutes := parseInt(duration[2])
		seconds, _ := strconv.ParseFloat(duration[3], 64)
		info.DurationMS = (hours*3600+minutes*60)*1000 + int64(seconds*1000)
	}
	if bitrate := ffmpegBitratePattern.FindStringSubmatch(text); bitrate != nil {
		info.Bitrate = parseInt(bitrate[1]) * 1000
	}
	return info, nil
}

func Convert(ctx context.Context, inputPath, outputPath string, opts ConvertOptions) error {
	if opts.Channels != 0 && opts.Channels != 1 && opts.Channels != 2 {
		return fmt.Errorf("audio channels must be 1 or 2")
	}
	if opts.SampleRate < 0 {
		return fmt.Errorf("audio sample rate must not be negative")
	}
	if opts.Bitrate != "" {
		if strings.TrimSpace(opts.Bitrate) == "" {
			return fmt.Errorf("audio bitrate must not be empty")
		}
	}
	if opts.Channels == 0 && opts.SampleRate == 0 && opts.Bitrate == "" {
		return fmt.Errorf("provide at least one audio conversion option")
	}
	ffmpeg, err := ResolveFFmpeg()
	if err != nil {
		return err
	}
	absoluteInput, err := filepath.Abs(inputPath)
	if err != nil {
		return fmt.Errorf("resolve media input: %w", err)
	}
	absoluteOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("resolve media output: %w", err)
	}
	if absoluteInput == absoluteOutput {
		return fmt.Errorf("media input and output must be different files")
	}
	if err := os.MkdirAll(filepath.Dir(absoluteOutput), 0o755); err != nil {
		return fmt.Errorf("create media output directory: %w", err)
	}
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-i", absoluteInput, "-map", "0:a:0", "-vn"}
	if opts.Channels != 0 {
		args = append(args, "-ac", strconv.Itoa(opts.Channels))
	}
	if opts.SampleRate != 0 {
		args = append(args, "-ar", strconv.Itoa(opts.SampleRate))
	}
	if opts.Bitrate != "" {
		args = append(args, "-b:a", opts.Bitrate)
	}
	if opts.Format != "" {
		args = append(args, "-f", normalizeFormat(opts.Format))
	}
	args = append(args, absoluteOutput)
	if output, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("convert media: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func normalizeFormat(value string) string {
	value = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), ".")
	switch value {
	case "m4a":
		return "ipod"
	case "oga":
		return "ogg"
	default:
		return value
	}
}

func parseInt(value string) int64 {
	parsed, _ := strconv.ParseInt(value, 10, 64)
	return parsed
}
