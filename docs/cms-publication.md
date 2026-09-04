# CMS 发布接入

后台提供 `POST /internal/v1/publications` 给内网 CMS 使用。该接口不是 App 公共 API，只有配置 `VOA_CMS_PUBLISH_TOKEN` 后才会挂载。

请求使用 Bearer token、`Idempotency-Key` 和 multipart body。`document` 必须是第一个 part，包含 schema v1 的内容、分类、正文和逐句时间轴；`audio` 是第二个 part。单次请求最大 260 MiB，音频声明上限为 250 MiB。

后台按以下顺序处理：

1. 常量时间比较发布令牌并校验文档。
2. 用幂等键检查是否已经发布；重复请求返回原内容 ID 和版本。
3. 将音频流写入本环境的 S3 兼容对象存储，并核对大小及 SHA-256。
4. 在单个数据库事务中 upsert 来源、内容、栏目、难度、主题、音频关联和增量发布事件。
5. 内容立即进入当前环境的 `published` 目录，App 可通过 v1 API 获取。

测试部署使用 `VOA_ENV=test`，生产部署必须使用 `VOA_ENV=production`。CMS 会校验响应中的环境名，配置错目标时不会把任务标记成功。

生产部署需要设置：

- 独立 PostgreSQL、Redis 与 S3 兼容存储。
- `VOA_PUBLISHED_OBJECT_BUCKET` 和对象存储访问凭据。
- 独立的 `VOA_CMS_PUBLISH_TOKEN`，不能与测试环境共用。
- 面向 CMS 的 HTTPS 域名或受保护入口。

音频由目标后台自己的 `/api/v1/media/{id}` 提供 Range/206 播放，不会依赖 CMS 或内网 Content Library。
