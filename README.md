# media-ingest (mingest)

一个 Media Ingestion 命令行工具。当前版本主要聚焦 **YouTube**：输入 URL，自动调用 `yt-dlp` 下载并合并为 `mp4`，并复用你浏览器里的登录状态，以降低用户的使用成本。

```bash
mingest get "https://www.youtube.com/watch?v=duZDsG3tvoA"
```

说明：

- 站点支持最终取决于 `yt-dlp` 本身；本项目会逐步补齐多站点的“鉴权/可观测/沙箱化”等能力。
- 目前 cookie/登录态相关逻辑主要针对 YouTube 进行了适配，后续会扩展到 B 站等站点。

## 能做什么

- 自动检测并调用：`yt-dlp`、`ffmpeg`/`ffprobe`、`deno|node`
- 默认下载并合并为 `mp4`，附带元数据并嵌入封面
- 自动从浏览器读取 cookies（默认优先 `chrome`）
- Windows 11 下 Chrome cookies 读取/解密失败时：自动尝试 **CDP**（让 Chrome 自己把已解密 cookies 交给工具）

## 安装

推荐直接下载 GitHub Release 的产物。

- `*_slim`：不内置工具，需要你自己装 `yt-dlp`、`ffmpeg`/`ffprobe`、`deno|node`
- `*_bundled`：内置 `yt-dlp`、`ffmpeg`/`ffprobe`、`deno`（开箱即用，体积更大）

## 快速开始

1. 下载：

```bash
mingest get "<url>"
```

## 登录态与 cookies（自动模式）

默认行为：

- 若只检测到 1 个浏览器数据目录：直接使用它
- 若检测到多个：默认顺序 `chrome -> firefox/chromium/edge`（失败会自动切换）

Windows 11 常见情况：

- `--cookies-from-browser chrome` 可能因为数据库锁定 / DPAPI / App-Bound Encryption 失败
- 工具会在 Chrome 失败后自动走 CDP（无需 DPAPI 解密）

可用环境变量覆盖：

- `MINGEST_BROWSER=chrome|firefox|chromium|edge`
- `MINGEST_BROWSER_PROFILE=Default|Profile 1|...`
- `MINGEST_JS_RUNTIME=node|deno`
- `MINGEST_CHROME_PATH=C:\\Path\\To\\chrome.exe`

## 依赖查找顺序

每个依赖（`yt-dlp`、`ffmpeg`、`ffprobe`、`deno`、`node`）按以下顺序查找：

1. 内置（`-tags embedtools` 构建时嵌入的工具）
2. 当前工作目录（你运行 `mingest` 的目录）
3. 程序所在目录
4. 系统 `PATH`

## 默认下载参数

当前固定为：

- `--output "%(title)s.%(ext)s"`
- `--embed-thumbnail`
- `--add-metadata`
- `-f "bestvideo[vcodec^=avc1]+bestaudio[ext=m4a]/best[ext=mp4]/best"`
- `--merge-output-format mp4`

## 退出码（给 Agent 用）

- `20` `AUTH_REQUIRED`：需要登录
- `21` `COOKIE_PROBLEM`：cookies 读取/解密/数据库占用问题
- `30` `RUNTIME_MISSING`：`deno|node` 不可用
- `31` `FFMPEG_MISSING`：`ffmpeg`/`ffprobe` 不可用
- `32` `YTDLP_MISSING`：`yt-dlp` 不可用
- `40` `DOWNLOAD_FAILED`：下载失败（其它原因）

## 常见问题

1. `Sign in to confirm you’re not a bot`

- 这通常与 IP/网络风控有关，和“浏览器能播放视频”不等价
- 请先在浏览器中完成登录/验证后重试；必要时更换网络环境或切换浏览器（见 `MINGEST_BROWSER`）

2. Windows：`Could not copy Chrome cookie database` / `Failed to decrypt with DPAPI`

- 这是 Chrome 数据库锁定或加密策略导致，工具会自动尝试 CDP
- 若仍失败：请先关闭浏览器后重试，或切换到 Firefox（见 `MINGEST_BROWSER=firefox`）

3. Linux：提示 `ffprobe not found`

- 说明你用的是 `*_slim` 且系统/目录里只有 `ffmpeg` 没有 `ffprobe`
- 换用 `*_bundled`，或确保 `ffprobe` 与 `ffmpeg` 同目录可用

## 从源码构建

默认（不嵌入工具）：

```bash
go build -o dist/mingest ./cmd/mingest
```

打包内置工具（推荐用于分发）：

```bash
scripts/fetch-embed-tools.sh --os <goos> --arch <goarch>
go build -tags embedtools -o dist/mingest ./cmd/mingest
```

说明：

- 工具下载目录：`ingest/embedtools/assets/<goos>/`
- 获取 `ffmpeg` 时会一并获取 `ffprobe`
- GitHub Actions 工作流见 `.github/workflows/build-and-release.yml`
