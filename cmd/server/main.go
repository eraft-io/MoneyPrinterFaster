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

	// 初始化 Material Provider
	var materialErr error
	materialProvider, materialErr := material.NewProvider(&cfg.Material)
	if materialErr != nil {
		log.Printf("========================================")
		log.Printf("警告: Material Provider 初始化失败: %v", materialErr)
		log.Printf("请配置 Pexels API Key: 访问 https://www.pexels.com/api/ 免费申请")
		log.Printf("然后在 config.toml 的 [material.pexels] 中填写 api_keys")
		log.Printf("========================================")
		materialProvider = nil
	} else {
		log.Printf("Material Provider 初始化完成 (source=%s)", cfg.Material.Source)
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
	if materialProvider != nil {
		steps = append(steps, pipeline.NewStepMaterial(materialProvider))
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
		Queue:         q,
		Pool:          pool,
		StaticFS:      webstatic.StaticFS,
		LLM:           llmProvider,
		MaterialReady: materialProvider != nil,
		MaterialError: materialErrMsg,
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
