# 故事创作部署说明

本文档说明团队当前服务器的部署、重启、状态检查和回退方式。当前采用单环境、裸机部署，不使用 Docker，不区分测试数据库和生产数据库。

## 当前架构

```text
CodeUp origin/custom/main
        |
        | make deploy
        v
39.97.235.150
  Caddy 80/443/3000
    |- 静态前端: /opt/open-ai-canvas-dev/web/dist
    `- /api/* -> 127.0.0.1:8080

  story-creation.service
    `- Go 后端: /opt/open-ai-canvas-dev/.local/current/backend

  PostgreSQL / Redis
    `- 独立 systemd 服务，应用部署不会重启或修改它们
```

线上地址：<https://canvas.marsvis.com>

## 分支与上线来源

```text
main
  只同步 GitHub upstream/main，不做业务开发，不部署。

custom/main
  团队开发主线，也是服务器唯一默认部署来源。

feat/* / fix/*
  从 custom/main 创建，完成后合并回 custom/main。
```

服务器执行 `make deploy` 时自动拉取 `origin/custom/main` 最新提交，不会从 `main` 或 GitHub upstream 部署。

## 日常部署

登录服务器并执行：

```bash
ssh root@39.97.235.150
cd /opt/open-ai-canvas-dev
make deploy
```

当前 `make deploy` 是前后端全量部署：同时构建前端和后端，并重启后端。它暂未根据文件差异自动区分“仅前端”或“仅后端”。

完整流程：

1. 获取部署锁，防止两个人同时部署。
2. 拉取 `origin/custom/main` 最新提交，并快进服务器工作树。
3. 在 `.local/build.*` 临时目录安装前端依赖、构建前端、编译 Linux 后端二进制。
4. 构建全部成功后创建新的 `.local/releases/<commit-time>`。
5. 原子切换 `.local/current`，让前端和后端使用同一个提交的产物。
6. 通过 systemd 重启 `story-creation.service`。
7. 检查 systemd、本机后端、Caddy API 和首页。
8. 全部成功后删除旧 release，服务器只保留当前成功产物。
9. 切换或健康检查失败时，恢复本次部署前的产物和 systemd unit。

构建发生在临时目录中，因此构建失败不会覆盖当前线上版本。部署脚本不会执行 `git clean`，也不会修改数据库、Redis、OSS 或 `.local/server.env`。

## 常用命令

在 `/opt/open-ai-canvas-dev` 下执行：

```bash
make deploy
```

拉取并部署 `origin/custom/main` 最新提交。

```bash
make deploy REF=<commit-sha>
```

部署 `custom/main` 历史中的指定提交。

```bash
make rollback REF=<commit-sha>
```

按指定 Git 提交重新构建并回退。服务器不长期保存旧 release，历史版本以 Git 提交为准。

```bash
make status
```

显示当前部署提交、systemd 状态、本机后端健康检查和 Caddy API 健康检查。

```bash
make logs
```

持续查看 `story-creation.service` 最近 200 行日志。

```bash
make restart
```

只重启现有后端，不拉代码、不重新构建前端或后端。

开发者合并前可以在本地执行：

```bash
make check
```

该命令执行完整 Go 测试、前端依赖锁校验、前端测试和生产构建。

## systemd 托管

后端服务名：`story-creation.service`。

```bash
systemctl status story-creation
systemctl restart story-creation
journalctl -u story-creation -f
```

systemd 已设置开机启动和异常退出自动重启。后端不再通过 tmux 或 `go run` 运行。

服务启动参数来自：

```text
/opt/open-ai-canvas-dev/.local/server.env
```

该文件包含数据库、Redis、CORS、OSS和部署通知等私有配置，权限为 `0600`，不得提交到 Git 或发送到群聊。

## 重启与运行任务

后端收到 SIGTERM 后会：

1. 立即停止接收新的 HTTP 连接。
2. 停止领取新的生成任务。
3. 等待当前进程已经领取的任务完成。
4. 最多等待 10 分钟，然后退出并启动新版本。

排空时间可以通过 `CANVAS_SHUTDOWN_TIMEOUT_SECONDS` 缩短，超过 600 秒的值按 600 秒处理。systemd 最多等待 11 分钟。

如果任务在排空超时后仍未结束，数据库中的运行状态和租约会保留。新进程会在租约过期后重新接管；已经保存上游任务 ID 的异步视频或音频任务会继续查询原任务。同步生图、文本流或尚未保存上游任务 ID 的请求不能保证从原断点恢复。

因此当前方案属于可恢复重启，不是严格零停机。重要上线前可以先在管理端确认没有关键运行任务。

## 飞书通知

服务器环境文件配置 `FEISHU_DEPLOY_WEBHOOK` 后，部署脚本会发送：

- 开始部署：主机、分支、提交。
- 部署成功：主机、分支、提交、systemd 服务名。
- 部署失败：失败提交，以及是否执行回退。

通知失败只会写警告，不会让一次原本成功的部署回滚。Webhook 只存在服务器私有环境文件中。

## 健康检查

公网检查：

```bash
curl -fsS https://canvas.marsvis.com/api/health
```

正常结果：

```json
{"code":0,"data":{"status":"ok"},"msg":"ok"}
```

服务器本机检查：

```bash
curl -fsS http://127.0.0.1:8080/api/health
curl -fsS http://127.0.0.1:3000/api/health
curl -fsS http://127.0.0.1:3000/
```

健康接口只能证明 HTTP 服务可用，不能代替登录、模型生成、OSS上传和 SSE 流式响应的业务验证。

## 故障处理

部署失败后先执行：

```bash
make status
make logs
```

如果新版本不可用，选择 `custom/main` 中已知正常的提交：

```bash
git log --oneline -20
make rollback REF=<正常提交>
```

如果 `make deploy` 提示已有部署正在运行，不要删除锁文件。先确认是否存在正在运行的部署进程，避免两个发布同时切换产物。

数据库备份和数据库回滚不属于当前部署脚本职责。涉及数据库结构的修改上线前必须单独评估新旧后端兼容性。
