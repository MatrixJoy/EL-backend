# 首发运行与验收

更新：2026-09-10。

## 已部署的服务能力

- `GET /health/ready` 在两秒内检查 PostgreSQL；失败返回 503，不泄漏连接信息。`/health/live` 只检查进程可响应。
- 账号快照 `/api/v1/me` 任意数据库查询失败都返回错误，不会把不完整快照当成空数据交给同步客户端；账号数据/身份响应禁止缓存。
- `POST /api/v1/auth/logout` 撤销当前 bearer session；原有 `DELETE /api/v1/me` 级联删除账号学习数据，并清理私有录音对象。
- `/` 产品介绍、`/privacy` 隐私说明、`/support` 使用帮助及私密反馈表单。反馈支持使用问题、隐私、内容版权三种类型，不要求登录。
- 反馈经过表单大小/字段校验及限流，写入 `support_requests` 后才返回带 UUID 的收件确认；内容不会显示在公开页面。

## 处理反馈

生产服务器上：

```sh
docker exec english-learning-api admin list-feedback
docker exec english-learning-api admin resolve-feedback <反馈 UUID>
```

测试服务器后台工程目录中：

```sh
docker compose exec -T api admin list-feedback
docker compose exec -T api admin resolve-feedback <反馈 UUID>
```

反馈可包含用户自行填写的联系方式，输出只给运营人员查看，不要贴到公开日志。`resolve-feedback` 记录处理时间，不发送邮件，也不表示已向用户回复。运营需处理回信与隐私/版权请求；当前没有自动通知接收方。

本轮在生产用不含个人数据的 `Launch verification` 消息验证了提交→入库→运营查询→标记处理流程；测试记录保持已处理状态。

## 备份与恢复

生产 `backup` 容器在数据库迁移成功后开始每日 `pg_dump --format=custom`。每份文件用 `pg_restore --list` 检查后才更名为成功备份，权限为 600；失败保留上一份成功文件，健康检查通过最后成功时间检测过期/中断。保留参数 `LEARNING_BACKUP_RETENTION_DAYS` 默认 14，按整日文件年龄清理。

本轮已将 `english_learning_20260909T171043Z.dump` 在本机独立数据库恢复，核对出 1 篇内容、207,227 条词典记录（当时 schema v13）。随后发布支持反馈表的 v14。备份复制到 `10.10.1.4:/mnt/download/english-learning-backups/production/`，两端 SHA-256 一致：

```text
812a156357126d16d638a8c5d1e6c9fa2884aac40e032c7ebd07c5be66963070
```

这是一份单次异机副本，**不代表已经配置持续异机备份**。公开运营前还需配置自动异机复制、保留/加密策略及告警接收方。恢复必须在隔离数据库完成校验，再执行迁移，并重新应用备份生成后的账号删除/内容下架决定；不要把旧快照直接恢复到在线生产库。

示例（仅对新建的验证数据库运行）：

```sh
createdb learning_restore_check
pg_restore --no-owner --no-privileges --exit-on-error --dbname=learning_restore_check <backup.dump>
migrate -path db/migrations -database '<验证数据库连接串>' up
```

## 发布验证

```sh
make check
VOA_INTEGRATION_DATABASE_URL='<独立测试数据库>' make integration-test
./scripts/deploy-docker.sh
./scripts/smoke-release.sh
./scripts/deploy-production.sh
./scripts/smoke-release.sh https://ela.wozdou.cn
```

`smoke-release.sh` 检查 readiness、匿名配置、非空首页、文章/时间轴、分类、搜索、词典、托管音频 Range/206、隐私/反馈页面与未登录账号隔离。它不批准或发布任何内容。测试仍使用 CMS 人工批准、手动发布的既定流程。

生产内容目前只有 1 篇（测试目录 6 篇），需要运营补齐首发选题并试听。数据源权利确认、持续异机备份、监控告警和正式运营信息仍属于上线门禁。Apple 账号相关能力另见 iOS 仓库首发清单。
