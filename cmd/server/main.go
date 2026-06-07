package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"moneyprinterFaster/internal/api"
	"moneyprinterFaster/internal/config"
	"moneyprinterFaster/internal/pipeline"
	"moneyprinterFaster/internal/queue"
	"moneyprinterFaster/internal/service/compose"
	"moneyprinterFaster/internal/service/llm"
	"moneyprinterFaster/internal/service/material"
	"moneyprinterFaster/internal/service/subtitle"
	"moneyprinterFaster/internal/service/tts"
	"moneyprinterFaster/internal/worker"
	webstatic "moneyprinterFaster/web"
)

var Version = "dev"

func main() {
	configPath := flag.String("config", "config.toml", "配置文件路径")
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	log.Printf("配置加载成功: %s", *configPath)

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 初始化任务队列
	q, err := queue.NewMemoryQueue(cfg.Queue.BufferSize, cfg.Queue.DBPath)
	if err != nil {
		log.Fatalf("初始化队列失败: %v", err)
	}
	log.Printf("任务队列初始化完成 (SQLite: %s)", cfg.Queue.DBPath)

	// 初始化 CPU 监控
	cpuMonitor := worker.NewCPUMonitor(cfg.Worker.CPUSampleInterval)
	cpuMonitor.Start(ctx)
	log.Printf("CPU 监控已启动 (采样间隔: %s)", cfg.Worker.CPUSampleInterval)

	// 初始化 Worker Pool
	poolCfg := worker.PoolConfig{
		MaxWorkers:    cfg.Worker.MaxWorkers,
		MinWorkers:    cfg.Worker.MinWorkers,
		HighWaterMark: cfg.Worker.HighWaterMark,
		LowWaterMark:  cfg.Worker.LowWaterMark,
		ScaleInterval: cfg.Worker.ScaleInterval,
	}
	pool := worker.NewPool(poolCfg, q, cpuMonitor)
	log.Printf("Worker Pool 已创建 (min=%d, max=%d, high=%.2f, low=%.2f)",
		poolCfg.MinWorkers, poolCfg.MaxWorkers,
		poolCfg.HighWaterMark, poolCfg.LowWaterMark)

	// 初始化 LLM Provider
	llmProvider, err := llm.NewProvider(&cfg.LLM)
	if err != nil {
		log.Printf("警告: LLM Provider 初始化失败: %v (文案预览/自动生成将不可用)", err)
		llmProvider = nil
	} else {
		log.Printf("LLM Provider 初始化完成 (provider=%s)", cfg.LLM.Provider)
	}

	// 初始化 TTS Provider
	ttsProvider, err := tts.NewProvider(&cfg.TTS)
	if err != nil {
		log.Printf("警告: TTS Provider 初始化失败: %v", err)
	} else {
		log.Printf("TTS Provider 初始化完成 (provider=%s)", cfg.TTS.Provider)
	}

	// 初始化 Subtitle Provider
	subtitleProvider := subtitle.NewProvider(&cfg.Subtitle)
	log.Printf("Subtitle Provider 初始化完成 (provider=%s)", cfg.Subtitle.Provider)

	// 初始化 Material Provider（多 provider 模式）
	materialProviders := make(map[string]pipeline.MaterialProvider)
	var materialSources []string
	defaultMaterialSource := cfg.Material.Source

	// 尝试初始化所有配置的素材来源
	sources := map[string]struct {
		check  func() bool
		create func() (pipeline.MaterialProvider, error)
	}{
		"pexels": {
			check:  func() bool { return len(cfg.Material.Pexels.APIKeys) > 0 && cfg.Material.Pexels.APIKeys[0] != "" },
			create: func() (pipeline.MaterialProvider, error) { return material.NewPexels(cfg.Material.Pexels.APIKeys), nil },
		},
		"pixabay": {
			check: func() bool { return len(cfg.Material.Pixabay.APIKeys) > 0 && cfg.Material.Pixabay.APIKeys[0] != "" },
			create: func() (pipeline.MaterialProvider, error) {
				return material.NewPixabay(cfg.Material.Pixabay.APIKeys), nil
			},
		},
		"aliyun_image": {
			check: func() bool { return cfg.Material.AliyunImage.APIKey != "" },
			create: func() (pipeline.MaterialProvider, error) {
				return material.NewAliyunImage(
					cfg.Material.AliyunImage.APIKey,
					cfg.Material.AliyunImage.Model,
					cfg.Material.AliyunImage.ImageCount,
					cfg.Material.AliyunImage.ClipSeconds,
				), nil
			},
		},
		"local": {
			check:  func() bool { return cfg.Material.LocalDir != "" },
			create: func() (pipeline.MaterialProvider, error) { return material.NewLocal(cfg.Material.LocalDir), nil },
		},
	}

	// 按固定顺序初始化素材来源（搜索素材在前，AI 生成在后）
	sourceOrder := []string{"pexels", "pixabay", "local", "aliyun_image"}
	for _, name := range sourceOrder {
		src, ok := sources[name]
		if !ok {
			continue
		}
		if src.check() {
			p, err := src.create()
			if err != nil {
				log.Printf("警告: %s Provider 初始化失败: %v", name, err)
				continue
			}
			materialProviders[name] = p
			materialSources = append(materialSources, name)
			log.Printf("Material Provider 初始化完成: %s", name)
		}
	}

	// 检查默认来源是否可用
	var materialErr error
	if len(materialProviders) == 0 {
		materialErr = fmt.Errorf("未配置任何素材服务")
		log.Printf("========================================")
		log.Printf("警告: 未配置任何素材服务")
		log.Printf("请配置 Pexels API Key: 访问 https://www.pexels.com/api/ 免费申请")
		log.Printf("然后在 config.toml 的 [material.pexels] 中填写 api_keys")
		log.Printf("========================================")
	} else if _, ok := materialProviders[defaultMaterialSource]; !ok {
		// 默认来源不可用，使用第一个可用的
		for name := range materialProviders {
			defaultMaterialSource = name
			log.Printf("警告: 默认素材来源 %q 未配置，使用 %q", cfg.Material.Source, name)
			break
		}
	}

	// 初始化 Composer
	composer, err := compose.NewComposer(cfg.FFmpeg.BinaryPath, cfg.FFmpeg.VideoCodec)
	if err != nil {
		log.Printf("警告: Composer 初始化失败: %v", err)
	} else {
		log.Printf("Composer 初始化完成 (codec=%s)", cfg.FFmpeg.VideoCodec)
	}

	// 组装 Pipeline
	var steps []pipeline.Step
	steps = append(steps, pipeline.NewStepLLM(llmProvider))
	if ttsProvider != nil {
		steps = append(steps, pipeline.NewStepTTS(ttsProvider))
	}
	steps = append(steps, pipeline.NewStepSubtitle(subtitleProvider))
	if len(materialProviders) > 0 {
		steps = append(steps, pipeline.NewStepMaterialMulti(materialProviders, defaultMaterialSource))
	}
	if composer != nil {
		steps = append(steps, pipeline.NewStepCompose(composer))
	}

	pipe := pipeline.NewPipeline(steps...)
	log.Printf("Pipeline 组装完成 (%d 个步骤)", len(steps))

	// 注入 Pipeline 到 Executor（必须在 pool.Start 之前）
	pool.GetExecutor().SetPipeline(pipe)

	// 现在启动 Worker Pool（Pipeline 已就绪）
	pool.Start(ctx)
	log.Printf("Worker Pool 已启动 (Pipeline %d 个步骤)", len(steps))

	// 初始化 API 路由
	materialErrMsg := ""
	if materialErr != nil {
		materialErrMsg = materialErr.Error()
	}
	deps := &api.Dependencies{
		Queue:           q,
		Pool:            pool,
		StaticFS:        webstatic.StaticFS,
		LLM:             llmProvider,
		MaterialReady:   len(materialProviders) > 0,
		MaterialError:   materialErrMsg,
		MaterialSources: materialSources,
		ImageConfig: api.ImageConfig{
			ImageCount:  cfg.Material.AliyunImage.ImageCount,
			ClipSeconds: cfg.Material.AliyunImage.ClipSeconds,
			PricePerUSD: 0.015, // z-image-turbo prompt_extend=false: $0.015/张
		},
	}
	router := api.NewRouter(deps)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	// 优雅关闭
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		log.Println("收到关闭信号，开始优雅关闭...")
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server 关闭失败: %v", err)
		}
	}()

	log.Printf("MoneyPrinterFaster v%s 启动", Version)
	log.Printf("HTTP 服务监听: %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP server 错误: %v", err)
	}

	log.Println("MoneyPrinterFaster 已关闭")
}
