# English Learning Backend

面向多数据源英语学习客户端的 Go 后台。服务负责接收 CMS 已审核内容、管理在线目录与媒体，并向客户端提供稳定的 v1 API。VOA Learning English 是当前的数据源连接器之一。

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

CMS 审核后的双环境发布协议见 [`docs/cms-publication.md`](docs/cms-publication.md)。测试和生产后台各自保存内容与音频，生产环境不需要连接内网内容库。

账号学习数据除收藏、播放进度、单词本和复述录音外，还保存幂等的语法练习会话。匿名客户端可先在本机记录，登录时通过 migration payload 合并；登录后使用 `/api/v1/me/grammar-attempts` 跨设备同步，为后续个性化推荐提供真实学习行为。

登录用户可通过 `/api/v1/me/home` 获取个性化首页：未完成内容进入继续学习，其他内容按质量、发布时间、常用难度、单词本行为和语法练习薄弱度排序；已完成内容不会重新进入推荐位。接口不可用时客户端自动回退到匿名公共首页。

## 测试服务器 Docker 部署

完成后端开发和测试后，部署到默认测试服务器 `oldj@10.10.1.4`：

```bash
./scripts/deploy-docker.sh
```

脚本同步代码、在服务器上构建镜像、执行数据库迁移、启动 API 与 Worker，并等待远端健康检查通过。可用 `LEARNING_DEPLOY_HOST`、`LEARNING_DEPLOY_USER` 和 `LEARNING_DEPLOY_DIR` 覆盖默认目标；原 `VOA_*` 部署变量暂时作为兼容别名保留。

远端 `.env` 至少设置测试环境发布令牌与媒体数据盘：

```dotenv
VOA_ENV=test
VOA_CMS_PUBLISH_TOKEN=<至少 32 个字符的随机令牌>
VOA_MINIO_DATA_PATH=/mnt/download/voa-learning-backend/minio
```

Worker 每 24 小时读取 VOA 官方 sitemap 索引并增量发现完整历史内容，最新 sitemap 每 30 分钟检查一次。正文抓取默认全局间隔为 `1500ms`，可通过 `VOA_CRAWL_DELAY` 调整。所有 URL 均去重且已成功抓取的内容不会重复入队。
