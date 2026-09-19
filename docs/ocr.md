# OCR 使用指南

`xfyun_ocr` 是面向 Agent 的文档 OCR 工具，负责图片预处理、PDF 逐页渲染、讯飞版面识别、可读结果整理以及版面框可视化。它既可以直接识别一张图片，也可以流式处理多页 PDF。

## 开始使用

先按 [Agent 产品接入指南](agent-integration.md) 安装 MCP，并配置：

```text
XFYUN_APP_ID
XFYUN_API_KEY
XFYUN_API_SECRET
```

之后可以直接向 Agent 提出自然语言请求：

```text
识别 /absolute/path/page.png，整理成 Markdown。
识别 /absolute/path/report.pdf 的第 1-5、8 页，并保留表格。
识别这张扫描件，同时返回字符坐标和完整布局 JSON。
在原图上标出标题、段落、表格和单元格，并返回标注图片。
```

本地文件路径必须对运行 MCP Server 的机器可见。

## 输入文件

- 讯飞接口原生格式：JPG、JPEG、PNG、BMP。
- GIF、WebP、TIFF 等可解码格式会自动转换为 JPEG 或 PNG。
- 支持扫描 PDF、文字 PDF 和矢量 PDF，不依赖系统安装 Poppler。
- 单张源图片最大 32 MiB；上传前会自动压缩到接口允许的大小。
- 单个 PDF 最大 500 MiB；一次最多选择 200 页。

PDF 默认按 150 DPI 渲染，可以用 `pdf_dpi` 设置 72–300 DPI。超大页面会自动降低 DPI，临时位图最多 4000 万像素。

## 常用调用

默认识别并返回 Markdown：

```json
{
  "input_path": "/absolute/path/page.png"
}
```

选择 PDF 页码：

```json
{
  "input_path": "/absolute/path/report.pdf",
  "pages": "1-5,8",
  "pdf_dpi": 150
}
```

同时请求 Markdown、SED 和完整布局 JSON：

```json
{
  "input_path": "/absolute/path/page.png",
  "result_format": "json,markdown,sed",
  "include_raw": true
}
```

请求字符级坐标：

```json
{
  "input_path": "/absolute/path/page.png",
  "result_option": "normal,char",
  "include_raw": true
}
```

## 结果格式

`result_format` 支持：

- `json`
- `json,markdown`，默认值
- `json,sed`
- `json,markdown,sed`

默认 `text` 是从讯飞 `document` 节点整理出的可读 Markdown；没有 Markdown 时会使用 SED。只有在需要坐标、布局属性或诊断原始响应时才设置 `include_raw=true`。

单张图片或只选择一页且未指定 `output_path` 时，结果直接返回。多页结果逐页写入 NDJSON，并返回：

```json
{
  "output_path": "/tmp/xfyun-ocr-....ndjson",
  "output_format": "ndjson",
  "page_count": 8,
  "auto_generated": true
}
```

NDJSON 每行对应一页，包含页码、DPI、文本、Markdown、SED、SID，以及可选的 raw 和标注路径。需要稳定文件位置时传入 `output_path`；已有文件默认不会覆盖，只有用户明确授权后才能设置 `force=true`。

## 版面标注图片

设置 `annotate=true` 可以在原图上绘制版面框和英文类型标签：

```json
{
  "input_path": "/absolute/path/page.png",
  "annotate": true,
  "annotation_types": "paragraph,title,table,cell",
  "annotation_output_path": "/absolute/path/page.annotated.png"
}
```

`annotation_types="all"` 选择全部类型。不传时默认选择常见语义类型，避免 `page`、`layout`、`region`、`textline` 等嵌套框大量重叠。

支持的 28 种类型为：

```text
page, layout, region, page_header, title, paragraph, textline,
table, cell, graph, list, item, formula, code, pseudocode,
information_bar, seal, fingerprint, barcode, qrcode, watermark,
page_footer, page_number, annotation, footnote, key, value, contents
```

单页结果返回 `annotation_path`；PNG 不超过 8 MiB 时还会作为 MCP 图片直接返回。多页结果返回 `annotation_paths`，此时 `annotation_output_path` 应当是目录。未指定路径时自动创建临时 PNG。

标注器会处理 OCR 尺寸与原图尺寸的缩放差异，也会把讯飞自动纠正后的 90°、180°、270° 坐标还原到原始图片方向。

## 版面元素控制

`markdown_options` 和 `sed_options` 使用逗号分隔的 `name=value`：

```json
{
  "markdown_options": "seal=1,qrcode=1,table=2,watermark=0",
  "sed_options": "seal=1,qrcode=1,table=2,watermark=0"
}
```

MCP 的 `markdown_options` 和 CLI 的 `--markdown-elements` 都会传给讯飞的 `markdown_element_option`。工具默认使用：

```text
table_format=0,formula_format=0,watermark=0,page_header=0,page_footer=0,page_number=0,graph=0
```

`table_format=0` 输出 HTML 表格，比 Markdown 管道表格更完整地保留合并单元格和复杂行列结构；`formula_format=0` 让 HTML 表格里的公式使用 MathML。仅当显式设置 `table_format=1` 时才输出 Markdown 格式表格，此时 `formula_format` 不生效。

`table=2` 请求有线表格处理。默认还会隐藏 `watermark`、`page_header`、`page_footer`、`page_number` 和 `graph`，只有用户明确要求时才建议改变这些选项。

## 方向、透明度和坐标

- `rotation_min_angle`：自动旋转的最小角度，范围 0–180，默认 5。
- `exif_option="1"`：遵循相机 EXIF 方向。
- `alpha_option="1"`：透明像素影响可见内容时保留 PNG 透明度。
- `result_option="normal,char"`：返回字符级信息。
- `result_option="normal,no_line_position"`：不返回文本行坐标。
- `result_option="normal,char,no_line_position"`：组合上述两个选项。

## 多页处理和资源限制

PDF 按以下顺序逐页执行：

```text
渲染一页 → 压缩 → OCR → 写入 NDJSON → 释放页面 → 下一页
```

因此内存不会随着整份文档页数持续增长。PDFium WebAssembly 运行时具有 512 MiB 硬限制；同一 MCP Server 进程中的多个 PDF 会串行处理。支持 progress token 的宿主会收到逐页进度通知。

当前调用讯飞 `one_shot` 输出；没有实现接口文档中语义不够明确的 `streaming_layout` 模式。这里的“流式处理”指客户端逐页渲染、识别和落盘，而不是讯飞的 `streaming_layout` 参数。

## CLI 保底入口

宿主不支持 MCP 时可以使用：

```bash
xfyun ocr --input page.png
xfyun ocr --input report.pdf --pages 1-5,8 > report.ocr.ndjson
xfyun ocr --input page.png --result-format json,markdown,sed --raw
```

运行 `xfyun ocr --help` 查看完整 CLI 参数。

## 常见问题

### 提示缺少凭证

MCP 进程没有继承 `XFYUN_APP_ID`、`XFYUN_API_KEY`、`XFYUN_API_SECRET`。在宿主启动环境中配置变量后，完全退出并重新启动 Codex、Claude Code 或其他宿主。

### PDF 无法打开

先用 `pdfinfo` 或 `qpdf --check` 检查文件。缺少 `startxref`、`trailer`、`endstream` 或文件被截断属于 PDF 损坏，并非 OCR 接口错误。

### 标注图没有框

检查 raw 布局中是否存在带 `coord` 或 `contour` 的节点。纯效果图或没有文字的页面可能正常返回零个版面节点。

### Markdown 很长

复杂工程图可能被识别成大量表格、单元格和公式，讯飞生成的 Markdown 会明显长于可见正文。需要精简结果时优先使用 SED，或只读取需要的布局类型。
