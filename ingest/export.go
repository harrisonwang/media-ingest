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
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type exportOptions struct {
	AssetRef string
	To       string
	With     []string
	OutDir   string
	Zip      bool
	JSON     bool
}

type exportJSONResult struct {
	OK          bool              `json:"ok"`
	ExitCode    int               `json:"exit_code"`
	Error       string            `json:"error,omitempty"`
	AssetID     string            `json:"asset_id,omitempty"`
	AssetPath   string            `json:"asset_path,omitempty"`
	To          string            `json:"to,omitempty"`
	With        []string          `json:"with,omitempty"`
	PrepPlan    string            `json:"prep_plan,omitempty"`
	OutDir      string            `json:"out_dir,omitempty"`
	Exported    map[string]string `json:"exported,omitempty"`
	ZipPath     string            `json:"zip_path,omitempty"`
	SubtitleSrc string            `json:"subtitle_source,omitempty"`
}

func parseExportOptions(args []string) (exportOptions, error) {
	opts := exportOptions{
		With: []string{"srt", "edl", "csv"},
	}

	withProvided := false

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "--json":
			opts.JSON = true
		case arg == "--zip":
			opts.Zip = true
		case arg == "--to":
			if i+1 >= len(args) {
				return exportOptions{}, fmt.Errorf("`--to` 缺少参数")
			}
			i++
			opts.To = strings.ToLower(strings.TrimSpace(args[i]))
		case strings.HasPrefix(arg, "--to="):
			opts.To = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(arg, "--to=")))
		case arg == "--out-dir":
			if i+1 >= len(args) {
				return exportOptions{}, fmt.Errorf("`--out-dir` 缺少参数")
			}
			i++
			opts.OutDir = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--out-dir="):
			opts.OutDir = strings.TrimSpace(strings.TrimPrefix(arg, "--out-dir="))
		case arg == "--with":
			if i+1 >= len(args) {
				return exportOptions{}, fmt.Errorf("`--with` 缺少参数")
			}
			i++
			formats, err := parseExportFormats(args[i])
			if err != nil {
				return exportOptions{}, err
			}
			opts.With = formats
			withProvided = true
		case strings.HasPrefix(arg, "--with="):
			formats, err := parseExportFormats(strings.TrimPrefix(arg, "--with="))
			if err != nil {
				return exportOptions{}, err
			}
			opts.With = formats
			withProvided = true
		case strings.HasPrefix(arg, "-"):
			return exportOptions{}, fmt.Errorf("不支持的参数: %s", arg)
		default:
			if opts.AssetRef != "" {
				return exportOptions{}, fmt.Errorf("`mingest export` 仅支持一个 asset_ref")
			}
			opts.AssetRef = arg
		}
	}

	if strings.TrimSpace(opts.AssetRef) == "" {
		return exportOptions{}, fmt.Errorf("缺少 asset_ref。用法: mingest export <asset_ref> --to <premiere|resolve>")
	}
	switch opts.To {
	case "premiere", "resolve":
	default:
		return exportOptions{}, fmt.Errorf("`--to` 仅支持 premiere|resolve")
	}
	if strings.TrimSpace(opts.OutDir) != "" {
		if abs, err := filepath.Abs(opts.OutDir); err == nil {
			opts.OutDir = abs
		}
	}
	if withProvided && len(opts.With) == 0 {
		return exportOptions{}, fmt.Errorf("`--with` 至少包含一个格式: srt|edl|csv")
	}

	return opts, nil
}

func parseExportFormats(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.ToLower(strings.TrimSpace(p))
		if v == "" {
			continue
		}
		switch v {
		case "srt", "edl", "csv":
		default:
			return nil, fmt.Errorf("`--with` 仅支持 srt|edl|csv（收到: %s）", v)
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out, nil
}

func runExport(opts exportOptions) int {
	asset, err := resolvePrepAsset(opts.AssetRef)
	if err != nil {
		return exportExitWithErr(opts.JSON, exitDownloadFailed, err.Error())
	}
	if strings.TrimSpace(asset.AssetID) == "" {
		assetID, err := computeAssetID(asset.OutputPath)
		if err != nil {
			return exportExitWithErr(opts.JSON, exitDownloadFailed, fmt.Sprintf("生成 asset_id 失败: %v", err))
		}
		asset.AssetID = assetID
	}

	prepDir, prepPlanPath, err := latestPrepBundle(asset)
	if err != nil {
		return exportExitWithErr(opts.JSON, exitDownloadFailed, err.Error())
	}

	plan, err := readPrepPlan(prepPlanPath)
	if err != nil {
		return exportExitWithErr(opts.JSON, exitDownloadFailed, fmt.Sprintf("读取 prep-plan.json 失败: %v", err))
	}

	outDir := strings.TrimSpace(opts.OutDir)
	if outDir == "" {
		outDir = filepath.Join(filepath.Dir(asset.OutputPath), ".mingest", "export", asset.AssetID, time.Now().UTC().Format("20060102T150405Z"))
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return exportExitWithErr(opts.JSON, exitDownloadFailed, fmt.Sprintf("创建导出目录失败: %v", err))
	}

	exported := make(map[string]string, len(opts.With))
	for _, f := range opts.With {
		switch f {
		case "srt":
			target := filepath.Join(outDir, asset.AssetID+".srt")
			src := strings.TrimSpace(plan.Outputs.SubtitlePath)
			if src == "" || !fileExists(src) {
				src = strings.TrimSpace(plan.Outputs.SubtitleTemplate)
			}
			if src == "" || !fileExists(src) {
				return exportExitWithErr(opts.JSON, exitDownloadFailed, "prep 结果中没有可导出的字幕文件（subtitle_path/subtitle_template 均不存在）")
			}
			if err := copyFileAtomic(src, target); err != nil {
				return exportExitWithErr(opts.JSON, exitDownloadFailed, fmt.Sprintf("导出 srt 失败: %v", err))
			}
			exported["srt"] = target
		case "csv":
			target := filepath.Join(outDir, asset.AssetID+"-markers.csv")
			if src := strings.TrimSpace(plan.Outputs.MarkersCSV); src != "" && fileExists(src) {
				if err := copyFileAtomic(src, target); err != nil {
					return exportExitWithErr(opts.JSON, exitDownloadFailed, fmt.Sprintf("导出 csv 失败: %v", err))
				}
			} else if err := writePrepMarkers(target, plan.Clips); err != nil {
				return exportExitWithErr(opts.JSON, exitDownloadFailed, fmt.Sprintf("导出 csv 失败: %v", err))
			}
			exported["csv"] = target
		case "edl":
			target := filepath.Join(outDir, asset.AssetID+".edl")
			if err := writeExportEDL(target, asset.AssetID, plan.Clips, plan.Probe.FPS); err != nil {
				return exportExitWithErr(opts.JSON, exitDownloadFailed, fmt.Sprintf("导出 edl 失败: %v", err))
			}
			exported["edl"] = target
		}
	}

	zipPath := ""
	if opts.Zip {
		zipPath = outDir + ".zip"
		if err := zipDir(outDir, zipPath); err != nil {
			return exportExitWithErr(opts.JSON, exitDownloadFailed, fmt.Sprintf("打包 zip 失败: %v", err))
		}
	}

	if opts.JSON {
		result := exportJSONResult{
			OK:        true,
			ExitCode:  exitOK,
			AssetID:   asset.AssetID,
			AssetPath: asset.OutputPath,
			To:        opts.To,
			With:      opts.With,
			PrepPlan:  prepPlanPath,
			OutDir:    outDir,
			Exported:  exported,
			ZipPath:   zipPath,
		}
		if plan.Subtitle != nil {
			result.SubtitleSrc = strings.TrimSpace(plan.Subtitle.SelectedSource)
		}
		printExportJSON(result)
		return exitOK
	}

	fmt.Printf("asset_id: %s\n", asset.AssetID)
	fmt.Printf("asset_path: %s\n", asset.OutputPath)
	fmt.Printf("to: %s\n", opts.To)
	fmt.Printf("prep_bundle: %s\n", prepDir)
	fmt.Printf("prep_plan: %s\n", prepPlanPath)
	fmt.Printf("out_dir: %s\n", outDir)
	keys := make([]string, 0, len(exported))
	for k := range exported {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%s: %s\n", k, exported[k])
	}
	if zipPath != "" {
		fmt.Printf("zip: %s\n", zipPath)
	}
	return exitOK
}

func latestPrepBundle(asset prepResolvedAsset) (dir string, prepPlanPath string, err error) {
	roots := make([]string, 0, 4)
	seen := map[string]struct{}{}
	addRoot := func(path string) {
		p := strings.TrimSpace(path)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		roots = append(roots, p)
	}

	addRoot(filepath.Join(filepath.Dir(asset.OutputPath), ".mingest", "prep", asset.AssetID))

	records, _ := readAssetRecords()
	sort.Slice(records, func(i, j int) bool {
		return parseRecordTime(records[i]).After(parseRecordTime(records[j]))
	})
	for _, r := range records {
		if strings.TrimSpace(r.AssetID) != strings.TrimSpace(asset.AssetID) {
			continue
		}
		addRoot(filepath.Join(filepath.Dir(strings.TrimSpace(r.OutputPath)), ".mingest", "prep", asset.AssetID))
	}

	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}

		dirs := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(root, e.Name()))
			}
		}
		if len(dirs) == 0 {
			continue
		}

		sort.Slice(dirs, func(i, j int) bool {
			return filepath.Base(dirs[i]) > filepath.Base(dirs[j])
		})
		dir = dirs[0]
		prepPlanPath = filepath.Join(dir, "prep-plan.json")
		if fileExists(prepPlanPath) {
			return dir, prepPlanPath, nil
		}
	}

	if len(roots) == 0 {
		return "", "", fmt.Errorf("未找到 prep 输出目录（请先执行 `mingest prep %s --goal subtitle`）", asset.AssetID)
	}
	return "", "", fmt.Errorf("未找到可用的 prep 输出目录（已检查 %d 个候选目录）。请先执行 `mingest prep %s --goal subtitle`", len(roots), asset.AssetID)
}

func readPrepPlan(path string) (prepPlan, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return prepPlan{}, err
	}
	var plan prepPlan
	if err := json.Unmarshal(b, &plan); err != nil {
		return prepPlan{}, err
	}
	return plan, nil
}

func writeExportEDL(path, assetID string, clips []prepClip, fps float64) error {
	if fps <= 0 {
		fps = 30
	}
	if fps > 120 {
		fps = 120
	}

	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("TITLE: mingest_%s\n", assetID))
	b.WriteString("FCM: NON-DROP FRAME\n\n")

	timelineSec := 0.0
	for i, clip := range clips {
		srcIn := secondsToTimecode(clip.StartSec, fps)
		srcOut := secondsToTimecode(clip.EndSec, fps)
		recIn := secondsToTimecode(timelineSec, fps)
		timelineSec += clip.DurationSec
		recOut := secondsToTimecode(timelineSec, fps)

		eventNum := fmt.Sprintf("%03d", i+1)
		b.WriteString(fmt.Sprintf("%s  AX       V     C        %s %s %s %s\n", eventNum, srcIn, srcOut, recIn, recOut))
		b.WriteString(fmt.Sprintf("* FROM CLIP NAME: %s\n", clip.Label))
		if strings.TrimSpace(clip.Reason) != "" {
			b.WriteString(fmt.Sprintf("* COMMENT: %s\n", clip.Reason))
		}
		b.WriteString("\n")
	}

	return os.WriteFile(path, b.Bytes(), 0o644)
}

func secondsToTimecode(sec float64, fps float64) string {
	if sec < 0 {
		sec = 0
	}
	totalFrames := int64(sec*fps + 0.5)
	fpsInt := int64(fps + 0.5)
	if fpsInt <= 0 {
		fpsInt = 30
	}

	frames := totalFrames % fpsInt
	totalSeconds := totalFrames / fpsInt
	s := totalSeconds % 60
	totalMinutes := totalSeconds / 60
	m := totalMinutes % 60
	h := totalMinutes / 60

	return fmt.Sprintf("%02d:%02d:%02d:%02d", h, m, s, frames)
}

func zipDir(srcDir, zipPath string) error {
	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		info, err := d.Info()
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = rel
		header.Method = zip.Deflate

		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
		return nil
	})
}

func exportExitWithErr(asJSON bool, exitCode int, msg string) int {
	if asJSON {
		printExportJSON(exportJSONResult{
			OK:       false,
			ExitCode: exitCode,
			Error:    msg,
		})
	} else {
		log.Print(msg)
	}
	return exitCode
}

func printExportJSON(v exportJSONResult) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("JSON 序列化失败: %v", err)
		return
	}
	fmt.Println(string(data))
}
