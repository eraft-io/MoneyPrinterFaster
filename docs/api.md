# REST API 接口文档

Base URL: `http://localhost:8080/api/v1`

---

## 目录

- [创建任务](#创建任务)
- [查询任务列表](#查询任务列表)
- [获取任务详情](#获取任务详情)
- [取消任务](#取消任务)
- [下载视频](#下载视频)
- [获取系统状态](#获取系统状态)
- [AI 生成文案](#ai-生成文案)
- [WebSocket 进度推送](#websocket-进度推送)
- [HTMX Web 页面](#htmx-web-页面)
- [通用错误响应格式](#通用错误响应格式)

---

## 创建任务

```
POST /tasks
```

支持 `application/json` 和 `application/x-www-form-urlencoded` 两种格式。

**请求头（JSON 请求时）：**

| Header | 必填 | 说明 |
|--------|------|------|
| `Content-Type` | 是 | `application/json` |
| `X-Client-Token` | 是 | 幂等 token，防止重复提交 |

**请求参数：**

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `client_token` | string | 是 | - | 幂等 token（Form 时为表单字段，JSON 时为 Header） |
| `subject` | string | 二选一 | - | 视频主题（与 `script` 至少填一个） |
| `script` | string | 二选一 | - | 自定义文案（留空则自动生成） |
| `language` | string | 否 | `zh-CN` | 语言：`zh-CN` / `en-US` |
| `paragraph_number` | int | 否 | 5 | 文案段落数（1-10） |
| `aspect` | string | 否 | `9:16` | 视频尺寸：`9:16`（竖屏）/ `16:9`（横屏） |
| `concat_mode` | string | 否 | `random` | 拼接模式：`random` / `sequential` / `reverse` |
| `transition_mode` | string | 否 | `fade` | 转场模式：`none` / `fade` / `slide` / `dissolve` |
| `clip_duration` | int | 否 | 5 | 单片段时长（秒） |
| `video_count` | int | 否 | 1 | 生成视频数量 |
| `voice_name` | string | 否 | `zh-CN-XiaoyiNeural` | 语音名称 |
| `voice_rate` | float | 否 | 0 | 语音速率（-50 ~ +100） |
| `subtitle_enabled` | bool | 否 | false | 启用字幕 |
| `subtitle_font` | string | 否 | `Arial` | 字幕字体 |
| `subtitle_size` | int | 否 | 36 | 字幕大小 |
| `subtitle_color` | string | 否 | `#FFFFFF` | 字幕颜色 |
| `subtitle_position` | string | 否 | `bottom` | 字幕位置：`top` / `center` / `bottom` |
| `subtitle_stroke` | bool | 否 | false | 字幕描边 |
| `bgm_enabled` | bool | 否 | false | 启用背景音乐 |
| `bgm_file` | string | 否 | - | BGM 文件路径（留空随机） |
| `bgm_volume` | float | 否 | 0.2 | BGM 音量（0.0-1.0） |
| `priority` | int | 否 | 1 | 优先级：0=高 / 1=普通 / 2=低 |

**curl 示例（JSON）：**

```bash
curl -s -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -H "X-Client-Token: $(uuidgen)" \
  -d '{
    "subject": "人工智能的未来",
    "language": "zh-CN",
    "aspect": "9:16",
    "voice_name": "zh-CN-XiaoyiNeural",
    "subtitle_enabled": true,
    "bgm_enabled": true
  }'
```

**curl 示例（Form）：**

```bash
curl -s -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "client_token=$(uuidgen)" \
  -d "subject=人工智能的未来" \
  -d "language=zh-CN" \
  -d "aspect=9:16" \
  -d "voice_name=zh-CN-XiaoyiNeural" \
  -d "subtitle_enabled=on" \
  -d "bgm_enabled=on"
```

**成功响应 `201 Created`：**

```json
{
  "task_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "submitted"
}
```

**错误响应：**

| 状态码 | 说明 |
|--------|------|
| `400` | 缺少必填参数 / 素材服务未配置 / 缺少 client_token |
| `409` | 重复提交（同一 client_token 60 秒内重复） |

```json
// 400 缺少 client_token
{"error": "缺少 client_token，请刷新页面重试"}

// 400 素材服务未配置
{"error": "素材服务未配置，无法生成视频。"}

// 409 重复提交
{"error": "重复提交，请勿频繁点击"}
```

---

## 查询任务列表

```
GET /tasks?offset=0&limit=20
```

**查询参数：**

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `offset` | int | 否 | 0 | 偏移量 |
| `limit` | int | 否 | 20 | 每页数量 |

**curl 示例：**

```bash
curl -s http://localhost:8080/api/v1/tasks?offset=0\&limit=10
```

**响应 `200 OK`：**

```json
{
  "tasks": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "state": "completed",
      "priority": 1,
      "progress": 100,
      "created_at": "2026-06-05T10:30:00Z",
      "started_at": "2026-06-05T10:30:01Z",
      "finished_at": "2026-06-05T10:32:15Z",
      "output_path": "./data/tasks/550e8400/final_1.mp4",
      "video_duration": 62.5,
      "params": { "subject": "人工智能的未来", "...": "..." }
    },
    {
      "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "state": "processing",
      "priority": 1,
      "progress": 45.0,
      "created_at": "2026-06-05T10:33:00Z",
      "started_at": "2026-06-05T10:33:02Z",
      "params": { "subject": "宇宙探索", "...": "..." }
    }
  ],
  "total": 42,
  "offset": 0,
  "limit": 10
}
```

---

## 获取任务详情

```
GET /tasks/{id}
```

**curl 示例：**

```bash
curl -s http://localhost:8080/api/v1/tasks/550e8400-e29b-41d4-a716-446655440000
```

**响应 `200 OK`：**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "state": "completed",
  "priority": 1,
  "progress": 100,
  "created_at": "2026-06-05T10:30:00Z",
  "started_at": "2026-06-05T10:30:01Z",
  "finished_at": "2026-06-05T10:32:15Z",
  "output_path": "./data/tasks/550e8400/final_1.mp4",
  "video_duration": 62.5,
  "params": {
    "subject": "人工智能的未来",
    "script": "人工智能正在改变我们的生活方式...",
    "language": "zh-CN",
    "paragraph_number": 5,
    "aspect": "9:16",
    "concat_mode": "random",
    "voice_name": "zh-CN-XiaoyiNeural",
    "subtitle_enabled": true,
    "bgm_enabled": true
  }
}
```

**错误响应：**

```json
// 404
{"error": "任务不存在: invalid-id"}
```

| 状态码 | 说明 |
|--------|------|
| `404` | 任务不存在 |

---

## 取消任务

```
DELETE /tasks/{id}
```

**curl 示例：**

```bash
curl -s -X DELETE http://localhost:8080/api/v1/tasks/550e8400-e29b-41d4-a716-446655440000
```

**成功响应 `200 OK`：**

```json
{
  "task_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "cancelled"
}
```

**错误响应：**

```json
// 400
{"error": "取消失败: 任务已完成，无法取消"}
```

| 状态码 | 说明 |
|--------|------|
| `400` | 取消失败（任务已完成或不可取消） |

---

## 下载视频

```
GET /tasks/{id}/download
```

**curl 示例：**

```bash
curl -o output.mp4 http://localhost:8080/api/v1/tasks/550e8400-e29b-41d4-a716-446655440000/download
```

**成功响应 `200 OK`：** 返回 `video/mp4` 文件流

```
Content-Type: video/mp4
Content-Disposition: attachment; filename=550e8400-e29b-41d4-a716-446655440000.mp4
```

**错误响应：**

```json
// 400
{"error": "任务未完成，无法下载"}

// 404
{"error": "任务不存在: invalid-id"}
```

| 状态码 | 说明 |
|--------|------|
| `400` | 任务未完成 |
| `404` | 任务不存在 / 输出文件不存在 |

---

## 获取系统状态

```
GET /status
```

**curl 示例：**

```bash
curl -s http://localhost:8080/api/v1/status
```

**响应 `200 OK`：**

```json
{
  "active_workers": 3,
  "target_workers": 4,
  "cpu_usage": 45.2,
  "queue_length": 2,
  "num_cpu": 8
}
```

---

## AI 生成文案

```
POST /llm/generate-script
```

**curl 示例：**

```bash
curl -s -X POST http://localhost:8080/api/v1/llm/generate-script \
  -H "Content-Type: application/json" \
  -d '{
    "subject": "人工智能的未来",
    "language": "zh-CN",
    "paragraph_number": 5
  }'
```

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `subject` | string | 是 | - | 视频主题 |
| `language` | string | 否 | `zh-CN` | 语言 |
| `paragraph_number` | int | 否 | 5 | 段落数 |

**成功响应 `200 OK`：**

```json
{
  "script": "人工智能正在改变我们的生活方式。从医疗诊断到自动驾驶，AI 技术正以前所未有的速度渗透到生活的方方面面。未来十年，人机协作将成为主流模式..."
}
```

**错误响应：**

```json
// 400 缺少主题
{"error": "必须提供视频主题"}

// 500 生成失败
{"error": "生成文案失败: context deadline exceeded"}

// 503 未配置
{"error": "LLM 服务未配置，请检查 config.toml"}
```

| 状态码 | 说明 |
|--------|------|
| `400` | 缺少主题参数 |
| `500` | LLM 生成失败 |
| `503` | LLM 服务未配置 |

---

## WebSocket 进度推送

```
GET /ws
```

WebSocket 连接，实时推送任务进度事件。

**连接示例（wscat）：**

```bash
# 安装 wscat: npm install -g wscat
wscat -c ws://localhost:8080/ws
```

**推送消息格式：**

```json
// LLM 文案生成中
{
  "task_id": "550e8400-e29b-41d4-a716-446655440000",
  "state": "processing",
  "progress": 10.0,
  "step": "LLM 文案生成",
  "message": "",
  "timestamp": "2026-06-05T10:31:05Z"
}

// TTS 语音合成中
{
  "task_id": "550e8400-e29b-41d4-a716-446655440000",
  "state": "processing",
  "progress": 35.0,
  "step": "TTS 语音合成",
  "message": "",
  "timestamp": "2026-06-05T10:31:20Z"
}

// 任务完成
{
  "task_id": "550e8400-e29b-41d4-a716-446655440000",
  "state": "completed",
  "progress": 100,
  "step": "",
  "message": "",
  "timestamp": "2026-06-05T10:32:15Z"
}

// 任务失败
{
  "task_id": "550e8400-e29b-41d4-a716-446655440000",
  "state": "failed",
  "progress": 45.0,
  "step": "Material 素材下载",
  "message": "",
  "timestamp": "2026-06-05T10:31:45Z",
  "error": "素材下载失败: API 请求超时"
}
```

**状态流转：**

```
pending → processing → completed
                    → failed
                    → cancelled
```

---

## HTMX Web 页面

| 路由 | 方法 | 说明 |
|------|------|------|
| `/` | GET | 首页（创建任务 + 任务列表） |
| `/tasks/list` | GET | 任务列表 HTML 片段（HTMX 局部刷新） |
| `/static/*` | GET | 静态资源 |

**curl 示例：**

```bash
# 访问首页
curl http://localhost:8080/

# 获取任务列表 HTML 片段
curl http://localhost:8080/tasks/list
```

---

## 通用错误响应格式

所有 API 错误统一返回以下格式：

```json
{
  "error": "错误描述信息"
}
```

---

## 端到端完整示例

一个典型的完整工作流：

```bash
# 1. 生成文案
curl -s -X POST http://localhost:8080/api/v1/llm/generate-script \
  -H "Content-Type: application/json" \
  -d '{"subject": "宇宙探索", "language": "zh-CN", "paragraph_number": 5}'
# => {"script": "人类对宇宙的探索从未停止..."}

# 2. 创建视频任务
TASK_ID=$(curl -s -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -H "X-Client-Token: $(uuidgen)" \
  -d '{
    "subject": "宇宙探索",
    "language": "zh-CN",
    "aspect": "9:16",
    "voice_name": "zh-CN-XiaoyiNeural",
    "subtitle_enabled": true,
    "bgm_enabled": true
  }' | grep -o '"task_id":"[^"]*"' | cut -d'"' -f4)
echo "Task ID: $TASK_ID"
# => Task ID: 550e8400-e29b-41d4-a716-446655440000

# 3. 查看系统状态
curl -s http://localhost:8080/api/v1/status
# => {"active_workers":3,"target_workers":4,"cpu_usage":12.5,"queue_length":1,"num_cpu":8}

# 4. 轮询任务状态
curl -s http://localhost:8080/api/v1/tasks/$TASK_ID | jq .state
# => "processing"

# 5. 任务完成后下载视频
curl -s http://localhost:8080/api/v1/tasks/$TASK_ID | jq .state
# => "completed"
curl -o output.mp4 http://localhost:8080/api/v1/tasks/$TASK_ID/download
```
