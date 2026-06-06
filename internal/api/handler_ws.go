package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"

	"moneyprinterFaster/internal/queue"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 开发阶段允许所有来源
	},
}

// WSHandler WebSocket Handler
type WSHandler struct {
	deps *Dependencies
}

func NewWSHandler(deps *Dependencies) *WSHandler {
	return &WSHandler{deps: deps}
}

// Handle 处理 WebSocket 连接
func (h *WSHandler) Handle(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebSocket] 升级失败: %v", err)
		return
	}
	defer conn.Close()

	// 订阅任务事件
	eventCh := h.deps.Queue.Subscribe()
	defer func() {
		// 取消订阅（类型断言到具体实现）
		if mq, ok := h.deps.Queue.(*queue.MemoryQueue); ok {
			mq.Unsubscribe(eventCh)
		}
	}()

	// 启动读取协程（处理客户端发来的消息，如 ping）
	var readWg sync.WaitGroup
	readWg.Add(1)
	go func() {
		defer readWg.Done()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	// 推送事件到客户端
	for evt := range eventCh {
		data, err := json.Marshal(evt)
		if err != nil {
			continue
		}
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[WebSocket] 写入失败: %v", err)
			break
		}
	}

	readWg.Wait()
}
