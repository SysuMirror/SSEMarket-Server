package controller

// RAGHook: best-effort 通知 sse-agent 更新向量索引（发帖/删帖/转私密）。
// 失败只记日志，agent 侧每 10 分钟对账兜底，不影响主链路。

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

var (
	ragHookURL   = ragEnvStr("RAG_HOOK_URL", "http://host.docker.internal:8100")
	ragHookToken = os.Getenv("RAG_HOOK_TOKEN")
)

func ragEnvStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// RagNotify 异步通知；action: upsert|delete, docType: post|comment。
func RagNotify(action, docType string, id int) {
	if ragHookToken == "" || id == 0 {
		return
	}
	body, err := json.Marshal(map[string]any{"type": docType, "id": id})
	if err != nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			ragHookURL+"/internal/rag/"+action, bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Internal-Token", ragHookToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("[raghook] %s %s/%d failed: %v", action, docType, id, err)
			return
		}
		resp.Body.Close()
	}()
}
