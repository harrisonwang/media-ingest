# 面向 get 用户的命令裁剪方案（v1）

## 目标边界
- 目标用户：先有 URL 再做素材处理的创作者/代剪。
- 产品边界：先做 `URL -> 下载 -> 处理 -> 导出`，不做全盘 DAM/MAM。
- `index`：仅作为 `get` 成功后的内部增量步骤，不对外暴露。
- `doctor`：作为导出前质量闸门，不负责“自动选爆款”，负责拦截明显低质量切片。

## 命令清单

### 1) `mingest auth <platform> [--browser <name>] [--browser-profile <name>] [--json]`
- `<platform>`：目标平台。当前支持 `youtube`、`bilibili`。
- `--browser <name>`：指定浏览器，`chrome|firefox|chromium|edge`。
- `--browser-profile <name>`：指定浏览器 profile，如 `Default`、`Profile 1`。
- `--json`：JSON 输出，便于脚本处理。

### 2) `mingest get <url> [--out-dir <dir>] [--name-template <tpl>] [--asset-id-only] [--json]`
- `<url>`：单条视频 URL。
- `--out-dir <dir>`：下载目录；未指定时使用当前工作目录。
- `--name-template <tpl>`：yt-dlp 输出模板；未指定时默认 `%(title)s.%(ext)s`。
- `--asset-id-only`：仅输出下载结果的 `asset_id`（stdout 仅一行，便于下游命令串联）。
- `--json`：输出结构化结果（包含 `asset_id`、`output_path`、`platform` 等）。

### 3) `mingest ls [--limit <n>] [--query <text>] [--format <table|json>]`
- `--limit <n>`：最多返回条目数，默认 `20`。
- `--query <text>`：按标题/来源关键字过滤。
- `--format <table|json>`：输出格式，默认 `table`。

### 4) `mingest prep <asset_ref> --goal <subtitle|highlights|shorts> [--lang <auto|zh|en>] [--max-clips <n>] [--clip-seconds <sec>] [--subtitle-style <clean|shorts>] [--json]`
- `<asset_ref>`：`asset_id` 或本地文件路径。
- `--goal`：处理目标。
  - `subtitle`：字幕流程优先。
  - `highlights`：片段提取优先。
  - `shorts`：竖版切条优先。
- `--lang`：转写语言，默认 `auto`。
- `--max-clips <n>`：建议片段数量上限。
- `--clip-seconds <sec>`：建议片段时长上限。
- `--subtitle-style <clean|shorts>`：字幕样式模板。
- `--json`：输出结构化处理结果。

### 5) `mingest export <asset_ref> --to <premiere|resolve|capcut> [--with <srt,edl,csv,fcpxml>] [--out-dir <dir>] [--zip] [--json]`
- `<asset_ref>`：`asset_id` 或本地文件路径。
- `--to <premiere|resolve|capcut>`：目标剪辑软件（`jianying` 也可作为 `capcut` 别名）。
- `--with <srt,edl,csv,fcpxml>`：导出内容类型（逗号分隔）。
- `--out-dir <dir>`：导出目录。
- `--zip`：导出结果压缩为 zip。
- `--json`：输出结构化导出清单。

### 6) `mingest doctor <asset_ref> [--target <youtube|bilibili|shorts>] [--strict] [--json]`
- `<asset_ref>`：`asset_id` 或本地文件路径。
- `--target`：按发布目标选择检查规则（`shorts` 会更关注短时长与重复度）。
- `--strict`：启用更严格阈值（更容易 fail）。
- `--json`：输出结构化诊断结果。

检查范围（当前实现）：
- clips 数量/时间戳合法性/越界
- 片段时长范围与片段重叠度
- 字幕来源（真实字幕 or 模板字幕）
- 字幕覆盖率、边界切断率
- 片段文本近重复度
- 是否疑似“均匀采样”模式（提醒切换为“AI 候选 + 人工决策”）

### 7) `mingest semantic <asset_ref> [--target <youtube|bilibili|shorts>] [--provider <auto|openai|openrouter>] [--model <name>] [--apply] [--json]`
- `<asset_ref>`：`asset_id` 或本地文件路径。
- 流程：Stage A-E（候选生成 -> GPT 重排 -> 约束选段 -> 评审包 -> 写回 + doctor）。
- `--apply`：把最终片段写回 `prep-plan.json`（默认仅生成评审包，不改原 plan）。
- `--decisions <path>`：指定人工评审后的决策文件。
- `--provider` / `--model`：支持 OpenAI 与 OpenRouter（OpenAI 兼容接口）。

## P0 / P2

### P0（当前阶段）
- `auth`
- `get`（含 `--out-dir`、`--name-template`、`--asset-id-only`、`--json`）
- `prep`
- `export`
- `doctor`（质量闸门）
- `semantic`（语义候选 + 人工决策）

### P2（延后）
- `mingest get --batch <urls.txt> [--concurrency <n>] [--retry <n>] [--continue-on-error] [--json]`
  - 说明：当前样本未显示“批量下载”是高频强痛点，因此延后。
