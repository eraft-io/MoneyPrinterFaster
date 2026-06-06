# MoneyPrinterFaster 架构设计方案

## 1. 整体架构概览

```
+---------------------------------------------------------------+
|                      MoneyPrinterFaster                        |
|                                                               |
|  +----------+    +------------+    +------------------------+ |
|  | HTMX Web |    | REST API   |    |  WebSocket (进度推送)   | |
|  | (embed)  |    | (net/http) |    |                        | |
|  +-----+----+    +------+-----+    +-----------+------------+ |
|        |                |                      |              |
|  +-----v----------------v----------------------v-----------+  |
|  |                  Router (chi)                            |  |
|  +----------------------------+----------------------------+  |
|                               |                               |
|  +----------------------------v----------------------------+  |
|  |                 Task Dispatcher                          |  |
|  |  - WAL 持久化 (追加写磁盘日志)                            |  |
|  |  - 内存 Channel 缓冲                                    |  |
|  |  - 优先级 / 取消 / 重试                                  |  |
|  +----------------------------+----------------------------+  |
|                               |                               |
|  +----------------------------v----------------------------+  |
|  |              CPU-Adaptive Worker Pool                    |  |
|  |  - 定时采样 CPU 负载 (2s 间隔)                           |  |
|  |  - 动态调整活跃 worker 数                                |  |
|  |  - 负载高时缩减并发，空闲时扩到上限                       |  |
|  +---+--------+--------+--------+--------+---------+------+  |
|      |        |        |        |        |         |          |
|  +---v--+ +---v--+ +---v--+ +---v--+ +---v--+ +---v--+       |
|  |Worker| |Worker| |Worker| |Worker| |Worker| |Worker|       |
|  +--+---+ +--+---+ +--+---+ +--+---+ +--+---+ +--+---+       |
|     |        |        |        |        |        |            |
|  +--v--------v--------v--------v--------v--------v--------+  |
|  |                  Pipeline Engine                         |  |
|  |                                                          |  |
|  |  [LLM] -> [TTS] -> [Subtitle] -> [Material] -> [Compose]|  |
|  |                                                          |  |
|  |  每个 Step 是独立接口，可替换实现                         |  |
|  +----------------------------------------------------------+  |
|                                                               |
|  +----------------------------------------------------------+  |
|  |                  External Services                        |  |
|  |  LLM (OpenAI/DeepSeek/Ollama/...)                        |  |
|  |  TTS (Edge/Azure/Gemini/MiMo)                            |  |
|  |  Material (Pexels/Pixabay/Local)                         |  |
|  |  Subtitle (Edge-TTS/Whisper)                             |  |
|  |  FFmpeg (本地二进制调用)                                   |  |
|  +----------------------------------------------------------+  |
+---------------------------------------------------------------+
```

## 2. 项目目录结构

```
MoneyPrinterFaster/
├── cmd/
│   └── server/
│       └── main.go              # 入口，启动 HTTP server + worker pool
├── internal/
│   ├── config/
│   │   └── config.go            # TOML 配置加载
│   ├── api/
│   │   ├── router.go            # chi router 注册
│   │   ├── handler_task.go      # 任务 CRUD API
│   │   ├── handler_llm.go       # LLM 测试 API
│   │   └── handler_ws.go        # WebSocket 进度推送
│   ├── web/
│   │   ├── handler.go           # HTMX 页面 handler
│   │   └── templates/           # Go html/template 模板文件
│   │       ├── layout.html
│   │       ├── task_new.html
│   │       ├── task_list.html
│   │       └── task_detail.html
│   ├── queue/
│   │   ├── queue.go             # 任务队列接口定义
│   │   ├── memory.go            # Channel-based 内存队列
│   │   ├── wal.go               # WAL 磁盘持久化
│   │   └── task.go              # Task 数据结构
│   ├── worker/
│   │   ├── pool.go              # CPU 自适应 Worker Pool
│   │   ├── cpu_monitor.go       # CPU 负载采样
│   │   └── executor.go          # 单任务执行器（调 pipeline）
│   ├── pipeline/
│   │   ├── pipeline.go          # Pipeline 编排引擎
│   │   ├── step.go              # Step 接口定义
│   │   ├── step_llm.go          # LLM 文案生成
│   │   ├── step_tts.go          # TTS 语音合成
│   │   ├── step_subtitle.go     # 字幕生成
│   │   ├── step_material.go     # 素材采集
│   │   └── step_compose.go      # FFmpeg 视频合成
│   ├── service/
│   │   ├── llm/
│   │   │   ├── provider.go      # LLM Provider 接口
│   │   │   ├── openai.go        # OpenAI 兼容实现
│   │   │   ├── ollama.go        # Ollama 本地模型
│   │   │   └── deepseek.go      # DeepSeek
│   │   ├── tts/
│   │   │   ├── provider.go      # TTS Provider 接口
│   │   │   ├── edge.go          # Edge TTS (调 Python edge-tts CLI)
│   │   │   ├── azure.go         # Azure Speech
│   │   │   └── openai.go        # OpenAI TTS
│   │   ├── subtitle/
│   │   │   ├── provider.go      # Subtitle Provider 接口
│   │   │   ├── edge.go          # Edge TTS 时间戳
│   │   │   └── whisper.go       # Whisper (调 CLI)
│   │   ├── material/
│   │   │   ├── provider.go      # Material Provider 接口
│   │   │   ├── pexels.go        # Pexels API
│   │   │   ├── pixabay.go       # Pixabay API
│   │   │   └── local.go         # 本地素材
│   │   └── compose/
│   │       ├── composer.go      # FFmpeg 视频合成
│   │       └── ffmpeg.go        # FFmpeg 命令封装
│   └── model/
│       ├── task.go              # Task 状态机
│       ├── video.go             # VideoParams
│       └── progress.go          # Progress 事件
├── pkg/
│   ├── ffmpeg/
│   │   └── ffmpeg.go            # FFmpeg 底层调用封装
│   └── utils/
│       └── utils.go             # 通用工具
├── web/
│   └── static/                  # 静态资源 (CSS/JS/HTMX)
│       ├── htmx.min.js
│       └── style.css
├── config.example.toml          # 配置模板
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## 3. 核心模块详细设计

### 3.1 任务队列 (internal/queue/)

**设计目标**: 零外部依赖、Channel 驱动、WAL 保证不丢任务

```go
// internal/queue/task.go

type TaskPriority int

const (
    PriorityHigh   TaskPriority = 0
    PriorityNormal TaskPriority = 1
    PriorityLow    TaskPriority = 2
)

type TaskState string

const (
    StatePending    TaskState = "pending"
    StateProcessing TaskState = "processing"
    StateCompleted  TaskState = "completed"
    StateFailed     TaskState = "failed"
    StateCancelled  TaskState = "cancelled"
)

type Task struct {
    ID         string            `json:"id"`
    State      TaskState         `json:"state"`
    Priority   TaskPriority      `json:"priority"`
    Params     VideoParams       `json:"params"`
    Progress   float64           `json:"progress"`
    Error      string            `json:"error,omitempty"`
    CreatedAt  time.Time         `json:"created_at"`
    StartedAt  *time.Time        `json:"started_at,omitempty"`
    FinishedAt *time.Time        `json:"finished_at,omitempty"`
    OutputPath string            `json:"output_path,omitempty"`
    cancelFunc context.CancelFunc `json:"-"` // 运行时取消句柄
}
```

```go
// internal/queue/queue.go

type Queue interface {
    // Submit 提交任务到队列，返回任务 ID
    Submit(ctx context.Context, params VideoParams, priority TaskPriority) (string, error)
    // Cancel 取消任务
    Cancel(taskID string) error
    // Get 获取任务状态
    Get(taskID string) (*Task, error)
    // List 列出任务（分页）
    List(offset, limit int) ([]*Task, int, error)
    // Dequeue 从队列取出一个任务（阻塞），仅 worker 内部使用
    Dequeue(ctx context.Context) (*Task, error)
}
```

```go
// internal/queue/memory.go — 核心逻辑

type MemoryQueue struct {
    mu       sync.RWMutex
    tasks    map[string]*Task          // 全量任务索引
    ch       chan *Task                 // 缓冲 channel，capacity = maxQueueSize
    wal      *WAL                       // WAL 日志
    onState  func(task *Task)           // 状态变更回调（用于 WebSocket 推送）
}

func NewMemoryQueue(bufSize int, walDir string) (*MemoryQueue, error) {
    wal, err := NewWAL(walDir)
    // ...
    q := &MemoryQueue{
        tasks: make(map[string]*Task),
        ch:    make(chan *Task, bufSize),
        wal:   wal,
    }
    // 启动时从 WAL 恢复未完成任务
    q.recoverFromWAL()
    return q, nil
}

func (q *MemoryQueue) Submit(ctx context.Context, params VideoParams, priority TaskPriority) (string, error) {
    task := &Task{
        ID:        uuid.New().String(),
        State:     StatePending,
        Priority:  priority,
        Params:    params,
        CreatedAt: time.Now(),
    }
    // 1. 先写 WAL（持久化保证）
    if err := q.wal.Append(task); err != nil {
        return "", err
    }
    // 2. 入内存索引
    q.mu.Lock()
    q.tasks[task.ID] = task
    q.mu.Unlock()
    // 3. 非阻塞入 channel（满了就走优先级排序兜底）
    select {
    case q.ch <- task:
    default:
        // channel 满，任务已在索引中，后续由 dequeue 兜底扫描
    }
    return task.ID, nil
}
```

**WAL 持久化策略** (`internal/queue/wal.go`):
- 追加写 JSON-lines 到 `data/queue.wal`
- 任务完成后写 `DONE {id}` 标记
- 启动时扫描 WAL，将未完成的任务重新入队
- 定期 compaction（已完成任务超过阈值时重写 WAL）

### 3.2 CPU 自适应 Worker Pool (internal/worker/)

**这是整个架构的核心差异点**，根据 CPU 负载动态调节并发数。

```go
// internal/worker/cpu_monitor.go

type CPUMonitor struct {
    mu         sync.RWMutex
    current    float64        // 当前 CPU 使用率 (0.0 ~ 1.0)
    interval   time.Duration  // 采样间隔，默认 2s
    numCPU     int            // runtime.NumCPU()
}

func NewCPUMonitor(interval time.Duration) *CPUMonitor {
    return &CPUMonitor{
        interval: interval,
        numCPU:   runtime.NumCPU(),
    }
}

// Start 启动后台采样 goroutine
// 通过读取 /proc/stat (Linux) 或 syscall (macOS) 获取系统级 CPU 使用率
func (m *CPUMonitor) Start(ctx context.Context) {
    go func() {
        ticker := time.NewTicker(m.interval)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                usage := m.sampleCPU()
                m.mu.Lock()
                m.current = usage
                m.mu.Unlock()
            }
        }
    }()
}

func (m *CPUMonitor) Usage() float64 {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return m.current
}

func (m *CPUMonitor) NumCPU() int { return m.numCPU }
```

```go
// internal/worker/pool.go

type PoolConfig struct {
    MaxWorkers      int     // 最大 worker 数，默认 NumCPU
    MinWorkers      int     // 最小 worker 数，默认 1
    HighWaterMark   float64 // CPU 高水位，超过则缩减 worker，默认 0.85
    LowWaterMark    float64 // CPU 低水位，低于则增加 worker，默认 0.50
    ScaleInterval   time.Duration // 扩缩容检查间隔，默认 5s
}

type WorkerPool struct {
    cfg      PoolConfig
    queue    queue.Queue
    monitor  *CPUMonitor
    executor *Executor

    mu           sync.Mutex
    activeCount  int              // 当前活跃 worker 数
    targetCount  int              // 目标 worker 数
    running      map[string]bool  // worker goroutine 标识
    cancelFuncs  map[string]context.CancelFunc
}

func NewPool(cfg PoolConfig, q queue.Queue, monitor *CPUMonitor) *WorkerPool {
    return &WorkerPool{
        cfg:         cfg,
        queue:       q,
        monitor:     monitor,
        executor:    NewExecutor(),
        activeCount: cfg.MinWorkers,
        targetCount: cfg.MinWorkers,
        running:     make(map[string]bool),
        cancelFuncs: make(map[string]context.CancelFunc),
    }
}

// Start 启动 pool 并拉起初始 worker
func (p *WorkerPool) Start(ctx context.Context) {
    // 1. 启动 CPU monitor
    p.monitor.Start(ctx)

    // 2. 拉起初始 worker
    for i := 0; i < p.cfg.MinWorkers; i++ {
        p.spawnWorker(ctx)
    }

    // 3. 启动扩缩容控制器
    go p.scaleLoop(ctx)
}

// scaleLoop 核心扩缩容逻辑
func (p *WorkerPool) scaleLoop(ctx context.Context) {
    ticker := time.NewTicker(p.cfg.ScaleInterval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            p.adjustWorkers(ctx)
        }
    }
}

func (p *WorkerPool) adjustWorkers(ctx context.Context) {
    cpuUsage := p.monitor.Usage()
    numCPU := p.monitor.NumCPU()

    p.mu.Lock()
    current := p.activeCount
    p.mu.Unlock()

    target := current

    switch {
    case cpuUsage > p.cfg.HighWaterMark:
        // CPU 过载，缩减 worker（但保留至少 MinWorkers 个）
        target = max(current-1, p.cfg.MinWorkers)
    case cpuUsage < p.cfg.LowWaterMark:
        // CPU 空闲，扩容 worker（不超过 MaxWorkers）
        // 扩容步长：距上限的一半，避免震荡
        headroom := p.cfg.MaxWorkers - current
        if headroom > 0 {
            step := max(headroom/2, 1)
            target = min(current+step, p.cfg.MaxWorkers)
        }
    }
    // 中水位区间保持不变，形成迟滞带，避免频繁扩缩

    if target == current {
        return
    }

    p.mu.Lock()
    p.targetCount = target
    p.mu.Unlock()

    if target > current {
        // 扩容：启动新 worker
        for i := 0; i < target-current; i++ {
            p.spawnWorker(ctx)
        }
    } else {
        // 缩容：取消多余的 worker
        p.mu.Lock()
        toRemove := current - target
        removed := 0
        for id, cancel := range p.cancelFuncs {
            if removed >= toRemove {
                break
            }
            cancel()
            delete(p.cancelFuncs, id)
            removed++
        }
        p.mu.Unlock()
    }
}

func (p *WorkerPool) spawnWorker(parentCtx context.Context) {
    workerID := uuid.New().String()
    ctx, cancel := context.WithCancel(parentCtx)

    p.mu.Lock()
    p.activeCount++
    p.running[workerID] = true
    p.cancelFuncs[workerID] = cancel
    p.mu.Unlock()

    go func() {
        defer func() {
            p.mu.Lock()
            p.activeCount--
            delete(p.running, workerID)
            p.mu.Unlock()
        }()

        for {
            select {
            case <-ctx.Done():
                return
            default:
                task, err := p.queue.Dequeue(ctx)
                if err != nil {
                    continue
                }
                // 执行任务（阻塞，一个 worker 同时只处理一个任务）
                p.executor.Execute(ctx, task)
            }
        }
    }()
}
```

**扩缩容策略图示**:

```
CPU Usage
  1.0 ─────────────────────────────────
      |                                |
  0.85 ──── HighWaterMark ──────────── | → 缩减 worker
      |                                |
      |     迟滞带 (保持不变)           |
      |                                |
  0.50 ──── LowWaterMark ───────────── | → 扩容 worker
      |                                |
  0.0 ──────────────────────────────── 
         MinWorkers          MaxWorkers
```

### 3.3 Pipeline 引擎 (internal/pipeline/)

```go
// internal/pipeline/step.go

type StepResult struct {
    Data  map[string]any // 步骤产出数据，传递给下游
    Error error
}

type Step interface {
    Name() string
    Execute(ctx context.Context, task *queue.Task, prevData map[string]any) (*StepResult, error)
}
```

```go
// internal/pipeline/pipeline.go

type Pipeline struct {
    steps []Step
}

func NewPipeline(steps ...Step) *Pipeline {
    return &Pipeline{steps: steps}
}

func (p *Pipeline) Run(ctx context.Context, task *queue.Task, onProgress func(float64)) error {
    data := make(map[string]any)
    stepWeight := 100.0 / float64(len(p.steps))

    for i, step := range p.steps {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }

        result, err := step.Execute(ctx, task, data)
        if err != nil {
            return fmt.Errorf("step %s failed: %w", step.Name(), err)
        }

        // 合并产出到 data，供下游步骤读取
        for k, v := range result.Data {
            data[k] = v
        }

        onProgress(float64(i+1) * stepWeight)
    }
    return nil
}
```

**Pipeline 步骤流转**:

```
Step 1: LLM        → data["script"], data["terms"]
Step 2: TTS        → data["audio_path"], data["sub_maker"]
Step 3: Subtitle   → data["subtitle_path"]
Step 4: Material   → data["video_paths"]
Step 5: Compose    → data["final_video_path"]
```

### 3.4 Service Provider 接口设计

以 LLM 为例展示 Provider 统一接口模式：

```go
// internal/service/llm/provider.go

type Provider interface {
    GenerateScript(ctx context.Context, subject, language string, paragraphs int) (string, error)
    GenerateTerms(ctx context.Context, subject, script string) ([]string, error)
}

func NewProvider(cfg *config.Config) Provider {
    switch cfg.LLM.Provider {
    case "openai":
        return NewOpenAI(cfg.LLM.OpenAI)
    case "deepseek":
        return NewDeepSeek(cfg.LLM.DeepSeek)
    case "ollama":
        return NewOllama(cfg.LLM.Ollama)
    // ... 其他 provider
    default:
        return NewOpenAI(cfg.LLM.OpenAI)
    }
}
```

TTS、Subtitle、Material 同理，都是 `Provider interface + 多实现`。

### 3.5 视频合成 (internal/service/compose/)

通过 `os/exec` 调用本地 FFmpeg 二进制，不依赖 CGo：

```go
// internal/service/compose/ffmpeg.go

type FFmpeg struct {
    binaryPath string  // 自动检测或用户配置
}

func (f *FFmpeg) ConcatVideos(ctx context.Context, inputs []string, output string, opts ConcatOpts) error {
    // 使用 ffmpeg concat demuxer 或 filter_complex
    args := f.buildConcatArgs(inputs, output, opts)
    cmd := exec.CommandContext(ctx, f.binaryPath, args...)
    return cmd.Run()
}

func (f *FFmpeg) ComposeVideo(ctx context.Context, opts ComposeOpts) error {
    // 合成视频轨道 + 音频轨道 + 字幕 + BGM
    // 支持硬件编码器，失败自动回退 libx264
}
```

### 3.6 API 与前端

**API 路由设计** (`internal/api/router.go`):

```go
func NewRouter(deps *Dependencies) http.Handler {
    r := chi.NewRouter()
    r.Use(middleware.Logger, middleware.Recoverer)

    // API
    r.Route("/api/v1", func(r chi.Router) {
        r.Post("/tasks", handler.CreateTask)       // 创建任务
        r.Get("/tasks", handler.ListTasks)          // 任务列表
        r.Get("/tasks/{id}", handler.GetTask)       // 任务详情
        r.Delete("/tasks/{id}", handler.CancelTask) // 取消任务
        r.Get("/ws", handler.WebSocket)             // 进度推送
    })

    // Web (HTMX)
    r.Get("/", web.IndexPage)
    r.Get("/tasks/new", web.NewTaskForm)
    r.Post("/tasks/submit", web.SubmitTask)         // HTMX 表单提交
    r.Get("/tasks/list", web.TaskList)              // HTMX 局部刷新
    r.Get("/tasks/{id}/detail", web.TaskDetail)     // HTMX 任务详情

    // 静态资源
    r.Handle("/static/*", http.FileServer(http.FS(staticFS)))

    // 视频文件下载
    r.Get("/tasks/{id}/download", handler.DownloadVideo)

    return r
}
```

**WebSocket 进度推送**:
- 任务状态变更时通过 channel 通知 WebSocket hub
- 前端 HTMX + 少量 JS 监听 WebSocket 更新进度条

### 3.7 配置结构 (config.example.toml)

```toml
[server]
host = "0.0.0.0"
port = 8080

[queue]
buffer_size = 100          # Channel 缓冲大小
wal_dir = "./data/wal"     # WAL 存储目录

[worker]
max_workers = 4            # 最大并发 worker 数
min_workers = 1            # 最小并发 worker 数
high_water_mark = 0.85     # CPU 高水位
low_water_mark = 0.50      # CPU 低水位
scale_interval = "5s"      # 扩缩容检查间隔
cpu_sample_interval = "2s" # CPU 采样间隔

[llm]
provider = "openai"

[llm.openai]
api_key = ""
base_url = "https://api.openai.com/v1"
model = "gpt-4o-mini"

[tts]
provider = "edge"

[tts.azure]
speech_key = ""
speech_region = "eastus"

[subtitle]
provider = "edge"          # "edge" | "whisper" | ""

[material]
source = "pexels"          # "pexels" | "pixabay" | "local"

[material.pexels]
api_keys = []

[material.pixabay]
api_keys = []

[ffmpeg]
binary_path = ""           # 留空自动检测
video_codec = "libx264"    # 支持硬件编码器
```

## 4. 关键依赖

| 依赖 | 用途 |
|------|------|
| `github.com/go-chi/chi/v5` | HTTP 路由 |
| `github.com/google/uuid` | 任务 ID 生成 |
| `github.com/BurntSushi/toml` | 配置解析 |
| `github.com/gorilla/websocket` | WebSocket 进度推送 |
| `github.com/sashabaranov/go-openai` | OpenAI SDK |
| `github.com/shirou/gopsutil/v3` | CPU 使用率采样 (跨平台) |
| 标准库 `html/template` | HTMX 模板渲染 |
| 标准库 `embed` | 嵌入静态资源和模板 |
| 标准库 `os/exec` | 调用 FFmpeg / edge-tts CLI |

## 5. 实施任务清单

### Task 1: 项目脚手架
- 初始化 Go module、目录结构、Makefile
- 创建 `cmd/server/main.go` 入口
- 集成 chi router、embed 静态资源
- 集成 TOML 配置加载

### Task 2: 任务队列
- 实现 Task 数据结构和状态机
- 实现 MemoryQueue（Channel + 优先级）
- 实现 WAL 持久化（写入 / 恢复 / compaction）
- 编写队列单元测试

### Task 3: CPU 自适应 Worker Pool
- 实现 CPUMonitor（基于 gopsutil）
- 实现 WorkerPool（扩缩容逻辑、worker 生命周期管理）
- 实现 Executor（调用 Pipeline、更新任务状态）
- 编写 Worker Pool 单元测试

### Task 4: Pipeline 引擎
- 定义 Step 接口和 Pipeline 编排
- 实现各 Step 骨架（LLM / TTS / Subtitle / Material / Compose）

### Task 5: Service Providers
- LLM Provider（OpenAI 兼容 + DeepSeek + Ollama）
- TTS Provider（Edge TTS 通过 CLI 调用 + Azure）
- Subtitle Provider（Edge 时间戳 + Whisper CLI）
- Material Provider（Pexels + Pixabay + Local）
- Compose（FFmpeg 命令封装 + 编码器回退）

### Task 6: API 层
- RESTful API（任务 CRUD）
- WebSocket 进度推送
- 视频文件下载

### Task 7: HTMX 前端
- 布局模板 + HTMX 集成
- 新建任务表单（主题、参数配置）
- 任务列表（实时状态刷新）
- 任务详情页（进度条、视频预览/下载）

### Task 8: 集成与测试
- 端到端流程测试（从提交任务到生成视频）
- CPU 负载模拟下的扩缩容验证
- WAL 故障恢复测试