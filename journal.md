# journal.md

详细记录每一步进展的流水账。

---

## 2026-05-12

### 诊断：多人共用 Claude 账号导致卡死

**问题描述**：多个用户使用同一个 Claude 账号时，代理服务长时间转圈不返回。

**根因分析**：

通过并发两个 haiku agent 分析代码和上游历史，定位到根因在 `internal/api/protocol_multiplexer.go:77`：

- `acceptMuxConnections` 的 accept 循环中，`reader.Peek(1)` 是同步调用
- 如果某个 TCP 连接建立后不发送数据（空闲连接），`Peek(1)` 会永久阻塞
- 整个 accept 循环卡住，所有后续连接都无法被接受
- 多人并发时，空闲/慢连接的概率大幅上升，一个卡住就全部卡住

**上游修复**：commit `28dfcae3`（"fix(api): prevent idle TCP connections from blocking the accept loop"）
- 将 TLS 握手和 `Peek(1)` 移到独立 goroutine (`go s.routeMuxConnection`)
- 每个连接设置 10 秒 `SetReadDeadline`
- 路由成功后清除 deadline

**状态**：上游已修复，存在于 `upstream/main`，但当前 `new` 分支未包含。

---

### Rebase 到上游最新代码

**操作**：将 `new` 分支 rebase 到 `upstream/main`

- 当前分支落后 upstream/main 13 个 commit，领先 3 个 commit
- 执行 `git rebase upstream/main`
- 遇到 1 个冲突：`internal/runtime/executor/helps/usage_helpers.go`
  - 冲突原因：上游将解析逻辑提取成 `parseClaudeUsageNode` 共享函数，我们的 commit 在内联代码中添加了 cached tokens 修正
  - 解决方式：保留上游的函数调用（`return parseClaudeUsageNode(usageNode)`），git 自动将我们的 cached tokens 逻辑合入共享函数（第二个 hunk 无冲突）
- Rebase 成功，编译通过

**测试结果**：
- `go build ./cmd/server/` — 通过
- `go vet ./...` — 通过
- `go test ./...` — 3 个测试失败，经验证与 `upstream/main` 上完全一致的失败，非 rebase 引入：
  - `TestCodexFreeModelsExcludeGPT55`
  - `TestEnsureAccessToken_WarmTokenLoadsCreditsHint`
  - `TestUpdateAntigravityCreditsBalance_LoadCodeAssistUserAgent`

---

### Push 到远端

- 删除 `origin` remote（仓库 `router-for-me/CLIProxyAPIPlus.git` 已不存在）
- Force push `new` 到 `ironbox/new` 和 `ironbox/new-v7`

---

### Push backup 分支

将 `backup/new-pre-origin-rebase-20260408-214748` 推送到 `ironbox`。该分支保留了原 CPAPlus 删库前的代码以及多项性能优化，作为历史存档。

---

### TDD 修复 Claude usage 计算

**问题**：`parseClaudeUsageNode` 在 `cache_read_input_tokens > 0` 时丢弃 `cache_creation_input_tokens`，导致 `CachedTokens`、`InputTokens`、`TotalTokens` 在两类 cache 同时存在时漏算。

**TDD 流程**：
1. 写 3 个新测试覆盖缺失场景（仅 cache_creation / 两者同时 / 启发式 InputAlreadyIncludesBoth），跑测试确认 red
2. 修复：`totalCachedTokens = cacheRead + cacheCreation`，`CachedTokens` 与启发式判断都基于二者之和
3. 跑测试确认 green，全部 6 个 Claude usage 测试通过

**文件**：[usage_helpers.go:376-394](internal/runtime/executor/helps/usage_helpers.go#L376-L394)

---

### 创建项目文档体系

创建 `env.md`、`journal.md`、`plan.md`，更新 `CLAUDE.md` 作为项目索引。

---

## 2026-06-21

### Merge 上游 172 个 commit（merge 而非 rebase）

**背景**：`new` 落后 `upstream/main` 172 个 commit（merge-base `44ea9abc`，上游已 rebase 历史，故计数偏大）。本地真正的定制只有 7 个非 merge commit（Fable 5、Claude usage cache tokens、request log、CI workflow、项目文档）。

**操作**：先建备份分支 `backup/new-pre-rebase-20260621-144654`，再 `git merge upstream/main`。

**上游主要变更**：移除 gemini-cli provider 与 amp 集成（`feat!: remove amp`）、新增 pluginstore 子系统 / videos handlers / websockets executors、translator 大量重构（Gemini 视频 URL、tool/call ID、cache token 明细）、management 日志游标与基于快照的 reload。

**冲突解决（原则：保留双方优化，冲突时取较好的 = 上游 canonical/重构版本）**：
1. `model_definitions.go`：两侧各自新增常量/builtin 函数，全部保留（本地 `claudeBuiltinFableModelInfo` + 上游 `codexBuiltinImage15ModelInfo`、`normalizeAntigravityCapabilityModelID`）；Fable 5 builtin 元数据对齐到上游 `models.json` 的 canonical 值（created `1781049600`、官方 description）。
2. `models.json`：采用上游 canonical Fable 5 元数据。
3. `model_updater.go`：`mergeModelCatalog` 删除 `GeminiCLI` 字段（上游移除了 gemini-cli provider 及 `staticModelsJSON.GeminiCLI`），否则编译失败。
4. `usage_helpers_test.go`：保留本地 Claude cache-token 测试（fork 优化）；上游重构后 `ParseGeminiCLI*` 函数消失，将可平滑映射的测试重指向 `ParseGeminiUsage`/`ParseGeminiStreamUsage`；删除 traffic-only guard 测试（上游移除了 `hasGeminiFamilyUsageTokenFields`，行为不再保证）。

**完整 TDD**：新增 `model_definitions_fable_test.go`——验证 `WithClaudeBuiltins` 始终注入 Fable 5（fork 优化的保障），并强制 builtin 与 `models.json` 元数据一致。已做 red→green 验证（临时把 builtin `created` 改回 `1781193600` → 一致性测试 red；恢复 → green）。

**验证**：
- `go build ./cmd/server` — 通过
- `go vet ./...` — 仅剩 pre-existing 警告（`request_logger.go` WriteTo 签名、pluginhost、sdk handlers，已确认上游与 fork 备份均存在）
- `go test ./...` — 唯一失败的 4 个测试（Codex image-edit ×2、XAI reasoning-effort、Gemini reasoning-signature）经独立 worktree 验证在 clean `upstream/main` 上同样失败，非本次 merge 引入

**Commit**：`9a50fd6a merge: sync with upstream/main (172 commits)`（parents `f4ffea6d` + `369e560f`）。
