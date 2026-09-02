# VOA Learning Backend

VOA Learning English 客户端的 Go 后台。服务负责采集、规范化、审核和代理上游内容，向客户端提供稳定的 v1 API。

## 本地运行

```bash
brew services start postgresql@16
brew services start redis
brew services start minio
migrate -path db/migrations -database "$VOA_DATABASE_URL" up
./scripts/dev.sh
```

如果需要关闭终端后继续为模拟器提供 API，使用由 macOS `launchd` 监督并自动重启的后台服务模式：

```bash
./scripts/dev-service.sh start
./scripts/dev-service.sh status
./scripts/dev-service.sh stop
```

运行日志和生成的服务文件保存在被 Git 忽略的 `tmp/dev-service/` 中。

首次投递一个已审核页面：

```bash
go run ./cmd/ingest https://learningenglish.voanews.com/a/6654462.html beginning
```

可选的第二个参数保留分级页上下文，支持 `beginning`、`intermediate`、`advanced`。

质量检查：

```bash
make check
VOA_INTEGRATION_DATABASE_URL="$VOA_DATABASE_URL" make integration-test
```

架构、API、采集与合规说明见 [`docs/`](docs/)。

## 测试服务器 Docker 部署

完成后端开发和测试后，部署到默认测试服务器 `oldj@10.10.1.4`：

```bash
./scripts/deploy-docker.sh
```

脚本同步代码、在服务器上构建镜像、执行数据库迁移、启动 API 与 Worker，并等待远端健康检查通过。可用 `VOA_DEPLOY_HOST`、`VOA_DEPLOY_USER` 和 `VOA_DEPLOY_DIR` 覆盖默认目标。
