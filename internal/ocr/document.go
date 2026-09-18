package ocr

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
)

const (
	MaxPDFBytes      int64 = 500 * 1024 * 1024
	MaxDocumentPages       = 200
	DefaultPDFDPI          = 150
	// MaxRenderedPagePixels limits the temporary PDFium bitmap to roughly
	// 160 MiB at four bytes per pixel. Oversized pages are rendered at a lower
	// DPI (never below 72) so one unusual page cannot exhaust process memory.
	MaxRenderedPagePixels int64 = 40_000_000
	// A WebAssembly page is 64 KiB. This caps one PDFium runtime at 512 MiB
	// instead of wazero's default 4 GiB maximum.
	pdfWASMMemoryLimitPages uint32 = 8192
)

// PDF rendering is intentionally serialized process-wide. MCP hosts may issue
// concurrent tool calls, and one PDFium runtime is already capable of using a
// substantial bounded working set.
var pdfRenderSlot = make(chan struct{}, 1)

// DocumentImage is one page image to submit to OCR. The callback passed to
// StreamPath must consume Data before returning; PDF rendering reuses and
// releases page resources between callbacks.
type DocumentImage struct {
	Page      int
	Index     int
	PageCount int
	DPI       int
	Data      []byte
	Encoding  string
	Width     int
	Height    int
}

// StreamPath reads a raster image once, or renders and emits one PDF page at a
// time through the embedded pure-Go WebAssembly PDFium backend. No CGO or
// system PDF renderer is required.
func StreamPath(ctx context.Context, path, pageSelection string, dpi int, consume func(DocumentImage) error) error {
	if consume == nil {
		return fmt.Errorf("OCR document consumer is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open OCR input: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat OCR input: %w", err)
	}
	header := make([]byte, 1024)
	n, readErr := file.Read(header)
	if readErr != nil && readErr != io.EOF {
		return fmt.Errorf("read OCR input header: %w", readErr)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind OCR input: %w", err)
	}
	if isPDF(header[:n]) {
		if info.Size() > MaxPDFBytes {
			return fmt.Errorf("PDF is %d bytes; local limit is 500 MiB", info.Size())
		}
		return streamPDF(ctx, file, info.Size(), pageSelection, dpi, consume)
	}
	if info.Size() > MaxSourceImageBytes {
		return fmt.Errorf("source image is %d bytes; local compression limit is %d MiB", info.Size(), MaxSourceImageBytes/(1024*1024))
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxSourceImageBytes+1))
	if err != nil {
		return fmt.Errorf("read OCR image: %w", err)
	}
	encoding, err := DetectEncoding(data)
	if err != nil {
		return err
	}
	return consume(DocumentImage{Page: 1, Index: 1, PageCount: 1, Data: data, Encoding: encoding})
}

func streamPDF(ctx context.Context, reader io.ReadSeeker, size int64, selection string, dpi int, consume func(DocumentImage) error) error {
	if dpi == 0 {
		dpi = DefaultPDFDPI
	}
	if dpi < 72 || dpi > 300 {
		return fmt.Errorf("PDF DPI must be between 72 and 300")
	}
	select {
	case pdfRenderSlot <- struct{}{}:
		defer func() { <-pdfRenderSlot }()
	case <-ctx.Done():
		return ctx.Err()
	}
	pool, err := webassembly.Init(webassembly.Config{
		Context: ctx, MinIdle: 0, MaxIdle: 0, MaxTotal: 1, ReuseWorkers: false,
		RuntimeConfig: wazero.NewRuntimeConfig().
			WithMemoryLimitPages(pdfWASMMemoryLimitPages).
			WithCloseOnContextDone(true),
	})
	if err != nil {
		return fmt.Errorf("initialize pure-Go PDF renderer: %w", err)
	}
	defer pool.Close()
	instance, err := pool.GetInstanceWithContext(ctx)
	if err != nil {
		return fmt.Errorf("start pure-Go PDF renderer: %w", err)
	}
	defer instance.Close()
	document, err := instance.OpenDocument(&requests.OpenDocument{FileReader: reader, FileReaderSize: size})
	if err != nil {
		return fmt.Errorf("open PDF: %w", err)
	}
	defer instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: document.Document})
	count, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: document.Document})
	if err != nil {
		return fmt.Errorf("read PDF page count: %w", err)
	}
	pages, err := parsePageSelection(selection, count.PageCount)
	if err != nil {
		return err
	}
	if len(pages) > MaxDocumentPages {
		return fmt.Errorf("PDF selection contains %d pages; select at most %d per request", len(pages), MaxDocumentPages)
	}
	for _, page := range pages {
		if err := ctx.Err(); err != nil {
			return err
		}
		effectiveDPI, err := boundedPageDPI(instance, document.Document, page-1, dpi)
		if err != nil {
			return fmt.Errorf("inspect PDF page %d: %w", page, err)
		}
		rendered, err := instance.RenderToFile(&requests.RenderToFile{
			RenderPageInDPI: &requests.RenderPageInDPI{
				DPI: effectiveDPI, RenderForm: true, Document: &document.Document,
				Page: requests.Page{ByIndex: &requests.PageByIndex{Document: document.Document, Index: page - 1}},
			},
			OutputFormat:  requests.RenderToFileOutputFormatJPG,
			OutputTarget:  requests.RenderToFileOutputTargetBytes,
			OutputQuality: 90,
			MaxFileSize:   MaxUploadImageBytes,
		})
		if err != nil {
			return fmt.Errorf("render PDF page %d at %d DPI: %w", page, effectiveDPI, err)
		}
		if rendered.ImageBytes == nil || len(*rendered.ImageBytes) == 0 {
			return fmt.Errorf("render PDF page %d: renderer returned no image", page)
		}
		if len(*rendered.ImageBytes) > MaxUploadImageBytes {
			return fmt.Errorf("render PDF page %d: JPEG exceeds the 4 MiB upload limit", page)
		}
		if err := consume(DocumentImage{
			Page: page, Index: 1, PageCount: len(pages), DPI: effectiveDPI,
			Data: *rendered.ImageBytes, Encoding: "jpg", Width: rendered.Width, Height: rendered.Height,
		}); err != nil {
			return err
		}
		// rendered and its byte slice become unreachable here before the next
		// page is rendered, keeping page-image memory bounded.
	}
	return nil
}

type pageSizer interface {
	FPDF_GetPageSizeByIndex(*requests.FPDF_GetPageSizeByIndex) (*responses.FPDF_GetPageSizeByIndex, error)
}

func boundedPageDPI(instance pageSizer, document references.FPDF_DOCUMENT, pageIndex, requestedDPI int) (int, error) {
	size, err := instance.FPDF_GetPageSizeByIndex(&requests.FPDF_GetPageSizeByIndex{
		Document: document,
		Index:    pageIndex,
	})
	if err != nil {
		return 0, err
	}
	if size.Width <= 0 || size.Height <= 0 {
		return 0, fmt.Errorf("invalid page dimensions %.2fx%.2f points", size.Width, size.Height)
	}
	dpi := requestedDPI
	for renderedPixels(size.Width, size.Height, dpi) > MaxRenderedPagePixels {
		maximum := int(math.Floor(72 * math.Sqrt(float64(MaxRenderedPagePixels)/(size.Width*size.Height))))
		if maximum >= dpi {
			maximum = dpi - 1
		}
		dpi = maximum
		if dpi < 72 {
			return 0, fmt.Errorf("page dimensions %.2fx%.2f points exceed the %d-megapixel render limit at 72 DPI", size.Width, size.Height, MaxRenderedPagePixels/1_000_000)
		}
	}
	return dpi, nil
}

func renderedPixels(widthPoints, heightPoints float64, dpi int) int64 {
	width := math.Ceil(widthPoints * float64(dpi) / 72)
	height := math.Ceil(heightPoints * float64(dpi) / 72)
	pixels := width * height
	const maxInt64AsFloat = float64(^uint64(0) >> 1)
	if math.IsNaN(pixels) || math.IsInf(pixels, 0) || pixels <= 0 || pixels >= maxInt64AsFloat {
		return int64(^uint64(0) >> 1)
	}
	return int64(pixels)
}

func isPDF(data []byte) bool {
	return bytes.Contains(data, []byte("%PDF-"))
}

func parsePageSelection(selection string, pageCount int) ([]int, error) {
	if pageCount <= 0 {
		return nil, fmt.Errorf("PDF contains no pages")
	}
	if strings.TrimSpace(selection) == "" {
		pages := make([]int, pageCount)
		for index := range pages {
			pages[index] = index + 1
		}
		return pages, nil
	}
	selected := map[int]struct{}{}
	for _, part := range strings.Split(selection, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("invalid empty PDF page selector")
		}
		startText, endText, isRange := strings.Cut(part, "-")
		start, err := strconv.Atoi(strings.TrimSpace(startText))
		if err != nil {
			return nil, fmt.Errorf("invalid PDF page %q", part)
		}
		end := start
		if isRange {
			end, err = strconv.Atoi(strings.TrimSpace(endText))
			if err != nil || end < start {
				return nil, fmt.Errorf("invalid PDF page range %q", part)
			}
		}
		if start < 1 || end > pageCount {
			return nil, fmt.Errorf("PDF page selection %q is outside 1-%d", part, pageCount)
		}
		for page := start; page <= end; page++ {
			selected[page] = struct{}{}
		}
	}
	pages := make([]int, 0, len(selected))
	for page := range selected {
		pages = append(pages, page)
	}
	sort.Ints(pages)
	return pages, nil
}
