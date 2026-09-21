# OCR 参数

`xfyun_ocr` 可处理图片或 PDF。接口原生支持 `jpg/jpeg/png/bmp`；GIF、WebP、TIFF 等可解码格式会在纯 Go 内自动转成 JPEG/PNG。源图片最大 32 MiB，并会自动压缩/缩放，使上传图像不超过 4 MiB、base64 不超过 10 MiB。`alpha_option="1"` 时保留 PNG；否则超限图片可能转为高质量 JPEG。

PDF 由内嵌 PDFium WebAssembly 渲染，无 CGO、无需系统安装渲染器，支持文本、矢量和扫描页。完整链路严格逐页流式执行：渲染一页、压缩、OCR、写出该页 NDJSON、释放页面后再继续。`pages="1-3,5"` 选择页码，`pdf_dpi` 为 72–300，默认 150；临时位图最多 4000 万像素，超大页面自动降 DPI（最低 72）。PDFium WASM 内存硬上限为 512 MiB，同一服务进程内的并发 PDF 会串行排队。PDF 总页数可以超过 200 页，但每次调用最多选择 200 页；更大的文档可用 `1-200`、`201-400` 等范围分批处理。单个 PDF 文件最大 500 MiB。

单图/单页直接返回 `text`。多页调用不会把所有文字装进 MCP 响应，而是每页写一条 NDJSON，并返回 `output_path`、`output_format="ndjson"` 和 `page_count`。用户要求固定位置时传 `output_path`；省略时工具创建临时文件并标记 `auto_generated=true`。只有用户允许覆盖时才传 `force=true`。WorkBuddy 若提供 progress token，还会逐页显示处理进度。

- 保留文档结构/返回 Markdown：`result_format="json,markdown"`（默认）。需要 SED 时使用 `json,sed` 或 `json,markdown,sed`；仅 JSON 使用 `json`。
- 工具响应会提取 `markdown`/`sed` 字段，并将可读结果放入 `text`；需要完整坐标和版面 JSON 时传 `include_raw=true`，再读取 `raw`。
- 在原图标注版面类型：`annotate=true`，并用 `annotation_types="paragraph,title,table"` 或 `"all"` 选择类型。`annotation_output_path` 对单页是 PNG 路径，对多页是目录；不指定则自动生成临时路径。单页 PNG 不超过 8 MiB 时也会作为 MCP 图片直接返回。
- 字符级坐标：`result_option="normal,char"`；不返回行坐标：`normal,no_line_position`；两者兼用：`normal,char,no_line_position`。
- 自动旋转阈值：`rotation_min_angle` 为 0–180，默认 5。
- 使用拍摄方向信息：`exif_option="1"`；透明通道影响内容时：`alpha_option="1"`。
- `markdown_options`、`sed_options` 使用逗号分隔的 `name=value`，按需求控制印章、二维码、条码、表格、公式、代码、水印、页眉、页脚、页码、图片等元素；`table=2` 表示有线表格处理。Markdown 默认使用 `table_format=0,formula_format=0`，输出 HTML 表格和 MathML 公式，以完整保留合并单元格及复杂结构；`table_format=1` 才输出 Markdown 表格，此时 `formula_format` 不生效。

标注支持完整的版面元素清单：`page`、`layout`、`region`、`page_header`、`title`、`paragraph`、`textline`、`table`、`cell`、`graph`、`list`、`item`、`formula`、`code`、`pseudocode`、`information_bar`、`seal`、`fingerprint`、`barcode`、`qrcode`、`watermark`、`page_footer`、`page_number`、`annotation`、`footnote`、`key`、`value`、`contents`。默认只选择常见语义类型，`all` 选择全部。

服务文档中的 `json_element_option` 暂不支持；本客户端也暂不支持 `streaming_layout`，不要向用户承诺流式版面输出。
