# OCR parameter guide

Use `xfyun_ocr` for a local image or PDF. The API-native image encodings are `jpg`, `jpeg`, `png`, and `bmp`; the tool detects content and converts decodable formats such as GIF, WebP, and TIFF to JPEG/PNG in pure Go. It accepts source images up to 32 MiB and automatically compresses oversized input so the uploaded image is at most 4 MiB raw and 10 MiB after base64 encoding. It preserves PNG when `alpha_option="1"`; otherwise an oversized or converted image may become high-quality JPEG.

PDFs are rendered with embedded PDFium WebAssembly, without CGO or an installed renderer. Text, vector, and scanned pages are supported. The entire pipeline is page-streamed: render one page, compress, OCR, write its NDJSON result, release the page, then continue. Use `pages="1-3,5"` to select pages and `pdf_dpi` from 72–300; default 150 DPI. Temporary page bitmaps are capped at 40 million pixels, with automatic DPI reduction down to 72 for oversized pages. The PDFium runtime has a 512 MiB hard memory limit, and concurrent PDFs are serialized inside one server process. A request supports at most 200 selected pages and a 500 MiB PDF.

For one image/page, `xfyun_ocr` returns `text` directly. For multiple selected pages it writes one `OCRPageOutput` JSON object per line and returns `output_path`, `output_format="ndjson"`, and `page_count`; read that file incrementally. Set `output_path` when the user wants a stable destination. If omitted, the tool creates a temporary file and reports `auto_generated=true`. Set `force=true` only after the user authorizes replacing an existing destination. Hosts that send an MCP progress token receive a notification after each page.

## Map user intent to arguments

- “Return Markdown / preserve document structure”: `result_format="json,markdown"` (default).
- “Also return SED”: `result_format="json,sed"` or `"json,markdown,sed"`.
- OCR tool responses expose extracted `markdown`/`sed` fields and use the readable result as `text`; pass `include_raw=true` to also return the full decoded JSON as `raw`.
- “Mark layout types on the image”: `annotate=true`. Use `annotation_types="paragraph,title,table"` or `"all"`; optionally set `annotation_output_path` to a PNG for one page or a directory for multiple pages. A single image is also returned as MCP `image/png` when it is at most 8 MiB.
- “Character boxes / character-level coordinates”: `result_option="normal,char"`.
- “Do not return line coordinates”: `result_option="normal,no_line_position"`; combine both as `"normal,char,no_line_position"`.
- “Recognize a slightly rotated scan”: lower or raise `rotation_min_angle` within 0–180 degrees. Default is 5.
- “Honor camera orientation”: `exif_option="1"`.
- “Honor transparency”: `alpha_option="1"` for images whose alpha channel affects visible content.

`markdown_options` and `sed_options` are comma-separated `name=value` controls. Use only when the user asks to include or exclude layout elements. Documented elements include seals, QR codes, barcodes, tables, formulas, code blocks, watermarks, page headers, page footers, page numbers, and graphics. `table=2` requests wired-table handling. The default suppresses `watermark`, `page_header`, `page_footer`, `page_number`, and `graph`.

Valid `result_format` values are `json`, `json,markdown`, `json,sed`, and `json,markdown,sed`. `json_element_option` is reserved by the service and is intentionally not exposed. The API documents `streaming_layout`, but this client currently supports only one-shot output; do not claim streaming-layout support.

Supported annotation types are `page`, `layout`, `region`, `page_header`, `title`, `paragraph`, `textline`, `table`, `cell`, `graph`, `list`, `item`, `formula`, `code`, `pseudocode`, `information_bar`, `seal`, `fingerprint`, `barcode`, `qrcode`, `watermark`, `page_footer`, `page_number`, `annotation`, `footnote`, `key`, `value`, and `contents`. The default selects common semantic content types and avoids structural/helper types; `all` selects the complete list.

Return extracted Markdown directly when requested. SED is a structured array of typed elements with coordinates and text; preserve that array instead of converting or summarizing it. Use `raw` only when the user needs the complete coordinate/layout tree.
