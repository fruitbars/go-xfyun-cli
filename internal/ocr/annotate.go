package ocr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"sort"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

var defaultAnnotationTypes = []string{"paragraph", "title", "table", "graph", "list", "formula", "code", "pseudocode", "seal", "fingerprint", "barcode", "qrcode", "watermark", "annotation", "footnote", "key", "value", "contents"}

// SupportedAnnotationTypes includes every documented layout element plus the
// markable cell and item container nodes that occur in the service response.
var SupportedAnnotationTypes = []string{
	"page", "layout", "region", "page_header", "title", "paragraph", "textline",
	"table", "cell", "graph", "list", "item", "formula", "code", "pseudocode", "information_bar",
	"seal", "fingerprint", "barcode", "qrcode", "watermark", "page_footer", "page_number", "annotation",
	"footnote", "key", "value", "contents",
}

type Annotation struct {
	Type   string
	Points []image.Point
}

type annotationGeometry struct {
	Width  float64
	Height float64
	Angle  float64
}

// ParseAnnotationTypes normalizes a comma-separated type selection. Empty
// input deliberately selects high-level layout nodes instead of every nested
// text unit and coordinate node.
func ParseAnnotationTypes(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return append([]string(nil), defaultAnnotationTypes...), nil
	}
	allowed := make(map[string]bool, len(SupportedAnnotationTypes))
	for _, item := range SupportedAnnotationTypes {
		allowed[item] = true
	}
	seen := make(map[string]struct{})
	var types []string
	all := false
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if item == "all" {
			all = true
			continue
		}
		if !allowed[item] {
			return nil, fmt.Errorf("unsupported OCR annotation type %q; supported: %s", item, strings.Join(SupportedAnnotationTypes, ","))
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		types = append(types, item)
	}
	if all {
		return append([]string(nil), SupportedAnnotationTypes...), nil
	}
	sort.Strings(types)
	if len(types) == 0 {
		return nil, fmt.Errorf("OCR annotation_types does not select any types")
	}
	return types, nil
}

// AnnotateImage draws selected OCR node types over the source image and
// returns a PNG. OCR coordinates are scaled when the service reports image
// dimensions that differ from the source bitmap.
func AnnotateImage(source, response []byte, typeSelection string) ([]byte, int, error) {
	decoded, _, err := image.Decode(bytes.NewReader(source))
	if err != nil {
		return nil, 0, fmt.Errorf("decode source image for OCR annotation: %w", err)
	}
	types, err := ParseAnnotationTypes(typeSelection)
	if err != nil {
		return nil, 0, err
	}
	annotations, geometry, err := collectAnnotations(response, types)
	if err != nil {
		return nil, 0, err
	}
	canvas := image.NewRGBA(image.Rect(0, 0, decoded.Bounds().Dx(), decoded.Bounds().Dy()))
	draw.Draw(canvas, canvas.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	for _, item := range annotations {
		drawAnnotation(canvas, item, geometry)
	}
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return nil, 0, fmt.Errorf("encode OCR annotation PNG: %w", err)
	}
	return output.Bytes(), len(annotations), nil
}

func collectAnnotations(data []byte, selected []string) ([]Annotation, annotationGeometry, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&value); err != nil {
		return nil, annotationGeometry{}, fmt.Errorf("decode OCR result for annotation: %w", err)
	}
	allowed := make(map[string]bool, len(selected))
	for _, item := range selected {
		allowed[item] = true
	}
	var annotations []Annotation
	var geometry annotationGeometry
	if root, ok := value.(map[string]any); ok {
		if images, ok := root["image"].([]any); ok && len(images) > 0 {
			if first, ok := images[0].(map[string]any); ok {
				geometry.Width = jsonFloat(first["width"])
				geometry.Height = jsonFloat(first["height"])
				geometry.Angle = jsonFloat(first["angle"])
			}
		}
	}
	var visit func(any)
	visit = func(current any) {
		object, ok := current.(map[string]any)
		if !ok {
			if list, ok := current.([]any); ok {
				for _, item := range list {
					visit(item)
				}
			}
			return
		}
		typeName, _ := object["type"].(string)
		if allowed[typeName] {
			points := objectPoints(object["coord"])
			if len(points) < 2 {
				points = objectPoints(object["contour"])
			}
			if len(points) >= 2 {
				annotations = append(annotations, Annotation{Type: typeName, Points: points})
			}
		}
		for _, child := range object {
			visit(child)
		}
	}
	visit(value)
	return annotations, geometry, nil
}

func objectPoints(value any) []image.Point {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	points := make([]image.Point, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		x, xOK := jsonNumber(object["x"])
		y, yOK := jsonNumber(object["y"])
		if !xOK || !yOK || math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
			continue
		}
		points = append(points, image.Point{X: int(math.Round(x)), Y: int(math.Round(y))})
	}
	return points
}

func jsonFloat(value any) float64 {
	number, _ := jsonNumber(value)
	return number
}

func jsonNumber(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func drawAnnotation(canvas *image.RGBA, annotation Annotation, geometry annotationGeometry) {
	if len(annotation.Points) < 2 {
		return
	}
	sourcePoints := annotation.Points
	if len(sourcePoints) == 2 {
		sourcePoints = []image.Point{sourcePoints[0], {X: sourcePoints[1].X, Y: sourcePoints[0].Y}, sourcePoints[1], {X: sourcePoints[0].X, Y: sourcePoints[1].Y}}
	}
	points := make([]image.Point, len(sourcePoints))
	minX, minY := canvas.Bounds().Max.X, canvas.Bounds().Max.Y
	for index, point := range sourcePoints {
		points[index] = transformAnnotationPoint(point, geometry, canvas.Bounds().Dx(), canvas.Bounds().Dy())
		minX, minY = min(minX, points[index].X), min(minY, points[index].Y)
	}
	lineColor := annotationColor(annotation.Type)
	for index := range points {
		drawLine(canvas, points[index], points[(index+1)%len(points)], lineColor)
	}
	label := annotation.Type
	face := basicfont.Face7x13
	labelWidth := font.MeasureString(face, label).Ceil() + 6
	labelHeight := face.Metrics().Height.Ceil()
	labelX, labelY := minX, minY-labelHeight
	if labelY < 0 {
		labelY = minY
	}
	if labelX+labelWidth >= canvas.Bounds().Max.X {
		labelX = max(0, canvas.Bounds().Max.X-labelWidth-1)
	}
	background := image.NewUniform(lineColor)
	draw.Draw(canvas, image.Rect(labelX, labelY, min(canvas.Bounds().Max.X, labelX+labelWidth), min(canvas.Bounds().Max.Y, labelY+labelHeight+2)), background, image.Point{}, draw.Src)
	drawer := font.Drawer{Dst: canvas, Src: image.NewUniform(color.White), Face: face, Dot: fixed.P(labelX+3, labelY+face.Metrics().Ascent.Ceil())}
	drawer.DrawString(label)
}

func transformAnnotationPoint(point image.Point, geometry annotationGeometry, outputWidth, outputHeight int) image.Point {
	responseWidth, responseHeight := geometry.Width, geometry.Height
	if responseWidth <= 0 || responseHeight <= 0 {
		responseWidth, responseHeight = float64(outputWidth), float64(outputHeight)
	}
	angle := math.Mod(geometry.Angle, 360)
	if angle < 0 {
		angle += 360
	}
	radians := angle * math.Pi / 180
	correctedWidth := math.Abs(responseWidth*math.Cos(radians)) + math.Abs(responseHeight*math.Sin(radians))
	correctedHeight := math.Abs(responseWidth*math.Sin(radians)) + math.Abs(responseHeight*math.Cos(radians))
	// The service reports coordinates in its deskewed coordinate system. Rotate
	// them back around the image center before scaling to the local source.
	inverse := (360 - angle) * math.Pi / 180
	x := float64(point.X) - correctedWidth/2
	y := float64(point.Y) - correctedHeight/2
	rotatedX := math.Cos(inverse)*x - math.Sin(inverse)*y + responseWidth/2
	rotatedY := math.Sin(inverse)*x + math.Cos(inverse)*y + responseHeight/2
	outputX := int(math.Round(rotatedX * float64(outputWidth) / responseWidth))
	outputY := int(math.Round(rotatedY * float64(outputHeight) / responseHeight))
	return image.Point{X: clamp(outputX, 0, outputWidth-1), Y: clamp(outputY, 0, outputHeight-1)}
}

func drawLine(canvas *image.RGBA, from, to image.Point, line color.Color) {
	dx, dy := abs(to.X-from.X), -abs(to.Y-from.Y)
	sx, sy := 1, 1
	if from.X > to.X {
		sx = -1
	}
	if from.Y > to.Y {
		sy = -1
	}
	err := dx + dy
	for {
		if canvas.Bounds().Max.X > from.X && canvas.Bounds().Max.Y > from.Y && from.X >= 0 && from.Y >= 0 {
			canvas.Set(from.X, from.Y, line)
		}
		if from == to {
			return
		}
		twice := 2 * err
		if twice >= dy {
			err += dy
			from.X += sx
		}
		if twice <= dx {
			err += dx
			from.Y += sy
		}
	}
}

func annotationColor(typeName string) color.RGBA {
	colors := []color.RGBA{{R: 208, G: 64, B: 64, A: 255}, {R: 32, G: 112, B: 192, A: 255}, {R: 32, G: 144, B: 88, A: 255}, {R: 160, G: 96, B: 32, A: 255}, {R: 128, G: 64, B: 160, A: 255}}
	hash := 0
	for _, character := range typeName {
		hash = (hash*31 + int(character)) & 0x7fffffff
	}
	return colors[hash%len(colors)]
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
