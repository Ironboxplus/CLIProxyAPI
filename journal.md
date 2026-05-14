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
