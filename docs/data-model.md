# 数据模型草案

## 1. 原始层

### source_items

记录被发现的上游实体：`id`、`source`、`canonical_url`、`external_id`、`page_type`、`first_seen_at`、`last_seen_at`、`fetch_state`、`next_fetch_at`。`(source, canonical_url)` 唯一。

### source_snapshots

记录每次有效响应：`id`、`source_item_id`、`fetched_at`、`http_status`、`etag`、`last_modified`、`content_hash`、`object_key`、`parser_version`、`parse_state`、`error_code`。相同内容指纹可跳过重复解析。

### parse_runs

记录解析器输入输出、字段完整率、警告和耗时，用于检测页面结构漂移。

## 2. 领域层

### categories

分级或主题分类：`id`、`slug`、`name`、`kind`、`parent_id`、`sort_order`、`status`。

### series

系列/课程：`id`、`slug`、`title`、`description`、`level`、`cover_asset_id`、`source_item_id`、`status`。

### contents

统一内容表：

- 身份：`id UUID`、`slug`、`source_item_id`、`external_id`；
- 类型：`article | lesson | podcast | video | quiz`；
- 展示：`title`、`summary`、`level`、`published_at`、`duration_seconds`；
- 内容：`body_blocks JSONB`、`transcript_blocks JSONB`、`metadata JSONB`；
- 状态：`draft | review | published | hidden | removed`；
- 版本：`revision`、`source_updated_at`、`created_at`、`updated_at`。

`body_blocks` 使用受控类型，如 `paragraph`、`heading`、`image`、`vocabulary`、`quote`、`list`，避免直接保存可执行 HTML。

### content_relations

表达 `series_episode`、`related`、`previous`、`next`、`contains_quiz` 等关系，字段为 `from_content_id`、`to_content_id`、`relation_type`、`position`。

### content_categories

内容与分类多对多关系，包含 `is_primary`、`position`。

### assets

图片、音频、视频、字幕与文档：`id`、`kind`、`source_url`、`source_host`、`mime_type`、`byte_size`、`duration_seconds`、`width`、`height`、`quality_label`、`checksum`、`delivery_policy`、`object_key`、`availability`。

### content_assets

资源与内容关系：`content_id`、`asset_id`、`role`（hero/audio/video/transcript/document）、`position`。

### content_revisions / publish_events

保存字段差异、操作者/任务、发布时间和变更原因。`publish_events` 同时作为客户端增量同步的单调游标来源。

## 3. 用户层（后续）

- `users`：Apple subject 的内部映射，不保存不必要资料。
- `devices`：匿名安装与用户合并。
- `bookmarks`：收藏及幂等版本。
- `learning_progress`：内容、位置、完成状态、客户端更新时间。
- `playback_progress`：媒体位置和完成度，可按产品需求合并到学习进度。
- `retell_attempts`：用户复述录音元数据；音频存放在独立私有对象桶，所有读写均需要用户会话。

## 4. 关键索引与约束

- `source_items(source, canonical_url)` unique。
- `contents(source_item_id)` unique；`contents(status, published_at desc, id)`。
- `series(slug)`、`categories(slug)` unique。
- `publish_events(sequence)` unique，sequence 单调递增。
- 对发布内容建立 PostgreSQL `tsvector` 索引；中文辅助字段加入后再评估 PG trigram 或专用搜索服务。
- 所有外键明确 `ON DELETE` 行为；发布内容默认软删除。
