package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"agentprimordia/internal/admin"
	"agentprimordia/internal/pool"
)

func main() {
	addr := flag.String("addr", ":8080", "管理面板监听地址")
	flag.Parse()

	poolCfg := pool.PoolConfig{MaxConcurrency: 10}
	p := pool.NewPool(poolCfg)
	defer p.Close()

	handler := admin.NewAdminHandler(p)

	fmt.Printf("管理面板监听于 %s\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, handler))
}
