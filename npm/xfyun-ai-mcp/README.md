# @fruitbars/xfyun-ai-mcp

Cross-platform `npx` launcher for the `xfyun-ai-mcp` stdio MCP server.

```bash
npx -y @fruitbars/xfyun-ai-mcp@0.3.0
```

The launcher selects an npm optional dependency for Windows, macOS, or Linux on x64/arm64 and starts the native Go server with inherited stdio and environment variables. Configure `XFYUN_APP_ID`, `XFYUN_API_KEY`, and `XFYUN_API_SECRET` in the MCP host.

PDF OCR uses embedded PDFium WebAssembly: no CGO, Poppler, or other system renderer is required. Text, vector, and scanned PDFs render at 150 DPI by default. Rendering, compression, OCR, NDJSON result writing, and cleanup happen one page at a time, so memory use does not grow with the full document. The PDFium runtime has a 512 MiB hard limit and concurrent PDFs are serialized within one server process. Multi-page MCP calls return an `output_path` instead of accumulating every page in one response.

For local launcher development, `XFYUN_AI_MCP_BINARY` can point to a locally built `xfyun-ai-mcp` executable.
