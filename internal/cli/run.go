package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fruitbars/go-xfyun-cli/internal/config"
	"github.com/fruitbars/go-xfyun-cli/internal/ifasr"
	"github.com/fruitbars/go-xfyun-cli/internal/ocr"
	"github.com/fruitbars/go-xfyun-cli/internal/outputfile"
	"github.com/fruitbars/go-xfyun-cli/internal/rtasr"
	"github.com/fruitbars/go-xfyun-cli/internal/tts"
	"github.com/fruitbars/go-xfyun-cli/internal/version"
)

func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stderr)
		return flag.ErrHelp
	}
	switch args[0] {
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	case "version", "--version":
		fmt.Fprintln(stdout, version.Current)
		return nil
	case "ocr":
		return runOCR(ctx, args[1:], stdin, stdout, stderr)
	case "tts":
		return runTTS(ctx, args[1:], stdin, stdout, stderr)
	case "rtasr":
		return runRTASR(ctx, args[1:], stdin, stdout, stderr)
	case "ifasr":
		return runIFASR(ctx, args[1:], stdin, stdout, stderr)
	default:
		usage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `xfyun - command-line access to XFYun large-model APIs

Usage:
  xfyun <command> [options]

Commands:
  ocr       Recognize a document image
  tts       Synthesize speech from text
  rtasr     Stream an audio file to real-time transcription
  ifasr     Upload an audio file and wait for transcription
  version   Print version

Credentials (all four commands):
  XFYUN_APP_ID, XFYUN_API_KEY, XFYUN_API_SECRET

Run "xfyun <command> --help" for command options.`)
}

func runOCR(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := newFlagSet("ocr", stderr)
	var creds config.Credentials
	credentialFlags(fs, &creds)
	input := fs.String("input", "", "image or PDF path, or - for stdin")
	encoding := fs.String("encoding", "", "optional input encoding override")
	pages := fs.String("pages", "", "PDF page selection such as 1-3,5; default all")
	pdfDPI := fs.Int("pdf-dpi", ocr.DefaultPDFDPI, "PDF rendering resolution from 72 to 300 DPI")
	resultFormat := fs.String("result-format", "json,markdown", "API result formats")
	resultOption := fs.String("result-option", "normal", "normal, normal,char, or no_line_position variants")
	markdownElements := fs.String("markdown-elements", ocr.DefaultMarkdownElementOption, "markdown element controls")
	sedElements := fs.String("sed-elements", ocr.DefaultSEDElementOption, "SED element controls")
	exifOption := fs.String("exif", "0", "parse image EXIF metadata: 0 off, 1 on")
	alphaOption := fs.String("alpha", "0", "honor transparent pixels: 0 off, 1 on")
	rotation := fs.Float64("rotation-min-angle", 5, "minimum auto-rotation angle")
	raw := fs.Bool("raw", false, "print the complete API response JSON")
	timeout := fs.Duration("timeout", 2*time.Minute, "request timeout per extracted image")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: xfyun ocr --input IMAGE_OR_PDF [options]")
		fmt.Fprintln(stderr, "Multi-page results are streamed as one JSON object per line (NDJSON).")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if *input == "" {
		return fmt.Errorf("--input is required")
	}
	creds.FromEnv()
	if err := creds.ValidateSigned(); err != nil {
		return err
	}
	inputPath, cleanup, err := prepareOCRInput(*input, stdin)
	if err != nil {
		return err
	}
	defer cleanup()
	client := ocr.Client{Credentials: creds}
	type pageResult struct {
		Type       string        `json:"type"`
		Page       int           `json:"page"`
		Image      int           `json:"image"`
		TotalPages int           `json:"total_pages"`
		DPI        int           `json:"dpi,omitempty"`
		Result     any           `json:"result,omitempty"`
		Response   *ocr.Response `json:"response,omitempty"`
	}
	encoder := json.NewEncoder(stdout)
	err = ocr.StreamPath(ctx, inputPath, *pages, *pdfDPI, func(documentImage ocr.DocumentImage) error {
		imageEncoding := documentImage.Encoding
		if *encoding != "" {
			imageEncoding = *encoding
		}
		requestCtx, cancel := context.WithTimeout(ctx, *timeout)
		decoded, response, recognizeErr := client.Recognize(requestCtx, documentImage.Data, ocr.Options{
			Encoding: imageEncoding, ResultFormat: *resultFormat, ResultOption: *resultOption,
			MarkdownElementOption: *markdownElements, SEDElementOption: *sedElements,
			ExifOption: *exifOption, AlphaOption: *alphaOption, RotationMinAngle: *rotation,
		})
		cancel()
		if recognizeErr != nil {
			return fmt.Errorf("OCR page %d image %d: %w", documentImage.Page, documentImage.Index, recognizeErr)
		}
		result := pageResult{
			Type: "page", Page: documentImage.Page, Image: documentImage.Index,
			TotalPages: documentImage.PageCount, DPI: documentImage.DPI,
		}
		if *raw {
			result.Response = response
		} else {
			var value any
			if json.Unmarshal(decoded, &value) != nil {
				value = string(decoded)
			}
			result.Result = value
		}
		if documentImage.PageCount > 1 {
			// One compact JSON value per line lets callers consume and discard
			// each completed page immediately. If a later page fails, the
			// already emitted lines remain valid and the process exits non-zero.
			return encoder.Encode(result)
		}
		if *raw {
			return writeJSON(stdout, response)
		}
		if text, ok := result.Result.(string); ok {
			_, err := fmt.Fprintln(stdout, text)
			return err
		}
		return writeJSON(stdout, result.Result)
	})
	if err != nil {
		return err
	}
	return nil
}

func runTTS(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := newFlagSet("tts", stderr)
	var creds config.Credentials
	credentialFlags(fs, &creds)
	textValue := fs.String("text", "", "text to synthesize; defaults to stdin")
	textFile := fs.String("text-file", "", "read text from a UTF-8 file")
	outputPath := fs.String("output", "", "output audio path, or - for stdout (required)")
	force := fs.Bool("force", false, "overwrite an existing output file")
	voice := fs.String("voice", "x5_lingxiaoxuan_flow", "voice ID enabled for the application")
	encoding := fs.String("encoding", "lame", "audio encoding: lame, raw, speex, opus, opus-wb, opus-swb, speex-wb")
	sampleRate := fs.Int("sample-rate", 24000, "sample rate: 8000, 16000, or 24000")
	speed := fs.Int("speed", 50, "speech speed from 0 to 100")
	volume := fs.Int("volume", 50, "volume from 0 to 100")
	pitch := fs.Int("pitch", 50, "pitch from 0 to 100")
	oralLevel := fs.String("oral-level", "mid", "oral style: low, mid, or high")
	sparkAssist := fs.Int("spark-assist", 1, "large-model oralization: 0 off, 1 on")
	remain := fs.Int("remain", 0, "preserve written form: 0 no, 1 yes")
	stopSplit := fs.Int("stop-split", 0, "disable server sentence splitting: 0 no, 1 yes")
	backgroundSound := fs.Int("background-sound", 0, "background sound: 0 off, 1 on")
	englishReading := fs.Int("english-reading", 0, "English reading: 0 auto/word fallback, 1 letters, 2 auto/letter fallback")
	numberReading := fs.Int("number-reading", 0, "number reading: 0 auto, 1 numeric, 2 digit string, 3 string preferred")
	returnPronounce := fs.Int("return-pronounce", 0, "return phoneme annotation on stderr: 0 off, 1 on")
	visibleWatermark := fs.Int("visible-watermark", 0, "audible watermark: 0 off, 1 sentence start, 2 sentence end")
	implicitWatermark := fs.Bool("implicit-watermark", false, "add an implicit watermark (lame only)")
	timeout := fs.Duration("timeout", 0, "overall request timeout; default 5 minutes per automatic text segment")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: xfyun tts (--text TEXT | --text-file FILE | < stdin) --output FILE [options]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *outputPath == "" {
		return fmt.Errorf("--output is required (use - to write audio to stdout)")
	}
	if *textValue != "" && *textFile != "" {
		return fmt.Errorf("--text and --text-file are mutually exclusive")
	}
	var source io.Reader = stdin
	if *textValue != "" {
		source = strings.NewReader(*textValue)
	} else if *textFile != "" {
		file, err := os.Open(*textFile)
		if err != nil {
			return fmt.Errorf("open text file: %w", err)
		}
		defer file.Close()
		source = file
	}
	textBytes, err := io.ReadAll(source)
	if err != nil {
		return fmt.Errorf("read text: %w", err)
	}
	creds.FromEnv()
	if err := creds.ValidateTTS(); err != nil {
		return err
	}
	var output io.Writer = stdout
	var temporary *os.File
	var destination string
	if *outputPath != "-" {
		destination, temporary, err = outputfile.Prepare(*outputPath, *force)
		if err != nil {
			return err
		}
		defer func() {
			_ = temporary.Close()
			_ = os.Remove(temporary.Name())
		}()
		output = temporary
	}
	requestTimeout := *timeout
	if requestTimeout <= 0 {
		requestTimeout = time.Duration(len(tts.SplitText(string(textBytes), tts.MaxTextBytes))) * 5 * time.Minute
	}
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	client := tts.Client{Credentials: creds}
	metadata, err := client.Synthesize(requestCtx, string(textBytes), output, tts.Options{
		Voice: *voice, Encoding: *encoding, SampleRate: *sampleRate,
		Speed: *speed, Volume: *volume, Pitch: *pitch, OralLevel: *oralLevel,
		SparkAssist: *sparkAssist, StopSplit: *stopSplit, Remain: *remain,
		BackgroundSound: *backgroundSound, EnglishReading: *englishReading,
		NumberReading: *numberReading, ReturnPronounce: *returnPronounce,
		VisibleWatermark: *visibleWatermark, ImplicitWatermark: *implicitWatermark,
	})
	if err != nil {
		return err
	}
	if temporary != nil {
		if err := temporary.Close(); err != nil {
			return fmt.Errorf("close audio output: %w", err)
		}
		if err := outputfile.Commit(temporary.Name(), destination, *force); err != nil {
			return fmt.Errorf("commit audio output: %w", err)
		}
	}
	if *outputPath != "-" {
		fmt.Fprintf(stderr, "wrote %d bytes to %s (%d segment(s), sid: %s)\n", metadata.Bytes, *outputPath, metadata.Segments, metadata.SID)
	}
	if metadata.Pronunciation != "" {
		fmt.Fprintf(stderr, "pronunciation: %s\n", metadata.Pronunciation)
	}
	return nil
}

func runRTASR(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := newFlagSet("rtasr", stderr)
	var creds config.Credentials
	credentialFlags(fs, &creds)
	inputPath := fs.String("input", "", "PCM/Opus/Speex audio path, or - for stdin")
	language := fs.String("language", "autodialect", "autodialect or autominor")
	recognizedLanguage := fs.String("recognized-language", "", "comma-separated target languages for autominor")
	audioEncoding := fs.String("audio-encoding", "pcm_s16le", "pcm_s16le, opus-wb, speex-7, or speex-10")
	sampleRate := fs.Int("sample-rate", 16000, "PCM sample rate: 8000 or 16000")
	chunkSize := fs.Int("chunk-size", 1280, "audio bytes per websocket frame")
	interval := fs.Duration("interval", 40*time.Millisecond, "delay between audio frames")
	roleType := fs.Int("role-type", 0, "speaker separation: 0 off, 2 on")
	featureIDs := fs.String("feature-ids", "", "comma-separated registered voiceprint IDs")
	domain := fs.String("domain", "", "domain optimization, such as finance or medical")
	speakerMatch := fs.Bool("speaker-match", false, "restrict separated roles to registered feature IDs")
	keepPunctuation := fs.Bool("punctuation", true, "keep punctuation in transcription")
	vadMode := fs.Int("vad-mode", 0, "VAD field mode: 0 default, 1 far field, 2 near field")
	jsonl := fs.Bool("jsonl", false, "emit every API event as JSON Lines")
	timeout := fs.Duration("timeout", 8*time.Hour+5*time.Minute, "overall timeout")
	var extra keyValueFlags
	fs.Var(&extra, "param", "extra API query parameter key=value (repeatable)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: xfyun rtasr --input AUDIO [options]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inputPath == "" {
		return fmt.Errorf("--input is required")
	}
	creds.FromEnv()
	if err := creds.ValidateSigned(); err != nil {
		return err
	}
	input, closeInput, err := openInput(*inputPath, stdin)
	if err != nil {
		return err
	}
	defer closeInput()
	requestCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	var callback func(json.RawMessage) error
	if *jsonl {
		callback = func(message json.RawMessage) error {
			if _, err := stdout.Write(message); err != nil {
				return err
			}
			_, err := io.WriteString(stdout, "\n")
			return err
		}
	}
	client := rtasr.Client{Credentials: creds}
	result, err := client.Transcribe(requestCtx, input, rtasr.Options{
		Language: *language, RecognizedLanguage: *recognizedLanguage,
		AudioEncoding: *audioEncoding, SampleRate: *sampleRate,
		ChunkSize: *chunkSize, Interval: *interval, RoleType: *roleType,
		FeatureIDs: *featureIDs, Domain: *domain, SpeakerMatch: *speakerMatch,
		KeepPunctuation: keepPunctuation, VADMode: *vadMode, Extra: extra,
	}, callback)
	if err != nil {
		return err
	}
	if !*jsonl {
		fmt.Fprintln(stdout, result.Transcript)
	}
	return nil
}

func runIFASR(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := newFlagSet("ifasr", stderr)
	var creds config.Credentials
	credentialFlags(fs, &creds)
	inputPath := fs.String("input", "", "audio file path (upload mode)")
	orderID := fs.String("order-id", "", "existing order ID (query mode)")
	signatureRandom := fs.String("signature-random", "", "signature random returned during upload (query mode)")
	language := fs.String("language", "autodialect", "autodialect or autominor")
	durationMS := fs.Int64("duration-ms", 0, "known audio duration in milliseconds; zero auto-detects")
	domain := fs.String("domain", "", "domain optimization, such as finance or medical")
	roleType := fs.Int("role-type", 0, "speaker separation: 0 off, 1 generic, 3 voiceprint")
	roleNum := fs.Int("role-num", 0, "expected speaker count from 0 to 10")
	featureIDs := fs.String("feature-ids", "", "comma-separated registered voiceprint IDs")
	callbackURL := fs.String("callback-url", "", "GET callback URL invoked after the order finishes")
	smooth := fs.Bool("smooth", true, "return a smoothed transcript in addition to the original")
	colloquial := fs.Bool("colloquial", false, "apply colloquial normalization")
	ifasrVADMode := fs.Int("vad-mode", 0, "VAD field mode: 0 default, 1 far field, 2 near field")
	cantoneseScript := fs.Int("cantonese-script", -1, "Cantonese script: -1 default, 0 simplified, 1 traditional")
	analysis := fs.Bool("language-analysis", false, "enable spoken-language analysis (multilingual entitlement required)")
	resultType := fs.String("result-type", "transfer", "transfer, analysis, or comma-separated combination")
	pollInterval := fs.Duration("poll-interval", 2*time.Second, "result polling interval")
	maxWait := fs.Duration("max-wait", 30*time.Minute, "maximum wait for a completed order")
	noWait := fs.Bool("no-wait", false, "upload or query once and print machine-readable state")
	raw := fs.Bool("raw", false, "print the complete result as JSON")
	var extra keyValueFlags
	fs.Var(&extra, "param", "extra upload query parameter key=value (repeatable)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: xfyun ifasr --input AUDIO [options]\n   or: xfyun ifasr --order-id ID --signature-random RANDOM [options]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *analysis && *resultType == "transfer" {
		*resultType = "transfer,analysis"
	}
	if (*inputPath == "") == (*orderID == "") {
		return fmt.Errorf("provide exactly one of --input or --order-id")
	}
	if *orderID != "" && *signatureRandom == "" {
		return fmt.Errorf("--signature-random is required with --order-id")
	}
	creds.FromEnv()
	if err := creds.ValidateSigned(); err != nil {
		return err
	}
	client := ifasr.Client{Credentials: creds}
	var cantoneseScriptOption *int
	if *cantoneseScript >= 0 {
		cantoneseScriptOption = cantoneseScript
	}
	opts := ifasr.Options{
		Language: *language, DurationMS: *durationMS, Domain: *domain,
		CallbackURL: *callbackURL, RoleType: *roleType, RoleNum: *roleNum, FeatureIDs: *featureIDs,
		Smooth: smooth, Colloquial: colloquial, VADMode: *ifasrVADMode,
		CantoneseScript: cantoneseScriptOption, Analysis: *analysis,
		ResultType: *resultType, PollInterval: *pollInterval, MaxWait: *maxWait,
		Extra: extra, NoWait: *noWait, SignatureRand: *signatureRandom,
	}
	var result ifasr.Result
	var batch ifasr.BatchResult
	var err error
	if *inputPath != "" {
		batch, err = client.TranscribeFile(ctx, *inputPath, opts)
		if len(batch.Parts) == 1 {
			result = batch.Parts[0].Result
		}
	} else if *noWait {
		var response ifasr.Response
		response, err = client.Query(ctx, *orderID, *signatureRandom, *resultType)
		result = ifasr.Result{OrderID: *orderID, SignatureRand: *signatureRandom, Status: response.Content.OrderInfo.Status, Response: response}
	} else {
		result, err = client.Wait(ctx, *orderID, *signatureRandom, opts)
	}
	if err != nil {
		return err
	}
	if *inputPath != "" && batch.Split {
		if *raw || *noWait {
			return writeJSON(stdout, batch)
		}
		fmt.Fprintln(stdout, batch.Transcript)
		return nil
	}
	if *raw || *noWait {
		return writeJSON(stdout, result)
	}
	fmt.Fprintln(stdout, result.Transcript)
	return nil
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func credentialFlags(fs *flag.FlagSet, creds *config.Credentials) {
	fs.StringVar(&creds.AppID, "app-id", "", "XFYun AppID (prefer XFYUN_APP_ID)")
	fs.StringVar(&creds.APIKey, "api-key", "", "XFYun APIKey (prefer XFYUN_API_KEY)")
	fs.StringVar(&creds.APISecret, "api-secret", "", "XFYun APISecret (prefer XFYUN_API_SECRET)")
}

type keyValueFlags map[string]string

func (f *keyValueFlags) String() string {
	if f == nil {
		return ""
	}
	pairs := make([]string, 0, len(*f))
	for key, value := range *f {
		pairs = append(pairs, key+"="+value)
	}
	return strings.Join(pairs, ",")
}

func (f *keyValueFlags) Set(value string) error {
	key, val, ok := strings.Cut(value, "=")
	if !ok || key == "" {
		return fmt.Errorf("expected key=value")
	}
	if *f == nil {
		*f = make(map[string]string)
	}
	(*f)[key] = val
	return nil
}

func prepareOCRInput(path string, stdin io.Reader) (string, func(), error) {
	if path != "-" {
		return path, func() {}, nil
	}
	temporary, err := os.CreateTemp("", "xfyun-ocr-input-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("create temporary OCR input: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	written, err := io.Copy(temporary, io.LimitReader(stdin, ocr.MaxPDFBytes+1))
	if err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("read OCR input: %w", err)
	}
	if written > ocr.MaxPDFBytes {
		cleanup()
		return "", func() {}, fmt.Errorf("OCR input exceeds the 500 MiB PDF limit")
	}
	if err := temporary.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("close temporary OCR input: %w", err)
	}
	return temporaryPath, cleanup, nil
}

func openInput(path string, stdin io.Reader) (io.Reader, func() error, error) {
	if path == "-" {
		return stdin, func() error { return nil }, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open input: %w", err)
	}
	return file, file.Close, nil
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
