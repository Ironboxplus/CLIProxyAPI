# env.md

本地与远程的硬件、软件环境记录。与环境配置相关的内容均记录在此。

## 本地开发环境

| 项目 | 值 |
|------|-----|
| OS | Windows 11 Pro for Workstations 10.0.26100 (amd64) |
| Shell | Git Bash (MINGW64) |
| Go | 1.26.0 windows/amd64 |
| Git | 2.45.1.windows.1 |
| Node.js | v22.19.0 |
| Python | 3.13.7 |
| Docker | 未安装 |

### 路径

| 路径 | 说明 |
|------|------|
| `E:\Go\aiproxy\CPA\CLIProxyAPIPlus` | 项目根目录 |
| `C:\Users\Arc\go` | GOPATH |
| `~/.cli-proxy-api` | 默认 auth-dir（token 文件存放） |

### Git Remotes

| Remote | URL | 用途 |
|--------|-----|------|
| `ironbox` | https://github.com/Ironboxplus/CLIProxyAPI.git | 我们的 fork，push 目标 |
| `upstream` | https://github.com/router-for-me/CLIProxyAPI.git | 上游主线仓库 |

> `origin` 已删除（2026-05-12），原指向 `router-for-me/CLIProxyAPIPlus.git`（仓库已不存在）。

### 分支策略

| 分支 | 说明 |
|------|------|
| `new` | 本地主开发分支 |
| `ironbox/new-v7` | 远端发布分支，与 `new` 保持同步 |
| `upstream/main` | 上游主线，定期 rebase |
| `backup/*` | merge/rebase 前的备份，命名格式 `backup/new-pre-*-YYYYMMDD-HHMMSS`；最新备份：`backup/new-pre-rebase-20260621-144654` |

> 2026-06-21 起改用 merge（而非 rebase）同步上游，保留双方提交历史。推送方式为 fast-forward，无需 force push。

## 远程环境

（待补充：部署服务器信息、显卡数量、调用方式等）
