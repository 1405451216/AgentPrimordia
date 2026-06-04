# Phase 9: Operator 债务清理 — 实施计划

> **日期**: 2026-06-05
> **状态**: Plan Complete (Code 待实施)
> **前置条件**: Phase 8.3 已落地 Metrics/Tracing 字段 + 手写 DeepCopy
> **后续**: Phase 11 (TS SDK 完善) / Phase 12 (工具链自动化)

---

## 总览

Phase 8.3 给 Operator 添加 Metrics/Tracing 字段时,**发现**模块本身存在两笔长期未偿债务:

1. `cmd/main.go` 用了 controller-runtime 0.20 已移除的 `MetricsBindAddress` 字段
2. 手写 `zz_generated_deepcopy.go` 替代 `controller-gen` codegen

本 Phase 修复这两笔债,并补 controller 单元测试。

| # | 子目标 | 落地形式 | 状态 |
|:-:|--------|----------|:----:|
| 9.1 | controller-runtime 0.20 API 适配 | `cmd/main.go` 改用 `metricsserver.Options` | ⏳ |
| 9.2 | DeepCopy 改回 codegen | 跑 `controller-gen` 重新生成 `zz_generated_deepcopy.go` | ⏳ |
| 9.3 | Controller 单元测试 | 用 `fake.NewClientBuilder()` 测 Reconcile 路径 | ⏳ |
| 9.4 | 端到端 e2e 验证 | `envtest` 跑 controller 真实环境 | ⏳ |
| 9.5 | Operator 文档完善 | `operator/README.md` 部署 + 测试说明 | ⏳ |

---

## 子阶段 9.1: controller-runtime 0.20 API 适配

### 问题

`operator/cmd/main.go:42` 编译失败:
```
unknown field MetricsBindAddress in struct literal of type manager.Options
```

`MetricsBindAddress` 在 controller-runtime 0.15 已弃用,0.20 移除。新 API 是 `metricsserver.Options` 嵌入 `manager.Options.Metrics`。

### 修复

```go
// Before (旧 API):
mgr, err := manager.New(cfg, manager.Options{
    MetricsBindAddress: metricsAddr,
    Port:              9443,
})

// After (新 API):
mgr, err := manager.New(cfg, manager.Options{
    Metrics: metricsserver.Options{
        BindAddress: metricsAddr,
    },
    HealthProbeBindAddress: probeAddr,
    PprofBindAddress:       pprofAddr,
})
```

### 文件改动

- `operator/cmd/main.go` (1 处) — 替换 `manager.Options` 字段
- `operator/cmd/main.go` 顶部 import — 加 `metricsserver`

### 验证

```bash
cd operator
GOWORK=off go build ./...
```

应通过编译,无 unknown field 错误。

---

## 子阶段 9.2: DeepCopy 改回 controller-gen

### 现状

Phase 8.3 因 module 整体不能编译,手写了 `zz_generated_deepcopy.go` (13 个 DeepCopy 方法 + DeepCopyInto)。这是**临时方案**,理想应改回 codegen。

### 实施步骤

1. **安装 controller-gen**(如未安装):
   ```bash
   go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest
   ```

2. **配置 `Makefile.targets`** (operator/ 独立 Makefile):
   ```makefile
   codegen:
       controller-gen object paths=./api/v1/... \
         output:rttazh:./ \
         output:object:dir=./api/v1
   ```

3. **删除手写** `operator/api/v1/zz_generated_deepcopy.go`

4. **跑 codegen** 重新生成

5. **对比新旧** 确保接口完全一致(DeepCopyObject / DeepCopy / DeepCopyInto)

### 风险

- codegen 输出可能与手写版本**字段顺序不同**(不影响功能,但 diff 会很大)
- codegen 可能产生 `// +k8s:deepcopy-gen=package` marker,需保持兼容

### 回滚

如 codegen 出问题,手写版本仍在 git 历史,`git revert` 即可。

---

## 子阶段 9.3: Controller 单元测试

### 现状

`operator/controller/agent_controller.go` 0% 测试覆盖。Reconcile 逻辑(根据 AgentDeployment 状态创建/更新 Pod)未验证。

### 测试策略

使用 `sigs.k8s.io/controller-runtime/pkg/client/fake` 提供伪 K8s 客户端,无需真实集群:

```go
func TestAgentController_Reconcile_CreatesPodIfNotExists(t *testing.T) {
    ad := &agentv1.AgentDeployment{
        ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
        Spec: agentv1.AgentDeploymentSpec{
            Replicas: 1,
            Template: agentv1.AgentTemplateSpec{
                Provider:     "openai",
                Model:        "gpt-4o",
                SystemPrompt: "你是一个测试 Agent",
            },
        },
    }

    fakeClient := fake.NewClientBuilder().
        WithScheme(scheme).
        WithObjects(ad).
        Build()

    reconciler := &AgentReconciler{
        Client: fakeClient,
        Scheme: scheme,
    }

    // 第一次 Reconcile: 应创建 Pod
    result, err := reconciler.Reconcile(ctx, reconcile.Request{
        NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"},
    })
    if err != nil { t.Fatal(err) }
    if result.Requeue { t.Error("首次 Reconcile 不应 requeue") }

    // 验证 Pod 已创建
    pod := &corev1.Pod{}
    err = fakeClient.Get(ctx, types.NamespacedName{Name: "test", Namespace: "default"}, pod)
    if err != nil { t.Errorf("Pod 应已创建: %v", err) }
}
```

### 测试清单

| 测试 | 覆盖点 |
|------|--------|
| `TestReconcile_CreatesPodIfNotExists` | 首次创建路径 |
| `TestReconcile_UpdatesPodOnSpecChange` | spec 变化触发更新 |
| `TestReconcile_DeletesPodOnAgentDeploymentDelete` | 删除路径 |
| `TestReconcile_UpdatesStatusActiveReplicas` | status 同步 |
| `TestReconcile_HandlesMissingAgentDeployment` | NotFound 不报错 |
| `TestReconcile_RespectsMaxReplicas` | Autoscaling 边界 |

### 工具

- `sigs.k8s.io/controller-runtime/pkg/client/fake` — 伪 K8s 客户端
- `sigs.k8s.io/controller-runtime/pkg/reconcile` — Reconcile 输入输出

---

## 子阶段 9.4: 端到端 e2e 验证

### envtest 方案

controller-runtime 提供 `envtest` 包,用 kubebuilder 下载的 `kube-apiserver` + `etcd` 二进制启动真实 K8s 环境。

### 实施

```go
//go:build envtest

func TestAgentControllerE2E(t *testing.T) {
    testEnv := &envtest.Environment{}
    cfg, err := testEnv.Start()
    if err != nil { t.Fatal(err) }
    defer testEnv.Stop()

    // 真实 K8s 客户端测试
    // ...
}
```

### 复杂度

envtest 需要:
1. 下载 K8s 二进制 (envtest 自动)
2. 标记 `envtest` build tag 隔离
3. CI 集成(用 setup-envtest action)

**本 Phase 范围**: 写 envtest 骨架,CI 跑通即可。完整 e2e 场景留给 Phase 11+。

---

## 子阶段 9.5: 文档完善

### operator/README.md 扩展

```markdown
## 测试

### 单元测试
cd operator
GOWORK=off go test ./api/v1/ -count=1

### 端到端测试 (envtest)
cd operator
GOWORK=off go test -tags=envtest ./controller/ -count=1

### Controller 代码生成
make -C operator codegen
```

### CRD 部署清单

补充:
- RBAC 模板
- ServiceAccount 配置
- Webhook 配置(可选)

---

## 验证结果(预)

### 构建/测试

| 命令 | 预期结果 |
|------|---------|
| `cd operator && GOWORK=off go build ./...` | ✅ 通过 |
| `GOWORK=off go test ./api/v1/ -count=1` | ✅ 8 个测试全过(Phase 8.3 已加) |
| `GOWORK=off go test ./controller/ -count=1` | ✅ 6 个新测试全过 |
| `GOWORK=off go test -tags=envtest ./controller/ -count=1` | ✅ 端到端测试通过 |

### 提交规模

- **5 个 commit**,每个子阶段 1 个
- 代码变动: ~400 行(controller 测试 + cmd 修复 + zz 删除/codegen 重生)
- 新增: 1 个 Makefile.targets(codegen 目标)
- 修改: `operator/README.md` 测试说明

---

## 风险与债务

### 高优先级

1. **controller-gen 版本锁定** — 不同版本输出格式可能微差异
   - 解决: Makefile 固定 `CONTROLLER_GEN_VERSION=v0.14.0`

2. **envtest 二进制下载** — 首次运行需 ~300MB 下载
   - 解决: CI 用 `setup-envtest` action 缓存

### 中优先级

3. **controller 测试的 mock 复杂度** — fake client 不支持所有 controller-runtime 特性
   - 现状: 仅测 Reconcile 路径
   - 后续: 复杂场景用 envtest

### 低优先级

4. **Codegen 产物的 git diff 噪声** — 字段顺序变化导致巨大 diff
   - 接受: 一次性 commit,后续 codegen 输出稳定

---

## 后续工作候选 (Phase 10+)

- Phase 11: TypeScript SDK 完善(跨 SDK 集成测试)
- Phase 12: 工具链自动化(api-diff / cover-trend)
- Phase 13: Webhook 支持(AgentDeployment admission validation)

---

## 反思:Phase 8.3 暴露的问题

1. **手写 DeepCopy 是债** — Phase 8.3 应急方案应标记 TODO:改回 codegen
2. **cmd/main.go 编译失败** 长期未发现 — 反映 "operator module 长期不在 CI 跑通列表"
3. **CI 验证不充分** — Phase 8.3 只测了 `./api/v1/`,没测 `./controller/` 和 `./cmd/`

Phase 9 修复以上,确保 operator 整个 module 在 CI 跑通。
