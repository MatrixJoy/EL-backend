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

账号学习数据除收藏、播放进度、单词本和复述录音外，还保存幂等的语法练习会话。匿名客户端可先在本机记录，登录时通过 migration payload 合并；登录后使用 `/api/v1/me/grammar-attempts` 跨设备同步，为后续个性化推荐提供真实学习行为。单词本同时保存间隔复习阶段、到期时间和遗忘次数，客户端通过 `/api/v1/me/vocabulary/{entryId}/review` 按更新时间幂等同步。

复述录音可附带设备生成的英语转写、关键词覆盖和完成度反馈。反馈通过独立 JSON 接口按客户端更新时间合并，不需要后台读取或分析用户私有音频；录音与反馈仍随账号删除一起清理。

登录用户可通过 `/api/v1/me/home` 获取个性化首页：未完成内容进入继续学习，其他内容按质量、发布时间、常用难度、单词本行为和语法练习薄弱度排序；已完成内容不会重新进入推荐位。接口不可用时客户端自动回退到匿名公共首页。

所有文章摘要接口都会返回规范化的 `learningGoals`。客户端据此只展示已经随运营发布内容真正可用的听力、复述和语法入口；新数据源可以增加新的目标字符串而不破坏旧客户端。

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

## 生产服务器 Docker 部署

生产环境使用独立 PostgreSQL、Redis 和 API 容器，媒体与用户录音保存在腾讯云 COS；API 仅加入现有 `lighttech_net` 网关网络，不在公网暴露明文端口。首次部署会从 `/opt/lighttech_deploy/.env` 读取 `COS_SECRET_ID`、`COS_SECRET_KEY`、`COS_REGION` 与 `COS_BUCKET`，并生成权限为 `600` 的 `.env.production`。后续部署不会覆盖生产密钥。

```bash
./scripts/deploy-production.sh
```

脚本默认部署到 `oldj@106.53.192.46:36987` 的 `/opt/english-learning-backend`，为 `ela.wozdou.cn` 申请并续期 Let's Encrypt 证书，最后验证公网健康检查。可通过 `LEARNING_PRODUCTION_HOST`、`LEARNING_PRODUCTION_SSH_PORT`、`LEARNING_PRODUCTION_USER`、`LEARNING_PRODUCTION_DIR` 与 `LEARNING_TLS_EMAIL` 覆盖目标。

生产数据库每天生成一次自包含的 custom-format `pg_dump` 到远端 `backups/`，默认保留参数为 14 天。备份经过归档目录检查后才标记成功，使用私有文件权限，并通过 Docker 健康检查暴露长时间未成功的状态。异机备份仍需配置独立的复制策略；恢复/演练、私密反馈处理及首发验证见 [`docs/release-operations.md`](docs/release-operations.md)。

首次部署后用同一个可重复执行的导入器初始化生产词典：

```bash
LEARNING_DEPLOY_HOST=106.53.192.46 \
LEARNING_DEPLOY_SSH_PORT=36987 \
LEARNING_DEPLOY_DIR=/opt/english-learning-backend \
LEARNING_DEPLOY_COMPOSE_FILE=compose.production.yaml \
LEARNING_DEPLOY_ENV_FILE=.env.production \
./scripts/import-wordnet.sh
```
