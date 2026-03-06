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
