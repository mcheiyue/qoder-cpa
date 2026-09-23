# qoder-cpa 后续实施计划

## 当前基线

- 当前版本：`v0.1.24`（流稳定性，已发布并部署）
- 已完成：Device OAuth、Qoder Global COSY/Bearer transport、动态模型目录、Chat/Responses 执行器、WebUI 账号与传输配置、缺失 usage 时的估算 Token、配额控制面（v0.1.21）。
- 已完成：模型目录驱动的动态公开 ID 映射；Qoder 上游返回的 `key/id` 与展示名生成客户端可见模型 ID，executor 按 `AuthID` 反向还原内部 key，不需要手动修改 CPA alias 配置（v0.1.22）。
- 本轮实施：Chat Completions 高级参数映射；Bearer 与 COSY transport 分别接收已支持的参数，保留显式 `false/0`，不伪造 COSY 未确认的 `temperature` 字段。
- 当前边界：估算 Token 仅用于没有真实 usage 的响应，并标记 `estimated=true`；不伪造缓存 Token，不覆盖真实 usage。
- 流稳定性已发布并完成现网受控流式验证：已有 EOF 截断错误保持不变；新增 HTTP 200 业务错误/限流识别、请求级 `agentLimitResetTime` 脱敏摘要和 `Tool calls: [...]` 跨 chunk 回退。
- 发布约束：使用 GitHub Actions 构建 Linux amd64/arm64；VPS 只部署 Release 产物，不在 VPS 编译。

## 阶段 1：配额与账号状态

目标版本：`v0.1.21`

实施状态：已完成并在 v0.1.21 发布。

### 目标

将 Qoder 账号的套餐、配额和限制状态接入 CPA WebUI，帮助用户判断账号是否可用；不把配额接口当作请求 Token 来源。

### 实施范围

- 核对 Orchids-2api 当前实现和 Qoder Global 实际接口，确认 quota、plan、status 的真实 URL、鉴权方式和响应结构。
- 重做 `internal/qodercontrol/quota.go` 的响应模型和请求路径；配额、套餐和状态统一走 `openapi.qoder.sh` 的 Bearer 控制面，不复用 Chat 的 COSY 签名 transport。
- 扩展账号状态模型，至少覆盖：套餐、剩余额度、额度上限、Agent limit、重置时间、是否耗尽、最后刷新时间和脱敏错误状态。
- 模型展示名自动适配：目录返回的真实 `key/id` 与 `name`、`display_name` 或 `displayName` 生成客户端可见 ID；缺失展示名时回退内部 ID。插件按 `AuthID` 保存公开 ID 到内部 key 的动态映射，发送到 Qoder 前还原内部 key；禁止手动 CPA alias 配置和静态模型猜测。
- 扩展 management API 返回账号配额状态，并提供单账号刷新；失败不得删除或失效现有凭据。
- 调整 `web/index.html`：在账号表展示状态摘要、配额、重置时间和刷新结果；模型相关区域同时显示客户端可见 ID、友好名称和来源账号，保持当前紧凑表格布局。

### 不做

- 不用配额数据填充 prompt/completion Token。
- 不在插件内实现多账号轮转。
- 不复制 CPA 的认证存储或刷新机制。

### 验收

- 使用本地假上游覆盖成功、字段缺失、额度耗尽、限流和接口失败。
- COSY API2、COSY API3、Bearer profile 至少分别通过请求构造回归测试。
- 模型目录覆盖新增模型、目录顺序变化、重复 ID、缺失展示名、展示名冲突和 `qoder/<id>` 去重；展示名变化应更新客户端可见 ID，反向映射必须仍指向当前内部 key。
- 管理接口不返回 access token、refresh token、COSY 签名材料或请求体。
- WebUI 能区分“额度耗尽”“刷新失败”“未刷新”和“正常”。
- `go test ./... -shuffle=on -count=1`、`go vet ./...`、内联 JavaScript 检查和 CI race 全部通过。

## 阶段 2：Qoder 高级参数映射

目标版本：`v0.1.23`

### 目标

补齐客户端请求到 Qoder 原生请求的高级参数映射，保持 OpenAI 兼容客户端和 Qoder 两条 transport 的行为一致。

### 实施范围

- 扩展 `internal/qodertransport/chat_request.go` 的输入模型，明确区分通用 OpenAI 字段和 Qoder 专用字段。
- 核对并按真实上游行为映射：
  - `reasoning_effort`
  - `temperature`
  - `max_tokens` / `max_completion_tokens`
  - `tool_choice`
  - `parallel_tool_calls`
  - tools、tool calls 及其相关选项
  - 已由 Qoder Global 实际接受的模型配置字段
- 分别实现和测试 COSY、Bearer 两条 transport 的字段映射；不能仅在 OpenAI 兼容层保存字段而不发送。
- 对不支持或冲突的参数采用明确策略：忽略、转换或返回结构化 `unsupported_parameter`，不得静默伪造成功语义。
- 保留已有消息、工具、reasoning 和 stream 行为，不改变账号选择、认证和 WebUI 管理接口。

### 验收

- 每个支持参数都有请求 JSON 回归测试，断言实际出站字段。
- COSY 与 Bearer 的差异有独立测试，避免一条 transport 的实现覆盖另一条。
- 工具调用、reasoning 和普通文本三类请求分别覆盖流式与非流式路径。
- 真实上游只做受控单请求验证，不重复发送不支持的参数。
- 真实 usage、估算 usage 和缓存字段行为保持不变。

实施状态：已完成并在 v0.1.23 发布、部署。

## 阶段 3：流稳定性

目标版本：`v0.1.24`

### 目标

让长连接 Agent 请求在上游异常、限流和不完整结束时返回可诊断、可重试的结果，避免静默成功或泛化错误。

### 实施范围

- EOF 时没有终止事件，返回明确的截断错误并保留已收到内容。
- 识别 HTTP 200 响应中的内嵌限流/业务错误文本。
- 解析并暴露 `agentLimitResetTime`，供错误摘要和账号状态使用。
- 完善 HTTP 错误、SSE 错误和业务错误的 code/type/message 分类，同时保持敏感信息脱敏。
- 为工具调用仅以文本形式返回的情况增加回退解析，避免丢失可执行工具意图。
- 保持 CPA 宿主需要的裸 JSON chunk 约束，由宿主统一包装 SSE；不得再次引入 `data: data:` 双重包装。

### 验收

- 覆盖正常终止、EOF 截断、`response.failed`、HTTP 200 内嵌限流、业务错误和客户端取消。
- 覆盖跨 chunk、CRLF、空事件、超大事件和末尾无空行等 SSE 边界。
- Agent 客户端收到的每个流帧均符合 Chat Completions 联合类型：有效 `choices[]` 或有效 `error{}`。
- 错误日志包含状态码、业务 code、类型、重置时间等脱敏诊断字段。
- 使用受控真实请求验证一次流式正常响应和一次可复现错误，不进行压力测试。

## 阶段 4：COSY 原生 Thinking/Context 语义

目标版本：`v0.1.25`（发布候选）

### 目标

将 Qoder 目录模型的 `is_reasoning` 和 `max_input_tokens` 元数据贯穿到 COSY 出站请求的 `model_config` 和 `parameters` 层，支持推理模型原生 thinking toggle 和 context length 声明；不伪造不存在的 effort 级别。

### 实施范围

- 目录解析保留 `is_reasoning`、`max_input_tokens`，per-AuthID 原子替换。
- 公开 ID 映射与 `ModelMeta` 组成同一份 per-AuthID 目录快照，`ModelResolver` 返回 `ResolvedModel{InternalID, IsReasoning, MaxInputTokens}`。
- `ModelInfo` 暴露 `InputTokenLimit`；推理模型设置 `ThinkingSupport{ZeroAllowed: true}`，不声称命名 effort 级别。
- COSY 出站：`model_config` 传递 `is_reasoning`/`max_input_tokens`；`parameters` 添加 `enable_thinking`（推理模型默认 true）、`context_length`（>0 时发送）；`reasoning_effort=none` 映射为 `enable_thinking=false` 且不发送 `reasoning_effort`；其他 effort 级别正常转发。
- Bearer transport 不变，无 COSY-only 字段泄漏。

### 本地实施状态

本地已完成。目录解析、CPA 模型元数据、per-AuthID 原子快照和 COSY 请求语义均有回归测试；排除依赖真实 API2 的不稳定 `TestTransport_TamperedBody` 后，确定性全量测试 shuffle×3、`go vet ./...`、`gofmt` 和差异检查通过。该外部测试单独复跑时偶发 5 秒上下文取消，未用重试或延时掩盖。CGO/`-race` 需要 gcc，当前环境不可用，留待 CI。

### 并发审计

`modelRegistry` 使用 `sync.RWMutex` 保护每个 AuthID 的目录快照；公开 ID、内部 ID 与模型元数据在同一次加锁中整体替换，resolver 在一次 RLock 内读取同一条记录。未发现 refresh、配额或流执行路径的其他共享状态竞态，无需新增锁。

### 待办

- CI race 验证（本机无 gcc）
- Release 产物构建与部署
- 现网受控单请求验证

## 暂缓项

- PAT 登录和 `jt-*` Job Token 交换：Device OAuth 已满足当前账号接入需求，除非出现明确使用场景。
- 多账号按配额轮转：优先交给 CPA 宿主调度，插件只提供准确状态。
- 后台 token 刷新：继续复用 CPA AuthProvider 的 refresh 链路，不新增重复任务。
- 批量账号导入：当前收益低于配额和流稳定性。
- 模型目录缓存：先展示刷新时间；只有真实刷新成本或上游限流成为问题时再加入缓存。

## 发布顺序

严格按小版本单主题发布：

1. `v0.1.21`：配额与账号状态。
2. `v0.1.22`：动态模型公开 ID 映射。
3. `v0.1.23`：高级参数映射。
4. `v0.1.24`：流稳定性。
5. `v0.1.25`：COSY 原生 Thinking/Context 语义（发布候选）。

阶段 3 实施状态：已在 v0.1.24 发布、部署，并完成现网受控流式验收。

阶段 4 实施状态：本地完成（TDD、go vet、gofmt 通过；-race 待 CI），正在构建 Release 并部署。

每个版本均遵循：本地红测 → 最小实现 → 全量测试/vet → CI race 与双架构构建 → Release 产物校验 → VPS 备份、原子替换、重启 CPA → 单次受控验证。
