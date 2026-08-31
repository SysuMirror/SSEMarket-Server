package config

import (
	"log"
	"os"
	"runtime"
)

// InitPprof 压测/诊断模式下启用 block/mutex 采样（供 /debug/pprof/block、mutex 使用）。
func InitPprof() {
	if os.Getenv("PPROF_ENABLED") != "true" {
		return
	}
	runtime.SetBlockProfileRate(1)
	runtime.SetMutexProfileFraction(5)
	log.Println("[pprof] enabled block=1 mutex=1/5")
}
