# VOA Learning Backend

VOA Learning English 客户端的 Go 后台。服务负责采集、规范化、审核和代理上游内容，向客户端提供稳定的 v1 API。

## 本地运行

```bash
brew services start postgresql@16
brew services start redis
brew services start minio
migrate -path db/migrations -database "$VOA_DATABASE_URL" up
go run ./cmd/worker
go run ./cmd/api
```

首次投递一个已审核页面：

```bash
go run ./cmd/ingest https://learningenglish.voanews.com/a/6654462.html
```

质量检查：

```bash
make check
VOA_INTEGRATION_DATABASE_URL="$VOA_DATABASE_URL" make integration-test
```

架构、API、采集与合规说明见 [`docs/`](docs/)。

