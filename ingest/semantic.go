// media-ingest (mingest) - Media Ingestion CLI tool
// Copyright (C) 2026  Harrison Wang <https://mingest.com>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

const (
	defaultSemanticModelOpenAI     = "gpt-4.1-mini"
	defaultSemanticModelOpenRouter = "openai/gpt-4.1-mini"
	defaultOpenRouterBaseURL       = "https://openrouter.ai/api/v1"
	maxSemanticCandidateWindows    = 900
)

type semanticOptions struct {
	AssetRef       string
	Target         string
	Provider       string
	Model          string
	BaseURL        string
	APIKey         string
	CandidateLimit int
	TopK           int
	PreviewLimit   int
	DecisionsPath  string
	NoLLM          bool
	Apply          bool
	Strict         bool
	JSON           bool
}

type semanticSignals struct {
	Hook        float64 `json:"hook"`
	Insight     float64 `json:"insight"`
	Controversy float64 `json:"controversy"`
	Density     float64 `json:"density"`
	Question    float64 `json:"question"`
}

type semanticCandidate struct {
	ID            string          `json:"id"`
	StartSec      float64         `json:"start_sec"`
	EndSec        float64         `json:"end_sec"`
	DurationSec   float64         `json:"duration_sec"`
	CueStartIndex int             `json:"cue_start_index"`
	CueEndIndex   int             `json:"cue_end_index"`
	Text          string          `json:"text"`
	BaseScore     float64         `json:"base_score"`
	SemanticScore float64         `json:"semantic_score,omitempty"`
	FinalScore    float64         `json:"final_score"`
	Type          string          `json:"type,omitempty"`
	Reason        string          `json:"reason,omitempty"`
	Signals       semanticSignals `json:"signals"`
	PreviewPath   string          `json:"preview_path,omitempty"`
}

type semanticLLMItem struct {
	ID            string  `json:"id"`
	SemanticScore float64 `json:"semantic_score"`
	Type          string  `json:"type"`
	Reason        string  `json:"reason"`
}

type semanticLLMResponse struct {
	Items []semanticLLMItem `json:"items"`
}

type semanticDecisionItem struct {
	ID   string `json:"id"`
	Keep bool   `json:"keep"`
	Rank int    `json:"rank,omitempty"`
	Note string `json:"note,omitempty"`
}

type semanticDecisionFile struct {
	Version   string                 `json:"version"`
	Target    string                 `json:"target"`
	AssetID   string                 `json:"asset_id"`
	CreatedAt string                 `json:"created_at"`
	Items     []semanticDecisionItem `json:"items"`
}

type semanticArtifacts struct {
	BundleDir       string `json:"bundle_dir"`
	StageAPath      string `json:"stage_a_path"`
	StageBPath      string `json:"stage_b_path,omitempty"`
	StageCPath      string `json:"stage_c_path"`
	ReviewHTMLPath  string `json:"review_html_path"`
	ReviewDecisions string `json:"review_decisions_path"`
	PreviewDir      string `json:"preview_dir"`
	AppliedPlanPath string `json:"applied_plan_path,omitempty"`
	BackupPlanPath  string `json:"backup_plan_path,omitempty"`
}

type semanticJSONResult struct {
	OK             bool              `json:"ok"`
	ExitCode       int               `json:"exit_code"`
	Error          string            `json:"error,omitempty"`
	AssetID        string            `json:"asset_id,omitempty"`
	AssetRef       string            `json:"asset_ref,omitempty"`
	AssetPath      string            `json:"asset_path,omitempty"`
	Target         string            `json:"target,omitempty"`
	Provider       string            `json:"provider,omitempty"`
	Model          string            `json:"model,omitempty"`
	UsedLLM        bool              `json:"used_llm"`
	Applied        bool              `json:"applied"`
	CandidateCount int               `json:"candidate_count,omitempty"`
	SelectedCount  int               `json:"selected_count,omitempty"`
	Artifacts      semanticArtifacts `json:"artifacts,omitempty"`
	Warnings       []string          `json:"warnings,omitempty"`
	DoctorSummary  doctorSummary     `json:"doctor_summary,omitempty"`
}

type semanticRunState struct {
	Asset      prepResolvedAsset
	PlanPath   string
	Plan       prepPlan
	Candidates []semanticCandidate
	Selected   []semanticCandidate
	Artifacts  semanticArtifacts
	Warnings   []string
	Provider   string
	Model      string
	UsedLLM    bool
}

type semanticLLMConfig struct {
	Provider string
	Model    string
	BaseURL  string
	APIKey   string
	Referer  string
	Title    string
}

func parseSemanticOptions(args []string) (semanticOptions, error) {
	opts := semanticOptions{
		Target:         "shorts",
		Provider:       "auto",
		CandidateLimit: 20,
		TopK:           3,
		PreviewLimit:   8,
	}

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "--json":
			opts.JSON = true
		case arg == "--strict":
			opts.Strict = true
		case arg == "--no-llm":
			opts.NoLLM = true
		case arg == "--apply":
			opts.Apply = true
		case arg == "--target":
			if i+1 >= len(args) {
				return semanticOptions{}, fmt.Errorf("`--target` 缺少参数")
			}
			i++
			opts.Target = strings.ToLower(strings.TrimSpace(args[i]))
		case strings.HasPrefix(arg, "--target="):
			opts.Target = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(arg, "--target=")))
		case arg == "--provider":
			if i+1 >= len(args) {
				return semanticOptions{}, fmt.Errorf("`--provider` 缺少参数")
			}
			i++
			opts.Provider = strings.ToLower(strings.TrimSpace(args[i]))
		case strings.HasPrefix(arg, "--provider="):
			opts.Provider = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(arg, "--provider=")))
		case arg == "--model":
			if i+1 >= len(args) {
				return semanticOptions{}, fmt.Errorf("`--model` 缺少参数")
			}
			i++
			opts.Model = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--model="):
			opts.Model = strings.TrimSpace(strings.TrimPrefix(arg, "--model="))
		case arg == "--base-url":
			if i+1 >= len(args) {
				return semanticOptions{}, fmt.Errorf("`--base-url` 缺少参数")
			}
			i++
			opts.BaseURL = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--base-url="):
			opts.BaseURL = strings.TrimSpace(strings.TrimPrefix(arg, "--base-url="))
		case arg == "--api-key":
			if i+1 >= len(args) {
				return semanticOptions{}, fmt.Errorf("`--api-key` 缺少参数")
			}
			i++
			opts.APIKey = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--api-key="):
			opts.APIKey = strings.TrimSpace(strings.TrimPrefix(arg, "--api-key="))
		case arg == "--candidate-limit":
			if i+1 >= len(args) {
				return semanticOptions{}, fmt.Errorf("`--candidate-limit` 缺少参数")
			}
			i++
			n, err := strconv.Atoi(strings.TrimSpace(args[i]))
			if err != nil {
				return semanticOptions{}, fmt.Errorf("`--candidate-limit` 必须是整数")
			}
			opts.CandidateLimit = n
		case strings.HasPrefix(arg, "--candidate-limit="):
			n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(arg, "--candidate-limit=")))
			if err != nil {
				return semanticOptions{}, fmt.Errorf("`--candidate-limit` 必须是整数")
			}
			opts.CandidateLimit = n
		case arg == "--preview-limit":
			if i+1 >= len(args) {
				return semanticOptions{}, fmt.Errorf("`--preview-limit` 缺少参数")
			}
			i++
			n, err := strconv.Atoi(strings.TrimSpace(args[i]))
			if err != nil {
				return semanticOptions{}, fmt.Errorf("`--preview-limit` 必须是整数")
			}
			opts.PreviewLimit = n
		case strings.HasPrefix(arg, "--preview-limit="):
			n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(arg, "--preview-limit=")))
			if err != nil {
				return semanticOptions{}, fmt.Errorf("`--preview-limit` 必须是整数")
			}
			opts.PreviewLimit = n
		case arg == "--top-k":
			if i+1 >= len(args) {
				return semanticOptions{}, fmt.Errorf("`--top-k` 缺少参数")
			}
			i++
			n, err := strconv.Atoi(strings.TrimSpace(args[i]))
			if err != nil {
				return semanticOptions{}, fmt.Errorf("`--top-k` 必须是整数")
			}
			opts.TopK = n
		case strings.HasPrefix(arg, "--top-k="):
			n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(arg, "--top-k=")))
			if err != nil {
				return semanticOptions{}, fmt.Errorf("`--top-k` 必须是整数")
			}
			opts.TopK = n
		case arg == "--decisions":
			if i+1 >= len(args) {
				return semanticOptions{}, fmt.Errorf("`--decisions` 缺少参数")
			}
			i++
			opts.DecisionsPath = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--decisions="):
			opts.DecisionsPath = strings.TrimSpace(strings.TrimPrefix(arg, "--decisions="))
		case strings.HasPrefix(arg, "-"):
			return semanticOptions{}, fmt.Errorf("不支持的参数: %s", arg)
		default:
			if opts.AssetRef != "" {
				return semanticOptions{}, fmt.Errorf("`mingest semantic` 仅支持一个 asset_ref")
			}
			opts.AssetRef = arg
		}
	}

	if strings.TrimSpace(opts.AssetRef) == "" {
		return semanticOptions{}, fmt.Errorf("缺少 asset_ref。用法: mingest semantic <asset_ref> [--target shorts] [--model gpt-4.1-mini] [--apply]")
	}
	switch opts.Target {
	case "youtube", "bilibili", "shorts":
	default:
		return semanticOptions{}, fmt.Errorf("`--target` 仅支持 youtube|bilibili|shorts")
	}
	switch opts.Provider {
	case "auto", "openai", "openrouter":
	default:
		return semanticOptions{}, fmt.Errorf("`--provider` 仅支持 auto|openai|openrouter")
	}
	if opts.CandidateLimit <= 0 || opts.CandidateLimit > 100 {
		return semanticOptions{}, fmt.Errorf("`--candidate-limit` 需在 1-100")
	}
	if opts.PreviewLimit <= 0 || opts.PreviewLimit > 50 {
		return semanticOptions{}, fmt.Errorf("`--preview-limit` 需在 1-50")
	}
	if opts.TopK <= 0 || opts.TopK > 10 {
		return semanticOptions{}, fmt.Errorf("`--top-k` 需在 1-10")
	}
	return opts, nil
}

func runSemantic(opts semanticOptions) int {
	state, exitCode := runSemanticPipeline(opts)
	if opts.JSON {
		printSemanticJSON(buildSemanticJSONResult(state, opts, exitCode))
	} else {
		printSemanticHuman(state, opts, exitCode)
	}
	return exitCode
}

func runSemanticPipeline(opts semanticOptions) (semanticRunState, int) {
	state := semanticRunState{}

	asset, err := resolvePrepAsset(opts.AssetRef)
	if err != nil {
		state.Warnings = append(state.Warnings, err.Error())
		return state, exitSemanticFailed
	}
	if strings.TrimSpace(asset.AssetID) == "" {
		assetID, err := computeAssetID(asset.OutputPath)
		if err != nil {
			state.Warnings = append(state.Warnings, fmt.Sprintf("生成 asset_id 失败: %v", err))
			return state, exitSemanticFailed
		}
		asset.AssetID = assetID
	}
	state.Asset = asset

	_, prepPlanPath, err := latestPrepBundle(asset)
	if err != nil {
		state.Warnings = append(state.Warnings, err.Error())
		return state, exitSemanticFailed
	}
	plan, err := readPrepPlan(prepPlanPath)
	if err != nil {
		state.Warnings = append(state.Warnings, fmt.Sprintf("读取 prep-plan.json 失败: %v", err))
		return state, exitSemanticFailed
	}
	state.PlanPath = prepPlanPath
	state.Plan = plan

	cues, subtitlePath, _ := loadDoctorSubtitle(plan)
	if len(cues) == 0 {
		state.Warnings = append(state.Warnings, "未找到可用字幕条目（subtitle.srt/subtitle-template.srt）")
		return state, exitSemanticFailed
	}

	artifacts, err := createSemanticArtifacts(asset)
	if err != nil {
		state.Warnings = append(state.Warnings, fmt.Sprintf("创建 semantic 输出目录失败: %v", err))
		return state, exitSemanticFailed
	}
	state.Artifacts = artifacts

	// Stage A: 基于字幕生成候选窗口
	minSec, maxSec := semanticTargetDurationRange(opts.Target)
	candidates := buildSemanticCandidates(cues, minSec, maxSec)
	candidates = semanticSelectTopCandidates(candidates, opts.CandidateLimit)
	if len(candidates) == 0 {
		state.Warnings = append(state.Warnings, "无法生成候选片段（字幕内容可能过短或不可解析）")
		return state, exitSemanticFailed
	}
	if err := writeJSONFile(artifacts.StageAPath, map[string]interface{}{
		"version":       "semantic-a-v1",
		"created_at":    time.Now().UTC().Format(time.RFC3339),
		"subtitle_path": subtitlePath,
		"target":        opts.Target,
		"items":         candidates,
	}); err != nil {
		state.Warnings = append(state.Warnings, fmt.Sprintf("写入 Stage A 结果失败: %v", err))
		return state, exitSemanticFailed
	}

	// Stage B: GPT 语义重排
	llmCfg, llmErr := resolveSemanticLLMConfig(opts)
	usedLLM := false
	if !opts.NoLLM {
		if llmErr != nil {
			return semanticExitWithErr(state, opts.JSON, exitSemanticFailed, llmErr.Error())
		}
		state.Provider = llmCfg.Provider
		state.Model = llmCfg.Model
		llmItems, raw, err := semanticRerankWithLLM(candidates, opts.Target, llmCfg)
		if err != nil {
			state.Warnings = append(state.Warnings, fmt.Sprintf("Stage B GPT 重排失败，已回退规则分: %v", err))
		} else {
			usedLLM = true
			candidates = applySemanticLLMScores(candidates, llmItems)
			_ = writeJSONFile(artifacts.StageBPath, map[string]interface{}{
				"version":    "semantic-b-v1",
				"created_at": time.Now().UTC().Format(time.RFC3339),
				"provider":   llmCfg.Provider,
				"model":      llmCfg.Model,
				"raw":        raw,
				"items":      llmItems,
			})
		}
	}
	if !usedLLM {
		candidates = applySemanticFallbackScores(candidates)
	}
	state.UsedLLM = usedLLM
	if state.Model == "" {
		state.Model = defaultSemanticModelOpenAI
	}

	// Stage C: 约束选 3 段
	selected := semanticPickFinalCandidates(candidates, opts.TopK, opts.Target)
	if len(selected) == 0 {
		state.Warnings = append(state.Warnings, "Stage C 未能选出有效片段")
		return state, exitSemanticFailed
	}
	if err := writeJSONFile(artifacts.StageCPath, map[string]interface{}{
		"version":    "semantic-c-v1",
		"created_at": time.Now().UTC().Format(time.RFC3339),
		"target":     opts.Target,
		"top_k":      opts.TopK,
		"items":      selected,
	}); err != nil {
		state.Warnings = append(state.Warnings, fmt.Sprintf("写入 Stage C 结果失败: %v", err))
		return state, exitSemanticFailed
	}

	// Stage D: 预览+评审包
	previewCandidates := semanticTopPreviewCandidates(candidates, selected, opts.PreviewLimit)
	if err := semanticGeneratePreviewFiles(asset.OutputPath, previewCandidates, artifacts.PreviewDir); err != nil {
		state.Warnings = append(state.Warnings, fmt.Sprintf("生成预览视频失败（将继续，使用原始时间戳评审）: %v", err))
	}
	if err := writeSemanticReviewHTML(artifacts.ReviewHTMLPath, previewCandidates, selected, artifacts.ReviewDecisions); err != nil {
		state.Warnings = append(state.Warnings, fmt.Sprintf("写入 review.html 失败: %v", err))
		return state, exitSemanticFailed
	}
	decisionTemplate := semanticBuildDecisionTemplate(asset.AssetID, opts.Target, previewCandidates, selected)
	if err := writeJSONFile(artifacts.ReviewDecisions, decisionTemplate); err != nil {
		state.Warnings = append(state.Warnings, fmt.Sprintf("写入评审模板失败: %v", err))
		return state, exitSemanticFailed
	}

	state.Candidates = candidates
	state.Selected = selected

	// Stage E: 应用 + doctor 闸门（可选）
	if opts.Apply {
		decisionsPath := strings.TrimSpace(opts.DecisionsPath)
		if decisionsPath == "" {
			decisionsPath = artifacts.ReviewDecisions
		}
		finalSelected, err := semanticApplyDecisions(decisionsPath, candidates, selected, opts.TopK, opts.Target)
		if err != nil {
			state.Warnings = append(state.Warnings, fmt.Sprintf("读取评审决策失败: %v", err))
			return state, exitSemanticFailed
		}

		planAfter := plan
		planAfter.Clips = semanticCandidatesToPrepClips(finalSelected)
		checks := runDoctorChecks(doctorOptions{
			Target: opts.Target,
			Strict: opts.Strict,
		}, planAfter)
		summary := summarizeDoctorChecks(checks)
		if summary.Fail > 0 {
			state.Selected = finalSelected
			state.Warnings = append(state.Warnings, fmt.Sprintf("Stage E 未通过 doctor（fail=%d）", summary.Fail))
			return state, exitDoctorFailed
		}

		backupPath := prepPlanPath + ".backup-" + time.Now().UTC().Format("20060102T150405Z")
		if err := copyFileAtomic(prepPlanPath, backupPath); err != nil {
			state.Warnings = append(state.Warnings, fmt.Sprintf("备份 prep-plan 失败: %v", err))
			return state, exitSemanticFailed
		}
		if err := writePrepPlan(prepPlanPath, planAfter); err != nil {
			state.Warnings = append(state.Warnings, fmt.Sprintf("写回 prep-plan 失败: %v", err))
			return state, exitSemanticFailed
		}

		state.Selected = finalSelected
		state.Artifacts.AppliedPlanPath = prepPlanPath
		state.Artifacts.BackupPlanPath = backupPath
	}

	return state, exitOK
}

func semanticExitWithErr(state semanticRunState, asJSON bool, code int, msg string) (semanticRunState, int) {
	state.Warnings = append(state.Warnings, msg)
	return state, code
}

func resolveSemanticLLMConfig(opts semanticOptions) (semanticLLMConfig, error) {
	if opts.NoLLM {
		return semanticLLMConfig{}, nil
	}

	provider := strings.TrimSpace(opts.Provider)
	if provider == "" || provider == "auto" {
		if strings.TrimSpace(os.Getenv("MINGEST_OPENROUTER_API_KEY")) != "" || strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) != "" {
			provider = "openrouter"
		} else {
			provider = "openai"
		}
	}

	cfg := semanticLLMConfig{
		Provider: provider,
	}
	switch provider {
	case "openrouter":
		cfg.APIKey = firstNonEmpty(strings.TrimSpace(opts.APIKey), strings.TrimSpace(os.Getenv("MINGEST_OPENROUTER_API_KEY")), strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")))
		cfg.BaseURL = firstNonEmpty(strings.TrimSpace(opts.BaseURL), strings.TrimSpace(os.Getenv("MINGEST_OPENROUTER_BASE_URL")), defaultOpenRouterBaseURL)
		cfg.Model = firstNonEmpty(strings.TrimSpace(opts.Model), strings.TrimSpace(os.Getenv("MINGEST_LLM_MODEL")), defaultSemanticModelOpenRouter)
		if !strings.Contains(cfg.Model, "/") {
			cfg.Model = "openai/" + cfg.Model
		}
		cfg.Referer = firstNonEmpty(strings.TrimSpace(os.Getenv("MINGEST_OPENROUTER_REFERER")), "https://mingest.local")
		cfg.Title = firstNonEmpty(strings.TrimSpace(os.Getenv("MINGEST_OPENROUTER_TITLE")), "mingest")
	case "openai":
		cfg.APIKey = firstNonEmpty(strings.TrimSpace(opts.APIKey), strings.TrimSpace(os.Getenv("MINGEST_OPENAI_API_KEY")), strings.TrimSpace(os.Getenv("OPENAI_API_KEY")))
		cfg.BaseURL = strings.TrimSpace(opts.BaseURL)
		cfg.Model = firstNonEmpty(strings.TrimSpace(opts.Model), strings.TrimSpace(os.Getenv("MINGEST_LLM_MODEL")), defaultSemanticModelOpenAI)
	default:
		return semanticLLMConfig{}, fmt.Errorf("不支持的 provider: %s", provider)
	}

	if strings.TrimSpace(cfg.APIKey) == "" {
		switch provider {
		case "openrouter":
			return semanticLLMConfig{}, errors.New("未设置 OpenRouter API Key。可用 `--api-key` 或环境变量 `MINGEST_OPENROUTER_API_KEY` / `OPENROUTER_API_KEY`")
		default:
			return semanticLLMConfig{}, errors.New("未设置 OpenAI API Key。可用 `--api-key` 或环境变量 `MINGEST_OPENAI_API_KEY` / `OPENAI_API_KEY`")
		}
	}
	return cfg, nil
}

func semanticRerankWithLLM(candidates []semanticCandidate, target string, cfg semanticLLMConfig) ([]semanticLLMItem, string, error) {
	clientOpts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if strings.TrimSpace(cfg.BaseURL) != "" {
		clientOpts = append(clientOpts, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.Provider == "openrouter" {
		clientOpts = append(clientOpts, option.WithHeader("HTTP-Referer", cfg.Referer))
		clientOpts = append(clientOpts, option.WithHeader("X-Title", cfg.Title))
	}

	client := openai.NewClient(clientOpts...)

	items := make([]map[string]interface{}, 0, len(candidates))
	for _, c := range candidates {
		items = append(items, map[string]interface{}{
			"id":         c.ID,
			"start_sec":  roundMillis(c.StartSec),
			"end_sec":    roundMillis(c.EndSec),
			"duration":   roundMillis(c.DurationSec),
			"base_score": roundMillis(c.BaseScore),
			"text":       semanticShortText(c.Text, 260),
		})
	}
	payload := map[string]interface{}{
		"target":     target,
		"candidates": items,
	}
	payloadBytes, _ := json.Marshal(payload)

	systemPrompt := "你是短视频剪辑总监。请对候选片段做语义重排，返回最值得保留的片段评分。仅输出 JSON。"
	userPrompt := "" +
		"任务:\n" +
		"1) 给每个候选返回 semantic_score (0~1)。\n" +
		"2) 给每个候选标注 type: hook|insight|controversy。\n" +
		"3) reason 用一句话解释。\n" +
		"4) 不要新增 id，不要遗漏 id。\n\n" +
		"输出格式:\n" +
		`{"items":[{"id":"...","semantic_score":0.0,"type":"hook","reason":"..."}]}` + "\n\n" +
		"候选数据:\n" + string(payloadBytes)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Model:       cfg.Model,
		Temperature: openai.Float(0.2),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &shared.ResponseFormatJSONObjectParam{Type: "json_object"},
		},
	})
	if err != nil {
		return nil, "", err
	}
	if len(resp.Choices) == 0 {
		return nil, "", errors.New("模型未返回任何候选结果")
	}
	raw := strings.TrimSpace(resp.Choices[0].Message.Content)
	if raw == "" {
		return nil, "", errors.New("模型返回为空")
	}

	var parsed semanticLLMResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		fixed := extractFirstJSONObject(raw)
		if fixed == "" {
			return nil, raw, fmt.Errorf("解析 JSON 失败: %w", err)
		}
		if err := json.Unmarshal([]byte(fixed), &parsed); err != nil {
			return nil, raw, fmt.Errorf("解析 JSON 失败: %w", err)
		}
	}
	if len(parsed.Items) == 0 {
		return nil, raw, errors.New("模型返回 items 为空")
	}
	return parsed.Items, raw, nil
}

func applySemanticLLMScores(candidates []semanticCandidate, llmItems []semanticLLMItem) []semanticCandidate {
	m := make(map[string]semanticLLMItem, len(llmItems))
	for _, it := range llmItems {
		m[strings.TrimSpace(it.ID)] = it
	}
	out := make([]semanticCandidate, 0, len(candidates))
	for _, c := range candidates {
		item, ok := m[c.ID]
		semanticScore := c.BaseScore
		if ok {
			semanticScore = clamp01(item.SemanticScore)
			c.Type = normalizeSemanticType(item.Type, c.Type)
			c.Reason = strings.TrimSpace(item.Reason)
		}
		c.SemanticScore = roundMillis(semanticScore)
		c.FinalScore = roundMillis(0.55*c.BaseScore + 0.45*semanticScore)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FinalScore == out[j].FinalScore {
			return out[i].BaseScore > out[j].BaseScore
		}
		return out[i].FinalScore > out[j].FinalScore
	})
	return out
}

func applySemanticFallbackScores(candidates []semanticCandidate) []semanticCandidate {
	out := make([]semanticCandidate, 0, len(candidates))
	for _, c := range candidates {
		c.SemanticScore = c.BaseScore
		c.FinalScore = c.BaseScore
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].FinalScore > out[j].FinalScore
	})
	return out
}

func buildSemanticCandidates(cues []subtitleCue, minSec, maxSec float64) []semanticCandidate {
	clean := make([]subtitleCue, 0, len(cues))
	for _, cue := range cues {
		t := strings.TrimSpace(cue.Text)
		if t == "" {
			continue
		}
		if cue.EndSec <= cue.StartSec {
			continue
		}
		clean = append(clean, subtitleCue{
			StartSec: cue.StartSec,
			EndSec:   cue.EndSec,
			Text:     t,
		})
	}
	if len(clean) == 0 {
		return nil
	}

	out := make([]semanticCandidate, 0, 256)
	for i := 0; i < len(clean); i++ {
		var b strings.Builder
		start := clean[i].StartSec
		for j := i; j < len(clean); j++ {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(clean[j].Text)
			end := clean[j].EndSec
			dur := end - start
			if dur > maxSec+1.0 {
				break
			}
			if dur < minSec {
				continue
			}
			text := strings.TrimSpace(b.String())
			if utf8.RuneCountInString(text) < 18 {
				continue
			}
			signals, semType := semanticScoreSignals(text, dur)
			base := semanticBaseScore(signals)
			out = append(out, semanticCandidate{
				ID:            fmt.Sprintf("w%03d", len(out)+1),
				StartSec:      roundMillis(start),
				EndSec:        roundMillis(end),
				DurationSec:   roundMillis(dur),
				CueStartIndex: i,
				CueEndIndex:   j,
				Text:          text,
				BaseScore:     roundMillis(base),
				FinalScore:    roundMillis(base),
				Type:          semType,
				Signals:       signals,
			})
			if len(out) >= maxSemanticCandidateWindows {
				return out
			}
		}
	}
	return out
}

func semanticScoreSignals(text string, durationSec float64) (semanticSignals, string) {
	lower := strings.ToLower(strings.TrimSpace(text))
	runes := float64(utf8.RuneCountInString(text))
	cps := 0.0
	if durationSec > 0 {
		cps = runes / durationSec
	}
	density := 1.0 - math.Min(math.Abs(cps-7.5)/7.5, 1.0)

	hookWords := []string{"先说结论", "你可能", "你以为", "注意", "重点", "结论", "别再", "马上", "核心", "remember", "important", "first", "key"}
	insightWords := []string{"因为", "所以", "本质", "逻辑", "原理", "步骤", "方法", "建议", "总结", "therefore", "because", "method", "insight"}
	controversyWords := []string{"争议", "反对", "错", "骗局", "翻车", "冲突", "质疑", "误区", "controvers", "wrong", "myth", "debate", "hot take"}
	hook := semanticKeywordScore(lower, hookWords)
	insight := semanticKeywordScore(lower, insightWords)
	controversy := semanticKeywordScore(lower, controversyWords)
	question := 0.0
	if strings.Contains(lower, "?") || strings.Contains(lower, "？") {
		question = 1.0
	}

	signals := semanticSignals{
		Hook:        roundMillis(math.Min(1, hook*0.85+question*0.15)),
		Insight:     roundMillis(insight),
		Controversy: roundMillis(controversy),
		Density:     roundMillis(clamp01(density)),
		Question:    roundMillis(question),
	}
	semType := "insight"
	maxVal := signals.Insight
	if signals.Hook > maxVal {
		semType = "hook"
		maxVal = signals.Hook
	}
	if signals.Controversy > maxVal {
		semType = "controversy"
	}
	return signals, semType
}

func semanticKeywordScore(text string, words []string) float64 {
	if len(words) == 0 {
		return 0
	}
	hits := 0
	for _, w := range words {
		if strings.Contains(text, w) {
			hits++
		}
	}
	if hits == 0 {
		return 0
	}
	return clamp01(float64(hits) / 3.0)
}

func semanticBaseScore(s semanticSignals) float64 {
	score := 0.32*s.Hook + 0.30*s.Insight + 0.20*s.Controversy + 0.18*s.Density
	return clamp01(score)
}

func semanticSelectTopCandidates(candidates []semanticCandidate, limit int) []semanticCandidate {
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].BaseScore == candidates[j].BaseScore {
			return candidates[i].DurationSec < candidates[j].DurationSec
		}
		return candidates[i].BaseScore > candidates[j].BaseScore
	})

	out := make([]semanticCandidate, 0, limit)
	for _, c := range candidates {
		dup := false
		for _, kept := range out {
			timeClose := math.Abs(kept.StartSec-c.StartSec) < 1.2 && math.Abs(kept.EndSec-c.EndSec) < 1.2
			textSim := doctorJaccardSimilarity(kept.Text, c.Text) > 0.93
			if timeClose || textSim {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		out = append(out, c)
		if len(out) >= limit {
			break
		}
	}
	for i := range out {
		out[i].ID = fmt.Sprintf("c%03d", i+1)
	}
	return out
}

func semanticTargetDurationRange(target string) (float64, float64) {
	switch target {
	case "shorts":
		return 15, 45
	default:
		return 18, 90
	}
}

func semanticPickFinalCandidates(candidates []semanticCandidate, topK int, target string) []semanticCandidate {
	if len(candidates) == 0 || topK <= 0 {
		return nil
	}
	threshold := doctorThresholdFor(target, false)
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].FinalScore == candidates[j].FinalScore {
			return candidates[i].BaseScore > candidates[j].BaseScore
		}
		return candidates[i].FinalScore > candidates[j].FinalScore
	})

	selected := make([]semanticCandidate, 0, topK)
	usedIDs := make(map[string]struct{}, topK)
	for _, c := range candidates {
		if len(selected) >= topK {
			break
		}
		if c.DurationSec < threshold.ClipMinSec || c.DurationSec > threshold.ClipMaxSec {
			continue
		}
		if !semanticCanAddCandidate(selected, c, threshold) {
			continue
		}
		selected = append(selected, c)
		usedIDs[c.ID] = struct{}{}
	}
	if len(selected) < topK {
		for _, c := range candidates {
			if len(selected) >= topK {
				break
			}
			if _, ok := usedIDs[c.ID]; ok {
				continue
			}
			if !semanticCanAddCandidate(selected, c, doctorThreshold{
				MaxOverlapRatio:       0.35,
				MaxNearDuplicateScore: 0.92,
			}) {
				continue
			}
			selected = append(selected, c)
			usedIDs[c.ID] = struct{}{}
		}
	}
	return selected
}

func semanticCanAddCandidate(selected []semanticCandidate, candidate semanticCandidate, threshold doctorThreshold) bool {
	for _, s := range selected {
		if doctorOverlapRatio(prepClip{StartSec: s.StartSec, EndSec: s.EndSec, DurationSec: s.DurationSec}, prepClip{StartSec: candidate.StartSec, EndSec: candidate.EndSec, DurationSec: candidate.DurationSec}) > threshold.MaxOverlapRatio {
			return false
		}
		if doctorJaccardSimilarity(s.Text, candidate.Text) > threshold.MaxNearDuplicateScore {
			return false
		}
	}
	return true
}

func semanticTopPreviewCandidates(candidates, selected []semanticCandidate, previewLimit int) []semanticCandidate {
	out := make([]semanticCandidate, 0, previewLimit)
	seen := make(map[string]struct{}, previewLimit)

	for _, s := range selected {
		out = append(out, s)
		seen[s.ID] = struct{}{}
		if len(out) >= previewLimit {
			return out
		}
	}
	for _, c := range candidates {
		if len(out) >= previewLimit {
			break
		}
		if _, ok := seen[c.ID]; ok {
			continue
		}
		out = append(out, c)
		seen[c.ID] = struct{}{}
	}
	return out
}

func semanticGeneratePreviewFiles(assetPath string, candidates []semanticCandidate, previewDir string) error {
	if len(candidates) == 0 {
		return nil
	}
	ffmpegPath, ok := detectSemanticFFmpeg()
	if !ok {
		return errors.New("未找到 ffmpeg")
	}
	if err := os.MkdirAll(previewDir, 0o755); err != nil {
		return err
	}

	for i := range candidates {
		c := &candidates[i]
		filename := fmt.Sprintf("%s.mp4", sanitizeFileName(c.ID))
		outPath := filepath.Join(previewDir, filename)
		duration := c.DurationSec
		if duration <= 0 {
			duration = c.EndSec - c.StartSec
		}
		if duration <= 0 {
			continue
		}

		args := []string{
			"-y",
			"-ss", fmt.Sprintf("%.3f", c.StartSec),
			"-t", fmt.Sprintf("%.3f", duration),
			"-i", assetPath,
			"-vf", "scale='min(960,iw)':-2",
			"-c:v", "libx264",
			"-preset", "veryfast",
			"-crf", "30",
			"-c:a", "aac",
			"-movflags", "+faststart",
			outPath,
		}
		cmd := exec.Command(ffmpegPath, args...)
		if err := cmd.Run(); err != nil {
			continue
		}
		c.PreviewPath = filepath.ToSlash(filepath.Join("previews", filename))
	}
	return nil
}

func detectSemanticFFmpeg() (string, bool) {
	exeDir, _ := executableDir()
	wd, _ := os.Getwd()
	return findBinary("ffmpeg", wd, exeDir)
}

func writeSemanticReviewHTML(path string, candidates, selected []semanticCandidate, decisionsPath string) error {
	selectedMap := make(map[string]struct{}, len(selected))
	for _, s := range selected {
		selectedMap[s.ID] = struct{}{}
	}

	var b strings.Builder
	b.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><title>Mingest Semantic Review</title>")
	b.WriteString("<style>body{font-family:ui-sans-serif,system-ui;margin:24px;background:#f8fafc;color:#111}h1{margin-bottom:8px}.tip{background:#eef2ff;padding:10px;border-radius:8px;margin-bottom:16px}.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(320px,1fr));gap:14px}.card{background:#fff;border:1px solid #dbe2ea;border-radius:10px;padding:10px}.meta{font-size:12px;color:#475569}video{width:100%;border-radius:8px;background:#000}.tag{display:inline-block;border-radius:999px;background:#e2e8f0;padding:2px 8px;font-size:12px;margin-right:6px}</style>")
	b.WriteString("</head><body>")
	b.WriteString("<h1>Mingest 语义候选评审</h1>")
	b.WriteString("<div class=\"tip\">建议先看系统已选中的 3 段，再看候补。若需修改，请编辑决策文件：<code>")
	b.WriteString(template.HTMLEscapeString(decisionsPath))
	b.WriteString("</code></div>")
	b.WriteString("<div class=\"grid\">")

	for _, c := range candidates {
		b.WriteString("<div class=\"card\">")
		b.WriteString("<div class=\"meta\"><span class=\"tag\">")
		if _, ok := selectedMap[c.ID]; ok {
			b.WriteString("已选")
		} else {
			b.WriteString("候补")
		}
		b.WriteString("</span>")
		b.WriteString(template.HTMLEscapeString(c.ID))
		b.WriteString(" | ")
		b.WriteString(fmt.Sprintf("%.3fs - %.3fs", c.StartSec, c.EndSec))
		b.WriteString("</div>")
		if strings.TrimSpace(c.PreviewPath) != "" {
			b.WriteString("<video controls preload=\"metadata\" src=\"")
			b.WriteString(template.HTMLEscapeString(c.PreviewPath))
			b.WriteString("\"></video>")
		} else {
			b.WriteString("<div class=\"meta\">（无预览片段，使用时间戳评审）</div>")
		}
		b.WriteString("<div class=\"meta\">final=")
		b.WriteString(fmt.Sprintf("%.3f", c.FinalScore))
		b.WriteString(" | type=")
		b.WriteString(template.HTMLEscapeString(c.Type))
		b.WriteString("</div>")
		b.WriteString("<p>")
		b.WriteString(template.HTMLEscapeString(semanticShortText(c.Text, 180)))
		b.WriteString("</p>")
		b.WriteString("</div>")
	}
	b.WriteString("</div></body></html>")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func semanticBuildDecisionTemplate(assetID, target string, candidates, selected []semanticCandidate) semanticDecisionFile {
	selectedID := make(map[string]int, len(selected))
	for i, s := range selected {
		selectedID[s.ID] = i + 1
	}
	items := make([]semanticDecisionItem, 0, len(candidates))
	for _, c := range candidates {
		rank, keep := selectedID[c.ID]
		items = append(items, semanticDecisionItem{
			ID:   c.ID,
			Keep: keep,
			Rank: rank,
			Note: "",
		})
	}
	return semanticDecisionFile{
		Version:   "semantic-decision-v1",
		Target:    target,
		AssetID:   assetID,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Items:     items,
	}
}

func semanticApplyDecisions(path string, candidates, selected []semanticCandidate, topK int, target string) ([]semanticCandidate, error) {
	decisionBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var decision semanticDecisionFile
	if err := json.Unmarshal(decisionBytes, &decision); err != nil {
		return nil, fmt.Errorf("解析 decisions 文件失败: %w", err)
	}

	all := make(map[string]semanticCandidate, len(candidates)+len(selected))
	for _, c := range candidates {
		all[c.ID] = c
	}
	for _, c := range selected {
		all[c.ID] = c
	}
	if len(decision.Items) == 0 {
		return nil, errors.New("decisions items 为空")
	}

	keep := make([]semanticCandidate, 0, len(decision.Items))
	keepRank := make(map[string]int, len(decision.Items))
	drop := make(map[string]struct{}, len(decision.Items))
	for _, it := range decision.Items {
		id := strings.TrimSpace(it.ID)
		if id == "" {
			continue
		}
		c, ok := all[id]
		if !ok {
			continue
		}
		if it.Keep {
			keep = append(keep, c)
			keepRank[id] = it.Rank
		} else {
			drop[id] = struct{}{}
		}
	}
	sort.Slice(keep, func(i, j int) bool {
		ri := keepRank[keep[i].ID]
		rj := keepRank[keep[j].ID]
		if ri > 0 && rj > 0 && ri != rj {
			return ri < rj
		}
		return keep[i].FinalScore > keep[j].FinalScore
	})

	threshold := doctorThresholdFor(target, false)
	final := make([]semanticCandidate, 0, topK)
	for _, c := range keep {
		if len(final) >= topK {
			break
		}
		if !semanticCanAddCandidate(final, c, threshold) {
			continue
		}
		final = append(final, c)
	}
	if len(final) < topK {
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].FinalScore > candidates[j].FinalScore })
		for _, c := range candidates {
			if len(final) >= topK {
				break
			}
			if _, blocked := drop[c.ID]; blocked {
				continue
			}
			if !semanticCanAddCandidate(final, c, threshold) {
				continue
			}
			final = append(final, c)
		}
	}
	if len(final) == 0 {
		return nil, errors.New("决策后没有可用片段")
	}
	return final, nil
}

func semanticCandidatesToPrepClips(in []semanticCandidate) []prepClip {
	out := make([]prepClip, 0, len(in))
	for i, c := range in {
		out = append(out, prepClip{
			Index:       i + 1,
			StartSec:    roundMillis(c.StartSec),
			EndSec:      roundMillis(c.EndSec),
			DurationSec: roundMillis(c.DurationSec),
			Label:       fmt.Sprintf("semantic-%02d", i+1),
			Reason:      "语义候选（AI + 人工决策）",
		})
	}
	return out
}

func createSemanticArtifacts(asset prepResolvedAsset) (semanticArtifacts, error) {
	ts := time.Now().UTC().Format("20060102T150405Z")
	base := filepath.Join(filepath.Dir(asset.OutputPath), ".mingest", "semantic", asset.AssetID, ts)
	previewDir := filepath.Join(base, "previews")
	if err := os.MkdirAll(previewDir, 0o755); err != nil {
		return semanticArtifacts{}, err
	}
	return semanticArtifacts{
		BundleDir:       base,
		StageAPath:      filepath.Join(base, "stage-a-candidates.json"),
		StageBPath:      filepath.Join(base, "stage-b-llm.json"),
		StageCPath:      filepath.Join(base, "stage-c-selected.json"),
		ReviewHTMLPath:  filepath.Join(base, "review.html"),
		ReviewDecisions: filepath.Join(base, "review-decisions.template.json"),
		PreviewDir:      previewDir,
	}, nil
}

func writeJSONFile(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func buildSemanticJSONResult(state semanticRunState, opts semanticOptions, exitCode int) semanticJSONResult {
	ok := exitCode == exitOK
	result := semanticJSONResult{
		OK:             ok,
		ExitCode:       exitCode,
		AssetID:        strings.TrimSpace(state.Asset.AssetID),
		AssetRef:       strings.TrimSpace(opts.AssetRef),
		AssetPath:      strings.TrimSpace(state.Asset.OutputPath),
		Target:         opts.Target,
		Provider:       state.Provider,
		Model:          state.Model,
		UsedLLM:        state.UsedLLM,
		Applied:        opts.Apply && state.Artifacts.AppliedPlanPath != "",
		CandidateCount: len(state.Candidates),
		SelectedCount:  len(state.Selected),
		Artifacts:      state.Artifacts,
		Warnings:       state.Warnings,
	}
	if !ok && len(state.Warnings) > 0 {
		result.Error = state.Warnings[len(state.Warnings)-1]
	}
	if opts.Apply && len(state.Selected) > 0 {
		p := state.Plan
		p.Clips = semanticCandidatesToPrepClips(state.Selected)
		result.DoctorSummary = summarizeDoctorChecks(runDoctorChecks(doctorOptions{
			Target: opts.Target,
			Strict: opts.Strict,
		}, p))
	}
	return result
}

func printSemanticJSON(v semanticJSONResult) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("JSON 序列化失败: %v", err)
		return
	}
	fmt.Println(string(data))
}

func printSemanticHuman(state semanticRunState, opts semanticOptions, exitCode int) {
	status := "PASS"
	if exitCode != exitOK {
		status = "FAIL"
	}
	fmt.Printf("semantic: %s\n", status)
	if strings.TrimSpace(state.Asset.AssetID) != "" {
		fmt.Printf("asset_id: %s\n", state.Asset.AssetID)
	}
	if strings.TrimSpace(state.Asset.OutputPath) != "" {
		fmt.Printf("asset_path: %s\n", state.Asset.OutputPath)
	}
	fmt.Printf("target: %s\n", opts.Target)
	fmt.Printf("provider: %s\n", firstNonEmpty(state.Provider, "rule-only"))
	fmt.Printf("model: %s\n", firstNonEmpty(state.Model, "-"))
	fmt.Printf("used_llm: %v\n", state.UsedLLM)
	fmt.Printf("candidate_count: %d\n", len(state.Candidates))
	fmt.Printf("selected_count: %d\n", len(state.Selected))
	if strings.TrimSpace(state.Artifacts.BundleDir) != "" {
		fmt.Printf("semantic_dir: %s\n", state.Artifacts.BundleDir)
	}
	if strings.TrimSpace(state.Artifacts.ReviewHTMLPath) != "" {
		fmt.Printf("review_html: %s\n", state.Artifacts.ReviewHTMLPath)
	}
	if strings.TrimSpace(state.Artifacts.ReviewDecisions) != "" {
		fmt.Printf("decisions_template: %s\n", state.Artifacts.ReviewDecisions)
	}
	if opts.Apply && strings.TrimSpace(state.Artifacts.AppliedPlanPath) != "" {
		fmt.Printf("applied_prep_plan: %s\n", state.Artifacts.AppliedPlanPath)
		fmt.Printf("backup_prep_plan: %s\n", state.Artifacts.BackupPlanPath)
	}
	for _, w := range state.Warnings {
		fmt.Printf("warning: %s\n", w)
	}
}

func sanitizeFileName(in string) string {
	in = strings.TrimSpace(in)
	if in == "" {
		return "clip"
	}
	var b strings.Builder
	for _, r := range in {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "clip"
	}
	return out
}

func semanticShortText(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	rs := []rune(s)
	return strings.TrimSpace(string(rs[:maxRunes])) + "..."
}

func extractFirstJSONObject(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return ""
	}
	return raw[start : end+1]
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func normalizeSemanticType(t string, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "hook":
		return "hook"
	case "insight":
		return "insight"
	case "controversy":
		return "controversy"
	default:
		if strings.TrimSpace(fallback) == "" {
			return "insight"
		}
		return fallback
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
