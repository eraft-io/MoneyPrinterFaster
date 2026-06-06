package compose

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"moneyprinterFaster/internal/model"
	"moneyprinterFaster/internal/pipeline"
	"moneyprinterFaster/pkg/utils"
)

// Composer FFmpeg 视频合成服务
type Composer struct {
	ffmpegPath         string
	ffprobePath        string
	codec              string
	hasSubtitlesFilter bool   // FFmpeg 是否支持 subtitles 滤镜（需要 libass）
	fontPath           string // 字幕字体路径
}

// NewComposer 创建合成器
func NewComposer(ffmpegPath, codec string) (*Composer, error) {
	if ffmpegPath == "" {
		var err error
		ffmpegPath, err = utils.FindFFmpeg("")
		if err != nil {
			return nil, fmt.Errorf("FFmpeg 未找到: %w", err)
		}
	}
	if codec == "" {
		codec = "libx264"
	}

	// ffprobe 路径：与 ffmpeg 同目录
	ffprobePath := filepath.Join(filepath.Dir(ffmpegPath), "ffprobe")

	// 检测字幕支持：优先 libass subtitles 滤镜，否则使用 drawtext 降级方案
	fontPath := findSubtitleFont()
	hasSubtitles := checkSubtitlesFilter(ffmpegPath)
	if hasSubtitles {
		log.Printf("[Composer] FFmpeg 支持 subtitles 滤镜（libass）")
	} else if fontPath != "" {
		log.Printf("[Composer] FFmpeg 不支持 subtitles 滤镜，将使用 drawtext 降级方案 (字体: %s)", fontPath)
	} else {
		log.Printf("[Composer] 警告: 未找到可用字体文件，字幕将无法烧录")
	}

	return &Composer{
		ffmpegPath:         ffmpegPath,
		ffprobePath:        ffprobePath,
		codec:              codec,
		hasSubtitlesFilter: hasSubtitles,
		fontPath:           fontPath,
	}, nil
}

// checkSubtitlesFilter 检测 FFmpeg 是否支持 subtitles 滤镜
func checkSubtitlesFilter(ffmpegPath string) bool {
	cmd := exec.Command(ffmpegPath, "-hide_banner", "-filters")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	// 检查输出中是否包含 "subtitles" 滤镜
	return strings.Contains(string(output), " subtitles ") ||
		strings.Contains(string(output), "\nsubtitles ")
}

// probeDuration 用 ffprobe 获取媒体文件时长（秒）
func (c *Composer) probeDuration(filePath string) float64 {
	cmd := exec.Command(c.ffprobePath,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)
	out, err := cmd.Output()
	if err != nil {
		log.Printf("[probeDuration] ffprobe 失败 (%s): %v", filePath, err)
		return 0
	}
	durStr := strings.TrimSpace(string(out))
	if durStr == "" || durStr == "N/A" {
		return 0
	}
	var dur float64
	if _, err := fmt.Sscanf(durStr, "%f", &dur); err != nil {
		return 0
	}
	return dur
}

// Compose 合成视频
func (c *Composer) Compose(ctx context.Context, opts pipeline.ComposeOpts) (pipeline.ComposeResult, error) {
	taskDir := opts.TaskDir
	if err := utils.EnsureDir(taskDir); err != nil {
		return pipeline.ComposeResult{}, fmt.Errorf("创建任务目录失败: %w", err)
	}

	log.Printf("[Composer] 开始合成: video_paths=%d, audio=%s, subtitle=%s, bgm=%s",
		len(opts.VideoPaths), opts.AudioPath, opts.SubtitlePath, opts.BGMPath)

	// 探测音频时长
	audioDuration := c.probeDuration(opts.AudioPath)
	log.Printf("[Composer] 音频时长: %.1fs (%s)", audioDuration, opts.AudioPath)

	// 探测视频时长，并只选取覆盖音频时长的片段
	var totalVideoDuration float64
	var selectedPaths []string
	for _, vp := range opts.VideoPaths {
		dur := c.probeDuration(vp)
		log.Printf("[Composer]   视频: %s (duration=%.1fs)", vp, dur)
		if totalVideoDuration < audioDuration*1.05 { // 选到刚好覆盖音频 + 5% 余量
			selectedPaths = append(selectedPaths, vp)
			totalVideoDuration += dur
		}
	}
	log.Printf("[Composer] 筛选视频: %d/%d 个, 总时长 %.1fs (音频 %.1fs)",
		len(selectedPaths), len(opts.VideoPaths), totalVideoDuration, audioDuration)

	if len(selectedPaths) == 0 {
		return pipeline.ComposeResult{}, fmt.Errorf("没有可用的视频素材")
	}

	// 输出路径
	outputFile := filepath.Join(taskDir, fmt.Sprintf("final-%d.mp4", opts.OutputIndex))

	// 第一步：拼接视频片段
	concatFile := filepath.Join(taskDir, fmt.Sprintf("combined-%d.mp4", opts.OutputIndex))
	log.Printf("[Composer] 第一步: 拼接 %d 个视频片段...", len(selectedPaths))
	if err := c.concatVideos(ctx, selectedPaths, concatFile, opts.Params); err != nil {
		return pipeline.ComposeResult{}, fmt.Errorf("视频拼接失败: %w", err)
	}
	concatDuration := c.probeDuration(concatFile)
	log.Printf("[Composer] 拼接完成: %s (duration=%.1fs)", concatFile, concatDuration)

	// 第二步：合成音频 + 字幕 + BGM
	log.Printf("[Composer] 第二步: 合成音频 + 字幕 + BGM...")
	if err := c.composeFinal(ctx, concatFile, opts, outputFile); err != nil {
		return pipeline.ComposeResult{}, fmt.Errorf("最终合成失败: %w", err)
	}

	// 清理中间文件
	os.Remove(concatFile)

	finalDuration := c.probeDuration(outputFile)
	log.Printf("[Composer] 合成完成: %s (duration=%.1fs)", outputFile, finalDuration)

	return pipeline.ComposeResult{
		OutputPath: outputFile,
		Duration:   finalDuration,
	}, nil
}

// concatVideos 拼接视频片段
// 策略：先逐个标准化每个视频（每次只处理 1 个），然后用 concat demuxer 流式拼接（内存占用极低）
func (c *Composer) concatVideos(ctx context.Context, inputs []string, output string, params model.VideoParams) error {
	if len(inputs) == 0 {
		return fmt.Errorf("无视频输入")
	}

	width := params.Width()
	height := params.Height()
	tempDir := filepath.Dir(output)

	// 第一步：逐个标准化每个视频（每次只打开 1 个输入文件，内存占用极低）
	var normalizedFiles []string
	for i, input := range inputs {
		normFile := filepath.Join(tempDir, fmt.Sprintf("norm_%d.ts", i))
		log.Printf("[FFmpeg] 标准化视频 [%d/%d]: %s → %s (%dx%d)",
			i+1, len(inputs), filepath.Base(input), filepath.Base(normFile), width, height)

		if err := c.normalizeVideo(ctx, input, normFile, width, height); err != nil {
			// 清理已生成的临时文件
			for _, f := range normalizedFiles {
				os.Remove(f)
			}
			return fmt.Errorf("标准化视频 [%d] 失败: %w", i+1, err)
		}
		normalizedFiles = append(normalizedFiles, normFile)
	}

	// 第二步：用 concat demuxer 拼接（只读取文件列表，不同时打开所有文件）
	log.Printf("[FFmpeg] 用 concat demuxer 拼接 %d 个标准化视频...", len(normalizedFiles))
	err := c.concatDemuxer(ctx, normalizedFiles, output)

	// 清理临时文件
	for _, f := range normalizedFiles {
		os.Remove(f)
	}

	return err
}

// normalizeVideo 标准化单个视频（统一分辨率、帧率、编码格式）
func (c *Composer) normalizeVideo(ctx context.Context, input, output string, width, height int) error {
	codec := c.codec
	args := []string{
		"-y", "-i", input,
		"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=30",
			width, height, width, height),
		"-c:v", codec,
		"-preset", "ultrafast", // 标准化用 ultrafast 优先速度
		"-crf", "23",
		"-r", "30",
		"-an",          // 不需要音频
		"-f", "mpegts", // 输出 MPEG-TS 格式（concat demuxer 需要统一格式）
		output,
	}

	cmd := exec.CommandContext(ctx, c.ffmpegPath, args...)
	cmdOutput, err := cmd.CombinedOutput()
	if err != nil {
		// 回退到 libx264
		if codec != "libx264" {
			for i, arg := range args {
				if arg == codec {
					args[i] = "libx264"
				}
			}
			cmd = exec.CommandContext(ctx, c.ffmpegPath, args...)
			cmdOutput, err = cmd.CombinedOutput()
		}
		if err != nil {
			return fmt.Errorf("标准化失败: %w, output: %s", err, string(cmdOutput))
		}
	}
	return nil
}

// concatDemuxer 用 concat demuxer 流式拼接（内存占用极低）
func (c *Composer) concatDemuxer(ctx context.Context, inputs []string, output string) error {
	// 创建 concat 列表文件
	listFile := output + ".concat.txt"
	defer os.Remove(listFile)

	var lines []string
	for _, f := range inputs {
		// concat demuxer 以 txt 文件所在目录为基准路径，必须使用绝对路径避免路径重复
		absPath, err := filepath.Abs(f)
		if err != nil {
			return fmt.Errorf("获取绝对路径失败: %w", err)
		}
		// concat demuxer 需要单引号转义路径中的特殊字符
		escaped := strings.ReplaceAll(absPath, "'", "'\\''")
		lines = append(lines, fmt.Sprintf("file '%s'", escaped))
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(listFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("创建 concat 列表失败: %w", err)
	}

	log.Printf("[FFmpeg] concat 列表: %s (%d 个文件)", listFile, len(inputs))

	args := []string{
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listFile,
		"-c", "copy", // 直接复制流，不重新编码
		output,
	}

	cmd := exec.CommandContext(ctx, c.ffmpegPath, args...)
	cmdOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[FFmpeg] concat demuxer 失败: %v\nOutput: %s", err, string(cmdOutput))
		return fmt.Errorf("concat demuxer 失败: %w, output: %s", err, string(cmdOutput))
	}

	log.Printf("[FFmpeg] concat demuxer 完成: %s", output)
	return nil
}

// composeFinal 最终合成（视频 + 音频 + 字幕 + BGM）
func (c *Composer) composeFinal(ctx context.Context, videoFile string, opts pipeline.ComposeOpts, output string) error {
	log.Printf("[FFmpeg] 最终合成: video=%s, audio=%s, subtitle=%s, bgm=%s (enabled=%v)",
		videoFile, opts.AudioPath, opts.SubtitlePath, opts.BGMPath, opts.Params.BGMEnabled)

	args := []string{"-y", "-i", videoFile, "-i", opts.AudioPath}

	filterComplex := ""
	audioMap := "1:a"

	// 如果有 BGM，混合音频
	if opts.BGMPath != "" && opts.Params.BGMEnabled {
		args = append(args, "-i", opts.BGMPath)
		volume := opts.Params.BGMVolume
		if volume <= 0 {
			volume = 0.2
		}
		filterComplex = fmt.Sprintf("[1:a]volume=1.0[voice];[2:a]volume=%.2f[bgm];[voice][bgm]amix=inputs=2:duration=first[aout]", volume)
		audioMap = "[aout]"
	}

	// 字幕烧录
	hasSubtitle := false
	if opts.SubtitlePath != "" && utils.FileExists(opts.SubtitlePath) {
		if c.hasSubtitlesFilter {
			// 方案一：subtitles 滤镜（需要 libass）
			hasSubtitle = true
			args = append(args, "-vf", fmt.Sprintf("subtitles='%s'", opts.SubtitlePath))
		} else if c.fontPath != "" {
			// 方案二：drawtext 降级方案（不需要 libass）
			drawtextFilter, tempSubFiles := c.buildDrawtextFilter(opts.SubtitlePath, opts.Params.Width(), opts.Params.Height())
			if drawtextFilter != "" {
				hasSubtitle = true
				args = append(args, "-vf", drawtextFilter)
				log.Printf("[Composer] 使用 drawtext 烧录字幕")
				// FFmpeg 完成后清理字幕临时文件
				defer func() {
					for _, f := range tempSubFiles {
						os.Remove(f)
					}
				}()
			}
		} else {
			log.Printf("[Composer] 无可用字幕方案，跳过字幕烧录")
		}
	}

	if filterComplex != "" {
		args = append(args, "-filter_complex", filterComplex)
	}

	args = append(args, "-map", "0:v", "-map", audioMap)

	// 有字幕时必须重新编码视频（不能 copy），无字幕时可以直接 copy
	if hasSubtitle {
		args = append(args, "-c:v", c.codec, "-preset", "fast", "-crf", "23")
	} else {
		args = append(args, "-c:v", "copy")
	}

	args = append(args,
		"-c:a", "aac",
		"-b:a", "192k",
		"-shortest",
		output,
	)

	log.Printf("[FFmpeg] 最终合成命令: %s %s", c.ffmpegPath, strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, c.ffmpegPath, args...)
	cmdOutput, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[FFmpeg] 最终合成失败: %v\nOutput: %s", err, string(cmdOutput))
		return fmt.Errorf("FFmpeg 合成失败: %w, output: %s", err, string(cmdOutput))
	}

	// 获取输出文件大小
	if info, err := os.Stat(output); err == nil {
		log.Printf("[FFmpeg] 最终合成完成: %s (%.1fMB)", output, float64(info.Size())/(1024*1024))
	}

	return nil
}

// srtEntry SRT 字幕条目
type srtEntry struct {
	Start float64 // 开始时间（秒）
	End   float64 // 结束时间（秒）
	Text  string  // 字幕文本
}

// parseSRT 解析 SRT 字幕文件
func parseSRT(filePath string) ([]srtEntry, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	content := strings.TrimSpace(string(data))
	// 移除 BOM
	content = strings.TrimPrefix(content, "\xef\xbb\xbf")

	// 统一换行符
	content = strings.ReplaceAll(content, "\r\n", "\n")
	blocks := strings.Split(content, "\n\n")

	var entries []srtEntry
	for _, block := range blocks {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) < 3 {
			continue
		}

		// 第一行是序号，第二行是时间轴，第三行及之后是文本
		timeLine := strings.TrimSpace(lines[1])
		parts := strings.Split(timeLine, "-->")
		if len(parts) != 2 {
			continue
		}

		start := parseSRTTime(strings.TrimSpace(parts[0]))
		end := parseSRTTime(strings.TrimSpace(parts[1]))
		text := strings.TrimSpace(strings.Join(lines[2:], " "))

		if text != "" && end > start {
			entries = append(entries, srtEntry{Start: start, End: end, Text: text})
		}
	}

	return entries, nil
}

// parseSRTTime 解析 SRT 时间格式 "HH:MM:SS,mmm" 或 "HH:MM:SS.mmm"
func parseSRTTime(s string) float64 {
	s = strings.Replace(s, ",", ".", 1)
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0
	}

	var h, m int
	var sec float64
	fmt.Sscanf(parts[0], "%d", &h)
	fmt.Sscanf(parts[1], "%d", &m)
	fmt.Sscanf(parts[2], "%f", &sec)

	return float64(h)*3600 + float64(m)*60 + sec
}

// buildDrawtextFilter 根据 SRT 文件生成 drawtext 滤镜链
// 每条字幕的文本写入临时文件，用 textfile 引用，避免 FFmpeg 滤镜语法转义问题
// 返回 (滤镜链, 临时文件列表)
func (c *Composer) buildDrawtextFilter(srtPath string, width, height int) (string, []string) {
	entries, err := parseSRT(srtPath)
	if err != nil {
		log.Printf("[Composer] 解析 SRT 失败: %v", err)
		return "", nil
	}
	if len(entries) == 0 {
		return "", nil
	}

	log.Printf("[Composer] 解析到 %d 条字幕 (视频: %dx%d)", len(entries), width, height)

	// 字体大小：视频宽度的 4.5%（1080 → 48px）
	fontSize := int(float64(width) * 0.045)
	if fontSize < 28 {
		fontSize = 28
	}
	if fontSize > 60 {
		fontSize = 60
	}

	// 每行最大字符数（保守估计：中文字符宽度 ≈ 字体大小）
	// 留 10% 安全边距，每侧 5%
	safeWidth := int(float64(width) * 0.9)
	maxCharsPerLine := safeWidth / fontSize
	if maxCharsPerLine < 6 {
		maxCharsPerLine = 6
	}
	if maxCharsPerLine > 20 {
		maxCharsPerLine = 20
	}

	log.Printf("[Composer] 字幕参数: fontSize=%d, maxCharsPerLine=%d", fontSize, maxCharsPerLine)

	// 字体路径需要转义冒号
	escapedFont := strings.ReplaceAll(c.fontPath, ":", "\\\\:")

	// 垂直位置：距底部 8% 的视频高度
	bottomMargin := int(float64(height) * 0.08)

	// 临时文件目录
	tempDir := filepath.Dir(srtPath)

	var filters []string
	var tempFiles []string

	for i, entry := range entries {
		// 先对原始文本换行（用 rune 计算，不受转义影响）
		wrappedLines := wrapLines(entry.Text, maxCharsPerLine)
		lineCount := len(wrappedLines)

		// 将换行后的文本写入临时文件
		textFile := filepath.Join(tempDir, fmt.Sprintf("sub_%d.txt", i))
		textContent := strings.Join(wrappedLines, "\n")
		if err := os.WriteFile(textFile, []byte(textContent), 0644); err != nil {
			log.Printf("[Composer] 写入字幕文本文件失败: %v", err)
			continue
		}
		tempFiles = append(tempFiles, textFile)

		log.Printf("[Composer] 字幕[%d] %d行: %s", i+1, lineCount, textContent)

		// 垂直位置考虑多行高度（每行约 1.4em）
		lineHeight := float64(fontSize) * 1.4
		textBlockHeight := lineHeight * float64(lineCount)
		yPos := fmt.Sprintf("h-%.0f", textBlockHeight+float64(bottomMargin))

		// 转义 textfile 路径中的冒号
		escapedTextFile := strings.ReplaceAll(textFile, ":", "\\:")

		filter := fmt.Sprintf(
			"drawtext=fontfile='%s':textfile='%s':fontsize=%d:fontcolor=white"+
				":borderw=3:bordercolor=black@0.8"+
				":x=(w-text_w)/2:y=%s"+
				":enable='between(t,%.3f,%.3f)'",
			escapedFont, escapedTextFile, fontSize,
			yPos,
			entry.Start, entry.End,
		)
		filters = append(filters, filter)
	}

	return strings.Join(filters, ","), tempFiles
}

// wrapLines 按指定行宽对中文字符串进行换行，返回行列表
func wrapLines(text string, maxChars int) []string {
	var result []string
	runes := []rune(text)
	for len(runes) > maxChars {
		result = append(result, string(runes[:maxChars]))
		runes = runes[maxChars:]
	}
	if len(runes) > 0 {
		result = append(result, string(runes))
	}
	return result
}

// escapeDrawtextString 转义 drawtext 滤镜中的特殊字符（保留作为备用）
func escapeDrawtextString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "'\"'\"'")
	s = strings.ReplaceAll(s, ":", "\\:")
	s = strings.ReplaceAll(s, "%", "%%")
	return s
}

// findSubtitleFont 查找系统可用的中文字体
func findSubtitleFont() string {
	candidates := []string{
		// macOS 常见中文字体
		"/System/Library/Fonts/PingFang.ttc",
		"/System/Library/Fonts/STHeiti Light.ttc",
		"/System/Library/Fonts/STHeiti Medium.ttc",
		"/Library/Fonts/Arial Unicode.ttf",
		"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
		// Linux 常见路径
		"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/truetype/droid/DroidSansFallbackFull.ttf",
		"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}
