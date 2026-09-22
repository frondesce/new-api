# OpenRouter 图片编辑兼容

New API 的 **OpenRouter 渠道**支持把 `POST /v1/images/edits` 转为 OpenRouter 的
`POST /api/v1/images` JSON 请求。参考图片通过 `input_references` 传递，模型名称沿用
渠道模型映射后的值。此转换不按模型名称匹配；实际编辑能力取决于上游模型是否支持参考图。

## Open WebUI 配置

在图片编辑设置中开启编辑，选择 OpenAI 引擎：

- API Base URL：`http://<new-api-host>:<port>/v1`
- API Key：New API 令牌
- 模型：渠道支持的完整模型名称，例如 `openai/gpt-image-2.5-sunburst`

New API 使用 OpenRouter 渠道的默认上游地址即可。请求体透传必须关闭，才能执行转换。
图片生成配置与编辑配置独立；本适配不改变 `/v1/images/generations` 的行为。

Open WebUI 会负责读取已生成的图片并在下一次编辑时重新发送。连续修改还需要在聊天中
启用绘图功能；使用原生工具调用时，聊天模型需能调用 `edit_image` 并选择参考图片。

## 请求与能力边界

- multipart：支持 `image`、重复的 `image[]`、按数字顺序排列的 `image[0]` 等文件字段。
  不混用不同的图片字段命名方式。文件内容用于识别 MIME 类型并编码成 data URL。
- JSON：支持 `image` 字符串或字符串数组、`images: [{"image_url": "..."}]`，以及
  OpenRouter 的 `input_references`。URL 可以是 HTTP(S) 或 Base64 图片 data URL；不支持文件 ID。
- 每次至少 1 张、最多 16 张参考图，`n` 为 1–10；具体上游可以有更严格的限制。
- `size: "auto"` 不发送固定尺寸。支持尺寸、质量、背景、输出格式、压缩率等公共参数；
  `output_compression: 0`、`seed: 0`、`stream: false` 会保留。
- 响应使用 `data[].b64_json`；不支持 `response_format: "url"`。
  保留上游 `media_type` 和 `usage`，复用现有图片响应与计费流程。
- 本适配支持非流式编辑，与 Open WebUI 的图片编辑调用方式一致；`stream: true` 返回 400。
  OpenRouter 与 OpenAI 编辑流的事件名称不同，暂不转换流式协议。
- 不支持的参数（包括 `mask`、`input_fidelity`）会返回明确的 400 错误，避免静默改变编辑语义。
  上游专用选项可通过 OpenRouter `provider` 对象传递，以模型端点公布的能力为准。
- 普通 OpenAI 及其他渠道继续使用各自的编辑协议；原始入站请求头保持不变，供渠道重试使用。

上游协议与能力查询：

- <https://openrouter.ai/docs/guides/overview/multimodal/image-generation>
- <https://openrouter.ai/api/v1/images/models>

## 验证

运行协议回归测试：`go test ./relay/channel/openai ./relay/helper -count=1`。

部署后，在 Open WebUI 中生成图片，再连续发送两次修改指令，确认每次都以正确的图片为参考，
并检查 New API 中的成功记录、返回图片数量与用量。也应验证多参考图编辑和原有渠道的编辑功能。
