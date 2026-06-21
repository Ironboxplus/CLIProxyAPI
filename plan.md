# plan.md

本文件内容写入后不可修改，应以 plan 为目标完成任务。

---

## Plan 1: Rebase 并同步上游修复（2026-05-12）

**目标**：将 `new` 分支 rebase 到 `upstream/main`，获取 idle TCP 连接阻塞的关键修复。

**步骤**：
1. 分析根因：多人共用 Claude 账号卡死的问题
2. 检查上游是否已修复
3. Rebase `new` 到 `upstream/main`
4. 解决冲突
5. Build + vet + 全量测试验证
6. Force push 到 `ironbox/new` 和 `ironbox/new-v7`

**状态**：已完成

---

## Plan 2: Merge 上游同步（2026-06-21）

**目标**：将 `upstream/main`（172 个新提交）merge 进本地 `new` 分支，获取上游 pluginstore、videos handlers、websockets executors、translator 重构等重要更新，同时保留本地 Fable 5 等定制改动。

**步骤**：

1. 创建安全备份分支 `backup/new-pre-rebase-20260621-144654`
2. 执行 `git merge upstream/main`（选用 merge 而非 rebase，保留双方历史）
3. 解决 3 处冲突：
   - `internal/registry/model_definitions.go`：保留双方新增常量/builtins，Fable 5 元数据对齐上游 canonical
   - `internal/registry/models/models.json`：采用上游 canonical Fable 5 元数据
   - `internal/runtime/executor/helps/usage_helpers_test.go`：保留本地 Claude cache-token 测试；将孤立的 `ParseGeminiCLI*` 测试重定向到上游 `ParseGeminiUsage`/`ParseGeminiStreamUsage`；移除仅流量保护测试
4. 修复 build 问题：从 `mergeModelCatalog` 移除已被上游删除的 `GeminiCLI` 字段
5. 补充 TDD：新增 `internal/registry/model_definitions_fable_test.go`（Fable 5 builtin 存在性 + builtin/models.json 元数据一致性，red→green 验证）
6. 验证：`go build` 通过；`go vet` 仅有预存警告；`go test ./...` 仅 4 个预存失败（Codex image-edit ×2、XAI reasoning-effort、Gemini reasoning-signature，均已确认为上游 clean main 同等失败）
7. Push `new` → `ironbox/new` 和 `ironbox/new-v7`（fast-forward，无需 force）

**状态**：已完成
