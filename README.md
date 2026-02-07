# media-ingest (MVP)

最小可用命令行：

```bash
mingest get <url>
mingest auth
```

当前版本能力：

- 自动检测 `yt-dlp`、`ffmpeg`、`deno|node`
- 自动调用 `yt-dlp` 下载并合并为 `mp4`
- 自动从浏览器读取 cookies（要求你已在浏览器登录 YouTube）
- 当浏览器 cookies 读取失败时，可用 `mingest auth` 生成一个工具专用的 Chrome 登录态（不需要导出 cookies.txt）

## 依赖查找顺序

1. 程序同目录（优先）
2. 系统 `PATH`

## 浏览器 cookies（自动模式）

默认浏览器选择：

- 若仅检测到 1 个浏览器数据目录，直接使用它
- 若检测到多个，默认优先 `chrome`，失败后自动尝试 `firefox/chromium/edge`

可用环境变量覆盖：

- `MINGEST_BROWSER=chrome|firefox|chromium|edge`
- `MINGEST_BROWSER_PROFILE=Default|Profile 1|...`
- `MINGEST_JS_RUNTIME=node|deno`
- `MINGEST_CHROME_PATH=C:\Path\To\chrome.exe`

## 默认下载参数

- `--output "%(title)s.%(ext)s"`
- `--embed-thumbnail`
- `--add-metadata`
- `-f "bestvideo[vcodec^=avc1]+bestaudio[ext=m4a]/best[ext=mp4]/best"`
- `--merge-output-format mp4`

## 退出码（给 Agent 用）

- `20` `AUTH_REQUIRED`
- `21` `COOKIE_PROBLEM`
- `30` `RUNTIME_MISSING`
- `31` `FFMPEG_MISSING`
- `32` `YTDLP_MISSING`
- `40` `DOWNLOAD_FAILED`

## 构建与打包

- 默认构建（不嵌入任何工具二进制）：`go build -o dist/mingest ./cmd/mingest`
- 可选嵌入（构建前下载工具到 `ingest/embedtools/assets/<goos>/`）：`scripts/fetch-embed-tools.sh --os <goos> --arch <goarch>`，然后 `go build -tags embedtools -o dist/mingest ./cmd/mingest`

GitHub Actions: `.github/workflows/build-and-release.yml` 会在推送 tag（如 `v0.1.0`）时，为 macOS / Windows / Linux 构建并打包产物，同时生成 `SHA256SUMS.txt` 并发布到 GitHub Release。
