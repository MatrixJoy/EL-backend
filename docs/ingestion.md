# VOA 数据采集与代理设计

## 1. 原则

- 优先使用公开、稳定、明确允许的 feed/sitemap/API；HTML 解析作为补充。
- 遵循 robots.txt、条款、许可和合理抓取频率，不绕过登录、付费墙、验证码或访问控制。
- 采集任务幂等且可回放；线上已发布内容不会因一次解析失败被覆盖。
- 任何新域名或 URL 类型都必须显式加入 source registry。

## 2. 页面分类与适配器

初始适配器：

- `LandingParser`：首页和分级专题，发现栏目/系列。
- `ListingParser`：列表与播客归档，发现内容及下一页。
- `ArticleParser`：标题、日期、系列、摘要、正文、相关内容。
- `LessonParser`：正文、视频、讲义、测验入口、上一课/下一课。
- `PodcastParser`：发布日期、音频、时长、说明。
- `MediaExtractor`：图片、MP3、视频清晰度、字幕/文档元数据。

每个解析器返回统一 `ParsedSourceItem`，包含字段 provenance（CSS/JSON-LD/meta/feed）与 warnings。

## 3. 抓取规则

- 单独可识别的 User-Agent，并提供联系邮箱/站点说明（上线时配置）。
- 默认每主机低并发和令牌桶限速；对 429/503 尊重 `Retry-After` 并指数退避。
- 连接、首字节和总时长分别超时；限制 HTML、图片、文档和媒体探测大小。
- 优先 `If-None-Match`/`If-Modified-Since`；304 不生成新 revision。
- 只跟随有限次 HTTPS 重定向；每次重定向重新校验域名/IP，阻止内网和 link-local 地址。
- 列表短周期抓取，历史详情长周期复查；调度加入 jitter 避免流量尖峰。

具体频率必须根据 robots、条款和上游响应调优，不在设计阶段硬编码。

## 4. 去重与一致性

- URL canonicalization 去除已知追踪参数，但不随意合并业务 query。
- 首选稳定上游 ID，其次 canonical URL；内容 hash 只用于修订判断，不能作为永久身份。
- 同一媒体多清晰度作为一个内容下多个 asset，保留质量、大小和 MIME。
- 数据库写入使用唯一约束和事务；任务至少一次投递，消费者必须幂等。

## 5. 校验与漂移检测

每类 fixture 提供 parser contract test。上线指标包括：

- 页面成功率与状态码分布；
- 必填字段完整率、正文长度分布和媒体数量；
- selector/provenance 命中率；
- 新增/更新/删除数量相对历史基线；
- snapshot 到 publish 的延迟。

当必填字段缺失、正文骤短、条目数骤降或未知页面模板集中出现时：隔离候选数据、暂停自动发布、告警并保留上一发布版本。

## 6. 媒体交付

客户端请求 `/media/{assetId}`。首期 `delivery_policy` 默认使用流式代理：后台按 asset 中登记的源地址读取并转发，不永久保存完整媒体；若许可不允许代理则降级为短时重定向或源页面链接。只有取得明确授权后，才允许对象存储、长期 CDN 缓存和离线下载。实现要求：

- 支持 GET/HEAD 和单 Range；正确返回 200/206/416。
- 透传经 allowlist 过滤的 `Content-Type`、`Content-Length`、`ETag`、`Last-Modified`。
- 不接受客户端传入上游 URL；签名 token 绑定 asset、过期时间和可选设备。
- 缓存键只由 asset revision 和 range 安全地产生。
- 上游异常时使用熔断；不得返回过期或已下架资源。
- 代理过程只使用有界内存缓冲，不因播放请求在本地生成完整临时文件。

## 7. 合规检查清单

- [ ] 归档 robots.txt 检查结果和日期。
- [ ] 审核网站服务条款及 VOA 内容/美国政府作品相关许可的适用范围。
- [ ] 确认第三方图片、视频、讲义、测验可能存在的独立版权。
- [ ] 确认是否允许全文再展示、媒体代理、离线下载和长期缓存。
- [ ] 定义 attribution、canonical link、投诉与下架流程。
- [ ] 避免在 App 名称、图标和域名中造成官方隶属关系误解。

## 8. Spike 验收标准

- 至少覆盖 5 种页面类型、20 个 fixture。
- 重复执行不产生重复内容或 revision。
- 修改 fixture 后能检测字段变化；破坏 selector 后能阻断发布。
- 解析结果能表达多媒体、系列顺序、正文 block 与来源信息。
- 媒体代理通过 Range、超时、域名 allowlist 和 SSRF 测试。
