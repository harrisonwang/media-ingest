package ingest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"media-ingest/ingest/embedtools"
	"media-ingest/ingest/platform/console"
)

const (
	exitOK             = 0
	exitUsage          = 2
	exitAuthRequired   = 20
	exitCookieProblem  = 21
	exitRuntimeMissing = 30
	exitFFmpegMissing  = 31
	exitYtDlpMissing   = 32
	exitDownloadFailed = 40
)

type tool struct {
	Name string
	Path string
}

type deps struct {
	YtDlp       tool
	FFmpeg      tool
	FFprobe     tool
	JSRuntime   tool
	JSRuntimeID string
}

type authKind string

const (
	authKindBrowser authKind = "browser"
)

type authSource struct {
	Kind  authKind
	Value string
}

func Main(args []string) int {
	log.SetFlags(0)
	console.EnsureUTF8()
	defer embedtools.Cleanup()

	if len(args) == 1 {
		usage()
		return exitUsage
	}

	if len(args) == 2 && isHelpArg(args[1]) {
		usage()
		return exitOK
	}

	switch strings.ToLower(strings.TrimSpace(args[1])) {
	case "get":
		if len(args) != 3 {
			usage()
			return exitUsage
		}
		return runGet(args[2])
	case "auth", "login":
		if len(args) != 3 {
			usage()
			return exitUsage
		}
		switch strings.ToLower(strings.TrimSpace(args[2])) {
		case "youtube":
			return runAuth()
		default:
			usage()
			return exitUsage
		}
	default:
		usage()
		return exitUsage
	}
}

func usage() {
	fmt.Println("用法:")
	fmt.Println("  mingest get <url>")
	fmt.Println("  mingest auth youtube")
	fmt.Println()
	fmt.Println("行为:")
	fmt.Println("  - 自动检测并调用 yt-dlp / ffmpeg / ffprobe / deno|node")
	fmt.Println("  - 自动维护 cookies 缓存（优先使用；必要时从浏览器读取 cookies 刷新登录态）")
	fmt.Println("  - 若 Windows 下 Chrome cookies 读取/解密失败，可用 `mingest auth youtube`（CDP）准备工具专用登录态")
	fmt.Println()
	fmt.Println("可选环境变量:")
	fmt.Println("  - MINGEST_BROWSER=chrome|firefox|chromium|edge")
	fmt.Println("  - MINGEST_BROWSER_PROFILE=Default|Profile 1|...")
	fmt.Println("  - MINGEST_JS_RUNTIME=node|deno")
	fmt.Println("  - MINGEST_CHROME_PATH=C:\\\\Path\\\\To\\\\chrome.exe")
	fmt.Println()
	fmt.Println("退出码:")
	fmt.Println("  - 20: 需要登录（AUTH_REQUIRED）")
	fmt.Println("  - 21: cookies 读取/解密问题（COOKIE_PROBLEM）")
	fmt.Println("  - 30: JS runtime 缺失（RUNTIME_MISSING）")
	fmt.Println("  - 31: ffmpeg 缺失（FFMPEG_MISSING）")
	fmt.Println("  - 32: yt-dlp 缺失（YTDLP_MISSING）")
	fmt.Println("  - 40: 下载失败（DOWNLOAD_FAILED）")
}

func isHelpArg(v string) bool {
	switch v {
	case "-h", "--help", "help":
		return true
	default:
		return false
	}
}

func runGet(targetURL string) int {
	if err := validateURL(targetURL); err != nil {
		log.Printf("输入的 URL 无效: %v", err)
		return exitUsage
	}

	found, err := detectDeps()
	if err != nil {
		var depErr dependencyError
		if errors.As(err, &depErr) {
			log.Print(depErr.Message)
			return depErr.ExitCode
		}
		log.Printf("依赖检测失败: %v", err)
		return exitDownloadFailed
	}

	authSources := buildAuthSources()
	cookieFile, err := youtubeCookiesFilePath()
	if err != nil {
		log.Printf("无法确定 cookies 缓存路径: %v", err)
		cookieFile = ""
	} else {
		// Ensure app state dir exists so yt-dlp can dump the cookie jar.
		_ = os.MkdirAll(filepath.Dir(cookieFile), 0o700)
	}

	log.Printf("使用 yt-dlp: %s", found.YtDlp.Path)
	log.Printf("使用 ffmpeg: %s", found.FFmpeg.Path)
	log.Printf("使用 ffprobe: %s", found.FFprobe.Path)
	log.Printf("使用 JS runtime: %s (%s)", found.JSRuntimeID, found.JSRuntime.Path)
	if strings.TrimSpace(cookieFile) != "" {
		log.Printf("将使用 cookies 缓存: %s", cookieFile)
	}
	log.Print("将优先使用 cookies 缓存；必要时从浏览器读取 cookies 刷新登录态")

	return runWithAuthFallback(targetURL, found, authSources, cookieFile)
}

func validateURL(raw string) error {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("仅支持 http/https URL")
	}
	if u.Host == "" {
		return fmt.Errorf("URL 缺少主机名")
	}
	return nil
}

type dependencyError struct {
	Message  string
	ExitCode int
}

func (e dependencyError) Error() string {
	return e.Message
}

func detectDeps() (deps, error) {
	exeDir, err := executableDir()
	if err != nil {
		return deps{}, err
	}
	wd, _ := os.Getwd()

	// Prefer current working directory (where users typically place the tool bundle),
	// then the executable directory, then PATH.
	ytPath, ok := findBinary("yt-dlp", wd, exeDir)
	if !ok {
		return deps{}, dependencyError{
			Message:  "未找到 yt-dlp。请将 yt-dlp 放在程序同目录，或加入 PATH。",
			ExitCode: exitYtDlpMissing,
		}
	}

	ffmpegPath, ok := findBinary("ffmpeg", wd, exeDir)
	if !ok {
		return deps{}, dependencyError{
			Message:  "未找到 ffmpeg。请将 ffmpeg 放在程序同目录，或加入 PATH。",
			ExitCode: exitFFmpegMissing,
		}
	}

	ffprobePath, ok := findBinary("ffprobe", wd, exeDir)
	if !ok {
		return deps{}, dependencyError{
			Message:  "未找到 ffprobe。请将 ffprobe 与 ffmpeg 放在同一目录（工作目录或程序同目录），或加入 PATH。",
			ExitCode: exitFFmpegMissing,
		}
	}

	// yt-dlp expects ffmpeg/ffprobe to be discoverable together. We pass --ffmpeg-location as a directory.
	if filepath.Dir(ffmpegPath) != filepath.Dir(ffprobePath) {
		return deps{}, dependencyError{
			Message:  fmt.Sprintf("检测到 ffmpeg 与 ffprobe 不在同一目录（ffmpeg=%s, ffprobe=%s）。请将它们放在同一目录，或改用 *_bundled。", ffmpegPath, ffprobePath),
			ExitCode: exitFFmpegMissing,
		}
	}

	jsID := ""
	jsPath := ""
	requestedRuntime := strings.ToLower(strings.TrimSpace(os.Getenv("MINGEST_JS_RUNTIME")))
	switch requestedRuntime {
	case "":
		// default: prefer deno first (bundled), then node
		if denoPath, exists := findBinary("deno", wd, exeDir); exists {
			jsID = "deno"
			jsPath = denoPath
		} else if nodePath, exists := findBinary("node", wd, exeDir); exists {
			jsID = "node"
			jsPath = nodePath
		}
	case "deno", "node":
		if p, exists := findBinary(requestedRuntime, wd, exeDir); exists {
			jsID = requestedRuntime
			jsPath = p
		} else {
			return deps{}, dependencyError{
				Message:  fmt.Sprintf("未找到指定 JS runtime: %s。请将其放在程序同目录，或加入 PATH。", requestedRuntime),
				ExitCode: exitRuntimeMissing,
			}
		}
	default:
		return deps{}, dependencyError{
			Message:  fmt.Sprintf("无效的 MINGEST_JS_RUNTIME: %s（仅支持 node 或 deno）", requestedRuntime),
			ExitCode: exitRuntimeMissing,
		}
	}

	if jsID == "" || jsPath == "" {
		return deps{}, dependencyError{
			Message:  "未找到 JS runtime（deno 或 node）。请将 deno/node 放在程序同目录，或加入 PATH。",
			ExitCode: exitRuntimeMissing,
		}
	}

	return deps{
		YtDlp:       tool{Name: "yt-dlp", Path: ytPath},
		FFmpeg:      tool{Name: "ffmpeg", Path: ffmpegPath},
		FFprobe:     tool{Name: "ffprobe", Path: ffprobePath},
		JSRuntime:   tool{Name: jsID, Path: jsPath},
		JSRuntimeID: jsID,
	}, nil
}

func executableDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exePath), nil
}

func findBinary(name string, preferredDirs ...string) (string, bool) {
	// 优先查找嵌入的二进制文件
	if path, ok := embedtools.Find(name); ok {
		return path, true
	}

	candidates := []string{name}
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		candidates = append(candidates, name+".exe")
	}

	for _, c := range candidates {
		for _, dir := range preferredDirs {
			if strings.TrimSpace(dir) == "" {
				continue
			}
			local := filepath.Join(dir, c)
			if isRunnableFile(local) {
				return local, true
			}
		}
	}

	for _, c := range candidates {
		if p, ok := findInPath(c); ok {
			return p, true
		}
	}

	return "", false
}

func findBinaryPreferPath(name string, fallbackDirs ...string) (string, bool) {
	candidates := []string{name}
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		candidates = append(candidates, name+".exe")
	}

	for _, c := range candidates {
		if p, ok := findInPath(c); ok {
			return p, true
		}
	}

	for _, dir := range fallbackDirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		for _, c := range candidates {
			local := filepath.Join(dir, c)
			if isRunnableFile(local) {
				return local, true
			}
		}
	}

	return "", false
}

func findInPath(name string) (string, bool) {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" && runtime.GOOS == "windows" {
		pathEnv = os.Getenv("Path")
	}
	if pathEnv == "" {
		return "", false
	}

	for _, dir := range filepath.SplitList(pathEnv) {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		if isRunnableFile(candidate) {
			return candidate, true
		}
	}
	return "", false
}

func isRunnableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	// Windows 不依赖可执行位，仅校验存在即可。
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

func buildAuthSources() []authSource {
	if v := strings.TrimSpace(os.Getenv("MINGEST_BROWSER")); v != "" {
		lower := strings.ToLower(v)
		return []authSource{{Kind: authKindBrowser, Value: lower}}
	}

	browsers := autoBrowserOrder()
	out := make([]authSource, 0, len(browsers))
	for _, b := range browsers {
		out = append(out, authSource{Kind: authKindBrowser, Value: b})
	}
	return out
}

func autoBrowserOrder() []string {
	available := detectBrowsers()
	if len(available) == 1 {
		return available
	}

	// Multiple or unknown: default to chrome first, then others.
	pick := func(list []string, v string) []string {
		for _, x := range list {
			if x == v {
				return list
			}
		}
		return append(list, v)
	}

	out := make([]string, 0, 4)
	if contains(available, "chrome") || len(available) == 0 {
		out = pick(out, "chrome")
	}
	if contains(available, "firefox") || len(available) == 0 {
		out = pick(out, "firefox")
	}
	if contains(available, "chromium") || len(available) == 0 {
		out = pick(out, "chromium")
	}
	if contains(available, "edge") || len(available) == 0 {
		out = pick(out, "edge")
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func detectBrowsers() []string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return nil
	}

	type browserPath struct {
		Browser string
		Paths   []string
	}

	var checks []browserPath
	switch runtime.GOOS {
	case "linux":
		checks = []browserPath{
			{Browser: "chrome", Paths: []string{filepath.Join(home, ".config", "google-chrome")}},
			{Browser: "chromium", Paths: []string{filepath.Join(home, ".config", "chromium")}},
			{Browser: "edge", Paths: []string{filepath.Join(home, ".config", "microsoft-edge")}},
			{Browser: "firefox", Paths: []string{filepath.Join(home, ".mozilla", "firefox")}},
		}
	case "darwin":
		checks = []browserPath{
			{Browser: "chrome", Paths: []string{filepath.Join(home, "Library", "Application Support", "Google", "Chrome")}},
			{Browser: "chromium", Paths: []string{filepath.Join(home, "Library", "Application Support", "Chromium")}},
			{Browser: "edge", Paths: []string{filepath.Join(home, "Library", "Application Support", "Microsoft Edge")}},
			{Browser: "firefox", Paths: []string{filepath.Join(home, "Library", "Application Support", "Firefox")}},
		}
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		appData := os.Getenv("APPDATA")
		checks = []browserPath{
			{Browser: "chrome", Paths: []string{filepath.Join(localAppData, "Google", "Chrome", "User Data")}},
			{Browser: "chromium", Paths: []string{filepath.Join(localAppData, "Chromium", "User Data")}},
			{Browser: "edge", Paths: []string{filepath.Join(localAppData, "Microsoft", "Edge", "User Data")}},
			{Browser: "firefox", Paths: []string{filepath.Join(appData, "Mozilla", "Firefox")}},
		}
	default:
		return nil
	}

	var out []string
	for _, c := range checks {
		for _, p := range c.Paths {
			if dirExists(p) {
				out = append(out, c.Browser)
				break
			}
		}
	}
	return out
}

func dirExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func runWithAuthFallback(targetURL string, d deps, sources []authSource, cookieFile string) int {
	// 0) Fast path: try cached cookies first (no browser DB access).
	if strings.TrimSpace(cookieFile) != "" {
		log.Print("认证方式: cookies 缓存 (本地)")
		code := runYtDlp(d, buildYtDlpArgsWithCookiesFile(targetURL, d, cookieFile))
		// Always attempt to filter after yt-dlp touches the cookie jar.
		if fileExists(cookieFile) {
			if err := filterYouTubeCookieFile(cookieFile); err != nil {
				log.Printf("过滤 cookies 失败（将继续）：%v", err)
			}
		}
		if code == exitOK {
			return exitOK
		}
		// If it's not an auth/cookie issue, browser fallbacks won't help.
		if !shouldTryNextAuth(code) {
			return code
		}
	}

	if len(sources) == 0 {
		return exitAuthRequired
	}

	lastCode := exitDownloadFailed
	for i, src := range sources {
		log.Printf("认证方式 (%d/%d): %s", i+1, len(sources), authSourceLabel(src))
		args := []string{}
		tmpCookieFile := ""
		tmpCleanup := func() {}
		// IMPORTANT:
		// yt-dlp's --cookies FILE is both an input and an output (it "dumps cookie jar" back).
		// If we pass the persistent cache file when extracting from a browser, an unauthenticated
		// browser (e.g. Edge not logged in) can overwrite the cache and break subsequent runs.
		//
		// To prevent this, browser-based attempts use a temp cookie jar file and only promote it
		// to the persistent cache if it looks authenticated.
		if strings.TrimSpace(cookieFile) != "" && src.Kind == authKindBrowser {
			dir := filepath.Dir(cookieFile)
			p, cleanup, err := createTempCookieJarFile(dir)
			if err == nil {
				tmpCookieFile = p
				tmpCleanup = cleanup
				args = buildYtDlpArgsWithCookieCache(targetURL, d, src, tmpCookieFile)
			} else {
				// Fallback: proceed without temp jar; this loses caching but keeps functionality.
				args = buildYtDlpArgs(targetURL, d, src)
			}
		} else {
			args = buildYtDlpArgsWithCookieCache(targetURL, d, src, cookieFile)
		}
		code := runYtDlp(d, args)
		// Best-effort: if the browser attempt produced an authenticated cookie jar, update cache.
		if tmpCookieFile != "" && fileExists(tmpCookieFile) && strings.TrimSpace(cookieFile) != "" {
			if err := filterYouTubeCookieFile(tmpCookieFile); err != nil {
				log.Printf("过滤 cookies 失败（将继续）：%v", err)
			} else if ok, err := cookieFileLooksLikeLoggedIn(tmpCookieFile); err == nil && ok {
				if err := copyFileAtomic(tmpCookieFile, cookieFile); err != nil {
					log.Printf("更新 cookies 缓存失败（将继续）：%v", err)
				}
			}
		}
		tmpCleanup()
		if strings.TrimSpace(cookieFile) != "" && fileExists(cookieFile) {
			// Keep the cache minimal even if yt-dlp added extra domains.
			if err := filterYouTubeCookieFile(cookieFile); err != nil {
				log.Printf("过滤 cookies 失败（将继续）：%v", err)
			}
		}
		if code == exitOK {
			if i > 0 && strings.TrimSpace(os.Getenv("MINGEST_BROWSER")) == "" {
				log.Printf("提示: 已自动切换并使用 %s 的登录态。可设置 MINGEST_BROWSER=%s 以固定使用该浏览器。", src.Value, src.Value)
			}
			return code
		}
		// Prefer Chrome, but on Windows Chrome cookie decryption frequently fails.
		// When chrome fails, try CDP (Chrome gives us decrypted cookies) before falling back to Firefox.
		if src.Kind == authKindBrowser && src.Value == "chrome" && shouldTryNextAuth(code) {
			log.Print("Chrome cookies 失败，尝试使用 Chrome 内部登录态（CDP）...")
			cdpCode := tryDownloadWithChromeCDP(targetURL, d, cookieFile)
			if cdpCode == exitOK {
				if strings.TrimSpace(cookieFile) != "" && fileExists(cookieFile) {
					if err := filterYouTubeCookieFile(cookieFile); err != nil {
						log.Printf("过滤 cookies 失败（将继续）：%v", err)
					}
				}
				return exitOK
			}
			// If CDP cannot provide a working session, guide the user to prepare the managed profile.
			if cdpCode == exitAuthRequired {
				log.Print("提示: CDP 登录态未能通过当前视频的鉴权（可能未登录/未完成年龄确认/账号受限）。请先执行: mingest auth youtube")
				// Keep classification as AUTH_REQUIRED so callers can decide what to do.
				code = exitAuthRequired
			} else if cdpCode == exitCookieProblem {
				code = exitCookieProblem
			}
		}

		lastCode = code

		if i < len(sources)-1 && shouldTryNextAuth(code) {
			log.Printf("当前认证方式失败（退出码 %d），尝试下一种认证方式", code)
			continue
		}
		break
	}

	if shouldTryNextAuth(lastCode) {
		log.Print("未能获取有效登录态。请先在浏览器登录目标网站，然后重试。")
		log.Print("若你实际登录在 Firefox，可尝试: MINGEST_BROWSER=firefox mingest get <url>")
		log.Print("或先执行一次: mingest auth youtube")
		return exitAuthRequired
	}
	return lastCode
}

func shouldTryNextAuth(code int) bool {
	return code == exitAuthRequired || code == exitCookieProblem
}

func authSourceLabel(src authSource) string {
	switch src.Kind {
	case authKindBrowser:
		return "浏览器 cookies (" + src.Value + ")"
	}
	return "unknown"
}

func buildYtDlpArgs(targetURL string, d deps, src authSource) []string {
	args := buildYtDlpBaseArgs(d)

	switch src.Kind {
	case authKindBrowser:
		browserArg := src.Value
		if p := strings.TrimSpace(os.Getenv("MINGEST_BROWSER_PROFILE")); p != "" {
			browserArg = browserArg + ":" + p
		}
		args = append(args, "--cookies-from-browser", browserArg)
	default:
		// no auth args
	}

	args = append(args, targetURL)
	return args
}

func buildYtDlpArgsWithCookieCache(targetURL string, d deps, src authSource, cookieFile string) []string {
	args := buildYtDlpBaseArgs(d)

	switch src.Kind {
	case authKindBrowser:
		browserArg := src.Value
		if p := strings.TrimSpace(os.Getenv("MINGEST_BROWSER_PROFILE")); p != "" {
			browserArg = browserArg + ":" + p
		}
		args = append(args, "--cookies-from-browser", browserArg)
	default:
		// no auth args
	}

	if strings.TrimSpace(cookieFile) != "" {
		args = append(args, "--cookies", cookieFile)
	}

	args = append(args, targetURL)
	return args
}

func buildYtDlpArgsWithCookiesFile(targetURL string, d deps, cookieFile string) []string {
	args := buildYtDlpBaseArgs(d)
	args = append(args, "--cookies", cookieFile, targetURL)
	return args
}

func buildYtDlpBaseArgs(d deps) []string {
	ffmpegDir := filepath.Dir(d.FFmpeg.Path)
	args := []string{
		"--ffmpeg-location", ffmpegDir,
		"--js-runtime", d.JSRuntimeID,
	}
	// When yt-dlp's output is piped through our wrapper, Windows locale encodings frequently
	// cause garbled filenames in the console. Forcing UTF-8 makes output consistent.
	if runtime.GOOS == "windows" {
		args = append(args, "--encoding", "utf-8")
	}

	args = append(args,
		"--output", "%(title)s.%(ext)s",
		"--embed-thumbnail",
		"--add-metadata",
		"-f", "bestvideo[vcodec^=avc1]+bestaudio[ext=m4a]/best[ext=mp4]/best",
		"--merge-output-format", "mp4",
	)
	return args
}

func runYtDlp(d deps, args []string) int {
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		log.Printf("创建 stdout 管道失败: %v", err)
		return exitDownloadFailed
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		log.Printf("创建 stderr 管道失败: %v", err)
		return exitDownloadFailed
	}

	procArgs := append([]string{d.YtDlp.Path}, args...)
	env := withPrependedPath(os.Environ(), filepath.Dir(d.JSRuntime.Path))
	// Make yt-dlp output deterministic on Windows consoles and when piped.
	env = withEnvVar(env, "PYTHONUTF8", "1")
	env = withEnvVar(env, "PYTHONIOENCODING", "utf-8")
	proc, err := os.StartProcess(
		d.YtDlp.Path,
		procArgs,
		&os.ProcAttr{
			Env: env,
			Dir: ".",
			Files: []*os.File{
				os.Stdin,
				stdoutW,
				stderrW,
			},
		},
	)
	_ = stdoutW.Close()
	_ = stderrW.Close()

	if err != nil {
		_ = stdoutR.Close()
		_ = stderrR.Close()
		log.Printf("启动 yt-dlp 失败: %v", err)
		return exitDownloadFailed
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	go streamAndCapture(stdoutR, os.Stdout, &stdoutBuf, &wg)
	go streamAndCapture(stderrR, os.Stderr, &stderrBuf, &wg)

	state, waitErr := proc.Wait()
	wg.Wait()
	combined := stdoutBuf.String() + "\n" + stderrBuf.String()

	if waitErr != nil {
		log.Printf("等待 yt-dlp 结束失败: %v", waitErr)
		return exitDownloadFailed
	}
	if state.Success() {
		return exitOK
	}

	code, hint := classifyFailure(combined)
	if hint != "" {
		log.Println(hint)
	}
	if code == exitDownloadFailed {
		log.Printf("yt-dlp 退出码: %d", state.ExitCode())
	}

	return code
}

func streamAndCapture(r *os.File, target *os.File, buf *bytes.Buffer, wg *sync.WaitGroup) {
	defer wg.Done()
	defer r.Close()
	_, _ = io.Copy(io.MultiWriter(target, buf), r)
}

func withPrependedPath(env []string, dir string) []string {
	if strings.TrimSpace(dir) == "" {
		return env
	}
	pathKey := "PATH"
	if runtime.GOOS == "windows" {
		pathKey = "Path"
	}
	sep := string(os.PathListSeparator)

	out := make([]string, 0, len(env)+1)
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, pathKey+"=") {
			found = true
			curr := strings.TrimPrefix(kv, pathKey+"=")
			out = append(out, pathKey+"="+dir+sep+curr)
			continue
		}
		out = append(out, kv)
	}
	if !found {
		out = append(out, pathKey+"="+dir)
	}
	return out
}

func withEnvVar(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			found = true
			out = append(out, prefix+value)
			continue
		}
		out = append(out, kv)
	}
	if !found {
		out = append(out, prefix+value)
	}
	return out
}

func classifyFailure(output string) (int, string) {
	lower := strings.ToLower(output)

	if strings.Contains(lower, "could not copy") && strings.Contains(lower, "cookie database") {
		return exitCookieProblem, "浏览器 cookies 数据库无法读取（常见原因: 浏览器仍在占用 cookies 数据库）。请先彻底退出浏览器（含后台进程）后重试；或改用 Firefox；或执行 `mingest auth youtube`（使用 CDP 从浏览器进程内导出 cookies，避免读取数据库）。"
	}

	if strings.Contains(lower, "failed to decrypt with dpapi") {
		return exitCookieProblem, "浏览器 cookies 解密失败。请改用 Firefox，或执行 `mingest auth youtube`。"
	}

	// Chrome's App-Bound Cookie Encryption on Windows intentionally makes third-party decryption harder.
	// When enabled, tools that read/decrypt the cookie DB may fail even with admin rights.
	if strings.Contains(lower, "app-bound") && strings.Contains(lower, "cookie") && strings.Contains(lower, "encrypt") {
		return exitCookieProblem, "检测到 Chrome App-Bound Cookie Encryption 相关错误。此模式下第三方工具可能无法直接解密 Chrome cookies。建议改用 `mingest auth youtube`（CDP 方式）或改用 Firefox/Edge 的登录态。"
	}

	if strings.Contains(lower, "permission denied") && strings.Contains(lower, "cookies") {
		return exitCookieProblem, "读取浏览器 cookies 被拒绝。请检查浏览器进程占用与文件权限。"
	}

	if strings.Contains(lower, "cannot decrypt v11 cookies: no key found") {
		return exitCookieProblem, "浏览器 cookies 解密失败（keyring 不可用）。如果你是 SSH 会话，请在本机桌面终端运行，或改用 Firefox，或执行 `mingest auth youtube`。"
	}

	if strings.Contains(lower, "sign in to confirm you're not a bot") ||
		strings.Contains(lower, "sign in to confirm you’re not a bot") {
		return exitAuthRequired, "需要登录 YouTube。请先在浏览器登录后重试，或执行 `mingest auth youtube`。"
	}

	if strings.Contains(lower, "sign in to confirm your age") ||
		(strings.Contains(lower, "this video may be inappropriate for some users") && strings.Contains(lower, "sign in")) {
		return exitAuthRequired, "需要登录 YouTube 并完成年龄确认（Age-restricted）。请在浏览器中登录并打开该视频完成确认后重试；或执行 `mingest auth youtube` 使用工具专用登录态。"
	}

	if strings.Contains(lower, "cookies file") && strings.Contains(lower, "netscape") {
		return exitCookieProblem, "cookies 文件格式异常。"
	}

	if strings.Contains(lower, "no supported javascript runtime could be found") {
		return exitRuntimeMissing, "JS runtime 不可用。请确认 deno 或 node 可执行，并可被该程序访问。"
	}

	if strings.Contains(lower, "ffmpeg not found") {
		return exitFFmpegMissing, "ffmpeg 不可用。请将 ffmpeg/ffprobe 放在同一目录（工作目录或程序同目录），或加入 PATH，或改用 *_bundled。"
	}

	if strings.Contains(lower, "ffprobe not found") {
		return exitFFmpegMissing, "ffprobe 不可用。请将 ffmpeg/ffprobe 放在同一目录（工作目录或程序同目录），或加入 PATH，或改用 *_bundled。"
	}

	return exitDownloadFailed, "下载失败。可先执行 `yt-dlp -U` 更新，再检查 cookies 是否过期。"
}
