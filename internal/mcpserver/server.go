package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fruitbars/go-xfyun-cli/internal/config"
	"github.com/fruitbars/go-xfyun-cli/internal/ifasr"
	"github.com/fruitbars/go-xfyun-cli/internal/media"
	"github.com/fruitbars/go-xfyun-cli/internal/ocr"
	"github.com/fruitbars/go-xfyun-cli/internal/outputfile"
	"github.com/fruitbars/go-xfyun-cli/internal/rtasr"
	"github.com/fruitbars/go-xfyun-cli/internal/tts"
	"github.com/fruitbars/go-xfyun-cli/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Service struct {
	credentials config.Credentials
}

// RequestDiagnostics makes every MCP result self-describing without exposing
// credentials or signed authentication material. Parameters are the effective
// values sent to (or used for) the provider; output contains safe local
// artifact metadata such as paths, byte counts, and page counts.
type RequestDiagnostics struct {
	Service    string            `json:"service"`
	Variant    string            `json:"variant,omitempty"`
	Operation  string            `json:"operation"`
	SID        string            `json:"sid,omitempty"`
	StartedAt  string            `json:"started_at"`
	ElapsedMS  int64             `json:"elapsed_ms"`
	Retries    int               `json:"retries"`
	Parameters map[string]string `json:"parameters,omitempty"`
	Output     map[string]string `json:"output,omitempty"`
}

func newDiagnostics(service, variant, operation string, started time.Time) RequestDiagnostics {
	return RequestDiagnostics{Service: service, Variant: variant, Operation: operation, StartedAt: started.UTC().Format(time.RFC3339Nano), Parameters: map[string]string{}, Output: map[string]string{}}
}

func (d *RequestDiagnostics) finish(started time.Time) {
	d.ElapsedMS = time.Since(started).Milliseconds()
	if d.Parameters != nil && len(d.Parameters) == 0 {
		d.Parameters = nil
	}
	if d.Output != nil && len(d.Output) == 0 {
		d.Output = nil
	}
}

const maxEmbeddedOCRAnnotationBytes = 8 * 1024 * 1024

func notifyProgress(ctx context.Context, req *mcp.CallToolRequest, progress, total float64, message string) {
	if req == nil || req.Params == nil || req.Session == nil {
		return
	}
	token := req.Params.GetProgressToken()
	if token == nil {
		return
	}
	params := &mcp.ProgressNotificationParams{
		ProgressToken: token,
		Progress:      progress,
		Message:       message,
	}
	if total > 0 {
		params.Total = total
	}
	_ = req.Session.NotifyProgress(ctx, params)
}

func New(credentials config.Credentials) *mcp.Server {
	service := &Service{credentials: credentials}
	server := mcp.NewServer(&mcp.Implementation{
		Name:        "xfyun-ai",
		Title:       "XFYun AI",
		Description: "OCR, speech synthesis, and speech transcription through XFYun large-model APIs",
		Version:     version.Current,
	}, &mcp.ServerOptions{
		Instructions: "XFYun media tools: use xfyun_ocr for document images, xfyun_tts for super-smart large-model speech synthesis, xfyun_rtasr for live-style PCM/Opus/Speex streams, and xfyun_ifasr_submit plus xfyun_ifasr_result for completed recordings. IFASR defaults to the Spark large-model variant; set variant=standard for the standard recording transcription API. Standard orders need only order_id; large-model orders need order_id plus signature_random. Use xfyun_media to inspect or convert local audio channels, sample rates, and bitrates. Use OCR annotate=true only when the user asks for layout types drawn on the source. Multi-page OCR returns an NDJSON output_path; consume it incrementally instead of loading the entire file. Long OCR, TTS, and IFASR work reports MCP progress when the caller supplies a progress token; preserve IFASR identifiers and poll with wait=false when the host has short timeouts. Credentials come from XFYUN_APP_ID, XFYUN_API_KEY, and XFYUN_API_SECRET. File paths are local to this server process. Never overwrite OCR, TTS, or media output unless the user authorized force=true.",
	})

	addOCRTool(server, service)
	addTTSTool(server, service)
	addRTASRTool(server, service)
	addIFASRSubmitTool(server, service)
	addIFASRResultTool(server, service)
	addMediaTool(server, service)
	return server
}

type OCRInput struct {
	InputPath       string   `json:"input_path" jsonschema:"Path to a local image or PDF. Unsupported raster formats are converted automatically."`
	OutputPath      string   `json:"output_path,omitempty" jsonschema:"Optional NDJSON destination. Multi-page results use a temporary NDJSON file when omitted."`
	Force           bool     `json:"force,omitempty" jsonschema:"Allow replacement of an existing output_path after all pages succeed."`
	Pages           string   `json:"pages,omitempty" jsonschema:"PDF page selection such as 1-3,5. Default: all pages."`
	PDFDPI          int      `json:"pdf_dpi,omitempty" jsonschema:"PDF page rendering resolution from 72 to 300 DPI. Default: 150."`
	ResultFormat    string   `json:"result_format,omitempty" jsonschema:"json; json,markdown; json,sed; or json,markdown,sed. Default: json,markdown."`
	IncludeRaw      bool     `json:"include_raw,omitempty" jsonschema:"Include the full decoded OCR JSON with coordinates and layout details. Default: false."`
	Annotate        bool     `json:"annotate,omitempty" jsonschema:"Draw selected OCR node types on the source image and return a PNG path. Default: false."`
	AnnotationTypes string   `json:"annotation_types,omitempty" jsonschema:"Comma-separated OCR types to draw, or all. Supports page/layout/region, headers/footers, text, tables/cells, figures, lists, formulas, code, seals, barcodes, annotations, footnotes, key/value, and contents."`
	AnnotationPath  string   `json:"annotation_output_path,omitempty" jsonschema:"PNG destination for one image; for multi-page PDFs this is an output directory. Omit to use a temporary PNG path."`
	ResultOption    string   `json:"result_option,omitempty" jsonschema:"normal plus optional char and no_line_position. Default: normal."`
	MarkdownOptions string   `json:"markdown_options,omitempty" jsonschema:"Element controls such as seal=1,qrcode=1,table=2,watermark=0."`
	SEDOptions      string   `json:"sed_options,omitempty" jsonschema:"SED element controls such as seal=1,qrcode=1,table=2,watermark=0."`
	ExifOption      string   `json:"exif_option,omitempty" jsonschema:"Parse EXIF metadata: 0 off or 1 on. Default: 0."`
	AlphaOption     string   `json:"alpha_option,omitempty" jsonschema:"Honor transparent pixels: 0 off or 1 on. Default: 0."`
	RotationAngle   *float64 `json:"rotation_min_angle,omitempty" jsonschema:"Minimum auto-rotation angle from 0 to 180. Default: 5."`
	TimeoutSeconds  int      `json:"timeout_seconds,omitempty" jsonschema:"OCR request timeout per page in seconds. Default: 120."`
}

type OCROutput struct {
	Text               string               `json:"text,omitempty"`
	Markdown           string               `json:"markdown,omitempty"`
	SED                []ocr.SEDElement     `json:"sed,omitempty"`
	Raw                string               `json:"raw,omitempty"`
	AnnotationPath     string               `json:"annotation_path,omitempty"`
	AnnotationPaths    []string             `json:"annotation_paths,omitempty"`
	AnnotationTypes    []string             `json:"annotation_types,omitempty"`
	AnnotationCount    int                  `json:"annotation_count,omitempty"`
	AnnotationEmbedded bool                 `json:"annotation_embedded,omitempty"`
	SID                string               `json:"sid,omitempty"`
	ResultFormat       string               `json:"result_format"`
	PageCount          int                  `json:"page_count"`
	OutputPath         string               `json:"output_path,omitempty"`
	OutputFormat       string               `json:"output_format,omitempty"`
	AutoGenerated      bool                 `json:"auto_generated,omitempty"`
	Diagnostics        []RequestDiagnostics `json:"diagnostics,omitempty"`
}

type OCRPageOutput struct {
	Type            string              `json:"type"`
	Page            int                 `json:"page"`
	Image           int                 `json:"image"`
	TotalPages      int                 `json:"total_pages"`
	DPI             int                 `json:"dpi,omitempty"`
	Text            string              `json:"text"`
	Markdown        string              `json:"markdown,omitempty"`
	SED             []ocr.SEDElement    `json:"sed,omitempty"`
	Raw             string              `json:"raw,omitempty"`
	AnnotationPath  string              `json:"annotation_path,omitempty"`
	AnnotationTypes []string            `json:"annotation_types,omitempty"`
	AnnotationCount int                 `json:"annotation_count,omitempty"`
	SID             string              `json:"sid,omitempty"`
	Diagnostics     *RequestDiagnostics `json:"diagnostics,omitempty"`
}

func addOCRTool(server *mcp.Server, service *Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "xfyun_ocr",
		Title:       "Recognize an image or PDF",
		Description: "Convert a local raster image or stream a PDF through render, compression, OCR, and release one page at a time. Returns readable Markdown/SED by default; set include_raw=true for full layout JSON. Set annotate=true to draw selected element types and return an annotated PNG for a single page or paths for multiple pages.",
		Annotations: annotations(false, true, true),
	}, func(ctx context.Context, req *mcp.CallToolRequest, input OCRInput) (*mcp.CallToolResult, OCROutput, error) {
		if input.InputPath == "" {
			return nil, OCROutput{}, fmt.Errorf("input_path is required")
		}
		if input.ResultFormat == "" {
			input.ResultFormat = "json,markdown"
		}
		if input.ResultOption == "" {
			input.ResultOption = "normal"
		}
		if input.MarkdownOptions == "" {
			input.MarkdownOptions = ocr.DefaultMarkdownElementOption
		}
		if input.ExifOption == "" {
			input.ExifOption = "0"
		}
		if input.AlphaOption == "" {
			input.AlphaOption = "0"
		}
		pdfDPI := input.PDFDPI
		if pdfDPI == 0 {
			pdfDPI = ocr.DefaultPDFDPI
		}
		rotationAngle := 5.0
		if input.RotationAngle != nil {
			rotationAngle = *input.RotationAngle
		}
		output := OCROutput{ResultFormat: input.ResultFormat}
		started := time.Now()
		ocrDiagnostics := newDiagnostics("ocr", "spark", "recognize", started)
		ocrDiagnostics.Parameters["resultFormat"] = input.ResultFormat
		ocrDiagnostics.Parameters["resultOption"] = input.ResultOption
		ocrDiagnostics.Parameters["markdownOptions"] = input.MarkdownOptions
		ocrDiagnostics.Parameters["exifOption"] = input.ExifOption
		ocrDiagnostics.Parameters["alphaOption"] = input.AlphaOption
		ocrDiagnostics.Parameters["rotationMinAngle"] = fmt.Sprintf("%.g", rotationAngle)
		ocrDiagnostics.Parameters["pdfDPI"] = fmt.Sprintf("%d", pdfDPI)
		if input.SEDOptions != "" {
			ocrDiagnostics.Parameters["sedOptions"] = input.SEDOptions
		}
		if input.Pages != "" {
			ocrDiagnostics.Parameters["pages"] = input.Pages
		}
		ocrDiagnostics.Parameters["annotate"] = fmt.Sprintf("%t", input.Annotate)
		if info, statErr := os.Stat(input.InputPath); statErr == nil {
			ocrDiagnostics.Parameters["fileName"] = filepath.Base(input.InputPath)
			ocrDiagnostics.Parameters["fileSize"] = fmt.Sprintf("%d", info.Size())
		}
		var annotationTypes []string
		if input.Annotate {
			var err error
			annotationTypes, err = ocr.ParseAnnotationTypes(input.AnnotationTypes)
			if err != nil {
				return nil, OCROutput{}, err
			}
			output.AnnotationTypes = annotationTypes
			ocrDiagnostics.Parameters["annotationTypes"] = strings.Join(annotationTypes, ",")
		}
		client := ocr.Client{Credentials: service.credentials}
		var outputFile *os.File
		var outputEncoder *json.Encoder
		var temporaryPath string
		var needsCommit bool
		var singleAnnotation []byte
		committed := false
		defer func() {
			if outputFile != nil {
				_ = outputFile.Close()
			}
			if !committed && temporaryPath != "" {
				_ = os.Remove(temporaryPath)
			}
		}()
		openPageSink := func(pageCount int) error {
			if outputFile != nil || (input.OutputPath == "" && pageCount == 1) {
				return nil
			}
			if input.OutputPath != "" {
				absolutePath, tempFile, err := prepareAtomicOutput(input.OutputPath, input.Force)
				if err != nil {
					return err
				}
				output.OutputPath = absolutePath
				outputFile = tempFile
				temporaryPath = tempFile.Name()
				needsCommit = true
			} else {
				tempFile, err := os.CreateTemp("", "xfyun-ocr-*.ndjson")
				if err != nil {
					return fmt.Errorf("create streaming OCR output: %w", err)
				}
				absolutePath, err := filepath.Abs(tempFile.Name())
				if err != nil {
					_ = tempFile.Close()
					_ = os.Remove(tempFile.Name())
					return fmt.Errorf("resolve streaming OCR output: %w", err)
				}
				output.OutputPath = absolutePath
				output.AutoGenerated = true
				outputFile = tempFile
				temporaryPath = tempFile.Name()
			}
			output.OutputFormat = "ndjson"
			outputEncoder = json.NewEncoder(outputFile)
			return nil
		}
		err := ocr.StreamPath(ctx, input.InputPath, input.Pages, input.PDFDPI, func(documentImage ocr.DocumentImage) error {
			if err := openPageSink(documentImage.PageCount); err != nil {
				return err
			}
			requestCtx, cancel := withTimeout(ctx, input.TimeoutSeconds, 120)
			decoded, response, recognizeErr := client.Recognize(requestCtx, documentImage.Data, ocr.Options{
				Encoding: documentImage.Encoding, ResultFormat: input.ResultFormat,
				ResultOption: input.ResultOption, MarkdownElementOption: input.MarkdownOptions,
				SEDElementOption: input.SEDOptions, ExifOption: input.ExifOption, AlphaOption: input.AlphaOption,
				RotationMinAngle: rotationAngle,
			})
			cancel()
			if recognizeErr != nil {
				return fmt.Errorf("OCR page %d image %d: %w", documentImage.Page, documentImage.Index, recognizeErr)
			}
			formats := ocr.ExtractResultFormats(decoded)
			displayText := string(decoded)
			if formats.Markdown != "" {
				displayText = formats.Markdown
			} else if len(formats.SED) > 0 {
				if sedJSON, err := json.Marshal(formats.SED); err == nil {
					displayText = string(sedJSON)
				}
			}
			page := OCRPageOutput{
				Type: "page", Page: documentImage.Page, Image: documentImage.Index,
				TotalPages: documentImage.PageCount, DPI: documentImage.DPI,
				Text: displayText, Markdown: formats.Markdown, SED: formats.SED, SID: response.Header.SID,
			}
			pageDiagnostics := ocrDiagnostics
			pageDiagnostics.SID = response.Header.SID
			pageDiagnostics.Parameters = cloneStringMap(ocrDiagnostics.Parameters)
			pageDiagnostics.Parameters["encoding"] = documentImage.Encoding
			pageDiagnostics.Output = map[string]string{"page": fmt.Sprintf("%d", documentImage.Page), "image": fmt.Sprintf("%d", documentImage.Index)}
			pageDiagnostics.finish(started)
			page.Diagnostics = &pageDiagnostics
			ocrDiagnostics.SID = response.Header.SID
			output.SID = response.Header.SID
			if input.IncludeRaw {
				page.Raw = string(decoded)
			}
			if input.Annotate {
				annotated, count, err := ocr.AnnotateImage(documentImage.Data, decoded, strings.Join(annotationTypes, ","))
				if err != nil {
					return fmt.Errorf("annotate OCR page %d image %d: %w", documentImage.Page, documentImage.Index, err)
				}
				annotationPath, err := saveOCRAnnotation(annotated, input.AnnotationPath, documentImage.Page, documentImage.PageCount, input.Force)
				if err != nil {
					return fmt.Errorf("write OCR annotation for page %d: %w", documentImage.Page, err)
				}
				page.AnnotationPath, page.AnnotationTypes, page.AnnotationCount = annotationPath, annotationTypes, count
				if documentImage.PageCount == 1 {
					output.AnnotationPath = annotationPath
					if len(annotated) <= maxEmbeddedOCRAnnotationBytes {
						singleAnnotation = annotated
						output.AnnotationEmbedded = true
					}
				} else {
					output.AnnotationPaths = append(output.AnnotationPaths, annotationPath)
				}
				output.AnnotationCount += count
			}
			output.PageCount++
			if page.Diagnostics != nil && page.Diagnostics.Output != nil {
				page.Diagnostics.Output["pageCount"] = fmt.Sprintf("%d", documentImage.PageCount)
			}
			if outputEncoder != nil {
				if err := outputEncoder.Encode(page); err != nil {
					return fmt.Errorf("write streaming OCR result for page %d: %w", page.Page, err)
				}
			} else {
				output.Text = page.Text
				output.Markdown = page.Markdown
				output.SED = page.SED
				output.Raw = page.Raw
				output.AnnotationPath = page.AnnotationPath
				output.SID = page.SID
			}
			if req != nil && req.Params != nil && req.Session != nil {
				if token := req.Params.GetProgressToken(); token != nil {
					_ = req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
						ProgressToken: token,
						Progress:      float64(output.PageCount),
						Total:         float64(documentImage.PageCount),
						Message:       fmt.Sprintf("OCR page %d of %d completed", output.PageCount, documentImage.PageCount),
					})
				}
			}
			return nil
		})
		if err != nil {
			return nil, OCROutput{}, err
		}
		ocrDiagnostics.SID = output.SID
		ocrDiagnostics.Output = map[string]string{"pageCount": fmt.Sprintf("%d", output.PageCount)}
		if output.OutputPath != "" {
			ocrDiagnostics.Output["outputPath"] = output.OutputPath
		}
		ocrDiagnostics.finish(started)
		output.Diagnostics = []RequestDiagnostics{ocrDiagnostics}
		if outputFile != nil {
			if err := outputFile.Close(); err != nil {
				return nil, OCROutput{}, fmt.Errorf("close streaming OCR output: %w", err)
			}
			outputFile = nil
			if needsCommit {
				if err := outputfile.Commit(temporaryPath, output.OutputPath, input.Force); err != nil {
					return nil, OCROutput{}, fmt.Errorf("commit OCR output: %w", err)
				}
			}
			committed = true
		}
		if len(singleAnnotation) > 0 {
			return &mcp.CallToolResult{Content: []mcp.Content{
				&mcp.TextContent{Text: output.Text},
				&mcp.ImageContent{Data: singleAnnotation, MIMEType: "image/png"},
			}}, output, nil
		}
		return nil, output, nil
	})
}

func saveOCRAnnotation(data []byte, requested string, page, pageCount int, force bool) (string, error) {
	destination := requested
	if pageCount > 1 && destination != "" {
		if info, err := os.Stat(destination); err == nil {
			if !info.IsDir() {
				return "", fmt.Errorf("annotation_output_path must be a directory for multi-page OCR")
			}
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect annotation directory: %w", err)
		}
		if err := os.MkdirAll(destination, 0o755); err != nil {
			return "", fmt.Errorf("create annotation directory: %w", err)
		}
		destination = filepath.Join(destination, fmt.Sprintf("page_%03d.png", page))
	}
	if destination == "" {
		temporary, err := os.CreateTemp("", "xfyun-ocr-annotated-*.png")
		if err != nil {
			return "", fmt.Errorf("create temporary annotation: %w", err)
		}
		name := temporary.Name()
		if _, err := temporary.Write(data); err != nil {
			_ = temporary.Close()
			_ = os.Remove(name)
			return "", fmt.Errorf("write temporary annotation: %w", err)
		}
		if err := temporary.Close(); err != nil {
			_ = os.Remove(name)
			return "", fmt.Errorf("close temporary annotation: %w", err)
		}
		return filepath.Abs(name)
	}
	absolute, temporary, err := outputfile.Prepare(destination, force)
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := temporary.Write(data); err != nil {
		return "", fmt.Errorf("write annotation: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close annotation: %w", err)
	}
	if err := outputfile.Commit(temporaryPath, absolute, force); err != nil {
		return "", fmt.Errorf("commit annotation: %w", err)
	}
	committed = true
	return absolute, nil
}

type TTSInput struct {
	Text              string `json:"text,omitempty" jsonschema:"UTF-8 text. Input over 64 KiB is split automatically. Use either text or text_path."`
	TextPath          string `json:"text_path,omitempty" jsonschema:"Local UTF-8 text file. Use either text or text_path."`
	OutputPath        string `json:"output_path" jsonschema:"Local destination path for generated audio."`
	Force             bool   `json:"force,omitempty" jsonschema:"Allow replacement of an existing output file after synthesis succeeds."`
	Voice             string `json:"voice,omitempty" jsonschema:"Enabled XFYun voice ID. Default: x5_lingxiaoxuan_flow."`
	Encoding          string `json:"encoding,omitempty" jsonschema:"Audio encoding. lame writes playable MP3; raw writes headerless PCM; Opus/Speex variants are raw codec streams, not Ogg containers. Default: lame."`
	SampleRate        int    `json:"sample_rate,omitempty" jsonschema:"8000, 16000, or 24000. Default: 24000."`
	Speed             *int   `json:"speed,omitempty" jsonschema:"Speech speed from 0 to 100. Default: 50."`
	Volume            *int   `json:"volume,omitempty" jsonschema:"Volume from 0 to 100. Default: 50."`
	Pitch             *int   `json:"pitch,omitempty" jsonschema:"Pitch from 0 to 100. Default: 50."`
	OralLevel         string `json:"oral_level,omitempty" jsonschema:"low, mid, or high. Official docs limit oral controls to x4 voices. Default: mid."`
	SparkAssist       *int   `json:"spark_assist,omitempty" jsonschema:"Large-model oralization: 0 off or 1 on. Default: 1."`
	StopSplit         *int   `json:"stop_split,omitempty" jsonschema:"Disable server sentence splitting: 0 no or 1 yes. Default: 0."`
	Remain            *int   `json:"remain,omitempty" jsonschema:"Preserve written form: 0 no or 1 yes. Default: 0."`
	BackgroundSound   *int   `json:"background_sound,omitempty" jsonschema:"Background sound: 0 off or 1 on. Default: 0."`
	EnglishReading    *int   `json:"english_reading,omitempty" jsonschema:"English mode: 0 auto/word fallback; 1 letters; 2 auto/letter fallback."`
	NumberReading     *int   `json:"number_reading,omitempty" jsonschema:"Number mode: 0 auto; 1 numeric; 2 digit string; 3 string preferred."`
	ReturnPronounce   *int   `json:"return_pronounce,omitempty" jsonschema:"Return phoneme timing annotation: 0 off or 1 on."`
	VisibleWatermark  *int   `json:"visible_watermark,omitempty" jsonschema:"Audible watermark: 0 off; 1 sentence start; 2 sentence end."`
	ImplicitWatermark bool   `json:"implicit_watermark,omitempty" jsonschema:"Enable an implicit watermark; lame/MP3 only."`
	TimeoutSeconds    int    `json:"timeout_seconds,omitempty" jsonschema:"Request timeout in seconds. Default: 300."`
}

type TTSOutput struct {
	OutputPath    string              `json:"output_path"`
	SID           string              `json:"sid,omitempty"`
	SIDs          []string            `json:"sids,omitempty"`
	Segments      int                 `json:"segments"`
	Encoding      string              `json:"encoding"`
	SampleRate    int                 `json:"sample_rate"`
	Bytes         int64               `json:"bytes"`
	Pronunciation string              `json:"pronunciation,omitempty"`
	Diagnostics   *RequestDiagnostics `json:"diagnostics,omitempty"`
}

func addTTSTool(server *mcp.Server, service *Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "xfyun_tts",
		Title:       "Synthesize speech",
		Description: "Synthesize text with XFYun super smart TTS and atomically write audio to a local output path.",
		Annotations: annotations(false, true, true),
	}, func(ctx context.Context, req *mcp.CallToolRequest, input TTSInput) (*mcp.CallToolResult, TTSOutput, error) {
		started := time.Now()
		text, err := resolveText(input.Text, input.TextPath)
		if err != nil {
			return nil, TTSOutput{}, err
		}
		if input.OutputPath == "" {
			return nil, TTSOutput{}, fmt.Errorf("output_path is required")
		}
		if input.Voice == "" {
			input.Voice = "x5_lingxiaoxuan_flow"
		}
		if input.Encoding == "" {
			input.Encoding = "lame"
		}
		if input.SampleRate == 0 {
			input.SampleRate = 24000
		}
		speed := 50
		if input.Speed != nil {
			speed = *input.Speed
		}
		volume := 50
		if input.Volume != nil {
			volume = *input.Volume
		}
		pitch := 50
		if input.Pitch != nil {
			pitch = *input.Pitch
		}
		if input.OralLevel == "" {
			input.OralLevel = "mid"
		}
		sparkAssist := intValue(input.SparkAssist, 1)
		absolutePath, tempFile, err := prepareAtomicOutput(input.OutputPath, input.Force)
		if err != nil {
			return nil, TTSOutput{}, err
		}
		tempPath := tempFile.Name()
		committed := false
		defer func() {
			_ = tempFile.Close()
			if !committed {
				_ = os.Remove(tempPath)
			}
		}()
		defaultTimeout := 300 * len(tts.SplitText(text, tts.MaxTextBytes))
		requestCtx, cancel := withTimeout(ctx, input.TimeoutSeconds, defaultTimeout)
		defer cancel()
		client := tts.Client{Credentials: service.credentials}
		metadata, err := client.Synthesize(requestCtx, text, tempFile, tts.Options{
			Voice: input.Voice, Encoding: input.Encoding, SampleRate: input.SampleRate,
			Speed: speed, Volume: volume, Pitch: pitch,
			OralLevel: input.OralLevel, SparkAssist: sparkAssist,
			StopSplit: intValue(input.StopSplit, 0), Remain: intValue(input.Remain, 0),
			BackgroundSound: intValue(input.BackgroundSound, 0), EnglishReading: intValue(input.EnglishReading, 0),
			NumberReading: intValue(input.NumberReading, 0), ReturnPronounce: intValue(input.ReturnPronounce, 0),
			VisibleWatermark: intValue(input.VisibleWatermark, 0), ImplicitWatermark: input.ImplicitWatermark,
			Progress: func(done, total int, message string) {
				notifyProgress(ctx, req, float64(done), float64(total), message)
			},
		})
		if err != nil {
			return nil, TTSOutput{}, err
		}
		if err := tempFile.Close(); err != nil {
			return nil, TTSOutput{}, fmt.Errorf("close temporary audio: %w", err)
		}
		if err := outputfile.Commit(tempPath, absolutePath, input.Force); err != nil {
			return nil, TTSOutput{}, fmt.Errorf("commit audio output: %w", err)
		}
		committed = true
		diagnostics := newDiagnostics("tts", "spark", "synthesize", started)
		diagnostics.SID = metadata.SID
		diagnostics.Parameters = map[string]string{
			"voice": input.Voice, "encoding": input.Encoding,
			"sampleRate": fmt.Sprintf("%d", input.SampleRate), "speed": fmt.Sprintf("%d", speed),
			"volume": fmt.Sprintf("%d", volume), "pitch": fmt.Sprintf("%d", pitch),
			"oralLevel": input.OralLevel, "sparkAssist": fmt.Sprintf("%d", sparkAssist),
			"stopSplit":         fmt.Sprintf("%d", intValue(input.StopSplit, 0)),
			"remain":            fmt.Sprintf("%d", intValue(input.Remain, 0)),
			"backgroundSound":   fmt.Sprintf("%d", intValue(input.BackgroundSound, 0)),
			"englishReading":    fmt.Sprintf("%d", intValue(input.EnglishReading, 0)),
			"numberReading":     fmt.Sprintf("%d", intValue(input.NumberReading, 0)),
			"returnPronounce":   fmt.Sprintf("%d", intValue(input.ReturnPronounce, 0)),
			"visibleWatermark":  fmt.Sprintf("%d", intValue(input.VisibleWatermark, 0)),
			"implicitWatermark": fmt.Sprintf("%t", input.ImplicitWatermark),
			"segments":          fmt.Sprintf("%d", metadata.Segments),
		}
		diagnostics.Output = map[string]string{"path": absolutePath, "bytes": fmt.Sprintf("%d", metadata.Bytes)}
		diagnostics.finish(started)
		return nil, TTSOutput{
			OutputPath: absolutePath, SID: metadata.SID, SIDs: metadata.SIDs, Segments: metadata.Segments, Encoding: metadata.Encoding,
			SampleRate: metadata.SampleRate, Bytes: metadata.Bytes, Pronunciation: metadata.Pronunciation, Diagnostics: &diagnostics,
		}, nil
	})
}

type RTASRInput struct {
	InputPath          string            `json:"input_path" jsonschema:"Local PCM, Opus, or Speex audio path."`
	Language           string            `json:"language,omitempty" jsonschema:"autodialect or autominor. Default: autodialect."`
	RecognizedLanguage string            `json:"recognized_language,omitempty" jsonschema:"Comma-separated target languages for autominor."`
	AudioEncoding      string            `json:"audio_encoding,omitempty" jsonschema:"pcm_s16le, opus-wb, speex-7, or speex-10. Default: pcm_s16le."`
	SampleRate         int               `json:"sample_rate,omitempty" jsonschema:"PCM sample rate, 8000 or 16000. Default: 16000."`
	RoleType           int               `json:"role_type,omitempty" jsonschema:"Speaker separation: 0 off, 2 on."`
	FeatureIDs         string            `json:"feature_ids,omitempty" jsonschema:"Comma-separated registered voiceprint IDs."`
	Domain             string            `json:"domain,omitempty" jsonschema:"Domain optimization such as finance or medical."`
	SpeakerMatch       bool              `json:"speaker_match,omitempty" jsonschema:"Restrict separated roles to registered feature IDs; requires role_type=2 and feature_ids."`
	KeepPunctuation    *bool             `json:"keep_punctuation,omitempty" jsonschema:"Keep punctuation. Default: true."`
	VADMode            int               `json:"vad_mode,omitempty" jsonschema:"VAD mode: 1 far field or 2 near field. Omit for service default."`
	Extra              map[string]string `json:"extra,omitempty" jsonschema:"Additional documented RTASR query parameters."`
	TimeoutSeconds     int               `json:"timeout_seconds,omitempty" jsonschema:"Overall timeout in seconds. Default: 29100."`
}

type RTASROutput struct {
	Transcript  string              `json:"transcript"`
	SID         string              `json:"sid,omitempty"`
	Segments    int                 `json:"segments"`
	Diagnostics *RequestDiagnostics `json:"diagnostics,omitempty"`
}

func addRTASRTool(server *mcp.Server, service *Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "xfyun_rtasr",
		Title:       "Transcribe audio in real time",
		Description: "Stream a local PCM/Opus/Speex file at real-time pacing and return final transcription text.",
		Annotations: annotations(true, false, true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input RTASRInput) (*mcp.CallToolResult, RTASROutput, error) {
		started := time.Now()
		file, err := os.Open(input.InputPath)
		if err != nil {
			return nil, RTASROutput{}, fmt.Errorf("open audio: %w", err)
		}
		defer file.Close()
		if input.Language == "" {
			input.Language = "autodialect"
		}
		if input.AudioEncoding == "" {
			input.AudioEncoding = "pcm_s16le"
		}
		if input.SampleRate == 0 {
			input.SampleRate = 16000
		}
		requestCtx, cancel := withTimeout(ctx, input.TimeoutSeconds, 8*60*60+5*60)
		defer cancel()
		client := rtasr.Client{Credentials: service.credentials}
		result, err := client.Transcribe(requestCtx, file, rtasr.Options{
			Language: input.Language, RecognizedLanguage: input.RecognizedLanguage,
			AudioEncoding: input.AudioEncoding, SampleRate: input.SampleRate,
			RoleType: input.RoleType, FeatureIDs: input.FeatureIDs,
			Domain: input.Domain, SpeakerMatch: input.SpeakerMatch,
			KeepPunctuation: input.KeepPunctuation, VADMode: input.VADMode, Extra: input.Extra,
		}, nil)
		if err != nil {
			return nil, RTASROutput{}, err
		}
		diagnostics := newDiagnostics("rtasr", "spark", "transcribe", started)
		diagnostics.SID = result.SID
		diagnostics.Parameters = map[string]string{
			"language": input.Language, "audioEncoding": input.AudioEncoding,
			"sampleRate": fmt.Sprintf("%d", input.SampleRate), "roleType": fmt.Sprintf("%d", input.RoleType),
			"recognizedLanguage": input.RecognizedLanguage, "featureIDs": input.FeatureIDs,
			"domain": input.Domain, "speakerMatch": fmt.Sprintf("%t", input.SpeakerMatch),
			"keepPunctuation": diagnosticOptionalBool(input.KeepPunctuation),
			"vadMode":         fmt.Sprintf("%d", input.VADMode),
			"segments":        fmt.Sprintf("%d", result.Segments),
		}
		for key, value := range input.Extra {
			if isSensitiveDiagnosticKey(key) {
				continue
			}
			diagnostics.Parameters["extra."+key] = value
		}
		diagnostics.finish(started)
		return nil, RTASROutput{Transcript: result.Transcript, SID: result.SID, Segments: result.Segments, Diagnostics: &diagnostics}, nil
	})
}

type IFASRSubmitInput struct {
	Variant             string            `json:"variant,omitempty" jsonschema:"Recording transcription engine: llm (default, Spark large model) or standard (legacy standard IFASR)."`
	InputPath           string            `json:"input_path,omitempty" jsonschema:"Local recording path. LLM supports mp3, wav, pcm, opus, flac, ogg, speex; standard also accepts aac, m4a, amr, ac3, ape, m4r, mp4, acc, wma. Use exactly one of input_path or audio_url. Local files over 5 hours or 500 MiB are split automatically."`
	AudioURL            string            `json:"audio_url,omitempty" jsonschema:"Absolute HTTP(S) audio URL for audioMode=urlLink. Use exactly one of input_path or audio_url; maximum 512 characters."`
	FileName            string            `json:"file_name,omitempty" jsonschema:"Remote audio filename including a supported extension; required with audio_url."`
	FileSizeBytes       int64             `json:"file_size_bytes,omitempty" jsonschema:"Remote audio byte size; required with audio_url and limited to 500 MiB."`
	Language            string            `json:"language,omitempty" jsonschema:"LLM: autodialect or autominor (default autodialect); standard: cn, en, ja, and other enabled languages (default cn)."`
	DurationMS          int64             `json:"duration_ms,omitempty" jsonschema:"Known duration in milliseconds. Zero probes local files automatically and disables duration validation for audio_url."`
	Domain              string            `json:"domain,omitempty" jsonschema:"Domain optimization: court, finance, medical, tech, sport, edu, isp, gov, game, ecom, mil, com, life, ent, culture, or car."`
	TrackMode           int               `json:"track_mode,omitempty" jsonschema:"Channel mode: omit for automatic local detection (mono omits trackMode; stereo selects 2), 1 mixed, or 2 stereo tracks. Explicit values win; 2 is incompatible with role_type and language_analysis."`
	RoleType            int               `json:"role_type,omitempty" jsonschema:"Speaker separation: 0 off, 1 generic, 3 voiceprint."`
	RoleNum             int               `json:"role_num,omitempty" jsonschema:"Expected speaker count from 0 to 10."`
	FeatureIDs          string            `json:"feature_ids,omitempty" jsonschema:"Comma-separated registered voiceprint IDs; role_type=3 only; maximum 64."`
	CallbackURL         string            `json:"callback_url,omitempty" jsonschema:"GET callback URL invoked after completion; maximum 512 characters."`
	Smooth              *bool             `json:"smooth,omitempty" jsonschema:"Enable transcript smoothing. Service default: true."`
	Colloquial          *bool             `json:"colloquial,omitempty" jsonschema:"Enable colloquial normalization. Service default: false."`
	VADMode             int               `json:"vad_mode,omitempty" jsonschema:"VAD mode: 1 far field or 2 near field."`
	CantoneseScript     *int              `json:"cantonese_script,omitempty" jsonschema:"Cantonese output: 0 simplified or 1 traditional. Service default: 1."`
	LanguageAnalysis    bool              `json:"language_analysis,omitempty" jsonschema:"Enable spoken-language analysis; multilingual entitlement required. Query result_type=analysis or transfer,analysis."`
	HotWord             string            `json:"hot_word,omitempty" jsonschema:"Standard only: pipe-separated hot words."`
	SysDicts            string            `json:"sys_dicts,omitempty" jsonschema:"Standard only: system dictionary names."`
	Candidate           int               `json:"candidate,omitempty" jsonschema:"Standard only: multi-candidate output, 0 or 1."`
	StandardWav         int               `json:"standard_wav,omitempty" jsonschema:"Standard only: standard 16k/16bit/mono WAV, 0 or 1."`
	LanguageType        int               `json:"language_type,omitempty" jsonschema:"Standard only: language mode 1 automatic, 2 Chinese, or 4 pure Chinese."`
	TranslationLanguage string            `json:"translation_language,omitempty" jsonschema:"Standard only: target translation language."`
	TranslationMode     int               `json:"translation_mode,omitempty" jsonschema:"Standard only: translation mode 1 VAD, 2 paragraph, or 3 full text."`
	SegmentMax          int               `json:"segment_max,omitempty" jsonschema:"Standard only: maximum segment characters, 0-500."`
	SegmentMin          int               `json:"segment_min,omitempty" jsonschema:"Standard only: minimum segment characters, 0-50."`
	SegmentWeight       float64           `json:"segment_weight,omitempty" jsonschema:"Standard only: segment character weight, 0-0.05."`
	VADMargin           int               `json:"vad_margin,omitempty" jsonschema:"Standard only: include leading/trailing silence, 0 or 1."`
	Extra               map[string]string `json:"extra,omitempty" jsonschema:"Additional documented upload query parameters."`
	TaskFilePath        string            `json:"task_file_path,omitempty" jsonschema:"Optional local 0600 continuation file. Saves order IDs, signature randoms, and request traces for later result queries."`
	TaskFileForce       bool              `json:"task_file_force,omitempty" jsonschema:"Allow replacement of an existing task_file_path."`
}

type IFASRSubmitOutput struct {
	OrderID          string               `json:"order_id,omitempty"`
	SignatureRandom  string               `json:"signature_random,omitempty"`
	TaskEstimateTime int64                `json:"task_estimate_time_ms,omitempty"`
	Split            bool                 `json:"split"`
	Requests         []ifasr.RequestTrace `json:"requests,omitempty"`
	Parts            []IFASRSubmitPart    `json:"parts,omitempty"`
	TaskFilePath     string               `json:"task_file_path,omitempty"`
}

type IFASRSubmitPart struct {
	Index            int                  `json:"index"`
	OrderID          string               `json:"order_id"`
	SignatureRandom  string               `json:"signature_random,omitempty"`
	Size             int64                `json:"size_bytes"`
	DurationMS       int64                `json:"duration_ms,omitempty"`
	TaskEstimateTime int64                `json:"task_estimate_time_ms,omitempty"`
	Requests         []ifasr.RequestTrace `json:"requests,omitempty"`
}

func addIFASRSubmitTool(server *mcp.Server, service *Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "xfyun_ifasr_submit",
		Title:       "Submit recording transcription",
		Description: "Submit a local or HTTP(S) recording, automatically split over-limit local files, and return one or more orders needed to query transcription.",
		Annotations: annotations(false, false, true),
	}, func(ctx context.Context, req *mcp.CallToolRequest, input IFASRSubmitInput) (*mcp.CallToolResult, IFASRSubmitOutput, error) {
		variant := ifasr.Variant(input.Variant)
		if variant == "" {
			variant = ifasr.VariantLLM
		}
		client := ifasr.Client{Credentials: service.credentials, Variant: variant}
		opts := ifasr.Options{
			Variant:  variant,
			Language: input.Language, DurationMS: input.DurationMS, Domain: input.Domain, TrackMode: input.TrackMode,
			CallbackURL: input.CallbackURL, RoleType: input.RoleType, RoleNum: input.RoleNum, FeatureIDs: input.FeatureIDs,
			Smooth: input.Smooth, Colloquial: input.Colloquial, VADMode: input.VADMode,
			CantoneseScript: input.CantoneseScript, Analysis: input.LanguageAnalysis,
			HotWord: input.HotWord, SysDicts: input.SysDicts, Candidate: input.Candidate, StandardWav: input.StandardWav,
			LanguageType: input.LanguageType, TransLanguage: input.TranslationLanguage, TransMode: input.TranslationMode,
			EngSegMax: input.SegmentMax, EngSegMin: input.SegmentMin, EngSegWeight: input.SegmentWeight, VADMargin: input.VADMargin,
			Extra: input.Extra, NoWait: true,
		}
		opts.Progress = func(done, total int, message string) {
			notifyProgress(ctx, req, float64(done), float64(total), message)
		}
		if opts.Variant != ifasr.VariantLLM && opts.Variant != ifasr.VariantStandard {
			return nil, IFASRSubmitOutput{}, fmt.Errorf("variant must be llm or standard")
		}
		hasPath, hasURL := input.InputPath != "", input.AudioURL != ""
		if hasPath == hasURL {
			return nil, IFASRSubmitOutput{}, fmt.Errorf("provide exactly one of input_path or audio_url")
		}
		if hasURL {
			if input.FileName == "" || input.FileSizeBytes <= 0 {
				return nil, IFASRSubmitOutput{}, fmt.Errorf("file_name and positive file_size_bytes are required with audio_url")
			}
			result, err := client.TranscribeURL(ctx, input.AudioURL, input.FileName, input.FileSizeBytes, opts)
			if err != nil {
				return nil, IFASRSubmitOutput{}, err
			}
			if input.TaskFilePath != "" {
				task := ifasr.TaskFile{Version: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Variant: client.Variant, Parts: []ifasr.TaskPart{{Index: 1, OrderID: result.OrderID, SignatureRandom: result.SignatureRand, SizeBytes: input.FileSizeBytes}}, Requests: result.Requests}
				if err := ifasr.SaveTask(input.TaskFilePath, task, input.TaskFileForce); err != nil {
					return nil, IFASRSubmitOutput{}, err
				}
			}
			notifyProgress(ctx, req, 1, 1, "IFASR remote recording submitted")
			return nil, IFASRSubmitOutput{
				OrderID: result.OrderID, SignatureRandom: result.SignatureRand,
				TaskEstimateTime: result.Response.Content.TaskEstimateTime, Requests: result.Requests, TaskFilePath: input.TaskFilePath,
			}, nil
		}
		batch, err := client.TranscribeFile(ctx, input.InputPath, opts)
		if err != nil {
			return nil, IFASRSubmitOutput{}, err
		}
		output := IFASRSubmitOutput{Split: batch.Split}
		for _, part := range batch.Parts {
			output.Parts = append(output.Parts, IFASRSubmitPart{
				Index: part.Index, OrderID: part.Result.OrderID, SignatureRandom: part.Result.SignatureRand,
				Size: part.Size, DurationMS: part.DurationMS,
				TaskEstimateTime: part.Result.Response.Content.TaskEstimateTime, Requests: part.Result.Requests,
			})
			output.Requests = append(output.Requests, part.Result.Requests...)
		}
		if len(output.Parts) == 1 {
			output.OrderID = output.Parts[0].OrderID
			output.SignatureRandom = output.Parts[0].SignatureRandom
			output.TaskEstimateTime = output.Parts[0].TaskEstimateTime
			output.Requests = output.Parts[0].Requests
			output.Parts = nil
		}
		if input.TaskFilePath != "" {
			task := ifasr.NewTaskFile(client.Variant, batch)
			if err := ifasr.SaveTask(input.TaskFilePath, task, input.TaskFileForce); err != nil {
				return nil, IFASRSubmitOutput{}, err
			}
			output.TaskFilePath = input.TaskFilePath
		}
		return nil, output, nil
	})
}

type IFASROrderRef struct {
	OrderID         string `json:"order_id"`
	SignatureRandom string `json:"signature_random"`
}

type IFASRResultInput struct {
	Variant          string          `json:"variant,omitempty" jsonschema:"Recording transcription engine: llm (default) or standard. Must match the submit variant."`
	OrderID          string          `json:"order_id,omitempty" jsonschema:"Order ID returned for a single-part submission."`
	SignatureRandom  string          `json:"signature_random,omitempty" jsonschema:"Signature random returned for a single-part submission."`
	Orders           []IFASROrderRef `json:"orders,omitempty" jsonschema:"Ordered references returned in parts for an automatically split submission."`
	ResultType       string          `json:"result_type,omitempty" jsonschema:"LLM: transfer, analysis, or transfer,analysis; standard: transfer, translate, or predict. Default: transfer."`
	Wait             bool            `json:"wait,omitempty" jsonschema:"Wait and poll until the order finishes."`
	PollSeconds      int             `json:"poll_seconds,omitempty" jsonschema:"Polling interval in seconds. Default: 2."`
	MaxWaitSeconds   int             `json:"max_wait_seconds,omitempty" jsonschema:"Maximum wait in seconds. Default: 1800."`
	IncludeRaw       bool            `json:"include_raw,omitempty" jsonschema:"Include the service orderResult JSON string; use for language analysis or fields not represented by transcript."`
	TranscriptFormat string          `json:"transcript_format,omitempty" jsonschema:"Presentation: dialogue (default), text, timeline, speaker_grouped, srt, or vtt."`
	TaskFilePath     string          `json:"task_file_path,omitempty" jsonschema:"Read order references from a local 0600 continuation file created by IFASR submit."`
}

type IFASRResultOutput struct {
	OrderID             string                 `json:"order_id,omitempty"`
	Status              int                    `json:"status"`
	FailType            int                    `json:"fail_type,omitempty"`
	Transcript          string                 `json:"transcript,omitempty"`
	OriginalTranscript  string                 `json:"original_transcript,omitempty"`
	FormattedTranscript string                 `json:"formatted_transcript,omitempty"`
	TranscriptFormat    string                 `json:"transcript_format"`
	Speakers            []IFASRSpeakerResult   `json:"speakers,omitempty"`
	Utterances          []IFASRUtteranceResult `json:"utterances,omitempty"`
	RawResult           string                 `json:"raw_result,omitempty"`
	RawResponse         string                 `json:"raw_response,omitempty"`
	Language            string                 `json:"language,omitempty"`
	OriginalDurationMS  int64                  `json:"original_duration_ms,omitempty"`
	ExpireTime          int64                  `json:"expire_time,omitempty"`
	TaskEstimateTime    int64                  `json:"task_estimate_time_ms,omitempty"`
	Requests            []ifasr.RequestTrace   `json:"requests,omitempty"`
	Split               bool                   `json:"split"`
	Parts               []IFASRPartResult      `json:"parts,omitempty"`
	TaskFilePath        string                 `json:"task_file_path,omitempty"`
}

type IFASRPartResult struct {
	Index               int                    `json:"index"`
	OrderID             string                 `json:"order_id"`
	Status              int                    `json:"status"`
	FailType            int                    `json:"fail_type,omitempty"`
	Transcript          string                 `json:"transcript,omitempty"`
	OriginalTranscript  string                 `json:"original_transcript,omitempty"`
	FormattedTranscript string                 `json:"formatted_transcript,omitempty"`
	Utterances          []IFASRUtteranceResult `json:"utterances,omitempty"`
	Speakers            []IFASRSpeakerResult   `json:"speakers,omitempty"`
	RawResult           string                 `json:"raw_result,omitempty"`
	RawResponse         string                 `json:"raw_response,omitempty"`
	Language            string                 `json:"language,omitempty"`
	OriginalDurationMS  int64                  `json:"original_duration_ms,omitempty"`
	ExpireTime          int64                  `json:"expire_time,omitempty"`
	TaskEstimateTime    int64                  `json:"task_estimate_time_ms,omitempty"`
	Requests            []ifasr.RequestTrace   `json:"requests,omitempty"`
}

type IFASRUtteranceResult struct {
	Speaker string `json:"speaker,omitempty"`
	Track   string `json:"track,omitempty"`
	StartMS int64  `json:"start_ms,omitempty"`
	EndMS   int64  `json:"end_ms,omitempty"`
	Text    string `json:"text"`
}

type IFASRSpeakerResult struct {
	Speaker    string                `json:"speaker"`
	Track      string                `json:"track,omitempty"`
	Transcript string                `json:"transcript"`
	Segments   []IFASRSpeakerSegment `json:"segments,omitempty"`
}

type IFASRSpeakerSegment struct {
	StartMS    int64  `json:"start_ms,omitempty"`
	EndMS      int64  `json:"end_ms,omitempty"`
	Transcript string `json:"transcript"`
}

func addIFASRResultTool(server *mcp.Server, service *Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "xfyun_ifasr_result",
		Title:       "Get file transcription result",
		Description: "Query once or wait for one or more asynchronous recording transcription orders and merge split transcripts in order.",
		Annotations: annotations(true, false, true),
	}, func(ctx context.Context, req *mcp.CallToolRequest, input IFASRResultInput) (*mcp.CallToolResult, IFASRResultOutput, error) {
		variant := ifasr.Variant(input.Variant)
		if variant == "" {
			variant = ifasr.VariantLLM
		}
		if variant != ifasr.VariantLLM && variant != ifasr.VariantStandard {
			return nil, IFASRResultOutput{}, fmt.Errorf("variant must be llm or standard")
		}
		taskDurations := make(map[string]int64)
		if input.TaskFilePath != "" {
			task, err := ifasr.LoadTask(input.TaskFilePath)
			if err != nil {
				return nil, IFASRResultOutput{}, err
			}
			if input.Variant == "" {
				variant = task.Variant
			}
			if input.OrderID != "" || len(input.Orders) > 0 {
				return nil, IFASRResultOutput{}, fmt.Errorf("task_file_path cannot be combined with order_id or orders")
			}
			for _, part := range task.Parts {
				input.Orders = append(input.Orders, IFASROrderRef{OrderID: part.OrderID, SignatureRandom: part.SignatureRandom})
				if part.DurationMS > 0 {
					taskDurations[part.OrderID] = part.DurationMS
				}
			}
		}
		hasSingle := input.OrderID != ""
		if input.TaskFilePath == "" && hasSingle == (len(input.Orders) > 0) {
			return nil, IFASRResultOutput{}, fmt.Errorf("provide either order_id with signature_random, or orders")
		}
		if hasSingle && variant == ifasr.VariantLLM && input.SignatureRandom == "" {
			return nil, IFASRResultOutput{}, fmt.Errorf("order_id and signature_random are both required")
		}
		if variant == ifasr.VariantStandard && input.SignatureRandom != "" {
			return nil, IFASRResultOutput{}, fmt.Errorf("standard variant does not use signature_random")
		}
		if input.ResultType == "" {
			input.ResultType = "transfer"
		}
		if input.PollSeconds <= 0 {
			input.PollSeconds = 2
		}
		if input.MaxWaitSeconds <= 0 {
			input.MaxWaitSeconds = 1800
		}
		if input.TranscriptFormat == "" {
			input.TranscriptFormat = "dialogue"
		}
		client := ifasr.Client{Credentials: service.credentials, Variant: variant}
		orders := input.Orders
		if hasSingle {
			orders = []IFASROrderRef{{OrderID: input.OrderID, SignatureRandom: input.SignatureRandom}}
		}
		output := IFASRResultOutput{Split: len(orders) > 1, Status: 4, TranscriptFormat: input.TranscriptFormat, TaskFilePath: input.TaskFilePath}
		var transcripts, originals []string
		var speakers []IFASRSpeakerResult
		var utterances []IFASRUtteranceResult
		var utteranceOffset int64
		for index, order := range orders {
			if order.OrderID == "" || (variant == ifasr.VariantLLM && order.SignatureRandom == "") {
				return nil, IFASRResultOutput{}, fmt.Errorf("orders[%d] requires order_id%s", index, func() string {
					if variant == ifasr.VariantLLM {
						return " and signature_random"
					}
					return ""
				}())
			}
			if variant == ifasr.VariantStandard && order.SignatureRandom != "" {
				return nil, IFASRResultOutput{}, fmt.Errorf("orders[%d] for standard variant must not include signature_random", index)
			}
			part, err := queryIFASRPart(ctx, req, &client, order, input, index, len(orders), taskDurations[order.OrderID])
			if err != nil {
				return nil, IFASRResultOutput{}, fmt.Errorf("query IFASR part %d/%d: %w", index+1, len(orders), err)
			}
			part.Index = index + 1
			output.Parts = append(output.Parts, part)
			output.Requests = append(output.Requests, part.Requests...)
			output.Status = aggregateIFASRStatus(output.Status, part.Status)
			if part.Transcript != "" {
				transcripts = append(transcripts, part.Transcript)
			}
			if part.OriginalTranscript != "" {
				originals = append(originals, part.OriginalTranscript)
			}
			speakers = mergeIFASRSpeakers(speakers, part.Speakers)
			for _, utterance := range part.Utterances {
				utterance.StartMS += utteranceOffset
				utterance.EndMS += utteranceOffset
				utterances = append(utterances, utterance)
			}
			utteranceOffset += part.OriginalDurationMS
			output.OriginalDurationMS += part.OriginalDurationMS
		}
		output.Transcript = strings.Join(transcripts, "\n")
		output.OriginalTranscript = strings.Join(originals, "\n")
		output.Speakers = speakers
		output.Utterances = utterances
		formatted, err := ifasr.FormatTranscript(input.TranscriptFormat, output.Transcript, fromMCPUtterances(utterances), fromMCPSpeakers(speakers))
		if err != nil {
			return nil, IFASRResultOutput{}, err
		}
		output.FormattedTranscript = formatted
		if len(output.Parts) == 1 {
			part := output.Parts[0]
			output.OrderID, output.FailType, output.Language = part.OrderID, part.FailType, part.Language
			output.Speakers = part.Speakers
			output.RawResult, output.RawResponse = part.RawResult, part.RawResponse
			output.ExpireTime, output.TaskEstimateTime = part.ExpireTime, part.TaskEstimateTime
			output.Requests = part.Requests
			output.FormattedTranscript = part.FormattedTranscript
			output.Parts = nil
		}
		return nil, output, nil
	})
}

func aggregateIFASRStatus(current, next int) int {
	if current == -1 || next == -1 {
		return -1
	}
	if current == 3 || next == 3 {
		return 3
	}
	if current == 0 || next == 0 {
		return 0
	}
	if next != 4 {
		return next
	}
	return current
}

func queryIFASRPart(ctx context.Context, req *mcp.CallToolRequest, client *ifasr.Client, order IFASROrderRef, input IFASRResultInput, index, total int, fallbackDuration int64) (IFASRPartResult, error) {
	var result ifasr.Result
	if input.Wait {
		var err error
		result, err = client.Wait(ctx, order.OrderID, order.SignatureRandom, ifasr.Options{
			Variant: client.Variant, ResultType: input.ResultType, PollInterval: time.Duration(input.PollSeconds) * time.Second,
			MaxWait: time.Duration(input.MaxWaitSeconds) * time.Second,
			Progress: func(done, _ int, message string) {
				notifyProgress(ctx, req, float64(index+done), float64(total), fmt.Sprintf("IFASR part %d of %d: %s", index+1, total, message))
			},
		})
		if err != nil {
			return IFASRPartResult{}, err
		}
	} else {
		response, err := client.Query(ctx, order.OrderID, order.SignatureRandom, input.ResultType)
		if err != nil {
			return IFASRPartResult{}, err
		}
		result = ifasr.Result{OrderID: order.OrderID, SignatureRand: order.SignatureRandom, Status: response.Content.OrderInfo.Status, Response: response}
		result.Requests = []ifasr.RequestTrace{ifasr.NewQueryRequestTrace(client.Variant, order.OrderID, input.ResultType)}
		notifyProgress(ctx, req, float64(index+1), float64(total), fmt.Sprintf("IFASR part %d of %d checked", index+1, total))
		if result.Status == 4 {
			result.Transcript, result.OriginalTranscript, err = ifasr.ExtractTranscripts(response.Content.OrderResult)
			if err != nil {
				return IFASRPartResult{}, err
			}
			result.Speakers, err = ifasr.ExtractSpeakerTranscripts(response.Content.OrderResult)
			if err != nil {
				return IFASRPartResult{}, err
			}
			result.Utterances, err = ifasr.ExtractUtterances(response.Content.OrderResult)
			if err != nil {
				return IFASRPartResult{}, err
			}
		}
	}
	info := result.Response.Content.OrderInfo
	originalDuration := info.OriginalDuration
	if originalDuration == 0 {
		originalDuration = fallbackDuration
	}
	part := IFASRPartResult{
		OrderID: order.OrderID, Status: info.Status, FailType: info.FailType,
		Transcript: result.Transcript, OriginalTranscript: result.OriginalTranscript,
		Utterances: toMCPIFASRUtterances(result.Utterances),
		Speakers:   toMCPIFASRSpeakers(result.Speakers),
		Language:   info.Language, OriginalDurationMS: originalDuration,
		ExpireTime: info.ExpireTime, TaskEstimateTime: result.Response.Content.TaskEstimateTime,
		Requests: result.Requests,
	}
	formatted, formatErr := ifasr.FormatTranscript(input.TranscriptFormat, part.Transcript, result.Utterances, result.Speakers)
	if formatErr != nil {
		return IFASRPartResult{}, formatErr
	}
	part.FormattedTranscript = formatted
	if input.IncludeRaw {
		part.RawResult = result.Response.Content.OrderResult
		encoded, err := json.Marshal(result.Response)
		if err != nil {
			return IFASRPartResult{}, fmt.Errorf("encode IFASR raw response: %w", err)
		}
		part.RawResponse = string(encoded)
	}
	return part, nil
}

func toMCPIFASRSpeakers(speakers []ifasr.SpeakerTranscript) []IFASRSpeakerResult {
	if len(speakers) == 0 {
		return nil
	}
	result := make([]IFASRSpeakerResult, 0, len(speakers))
	for _, speaker := range speakers {
		converted := IFASRSpeakerResult{Speaker: speaker.Speaker, Track: speaker.Track, Transcript: speaker.Transcript}
		for _, segment := range speaker.Segments {
			converted.Segments = append(converted.Segments, IFASRSpeakerSegment{StartMS: segment.StartMS, EndMS: segment.EndMS, Transcript: segment.Transcript})
		}
		result = append(result, converted)
	}
	return result
}

func toMCPIFASRUtterances(utterances []ifasr.Utterance) []IFASRUtteranceResult {
	if len(utterances) == 0 {
		return nil
	}
	result := make([]IFASRUtteranceResult, 0, len(utterances))
	for _, utterance := range utterances {
		result = append(result, IFASRUtteranceResult{Speaker: utterance.Speaker, Track: utterance.Track, StartMS: utterance.StartMS, EndMS: utterance.EndMS, Text: utterance.Text})
	}
	return result
}

func fromMCPUtterances(utterances []IFASRUtteranceResult) []ifasr.Utterance {
	if len(utterances) == 0 {
		return nil
	}
	result := make([]ifasr.Utterance, 0, len(utterances))
	for _, utterance := range utterances {
		result = append(result, ifasr.Utterance{Speaker: utterance.Speaker, Track: utterance.Track, StartMS: utterance.StartMS, EndMS: utterance.EndMS, Text: utterance.Text})
	}
	return result
}

func fromMCPSpeakers(speakers []IFASRSpeakerResult) []ifasr.SpeakerTranscript {
	if len(speakers) == 0 {
		return nil
	}
	result := make([]ifasr.SpeakerTranscript, 0, len(speakers))
	for _, speaker := range speakers {
		converted := ifasr.SpeakerTranscript{Speaker: speaker.Speaker, Track: speaker.Track, Transcript: speaker.Transcript}
		for _, segment := range speaker.Segments {
			converted.Segments = append(converted.Segments, ifasr.SpeakerSegment{StartMS: segment.StartMS, EndMS: segment.EndMS, Transcript: segment.Transcript})
		}
		result = append(result, converted)
	}
	return result
}

func mergeIFASRSpeakers(current, next []IFASRSpeakerResult) []IFASRSpeakerResult {
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

type MediaInput struct {
	Operation   string `json:"operation" jsonschema:"info or convert"`
	InputPath   string `json:"input_path" jsonschema:"Local audio path"`
	OutputPath  string `json:"output_path,omitempty" jsonschema:"Converted audio destination; required for convert"`
	Channels    int    `json:"channels,omitempty" jsonschema:"Output channels: 1 mono or 2 stereo"`
	SampleRate  int    `json:"sample_rate,omitempty" jsonschema:"Output sample rate in Hz"`
	Bitrate     string `json:"bitrate,omitempty" jsonschema:"Output audio bitrate such as 64k or 128k"`
	Force       bool   `json:"force,omitempty" jsonschema:"Allow replacement of output_path"`
	TimeoutSecs int    `json:"timeout_seconds,omitempty" jsonschema:"Operation timeout in seconds; default 120"`
}

type MediaOutput struct {
	Operation  string     `json:"operation"`
	OutputPath string     `json:"output_path,omitempty"`
	Info       media.Info `json:"info"`
}

func addMediaTool(server *mcp.Server, service *Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "xfyun_media",
		Title:       "Inspect or convert audio media",
		Description: "Probe audio sample rate, channels, codec, bitrate, and duration, or convert local audio between mono/stereo, sample rates, and bitrates. Conversion never replaces an existing output unless force=true.",
		Annotations: annotations(false, true, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input MediaInput) (*mcp.CallToolResult, MediaOutput, error) {
		if input.InputPath == "" {
			return nil, MediaOutput{}, fmt.Errorf("input_path is required")
		}
		if input.Operation == "" {
			input.Operation = "info"
		}
		requestCtx, cancel := withTimeout(ctx, input.TimeoutSecs, 120)
		defer cancel()
		if input.Operation == "info" {
			info, err := media.Probe(requestCtx, input.InputPath)
			if err != nil {
				return nil, MediaOutput{}, err
			}
			return nil, MediaOutput{Operation: "info", Info: info}, nil
		}
		if input.Operation != "convert" {
			return nil, MediaOutput{}, fmt.Errorf("unsupported media operation %q", input.Operation)
		}
		if input.OutputPath == "" {
			return nil, MediaOutput{}, fmt.Errorf("output_path is required for media conversion")
		}
		absolute, temporary, err := prepareAtomicOutput(input.OutputPath, input.Force)
		if err != nil {
			return nil, MediaOutput{}, err
		}
		temporaryPath := temporary.Name()
		if err := temporary.Close(); err != nil {
			_ = os.Remove(temporaryPath)
			return nil, MediaOutput{}, fmt.Errorf("close temporary media output: %w", err)
		}
		committed := false
		defer func() {
			if !committed {
				_ = os.Remove(temporaryPath)
			}
		}()
		if err := media.Convert(requestCtx, input.InputPath, temporaryPath, media.ConvertOptions{Channels: input.Channels, SampleRate: input.SampleRate, Bitrate: input.Bitrate, Format: filepath.Ext(absolute)}); err != nil {
			return nil, MediaOutput{}, err
		}
		if err := outputfile.Commit(temporaryPath, absolute, input.Force); err != nil {
			return nil, MediaOutput{}, err
		}
		committed = true
		info, err := media.Probe(requestCtx, absolute)
		if err != nil {
			return nil, MediaOutput{}, err
		}
		return nil, MediaOutput{Operation: "convert", OutputPath: absolute, Info: info}, nil
	})
}

func annotations(readOnly, destructive, openWorld bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint: readOnly, DestructiveHint: boolPointer(destructive),
		OpenWorldHint: boolPointer(openWorld), IdempotentHint: readOnly,
	}
}

func boolPointer(value bool) *bool { return &value }

func intValue(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func diagnosticOptionalBool(value *bool) string {
	if value == nil {
		return "service-default"
	}
	return fmt.Sprintf("%t", *value)
}

func isSensitiveDiagnosticKey(key string) bool {
	key = strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(key))
	for _, marker := range []string{"token", "secret", "signature", "signa", "password", "apikey", "accesskey", "authorization"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	copy := make(map[string]string, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func withTimeout(ctx context.Context, seconds, defaultSeconds int) (context.Context, context.CancelFunc) {
	if seconds <= 0 {
		seconds = defaultSeconds
	}
	return context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
}

func resolveText(text, textPath string) (string, error) {
	if (text == "") == (textPath == "") {
		return "", fmt.Errorf("provide exactly one of text or text_path")
	}
	if text != "" {
		return text, nil
	}
	file, err := os.Open(textPath)
	if err != nil {
		return "", fmt.Errorf("open text file: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("read text file: %w", err)
	}
	return string(data), nil
}

func prepareAtomicOutput(outputPath string, force bool) (string, *os.File, error) {
	return outputfile.Prepare(outputPath, force)
}
