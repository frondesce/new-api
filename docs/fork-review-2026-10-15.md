# Fork 独立功能与补丁复核（2026-10-15）

状态：四项清理已提交为 `030308f1e`；2026-10-16 用户另行授权同步最新上游，同步与验证记录见“后续上游同步”。清理范围以“确认后的实施记录”为准；AGY 二次复核与初审作为历史保留。

## 后续上游同步（2026-10-22）

- 合并前 HEAD：`80b448d08313e3b4fa3372c82bdd3cd8cdd180b8`。
- 共同祖先及上次整合点：`ca2cae676db87333a583b6e104d0f1f7366f3258`。
- 本次目标：`upstream/main@7d8b91268dab69d7150f345c448b5e250163146a`，也是 `v1.0.0-rc.46`；新增 30 个上游提交。
- 使用普通双亲 merge，保留已有历史；原有未跟踪 `.codex` 不纳入提交，没有推送或部署。

唯一文本冲突位于 `relay/channel/gemini/adaptor.go` 的 import：保留本地 Custom Gemini/Vertex URL 所需的 `net/url`，并加入上游多模态 embedding 输入校验所需的 `slices`。自动合并后的渠道 schema、默认值、读取、提交和类型同时保留本地 `custom_protocol`、已有 `vertex_project_id` 和新增 `aws_api_type`。原有渠道配置回归包含在前端全量测试中。

对本次相交的分支差异作了核对：MiMo metadata 保护、Cloudflare 精确主机和无损索引处理、OpenRouter 图片桥接、GHCR/Compose 和已归档工作流与合并前逐字节一致；用户软删除筛选、本地 Gemini/Vertex 配置及旧式 Google Search 转换回归继续保留。`service/error.go` 仍与上游一致，没有恢复旧的全局 429 归一化补丁。

复核新引入的 Resend 代码时，发现其服务器日志会直接记录供应商错误正文和传输错误，能够带出 API key、验证码或重置令牌。扩展现有 `controller/email_delivery_test.go`，先确认两条日志泄漏路径失败，再修改 `common/email_resend.go`：HTTP 失败只记录状态码，传输失败只记录固定事件；中文服务器日志键同步更新。对外错误、发送次数、不重定向和不自动回退的行为保持。没有新增 UI 或通用组件实现。

### 值得关注的功能和兼容变化

- **渠道优先级拖拽**：渠道列表支持拖拽加入同一优先级或插入新层级，提供引导演示、预览和撤销；后端包含版本冲突及事务回滚保护。见上游 `5e9595190`、`5c31c1fa9` 和 `web/src/features/channels/components/channels-table.tsx`。
- **DeepSeek V4.1 Flash 与工具接口**：支持 Search/Fetch API 和 `@thinking` / `@effort` 修饰符。渠道模型列表使用 `deepseek-flash` 等名称；工具有独立价格配置，更新时应检查工具价格。见 `6cf0d90b5`、`relay/channel/deepseek/`、`router/relay-router.go`。
- **Resend 邮件发送**：系统设置的邮件集成中可以在 SMTP 与 Resend 间选择；需配置已验证域名的发件地址和 API key。账户安全邮件也按用户语言发送。见 `32e587406` 和 `web/src/features/system-settings/integrations/email-settings-section.tsx`。
- **通知分类开关**：个人设置可以分别控制余额、渠道及模型更新通知；余额提醒改为余额跨越阈值时触发，系统也有总开关。见 `6461f084f` 和 `web/src/features/profile/components/tabs/notification-tab.tsx`。
- **仪表盘缓存用量**：在总 token 用量旁展示缓存读取和缓存创建统计。`quota_data` 新增两列，历史数据初始为零；审计日志另增加 `(created_at, event_id)` 排序索引。见 `603a5ceb5`、`058c8231b`。
- **更多上游接口**：AWS 渠道可选择 Mantle Messages API，已有渠道默认保持 InvokeModel；阿里支持 `qwen3-vl-embedding` 多模态向量化。见 `d03220473`、`afe73b08f`。
- **运行与任务修复**：Go 最低版本提高到 1.26.10；Claude 流式请求在准备和重试期间持续保活，异步任务在重试前重新定价并补足预留，轮询重定向限制凭据接收主机。见 `7d8b91268`、`25372e30b`、`55e2ae54d`、`590042682`。

### 验证结果

- `make test`：根模块、relaykit、tokenkit 全量测试通过。Resend 修复后，额外执行 `go test ./common ./controller -count=1`，两个包通过；`go test ./controller -run '^TestResend' -count=1 -v` 全部通过。修复前的两项日志脱敏回归确实失败，日志在 `/tmp/new-api-sync-20261022-resend-before.log`。
- 根目录、`relaykit/`、`tokenkit/` 各自执行 `GOWORK=off go build ./...` 和 `GOWORK=off go vet ./...`，均通过；根模块在 Resend 修复后再次通过。根工具链为 Go 1.26.10，两个子模块遵循各自 `go.mod`。
- AGY 执行前端机械验证，Codex 复核日志与比较结果：`bun install --frozen-lockfile`、`bun run typecheck`、`bun run build` 通过；`bun run test -- --maxWorkers=2` 为 **200 个测试文件、2386 项用例全部通过**。
- `bun run lint` 仍有 158 条 error、66 条 warning；`bun run format:check` 仍有 25 个文件失败。所有报错文件与目标上游逐字节相同；在 `/tmp` 隔离提取纯上游文件、使用同版依赖和配置复核，224 条 lint 诊断的完整集合相同，格式失败列表也相同。没有合并新增项，没有顺带修改这些上游问题。
- `bun run i18n:sync` 通过，七种语言 missingCount/extrasCount 均为 0，没有产生额外 tracked diff。工具报告 ja/ru/zh 各有 4/5/3 项与英文值相同，此结果不等同于人工判定翻译错误。前端完整日志及基线比较保存在 `/tmp/new-api-sync-20261022-frontend/`。
- 修改的 Go 文件已 gofmt；无未合并索引项或冲突标记，`git diff --check` 和 `git diff --cached --check` 通过。

数据库均为本次创建的隔离 Docker 容器，无现有数据卷：**SQLite 3.50.4、MySQL 5.7.44、PostgreSQL 9.6.24、ClickHouse 25.8.33.6**。实际执行：

```bash
export TEST_MYSQL_DSN='root:sync-test-only@tcp(127.0.0.1:13306)/merge_review?charset=utf8mb4&parseTime=True&loc=Local'
export TEST_POSTGRES_DSN='postgres://merge_review:sync-test-only@127.0.0.1:15432/merge_review?sslmode=disable'
GOWORK=off go test ./model -count=1 -v
export AUDIT_MYSQL_DSN="$TEST_MYSQL_DSN"
export AUDIT_POSTGRES_DSN="$TEST_POSTGRES_DSN"
export AUDIT_CLICKHOUSE_DSN='clickhouse://merge_review:sync-test-only@127.0.0.1:19000/default'
GOWORK=off go test ./controller -run '^(TestAPITokenAuditDatabaseMatrix|TestLogExportDatabaseMatrix|TestAuditDatabaseMatrix|TestIndependentAuditLogStores)$' -count=1 -v
```

上述命令全部通过，没有跳过三库分支。覆盖渠道优先级事务、分页/软删除、已有迁移、审计索引的新建和升级、重复迁移、数据/索引/唯一性保留，以及独立日志库。日志分别为 `/tmp/new-api-sync-20261022-model-matrix.log`、`/tmp/new-api-sync-20261022-audit-matrix.log`。

另用 `/tmp/new-api-sync-20261022-cache-migration.go` 验证新增缓存统计字段：三库分别覆盖 fresh、旧整合点 `ca2cae676` 的 QuotaData 结构、最新发布 rc.46 的 QuotaData 结构，共九种组合。旧结构从对应提交原样提取；rc.46 与本目标的模型代码相同。每种组合运行三次实际 `InitDB` / `InitLogDB`，保留既有行和索引，并经过内存合并、首次写入、后续增量更新及四种统计查询，检查缓存累计值超过 int32 时仍准确。全部通过；结果在 `/tmp/new-api-sync-20261022-cache-migration.log`。这覆盖代表性结构与数据，不是生产数据库备份的升级演练。

缓存迁移程序首次因 MySQL 测试库默认 latin1 被启动保护拒绝；随后创建独立 utf8mb4 `cache_review` 数据库，将上述两个 TEST DSN 中的数据库名改为 `cache_review`，运行 `GOWORK=off go run /tmp/new-api-sync-20261022-cache-migration.go`，得到全部通过结果。没有改动数据库兼容保护。结束后已删除三个临时容器及匿名卷，镜像留作缓存。

认证相关复核参考 [Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)、[Forgot Password](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html)、[Logging](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html) 和 [ASVS 5.0.0 V16](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x25-V16-Security-Logging-and-Error-Handling.md)。Resend 修复针对 **16.2.5** 的日志凭据保护，并保留 **16.3.4** 的失败可观测性及对外错误脱敏。账户证明、验证码失效/重放/次数限制、会话失效与审计脱敏沿用并运行现有后端回归；没有改变认证保证或跳过后端验证。未调用真实邮件或 AI 供应商，未执行生产迁移，也不宣称完成 ASVS 全量合规评估。

## 后续上游同步（2026-10-16）

清理提交 `030308f1e` 完成后，用户另行要求同步上游并解决冲突；此前“不自动合并新版上游”的边界至此获得了明确授权。本节记录这次独立的同步，不改写下方初审与清理阶段的历史基准。

- 合并前本地提交：`030308f1e`（`fix(relay): tighten fork compatibility patches`）。
- 原先已合入上游：`6370b29424168039e94d40d610191e7d2e65dbf4`。
- 本次固定同步目标：`ca2cae676db87333a583b6e104d0f1f7366f3258`，`fix(relaykit): emit summary on reasoning items announced without one (#7833)`，提交时间 `2026-10-16 13:44:03 +0800`。
- `git fetch --no-tags upstream main` 后，目标包含本地尚未合入的 72 个上游提交；共同基准至目标涉及 506 个文件。使用普通双亲合并，不 rebase、不重写现有历史。
- 仅更新本地代码；没有连接用户的独立部署服务器，没有推送、部署或运行生产迁移。原有未跟踪 `.codex` 不纳入提交。

### 冲突与集成处理

| 文件 | 处理结果 |
| --- | --- |
| `relaykit/dto/channel_settings.go` | 同时保留本地 `CustomProtocol` 和上游 `VertexProjectID`，两种渠道各自的配置语义保持 |
| `relaykit/relayconvert/request_registry_test.go` | 同时保留旧式 `googleSearch` 输入回归，以及上游两项 Claude 文本与工具调用转换回归 |
| `web/src/features/channels/lib/channel-form.ts` | 五处相邻新增冲突均合并双方字段；schema、默认值、读取与提交同时保留 `custom_protocol` / `vertex_project_id`；沿用各自渠道类型的字段清理规则 |
| `web/src/features/channels/types.ts` | 同时保留两项设置类型，匹配后端 DTO |
| `relay/channel/openai/relay-openai.go` | 无文本冲突，但上游 `logger.LogDebug` 改用格式化调用；本地 Cloudflare 错误日志改为固定格式串与参数，修复 vet 失败 |
| `controller/token_test.go` | 独立运行审计矩阵复现 legacy PAT 用例失败：fixture 未初始化生产启动时加载的过渡期限，曾依赖其他测试的全局状态。为临时测试库补建 `Option` 并调用已有初始化方法；不修改认证生产逻辑，原有拒绝、越权、审计脱敏断言保留 |

上一轮 MiMo metadata 保护和 Cloudflare 精确主机/无损索引修改，与 `030308f1e` 逐字节一致；两份归档工作流也逐字节一致。`service/error.go` 与本次上游目标一致，已删除的 429 归一化层没有重新引入。GHCR 发布与其余 fork 功能继续保留。

### 同步验证与环境

- `mise exec -- make test`：根模块、relaykit、tokenkit 全量后端测试通过。补齐审计 fixture 后，独立筛选运行日志/审计矩阵再次通过，见下方命令；没有削弱认证边界来让测试通过。
- 在根目录、`relaykit/`、`tokenkit/` 分别运行 `GOWORK=off mise exec -- go vet ./...` 和 `GOWORK=off mise exec -- go build ./...`：通过；relaykit/tokenkit 均独立构建，不依赖根 workspace。
- 前端 `bun install --frozen-lockfile`、`bun run typecheck`、`bun run build`：通过，锁文件保持上游版本；`bun run i18n:sync` 后没有新增工作区差异，七种语言缺键均为 0。
- 前端全量默认并行测试首次为 192 个文件、2291 项用例，其中 2 项失败（一个超时、一个通知可见性断言）。以 `bun run test -- --maxWorkers=2` 独立复核两个文件，51 项全通过；随后 `bun run test -- --maxWorkers=2` 整套复核 **192 个文件、2291 项用例全部通过**，耗时 426.49 秒。未修改生产逻辑或放宽测试断言来处理首次失败。
- 四个涉及合并的渠道前端文件单独运行 `oxlint -c .oxlintrc.json`：通过。全项目 `bun run lint` 有 **158 条 error**；在 `/tmp` 解包纯上游目标并使用同版依赖与配置独立运行 oxlint，error 的文件、规则和消息集合完全相同，合并新增 error 为 0。没有扩大同步范围去改这些上游既有问题。
- `bun run format:check` 有 **25 个文件**未通过；逐个与目标上游文件作字节比较，全部相同。本轮手工解决冲突的文件不在该列表中。检查脚本已恢复临时格式化写入，没有顺带格式化其他文件。
- 修改的 Go 文件已 gofmt；`git diff --check` 和 `git diff --cached --check` 通过，四个冲突文件及两项额外集成调整没有遗留冲突标记。

数据库测试仅使用本机 Docker 临时容器和回环地址，未挂载现有数据卷。真实版本为 **SQLite 3.50.4、MySQL 5.7.44、PostgreSQL 9.6.24**；专用 ClickHouse 日志库为 **25.8.33.6**，四库日志导出及三库独立审计存储验证通过。上游变化没有新增持久化模型字段或修改 GORM 字段标签；已有新旧结构迁移、重复迁移、数据/索引保留用例也随矩阵执行，不把通用迁移 fixture 声称为对生产备份的升级演练。

运行命令如下；口令仅用于本次临时容器，容器不用于实际部署：

```sh
export TEST_MYSQL_DSN='merge_review:merge-review-test@tcp(127.0.0.1:13306)/merge_review?charset=utf8mb4&parseTime=True&loc=Local'
export TEST_POSTGRES_DSN='postgres://merge_review:merge-review-test@127.0.0.1:15432/merge_review?sslmode=disable'
GOWORK=off mise exec -- go test ./model -count=1 -v
export AUDIT_MYSQL_DSN="$TEST_MYSQL_DSN"
export AUDIT_POSTGRES_DSN="$TEST_POSTGRES_DSN"
GOWORK=off mise exec -- go test ./controller -run '^(TestAPITokenAuditDatabaseMatrix|TestLogExportDatabaseMatrix)$' -count=1 -v
export AUDIT_CLICKHOUSE_DSN='clickhouse://merge_review:merge-review-test@127.0.0.1:19000/default'
GOWORK=off mise exec -- go test ./controller -run '^(TestLogExportDatabaseMatrix|TestAuditDatabaseMatrix|TestIndependentAuditLogStores)$' -count=1 -v
```

上述 model 全包测试和三库日志/审计矩阵通过，覆盖用户分页和软删除、配置往返、查询策略、迁移幂等、日志检索/导出，以及单独日志库的审计存储与隐私边界。MySQL 审计用例需为每个 fixture 建独立数据库，首次使用仅单库权限的测试账户失败；授予临时容器内的建库权限后继续验证，不对用户实际账户作权限调整。

所有数据库验证结束后，已删除本轮三个临时测试容器及其匿名卷。下载的镜像留作本机缓存，未操作原有容器。

认证测试参考 [OWASP Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html) 和 [ASVS 5.0.0](https://owasp.org/www-project-application-security-verification-standard/)：重点保留服务端鉴权、失效/越权拒绝和审计不记录可用凭据的现有断言。本次不是 ASVS 全量合规评估；未调用真实供应商，也未在独立部署服务器上验证。

## 确认后的实施记录（2026-10-16）

用户在逐项说明后确认“这些都可以修改”。实施基于审查 HEAD `37a8a4ca45eec250e00d436483da527398d6ff8a`，当前已合入上游为 `6370b29424168039e94d40d610191e7d2e65dbf4`，对照的最新上游快照为 `6549a17eda83eaa270fd8cbb0c03623836b351d6`。实施期间没有 fetch、合并新版上游或推送。

| 已实施项目 | 文件与结果 | 判断依据及行为边界 |
| --- | --- | --- |
| C1 / F14：工作流归档 | 两份 `docker-image-alpha.yml`、`sync-to-gitee.yml` 从 `.github/workflows/` 移至 `.github/archived-workflows/` | 当前已合入上游早已删除；本 fork 作业被仓库条件跳过。逐字节验证原文不变，保留全部归属信息；现行 GHCR 发布保持原状。实际收益是减少活动工作流目录中的遗留项，不宣称节省原本未执行的构建费用 |
| C4 / F10：撤销全局 429 归一化 | `service/error.go` 删除 deferred 归一化及三个相关函数 | 保留供应商错误内容和类型；由现有状态码、关键词、重试预算及请求约束决定重试或禁用，避免把每分钟 quota 当成余额耗尽，也避免 metadata 重复拼接。没有变更默认关键词或运行设置；中文余额关键词需管理员按需求配置。恢复已有重试规则后，部分过去立即停止的请求可能尝试备用渠道，次数仍受现有策略约束 |
| C3a / F03–F05：Cloudflare 适配收窄 | `cloudflare_gateway_compat.go` 用 URL 解析后的 HTTP(S) 精确主机匹配，补 index 改为修改原 JSON 的缺失字段 | 仅匹配 `gateway.ai.cloudflare.com`，允许大小写和端口；域名后缀、路径、查询参数或 userinfo 中出现同名字符串不会触发。原有索引关联算法保持；不新增无 ID 分片归属猜测。保留未知扩展字段，包括真实 `extra_content.google.thought_signature` 和 usage 扩展；不新增供应商请求 |
| F02：MiMo metadata 边界 | `mimo_tts.go` 在合并扩展参数前拒绝顶层 `model`、`messages`、`stream` 键 | 保护已完成的模型映射、输入转换和流式策略；键存在即拒绝，包括 `null` 和 `false`，返回 HTTP 400 / `invalid_request` 并禁止重试。其他 metadata 继续递归合并，保留显式零值和 false。已有客户端若使用上述核心键，需要迁移到 speech 标准字段；metadata.stream 不作为流式开关 |

**本轮明确保留：** Cloudflare 现有认证默认行为（`adaptor.go` 未改动），显式双凭据配置、孤立无名工具消息清理、已有开关控制的签名兼容；MiMo 语速近似、voice/format 转换、多返回格式及 usage 估算和溢出保护。二次复核中对认证适用范围的讨论没有扩展成本轮认证策略修改。未修改数据库、计费公式、relaykit/tokenkit 公共 API、前端或任务插件。

### 实施验证

- 新增回归用例在生产代码修改前运行，已复现错误主机匹配、扩展字段丢失、MiMo 核心覆盖、429 重分类及重试/禁用受阻、metadata 重复拼接；失败符合预期。
- `mise exec -- go test ./relay/channel/openai ./service -count=1`：两个包完整测试通过。覆盖保留供应商错误与配置化重试/禁用、多 choice 工具索引与扩展字段、畸形 chunk 原样返回，以及 MiMo 400 经 AudioHelper 同款错误包装后保持状态和禁止重试；原有桥接、语速、签名、工具历史测试也通过。
- 已对六个修改的 Go 文件执行 `mise exec -- gofmt -w`。
- `mise exec -- go vet ./...`、`mise exec -- go build ./...`：根模块全部包静态检查和构建通过。
- `mise exec -- gofmt -l service/error.go service/error_test.go relay/channel/openai/cloudflare_gateway_compat.go relay/channel/openai/cloudflare_gateway_compat_test.go relay/channel/openai/mimo_tts.go relay/channel/openai/mimo_tts_test.go`：无输出；`git diff --check`：通过。
- 归档与原 HEAD 文件逐字节一致，SHA256：alpha 为 `8c84e0a104afdd0da704e4e8467de1d4115e567a02c1bb7d6ddce68665fdac6b`，Gitee 为 `fd5149c1da3923b95160ff597d80b6efef43a4ae76d09838b89306d200b6f65b`。
- 没有请求真实供应商、发布 Actions 产物或执行生产迁移；测试结果不代表已验证供应商线上可用性或完整结算退款链路。没有数据库行为变更，本轮未运行三库矩阵；没有前端改动，本轮未重跑前端检查。

## 二次复核结论（2026-10-16，实施前决策）

本轮沿用下文三个固定 SHA，未 fetch 或合并新版上游。AGY 报告已完成，原始报告为 `/tmp/new-api-fork-second-review-agy-2026-10-16.md`，证据为 `/tmp/new-api-fork-second-review-agy-evidence-2026-10-16.txt`。AGY 报告会话配置显示 `Gemini 4 (High)`，底层精确模型 ID 未独立验证。Codex 随后核对了反例、调用者和两项不联网的请求转换复现；该审查阶段没有修改生产代码，后续实施见上节。

初审把部分兼容性取舍提前列为确定清理项；本轮撤回“直接删除 Cloudflare 认证推断”和“取消孤立工具消息清理”，并将 MiMo metadata 边界问题提升为明确修复候选。

| 当前范围 | 决策 | 判断依据和边界 |
| --- | --- | --- |
| C1：遗留 alpha / Gitee 工作流 | 原文归档 | 已合入上游早已删除；本 fork 作业被仓库条件跳过，未发现其他工作流依赖。全部归属信息保持原文 |
| C4：全局 429 归一化 | 删除本地归一化层 | 文字匹配强行改变错误语义，skip-retry 同时阻止备用渠道和自动禁用；恢复当前已合入上游的管理规则 |
| C3a：Cloudflare 适配；C2 认证保留 | 小范围修正 | 精确匹配兼容处理的目标主机、补索引时保留原始 JSON 扩展字段；认证保留现有行为及显式双凭据配置，不直接删除认证特判，本轮不改其适用范围 |
| F02：MiMo metadata | 修复核心请求字段边界 | metadata 不得绕过已完成的模型映射、输入检查和流式策略；优先明确拒绝核心字段覆盖，保留其他扩展字段。相关调用方需要相应调整，不能宣称零兼容影响 |
| C3b / F05：工具历史与签名 | 保持现状 | 不取消孤立无名工具消息清理，不删除已有开关控制的签名兼容 |
| MiMo 语速及多返回格式、估算兜底 | 暂缓缩减 | 缺少生产样本和同等能力替代，不在本轮凭猜测删除 |

### Codex 复核后采用与未采用的 AGY 建议

1. **认证默认行为需要保留兼容性。** 当前 BYOK 可只配网关头，双凭据可显式配两个头；直接撤掉抑制会使既有 BYOK 配置开始把渠道 Key 放进供应商 Authorization。Cloudflare 官方说明：请求自带供应商认证头时，会优先转发该值；错误占位值会导致供应商认证失败（[BYOK 凭据优先级](https://developers.cloudflare.com/ai-gateway/configuration/bring-your-own-keys/)）。没有读取生产渠道，因此不能认定现有配置已完成迁移。
2. **不采用 AGY 的“仅凭 `{api_key}` 模板判断 BYOK，且零迁移”方案。** 字面量网关头也可以用于 BYOK；模板是否出现不能证明凭据属于哪种模式。该新推断仍可能改变现有 BYOK 行为。优先限定实际主机并保留原有 Cloudflare 行为，不另造认证模式猜测。
3. **静态空 Authorization 有效，但不能泛化为所有覆盖路径。** `SetupRequestHeader` 会因静态配置中出现 Authorization 键而不注入默认值；后续空值虽被过滤，默认值此前已被抑制。运行时覆盖经过清洗且 adaptor 仍只看静态 map，单纯动态写空值无法获得相同保证。AGY 对动态路径的提醒成立，但“空值在当前静态配置完全无效”并不成立。
4. **保留孤立消息清理是兼容性判断，不是已验证的线上成功率。** [Gemini FunctionResponse 协议](https://ai.google.dev/api/generate-content#FunctionResponse) 要求函数名，原提交和现有回归也保护无名孤立消息容错；足以说明取消清理会改变既有兼容行为。本轮没有真实请求 Cloudflare，不采信原报告“线上必然 100% 失败”“某些客户端经常产生该请求”等未给运行证据的绝对说法，也不新增猜测工具名称的回退。
5. **SSE 丢扩展字段的条件可以由代码确定。** 缺 index 时整块经 `ChatCompletionsStreamResponse` 重编码，而 `ToolCallResponse` 不含 `extra_content`，输入块若有真实签名会丢失。采用只补字段的修正。AGY 建议的“无 ID 且无 index 时总归给最近调用”和“同 ID 不同函数名即视为新调用”缺少可靠关联依据，不作为本轮确定改法；并行交错分片不能凭最近位置恢复身份。不把两个历史提交的时间相近解释为已证明的作者动机。
6. **429 的额外问题和运营边界。** 已检查 `WithOpenAIError`，重复调用可把 metadata 重复附加到错误消息。默认禁用关键词只有英文，恢复原规则不会自动识别所有中文余额错误；管理员如有此需求，可用现有设置配置。中文自动禁用不是删除代码的必需前置条件：该补丁此前也会因 skip-retry 阻止自动禁用。本轮不顺带改默认关键词、数据库或运行设置。
7. **MiMo 核心字段风险不必等厂商返回样本才能确认。** 已复现 metadata 改写模型、输入和流式标记；还检查了请求预估、转换和失败退款调用链。接受优先修复边界的建议。未实际调用厂商或执行完整退款集成测试，不将其表述为已观测到“任意高价模型均可调用”“用户已免费获得音频”或“全额退款始终必然发生”。

### 审查阶段独立复现与当时状态

- `mise exec -- go run /tmp/new-api-auth-header-review-2026-10-16.go`：通过真实 `Adaptor.SetupRequestHeader`、`channel.ResolveHeaderOverride` 和对应 header 应用顺序检查六种情况；仅使用虚构测试值，不联网、不打印凭据。结果：普通默认认证存在；BYOK 省略 Authorization；显式双凭据并存；静态空 Authorization（含无 CF 头的隔离对照）抑制默认认证；非 Cloudflare 主机带 CF 头时也被错误抑制。后者支持收窄作用范围。
- `mise exec -- go run /tmp/new-api-mimo-metadata-review-2026-10-16.go`：真实调用 `Adaptor.ConvertAudioRequest`，结果为 `mapped model preserved=false; outbound stream=true while relay stream=false; input preserved=false`。这是请求转换复现，不是供应商或结算集成测试。
- AGY 首次连接因环境服务重启失效，原 CLI 进程结束且报告未写出；确认后使用 `--continue` 续接，等待至报告和进程完成。没有因正常静默而中断任务或并行另起复核。
- 当时仅更新复核文档，生产代码、CI、测试和运行配置未修改；之后用户已确认四项范围，实施和验证记录见文首。

## 基准与方法

- 项目：frondesce/new-api；上游：QuantumNous/new-api。
- 本地 HEAD：`37a8a4ca45eec250e00d436483da527398d6ff8a`，合并标题为 `Merge upstream v1.0.0-rc.42`。
- **当前已合入的上游基准**：`6370b29424168039e94d40d610191e7d2e65dbf4`。以 `git merge-base HEAD upstream/main` 确认，不仅依赖合并标题。
- **审查时最新上游快照**：`6549a17eda83eaa270fd8cbb0c03623836b351d6`，2026-10-15，`fix(dashboard): draw flow chart labels in the theme text color (#7821)`。本次只 fetch 跟踪引用，没有合入。
- 两条分支各自独有的提交为本地 60 个、上游 70 个；本地独有的非合并提交 23 个。按实际功能分组，不能当成 60 项功能。
- 已合入基准到 HEAD 的实际树差异为 **44 个文件、4,178 行增加、71 行删除**。这才是现存定制的主要范围。`HEAD..最新上游` 的大量通用更新不属于本地独立功能。
- 第一阶段由 AGY 只读粗筛；第二阶段由 Codex 自行读取提交、diff、入口及调用者，并检查有争议的分支与现有回归测试。AGY 原始产物和建议差异见后文。
- AGY 按 8 个功能组归纳；下表拆成 14 个可分别决策的子项（例如 Cloudflare 的索引、消息清理、签名和认证），不是 14 项互不相关的新功能，也不重复累计提交或文件。
- “调用”默认指代码可达和测试覆盖；没有读取生产数据库、密钥、访问日志或渠道配置，不把静态可达说成生产正在使用，也不把缺少观测说成死代码。

## 2026-10-15 初审逐项结论（部分已由二次复核修订）

| 编号 | 功能或定制 | 当前已合入上游 / 最新快照 | Codex 结论 |
| --- | --- | --- | --- |
| F01 | OpenRouter 图片编辑协议适配 | 两者均无对应适配 | 保留功能与协议测试 |
| F02 | MiMo `/audio/speech` → Chat TTS 桥接 | 两者均无对应桥接 | 保留核心；收窄不受控覆盖，其他兜底需样本 |
| F03 | Cloudflare 流式 tool-call index 修补 | 两者均无同路径修补 | 保留；限定真实主机，只修改缺失字段 |
| F04 | Cloudflare Gemini 工具历史清理 | 原生 Gemini 转换不等于此 OpenAI 兼容路径 | 保留名称回填；建议取消静默删除消息 |
| F05 | Cloudflare Gemini thought-signature 占位注入 | 上游有原生格式逻辑，但未覆盖此路径 | 暂留现有开关；不能因常量相同而删除 |
| F06 | 出现 `cf-aig-authorization` 就抑制默认 Authorization | 两者均保留供应商认证与显式 override 机制 | 删除这项隐式推断，恢复显式配置 |
| F07 | Custom 渠道 Gemini/Vertex 完整 URL 与协议选择 | 两者均无同等完整 URL 模板能力 | 保留前后端整条链路 |
| F08 | 旧式 `tools: [{googleSearch:{}}]` 输入 | 上游支持其他工具表示，未覆盖该输入分支 | 暂留兼容；不自动引入搜索 |
| F09 | Gemini 来源链接及标题扩展 | 主体引用转换已覆盖；source-only 与标题降级仍未覆盖 | 保留残余小补丁，不重复维护主体转换 |
| F10 | 429 配额/限流文字分类与 skip-retry | 两者均无本地归一化 | 删除全局归一化补丁，恢复上游可配置重试 |
| F11 | 用户列表默认隐藏软删除用户 | 两者均仍使用不同的 Unscoped 行为 | 保留；属于有意产品行为差异 |
| F12 | fork GHCR 发布及 Compose 镜像变量 | 上游发布目标和触发条件不同 | 保留；可单独压缩维护成本 |
| F13 | 上游 Docker/Electron 作业的仓库隔离 | 当前与最新上游均未提供这些 fork 隔离条件 | 保留必要条件；已消失的旧门禁不再维护 |
| F14 | 遗留 alpha 与 Gitee 工作流 | 已合入上游早已删除，最新上游亦不存在 | 可以原文移出工作流目录归档 |

### F01 — OpenRouter 图片编辑：保留

- 提交：`4aac3567e`；`37a8a4ca4` 合并时保留。
- 文件：`relay/channel/openai/openrouter_image.go:33`、`adaptor.go:565`、`openrouter_image_test.go`、`docs/openrouter-image-edits.md`。
- 调用链：`relay.ImageHelper` → OpenAI adaptor 的 `ConvertImageRequest`，仅 OpenRouter 渠道且编辑模式进入；转换成功后使用 `/v1/images` 和 JSON 发送，响应复用普通图片处理与结算。原始请求透传不会进入转换。
- 当前与最新上游均没有本地转换器及 `openRouterImageEdit` 路由状态；不能以通用图片接口存在为由删掉。
- 正常每次一次供应商生成请求；文件仅在本地编码，HTTP 参考图交由 OpenRouter 使用，本地适配不逐张下载。全局渠道重试可能增加供应商调用，但此功能没有自带探测或失败后切换协议的第二次请求。
- `n` 上限、模型映射、显式零值、multipart 复用、响应图片数量、拒绝不支持参数都是有效契约。1,215 行测试并非自动等于冗余；不按行数删测试。
- [OpenRouter 官方协议](https://openrouter.ai/docs/guides/overview/multimodal/image-generation) 使用 `input_references`，支持本地适配的核心设计。没有执行真实付费图像请求；生产连续编辑仍需部署验证。

### F02 — MiMo TTS：保留桥接，限制隐式行为

- 提交：`1a4cc61b0`、`901c4cef6`、`b90a6bed5`，作为同一功能计算。
- 文件：`relay/channel/openai/mimo_tts.go`、`mimo_tts_test.go`；`adaptor.go:206,227,494,824` 接入 URL、认证、请求和响应。
- 入口：speech 模式且上游模型匹配 `mimo-v<版本>-tts...`；Azure/Custom 等 URL 特殊分支不全经过默认 URL 改写，不能称为任意渠道均正确适用。上游普通音频转发无法替代 Chat TTS 协议。
- 核心：文本及指令组成消息、`api-key`、格式映射、`choices[0].message.audio.data` 解码、usage 优先使用上游，缺失时估算。正常一次生成请求。
- [MiMo 官方文档](https://mimo.mi.com/docs/en-US/quick-start/usage-guide/audio/speech-synthesis-v2.5) 仍提供 Chat 风格语音合成及多个 TTS 变体；不能无样本就删掉整个桥接。
- 当前厂商文档已经说明基础 TTS 的低延迟流式接口恢复；本地桥接仍拒绝流式，是此适配的能力边界，不能据此声称厂商本身不支持流式。支持流式属于另项功能工作，本轮不扩大范围。
- 具体缩减点：`mimo_tts.go:104–109,174–188` 把客户端 metadata 递归覆盖最终请求，能覆盖 `model`、`messages`、`stream`。这可能使实际请求与路由/预估身份不同；建议拒绝核心字段覆盖，保留确有协议依据的扩展参数。
- `Speed up` / `Slow down` 把连续 speed 参数离散成两个提示词（`:133–170`），属于行为近似；是否取消需使用方确认，不能假装是精确语速控制。
- 多种音频封装、URL 重定向、按时长/文件大小估算均有兼容目的；未找到生产样本证明这些分支使用或废弃。URL 分支是给客户端 302，不是服务端额外抓取。建议暂留，先取得返回样本再删除。
- 后续若改 usage 推导，必须继续满足 quota 饱和及审计规则；不能为了变短删除现有溢出保护。

### F03 — Cloudflare 流式工具索引：保留并缩小修改面

- 提交：`3c4452c5d`，后续合并迁移保留。
- 文件：`relay/channel/openai/cloudflare_gateway_compat.go:16–81,211–242`、`relay-openai.go:125–148` 及同名测试。
- 入口：OpenAI Chat SSE，Base URL 字符串包含 `gateway.ai.cloudflare.com`；每个 chunk 在输出和使用量观察前归一化。它不是原生 Gemini handler 的索引分配逻辑，后者不会执行此路径。
- 没有额外 HTTP；收益是让缺失 index 的工具调用能被客户端正确拼接。现有 index 不应覆盖。
- 问题：主机使用子串判断；修补时整个 chunk 经有限 DTO 解码再编码，可能删除 DTO 不认识的扩展字段。建议解析实际 hostname，使用现有 JSON 修改能力仅补缺失 index，保留扩展字段，不引入通用新工具框架。
- 没有供应商实时样本证明目前已修复；Cloudflare 新推荐的 API hostname 也不匹配此旧兼容路径，不能推断旧路径已经无人使用。

### F04 — Cloudflare 工具历史清理：保留名称回填，取消静默丢弃

- 提交：`5c4187fc0`。
- 文件：`cloudflare_gateway_compat.go:82–150`、`adaptor.go:468`；测试明确断言孤立工具消息被丢弃。
- 入口：非透传的 OpenAI Chat 转换，Cloudflare URL 加 Gemini 模型名匹配。一次本地改写，不新增请求。
- 从前面的 assistant tool call 按 ID 回填 `name` 是有依据的协议补全；原样保存消息内容即可。
- 无法推导名称时直接 `continue` 会删除客户端提交的历史，且没有独立开关。建议保留该消息交由原有上游校验，停止静默删消息。结果可能由“请求成功但丢上下文”变为明确上游失败，这是需要确认的行为变化。
- 不把原生 Gemini 转换器处理工具历史的代码误认为已覆盖该 OpenAI 兼容路径。

### F05 — Cloudflare thought signature：现阶段保留

- 提交：`f154ebe67`。
- 文件：`cloudflare_gateway_compat.go:98–106,153–209`、`adaptor.go:474`；配置为既有 `FunctionCallThoughtSignatureEnabled`。
- 上游 `relaykit/relayconvert/internal/shared/gemini/request.go:64` 已有相同 bypass 值，但输出是原生 `GeminiPart.ThoughtSignature`；本地处理 OpenAI tool call 的 `extra_content.google.thought_signature`，路径和字段不同。
- 不替换非空签名、不增加供应商请求；受现有开关约束。重复的是字符串常量，不是整项行为，不值得为去掉一个常量扩大独立模块的 API。
- [Google 当前 thinking 文档](https://ai.google.dev/gemini-api/docs/thinking) 强调保留签名，但原先 thought-signatures 页面已迁移，未在当前页面找到这个历史 bypass 字符串。此资料不能证明旧 Gemini/Cloudflare 兼容路径可删除；具体模型实时验证留作证据缺口。

### F06 — Cloudflare 鉴权头推断：建议删除增量

- 提交：`git blame` 确认来自 `1a4cc61b0` 的附带变更；位置 `adaptor.go:47–57,240–244`。它不属于 MiMo 专用的 `api-key` 分支，不能跟着桥接功能一起笼统判保留。
- `shouldSkipDefaultAuthorizationHeader` 在任何渠道的 override 中发现 `cf-aig-authorization` 就阻止默认供应商 Bearer 头；没有验证域名或 BYOK 设置。
- [Cloudflare OpenAI 官方示例](https://developers.cloudflare.com/ai-gateway/usage/providers/openai/) 区分供应商凭据与网关凭据；非 BYOK 可以同时需要两者。单个网关头的存在不能证明供应商密钥已经存于 Cloudflare。
- 已有 `relay/channel/api_request.go:266–335` 在 adaptor 设置头之后应用显式 override；当前上游也保留此机制。建议恢复上游的 Authorization 显式 override 判断，移除新增的 `cf-aig-authorization` 自动推断；BYOK 使用者显式配置 Authorization。
- 不额外发请求；收益是减少不正确的认证猜测，代价是依赖现有隐式行为的 BYOK 配置需要调整。确认范围时应包含这项配置迁移。
- 已查阅 OWASP Authentication、Session Management Cheat Sheets 和稳定 ASVS **5.0.0**。此项只审查上游凭据选择；后续实现验证两种凭据相互独立、显式 override 优先、客户端凭据不被意外转发、不记录可用密钥。未在本轮对整个认证系统作 ASVS 合规认证，也未给未经验证的条款 ID 背书。

### F07 — 自定义 Gemini/Vertex 协议：保留完整链路

- 提交：`5ae888b3b`、`d458c371e`、`dbb61afc0`；旧 classic UI 已随上游移除，当前保留的是新前端。
- DTO：`relaykit/dto/channel_settings.go:95,112,140`；渠道选择 `relay/common/relay_info.go:309`；校验 `controller/channel.go:639`；渠道测试 `controller/channel-test.go:274`。
- 运行入口：Custom 类型 + `custom_protocol=gemini_vertex` → Gemini adaptor → `{model}/{action}` 完整 URL、流式 action 与 `alt=sse`、显式 header override → 原有 Gemini 转换与响应。
- `relay/compatible_handler.go:77–103` 必须避免 Chat→Responses 或原始 OpenAI body 透传绕过原生协议转换；这些差异不能当成多余判断逐个删除。
- 前端 `channel-mutate-drawer.tsx:3611`、`channel-form.ts`、`types.ts` 和七种 locale 都仍有读写链路。字段不是死配置；表单复用已有 Form/Select/Input，没有发现需要另建组件的能力缺口。
- 最新上游 `a0bf6a8a4` 的 Vertex project_id 修复和 `718a4e7f9` 的区域 hostname 修复服务于内置 Vertex 接入，不能替代任意完整 URL 模板；二者也尚未合入当前分支。
- 正常一次生成请求；无自动探测。若生产已全部迁到内置 Vertex，可作为后续整体删除候选，但必须先证明配置已迁移。

### F08 — 旧 googleSearch 工具输入：保留少量兼容

- 来源：`5ae888b3b` 的自定义 Gemini 协议适配同时增加该 DTO 字段；`37a8a4ca4` 合并时保留字段并接入新 relaykit 工具解码。与 F07 是同一功能的兼容子项，单列只为决定是否仍需支持旧输入。
- 文件：`relaykit/dto/openai_request.go:407`、`relayconvert/internal/toolconv/decode.go:58–63`、`request_registry_test.go:16`；`custom_vertex_test.go:121`。
- 把旧 `tools:[{googleSearch:{}}]` 归为已有 hosted web search tool。上游另有伪 function 名、原生工具等编码，均不等价于这个 JSON 输入分支。
- 客户端明确声明搜索才触发，不主动给普通请求增加搜索；搜索费用仍由既有用量和计费链路处理。删除会破坏现有客户端输入，节省代码很少。
- GoogleSearch map 的具体配置是否被完全保留并非本轮通过一个空对象测试就能证明；不据此宣称所有搜索配置兼容。

### F09 — 引用元数据：主体已上游化，残余仍有用

- 提交：`2bed32577`、`3ee170250`；上游 `0ed497f06` 已引入共享 hosted-tool/引用转换，当前基准已经包含。
- 本地残余：`relaykit/relayconvert/internal/gemini_chat/grounding.go:18,89–99,196–211,293–348` 和两项回归测试。
- 实际入口：Gemini → Chat 普通响应和有状态流式转换。保留没有 `groundingSupports` 的来源链接，标题按 title → domain → URI 降级，按候选与来源去重；有真实文本范围时沿用其位置。
- 当前与最新上游在该文件没有新变化，仍缺这些残余行为。不存在第二套完整 Gemini 引用实现可整项删除。
- 纯元数据转换，无来源网页抓取或额外模型请求。source-only annotation 不含 start/end，属于兼容扩展；严格客户端是否接纳需要实际集成样本，不能编造文本范围来满足 schema。

### F10 — 429 分类：建议删除全局归一化补丁

- 提交：`171e6b613`。
- 文件：`service/error.go:87–90,144–210`、`service/error_test.go:124`。
- 通用 `RelayErrorHandler` 处理任意供应商 HTTP 429，拼接 message/type/code 做子串匹配，`quota exceeded` 等即转换 `insufficient_quota` 并附 skip-retry。
- 后果不止提示文案：`service/relay_error.go:38` 会停止此请求的渠道重试；`service/channel.go:67` 对 skip-retry 直接不自动禁用。因此不能把它描述成只节省一次对同一渠道的请求；耗尽渠道也可能持续留在选择池，健康备用渠道得不到尝试。
- 模糊 `quota exceeded` 也可以表示每分钟额度/速率限制；文字推断不足以决定不可重试。最终建议删除整个本地 `normalizeUpstreamRateLimitError` 及其专属辅助函数/调用，恢复已合入上游 `RelayErrorHandler` 的错误字段保留行为，不再强加前缀。明确的供应商 type/code 仍由已有解析器保留，重试/禁用服从已有全局规则和管理配置，不新增一套分类层。
- 减少重试能降低延迟及请求次数；恢复备用渠道可能增加失败后请求，但不能一概认定会重复计费。该分支针对上游明确拒绝的 HTTP 429；真正已接受的任务/SSE 有各自不可重试约束，不应一起放开。
- 最新上游仍无本地归一化器，不能说是已被最新修复取代。现有测试验证当前行为，不证明该全局产品策略正确；清理时应改为覆盖临时限制、明确账户余额、备用渠道、自动禁用和错误字段保留。

### F11 — 用户软删除：保留

- 提交：`a51cac5f9`。
- 文件：`model/user.go:415–426,456–483`、`user_pagination_test.go:69–149`；入口 `controller/user.go:337,353`。
- 普通列表/搜索默认排除软删除；显式 `status=-1` 用 Unscoped 取回已删除用户。Count 与分页查询共用同一过滤，避免数量和行不一致。
- 当前与最新上游仍存在此产品行为差异；后台可以显式查询删除状态。不是不能恢复的死代码，也不是登录状态修复。
- 不增加供应商请求。此次没有修改数据库代码，也没有重新执行 MySQL/PostgreSQL 实库矩阵；不能把现有可跳过测试当成本轮三库验证。若确认清理涉及此处，须实测三库并记录版本。本轮建议不动。

### F12 — GHCR 和 Compose：保留

- 提交：`b0d0725c3`、`124b9beb8`；文件 `.github/workflows/docker-image-ghcr.yml`、`docker-compose.yml:19`。
- main push、tag 和手工触发，用当前仓库名发布 GHCR，`NEW_API_IMAGE` 保持上游镜像为默认值。它是 fork 自身发布入口，不等于上游 Docker Hub 作业。
- 每次 main push 构建两个架构，主要成本是 CI 构建时间；是否减少自动发布频率是发布策略选择，不由静态“重复工作流”自动决定。
- 可后续缩减：确实未使用的 dispatch 参数、全量 checkout、QEMU 构建策略等需以运行时间和发布需求证据评估；不为本轮审查顺带重构发布架构。
- 已核对 GitHub 公共运行记录：2026-10-09 的当前 HEAD 构建 [37882273511](https://github.com/frondesce/new-api/actions/runs/37882273511) 成功，最近 20 条仓库记录均为这份 GHCR 工作流。因此它不仅静态可达，而且有实际使用证据，不能当作重复 CI 删除。没有执行构建发布、登录 registry 或推送镜像。

### F13 — 上游作业隔离：保留

- 提交：`7e2c63f3f`，`09294d768` 恢复 fork tag 的普通 Release。
- 当前增量只在 `.github/workflows/docker-build.yml:18,109`、`electron-build.yml:13,126` 的仓库条件和作业名称。
- 上游 Docker 发布依赖上游 registry secrets；Electron 无 fork 发布需求时跳过合理，正常 Release 则仍可运行。当前 `release.yml` 与已合入上游无差异，不能再删一个不存在的“Release 禁用补丁”。
- 仅有隔离条件不等于文件不被 GitHub 登记；API 的 `active` 状态也不等于作业实际执行，须同时看 `if` 与 runs。

### F14 — 已失效的历史工作流：原文归档

- 文件：`.github/workflows/docker-image-alpha.yml`（181 行）、`sync-to-gitee.yml`（91 行）。
- 上游 `55858f353` 在 2026-07-03 已删除这两份文件并引入 branch 手工 Docker 发布；它是 **当前基准已有的上游变更**，不是本轮最新上游才出现的替代。
- 本地仍保留上游专用 `github.repository == 'QuantumNous/new-api'` 条件；此 fork 中作业不执行。alpha 分支触发和 Gitee 的 `sync` runner / secrets 也不服务 fork 的 GHCR 发布。
- 这是可证实的失活定制，不等于整项 CI 应移除。建议逐字节移至 `.github/archived-workflows/`，从 Actions 识别目录移出，但保留全部项目/组织标识、归属、URL 和原始内容；不直接删文件或删品牌信息。
- 节省的是工作流发现和后续合并维护成本；现状作业已跳过，不夸大成能省下实际运行的 272 行对应构建费用。

## 历史项：已经被吸收或消失，不重复清理

| 旧提交 | 内容 | 当前处理 |
| --- | --- | --- |
| `fa9266f55` | AGENTS/CLAUDE 项目说明 | 与基准没有树差异；是共享项目规则，不删除 |
| `ed349f316` | Claude 编译、classic 构建和 locale 合并修复 | 已被后续上游重构覆盖，无独立残余 |
| `dbd6d5b19` | classic date-fns 路径 | classic 目录已移除，无现存补丁可删 |
| `d24b58868` | Bun lock 同步 | 当前锁文件与基准相同；不回退依赖 |
| `8d8bbbf49` | advanced custom model 大小写 | 已合入上游 `e8596cab7` 改为精确大小写匹配；当前 `multi-select.tsx:170–176` 已具该行为且与基准相同。不是仅因目录迁移就断言原问题消失 |
| `d458c371e` | classic 的 Gemini/Vertex 选项 | classic 已删除；现有新 UI 属于 F07，不能一并清除 |
| `09294d768` | 恢复 fork 普通 Release | 当前 release.yml 与基准相同，无需保留额外补丁 |

23 个独有非合并提交的覆盖：F01 1 个、F02 3 个、F03/F04/F05 各 1 个、F07 3 个、F09 2 个、F10 1 个、F11 1 个、F12 2 个、F13 2 个、上表剩余独立历史项 5 个，共 23 个；F06/F08/F14 是相关功能及合并保留的树差异，不能仅靠非合并提交标题发现。

## 2026-10-15 原建议清理边界（不再整体执行）

建议先确认以下四项；均不需要合并新版上游：

1. **C1 / F14：归档**两份已失活工作流，原文和归属不变。
2. **C2 / F06：移除** `cf-aig-authorization` 自动抑制 Authorization 的推断，保持显式 override；依赖 BYOK 隐式抑制的配置改为显式声明。
3. **C3 / F03–F04：收窄** Cloudflare 主机识别和 JSON 修改范围，保留缺失 index/名称回填，取消静默丢历史。现有 signature 开关继续保留。
4. **C4 / F10：删除**本地全局 429 归一化补丁及文字前缀，恢复当前已合入上游的解析与可配置重试/禁用规则；保留既有请求/任务接受后的不可重试边界，调整策略回归测试。

F02 metadata 的核心字段保护可列为下一项，但需要先确认兼容方使用的扩展参数；语速近似、非标准返回格式和估算不在无样本删除范围。F07/F08/F09 若使用方已迁移，可另行整体缩减；目前证据不支持直接删除。`relaykit/`、`tokenkit/`、moejs 插件体系、新前端与表达式计费主体均已包含在上游基准，不作为 fork 独立功能重复计数或拆除。

清理后才进行对应测试调整与最终格式/静态检查；涉及 relaykit 要在模块内 `GOWORK=off go build ./...`，涉及前端改动要执行类型、lint、构建与相应回归，涉及数据库行为要真实三库矩阵。提交信息遵循近期英文 `fix(scope): ...` / `ci: ...` 惯例，未经要求不推送，不自动合并上游。

## 验证与局限

本节只记录已观察到的执行结果，后续清理验证应追加，不能覆盖本轮基准。

- `git fetch --no-tags upstream main`：成功，仅刷新 upstream/main。
- `git merge-base HEAD upstream/main`、`git log --no-merges upstream/main..HEAD`、`git diff --stat 6370b294 HEAD`、逐组 `git diff/show/rg`：确认上述基准、23 个非合并提交和 44 个文件差异。
- Go：`mise exec -- go version` 为 **go1.26.0 linux/amd64**；relaykit 使用声明兼容的 **go1.25.1** 工具链。
- `mise exec -- go test ./relay/channel/openai ./relay/channel/gemini ./service -run 'Test(OpenRouterImage_|.*MiMo|.*CloudflareGateway|RelayErrorHandler.*RateLimit|CustomGemini)' -count=1`：三个包通过。
- 在 `relaykit/`：`GOWORK=off go test ./relayconvert/internal/gemini_chat ./relayconvert -run 'Test(Grounding|ConvertRequestPreservesLegacyGoogleSearchTool)' -count=1`：两个包通过。
- `mise exec -- go test ./relay/common ./controller -run 'Test(InitChannelMeta.*Custom|ValidateChannel.*Custom)' -count=1`：两个包通过。
- 初次 Go 测试在沙箱内因 `~/.cache/go-build` 只读而无法启动；已获准在沙箱外重跑，上述通过结果来自重跑。
- 前端：首次渠道测试因缺失已在锁文件中的 `node-forge` 无法导入；`bun install --frozen-lockfile` 补齐两项依赖，锁文件未改动。重跑 `bun run test -- src/features/channels/components/__tests__/channel-configuration.test.tsx`：**88 项通过**。
- Actions：`curl --fail --silent --show-error --max-time 20 'https://api.github.com/repos/frondesce/new-api/actions/workflows?per_page=100'` 和相同仓库 `/actions/runs?per_page=20`：成功。登记 9 份 active 工作流，最近 20 条 run 全部为 GHCR；当前 HEAD 的那次成功。未获取到其他工作流使用证据不代表历史从未执行。本机没有 `gh`，通过只读 curl 获取，不访问 secrets。
- 未调用真实供应商、未执行数据库变更或三库矩阵、未发布 CI 产物；本轮结果不表示生产使用率、供应商实时兼容或全项目安全合规已验证。
- OWASP 参考：[Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)、[ASVS 5.0.0](https://owasp.org/projects/asvs)。本轮审查未修改登录、会话或令牌生命周期。

## 2026-10-15 AGY 粗筛与 Codex 初审差异（历史）

AGY 已完成。原始报告 `/tmp/new-api-fork-audit-agy-2026-10-15.md`，证据 `/tmp/new-api-fork-audit-agy-evidence-2026-10-15.txt`，均固定相同 HEAD/Base/最新上游 SHA。报告按 8 组逐一列出 23 个非合并提交和 44 个差异文件；Codex 另用当前 `git diff --name-only` 核对 44 个路径，没有遗留未分类文件。以下为初审相对粗筛的实质修正，其中认证和孤立消息判断已由文首二次复核修订：

| AGY 粗筛 | Codex 复核与最终采用结论 |
| --- | --- |
| Cloudflare 组建议保留鉴权头互斥 | 不采用。官方非 BYOK 示例明确同时有两种凭据；F06 列为删除候选 |
| 建议剥离或新加开关控制签名注入 | 不直接删除，也不重复加开关。已有开关，原生上游逻辑不覆盖本地字段；F05 暂留 |
| 所有 fork CI 整体保留 | 拆开。GHCR 有实际成功记录，隔离条件有用；alpha/Gitee 是已合入上游删过的遗留文件，F14 可原文归档 |
| MiMo 整体保留、侵入性很低 | 保留桥接，但补充 metadata 覆盖核心请求、离散语速、模型匹配范围和 usage 兜底的维护风险；未接受“无计费漏洞”之类绝对判断 |
| 429 去前缀并重新评估停止重试 | 结合 `DecideRelayRetry` 和 `ShouldDisableChannel`，建议直接撤掉这套全局归一化，恢复已有可配置规则 |
| 大小写修复因 web/default 路径消失而失效 | 进一步核实上游 `e8596cab7` 和当前实现，确认核心行为已被覆盖；路径重命名本身不能证明替代 |
| 用户列表具备严密三库回归覆盖 | 只确认存在可选 DSN 的测试代码，不把它当成已运行三库。此次未改此处，不宣称三库验证完成 |
| 前端只读约束导致没有运行证据 | Codex 在授权工作区补齐锁定依赖后独立运行，88 项通过 |

AGY F-01 → F01；F-02 → F02（附带认证增量另列 F06）；F-03 → F03–F06；F-04 → F07/F08；F-05 → F08/F09；F-06 → F10；F-07 → F11；F-08 → F12–F14。共享子项按提交和文件去重，非新增重复功能。

## 当前 44 个差异文件的覆盖清单

下列清单以审查 HEAD 为准，清理后原路径的删除或归档不反过来修改本轮范围。

| 归属 | 差异文件（完整路径） |
| --- | --- |
| F01 | `relay/channel/openai/openrouter_image.go`；`relay/channel/openai/openrouter_image_test.go`；`docs/openrouter-image-edits.md` |
| F02 | `relay/channel/openai/mimo_tts.go`；`relay/channel/openai/mimo_tts_test.go`（另含 F07 URL 拒绝用例） |
| F03–F05 | `relay/channel/openai/cloudflare_gateway_compat.go`；`relay/channel/openai/cloudflare_gateway_compat_test.go` |
| F03 | `relay/channel/openai/relay-openai.go` |
| 多组接入 | `relay/channel/openai/adaptor.go`（F01/F02/F04/F05/F06/F07） |
| F07 | `controller/channel-test.go`；`controller/channel.go`；`controller/channel_validation_test.go`；`relay/channel/gemini/adaptor.go`；`relay/channel/gemini/custom_vertex_test.go`（含工具/响应测试）；`relay/common/relay_info.go`；`relay/common/relay_info_test.go`；`relay/compatible_handler.go`；`relaykit/dto/channel_settings.go` |
| F07 前端 | `web/src/features/channels/components/__tests__/channel-configuration.test.tsx`；`web/src/features/channels/components/drawers/channel-mutate-drawer.tsx`；`web/src/features/channels/lib/channel-form.ts`；`web/src/features/channels/types.ts` |
| F07 翻译 | `web/src/i18n/locales/en.json`；`web/src/i18n/locales/fr.json`；`web/src/i18n/locales/ja.json`；`web/src/i18n/locales/ru.json`；`web/src/i18n/locales/vi.json`；`web/src/i18n/locales/zh-TW.json`；`web/src/i18n/locales/zh.json` |
| F08 | `relaykit/dto/openai_request.go`；`relaykit/relayconvert/internal/toolconv/decode.go`；`relaykit/relayconvert/request_registry_test.go` |
| F09 | `relaykit/relayconvert/internal/gemini_chat/grounding.go`；`relaykit/relayconvert/internal/gemini_chat/grounding_test.go` |
| F10 | `service/error.go`；`service/error_test.go` |
| F11 | `model/user.go`；`model/user_pagination_test.go` |
| F12 | `.github/workflows/docker-image-ghcr.yml`；`docker-compose.yml` |
| F13 | `.github/workflows/docker-build.yml`；`.github/workflows/electron-build.yml` |
| F14 | `.github/workflows/docker-image-alpha.yml`；`.github/workflows/sync-to-gitee.yml` |
