# media-ingest (mingest)

![og-image](og-image.png)

一个 Media Ingestion 命令行工具：输入 URL，自动调用 `yt-dlp` 下载，并默认合并为 `mp4`（嵌入封面与元数据）。对需要登录/会员/验证的内容，提供一键 `auth` 交互登录 + cookies 缓存能力，降低使用门槛。

```bash
mingest get "https://www.youtube.com/watch?v=******"
mingest get "https://www.bilibili.com/bangumi/play/ss******"
mingest get "https://www.bilibili.com/bangumi/play/ep******"
```

说明：

- 站点/格式支持最终取决于 `yt-dlp` 本身
- 当前内置平台：`youtube`、`bilibili`

## 能做什么

- 自动检测并调用：`yt-dlp`、`ffmpeg`/`ffprobe`、`deno|node`
- 默认下载并合并为 `mp4`，附带元数据并嵌入封面
- 自动维护 **cookies 缓存**（优先使用；必要时从浏览器读取 cookies 刷新登录状态）
- Windows 下 Chrome cookies 读取失败时：自动尝试 **CDP**（让 Chrome 在进程内导出明文 cookies，避免读取/解密数据库）

## 安装

推荐直接下载 GitHub Release 的产物。

- `*_slim`：不内置工具，需要你自己装 `yt-dlp`、`ffmpeg`/`ffprobe`、`deno|node`
- `*_bundled`：内置 `yt-dlp`、`ffmpeg`/`ffprobe`、`deno`（开箱即用，体积更大；含 `THIRD_PARTY_LICENSES` 满足各组件许可归属）

## 用法

下载：

```bash
mingest get "<url>"
```

交互登录（一次性准备登录信息，写入 cookies 缓存）：

```bash
mingest auth <platform>
```

支持的平台：

- `youtube`
- `bilibili`

## 登录信息与 cookies（自动模式）

cookies 缓存文件（按平台分别保存）：

- Windows：`%LOCALAPPDATA%\\mingest\\youtube-cookies.txt` / `%LOCALAPPDATA%\\mingest\\bilibili-cookies.txt`
- macOS / Linux：位于 `os.UserConfigDir()/mingest/` 下的同名文件

默认行为（每次 `mingest get`）：

- 优先使用 cookies 缓存（避免频繁读取浏览器数据）
- 若缓存失效/缺失：按顺序从浏览器读取并刷新（默认顺序 `chrome -> firefox -> chromium -> edge`，失败会自动切换）
- 为避免“未登录的浏览器覆盖掉已登录缓存”，浏览器导出的 cookies 会先写入临时文件；检测到有效登录信号后才会更新缓存

`mingest auth <platform>` 行为：

- 启动一个工具专用的 Chrome profile（位于状态目录下的 `mingest/chrome-profile`）
- 你在弹出的 Chrome 窗口完成登录后回到终端按回车
- 工具从 Chrome 进程内导出 cookies，写入该平台的 cookies 缓存文件

Windows 常见情况：

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

## 退出码

- `20` `AUTH_REQUIRED`：需要登录
- `21` `COOKIE_PROBLEM`：cookies 读取/解密/数据库占用问题
- `30` `RUNTIME_MISSING`：`deno|node` 不可用
- `31` `FFMPEG_MISSING`：`ffmpeg`/`ffprobe` 不可用
- `32` `YTDLP_MISSING`：`yt-dlp` 不可用
- `40` `DOWNLOAD_FAILED`：下载失败（其它原因）

## 常见问题

1. 提示需要登录/会员/验证

- 先在浏览器确认账号可正常观看
- 再执行：`mingest auth youtube` / `mingest auth bilibili`
- 若是“额外确认”（例如年龄确认/风险提示/会员提示），建议在 `auth` 打开的窗口中打开目标视频并完成确认后再回车

2. Windows：`Could not copy Chrome cookie database` / `Failed to decrypt with DPAPI`

- 这是 Chrome 数据库锁定或加密策略导致，工具会自动尝试 CDP
- 若仍失败：请先彻底退出浏览器后重试，或切换到 Firefox（见 `MINGEST_BROWSER=firefox`）

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

## 许可证

本项目采用 [GNU Affero General Public License v3.0 (AGPL-3.0)](LICENSE) 开源协议。  
Copyright (C) 2026 Harrison Wang <https://mingest.com>

`*_bundled` 版本内置的 yt-dlp、ffmpeg/ffprobe、deno 为独立第三方组件，其版权与许可见 [THIRD_PARTY_LICENSES](THIRD_PARTY_LICENSES)。

