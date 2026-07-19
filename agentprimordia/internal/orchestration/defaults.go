package orchestration

// defaultWorkerMaxConcurrency 是 worker 池的默认最大并发数。
//
// 原本定义在 supervisor.go（已移除）；engine / mapreduce / orchestrator 等
// 编排主线仍依赖此默认值，故保留。
const defaultWorkerMaxConcurrency = 10
