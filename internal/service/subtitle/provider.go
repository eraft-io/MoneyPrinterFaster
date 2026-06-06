package subtitle

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"moneyprinterFaster/internal/config"
)

// Provider 字幕服务接口
type Provider interface {
	Generate(ctx context.Context, audioPath string, subMaker any, script string, outputPath string) error
}

// NewProvider 根据配置创建 Subtitle Provider
func NewProvider(cfg *config.SubtitleConfig) Provider {
	switch cfg.Provider {
	case "edge":
		return &EdgeSubtitle{}
	case "whisper":
		return &WhisperSubtitle{}
	default:
		return nil // 不生成字幕
	}
}

// EdgeSubtitle 使用 Edge TTS 生成的 VTT 文件转换为 SRT 字幕
type EdgeSubtitle struct{}

func (e *EdgeSubtitle) Generate(ctx context.Context, audioPath string, subMaker any, script string, outputPath string) error {
	// Edge TTS 在合成语音时会同时输出 VTT 文件：audioPath + ".vtt"
	vttPath := audioPath + ".vtt"

	// 检查 VTT 文件是否存在
	if _, err := os.Stat(vttPath); os.IsNotExist(err) {
		return fmt.Errorf("VTT 字幕文件不存在: %s", vttPath)
	}

	// 读取 VTT 文件
	vttData, err := os.ReadFile(vttPath)
	if err != nil {
		return fmt.Errorf("读取 VTT 文件失败: %w", err)
	}

	// 转换为 SRT 格式并写入
	// Edge TTS 的 VTT 实际上已经是 SRT 兼容格式（用逗号分隔毫秒，--> 分隔时间）
	// 只需去掉可能的 WEBVTT 头部
	content := string(vttData)
	content = strings.ReplaceAll(content, "WEBVTT\n", "")
	content = strings.ReplaceAll(content, "WEBVTT\r\n", "")
	// 去掉 VTT 的 NOTE 和样式块
	content = strings.TrimSpace(content)

	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入 SRT 文件失败: %w", err)
	}

	// 统计字幕条数
	count := countSubtitleBlocks(outputPath)
	log.Printf("[EdgeSubtitle] 字幕生成完成: %s → %s (%d 条)", vttPath, outputPath, count)
	return nil
}

// countSubtitleBlocks 统计 SRT 文件中的字幕条数
func countSubtitleBlocks(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "-->") {
			count++
		}
	}
	return count
}

// WhisperSubtitle 使用 faster-whisper 转写生成字幕
type WhisperSubtitle struct{}

func (w *WhisperSubtitle) Generate(ctx context.Context, audioPath string, subMaker any, script string, outputPath string) error {
	// TODO: 调用 faster-whisper CLI 生成 SRT 字幕
	_ = ctx
	_ = audioPath
	_ = subMaker
	_ = script
	_ = outputPath
	return fmt.Errorf("whisper subtitle 尚未实现")
}
