# 上游代码合并检查清单

适用场景：

- 当前仓库需要同步 `upstream`（`https://github.com/router-for-me/CLIProxyAPI.git`）的更新
- 当前仓库同时保留了本地的 Hugging Face（HF）部署定制
- 目标是在吸收上游功能/修复的同时，不把 HF 专属逻辑冲掉

本文档基于 2026-04-08 的一次真实合并过程整理，重点覆盖 HF 相关风险点。

## 一、合并前检查

### 1. 确认 remote

先确认 remote 配置，尤其要区分：

- `upstream`：上游官方仓库
- `origin`：你自己的 GitHub fork
- `hf-clipgit`：Hugging Face Space 仓库

建议命令：

```powershell
git remote -v
```

注意：

- 不要默认依赖 `git fetch --all`。
- 这个仓库里 `hf-clipgit` 可能在 fetch 时失败，所以真正要同步上游时，优先单独执行：

```powershell
git fetch upstream --prune
```

### 2. 检查本地工作区是否有未提交改动

建议命令：

```powershell
git status --short --branch
```

如果存在未提交改动，尤其是下面这些 HF 定制文件，先 `stash` 或单独提交：

- `deploy-hf.ps1`
- `deploy/hf-profile/Dockerfile`
- `internal/api/server.go`
- `internal/api/server_test.go`
- `internal/managementasset/updater.go`
- `internal/managementasset/patch_test.go`
- `test/hf_deploy_profile_guard_test.go`

推荐做法：

```powershell
git stash push -u -m "before-upstream-merge"
```

## 二、建议的合并流程

### 1. 获取上游更新

```powershell
git fetch upstream --prune
```

### 2. 查看上游 main 相对当前分支的改动范围

```powershell
git log --oneline HEAD..upstream/main
git diff --name-only HEAD..upstream/main
```

重点看是否触碰了 HF 相关共享文件。

### 3. 合并上游

```powershell
git merge --no-ff upstream/main
```

### 4. 合并完成后再恢复本地暂存改动

```powershell
git stash pop
```

如果 `stash pop` 再次引发冲突，不要直接选一边，按下文“高风险文件处理原则”手工合并。

## 三、高风险文件处理原则

以下文件在上游合并时最容易把 HF 定制冲掉，需要重点人工检查。

### A. `internal/api/server.go`

风险点：管理页返回逻辑

必须保留的行为：

- 读取 `management.html` 后调用 `managementasset.PatchManagementHTML(...)`
- 给 `/management.html` 增加禁缓存头：
  - `Cache-Control: no-store, no-cache, must-revalidate, max-age=0`
  - `Pragma: no-cache`
  - `Expires: 0`

不要接受会把逻辑改回 `c.File(filePath)` 的版本，否则会丢失：

- 运行时注入的 URL 归一化补丁
- 管理页刷新登录修复
- 禁缓存行为

当前应保留的关键路径：

- `internal/api/server.go`
- `internal/api/server_test.go`

### B. `internal/managementasset/updater.go`

风险点：管理页运行时补丁

必须保留的行为：

- 会去掉错误的 `/management.html`
- 会合并重复的 `/v0/management`
- 会规范化保存在 session/localStorage 里的 `apiBase`
- 会在 XHR/fetch 发请求前规范化错误 URL

这是修复以下问题的关键：

- 用户把连接地址填成 `https://.../management.html`
- 前端误请求 `/management.html/v0/management/...`
- 配置页出现 `404` 后退回空白默认态

### C. `deploy-hf.ps1`

风险点：HF 快照导出逻辑

必须保留的行为：

- 在普通 tracked files 之外，额外把 `static/management.html` 带进 HF 快照
- 在临时仓库里执行 `git add --force -- static/management.html`

原因：

- 仓库 `.gitignore` 里忽略了 `static/*`
- 如果不额外处理，HF 临时仓库里虽然有文件拷贝，但不会被提交进 Space 仓库

### D. `deploy/hf-profile/Dockerfile`

风险点：最终镜像静态资源

必须保留的行为：

- `COPY static/management.html /CLIProxyAPI/static/management.html`
- `ENV MANAGEMENT_STATIC_PATH=/CLIProxyAPI/static`

原因：

- 仅把文件推到 HF 仓库还不够
- 如果最终镜像没把这份资源复制进去，运行时依然可能出现 `/management.html` 404

### E. `deploy/hf-profile/start.sh`

风险点：HF 上 Qwen OAuth 的代理启动链路

必须保留的行为：

- 继续使用 mihomo
- 继续强制依赖 `CLASH_SUB_URL`
- 启动失败要 fail fast，不能静默降级

不要把 HF 路径改回旧的 xray/双代理路径。

## 四、这次实际遇到过的冲突类型

### 1. 存储层冲突

文件：

- `internal/store/gitstore.go`
- `internal/store/objectstore.go`
- `internal/store/postgresstore.go`

处理原则：

- `gitstore`：保留上游新的 branch/refspec push 逻辑
- `objectstore` / `postgresstore`：同时保留
  - `HydrateRuntimeStateFromMetadata(...)`
  - `ApplyCustomHeadersFromMetadata(...)`

原因：

- 前者负责运行时状态恢复
- 后者负责从元数据恢复自定义请求头
- 两者都不能丢

### 2. 管理页逻辑回退风险

上游可能会把管理页路由简化成直接返回文件；HF fork 不能直接照收。

只要看到下面这类变化，就要人工复核：

- `c.File(filePath)`
- 删除 `PatchManagementHTML(...)`
- 删除缓存响应头

## 五、合并后必须执行的验证

### 1. 最低要求：本地测试

建议最少运行：

```powershell
go test ./internal/store ./internal/managementasset ./internal/api ./sdk/auth ./sdk/cliproxy/auth ./test
```

如果只是改了管理页和 HF 部署逻辑，也至少跑：

```powershell
go test ./internal/managementasset ./internal/api ./test
```

### 2. HF 部署保护测试

```powershell
go test ./test -run TestHFDeployProfileGuard
```

这个测试要确保：

- HF 部署脚本仍然包含 `static/management.html`
- HF Dockerfile 仍然复制 `static/management.html`
- `MANAGEMENT_STATIC_PATH` 仍然存在

### 3. 文本差异检查

如果只是文档或脚本改动，也建议跑：

```powershell
git diff --check
```

## 六、部署到 HF 后的线上回归清单

### P0：管理页入口

浏览器直接访问：

```text
https://spongyicybulk-clipgit.hf.space/management.html
```

期望：

- 返回 `200`
- 页面能打开

注意：

- 不要用 `HEAD /management.html` 替代浏览器检查
- 以浏览器访问或普通 `GET` 为准

### P0：管理接口存活

```text
GET /v0/management/config
```

期望：

- 未带密钥时返回 `401`
- 带错误密钥时返回 `401` 或 `403`
- 不应再出现 `404`

### P0：登录连接地址回归

登录页只填根地址：

```text
https://spongyicybulk-clipgit.hf.space
```

不要填：

```text
https://spongyicybulk-clipgit.hf.space/management.html
```

同时要验证：

- 即使用户误填了带 `/management.html` 的地址，前端也能把请求归一化为 `/v0/management/...`
- 不再出现 `/management.html/v0/management/...`

### P1：配置页真实加载

进入：

```text
/management.html#/config
```

确认以下内容能正常显示：

- `auth-dir`
- `api-keys`
- 源码模式的 `config.yaml`

### P1：缓存行为

`GET /management.html` 的响应头应包含：

- `Cache-Control: no-store, no-cache, must-revalidate, max-age=0`
- `Pragma: no-cache`
- `Expires: 0`

### P2：健康检查

```text
GET /healthz
```

期望：

```json
{"status":"ok"}
```

## 七、推荐的长期策略

如果以后还会频繁同步上游，建议把 HF 定制当成一个明确的“本地 overlay 层”看待，而不是普通改动。

建议策略：

- 上游功能逻辑跟随 `upstream/main`
- HF 定制只集中维护在以下几个边界点：
  - `deploy-hf.ps1`
  - `deploy/hf-profile/Dockerfile`
  - `deploy/hf-profile/start.sh`
  - `internal/api/server.go`
  - `internal/managementasset/updater.go`
  - 对应测试文件

这样每次上游合并后，人工复查范围会稳定很多。

## 八、快速口令版

每次合并上游前，至少记住这几条：

- 先 `git fetch upstream --prune`
- 先 `git stash push -u`
- 合并后重点检查：
  - `internal/api/server.go`
  - `internal/managementasset/updater.go`
  - `deploy-hf.ps1`
  - `deploy/hf-profile/Dockerfile`
- 跑：

```powershell
go test ./internal/store ./internal/managementasset ./internal/api ./sdk/auth ./sdk/cliproxy/auth ./test
```

- 部署后用浏览器打开：

```text
https://spongyicybulk-clipgit.hf.space/management.html
```

