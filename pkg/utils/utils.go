package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// EnsureDir 确保目录存在，不存在则创建
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// TaskDir 返回任务的工作目录
func TaskDir(baseDir, taskID string) string {
	return filepath.Join(baseDir, "tasks", taskID)
}

// EnsureTaskDir 确保任务目录存在
func EnsureTaskDir(baseDir, taskID string) (string, error) {
	dir := TaskDir(baseDir, taskID)
	return dir, EnsureDir(dir)
}

// FileExists 判断文件是否存在
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ToJSON 将对象序列化为 JSON 字符串
func ToJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%+v", v)
	}
	return string(b)
}

// FormatDuration 格式化时间为可读字符串
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return "< 1s"
	}
	d = d.Round(time.Second)
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// FindFFmpeg 查找 FFmpeg 二进制路径
func FindFFmpeg(configPath string) (string, error) {
	// 1. 优先使用配置指定的路径
	if configPath != "" {
		if FileExists(configPath) {
			return configPath, nil
		}
		return "", fmt.Errorf("configured ffmpeg not found: %s", configPath)
	}

	// 2. 尝试 PATH 中查找
	path, err := exec.LookPath("ffmpeg")
	if err == nil {
		return path, nil
	}

	// 3. macOS 常见路径
	if runtime.GOOS == "darwin" {
		for _, p := range []string{
			"/opt/homebrew/bin/ffmpeg",
			"/usr/local/bin/ffmpeg",
		} {
			if FileExists(p) {
				return p, nil
			}
		}
	}

	// 4. Windows 常见路径
	if runtime.GOOS == "windows" {
		for _, p := range []string{
			`C:\ffmpeg\bin\ffmpeg.exe`,
			`C:\Program Files\ffmpeg\bin\ffmpeg.exe`,
		} {
			if FileExists(p) {
				return p, nil
			}
		}
	}

	return "", fmt.Errorf("ffmpeg not found in PATH or common locations")
}

// SanitizeFilename 清理文件名，去除非法字符
func SanitizeFilename(name string) string {
	// 替换常见非法字符
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_", "|", "_",
	)
	return replacer.Replace(name)
}

// SplitStringByPunctuations 按标点符号分割字符串
func SplitStringByPunctuations(s string) []string {
	// 简单实现：按常见标点分割
	var result []string
	var current strings.Builder

	for _, r := range s {
		current.WriteRune(r)
		if isPunctuation(r) {
			text := strings.TrimSpace(current.String())
			if text != "" {
				result = append(result, text)
			}
			current.Reset()
		}
	}
	// 处理末尾无标点的部分
	text := strings.TrimSpace(current.String())
	if text != "" {
		result = append(result, text)
	}
	return result
}

func isPunctuation(r rune) bool {
	switch r {
	case '.', '。', '!', '！', '?', '？', ';', '；',
		',', '，', ':', '：', '\n':
		return true
	}
	return false
}
