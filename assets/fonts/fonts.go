package fonts

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed NotoSansSC-Regular.ttf
var notoSansSC []byte

// ExtractFont 将内嵌字体提取到磁盘，返回字体文件路径
// 优先使用缓存目录，避免重复写入
func ExtractFont() (string, error) {
	// 优先写入 XDG cache，降级到 /tmp
	cacheDir := os.Getenv("XDG_CACHE_HOME")
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			cacheDir = filepath.Join(home, ".cache")
		} else {
			cacheDir = os.TempDir()
		}
	}

	fontDir := filepath.Join(cacheDir, "moneyprinter", "fonts")
	if err := os.MkdirAll(fontDir, 0755); err != nil {
		return "", fmt.Errorf("创建字体缓存目录失败: %w", err)
	}

	fontPath := filepath.Join(fontDir, "NotoSansSC-Regular.ttf")

	// 如果文件已存在且大小正确，直接返回
	if info, err := os.Stat(fontPath); err == nil && info.Size() == int64(len(notoSansSC)) {
		return fontPath, nil
	}

	// 写入字体文件
	if err := os.WriteFile(fontPath, notoSansSC, 0644); err != nil {
		return "", fmt.Errorf("写入字体文件失败: %w", err)
	}

	return fontPath, nil
}

// FontBytes 返回内嵌字体的字节数据（供其他模块使用）
func FontBytes() []byte {
	return notoSansSC
}
