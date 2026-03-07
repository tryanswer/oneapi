# Tasks

## Channel type_name (渠道名称) & UI
- [x] Add `type_name` field to Channel model (persisted) and ensure `/api/channel/` returns it.
- [x] Require `type_name` on create (backend validation).
- [x] Add channel name selector to channel create/edit forms (default/air/berry) with common options and allow custom input.
- [x] Update form validation to require `type_name`.

## Build script
- [x] Fix `web/build.sh` to read `THEMES` and build from its own directory when invoked from repo root.

## Reset script
- [x] Add `reset_database.sh` to clear OneAPI business tables and restore initial state for MySQL / SQLite.

## Deterministic load balancing (same priority)
- [x] Use token hash to deterministically pick a channel among same-priority candidates.
- [x] Keep random fallback when no token hash is available.
- [x] Apply to retry path while respecting priority tiers.

## Switch DB to MySQL (Aliyun RDS)
- [x] Local dev: migrate SQLite data to MySQL `openclaw` and set `SQL_DSN`.
- [x] Local dev: enable Redis cache via `REDIS_CONN_STRING` + `SYNC_FREQUENCY`.
- [ ] RDS: collect connection info and decide migration approach (fresh vs. migrate from SQLite).
- [ ] RDS: configure `SQL_DSN` (and optional `LOG_SQL_DSN`) for production.
- [ ] RDS: verify schema auto-migration and application startup with MySQL.

## 2026-03-07 多模态消息兼容
- [ ] 核对 OpenAI 兼容接口下图片 / 音频 / 视频输入的实际支持矩阵，明确哪些模型可直接走 `/v1/chat/completions`
- [ ] 补齐 `relay/model/message.go` 对视频内容块的统一解析，避免当前仅支持 `text` / `image_url`
- [ ] 评估是否将音频兼容逻辑从 `audio.go` 的模型特判，收敛为统一的多模态消息透传层
- [ ] 若视频输入当前不兼容，定义降级策略（抽帧图片 / 仅文本提示 / 显式报错）
- [ ] 校验阿里百炼通道下图片识别、语音识别、视频理解请求体与返回体是否与 OpenClaw 当前实现一致
- [ ] 输出 FatClaw 可依赖的“多媒体输入模型清单”，避免前端/网关误把不支持视频的模型当成视频模型使用
- [ ] 单独核对豆包视觉模型在 OpenAI 兼容协议下的 provider/model 写法，确认 `doubao-1-5-vision-pro-250328` / `doubao-1-5-vision-lite-250315` 可直接用于图片理解
- [ ] 单独核对 `qwen3.5-max` 的视频理解输入格式；若官方无此模型或无视频能力，替换默认视频模型并同步给产品端
- [ ] 为不支持视频的渠道返回显式错误码/错误文案，避免上游 400 直接透出给 FatClaw

## 2026-03-07 图片/视频生成接口补齐
- [x] 盘点当前 `POST /v1/images/generations` 在各国内渠道的可用模型清单，至少输出 FatClaw 当前可直接使用的图片生成模型列表
- [x] 评估并设计统一的视频生成兼容入口，确认是新增 `/v1/videos/generations` 还是通过 provider 代理路径暴露
- [x] 若暂不支持统一视频生成接口，至少输出清晰的错误码与错误文案，供 FatClaw `fatclaw-video-gen` 后续接入时区分“模型不支持”和“接口未实现”
- [x] 为豆包 `doubao-seedance-*` 与阿里 `wan*` 系列生成模型补充能力矩阵：文生图 / 文生视频 / 图生视频 / 首尾帧 / 输出格式 / 是否异步任务
- [x] 若落地 `/v1/videos/generations`，补齐异步任务模型：创建任务、查询任务、取消任务、统一结果对象
- [x] 补充 `/v1/videos/generations`、`/v1/videos/generations/:id`、`/v1/videos/generations/:id/cancel` 占位路由，返回明确错误码
- [x] 首个 provider 落地：阿里万相 `wan2.2-t2v-plus` / `wanx2.1-*` / `wan2.2-i2v-*` / `wan2.2-kf2v-*` 通过任务表实现创建、查询、取消
- [x] 视频生成计费接入 OneAPI quota / consume log，任务成功后按模型倍率估算额度扣费并写消费日志
- [ ] 视频生成从“成功后扣费”升级为“创建时预扣 / 失败退款”，降低并发下的额度竞态风险
