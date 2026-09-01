# 后台技术架构设计（v0.1）

## 1. 目标与范围

后台作为 VOA Learning English 与 iOS App 之间的数据代理层，负责：

- 发现分级栏目、系列、课程、文章与播客条目；
- 提取正文、词汇/学习区块、图片、音频、视频、讲义与测验元数据；
- 保存原始快照并规范化为稳定领域模型；
- 内容校验、审核、上下架、修订和删除传播；
- 为 iOS 提供分页、搜索、增量同步、播放地址和学习进度 API；
- 对源站限速、缓存、熔断和可观测，避免客户端直接访问源站。

首发采用匿名优先：用户无需注册即可浏览、搜索和播放；匿名设备可保存本机进度。自动翻译、AI 生成内容、完整 CMS、社交评论和付费系统不在首期范围。账号与云端学习进度在 API v1 后半阶段加入，通过 Sign in with Apple 将匿名数据合并到账号。

## 2. 已观察到的上游内容

源站首页按 Beginning、Intermediate、Advanced、US History 等入口组织内容，并包含系列课程、新闻学习文章、Learning English Podcast、图片、MP3/播客、带多清晰度直链的视频、课程讲义和测验。页面 URL 大致存在 `/a/{id}.html` 文章、`/z/{id}` 列表、`/p/{id}.html` 专题页等形态。

这些是初步观察，不应作为长期契约；采集器需要用 fixture 和告警抵抗 DOM 变化。

## 3. 推荐技术栈

| 层 | 选择 | 理由 |
|---|---|---|
| API/业务服务 | Go 1.24+、`net/http` + chi | 依赖轻、性能稳定、部署简单；以 OpenAPI 保持 iOS 契约稳定 |
| 数据访问 | pgx + sqlc | 使用显式 SQL，并生成类型安全的 Go 查询代码；迁移使用 golang-migrate |
| 采集 Worker | Go + goquery/Colly | 静态 HTML 优先；仅在确有动态渲染需求时用 chromedp |
| 数据库 | PostgreSQL 16+ | 关系模型、JSONB、全文检索、事务和成熟运维 |
| 队列/缓存 | Redis + Asynq | Go 原生任务队列，支持去重、重试、调度、超时与短期缓存 |
| 对象存储 | S3 兼容存储 | 原始 HTML、解析 fixture、允许缓存的图片/文档/媒体 |
| API 网关/CDN | Cloudflare 或等价产品 | TLS、WAF、限流、缓存、媒体 Range 请求 |
| 可观测 | OpenTelemetry + Sentry + Prometheus/Grafana | 追踪采集链路、解析失败、API 延迟和队列积压 |
| 部署 | Docker；初期单区托管容器 | 保持可迁移，避免 MVP 过早引入 Kubernetes |

建议采用 Go 模块化单体加独立 Worker，而非一开始拆微服务。API 与 Worker 编译为不同二进制，共享 `internal` 领域代码，但可以独立部署和扩缩容。

## 4. 逻辑架构

```text
VOA website / feeds
        |
        v
 Discovery -> Fetch -> Raw Snapshot -> Parse -> Validate -> Normalize
                                   |                    |
                                   v                    v
                              Object Storage       PostgreSQL
                                                        |
Admin review / policy ----------------------------> Publish state
                                                        |
iOS App -> CDN/API Gateway -> Public API -> Redis ------+
                                  |
                                  +-> Media resolver/proxy -> VOA/CDN or owned cache
```

### 4.1 模块边界

- `source-registry`：允许访问的主机、入口 URL、频率、条款记录和 kill switch。
- `discovery`：从栏目、分页、feed 或 sitemap 发现 canonical URL。
- `fetcher`：条件请求、超时、限速、重试、响应大小限制、快照保存。
- `parser`：按页面类型和版本提取 DTO，不直接写发布表。
- `catalog`：栏目、系列、内容、资源和关联的领域逻辑。
- `publishing`：校验、审核状态、定时发布、下架及客户端增量事件。
- `media`：媒体探测、允许列表、签名 URL、Range 代理和缓存策略。
- `public-api`：移动端 BFF，返回紧凑稳定的 JSON。
- `admin-api`：内部审核、重抓、对比、上下架；必须独立鉴权和网络策略。
- `identity/progress`：匿名设备迁移、Sign in with Apple、收藏和学习进度。

## 5. 核心处理流程

### 5.1 采集与发布

1. Scheduler 为入口生成 discovery job。
2. Discovery 仅接受源注册表允许的域名与路径，写入 `source_items`。
3. Fetcher 使用固定 User-Agent、全局/主机级限速和条件请求获取页面。
4. 原始响应写对象存储；数据库记录状态、ETag、Last-Modified、SHA-256。
5. Parser 生成带 parser version 的候选记录和资源列表。
6. Validator 检查标题、发布日期、正文、资源 URL、重复内容和异常变化。
7. Normalizer 在事务中 upsert 领域数据，生成 revision。
8. 符合自动发布规则或人工审核通过后，写 publish event 并清 CDN 缓存。
9. 连续失败或结构异常时保留旧版本，不把空内容覆盖到线上。

### 5.2 删除与修订

- 上游 404/410 不立即物理删除；进入 `suspected_removed`，多次确认后下架。
- 标题、正文或媒体变化产生 revision，可比较、回滚和审计。
- API 使用 tombstone 事件通知已同步客户端删除或隐藏本地条目。

## 6. 媒体代理策略

### 6.1 首期决策

首期默认采用 `stream_proxy`：iOS 只访问 `/api/v1/media/{assetId}`，后台从已登记的 VOA 媒体源读取并向客户端流式转发，不把完整音视频永久写入我方对象存储。代理必须支持 `HEAD`、`Range` 和断点续播；允许 CDN 做短时传输缓存时，也不得把它视为永久媒体库。

这仍属于媒体再传输行为，上线前必须确认许可。若许可不允许代理，则把对应 asset 切换为 `redirect` 或只提供源页面链接；若未来得到明确的缓存与离线授权，再启用 `managed_cache`。

### 6.2 可配置交付模式

按授权结果配置三种策略：

1. `stream_proxy`（首期默认）：后台/CDN 代转并支持 `Range`、`HEAD`、内容类型与长度透传；满足统一代理诉求，但承担带宽成本。
2. `redirect`（合规降级）：后台解析和校验源 URL，返回短时 302；成本最低，但客户端最终连接上游。
3. `managed_cache`（需明确授权）：允许缓存的资源写入对象存储并经 CDN 分发；稳定性最好，也可作为离线下载的基础。

媒体端点必须阻止任意 URL 参数，资源只能按数据库 ID 解析，以避免 SSRF/开放代理。允许的上游域名、协议、端口和 MIME 类型均采用 allowlist；限制响应大小和重定向次数。

## 7. API 与同步

- 基础路径：`/api/v1`；JSON 使用 camelCase，时间为 UTC ISO 8601。
- 游标分页，不使用页码，保证源内容更新时列表稳定。
- `ETag`/`If-None-Match` 和合理 `Cache-Control` 降低移动端流量。
- `GET /sync?cursor=...` 返回 upsert 与 tombstone，支持离线数据库增量同步。
- 首页、分类、搜索、详情和媒体播放均可匿名使用。首期播放不依赖登录。
- 本机进度由 iOS 保存；云端进度需要 access token，登录时用一次性 migration token 合并匿名数据。
- 错误统一为 `{ error: { code, message, requestId, details? } }`。

详情见 `api-v1.md`。

## 8. 安全与合规

- 正式采集前保存并审核 robots.txt、服务条款和内容许可；设置一键停采/下架。
- 展示明确的来源与 canonical URL，不暗示 VOA 官方授权或隶属关系。
- 管理端使用 OIDC + RBAC + MFA；所有发布动作写审计日志。
- HTML 只保存清洗后的结构化 block；禁止把未清洗 HTML 传给 App。
- 出站请求防 SSRF，入站 API 做 schema 校验、速率限制、WAF 和密钥轮换。
- 用户数据最小化；进度与账号分表，设计账号删除和数据导出流程。
- 日志不得记录 token、完整 IP 或用户正文；对象存储默认私有和加密。

## 9. 可用性与 SLO（MVP）

- Public API 月可用性目标：99.9%。
- 缓存命中时 p95 < 250 ms；数据库查询 p95 < 150 ms。
- 新内容发现目标：源站发布后 30 分钟内；不承诺实时。
- 解析异常率 > 5%、关键字段缺失或条目数骤降 30% 时暂停自动发布并告警。
- RPO 24 小时，RTO 4 小时；PostgreSQL 每日备份并定期验证恢复。

## 10. 仓库规划

```text
backend/
├── cmd/
│   ├── api/                 # public/admin HTTP API 入口
│   ├── worker/              # 采集与发布任务入口
│   └── migrate/             # 可选迁移入口
├── internal/
│   ├── catalog/             # 内容领域与用例
│   ├── ingestion/           # 调度、抓取、解析、校验
│   ├── media/               # 媒体解析与代理
│   ├── publishing/          # 审核、版本和增量事件
│   ├── progress/            # 用户学习进度
│   ├── httpapi/             # handler、middleware、DTO
│   ├── repository/          # pgx/sqlc repository
│   └── platform/            # Redis、S3、日志、追踪、配置
├── db/
│   ├── migrations/
│   ├── queries/             # sqlc SQL
│   └── sqlc.yaml
├── api/openapi.yaml
├── fixtures/voa/
├── deployments/
├── go.mod
├── go.sum
└── Makefile
```

### 10.1 Go 工程约定

- 尽量使用标准库；HTTP 路由采用 chi，不引入重量级 Web 框架。
- 依赖由构造函数显式注入，不使用全局 service locator。
- OpenAPI 是移动端契约源；可用 oapi-codegen 生成服务端类型和接口骨架。
- 数据库事务由用例层控制，repository 不隐式开启嵌套事务。
- 错误使用 `errors.Is/As` 分类，在 HTTP 边界映射为稳定错误码。
- 所有网络调用传递 `context.Context`，设置超时并支持优雅关闭。
- 日志使用 `log/slog` 输出结构化 JSON；追踪使用 OpenTelemetry。
- CI 至少运行 `go test ./...`、`go vet ./...`、golangci-lint、迁移验证和 OpenAPI breaking-change 检查。

## 11. 里程碑

### M0：合规与采集 Spike（3–5 天）

- 确认许可、robots 和品牌边界。
- 保存 10–20 个不同页面 fixture，验证列表、详情、音视频、讲义解析。
- 验证 feed/sitemap 是否可作为优先发现渠道，HTML 抓取作为补充。

### M1：内容管道（1–2 周）

- 初始化 Go module 与 `cmd/api`、`cmd/worker`，建立配置和可观测基础设施。
- PostgreSQL schema、sqlc、对象存储、Asynq、抓取和版本化 parser。
- 内容校验、幂等 upsert、基本管理端查询和手工上下架。

### M2：移动 API（1 周）

- 首页、分类、搜索、详情、媒体解析、游标分页与增量同步。
- OpenAPI 契约测试、缓存、限流和基础监控。

### M3：账号与学习能力（1–2 周）

- Sign in with Apple、收藏、播放/阅读进度、跨设备同步和删除账号。

## 12. 待验证决策

- VOA 是否提供稳定官方 feed/API，以及允许的请求频率。
- 正文是否允许持久存储与再展示，音视频能否代理或缓存。
- 中国大陆目标网络环境、部署地域、域名与合规要求。
- App 是否需要中英双语、逐句字幕和词典。
- 是否能取得媒体代理、短时 CDN 缓存和后续离线下载的书面许可。
