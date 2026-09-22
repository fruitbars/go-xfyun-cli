package ocr

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
)

func TestParsePageSelection(t *testing.T) {
	pages, err := parsePageSelection("3,1-2,2", 4)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 3}
	for index := range want {
		if pages[index] != want[index] {
			t.Fatalf("pages = %v, want %v", pages, want)
		}
	}
	if _, err := parsePageSelection("2-5", 4); err == nil {
		t.Fatal("expected out-of-range page error")
	}
}

func TestStreamPathRendersTextPDFAtDefaultDPI(t *testing.T) {
	const encodedPDF = "JVBERi0xLjcKJaDypPQKMSAwIG9iaiA8PAogIC9UeXBlIC9DYXRhbG9nCiAgL1BhZ2VzIDIgMCBSCj4+CmVuZG9iagoyIDAgb2JqIDw8CiAgL1R5cGUgL1BhZ2VzCiAgL01lZGlhQm94IFswIDAgMjAwIDIwMF0KICAvQ291bnQgMQogIC9LaWRzIFszIDAgUl0KPj4KZW5kb2JqCjMgMCBvYmogPDwKICAvVHlwZSAvUGFnZQogIC9QYXJlbnQgMiAwIFIKICAvUmVzb3VyY2VzIDw8CiAgICAvRm9udCA8PAogICAgICAvRjEgNCAwIFIKICAgICAgL0YyIDUgMCBSCiAgICA+PgogID4+CiAgL0NvbnRlbnRzIDYgMCBSCj4+CmVuZG9iago0IDAgb2JqIDw8CiAgL1R5cGUgL0ZvbnQKICAvU3VidHlwZSAvVHlwZTEKICAvQmFzZUZvbnQgL1RpbWVzLVJvbWFuCj4+CmVuZG9iago1IDAgb2JqIDw8CiAgL1R5cGUgL0ZvbnQKICAvU3VidHlwZSAvVHlwZTEKICAvQmFzZUZvbnQgL0hlbHZldGljYQo+PgplbmRvYmoKNiAwIG9iaiA8PAogICUgTm90ZSB0aGlzIG9iamVjdCBkZWxpYmVyYXRlbHkgZG9lcyBub3QgdXNlIC9MZW5ndGggODMuCj4+CnN0cmVhbQpCVAoyMCA1MCBUZAovRjEgMTIgVGYKKEhlbGxvLCB3b3JsZCEpIFRqCjAgNTAgVGQKL0YyIDE2IFRmCihHb29kYnllLCB3b3JsZCEpIFRqCkVUCmVuZHN0cmVhbQplbmRvYmoKeHJlZgowIDcKMDAwMDAwMDAwMCA2NTUzNSBmIAowMDAwMDAwMDE1IDAwMDAwIG4gCjAwMDAwMDAwNjggMDAwMDAgbiAKMDAwMDAwMDE1NyAwMDAwMCBuIAowMDAwMDAwMjk5IDAwMDAwIG4gCjAwMDAwMDAzNzcgMDAwMDAgbiAKMDAwMDAwMDQ1MyAwMDAwMCBuIAp0cmFpbGVyIDw8CiAgL1Jvb3QgMSAwIFIKICAvU2l6ZSA3Cj4+CnN0YXJ0eHJlZgo2MzMKJSVFT0YK"
	data, err := base64.StdEncoding.DecodeString(encodedPDF)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "text.pdf")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var calls int
	err = StreamPath(ctx, path, "", 0, func(page DocumentImage) error {
		calls++
		if page.Page != 1 || page.PageCount != 1 || page.DPI != DefaultPDFDPI || page.Encoding != "jpg" {
			t.Fatalf("unexpected page metadata: %+v", page)
		}
		if page.Width < 400 || page.Height < 400 {
			t.Fatalf("default 150 DPI render is too small: %dx%d", page.Width, page.Height)
		}
		if len(page.Data) == 0 || len(page.Data) > MaxUploadImageBytes {
			t.Fatalf("rendered page size = %d", len(page.Data))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("callback calls = %d, want 1", calls)
	}
}

type fixedPageSizer struct {
	width  float64
	height float64
}

func (s fixedPageSizer) FPDF_GetPageSizeByIndex(*requests.FPDF_GetPageSizeByIndex) (*responses.FPDF_GetPageSizeByIndex, error) {
	return &responses.FPDF_GetPageSizeByIndex{Width: s.width, Height: s.height}, nil
}

func TestBoundedPageDPILimitsTemporaryBitmap(t *testing.T) {
	dpi, err := boundedPageDPI(fixedPageSizer{width: 595, height: 842}, references.FPDF_DOCUMENT(""), 0, 150)
	if err != nil {
		t.Fatal(err)
	}
	if dpi != 150 {
		t.Fatalf("A4 DPI = %d, want 150", dpi)
	}

	dpi, err = boundedPageDPI(fixedPageSizer{width: 2000, height: 2000}, references.FPDF_DOCUMENT(""), 0, 300)
	if err != nil {
		t.Fatal(err)
	}
	if dpi >= 300 || dpi < 72 {
		t.Fatalf("bounded DPI = %d, want 72..299", dpi)
	}
	if pixels := renderedPixels(2000, 2000, dpi); pixels > MaxRenderedPagePixels {
		t.Fatalf("bounded render pixels = %d, limit %d", pixels, MaxRenderedPagePixels)
	}

	if _, err := boundedPageDPI(fixedPageSizer{width: 10000, height: 10000}, references.FPDF_DOCUMENT(""), 0, 150); err == nil {
		t.Fatal("expected oversized page error at minimum DPI")
	}
}

func TestLargePDFConfirmationError(t *testing.T) {
	err := (&LargePDFConfirmationError{Pages: 1001}).Error()
	if !strings.Contains(err, "confirm_large_pdf") || !strings.Contains(err, "--confirm-large-pdf") {
		t.Fatalf("confirmation error = %q", err)
	}
}
