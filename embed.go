package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// 嵌入外部可执行文件
// 将外部 exe 放在 embed/ 目录下，然后在这里声明嵌入
// 注意：embed 指令必须在 var 声明之前，且只能嵌入文件，不能嵌入目录
// 
// 使用方法：
// 1. 创建 embed/ 目录：mkdir -p embed
// 2. 将要嵌入的文件放入 embed/ 目录
// 3. 取消下面对应行的注释
// 4. 在 embeddedBinaries map 中添加对应的条目
//
// 示例（取消注释以启用嵌入）：
// 注意：如果文件不存在，编译会失败。请先确保文件存在于 embed/ 目录中
//
// 步骤：
// 1. 将外部 exe 放入 embed/ 目录（如 embed/yt-dlp, embed/ffmpeg）
// 2. 取消下面对应行的注释
// 3. 编译：go build -o youtube
//
// var embeddedYtDlp []byte   // 取消注释：//go:embed embed/yt-dlp
// var embeddedFFmpeg []byte   // 取消注释：//go:embed embed/ffmpeg
// var embeddedDeno []byte     // 取消注释：//go:embed embed/deno
// var embeddedNode []byte     // 取消注释：//go:embed embed/node

// 当前已嵌入文件（已启用）
//go:embed embed/windows/yt-dlp.exe
var embeddedYtDlp []byte
//go:embed embed/windows/ffmpeg.exe
var embeddedFFmpeg []byte
//go:embed embed/windows/deno.exe
var embeddedDeno []byte

// 如果需要嵌入 node.exe，取消下面的注释：
// //go:embed embed/windows/node.exe
// var embeddedNode []byte

// 当前未嵌入 node
var embeddedNode []byte

// embeddedBinaries 存储所有嵌入的二进制文件
// 如果某个文件未嵌入（对应变量为空），程序会自动跳过，使用系统版本
var embeddedBinaries = map[string][]byte{
	"yt-dlp": embeddedYtDlp,
	"ffmpeg": embeddedFFmpeg,
	"deno":   embeddedDeno,
	"node":   embeddedNode,
}

var (
	extractOnce sync.Once
	extractDir  string
	extractErr  error
)

// extractEmbeddedBinaries 提取嵌入的二进制文件到程序同目录
// 优先提取到程序同目录，如果不可写则回退到临时目录
func extractEmbeddedBinaries() (string, error) {
	extractOnce.Do(func() {
		// 优先尝试提取到程序同目录
		exeDir, err := executableDirForEmbed()
		if err == nil {
			// 检查程序目录是否可写
			testFile := filepath.Join(exeDir, ".youtube-cli-write-test")
			if err := os.WriteFile(testFile, []byte("test"), 0644); err == nil {
				os.Remove(testFile) // 清理测试文件
				extractDir = exeDir
				// 提取到程序目录
				if err := extractToDir(exeDir); err == nil {
					return // 成功提取到程序目录
				}
				// 如果提取失败，继续尝试临时目录
			}
		}

		// 回退到临时目录
		tmpDir, err := os.MkdirTemp("", "youtube-cli-embedded-*")
		if err != nil {
			extractErr = fmt.Errorf("创建临时目录失败: %w", err)
			return
		}
		extractDir = tmpDir
		if err := extractToDir(tmpDir); err != nil {
			os.RemoveAll(tmpDir)
			extractErr = err
		}
	})

	return extractDir, extractErr
}

// extractToDir 将嵌入的文件提取到指定目录
func extractToDir(targetDir string) error {
	for name, data := range embeddedBinaries {
		// 跳过空文件（未嵌入的文件）
		if len(data) == 0 {
			continue
		}

		// 根据平台确定文件名
		binaryName := name
		if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
			binaryName = name + ".exe"
		}

		outputPath := filepath.Join(targetDir, binaryName)
		
		// 检查文件是否已存在（避免重复提取）
		if info, err := os.Stat(outputPath); err == nil && !info.IsDir() {
			// 文件已存在，跳过提取
			continue
		}

		if err := os.WriteFile(outputPath, data, 0755); err != nil {
			return fmt.Errorf("写入文件 %s 失败: %w", binaryName, err)
		}

		// Windows 不需要设置可执行权限，但其他平台需要
		if runtime.GOOS != "windows" {
			if err := os.Chmod(outputPath, 0755); err != nil {
				return fmt.Errorf("设置执行权限失败 %s: %w", binaryName, err)
			}
		}
	}
	return nil
}

// findEmbeddedBinary 查找嵌入的二进制文件
func findEmbeddedBinary(name string) (string, bool) {
	// 检查是否在嵌入列表中，且文件不为空
	var embeddedName string
	for k, data := range embeddedBinaries {
		// 跳过空文件（未嵌入的文件）
		if len(data) == 0 {
			continue
		}
		baseName := k
		if runtime.GOOS == "windows" {
			baseName = strings.TrimSuffix(strings.ToLower(k), ".exe")
		}
		if strings.EqualFold(baseName, name) {
			embeddedName = k
			break
		}
	}

	if embeddedName == "" {
		return "", false
	}

	// 提取嵌入文件
	extractDir, err := extractEmbeddedBinaries()
	if err != nil {
		return "", false
	}

	// 确定输出文件名
	binaryName := name
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		binaryName = name + ".exe"
	}

	outputPath := filepath.Join(extractDir, binaryName)
	if _, err := os.Stat(outputPath); err == nil {
		return outputPath, true
	}

	return "", false
}

// executableDirForEmbed 获取程序所在目录（用于 embed）
func executableDirForEmbed() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	// 如果是符号链接，解析真实路径
	realPath, err := filepath.EvalSymlinks(exePath)
	if err == nil {
		exePath = realPath
	}
	return filepath.Dir(exePath), nil
}

// cleanupEmbeddedBinaries 清理临时文件
// 注意：如果文件提取到程序同目录，不会自动删除（可缓存复用）
// 只有提取到临时目录时才会清理
func cleanupEmbeddedBinaries() {
	if extractDir == "" {
		return
	}
	
	// 检查是否是临时目录（包含 "youtube-cli-embedded-" 且不在程序目录）
	exeDir, _ := executableDirForEmbed()
	if exeDir != "" && extractDir == exeDir {
		// 提取到程序目录，不删除（保留文件以便下次使用）
		return
	}
	
	// 是临时目录，清理它
	if strings.Contains(extractDir, "youtube-cli-embedded-") {
		os.RemoveAll(extractDir)
	}
}
