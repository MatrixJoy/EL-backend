# 后端开发状态

更新日期：2026-09-01

## M0：合规与采集 Spike — 已完成

- 归档 robots、sitemap 和官方内容使用结论。
- 建立 21 个真实页面 fixture manifest 和可重复采集脚本。
- 覆盖 sitemap、landing、listing、article、lesson、podcast 与媒体解析。
- 使用 JSON-LD 契约阻止文章 URL 被直播页或其他模板污染。
- `rights_status` 阻止 AP、Reuters、AFP 或未知权利内容自动发布。

生产上线仍需项目责任人执行的外部动作：通知 VOA 使用计划、配置投诉入口、完成产品名称和商标复核。这些不影响软件里程碑完成，但属于发布门禁。

## M1：内容管道 — 已完成

- Go API/Worker、PostgreSQL、Redis/Asynq、MinIO 和 sqlc。
- 定时 sitemap discovery、任务去重/重试、抓取、HTML snapshot、版本解析、事务 normalize、发布事件。
- 内容、资源、系列和分类幂等 upsert。
- 管理 CLI 支持待审核查询、手工发布及下架。
- 已通过真实 VOA URL → MinIO → PostgreSQL → published content 端到端验证。

## M2：移动 API — 已完成

- bootstrap、home、categories、contents、series、search、sync。
- 内容游标分页、增量 tombstone/event 模型、ETag/304 和 Cache-Control。
- 匿名 IP 限流与 Prometheus 文本指标。
- 媒体 GET/HEAD、Range/206、断点续播头透传和逐跳 SSRF allowlist。
- OpenAPI 3.1 由自动测试解析、校验并检查所有已实现路径。

## M3：账号与学习能力 — 已完成

- Apple identity JWT 签名、issuer、audience、expiry 和 nonce 验证；JWKS 缓存。
- identity token 摘要唯一记录，阻止同一 Apple 凭据重放。
- Apple subject 作为账号键，不使用邮箱识别用户。
- 随机 bearer session，仅存储 SHA-256 token hash。
- 首次登录事务化合并本机收藏和进度。
- 收藏、播放/阅读进度、跨设备增量同步和账号级联删除。
- 客户端时间冲突规则阻止旧进度覆盖新进度。

## 发布配置

以下值不能由源码生成，部署时必须配置：

- `VOA_APPLE_CLIENT_ID` 和 Apple Developer 中对应的 Sign in with Apple capability。
- 生产 PostgreSQL、Redis、S3 凭据与 TLS。
- API 域名、WAF/CDN、投诉邮箱和监控告警接收方。
- 向 VOA Learning English 告知使用计划的外部记录。
