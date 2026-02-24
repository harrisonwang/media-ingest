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
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type prepOptions struct {
	AssetRef      string `json:"asset_ref"`
	Goal          string `json:"goal"`
	Lang          string `json:"lang"`
	MaxClips      int    `json:"max_clips"`
	ClipSeconds   int    `json:"clip_seconds"`
	SubtitleStyle string `json:"subtitle_style"`
	JSON          bool   `json:"-"`
}

type prepResolvedAsset struct {
	AssetID    string `json:"asset_id"`
	URL        string `json:"url,omitempty"`
	Platform   string `json:"platform,omitempty"`
	Title      string `json:"title"`
	OutputPath string `json:"output_path"`
}

type mediaProbe struct {
	DurationSec float64 `json:"duration_sec"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	FPS         float64 `json:"fps"`
	VideoCodec  string  `json:"video_codec"`
	AudioTracks int     `json:"audio_tracks"`
}

type prepClip struct {
	Index       int     `json:"index"`
	StartSec    float64 `json:"start_sec"`
	EndSec      float64 `json:"end_sec"`
	DurationSec float64 `json:"duration_sec"`
	Label       string  `json:"label"`
	Reason      string  `json:"reason"`
}

type prepPlan struct {
	Version   string            `json:"version"`
	CreatedAt string            `json:"created_at"`
	Asset     prepResolvedAsset `json:"asset"`
	Options   prepOptions       `json:"options"`
	Probe     mediaProbe        `json:"probe"`
	Clips     []prepClip        `json:"clips"`
	Outputs   prepOutputFiles   `json:"outputs"`
}

type prepOutputFiles struct {
	BundleDir        string `json:"bundle_dir"`
	PlanPath         string `json:"plan_path"`
	MarkersCSV       string `json:"markers_csv"`
	SubtitleTemplate string `json:"subtitle_template,omitempty"`
}

type prepJSONResult struct {
	OK               bool    `json:"ok"`
	ExitCode         int     `json:"exit_code"`
	Error            string  `json:"error,omitempty"`
	AssetID          string  `json:"asset_id,omitempty"`
	AssetPath        string  `json:"asset_path,omitempty"`
	Goal             string  `json:"goal,omitempty"`
	DurationSec      float64 `json:"duration_sec,omitempty"`
	ClipCount        int     `json:"clip_count,omitempty"`
	BundleDir        string  `json:"bundle_dir,omitempty"`
	PlanPath         string  `json:"plan_path,omitempty"`
	MarkersCSV       string  `json:"markers_csv,omitempty"`
	SubtitleTemplate string  `json:"subtitle_template,omitempty"`
}

func parsePrepOptions(args []string) (prepOptions, error) {
	opts := prepOptions{
		Lang:          "auto",
		SubtitleStyle: "clean",
	}

	var maxClipsProvided bool
	var clipSecondsProvided bool

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "--json":
			opts.JSON = true
		case arg == "--goal":
			if i+1 >= len(args) {
				return prepOptions{}, fmt.Errorf("`--goal` 缺少参数")
			}
			i++
			opts.Goal = strings.ToLower(strings.TrimSpace(args[i]))
		case strings.HasPrefix(arg, "--goal="):
			opts.Goal = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(arg, "--goal=")))
		case arg == "--lang":
			if i+1 >= len(args) {
				return prepOptions{}, fmt.Errorf("`--lang` 缺少参数")
			}
			i++
			opts.Lang = strings.ToLower(strings.TrimSpace(args[i]))
		case strings.HasPrefix(arg, "--lang="):
			opts.Lang = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(arg, "--lang=")))
		case arg == "--max-clips":
			if i+1 >= len(args) {
				return prepOptions{}, fmt.Errorf("`--max-clips` 缺少参数")
			}
			i++
			v := strings.TrimSpace(args[i])
			n, err := strconv.Atoi(v)
			if err != nil {
				return prepOptions{}, fmt.Errorf("`--max-clips` 必须是整数: %s", v)
			}
			opts.MaxClips = n
			maxClipsProvided = true
		case strings.HasPrefix(arg, "--max-clips="):
			v := strings.TrimSpace(strings.TrimPrefix(arg, "--max-clips="))
			n, err := strconv.Atoi(v)
			if err != nil {
				return prepOptions{}, fmt.Errorf("`--max-clips` 必须是整数: %s", v)
			}
			opts.MaxClips = n
			maxClipsProvided = true
		case arg == "--clip-seconds":
			if i+1 >= len(args) {
				return prepOptions{}, fmt.Errorf("`--clip-seconds` 缺少参数")
			}
			i++
			v := strings.TrimSpace(args[i])
			n, err := strconv.Atoi(v)
			if err != nil {
				return prepOptions{}, fmt.Errorf("`--clip-seconds` 必须是整数: %s", v)
			}
			opts.ClipSeconds = n
			clipSecondsProvided = true
		case strings.HasPrefix(arg, "--clip-seconds="):
			v := strings.TrimSpace(strings.TrimPrefix(arg, "--clip-seconds="))
			n, err := strconv.Atoi(v)
			if err != nil {
				return prepOptions{}, fmt.Errorf("`--clip-seconds` 必须是整数: %s", v)
			}
			opts.ClipSeconds = n
			clipSecondsProvided = true
		case arg == "--subtitle-style":
			if i+1 >= len(args) {
				return prepOptions{}, fmt.Errorf("`--subtitle-style` 缺少参数")
			}
			i++
			opts.SubtitleStyle = strings.ToLower(strings.TrimSpace(args[i]))
		case strings.HasPrefix(arg, "--subtitle-style="):
			opts.SubtitleStyle = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(arg, "--subtitle-style=")))
		case strings.HasPrefix(arg, "-"):
			return prepOptions{}, fmt.Errorf("不支持的参数: %s", arg)
		default:
			if opts.AssetRef != "" {
				return prepOptions{}, fmt.Errorf("`mingest prep` 仅支持一个 asset_ref")
			}
			opts.AssetRef = arg
		}
	}

	if strings.TrimSpace(opts.AssetRef) == "" {
		return prepOptions{}, fmt.Errorf("缺少 asset_ref。用法: mingest prep <asset_ref> --goal <subtitle|highlights|shorts>")
	}

	switch opts.Goal {
	case "subtitle", "highlights", "shorts":
	default:
		return prepOptions{}, fmt.Errorf("`--goal` 仅支持 subtitle|highlights|shorts")
	}

	switch opts.Lang {
	case "auto", "zh", "en":
	default:
		return prepOptions{}, fmt.Errorf("`--lang` 仅支持 auto|zh|en")
	}

	switch opts.SubtitleStyle {
	case "clean", "shorts":
	default:
		return prepOptions{}, fmt.Errorf("`--subtitle-style` 仅支持 clean|shorts")
	}

	if maxClipsProvided && opts.MaxClips <= 0 {
		return prepOptions{}, fmt.Errorf("`--max-clips` 必须大于 0")
	}
	if clipSecondsProvided && opts.ClipSeconds <= 0 {
		return prepOptions{}, fmt.Errorf("`--clip-seconds` 必须大于 0")
	}

	defaultMax, defaultClipSeconds := prepGoalDefaults(opts.Goal)
	if !maxClipsProvided {
		opts.MaxClips = defaultMax
	}
	if !clipSecondsProvided {
		opts.ClipSeconds = defaultClipSeconds
	}

	return opts, nil
}

func runPrep(opts prepOptions) int {
	asset, err := resolvePrepAsset(opts.AssetRef)
	if err != nil {
		return prepExitWithErr(opts.JSON, exitDownloadFailed, err.Error())
	}

	ffprobePath, err := detectPrepFFprobe()
	if err != nil {
		var depErr dependencyError
		if errors.As(err, &depErr) {
			return prepExitWithErr(opts.JSON, depErr.ExitCode, depErr.Message)
		}
		return prepExitWithErr(opts.JSON, exitDownloadFailed, fmt.Sprintf("依赖检测失败: %v", err))
	}

	probe, err := probeMediaFile(ffprobePath, asset.OutputPath)
	if err != nil {
		return prepExitWithErr(opts.JSON, exitDownloadFailed, fmt.Sprintf("读取媒体元数据失败: %v", err))
	}

	if strings.TrimSpace(asset.AssetID) == "" {
		assetID, err := computeAssetID(asset.OutputPath)
		if err != nil {
			return prepExitWithErr(opts.JSON, exitDownloadFailed, fmt.Sprintf("生成 asset_id 失败: %v", err))
		}
		asset.AssetID = assetID
	}
	if strings.TrimSpace(asset.Title) == "" {
		asset.Title = filepath.Base(asset.OutputPath)
	}

	clips := buildPrepClips(probe.DurationSec, opts.MaxClips, opts.ClipSeconds, opts.Goal)

	outputs, err := createPrepBundle(asset.OutputPath, asset.AssetID)
	if err != nil {
		return prepExitWithErr(opts.JSON, exitDownloadFailed, fmt.Sprintf("创建 prep 输出目录失败: %v", err))
	}
	if opts.Goal == "subtitle" || opts.Goal == "shorts" {
		outputs.SubtitleTemplate = filepath.Join(outputs.BundleDir, "subtitle-template.srt")
	}

	planDoc := prepPlan{
		Version:   "prep-v1",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Asset:     asset,
		Options:   opts,
		Probe:     probe,
		Clips:     clips,
		Outputs:   outputs,
	}

	if err := writePrepPlan(outputs.PlanPath, planDoc); err != nil {
		return prepExitWithErr(opts.JSON, exitDownloadFailed, fmt.Sprintf("写入 prep-plan.json 失败: %v", err))
	}
	if err := writePrepMarkers(outputs.MarkersCSV, clips); err != nil {
		return prepExitWithErr(opts.JSON, exitDownloadFailed, fmt.Sprintf("写入 markers.csv 失败: %v", err))
	}
	if outputs.SubtitleTemplate != "" {
		if err := writeSubtitleTemplate(outputs.SubtitleTemplate, clips, opts.SubtitleStyle, opts.Lang); err != nil {
			return prepExitWithErr(opts.JSON, exitDownloadFailed, fmt.Sprintf("写入 subtitle-template.srt 失败: %v", err))
		}
	}

	if opts.JSON {
		printPrepJSON(prepJSONResult{
			OK:               true,
			ExitCode:         exitOK,
			AssetID:          asset.AssetID,
			AssetPath:        asset.OutputPath,
			Goal:             opts.Goal,
			DurationSec:      roundMillis(probe.DurationSec),
			ClipCount:        len(clips),
			BundleDir:        outputs.BundleDir,
			PlanPath:         outputs.PlanPath,
			MarkersCSV:       outputs.MarkersCSV,
			SubtitleTemplate: outputs.SubtitleTemplate,
		})
		return exitOK
	}

	fmt.Printf("asset_id: %s\n", asset.AssetID)
	fmt.Printf("asset_path: %s\n", asset.OutputPath)
	fmt.Printf("goal: %s\n", opts.Goal)
	fmt.Printf("duration_sec: %.3f\n", roundMillis(probe.DurationSec))
	fmt.Printf("clip_count: %d\n", len(clips))
	fmt.Printf("bundle_dir: %s\n", outputs.BundleDir)
	fmt.Printf("plan_path: %s\n", outputs.PlanPath)
	fmt.Printf("markers_csv: %s\n", outputs.MarkersCSV)
	if outputs.SubtitleTemplate != "" {
		fmt.Printf("subtitle_template: %s\n", outputs.SubtitleTemplate)
	}
	return exitOK
}

func prepExitWithErr(asJSON bool, exitCode int, msg string) int {
	if asJSON {
		printPrepJSON(prepJSONResult{
			OK:       false,
			ExitCode: exitCode,
			Error:    msg,
		})
	} else {
		log.Print(msg)
	}
	return exitCode
}

func prepGoalDefaults(goal string) (maxClips int, clipSeconds int) {
	switch goal {
	case "shorts":
		return 3, 30
	default:
		return 5, 45
	}
}

func resolvePrepAsset(assetRef string) (prepResolvedAsset, error) {
	ref := strings.TrimSpace(assetRef)
	if ref == "" {
		return prepResolvedAsset{}, fmt.Errorf("asset_ref 不能为空")
	}

	if p, ok := resolveLocalAssetPath(ref); ok {
		assetID, err := computeAssetID(p)
		if err != nil {
			return prepResolvedAsset{}, fmt.Errorf("读取本地文件成功，但生成 asset_id 失败: %w", err)
		}
		return prepResolvedAsset{
			AssetID:    assetID,
			Title:      filepath.Base(p),
			OutputPath: p,
		}, nil
	}

	records, err := readAssetRecords()
	if err != nil {
		return prepResolvedAsset{}, err
	}
	if len(records) == 0 {
		return prepResolvedAsset{}, fmt.Errorf("未找到素材: %s（既不是本地文件，也不在 mingest 索引中）", ref)
	}

	sort.Slice(records, func(i, j int) bool {
		return parseRecordTime(records[i]).After(parseRecordTime(records[j]))
	})

	for _, r := range records {
		if prepRecordMatchesRef(r, ref) {
			if p, ok := resolveLocalAssetPath(r.OutputPath); ok {
				return prepResolvedAsset{
					AssetID:    strings.TrimSpace(r.AssetID),
					URL:        strings.TrimSpace(r.URL),
					Platform:   strings.TrimSpace(r.Platform),
					Title:      strings.TrimSpace(r.Title),
					OutputPath: p,
				}, nil
			}
			return prepResolvedAsset{}, fmt.Errorf("在索引中找到了 %s，但本地文件不存在: %s", ref, strings.TrimSpace(r.OutputPath))
		}
	}

	return prepResolvedAsset{}, fmt.Errorf("未在索引中找到素材: %s", ref)
}

func resolveLocalAssetPath(raw string) (string, bool) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "", false
	}
	if fileExists(p) {
		if abs, err := filepath.Abs(p); err == nil {
			return abs, true
		}
		return p, true
	}
	if abs, err := filepath.Abs(p); err == nil && fileExists(abs) {
		return abs, true
	}
	return "", false
}

func prepRecordMatchesRef(r assetRecord, ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false
	}

	if strings.EqualFold(strings.TrimSpace(r.AssetID), ref) {
		return true
	}
	if strings.TrimSpace(r.URL) == ref {
		return true
	}

	recPath := strings.TrimSpace(r.OutputPath)
	if recPath != "" {
		if recPath == ref {
			return true
		}
		if absRef, err := filepath.Abs(ref); err == nil {
			if absRec, err := filepath.Abs(recPath); err == nil && filepath.Clean(absRec) == filepath.Clean(absRef) {
				return true
			}
		}
	}
	return false
}

func detectPrepFFprobe() (string, error) {
	exeDir, err := executableDir()
	if err != nil {
		return "", err
	}
	wd, _ := os.Getwd()
	ffprobePath, ok := findBinary("ffprobe", wd, exeDir)
	if !ok {
		return "", dependencyError{
			Message:  "未找到 ffprobe。请将 ffprobe 与 ffmpeg 放在同一目录（工作目录或程序同目录），或加入 PATH。",
			ExitCode: exitFFmpegMissing,
		}
	}
	return ffprobePath, nil
}

func probeMediaFile(ffprobePath, mediaPath string) (mediaProbe, error) {
	type ffprobeStream struct {
		CodecType    string `json:"codec_type"`
		CodecName    string `json:"codec_name"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		RFrameRate   string `json:"r_frame_rate"`
		AvgFrameRate string `json:"avg_frame_rate"`
	}
	type ffprobeFormat struct {
		Duration string `json:"duration"`
	}
	type ffprobeResult struct {
		Streams []ffprobeStream `json:"streams"`
		Format  ffprobeFormat   `json:"format"`
	}

	args := []string{
		"-v", "error",
		"-show_entries", "format=duration:stream=codec_type,codec_name,width,height,avg_frame_rate,r_frame_rate",
		"-of", "json",
		mediaPath,
	}
	cmd := exec.Command(ffprobePath, args...)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(err.Error())
		}
		return mediaProbe{}, fmt.Errorf("ffprobe 执行失败: %s", detail)
	}

	var parsed ffprobeResult
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		return mediaProbe{}, fmt.Errorf("解析 ffprobe 输出失败: %w", err)
	}

	var probe mediaProbe
	if parsed.Format.Duration != "" {
		if v, err := strconv.ParseFloat(strings.TrimSpace(parsed.Format.Duration), 64); err == nil && v > 0 {
			probe.DurationSec = roundMillis(v)
		}
	}

	for _, s := range parsed.Streams {
		switch strings.TrimSpace(s.CodecType) {
		case "video":
			if probe.Width == 0 && probe.Height == 0 {
				probe.Width = s.Width
				probe.Height = s.Height
				probe.VideoCodec = strings.TrimSpace(s.CodecName)
				probe.FPS = roundMillis(selectFrameRate(s.AvgFrameRate, s.RFrameRate))
			}
		case "audio":
			probe.AudioTracks++
		}
	}

	if probe.Width == 0 || probe.Height == 0 {
		return mediaProbe{}, fmt.Errorf("未检测到视频流（文件可能不是视频）")
	}

	return probe, nil
}

func selectFrameRate(avgFrameRate, rawFrameRate string) float64 {
	if v := parseRate(strings.TrimSpace(avgFrameRate)); v > 0 {
		return v
	}
	return parseRate(strings.TrimSpace(rawFrameRate))
}

func parseRate(v string) float64 {
	if v == "" || v == "0/0" {
		return 0
	}
	if strings.Contains(v, "/") {
		parts := strings.SplitN(v, "/", 2)
		if len(parts) != 2 {
			return 0
		}
		a, errA := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		b, errB := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if errA != nil || errB != nil || b == 0 {
			return 0
		}
		return a / b
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return n
}

func buildPrepClips(durationSec float64, maxClips int, clipSeconds int, goal string) []prepClip {
	if durationSec <= 0 || maxClips <= 0 || clipSeconds <= 0 {
		return []prepClip{}
	}

	clipLen := float64(clipSeconds)
	if durationSec <= clipLen {
		return []prepClip{
			{
				Index:       1,
				StartSec:    0,
				EndSec:      roundMillis(durationSec),
				DurationSec: roundMillis(durationSec),
				Label:       "clip-01",
				Reason:      prepClipReason(goal),
			},
		}
	}

	maxUseful := int(math.Ceil(durationSec / clipLen))
	if maxUseful < 1 {
		maxUseful = 1
	}
	if maxClips > maxUseful {
		maxClips = maxUseful
	}

	step := durationSec / float64(maxClips+1)
	out := make([]prepClip, 0, maxClips)
	for i := 0; i < maxClips; i++ {
		center := step * float64(i+1)
		start := center - clipLen/2
		if start < 0 {
			start = 0
		}
		if start+clipLen > durationSec {
			start = durationSec - clipLen
		}
		if start < 0 {
			start = 0
		}

		end := start + clipLen
		if end > durationSec {
			end = durationSec
		}

		out = append(out, prepClip{
			Index:       i + 1,
			StartSec:    roundMillis(start),
			EndSec:      roundMillis(end),
			DurationSec: roundMillis(end - start),
			Label:       fmt.Sprintf("clip-%02d", i+1),
			Reason:      prepClipReason(goal),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].StartSec < out[j].StartSec
	})
	return out
}

func prepClipReason(goal string) string {
	switch goal {
	case "subtitle":
		return "优先做字幕校对"
	case "shorts":
		return "候选短视频片段，适合二次竖版处理"
	default:
		return "候选高光片段，需要人工确认"
	}
}

func createPrepBundle(assetPath, assetID string) (prepOutputFiles, error) {
	ts := time.Now().UTC().Format("20060102T150405Z")
	base := filepath.Join(filepath.Dir(assetPath), ".mingest", "prep", assetID, ts)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return prepOutputFiles{}, err
	}
	return prepOutputFiles{
		BundleDir:  base,
		PlanPath:   filepath.Join(base, "prep-plan.json"),
		MarkersCSV: filepath.Join(base, "markers.csv"),
	}, nil
}

func writePrepPlan(path string, plan prepPlan) error {
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writePrepMarkers(path string, clips []prepClip) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write([]string{"index", "start_sec", "end_sec", "duration_sec", "label", "reason"}); err != nil {
		return err
	}
	for _, c := range clips {
		row := []string{
			strconv.Itoa(c.Index),
			fmt.Sprintf("%.3f", roundMillis(c.StartSec)),
			fmt.Sprintf("%.3f", roundMillis(c.EndSec)),
			fmt.Sprintf("%.3f", roundMillis(c.DurationSec)),
			c.Label,
			c.Reason,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func writeSubtitleTemplate(path string, clips []prepClip, style, lang string) error {
	var builder strings.Builder
	if len(clips) == 0 {
		builder.WriteString("1\n")
		builder.WriteString("00:00:00,000 --> 00:00:05,000\n")
		builder.WriteString(fmt.Sprintf("[%s/%s] TODO: 填写字幕内容\n\n", style, lang))
	} else {
		for _, c := range clips {
			builder.WriteString(strconv.Itoa(c.Index))
			builder.WriteByte('\n')
			builder.WriteString(formatSRTTime(c.StartSec))
			builder.WriteString(" --> ")
			builder.WriteString(formatSRTTime(c.EndSec))
			builder.WriteByte('\n')
			builder.WriteString(fmt.Sprintf("[%s/%s] TODO: %s\n\n", style, lang, c.Label))
		}
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func formatSRTTime(sec float64) string {
	totalMillis := int64(math.Round(sec * 1000))
	if totalMillis < 0 {
		totalMillis = 0
	}
	ms := totalMillis % 1000
	totalSeconds := totalMillis / 1000
	s := totalSeconds % 60
	totalMinutes := totalSeconds / 60
	m := totalMinutes % 60
	h := totalMinutes / 60
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

func roundMillis(v float64) float64 {
	return math.Round(v*1000) / 1000
}

func printPrepJSON(v prepJSONResult) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("JSON 序列化失败: %v", err)
		return
	}
	fmt.Println(string(data))
}
