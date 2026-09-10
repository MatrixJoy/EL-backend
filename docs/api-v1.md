# Mobile API v1 草案

## 1. 通用约定

- Base URL：`https://api.example.com/api/v1`
- 响应：JSON / UTF-8；字段 camelCase；时间 UTC ISO 8601。
- 分页：`limit`（默认 20，最大 50）与 opaque `cursor`。
- 缓存：列表和详情返回 `ETag`；客户端使用 `If-None-Match`。
- 契约最终以 OpenAPI 3.1 文件为准。
- 首发匿名可用：公共读取和媒体播放不需要 access token，但仍受 IP/设备级限流保护。

## 2. 公共读取端点

| Method | Path | 用途 |
|---|---|---|
| GET | `/bootstrap` | App 配置、分类、功能开关和最低支持版本 |
| GET | `/home` | 首页编排后的 section 列表 |
| GET | `/categories` | 分级与主题树 |
| GET | `/categories/{slug}/contents` | 分类内容流 |
| GET | `/series/{id}` | 系列信息及剧集/课程列表 |
| GET | `/contents/{id}` | 内容详情、正文 block、资源与来源 |
| GET | `/search?q=` | 搜索标题、摘要、正文和精选单词；停用词与标题片段也可匹配 |
| GET | `/dictionary/{word}` | 查询自托管英语词典，返回多词性释义与例句 |
| GET | `/sync?cursor=` | 内容增量 upsert/tombstone |
| GET/HEAD | `/media/{assetId}` | 首期通过后台流式代理交付，支持 Range |

## 3. 用户端点（后续）

| Method | Path | 用途 |
|---|---|---|
| GET | `/auth/capabilities` | 获取真实登录能力与 Apple Client ID；禁止缓存 |
| POST | `/auth/apple` | identityToken + authorizationCode + nonce 校验换取会话；返回 appleUserId |
| POST | `/auth/development` | 仅测试环境：设备开发身份换取会话，生产环境不挂载 |
| GET | `/me` | 当前用户 |
| PUT/DELETE | `/me/bookmarks/{contentId}` | 收藏/取消收藏 |
| PUT | `/me/progress/{contentId}` | 幂等写学习进度 |
| GET | `/me/sync?cursor=` | 用户数据增量同步 |
| GET | `/me/retell-attempts?contentId=` | 获取当前用户的复述录音元数据 |
| PUT | `/me/retell-attempts/{attemptId}` | 上传或幂等更新私有复述录音（最大 25 MiB） |
| GET/HEAD | `/me/retell-attempts/{attemptId}/audio` | 鉴权下载私有复述录音，支持 Range |
| PUT | `/me/retell-attempts/{attemptId}/review` | 幂等保存设备转写、关键词覆盖与完成度反馈 |
| DELETE | `/me/retell-attempts/{attemptId}` | 同时删除对象存储文件与录音记录 |
| GET | `/me/vocabulary` | 获取当前用户的生词本 |
| PUT | `/me/vocabulary/{entryId}` | 幂等保存生词、释义、原句和文章来源 |
| PUT | `/me/vocabulary/{entryId}/review` | 按客户端更新时间幂等保存间隔复习状态 |
| DELETE | `/me/vocabulary/{entryId}` | 删除生词 |
| DELETE | `/me` | 删除账号与关联个人数据 |

## 4. 代表性响应

```json
{
  "data": {
    "id": "018f...",
    "type": "lesson",
    "title": "Lesson 1: Who Are You?",
    "summary": null,
    "level": "beginning",
    "publishedAt": "2022-07-20T00:00:00Z",
    "releasedAt": "2026-09-08T01:17:45Z",
    "series": { "id": "018e...", "title": "Let's Learn English with Anna" },
    "bodyBlocks": [
      { "type": "heading", "text": "Lesson Plan", "level": 2 },
      { "type": "paragraph", "text": "..." }
    ],
    "featuredWords": [
      { "word": "pest", "partOfSpeech": "noun", "definition": "an animal or insect that causes problems" }
    ],
    "grammarPoints": [
      {
        "kind": "modal",
        "title": "Modal verb",
        "explanation": "A modal verb comes before the base form of another verb.",
        "example": "People can learn the process quickly.",
        "prompt": "People _____ learn the process quickly.",
        "answer": "can",
        "options": ["can", "could", "might"]
      }
    ],
    "assets": [
      {
        "id": "0190...",
        "kind": "video",
        "quality": "720p",
        "durationSeconds": 301,
        "playbackUrl": "/api/v1/media/0190..."
      }
    ],
    "source": {
      "name": "VOA Learning English",
      "canonicalUrl": "https://learningenglish.voanews.com/a/6654462.html"
    },
    "revision": 3
  }
}
```

## 5. 增量同步

```json
{
  "data": {
    "changes": [
      { "sequence": 1001, "operation": "upsert", "entity": "content", "id": "018f...", "revision": 3 },
      { "sequence": 1002, "operation": "delete", "entity": "content", "id": "017a..." }
    ],
    "nextCursor": "opaque-token",
    "hasMore": false
  }
}
```

客户端对 upsert 再取详情，或后续由 `include=compact` 返回紧凑实体。cursor 只作为不透明值保存；服务端可将其签名并包含 sequence 与过期策略。

## 6. 错误

```json
{
  "error": {
    "code": "CONTENT_NOT_FOUND",
    "message": "The requested content is unavailable.",
    "requestId": "req_..."
  }
}
```

稳定错误码包括 `VALIDATION_ERROR`、`UNAUTHORIZED`、`FORBIDDEN`、`RATE_LIMITED`、`CONTENT_NOT_FOUND`、`MEDIA_UNAVAILABLE`、`UPSTREAM_TEMPORARILY_UNAVAILABLE` 和 `INTERNAL_ERROR`。

## 7. 契约测试要求

- OpenAPI lint 与 breaking-change 检查进入 CI。
- iOS 从固定版本契约生成或校验 DTO。
- 所有端点测试认证、分页边界、304、限流、未知字段兼容和错误结构。
- 媒体单独测试 HEAD、Range、缓存和断点续传。
