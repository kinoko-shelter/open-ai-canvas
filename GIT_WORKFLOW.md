# Git 仓库与分支协作说明

本文档说明本项目二次开发时的 Git 远程仓库、分支、上游同步和服务器部署建议。

## 目标

本项目基于上游开源仓库二次开发。推荐把 CodeUp 作为团队主仓库，把 GitHub 上游作为只读更新来源。

核心原则：

- CodeUp 保存我们的所有开发分支、生产分支和提交记录。
- GitHub 上游只用于拉取新版本，不向上游推送。
- `main` 只同步上游，不直接做业务开发。
- `custom/main` 是我们的二次开发主线。

## 远程仓库

推荐 remote 配置：

```bash
origin    git@codeup.aliyun.com:marsvision/canvas.git
upstream  https://github.com/ddcat-ai/open-ai-canvas.git
github    个人 GitHub fork，可选保留
```

含义：

- `origin`：团队主仓库，默认 `pull` / `push` 都走这里。
- `upstream`：开源上游，只拉取，不推送。
- `github`：如果个人还想保留 GitHub fork，可以作为备份 remote。

初始化或调整 remote：

```bash
git remote rename origin github
git remote add origin git@codeup.aliyun.com:marsvision/canvas.git
git remote set-url --push upstream DISABLED
```

如果本地没有 `upstream`：

```bash
git remote add upstream https://github.com/ddcat-ai/open-ai-canvas.git
git remote set-url --push upstream DISABLED
```

## 分支约定

推荐分支结构：

```text
main
  只同步 upstream/main，不做业务开发，不直接修改。

custom/main
  我们的二次开发主分支，开发、测试、部署都以它为基准。

feat/xxx
  日常功能分支，从 custom/main 拉出，完成后合并回 custom/main。

fix/xxx
  修复分支，从 custom/main 拉出，完成后合并回 custom/main。

sync/upstream-YYYYMMDD
  合并上游时的临时分支，用来解决冲突和验证。

release/xxx
  如果以后需要更严格的上线节奏，可以从 custom/main 拉发布分支。
```

## `custom/main` 与 `main` 的合并原则

本项目二次开发后，`custom/main` 和 `main` 的关系必须按下面的原则理解：

- `main` 是上游基线分支，只代表当前同步到的开源版本。
- `custom/main` 是我们的长期主分支，代表 `main` 加上我们的二次开发差异。
- 合并目标不是让 `custom/main` 和 `main` 完全一致，而是把 `main` 的新增变化吸收到 `custom/main`，同时保留我们的定制。
- 已推送的 `custom/main` 不做 rebase，不改历史；上游同步统一使用 merge commit。

每一次把 `main` 合入 `custom/main` 前，都必须先过滤一遍差异：

```bash
git fetch origin

# main 有、custom/main 还没有的提交，也就是本次准备吸收的上游变化
git log --oneline custom/main..main

# custom/main 有、main 没有的提交，也就是必须保护的二开差异
git log --oneline main..custom/main

# 从共同祖先到 main 的文件变化，用来判断本次上游会碰哪些模块
git diff --name-status custom/main...main
```

判断原则：

- 只在 `main` 改、`custom/main` 没改的文件，正常吸收。
- 只在 `custom/main` 改、`main` 没改的文件，必须保留。
- 双方都改但不重叠的文件，允许 Git 自动合并，但仍要看 diff。
- 双方都改且重叠的文件，必须人工语义合并，不能简单选择 `ours` 或 `theirs`。
- `main` 删除、`custom/main` 修改或新增的文件，默认保留 `custom/main`，除非确认该二开功能已经废弃。
- `main` 重构了某个模块时，要把 `custom/main` 的业务语义迁移到新结构里，而不是直接覆盖。

冲突处理时的优先级：

1. 业务定制优先保留：品牌名、用户体系、密码能力、模型渠道、计费规则、OSS、部署脚本。
2. 上游通用修复优先吸收：协议兼容、provider 请求结构、资源处理、测试用例、安全修复。
3. 核心路径必须逐段看：任务状态、计费结算、资源存储、认证会话、模型调用。
4. 前端 UI 冲突按业务入口判断：保留我们的产品命名和功能入口，吸收上游结构或 bugfix。

合并完成后再做一次差异确认：

```bash
# 确认没有未解决冲突
git status --short

# 看本次 merge 实际带进来的改动
git diff --stat HEAD^1..HEAD

# 确认 custom/main 的二开提交仍然存在
git log --oneline main..HEAD --max-count=20
```

一句话规则：

> `custom/main` 永远是 `main` 加我们的定制差异。合并 `main` 时只吸收新增上游变化，不反向削掉 `custom/main` 的业务改动。

## 日常开发流程

开始开发前同步团队主线：

```bash
git checkout custom/main
git pull --rebase origin custom/main
```

创建功能分支：

```bash
git checkout -b feat/example
```

提交并推送：

```bash
git add .
git commit -m "feat: example"
git push -u origin feat/example
```

合并回 `custom/main` 前，先更新自己的分支：

```bash
git fetch origin
git rebase origin/custom/main
```

合并方式可以用 CodeUp 的合并请求，也可以本地合并：

```bash
git checkout custom/main
git pull --rebase origin custom/main
git merge --no-ff feat/example
git push origin custom/main
```

## 同步上游流程

同步上游分三步：

1. 先让本地 `main` 跟上游 `upstream/main` 对齐。
2. 按上一节规则过滤 `custom/main` 与 `main` 的差异。
3. 再把 `main` 合并进 `custom/main` 的临时同步分支。

操作步骤：

```bash
git fetch upstream
git fetch origin
```

更新 `main`：

```bash
git checkout main
git merge --ff-only upstream/main
git push origin main
```

合并前必须查看差异：

```bash
git log --oneline custom/main..main
git log --oneline main..custom/main
git diff --name-status custom/main...main
```

从 `custom/main` 创建同步分支：

```bash
git checkout custom/main
git pull --rebase origin custom/main
git checkout -b sync/upstream-YYYYMMDD
git merge --no-ff main
```

如果发生冲突：

```bash
# 手动解决冲突
git status
git add <resolved-files>
git commit
```

完成后进行构建和测试。确认没有问题再合回 `custom/main`：

```bash
git checkout custom/main
git merge --no-ff sync/upstream-YYYYMMDD
git push origin custom/main
```

不要直接用上游覆盖 `custom/main`。临时同步分支可以隔离风险，方便冲突处理和回滚。

## 冲突处理建议

开启 Git 的冲突记忆：

```bash
git config rerere.enabled true
```

`rerere` 会记住之前解决冲突的方式。后续反复合并上游时，如果遇到类似冲突，Git 可以自动复用之前的解决结果。

处理冲突时优先级：

1. 保留我们的业务配置、模型接入、OSS、部署相关改动。
2. 保留上游的安全修复和通用 bugfix。
3. 对 UI 和业务流程冲突，先在 `sync/upstream-YYYYMMDD` 验证，不要直接污染 `custom/main`。
4. 合并完成后必须跑构建，必要时做浏览器验证。

## 服务器部署建议

推荐最终让服务器直接从 CodeUp 拉代码。

服务器需要具备：

- CodeUp SSH 权限。
- `custom/main` 分支拉取权限。
- 项目运行所需环境变量和本地服务配置。

部署流程：

```bash
cd /opt/open-ai-canvas-dev
git fetch origin
git checkout custom/main
git pull --ff-only origin custom/main

cd web
bun run build
```

当前服务结构建议保持：

```text
Caddy 80/443/3000
  -> 静态前端 dist
  -> /api 反代到 127.0.0.1:8080

Go 后端
  -> 127.0.0.1:8080
```

如果服务器尚未加入 Git 仓库，可以先备份当前目录，再 clone CodeUp：

```bash
mv /opt/open-ai-canvas-dev /opt/open-ai-canvas-dev.bak.$(date +%Y%m%d%H%M%S)
git clone git@codeup.aliyun.com:marsvision/canvas.git /opt/open-ai-canvas-dev
cd /opt/open-ai-canvas-dev
git checkout custom/main
```

注意：`.local/server.env`、数据库密码、OSS 密钥等环境配置不能提交到 Git，需要从备份目录迁回或重新配置。

## 推荐上线节奏

小团队开发阶段可以直接部署 `custom/main`。

如果后续进入稳定生产，建议增加发布分支：

```text
custom/main
  日常开发主线

release/YYYYMMDD
  本次准备上线的稳定分支

hotfix/xxx
  生产紧急修复分支
```

生产部署从 `release/*` 或打 tag 的提交进行：

```bash
git tag v2026.08.11-1
git push origin v2026.08.11-1
```

## 建议的保护规则

在 CodeUp 上建议设置：

- `main`：禁止直接 push，只允许同步上游后合并。
- `custom/main`：禁止 force push，建议通过合并请求合并。
- `release/*`：禁止 force push。
- 所有分支：禁止提交 `.env`、密钥、数据库密码、API key。

## 常用检查命令

查看 remote：

```bash
git remote -v
```

查看当前分支：

```bash
git branch --show-current
```

查看本地改动：

```bash
git status --short
```

查看上游是否有更新：

```bash
git fetch upstream
git log --oneline main..upstream/main
```

查看我们的二开分支相对上游差异：

```bash
git log --oneline main..custom/main
git diff main...custom/main
```
