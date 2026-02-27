# Semantic 候选流水线方案（A-E）

## 1. 目标
在 `mingest` 现有 `prep-plan` 基础上，实现可落地的 “AI 候选 + 人工决策 + 质量闸门”：

1. 不承诺“自动爆款”。
2. 承诺“更快给出可用候选 + 可回滚 + 可验证”。

## 2. 命令入口

```bash
mingest semantic <asset_ref> [options]
```

关键参数：
- `--target shorts|youtube|bilibili`（默认 `shorts`）
- `--provider auto|openai|openrouter`
- `--model <name>`
- `--no-llm`（跳过 Stage B）
- `--apply`（执行 Stage E：写回 `prep-plan`）
- `--decisions <path>`（人工评审决策文件）
- `--json`

## 3. A-E 设计

### Stage A：候选窗口生成（规则）
输入：`subtitle.srt`（或模板字幕兜底）  
输出：`stage-a-candidates.json`

规则要点：
1. 按字幕 cue 组合成时长窗口（`shorts` 默认 15-45s）。
2. 计算基础信号：`hook/insight/controversy/density/question`。
3. 计算 `base_score`，去重后保留 `candidate_limit`。

### Stage B：GPT 语义重排（OpenAI SDK）
输入：Stage A 候选 JSON  
输出：`stage-b-llm.json`

规则要点：
1. 模型仅返回：`id/semantic_score/type/reason`。
2. 不允许新增/删除候选 id。
3. `final_score = 0.55*base + 0.45*semantic`。
4. 失败自动回退规则分（不中断全流程）。

### Stage C：约束选段（自动补位）
输入：Stage B（或回退分数）  
输出：`stage-c-selected.json`

约束：
1. 时长范围检查。
2. 重叠阈值检查。
3. 语义重复度检查（Jaccard）。
4. 不足 `top_k` 时自动补位。

### Stage D：评审包（人工决策）
输入：Stage C 结果  
输出：
1. `review.html`
2. `review-decisions.template.json`
3. `previews/*.mp4`（若 ffmpeg 可用）

操作方式：
1. 在本地打开 `review.html` 预览候选片段。
2. 按需编辑决策文件（`keep/rank/note`）。

### Stage E：写回 + doctor 闸门
触发条件：`--apply`  
输入：决策文件（默认模板）  
输出：
1. 写回 `prep-plan.json`
2. 备份 `prep-plan.json.backup-<timestamp>`

闸门：
1. 执行 `doctor` 规则集（内存检查后写回）。
2. 若 `FAIL > 0`，拒绝写回并返回失败码。

## 4. OpenAI / OpenRouter 兼容策略

### OpenAI
- Key：`MINGEST_OPENAI_API_KEY` 或 `OPENAI_API_KEY`
- 默认模型：`gpt-4.1-mini`

### OpenRouter
- Key：`MINGEST_OPENROUTER_API_KEY` 或 `OPENROUTER_API_KEY`
- Base URL：`https://openrouter.ai/api/v1`（可覆盖）
- 默认模型：`openai/gpt-4.1-mini`
- 自动追加 `HTTP-Referer` 与 `X-Title` 请求头

## 5. 产物目录

`<asset_dir>/.mingest/semantic/<asset_id>/<timestamp>/`

- `stage-a-candidates.json`
- `stage-b-llm.json`（若使用 LLM）
- `stage-c-selected.json`
- `review.html`
- `review-decisions.template.json`
- `previews/`

## 6. 用户故事（最小闭环）

1. 用户运行：
```bash
mingest semantic ast_xxx --target shorts
```
2. 打开 `review.html`，确认候选质量，编辑决策文件。
3. 用户应用：
```bash
mingest semantic ast_xxx --target shorts --apply --decisions /path/to/review-decisions.json
```
4. 通过后继续：
```bash
mingest export ast_xxx --to capcut --zip
```

## 7. 验收指标（建议）

1. `first-pass accept rate`：候选一次通过率
2. `edit time saved`：找片段时间下降比例
3. `rework count`：返工次数下降比例
4. `doctor fail rate`：写回失败率（期望持续下降）

