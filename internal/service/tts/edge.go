package tts

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// EdgeTTS 通过 Python edge-tts CLI 合成语音
type EdgeTTS struct {
	once       sync.Once
	binPath    string   // 缓存找到的可执行路径
	prefixArgs []string // 缓存前缀参数（如 -m edge_tts）
	initErr    error    // 缓存初始化错误
}

func NewEdgeTTS() *EdgeTTS {
	return &EdgeTTS{}
}

// findBinary 查找 edge-tts 可执行文件，找不到则尝试 pip 安装
func (e *EdgeTTS) findBinary() (string, []string, error) {
	// 1. 直接找 edge-tts CLI
	if path, err := exec.LookPath("edge-tts"); err == nil {
		return path, nil, nil
	}

	// 2. 尝试 python3/python + edge_tts 模块
	for _, py := range []string{"python3", "python"} {
		if path, err := exec.LookPath(py); err == nil {
			cmd := exec.Command(path, "-c", "import edge_tts")
			if err := cmd.Run(); err == nil {
				return path, []string{"-m", "edge_tts"}, nil
			}
		}
	}

	// 3. 都找不到，尝试用 pip 自动安装
	log.Printf("[EdgeTTS] edge_tts 未安装，尝试通过 pip 安装...")

	// 找 pip
	var pipCmd string
	for _, p := range []string{"pip3", "pip"} {
		if path, err := exec.LookPath(p); err == nil {
			pipCmd = path
			break
		}
	}

	// 或者用 python -m pip
	if pipCmd == "" {
		for _, py := range []string{"python3", "python"} {
			if path, err := exec.LookPath(py); err == nil {
				pipCmd = path + " -m pip"
				break
			}
		}
	}

	if pipCmd == "" {
		return "", nil, fmt.Errorf("未找到 edge-tts 且无法通过 pip 安装（未找到 pip/python）")
	}

	// 执行安装
	parts := strings.Fields(pipCmd)
	installArgs := append(parts[1:], "install", "edge-tts")
	cmd := exec.Command(parts[0], installArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", nil, fmt.Errorf("pip install edge-tts 失败: %w, output: %s", err, string(out))
	}
	log.Printf("[EdgeTTS] pip install 成功: %s", string(out))

	// 安装后重新查找
	if path, err := exec.LookPath("edge-tts"); err == nil {
		return path, nil, nil
	}
	for _, py := range []string{"python3", "python"} {
		if path, err := exec.LookPath(py); err == nil {
			return path, []string{"-m", "edge_tts"}, nil
		}
	}

	return "", nil, fmt.Errorf("pip install 成功但未找到 edge-tts 可执行文件")
}

// ensureBinary 确保 edge-tts 可用（只初始化一次）
func (e *EdgeTTS) ensureBinary() (string, []string, error) {
	e.once.Do(func() {
		e.binPath, e.prefixArgs, e.initErr = e.findBinary()
		if e.initErr == nil {
			log.Printf("[EdgeTTS] 使用: %s %v", e.binPath, e.prefixArgs)
		}
	})
	return e.binPath, e.prefixArgs, e.initErr
}

func (e *EdgeTTS) Synthesize(ctx context.Context, text, voiceName string, rate float64, outputPath string) (any, float64, error) {
	binary, prefixArgs, err := e.ensureBinary()
	if err != nil {
		return nil, 0, fmt.Errorf("edge-tts 初始化失败: %w", err)
	}

	// 构建速率参数（edge-tts 使用 +50% / -20% 格式）
	rateStr := "+0%"
	if rate > 0 {
		rateStr = fmt.Sprintf("+%.0f%%", rate)
	} else if rate < 0 {
		rateStr = fmt.Sprintf("%.0f%%", rate)
	}

	if voiceName == "" {
		voiceName = "zh-CN-XiaoyiNeural"
	}

	args := make([]string, len(prefixArgs))
	copy(args, prefixArgs)
	args = append(args,
		"--voice", voiceName,
		"--rate", rateStr,
		"--text", text,
		"--write-media", outputPath,
		"--write-subtitles", outputPath+".vtt",
	)

	log.Printf("[EdgeTTS] 执行: %s %v", binary, args)
	cmd := exec.CommandContext(ctx, binary, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, 0, fmt.Errorf("edge-tts 执行失败: %w, output: %s", err, string(output))
	}

	log.Printf("[EdgeTTS] 合成完成: %s (voice=%s, rate=%s)", outputPath, voiceName, rateStr)

	// 用 ffprobe 探测实际音频时长
	duration := e.probeDuration(outputPath)
	log.Printf("[EdgeTTS] 音频时长: %.1fs (%s)", duration, outputPath)

	return nil, duration, nil
}

// probeDuration 用 ffprobe 获取音频文件时长（秒）
func (e *EdgeTTS) probeDuration(filePath string) float64 {
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		// 尝试与 ffmpeg/edge-tts 同目录
		if e.binPath != "" {
			ffprobePath = filepath.Join(filepath.Dir(e.binPath), "ffprobe")
		}
		if ffprobePath == "" {
			log.Printf("[EdgeTTS] ffprobe 未找到，无法探测音频时长")
			return 0
		}
	}

	cmd := exec.Command(ffprobePath,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)
	out, err := cmd.Output()
	if err != nil {
		log.Printf("[EdgeTTS] ffprobe 失败 (%s): %v", filePath, err)
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
