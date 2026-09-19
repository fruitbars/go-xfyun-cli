package ocr

import "encoding/json"

// ResultFormats contains the readable Markdown and structured SED document
// formats returned by the OCR large-model response. The raw decoded response
// remains available to callers that need the full layout tree.
type ResultFormats struct {
	Markdown string
	SED      []SEDElement
}

// SEDElement preserves one service-defined structured element. The service
// may add fields over time, so the map retains data not known by this client.
type SEDElement map[string]any

// ExtractResultFormats extracts document-level Markdown and SED values without
// walking the large image/coordinate tree. Plain-text or unknown result
// payloads simply return empty formats.
func ExtractResultFormats(data []byte) ResultFormats {
	var wire struct {
		Document []struct {
			Name  string          `json:"name"`
			Value json.RawMessage `json:"value"`
		} `json:"document"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return ResultFormats{}
	}
	formats := ResultFormats{}
	for _, document := range wire.Document {
		switch document.Name {
		case "markdown":
			_ = json.Unmarshal(document.Value, &formats.Markdown)
		case "sed":
			_ = json.Unmarshal(document.Value, &formats.SED)
		}
	}
	return formats
}
