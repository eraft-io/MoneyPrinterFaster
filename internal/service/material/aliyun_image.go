package material

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"moneyprinterFaster/internal/model"
	"moneyprinterFaster/pkg/utils"
)

// AliyunImage 阿里云百炼 Z-Image 文生图素材 Provider
type AliyunImage struct {
	apiKey      string
	model       string
	imageCount  int // 每个关键词生成的图片数量
	clipSeconds int // 每张图片展示时长（秒）
	client      *http.Client
}

func NewAliyunImage(apiKey, model string, imageCount, clipSeconds int) *AliyunImage {
	if model == "" {
		model = "z-image-turbo"
	}
	if imageCount <= 0 {
		imageCount = 1
	}
	if clipSeconds <= 0 {
		clipSeconds = 5
	}
	return &AliyunImage{
		apiKey:      apiKey,
		model:       model,
		imageCount:  imageCount,
		clipSeconds: clipSeconds,
		client: &http.Client{
			Timeout: 120 * time.Second, // 文生图 API 可能需要较长时间
		},
	}
}

// ClipSeconds 返回每张图片的展示时长（秒）
func (a *AliyunImage) ClipSeconds() int {
	return a.clipSeconds
}

func (a *AliyunImage) Download(ctx context.Context, terms []string, aspect model.VideoAspect, duration float64, outputDir string) ([]string, error) {
	log.Printf("[AliyunImage] 开始生成图片: terms=%v, aspect=%s, duration=%.1fs, image_count=%d, clip_seconds=%d",
		terms, aspect, duration, a.imageCount, a.clipSeconds)

	if err := utils.EnsureDir(outputDir); err != nil {
		return nil, fmt.Errorf("创建素材目录失败: %w", err)
	}

	// 计算需要的图片总数（覆盖音频时长）
	totalClipsNeeded := int(duration/float64(a.clipSeconds)) + 1
	clipsPerTerm := a.imageCount
	if clipsPerTerm <= 0 {
		clipsPerTerm = 1
	}

	// 如果图片总数不够覆盖时长，自动增加每个 term 的图片数
	totalImages := len(terms) * clipsPerTerm
	if totalImages < totalClipsNeeded {
		clipsPerTerm = (totalClipsNeeded + len(terms) - 1) / len(terms)
		if clipsPerTerm < 1 {
			clipsPerTerm = 1
		}
		log.Printf("[AliyunImage] 调整每个关键词图片数: %d -> %d (需要覆盖 %.1fs)", a.imageCount, clipsPerTerm, duration)
	}

	log.Printf("[AliyunImage] 计划生成: %d 个关键词 × %d 张 = %d 张图片 (需要 %d 张覆盖 %.1fs)",
		len(terms), clipsPerTerm, len(terms)*clipsPerTerm, totalClipsNeeded, duration)

	var imagePaths []string

	for i, term := range terms {
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			return imagePaths, ctx.Err()
		default:
		}

		log.Printf("[AliyunImage] 生成关键词 [%d/%d]: %q (%d 张)", i+1, len(terms), term, clipsPerTerm)

		for j := 0; j < clipsPerTerm; j++ {
			select {
			case <-ctx.Done():
				return imagePaths, ctx.Err()
			default:
			}

			imagePath := filepath.Join(outputDir, fmt.Sprintf("aliyun_%d_%d.png", i, j))

			// 如果图片已存在，跳过
			if _, err := os.Stat(imagePath); err == nil {
				log.Printf("[AliyunImage] 图片已存在，跳过: %s", imagePath)
				imagePaths = append(imagePaths, imagePath)
				continue
			}

			if err := a.generateImage(ctx, term, aspect, imagePath); err != nil {
				log.Printf("[AliyunImage] 生成图片失败 [term=%q, idx=%d]: %v", term, j, err)
				continue
			}

			log.Printf("[AliyunImage] 图片生成成功: %s", imagePath)
			imagePaths = append(imagePaths, imagePath)
		}
	}

	if len(imagePaths) == 0 {
		return nil, fmt.Errorf("未生成任何图片")
	}

	log.Printf("[AliyunImage] 图片生成完成: 共 %d 张图片 (预计视频时长: %.1fs)",
		len(imagePaths), float64(len(imagePaths)*a.clipSeconds))

	return imagePaths, nil
}

// generateImage 调用 Z-Image API 生成单张图片
func (a *AliyunImage) generateImage(ctx context.Context, prompt string, aspect model.VideoAspect, outputPath string) error {
	// 根据视频尺寸选择最接近的图片分辨率
	size := a.selectImageSize(aspect)
	log.Printf("[AliyunImage] 调用 Z-Image API: prompt=%q, size=%s", prompt, size)

	// 构建请求体
	reqBody := map[string]any{
		"model": a.model,
		"input": map[string]any{
			"messages": []map[string]any{
				{
					"role": "user",
					"content": []map[string]string{
						{"text": prompt},
					},
				},
			},
		},
		"parameters": map[string]any{
			"prompt_extend": false,
			"size":          size,
		},
	}

	reqData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %w", err)
	}

	// 创建 HTTP 请求
	apiURL := "https://dashscope.aliyuncs.com/api/v1/services/aigc/multimodal-generation/generation"
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(reqData))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	// 发送请求
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[AliyunImage] API 返回 %d: %s", resp.StatusCode, string(respData))
		return fmt.Errorf("API 返回 %d", resp.StatusCode)
	}

	// 解析响应
	var result struct {
		Output struct {
			Choices []struct {
				Message struct {
					Content []struct {
						Image string `json:"image"`
						Text  string `json:"text"`
					} `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		} `json:"output"`
		Usage struct {
			ImageCount int `json:"image_count"`
		} `json:"usage"`
		RequestID string `json:"request_id"`
	}

	if err := json.Unmarshal(respData, &result); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	// 提取图片 URL
	var imageURL string
	for _, choice := range result.Output.Choices {
		for _, content := range choice.Message.Content {
			if content.Image != "" {
				imageURL = content.Image
				break
			}
		}
		if imageURL != "" {
			break
		}
	}

	if imageURL == "" {
		return fmt.Errorf("响应中未找到图片 URL")
	}

	log.Printf("[AliyunImage] 图片 URL: %s", imageURL)

	// 下载图片
	if err := a.downloadImage(ctx, imageURL, outputPath); err != nil {
		return fmt.Errorf("下载图片失败: %w", err)
	}

	// 检查文件大小
	if info, err := os.Stat(outputPath); err == nil {
		log.Printf("[AliyunImage] 图片下载完成: %s (%.1fKB)", outputPath, float64(info.Size())/1024)
	}

	return nil
}

// downloadImage 下载图片文件
func (a *AliyunImage) downloadImage(ctx context.Context, imageURL, outputPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return err
	}

	resp, err := a.client.Do(req)
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

// selectImageSize 根据视频宽高比选择最合适的图片分辨率
func (a *AliyunImage) selectImageSize(aspect model.VideoAspect) string {
	// Z-Image 推荐分辨率（总像素约 1280*1280）
	switch aspect {
	case model.AspectPortrait: // 9:16 竖屏
		return "864*1536"
	case model.AspectLandscape: // 16:9 横屏
		return "1536*864"
	default:
		return "1024*1024" // 默认正方形
	}
}
