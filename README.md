# youtube-cli (MVP)

最小可用命令行：

```bash
youtube <youtube_url>
youtube auth
```

当前版本能力：

- 自动检测 `yt-dlp`、`ffmpeg`、`deno|node`
- 自动调用 `yt-dlp` 下载并合并为 `mp4`
- 自动从浏览器读取 cookies（要求你已在浏览器登录 YouTube）
- 当浏览器 cookies 读取失败时，可用 `youtube auth` 生成一个工具专用的 Chrome 登录态（不需要导出 cookies.txt）

## 依赖查找顺序

1. 程序同目录（优先）
2. 系统 `PATH`

## 浏览器 cookies（自动模式）

默认浏览器选择：

- 若仅检测到 1 个浏览器数据目录，直接使用它
- 若检测到多个，默认优先 `chrome`，失败后自动尝试 `firefox/chromium/edge`

可用环境变量覆盖：

- `YOUTUBE_BROWSER=chrome|firefox|chromium|edge`
- `YOUTUBE_BROWSER_PROFILE=Default|Profile 1|...`
- `YOUTUBE_JS_RUNTIME=node|deno`
- `YOUTUBE_CHROME_PATH=C:\Path\To\chrome.exe`

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
