# MoneyPrinterFaster

> 輸入一句話，AI 全自動生成短影片。從文案到成片，零人工干預。

**語言：** [English](README.en.md) | [简体中文](README.md) | [繁體中文](README.zh-TW.md) | [Español](README.es.md) | [العربية](README.ar.md) | [Français](README.fr.md)

基於 Go 建構的短影片自動化生產線。只需給出主題，系統自動完成 LLM 文案生成 → TTS 語音合成 → 字幕生成 → 素材下載 → 影音合成，最終輸出帶字幕 + BGM 的 MP4 影片。

**線上體驗：[http://47.99.221.247:8080/](http://47.99.221.247:8080/)**

## 架構

```
+---------------------------------------------------------------+
|                     MoneyPrinterFaster                        |
|                                                               |
|   +----------+   +------------+   +------------------------+  |
|   | HTMX Web |   | REST API   |   | WebSocket (進度推送)    |  |
|   | (embed)  |   | (net/http) |   |                        |  |
|   +----+-----+   +-----+------+   +----------+-------------+  |
|        |               |                     |                |
|   +----v---------------v---------------------v------------+   |
|   |                 Router (chi)                            |   |
|   +--------------------------------------------------------+  |
|                            |                                  |
|   +------------------------v------------------------------+   |
|   |                Task Dispatcher                         |   |
|   |  - SQLite 持久化                                       |   |
|   |  - 記憶體 Channel 緩衝                                |   |
|   |  - 優先級 / 取消 / 重試                                |   |
|   +-------------------------------------------------------+   |
|                            |                                  |
|   +------------------------v------------------------------+   |
|   |            CPU-Adaptive Worker Pool                    |   |
|   |  - 定時取樣 CPU 負載 (2s 間隔)                         |   |
|   |  - 動態調整活躍 worker 數                              |   |
|   |  - 負載高時縮減並行，空閒時擴到上限                    |   |
|   +----+--------+--------+--------+--------+---------+---+   |
|        |        |        |        |        |         |       |
|   +----v--+ +---v--+ +---v--+ +---v--+ +---v--+ +---v--+    |
|   |Worker| |Worker| |Worker| |Worker| |Worker| |Worker|     |
|   +---+---+ +---+--+ +---+--+ +---+--+ +---+--+ +---+--+    |
|       |         |        |        |        |        |        |
|   +---v---------v--------v--------v--------v--------v----+   |
|   |               Pipeline Engine                         |   |
|   |                                                       |   |
|   | [LLM] -> [TTS] -> [Subtitle] -> [Material] -> [Compose]|  |
|   |                                                       |   |
|   | 每個 Step 是獨立介面，可替換實作                       |   |
|   +------------------------------------------------------+   |
|                                                               |
|   +------------------------------------------------------+   |
|   |              External Services                         |   |
|   |  LLM (OpenAI / DeepSeek / Ollama)                     |   |
|   |  TTS (Edge-TTS / Azure / OpenAI)                      |   |
|   |  Material (Pexels / Pixabay / Local / AliyunImage)    |   |
|   |  Subtitle (Edge-TTS / Whisper)                        |   |
|   |  FFmpeg (本機二進位呼叫)                                |   |
|   +------------------------------------------------------+   |
+---------------------------------------------------------------+
```

## WebUI

基於 HTMX + Go embed 建構的極簡黑白直角 Web 介面，零前端框架依賴，單檔案部署。

![WebUI](images/webui.png)

## Demo 演示

以下影片均為系統自動生成，無任何人工剪輯：

| 主題 | 演示影片 |
|------|----------|
| 人類為什麼要探索宇宙 | http://xhslink.com/o/2hP8Yp1x07g |
| 閱讀的意義 | http://xhslink.com/o/3xoZeJm0bUc |

## 目錄結構

```
MoneyPrinterFaster/
├── cmd/server/          # 入口，初始化並啟動服務
├── internal/
│   ├── api/             # HTTP 路由、handler、HTML 模板
│   ├── config/          # TOML 設定載入
│   ├── model/           # 資料模型（Task、Params、VideoParams）
│   ├── pipeline/        # 5 步流水線編排
│   ├── queue/           # Channel 佇列 + SQLite 持久化
│   ├── service/         # 外部服務整合
│   │   ├── compose/     # FFmpeg 影片合成
│   │   ├── llm/         # LLM 文案生成（OpenAI/DeepSeek/Ollama）
│   │   ├── material/    # 素材下載（Pexels/AliyunImage/Pixabay）
│   │   ├── subtitle/    # 字幕生成（Edge/Whisper）
│   │   └── tts/         # 語音合成（Edge/Azure/OpenAI）
│   └── worker/          # CPU 自適應 Worker Pool
├── pkg/utils/           # 通用工具（檔案、FFmpeg 路徑偵測）
├── web/                 # 靜態資源 + 前端
├── config.example.toml  # 設定範本
├── vedios/              # 演示影片
└── Makefile             # 建構腳本
```

## 快速開始

### 依賴

- Go 1.22+
- FFmpeg（需支援 libx264）
- Python 3 + edge-tts（`pip install edge-tts`，首次執行自動安裝）

### 設定

```bash
cp config.example.toml config.toml
```

編輯 `config.toml`，填寫 API Key：

| 設定項 | 說明 | 必填 |
|--------|------|------|
| `[llm.deepseek].api_key` | DeepSeek API Key | 是（至少配一個 LLM） |
| `[material.pexels].api_keys` | Pexels API Key 列表（搜尋影片素材） | 是（至少一個素材來源） |
| `[material.aliyun_image].api_key` | 阿里雲百煉 API Key（AI 文生圖） | 否（二選一即可） |
| `[tts]` | TTS 引擎（預設 edge，無需 Key） | 否 |

### 執行

```bash
# 開發模式
make run

# 編譯
make build
./build/moneyprinterFaster

# 交叉編譯
make build-darwin
make build-linux
```

啟動後訪問 `http://localhost:8080`。

### 使用流程

1. 輸入影片主題（如「人工智慧的未來」）
2. 點擊「AI 生成文案」自動生成腳本，或手動填寫
3. 選擇素材來源：**搜尋影片素材**（Pexels）或 **AI 生成圖片**（Z-Image）
4. 調整參數（語言、影片尺寸、語音等）
5. 點擊「提交任務」
6. 任務列表中查看進度，完成後下載 MP4

### 素材來源

| 來源 | 說明 | 費用 |
|------|------|------|
| **搜尋影片素材 (Pexels)** | 根據關鍵詞自動搜尋免版權影片素材 | 免費 |
| **AI 生成圖片 (Z-Image)** | 阿里雲百煉文生圖，根據主題生成 AI 圖片，Ken Burns 緩慢縮放效果轉影片 | $0.015/張 |

## 核心特性

- **多素材來源**：支援 Pexels 影片搜尋 / 阿里雲百煉 Z-Image 文生圖，任務級別可選
- **AI 圖片轉影片**：Z-Image 生成的圖片透過 Ken Burns 縮放效果自動轉為影片片段
- **費用即時預估**：選擇 AI 生成圖片時，前端根據文案即時估算圖片數量和費用
- **CPU 自適應 Worker Pool**：根據 CPU 負載自動擴縮容，不會過載
- **SQLite 持久化佇列**：服務重啟不遺失任務，支援歷史任務查詢
- **FFmpeg 記憶體優化**：逐檔案標準化 + concat demuxer 串流拼接，避免 OOM
- **字幕雙方案**：優先 libass subtitles 濾鏡，降級使用 drawtext
- **時長精準控制**：ffprobe 偵測真實時長，下載素材不超過音訊時長的 110%

## REST API

詳見 [docs/api.md](docs/api.md)
