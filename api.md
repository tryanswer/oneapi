# API Notes

## Channel API: 新增 `type_name` (渠道名称)

### 变更说明
- `Channel` 新增字段 `type_name`（渠道名称，用于展示“千问/豆包/智谱”等类型名称）。
- 创建渠道时 `type_name` **必填**（后端校验）。
- 查询接口 `/api/channel/` 与 `/api/channel/{id}` 会返回 `type_name`。

### Create Channel
**POST** `/api/channel/`

**Request Body (示例)**
```json
{
  "name": "qwen-channel-1",
  "type": 49,
  "type_name": "千问",
  "key": "sk-xxx",
  "base_url": "https://dashscope.aliyuncs.com",
  "models": "qwen3.5-plus,qwen3-omni-flash",
  "group": "default",
  "config": "{}"
}
```

**Response (成功)**
```json
{
  "success": true,
  "message": ""
}
```

### Get Channels
**GET** `/api/channel/`

**Response (节选)**
```json
{
  "success": true,
  "data": [
    {
      "id": 3,
      "name": "openclaw_bridge",
      "type": 50,
      "type_name": "OpenAI",
      "base_url": "http://127.0.0.1:8090/v1",
      "models": "cosyvoice-v3-flash"
    }
  ]
}
```

### Update Channel
**PUT** `/api/channel/`

> 更新时同样需要传 `type_name`（保持必填）。

**Request Body (示例)**
```json
{
  "id": 3,
  "name": "openclaw_bridge",
  "type": 50,
  "type_name": "OpenAI",
  "base_url": "http://127.0.0.1:8090/v1",
  "models": "cosyvoice-v3-flash",
  "config": "{}"
}
```

## 负载均衡：同优先级渠道的确定性分流

### 变更说明
- 当同一模型存在多个 **相同优先级** 的渠道时，使用 **用户 API Key 哈希** 进行确定性选择，避免请求集中在单一渠道。
- 如果请求没有可用 token hash，则仍使用随机分配。
- 重试路径也使用相同的 token hash，但会遵循优先级层级（优先尝试最高优先级，失败后才进入低优先级）。

### 注意事项
- 无需修改客户端请求；逻辑在服务端自动生效。
- 请重启 OneAPI 使变更生效。

## 其他注意事项
- 新增字段后需重启 OneAPI 触发自动迁移。
- 前端表单新增“渠道名称”下拉选项（千问/豆包/智谱/Kimi/MinMax/OpenAI/其他），允许自定义输入。

## Database: SQLite -> MySQL (Local Dev)

### Local MySQL DSN
```
SQL_DSN=root:root@tcp(127.0.0.1:3306)/openclaw?charset=utf8mb4&parseTime=True&loc=Local
```

### Migration Steps (completed)
1. Start OneAPI once with `SQL_DSN` to auto-migrate schema.
2. Export SQLite data to SQL inserts.
3. Truncate MySQL tables and import data.

### Notes
- `start_oneapi.sh` now sources `.env` if present (local-only, not committed).
- Do not commit secrets or `.env`.

## Redis (Local Dev)

### Environment
```
REDIS_CONN_STRING=redis://localhost:6379
SYNC_FREQUENCY=60
```

### Notes
- `SYNC_FREQUENCY` 必须设置，否则 Redis 会被禁用。
- 若 Redis 有密码，使用：`redis://default:<password>@localhost:6379`。

## Reset Database Script

### Script
- `reset_database.sh`

### Purpose
- 清空 OneAPI 业务数据，保留表结构。
- 支持：
  - MySQL：读取 `SQL_DSN`
  - SQLite：读取 `SQLITE_PATH`，默认 `one-api.db`

### Cleared tables
- `abilities`
- `channels`
- `logs`
- `options`
- `redemptions`
- `tokens`
- `users`
- `video_generation_tasks`

### Usage
```bash
./reset_database.sh
./reset_database.sh --yes
```

### Notes
- 脚本不会修改 `.env`
- 脚本不会清 Redis
- 执行后需要重启 one-api
- 重启后系统会自动重建初始 root 账户：
  - username: `root`
  - password: `123456`

## Multimodal Compatibility Notes (2026-03-07)

### Current OneAPI implementation status
- `chat/completions` 消息内容统一解析当前只覆盖 `text` 和 `image_url`。
- `input_audio` 没有进入统一的 `Message.ParseContent()` 解析层，而是仅在 `relay/controller/audio.go` 中对 `qwen3-asr-*` 转写请求做兼容转换。
- `video` / `input_video` 当前没有统一解析和转发逻辑，不能视为已完整支持。

### OpenAI-compatible provider findings
- 阿里百炼兼容模式的 `chat/completions` 可承载多模态内容块，适合图片理解，也可用于部分音频/视频理解模型。
- 豆包通道在本项目当前实现中走 `/api/v3/chat/completions`，图片理解模型应继续使用 `provider=openai` 前缀下的模型名透传，但需要按官方能力再次校验视频是否可直接复用同一路径。
- `doubao-1-5-vision-pro-250328` / `doubao-1-5-vision-lite-250315` 目前仅应作为图片理解候选；在完成官方核对前，不应默认视为视频模型。

### Video model caution
- `openai/qwen3.5-max` 当前仅在 OpenClaw 前端默认值和 allowlist 中出现。
- 在本次核对中，OneAPI 本地实现并未发现专门的视频内容块支持。
- 若官方侧无法确认 `qwen3.5-max` 的视频输入协议，则需要将默认视频模型切换为已确认支持 `input_video` 的模型，或在运行时走抽帧降级。

### Recommended fallback policy
1. 图片：直接走 `chat/completions` 多内容块。
2. 音频：优先走 provider 官方 ASR / audio 接口；仅在已确认支持时走 `chat/completions + input_audio`。
3. 视频：若上游与网关都确认支持 `input_video`，再直接透传；否则优先抽帧为图片，再不行则显式报错，不要静默降级。

## Image / Video Generation Notes (2026-03-07)

### Current route status
- 已实现：`POST /v1/images/generations`
- 已实现：`POST /v1/videos/generations`
- 已实现：`GET /v1/videos/generations/{id}`
- 已实现：`POST /v1/videos/generations/{id}/cancel`
- 未实现：`POST /v1/images/edits`
- 未实现：`POST /v1/images/variations`

### Confirmed image generation models from current code
- 阿里：`ali-stable-diffusion-xl`、`ali-stable-diffusion-v1.5`、`wanx-v1`
- 智谱：`cogview-3-plus`、`cogview-3`、`cogview-3-flash`、`cogviewx`、`cogviewx-flash`
- StepFun：`step-1x-medium`

### Scope boundary
- 上述模型清单来自本地 adaptor 常量、图片路由和图片计费配置，可视为当前网关“已接线”的图片生成模型集合。
- 这不等于这些模型都具备视频生成能力。
- `wanx-v1` 在本仓库当前仍走图片生成路径，不能直接当作视频生成模型使用。

### Recommended unified video generation design
- 推荐新增统一入口：`POST /v1/videos/generations`
- 不建议继续让 FatClaw 直连 provider 私有视频生成接口；否则客户端会感知豆包、阿里等不同异步协议，网关抽象会失效。
- 设计上应按“异步任务”处理，而不是复用当前同步风格的 `/v1/images/generations`。

### Proposed minimal API
**POST** `/v1/videos/generations`
```json
{
  "model": "doubao-seedance-xxx",
  "prompt": "一只橘猫在雪地里奔跑",
  "image": null,
  "first_frame_image": null,
  "last_frame_image": null,
  "size": "1280x720",
  "duration": 5,
  "response_format": "url"
}
```

**Response**
```json
{
  "id": "vidgen_task_xxx",
  "object": "video.generation.task",
  "status": "queued",
  "model": "doubao-seedance-xxx"
}
```

**GET** `/v1/videos/generations/{id}`
```json
{
  "id": "vidgen_task_xxx",
  "object": "video.generation.task",
  "status": "succeeded",
  "data": [
    {
      "url": "https://example.com/result.mp4"
    }
  ]
}
```

### Recommended pre-implementation errors
- 当前首版已接阿里 provider；对不支持的视频生成场景，建议继续返回：
```json
{
  "error": {
    "message": "Model does not support video generation",
    "type": "invalid_request_error",
    "code": "model_not_supported_for_video_generation"
  }
}
```

### Current implemented provider
- 当前只实现阿里万相任务链路。
- 入口会要求所选渠道类型为 `Ali` 或 `AliBailian`，否则返回：
```json
{
  "error": {
    "message": "Current channel does not support video generation yet",
    "type": "invalid_request_error",
    "code": "provider_not_supported_for_video_generation"
  }
}
```
- 查询和取消基于本地表 `video_generation_tasks` 记录的 `channel_id` / `provider_task_id` 回查上游任务。

### Implemented request mapping (Ali)
- 文生视频：
  - 路径：`/api/v1/services/aigc/video-generation/video-synthesis`
  - 输入：`prompt` + `size`
- 图生视频：
  - 路径：`/api/v1/services/aigc/image2video/video-synthesis`
  - 输入：`prompt` + `image` + `resolution`
- 首尾帧视频：
  - 路径：`/api/v1/services/aigc/image2video/video-synthesis`
  - 输入：`prompt` + `first_frame_image` + `last_frame_image` + `resolution`

### Implemented task object
**POST** `/v1/videos/generations`
```json
{
  "model": "wan2.2-t2v-plus",
  "prompt": "一只橘猫在雪地里奔跑",
  "size": "1280x720"
}
```

**Response**
```json
{
  "id": "9d0f8c6e-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "object": "video.generation.task",
  "status": "queued",
  "model": "wan2.2-t2v-plus",
  "provider": "ali",
  "channel_id": 2
}
```

**GET** `/v1/videos/generations/{id}`
```json
{
  "id": "9d0f8c6e-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "object": "video.generation.task",
  "status": "succeeded",
  "model": "wan2.2-t2v-plus",
  "provider": "ali",
  "channel_id": 2,
  "data": [
    {
      "url": "https://dashscope-result-example/video.mp4"
    }
  ]
}
```

### Status normalization
- 阿里 `PENDING` -> `queued`
- 阿里 `RUNNING` -> `running`
- 阿里 `SUCCEEDED` -> `succeeded`
- 阿里 `FAILED` -> `failed`
- 阿里 `CANCELED` -> `cancelled`

### Persistence
- 新表：`video_generation_tasks`
- 关键字段：
  - `id`
  - `user_id`
  - `token_id`
  - `channel_id`
  - `provider`
  - `model`
  - `provider_task_id`
  - `status`
  - `request_body`
  - `response_body`
  - `result_url`
  - `error_code`
  - `error_message`

### Current limitations
- 当前只接了阿里万相，豆包 Seedance 还未接入。
- 当前 `response_format` 仅保留字段，不参与上游分支。
- 当前计费策略为：
  - 创建任务时按 `model_ratio × group_ratio × channel_ratio × 1000` 估算本次任务 quota，并先校验用户余额是否足够。
  - 查询任务首次进入 `succeeded` 时，执行一次性扣费，并写入 consume log / channel used quota。
  - 失败、取消不扣费。
- 当前不是“预扣 -> 失败退款”模式，因此并发下仍可能出现多个任务同时通过余额校验，最终在成功阶段竞争额度；后续需要升级为预扣模型。

### FatClaw current recommended image-generation models
- `ali/wanx-v1`
- `zhipu/cogview-3-plus`
- `zhipu/cogview-3`
- `zhipu/cogviewx`
- `stepfun/step-1x-medium`

### Official capability matrix

| Provider | Model / Family | Text-to-image | Text-to-video | Image-to-video | First/last frame | Output format | Async task |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 火山 | `doubao-seedance-pro` | No | Yes | Yes | Yes | Video task result | Yes |
| 火山 | `doubao-seedance-lite` | No | Yes | Yes | Yes | Video task result | Yes |
| 阿里 | `wanx-v1` | Yes | No | No | No | Image (`url` / provider image response) | No |
| 阿里 | `wan2.2-t2v-plus` | No | Yes | No | No | MP4 | Yes |
| 阿里 | `wanx2.1-t2v-turbo` | No | Yes | No | No | MP4 | Yes |
| 阿里 | `wanx2.1-t2v-plus` | No | Yes | No | No | MP4 | Yes |
| 阿里 | `wan2.2-i2v-flash` | No | No | Yes | No | MP4 | Yes |
| 阿里 | `wan2.2-i2v-plus` | No | No | Yes | No | MP4 | Yes |
| 阿里 | `wanx2.1-i2v-turbo` | No | No | Yes | No | MP4 | Yes |
| 阿里 | `wanx2.1-i2v-plus` | No | No | Yes | No | MP4 | Yes |
| 阿里 | `wan2.2-kf2v-flash` | No | No | No | Yes | MP4 | Yes |
| 阿里 | `wanx2.1-kf2v-plus` | No | No | No | Yes | MP4 | Yes |
| 阿里 | `wan2.6-r2v` / `wan2.6-r2v-flash` | No | No | Yes | Multi-reference / video reference | MP4 | Yes |

### Matrix notes
- 豆包 Seedance 能力矩阵来自火山引擎官方发布说明与 API 目录：当前官方已明确提供“创建任务 / 查询任务 / 查询任务列表 / 取消或删除任务”四类视频生成 API，并说明 `doubao-seedance-pro`、`doubao-seedance-lite` 支持文生视频和基于首帧/尾帧图片的图生视频。
- 阿里万相能力矩阵来自百炼官方模型列表、视频生成使用指南、图生视频 API 参考、参考生视频 API 参考：
  - `wanx-v1` 在 one-api 当前接线中仍属于图片生成，不属于视频生成。
  - `wan2.2-t2v-plus`、`wanx2.1-t2v-*` 属于文生视频。
  - `wan2.2-i2v-*`、`wanx2.1-i2v-*` 属于首帧图生视频。
  - `wan2.2-kf2v-flash`、`wanx2.1-kf2v-plus` 属于首尾帧生视频。
  - `wan2.6-r2v*` 属于参考生视频，支持图像/视频参考，多角色、多镜头，异步返回结果。

### Async task abstraction recommendation
- 建议 one-api 统一内部任务对象：
```json
{
  "id": "vidgen_task_xxx",
  "object": "video.generation.task",
  "status": "queued",
  "model": "wan2.2-t2v-plus",
  "provider": "ali",
  "channel_id": 12,
  "request": {
    "prompt": "一只橘猫在雪地里奔跑",
    "image": null,
    "first_frame_image": null,
    "last_frame_image": null,
    "size": "1280x720",
    "duration": 5
  },
  "result": {
    "url": null,
    "expires_at": null
  },
  "error": null,
  "created_at": 1741334400,
  "updated_at": 1741334400
}
```
- 状态建议统一为：`queued`、`running`、`succeeded`、`failed`、`cancelled`
- provider 映射建议：
  - 豆包：创建任务 -> 保存 `RunId`；查询任务 -> 轮询 `GetExecution`
  - 阿里：创建任务 -> 保存 `task_id`；查询任务 -> 调用任务查询接口
- 只有在这套任务对象落地后，FatClaw 才应接入 `fatclaw-video-gen`
