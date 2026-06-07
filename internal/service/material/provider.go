package material

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"moneyprinterFaster/internal/config"
	"moneyprinterFaster/internal/model"
	"moneyprinterFaster/pkg/utils"
)

// Provider 素材服务接口
type Provider interface {
	Download(ctx context.Context, terms []string, aspect model.VideoAspect, duration float64, outputDir string) ([]string, error)
}

// NewProvider 根据配置创建 Material Provider
func NewProvider(cfg *config.MaterialConfig) (Provider, error) {
	switch cfg.Source {
	case "pexels":
		if len(cfg.Pexels.APIKeys) == 0 {
			return nil, fmt.Errorf("Pexels API Key 未配置")
		}
		return NewPexels(cfg.Pexels.APIKeys), nil
	case "pixabay":
		if len(cfg.Pixabay.APIKeys) == 0 {
			return nil, fmt.Errorf("Pixabay API Key 未配置")
		}
		return NewPixabay(cfg.Pixabay.APIKeys), nil
	case "aliyun_image":
		if cfg.AliyunImage.APIKey == "" {
			return nil, fmt.Errorf("阿里云百炼 API Key 未配置，请在 [material.aliyun_image] 中配置 api_key")
		}
		return NewAliyunImage(cfg.AliyunImage.APIKey, cfg.AliyunImage.Model, cfg.AliyunImage.ImageCount, cfg.AliyunImage.ClipSeconds), nil
	case "local":
		return NewLocal(cfg.LocalDir), nil
	default:
		return nil, fmt.Errorf("不支持的素材来源: %s", cfg.Source)
	}
}

// Pexels Pexels API 素材 Provider
type Pexels struct {
	apiKeys []string
	client  *http.Client
}

func NewPexels(apiKeys []string) *Pexels {
	return &Pexels{
		apiKeys: apiKeys,
		client:  &http.Client{},
	}
}

func (p *Pexels) Download(ctx context.Context, terms []string, aspect model.VideoAspect, needDuration float64, outputDir string) ([]string, error) {
	log.Printf("[Pexels] 开始下载素材: terms=%v, aspect=%s, need_duration=%.1fs, output_dir=%s",
		terms, aspect, needDuration, outputDir)

	if err := utils.EnsureDir(outputDir); err != nil {
		return nil, fmt.Errorf("创建素材目录失败: %w", err)
	}

	orientation := "portrait"
	if aspect == model.AspectLandscape {
		orientation = "landscape"
	}

	var videoPaths []string
	var accumulatedDuration float64

	for i, term := range terms {
		if accumulatedDuration >= needDuration && needDuration > 0 {
			log.Printf("[Pexels] 已累积 %.1fs 素材，满足目标 %.1fs，停止下载", accumulatedDuration, needDuration)
			break
		}
		remaining := needDuration - accumulatedDuration
		log.Printf("[Pexels] 搜索关键词 [%d/%d]: %q (orientation=%s, remaining=%.1fs)",
			i+1, len(terms), term, orientation, remaining)

		paths, duration, err := p.searchAndDownload(ctx, term, orientation, outputDir, remaining)
		if err != nil {
			log.Printf("[Pexels] 搜索 '%s' 失败: %v", term, err)
			continue
		}
		log.Printf("[Pexels] 关键词 '%s' 下载 %d 个视频 (新增 %.1fs)", term, len(paths), duration)
		videoPaths = append(videoPaths, paths...)
		accumulatedDuration += duration
	}

	log.Printf("[Pexels] 素材下载完成: 共 %d 个视频, 总时长 %.1fs (需要 %.1fs)",
		len(videoPaths), accumulatedDuration, needDuration)
	return videoPaths, nil
}

// searchAndDownload 搜索并下载视频，remaining 为还需的时长（秒），返回 (路径列表, 累计时长, error)
func (p *Pexels) searchAndDownload(ctx context.Context, query, orientation, outputDir string, remaining float64) ([]string, float64, error) {
	apiKey := p.apiKeys[rand.Intn(len(p.apiKeys))]

	searchURL := fmt.Sprintf("https://api.pexels.com/videos/search?query=%s&orientation=%s&per_page=10",
		url.QueryEscape(query), orientation)

	log.Printf("[Pexels] API 请求: %s", searchURL)

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[Pexels] API 返回 %d: %s", resp.StatusCode, string(body))
		return nil, 0, fmt.Errorf("API 返回 %d", resp.StatusCode)
	}

	var result struct {
		Videos []struct {
			ID       int `json:"id"`
			Duration int `json:"duration"`
			Files    []struct {
				Link    string `json:"link"`
				Quality string `json:"quality"`
				Width   int    `json:"width"`
				Height  int    `json:"height"`
			} `json:"video_files"`
		} `json:"videos"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("解析响应失败: %w", err)
	}

	log.Printf("[Pexels] 搜索结果 '%s': 返回 %d 个视频", query, len(result.Videos))
	for i, v := range result.Videos {
		log.Printf("[Pexels]   [%d] id=%d, duration=%ds, files=%d", i+1, v.ID, v.Duration, len(v.Files))
	}

	var paths []string
	var downloadedDuration float64
	for _, video := range result.Videos {
		// 检查是否已满足时长需求（留 10% 余量）
		if remaining > 0 && downloadedDuration >= remaining*1.1 {
			log.Printf("[Pexels]   已累积 %.1fs，超过目标 %.1fs 的 110%%，停止下载",
				downloadedDuration, remaining)
			break
		}

		// 选择 HD 质量文件
		var bestFile struct {
			Link    string
			Quality string
			Width   int
			Height  int
		}
		for _, f := range video.Files {
			if f.Quality == "hd" {
				bestFile.Link = f.Link
				bestFile.Quality = f.Quality
				bestFile.Width = f.Width
				bestFile.Height = f.Height
				break
			}
		}
		if bestFile.Link == "" && len(video.Files) > 0 {
			bestFile.Link = video.Files[0].Link
			bestFile.Quality = video.Files[0].Quality
			bestFile.Width = video.Files[0].Width
			bestFile.Height = video.Files[0].Height
		}
		if bestFile.Link == "" {
			log.Printf("[Pexels]   视频 id=%d 无可下载文件，跳过", video.ID)
			continue
		}

		log.Printf("[Pexels]   下载视频 id=%d, duration=%ds, quality=%s, resolution=%dx%d",
			video.ID, video.Duration, bestFile.Quality, bestFile.Width, bestFile.Height)

		// 下载文件
		filePath := filepath.Join(outputDir, fmt.Sprintf("pexels_%d.mp4", video.ID))
		if err := downloadFile(ctx, bestFile.Link, filePath); err != nil {
			log.Printf("[Pexels]   下载 id=%d 失败: %v", video.ID, err)
			continue
		}

		// 获取下载后的文件大小
		if info, err := os.Stat(filePath); err == nil {
			log.Printf("[Pexels]   下载完成: %s (%.1fMB)", filePath, float64(info.Size())/(1024*1024))
		}
		paths = append(paths, filePath)
		// 用 ffprobe 探测实际时长（比 API 的 duration 更准）
		actualDuration := probeDuration(filePath)
		if actualDuration > 0 {
			downloadedDuration += actualDuration
		} else {
			downloadedDuration += float64(video.Duration)
		}
		log.Printf("[Pexels]   累计时长: %.1fs / 需要 %.1fs", downloadedDuration, remaining)
	}

	return paths, downloadedDuration, nil
}

// Pixabay Pixabay API 素材 Provider
type Pixabay struct {
	apiKeys []string
	client  *http.Client
}

func NewPixabay(apiKeys []string) *Pixabay {
	return &Pixabay{
		apiKeys: apiKeys,
		client:  &http.Client{},
	}
}

func (p *Pixabay) Download(ctx context.Context, terms []string, aspect model.VideoAspect, duration float64, outputDir string) ([]string, error) {
	// TODO: 实现 Pixabay API 下载
	return nil, fmt.Errorf("Pixabay 下载尚未实现")
}

// Local 本地素材 Provider
type Local struct {
	dir string
}

func NewLocal(dir string) *Local {
	return &Local{dir: dir}
}

func (l *Local) Download(ctx context.Context, terms []string, aspect model.VideoAspect, duration float64, outputDir string) ([]string, error) {
	if l.dir == "" {
		return nil, fmt.Errorf("本地素材目录未配置")
	}

	// 扫描目录中的视频文件
	var paths []string
	extensions := []string{".mp4", ".mov", ".avi", ".mkv", ".webm"}

	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, fmt.Errorf("读取素材目录失败: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		for _, e := range extensions {
			if ext == e {
				paths = append(paths, filepath.Join(l.dir, entry.Name()))
				break
			}
		}
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("素材目录中未找到视频文件")
	}

	return paths, nil
}

// downloadFile 下载文件
func downloadFile(ctx context.Context, fileURL, outputPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", fileURL, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载返回 %d", resp.StatusCode)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// probeDuration 用 ffprobe 获取视频文件时长（秒）
func probeDuration(filePath string) float64 {
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return 0
	}

	cmd := exec.Command(ffprobePath,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	durStr := strings.TrimSpace(string(out))
	if durStr == "" || durStr == "N/A" {
		return 0
	}
	var dur float64
	fmt.Sscanf(durStr, "%f", &dur)
	return dur
}
