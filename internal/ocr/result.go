package ocr

import "encoding/json"

// ResultFormats contains the human-readable document formats returned by the
// OCR large-model response. The raw decoded response remains available to
// callers that need coordinates, font attributes, or other image details.
type ResultFormats struct {
	Markdown string
	SED      string
}

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
		var value string
		if err := json.Unmarshal(document.Value, &value); err != nil {
			continue
		}
		switch document.Name {
		case "markdown":
			formats.Markdown = value
		case "sed":
			formats.SED = value
		}
	}
	return formats
}
