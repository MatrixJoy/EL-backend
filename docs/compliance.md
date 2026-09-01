# 来源与合规记录

审核日期：2026-09-01。此文件是工程上线门禁记录，不构成法律意见。

## Robots

来源：`https://learningenglish.voanews.com/robots.txt`

- Sitemap：`https://learningenglish.voanews.com/sitemap.xml`。
- 普通文章 `/a/...` 与专题 `/p/...` 未被通配 user-agent 禁止。
- 禁止抓取四级日期归档、`/embed/`、`/comments/`、站内搜索及指定 podcast 子链接等路径。
- Source registry 必须拒绝 robots 中禁止的路径；robots 变化时暂停发现任务并要求复核。

## 内容使用结论

官方页面：`https://learningenglish.voanews.com/p/6861.html`（Request Our Content）。

- VOA Learning English 自制文本、MP3、照片和视频声明为 public domain，可用于教育和商业用途，但必须署名 `learningenglish.voanews.com`。
- AP、Reuters、AFP 等新闻机构提供的故事、照片和视频不在授权范围内，不得复制或再发布。
- 高分辨率素材可通过需要注册的 USAGM Direct 获取；工程不得绕过其注册流程。
- 若无法可靠确认资源为 VOA 独家制作，默认 `review_required`，不得自动发布或媒体代理。

## 产品执行规则

1. 每条内容和每个 asset 都必须有 `rights_status`、attribution 和来源 URL。
2. 只有 `public_domain_verified` 可进入自动发布和 `stream_proxy`。
3. `third_party_restricted` 仅保存必要元数据和 canonical link，不保存正文或代理媒体。
4. `unknown/review_required` 保持 review 状态，由管理员确认。
5. App 与站点必须展示 “Source: VOA Learning English” 和 canonical link，不使用 VOA 商标作为产品品牌。
6. 提供投诉与紧急下架入口；后台保留全局采集和媒体 kill switch。

## 尚需外部动作

- 上线前向 `learningenglish@voanews.com` 告知使用计划并保留往来记录。
- 商业化或品牌设计上线前，由项目责任人完成法律/商标复核。
