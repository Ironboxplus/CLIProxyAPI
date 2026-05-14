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
