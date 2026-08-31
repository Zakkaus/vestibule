<!-- The per-file source of truth for the rewrite. The plan cites it; it lived
     outside the repository until an agent working from the plan reported that it
     could not open what the plan told it to follow. -->

# v5 代码清点与逐文件去向

覆盖 46 个非测试 `.go` 文件和 88 个 `_test.go` 文件。范围沿用任务给出的 21,434 行非测试代码与 25,569 行测试代码。

命名遵循 `web/architecture.html` 的详细目录和本任务指定的 `console`。`docs/ARCHITECTURE.md` 中的 `adminhttp` 是同一 HTTP 边界的较早名称；不新设平行包。

处置含义：

- `移入`：现有代码可整体进入目标职责。
- `拆分`：按列出的行段进入多个目标职责。
- `重写`：当前依赖方向、状态模型或控制流违反目标架构；保留行为，不搬实现。
- `删除`：目标架构不存在对应协议或存储介质。
- `实现锁定`：测试断言 JSON 字段、telego 调用形状、callback 编码或内存时序，而非用户可观察结果。

## 一、逐文件去向

| 现在的路径 | 行数 | 它实际做什么 | 目标包 | 处置 |
|---|---:|---|---|---|
| `internal/verify/service.go` | 2859 | 单体验证服务：配置读写、内存 pending 队列、`time.AfterFunc`、Telegram Update 处理、题目投递、管理员按钮、批准/拒绝/封禁和 JSON 保存。 | `verification`；`telegram/updates`、`telegram/tgfmt`；`settings` | **重写**：1–580 混合构造、配置和 telego；582–2823 把内存状态、外部副作用和结算混在一起；2825–2859 是题目随机化。不能移入：直接出现 telego 类型，以内存锁和计时器代替条件更新，并在对外投递后补全状态。保留两种入群模式、可信群绕过、冷却、群/私聊/双投递、确认投递后计时、nonce/epoch 防旧事件、管理员结算、失败不误罚、挑战清理和不重复验证。 |
| `internal/verify/state.go` | 1106 | AI tripwire 统计、失败次数和冷却、pending/heartbeat 的 JSON 保存与恢复、进程内超时、掉线探测和恢复重投递。 | `rules`；`verification`；`database`；`telegram/tgfmt`；`status` | **重写**：21–177 可提炼为结构信号；179–593 是 JSON 状态；595–782 是进程内到期与延期；784–848 直接渲染 Telegram；850–1106 是心跳恢复。目标使用 `challenge`、`challenge_message`、条件领取和 durable outbox。保留失败窗口、冷却、成功清除 strike、掉线不计 strike、48 小时延期上限、恢复前核验成员状态、替代挑战送达后删除旧消息。 |
| `internal/verify/kernel.go` | 853 | 内核版本题、fallback 题、AI trap、答案归一化与判定；同时处理 DM Update、设置读取、题面 HTML 和结算。 | `rules/condition.go`；`verification`；`settings`；`telegram/updates`、`telegram/tgfmt` | **拆分**：22–232 的版本形状与上下文判定进入 `rules`；237–288 的模式/题库选择进入 `verification` 与 `settings`；290–539 的题面和 fallback 文案进入 `tgfmt`；541–853 的 Update 路由、nonce 领取和结算改为 `telegram` 事件调用 `verification`。保留真实 `uname` 输出、命令回显剥离、Windows/macOS 转 fallback、全角分钟证明、跨群 fallback 不误扣次数、nonce 绑定和三次尝试。 |
| `internal/store/baseline.go` | 178 | 从 `config.json` 的字段存在性和旧 `Config` 建立全局默认、每群 baseline 与来源。 | `settings` | **重写**：目标是 `defaults.yaml`、环境覆盖、每群数据库 override 和 provisioning，不再有 JSON presence 及旧 global/control-group 模型。保留默认值夹取、显式配置来源、空 override 继承出厂默认和每群隔离。 |
| `internal/store/settings.go` | 1592 | `settings.json` 的版本迁移、不可变快照、稀疏 override、乐观 revision、运行时注册、owner claim/enrollment 和校验。 | `settings`；`database` | **重写**：334–951 的文件加载/版本链/注册状态，952–1355 的快照和原子文件提交，都与 `configupgrade`、`chat.settings` 和数据库事务冲突。保留每群稀疏覆盖、来源、revision 冲突、整份校验、失败不发布半个新快照；删除单全局 owner/enrollment，改为每群管理员授权。 |
| `internal/store/json.go` | 134 | 通用 JSON 原子读写、损坏备份、临时文件回收和目录 `fsync`。 | `database` | **删除**：目标的可变状态全部在数据库；配置文件升级由 `configupgrade` 负责。原子性、损坏不覆盖、并发不回退由数据库事务、迁移和导入测试承接。 |
| `internal/tg/tg.go` | 555 | 一个 telego 包装器，混合消息发送、HTML fallback、删除重试/定时清理、告警、管理员缓存、权限操作和 linked chat 缓存。 | `telegram/connector.go`；`telegram/tgfmt`；`telegram/queue` | **拆分**：52–153 进入 `tgfmt`/connector；155–229、507–534 改为 durable outbox 的 `queue`；231–320 是告警投递；322–392 是成员/管理员查询缓存；394–505 是 Gateway 的成员处罚实现。保留私聊不自动删除、删除已不存在消息视为成功、写入前 fresh admin、权限恢复使用群默认权限；所有发送经队列和 429 退避。 |
| `internal/tg/errors.go` | 280 | Telegram API 错误分类、`retry_after`、文本 UTF-16 限长与截断。 | `telegram`；`telegram/tgfmt` | **拆分**：错误分类和 `RetryAfter` 进入 connector/queue；`TextUnits`、`CapText` 进入 `tgfmt`。保留“申请已消失”“群不可达”“消息已删除”“目的地失败不算单条永久失败”的判定。 |
| `internal/tg/redact.go` | 37 | 通过日志 writer 清除 Bot API URL 中的 token。 | `status` | **移入**：整体成为结构化日志出口的 redacting writer。保留调用点遗漏 `%v` 时仍不泄露 token 的边界。 |
| `internal/panel/session.go` | 341 | 旧 Telegram 设置面板的内存 session、token 轮换、ForceReply/chat picker 输入和与 kernel DM 的抢占规则。 | `console/auth`；`console/api` | **重写**：目标使用 Mini App/OIDC 会话，不存在 Telegram callback token、ForceReply 或 chat picker。保留身份与群绑定、过期、单用户会话、并发重放只成功一次、权限失效即拒绝；改为目标的 8 小时会话、PKCE/nonce 和敏感写入时现查管理员。 |
| `internal/panel/settings_input.go` | 852 | 旧面板的 callback 分发、草稿、确认、ForceReply 输入、chat picker 校验和对 `store` 的直接提交。 | `console/api`；`settings`；`rules` | **重写**：22–310、400–796 是 Telegram 专用交互；797–851 的输入范围和 URL/题目复制可作为校验来源。保留题库/fallback/频道/白名单的整份校验、确认删除、目标群隔离和 revision 冲突；改为 OpenAPI DTO、HTTP 参数校验和同一份 `rules` 试答。 |
| `internal/panel/settings_panel.go` | 1399 | 旧 Telegram 内联键盘的所有页面、分页、屏幕渲染、callback 分发和直接 settings mutation。 | `console/api`；`web/`；少量 `panel` | **重写**：33–460 是回调流程，473–1210 是 Telegram 屏幕构造，1211–1398 是旧显示和 callback 辅助。目标控制台保留群选择、设置来源、题库、频道、反垃圾和并发冲突能力，但改为 HTTP DTO 与 React；不搬行内键盘、64 字节 callback 协议或 `telego.InlineKeyboardMarkup`。 |
| `internal/panel/panel.go` | 445 | `/ping`、`/stats`、`/help` 和旧管理命令；混合群判断、权限检查、设置改写、状态文案。 | `telegram/updates`；`status`；`panel` | **拆分**：87–220 的状态/信息命令进入 `telegram` 调 `status`；223–325 的旧管理命令保留为窄 Telegram 命令面或重定向到控制台；332–445 的 fresh/cached admin 规则由 `telegram` 和 `console/auth` 共用。保留信息命令不要求管理员、写入命令 fail closed、群范围限制；不允许直接写 SQL 或绕过 `verification.Service`。 |
| `internal/panel/codec.go` | 200 | `p1:` Telegram callback 数据的压缩、签名整数编码、屏幕转移白名单和 64 字节约束。 | 无 | **删除**：浏览器请求由 OpenAPI、会话和 URL/JSON 参数承载，不存在 Telegram callback payload。旧防伪造目标由 `console/auth` 的会话、CSRF/state、DTO 校验和 revision 控制实现，不保留编码协议。 |
| `internal/lookup/repology.go` | 104 | Repology 名称校验、按发行版家族归并与 `/repology` Telegram handler。 | `lookup`；`telegram/updates`、`telegram/tgfmt` | **拆分**：输入校验和家族归并保留在 `lookup`；18–41 的 Update/发送分支改为 `telegram` 调用 lookup 结果并由 `tgfmt` 渲染。保留未知家族计数而非伪装为未找到。 |
| `internal/lookup/content.go` | 698 | Bugzilla、新闻、Wiki、论坛的抓取、缓存、解析、文案和 Telegram handlers。 | `lookup`；`telegram/updates`、`telegram/tgfmt` | **拆分**：纯 fetch/缓存/解析保留在 `lookup`；所有 `On*` 和 HTML 文案分离到更新适配器/格式化器。保留 404 与暂时失败的区别、新闻缓存、Wiki 语言去重和受限输入。 |
| `internal/lookup/cve.go` | 152 | CVE 标识校验、NVD JSON 解析、描述截断和 `/cve` handler。 | `lookup`；`telegram/updates`、`telegram/tgfmt` | **拆分**：标识校验、解析、rune 边界截断留在 `lookup`；handler/展示移到 Telegram 边界。保留未知 CVE 不等于上游故障。 |
| `internal/lookup/distros.go` | 942 | Repology 家族与版本排序、跨发行版/arm64 查询、Debian/Ubuntu release metadata 缓存和 Telegram handlers。 | `lookup`；`telegram/updates`、`telegram/tgfmt` | **拆分**：19–339 的排序/查询、454–632 的 arm64 数据、703–942 的 release metadata 按职责拆为 lookup 文件；340–453、633–702 的 Update 入口移至 `telegram`。保留开发版不冒充 stable、失败不当作缺失、短失败缓存和 EOL 处理。 |
| `internal/lookup/http.go` | 355 | 查找服务设置读取、私聊限流和共享 HTTP 连接数/状态码/体积上限。 | `lookup`；`telegram/updates` | **拆分**：18–231 的 service/私聊策略留在 `lookup` 但改读 `settings`；250–355 的有界 HTTP transport 继续由 `lookup` 提供给 lookup 与 feed 的显式接口。保留 24 并发上限、2 秒排队上限、非 200 分类和超限体不交给解析器。 |
| `internal/lookup/kernel.go` | 113 | kernel.org release JSON、短 TTL 缓存和 `/kernel` handler。 | `lookup`；`telegram/updates`、`telegram/tgfmt` | **拆分**：解析/有界缓存进入 `lookup`；`OnKernel` 与发送进入 Telegram 边界。保留空或畸形列表不可被报告为空结果。 |
| `internal/lookup/manpage.go` | 144 | man page 名称/section 校验、顺序探测、NAME/SYNOPSIS 解析和 `/man` handler。 | `lookup`；`telegram/updates`、`telegram/tgfmt` | **拆分**：校验、探测和解析留在 `lookup`；Update 和消息渲染移出。保留 404 是未找到、网络失败不是未找到。 |
| `internal/lookup/packages.go` | 1594 | Gentoo/overlay 包搜索、版本比较、缓存、USE/arm64 解析、HTML 渲染和三个 Telegram handler。 | `lookup`；`telegram/updates`；`telegram/tgfmt`；`settings` | **拆分**：118–675 的版本、缓存、数据源查询进 `lookup`；676–727、1421–1494、1568–1594 的 Update 入口进 `telegram/updates`；728–837、1033–1294、1385–1420 的 HTML 进 `tgfmt`；34–57 的部署配置改由 `settings` 注入。保留自然版本排序、9999 规则、部分源不可用不报未找到、URL 归一化、USE_EXPAND 限制。 |
| `internal/moderate/antispam.go` | 237 | 频道身份反垃圾、白名单、ID 解析和 Telegram handler。 | `moderate`；`telegram/ids`；`telegram/updates`；`settings` | **拆分**：21–64 的策略和白名单更新进 `moderate/settings`；`parseChannelID` 进 `telegram/ids`；65–237 的 Update/封禁调用变为 `telegram` 适配器调 moderation use case。保留只更新当前群、4,096 上限、linked channel 例外和解除白名单时的 unban。 |
| `internal/moderate/service.go` | 601 | 权限预检报告、警告/踢人/封禁/禁言命令、设置写入、文案与 Telegram 调用。 | `moderate`；`telegram`；`status`；`settings` | **拆分**：74–178 的启动权限预检进入 `status` 使用 Telegram 端口；183–259、513–587 的规则/设置进入 `moderate` 和 `settings`；272–511 的 Update 解码进入 `telegram/updates`。保留 fresh admin、目标管理员保护、先处罚成功再删证据、失败告警、警告达到阈值后的可重新加入语义。 |
| `internal/moderate/state.go` | 126 | `warns.json` 的群/用户 warning 计数、上限驱逐和文件保存。 | `moderate`；`database` | **重写**：目标没有 JSON warning 文件；用 audit 记录与数据库事务承载当前计数/清除，或在已有 `audit` 上派生。保留群与用户双键、确定性有界驱逐、写失败不伪称成功、重启后持续的处罚语义。 |
| `internal/bot/edition.go` | 9 | 用 build edition 给 Gentoo 命令加前缀。 | 无 | **删除**：目标是单一多群产品，功能按群配置，不用二进制 edition 决定命令名。 |
| `internal/bot/bot.go` | 126 | telegohandler middleware、首个命中路由顺序、命令菜单和启动诊断装配。 | `telegram/updates`；`app`；`status` | **拆分**：路由表和 first-match 边界进 `telegram/updates`；依赖装配进 `app`；启动诊断进 `status`。保留一个 Update 只落到预期 handler、验证/面板 DM 谓词优先级和注册后刷新命令菜单的可观察结果。 |
| `internal/bot/commands.go` | 140 | Telegram 命令菜单、语言 scope、管理员/owner 菜单安装。 | `telegram`；`telegram/tgfmt` | **拆分**：菜单声明和 Bot API 安装进 `telegram`，描述继续由 `i18n` 提供。保留语言 scope 与运行时群更新；删除全局 owner 和 edition 特有菜单。 |
| `internal/bot/dm.go` | 122 | 普通私聊自动回复、按用户冷却和命令例外。 | `telegram/updates`；`settings` | **拆分**：DM 谓词/冷却进入 Telegram 更新层，回复内容进入 `tgfmt/i18n`，频率配置来自 `settings`。保留成员命令不被自动回复吞掉、冷却 map 有上限；移除 edition 命令前缀。 |
| `internal/config/config.go` | 733 | 旧 JSON 配置 schema、默认、版本/时长/题目/Feed 校验和每群解析。 | `settings`；少量 `rules` | **重写**：目标是 YAML、嵌入 defaults、环境映射、secret 引用和 `configupgrade`；旧 `Config` 同时承担进程、群、资源三层，不能搬。保留时长溢出保护、Telegram 处罚边界、模式/语言/题目校验、feed 间隔和反垃圾默认值；规则定义校验交给 `rules`。 |
| `internal/edition/edition_gentoo.go` | 19 | `gentoo` build tag 的二进制名、命令前缀、社区身份和 kernel 示例后缀。 | `edition` | **重写**：同一服务面对多群，不能由 build tag 宣称某社区身份或改变命令。保留只读构建元数据接口；改为单一产品名和 `ldflags` 版本信息。 |
| `internal/edition/edition_generic.go` | 20 | 非 Gentoo build 的另一套名称、命令前缀和身份。 | `edition` | **重写**：与上一文件一起合并为无 build tag 的单文件 edition。保留版本/发行元数据，不保留双产品行为。 |
| `internal/feed/feed.go` | 976 | Bugzilla/news 抓取、JSON cursor、状态编辑、Telegram 发送重试/节流、权限探测和循环生命周期。 | `feed`；`database`；`telegram/queue`、`telegram/tgfmt`；`status`；`app` | **拆分**：27–341 的 feed 状态和抓取进 `feed`/repository；343–380 的 JSON 迁入 `database`；381–400、525–625、674–755 的发送/编辑进 queue Gateway；401–674 的展示进 `tgfmt`；771–863 的调度进 `feed` 任务；864–925 的诊断进 `status`；927–976 由 `app` 注册。保留 cursor 不越过未投递项、状态变化编辑、确认 ping、永久错误丢弃与瞬态错误保留、首次运行 baseline、每源失败暂停。 |
| `internal/i18n/bot.go` | 159 | Bot 菜单、生命周期、私聊和注册的 typed catalogue 类型；末尾按 edition 选择身份文案。 | `i18n` | **拆分**：绝大多数类型移入；`Who` 的 edition 分支改为单一产品文案。保留 typed key 边界，不把文案写回 handler。 |
| `internal/i18n/catalog.go` | 288 | locale 枚举、Telegram/存储语言解析、嵌入 JSON 加载、格式化占位符和 edition token 替换。 | `i18n` | **拆分**：语言解析、加载和 typed catalog 移入；71–118 的 `{g}`/`{ks}` edition 替换删除，改为显式参数或通用文案。 |
| `internal/i18n/verification.go` | 307 | 验证流程的 typed 文案与内置 fallback 题；末尾按 edition 选题。 | `i18n`；`settings` | **拆分**：catalog 类型移入 `i18n`；`BuiltinFallback` 改为出厂 rules/provisioning，不按 build edition 选题。 |
| `internal/i18n/lookup_content.go` | 182 | Bug、新闻、Wiki、论坛 lookup 的 typed 文案。 | `i18n` | **移入**：整体保留；调用方改为 `tgfmt`，不在 lookup use case 中直接 Render。 |
| `internal/i18n/lookup_distros.go` | 152 | 跨发行版、man、CVE、Repology、kernel 的 typed 文案。 | `i18n` | **移入**：整体保留；展示移到 `tgfmt`。 |
| `internal/i18n/lookup_packages.go` | 110 | 包、USE、arm 查询的 typed 文案。 | `i18n` | **移入**：整体保留；删去 edition 命令名占位的使用点。 |
| `internal/i18n/moderate.go` | 174 | moderation、处罚时长、反垃圾和权限预检的 typed 文案。 | `i18n` | **移入**：整体保留；`moderate` 返回结构结果，Telegram/console 各自渲染。 |
| `internal/i18n/panel.go` | 268 | 旧 Telegram 面板和命令的 typed 文案。 | `i18n`；`web/` locale | **拆分**：状态、帮助、Telegram 命令文案留在 Go `i18n`；旧 settings screen、按钮、ForceReply 提示迁至前端 locale 资源。不能继续把控制台屏幕文案编进 Go。 |
| `internal/i18n/feed.go` | 42 | Feed Bug 字段和配置拒绝文案。 | `i18n` | **移入**：整体保留；由 `telegram/tgfmt` 渲染。 |
| `internal/i18n/doc.go` | 7 | i18n 包文档和 locale 约束。 | `i18n` | **移入**：整体保留并更新为目标 locale 资源约束。 |
| `cmd/vestibule/sd_notify.go` | 151 | systemd notify/watchdog 和长轮询进度 caller。 | `app`；`status`；`telegram` | **拆分**：systemd notifier/lifecycle 进入 `app` 与 `status`；115–150 的 polling progress caller 进入 Telegram 更新 transport。保留无 `NOTIFY_SOCKET` 时 no-op、就绪晚于启动完成、卡住 Bot API 调用不阻塞 handler 关闭。 |
| `cmd/vestibule/main.go` | 552 | CLI、全部依赖装配、长轮询、并发转发、JSON 状态加载、关机顺序、掉线告警。 | `cmd/bot`；`app`；`status`；`telegram` | **重写**：目标 `cmd/bot` 只能解析命令行并调用 `app.Run`；现文件直接管理 telego、旧 Config/Settings 和 file state。保留 handler 在开始拉取前已注册、更新并发上限、意外流结束使进程退出、先摘流量再停止、等待处理完成和 watchdog 进度；数据库迁移/任务注册改由 `app`。 |
| `cmd/vestibule/registration.go` | 1045 | 全局 owner claim、enrollment nonce、未知群延迟离开、运行时注册和群 membership Update。 | `telegram/updates`；`database`；`settings` | **重写**：目标 `chat` 表在 bot 被拉入/移出时记录状态，每群管理员自己授权，不存在单一 owner、enrollment link 或未知群十分钟离开。保留 bot 加入/移出事件的幂等处理、同群转移串行化、标题更新、移出标记和注册后立即生效；旧 owner 流程测试改写为多租户 onboarding。 |

## 二、测试资产

事故来源只使用仓库可核验材料：

- `是`：测试注释、`CHANGELOG.md` 或架构书可对应已发生缺陷。
- `未证实`：测试有行为价值，但仓库没有事故出处。
- `否`：主要是 fixture、测试支撑或已废弃协议的实现契约。

所有文件保留为测试资产；`重写` 只替换断言边界，不删除覆盖的行为。

| 路径 | 行数 | 它锁住的行为 | 是否生产事故换来的 | 改造后怎么办 |
|---|---:|---|---|---|
| `internal/verify/settings_test.go` | 337 | 每群设置隔离、默认/覆盖来源、运行时群 pending 跨重启。 | 未证实 | **混合**。改为 `settings`/`database` 集成测试；保留隔离和来源，不再断言 `settings.json`。 |
| `internal/verify/settle_bounds_test.go` | 73 | 临时权限故障有上限重试；群已不可达立即停止；提示只发一次。 | 是 | **行为**。改为 action worker、lease 和最大尝试次数测试。 |
| `internal/verify/state_compat_test.go` | 446 | `pending.json`、`verifyfail.json`、heartbeat、agent tally 的历史字段兼容。 | 否（兼容 fixture） | **实现锁定**。保留 fixture，重写为旧 JSON 一次性导入到数据库后的行数、状态、deadline、epoch 和 strike 结果断言。 |
| `internal/verify/state_write_failure_test.go` | 184 | 写状态失败时运行中状态仍继续，但重启恢复会丢失。 | 未证实 | **混合**。重写为事务失败不发布状态转换、outbox/挑战一致性测试；不保留文件不可写分支。 |
| `internal/verify/transport_wrapper_test.go` | 50 | outage wrapper 不触发类型断言 panic。 | 是（`CHANGELOG.md` 4.5.6） | **行为**。用 `verification.Gateway` fake 验证恢复路径不 panic；不再测试 `Unwrap` 类型断言。 |
| `internal/verify/verify_test.go` | 2354 | 加群投递、DM fallback、callback、结算、删除、文案、语言、失败重开等主流程。 | 是（含已记录投递/误罚修复） | **混合**。按 `join`、`answer`、`settle`、`postjoin`、`telegram/tgfmt` 拆分；把 telego 参数、内存 map 和 timer 断言改成领域结果、条件转换和 Gateway 调用。 |
| `internal/verify/verifyfail_test.go` | 200 | strike、冷却、自动封禁阈值、衰减、容量和 nonce 领取。 | 未证实 | **混合**。保留处罚规则；JSON 保存、map 驱逐改为数据库记录和条件更新测试。 |
| `internal/verify/writejson_test.go` | 33 | 各状态文件读失败后禁止覆盖。 | 否（旧文件介质） | **实现锁定**。改为迁移/数据库不可用时拒绝启动或拒绝事务，不保留文件路径断言。 |
| `internal/verify/fallback_cross_group_test.go` | 69 | 同一用户跨群 fallback 不误扣次数；同题库复用题；不同题库不串题。 | 未证实 | **行为**。移到 `rules` 与 `verification`，以 challenge payload/群 ID 测试。 |
| `internal/verify/kernel_test.go` | 1135 | 内核题判定、fallback、AI trap、送达确认、频道 gate、重启恢复。 | 是（记录了 fallback、未确认投递和频道误罚） | **混合**。纯解析移到 `rules`；流程改测 `verification.Service` 和 Store/Gateway fake，不测 `telego.Update` 细节。 |
| `internal/verify/member_settlement_test.go` | 260 | 申请人进入群后转为 held；队列满安全移出；不解除他人限制；旧管理员按钮无效。 | 未证实 | **行为**。保留两种群模式和限制归属，改测数据库状态、Gateway action 与 audit。 |
| `internal/verify/post_join_flood_test.go` | 139 | 重复进群不重发题、不延长窗口；冷却期间拒绝而不 DM 洪泛。 | 未证实 | **行为**。用部分唯一索引和到期记录测试替换内存 map/timer 断言。 |
| `internal/verify/post_join_journey_test.go` | 175 | 进群后被限制者通过会释放，超时会移出；已通过申请者不再被题目覆盖。 | 未证实 | **行为**。保留为 postjoin 端到端 use-case 测试。 |
| `internal/verify/post_join_test.go` | 406 | `chat_member` 识别、通过后不再抓一次、held/unheld 分支、可信群、加入与结算竞态。 | 是（用户明确举例；架构书第 5 节） | **行为**。重点重写为并发 `pending→approved` 条件更新后到达 membership event 的回归测试；不得用 `recentlyPassed` 内存窗口替代。 |
| `internal/verify/quiz_test.go` | 28 | 选项洗牌后正确答案索引仍对应原正确项。 | 未证实 | **行为**。移入 `rules`/挑战生成测试。 |
| `internal/verify/recovery_held_test.go` | 59 | 掉线恢复不丢 held 成员；已入群申请者才清理。 | 是 | **行为**。改为 challenge 的持久状态、成员核验和恢复 action 测试。 |
| `internal/verify/recovery_request_test.go` | 98 | 恢复时已入群者不重发题；成员查询失败不跳过；达到延期上限不重投递。 | 是 | **行为**。改测扫描器/恢复任务和数据库 epoch。 |
| `internal/verify/recovery_test.go` | 1015 | 心跳、离线延期、48 小时上限、旧 epoch、恢复窗口、重投递和失败保留旧题。 | 是 | **混合**。保留所有失败方向和时间语义；`time.AfterFunc`、heartbeat JSON、日志次数改为 Clock/Store/任务测试。 |
| `internal/verify/recovery_window_test.go` | 64 | 恢复题说明真实窗口并选可读时长单位。 | 是（`CHANGELOG.md` 4.5.6） | **混合**。窗口业务规则留 `verification`；文字单位移到 `i18n/tgfmt`。 |
| `internal/verify/failure_time_test.go` | 60 | 重试结算不把原始失败时间改晚，避免错误延长冷却。 | 未证实 | **行为**。保留为数据库 failure timestamp 不变测试。 |
| `internal/verify/agents_test.go` | 113 | model 声明清洗、计数上限、tripwire 记录和持久化。 | 未证实 | **混合**。保留结构信号/输入清洗；统计改为 `status` 指标或 audit，不保留 `agents.json`。 |
| `internal/verify/channel_gate_test.go` | 81 | 必关频道查询不可读时拒绝但不 strike；可读且未加入才 strike。 | 是 | **行为**。移为 `verification` 的 fail-closed、no-fault 分类测试。 |
| `internal/verify/channel_outage_test.go` | 59 | 题发出后频道故障不能算申请人超时；正常频道仍计入。 | 是 | **行为**。保留，改用 Gateway 查询错误和条件结算测试。 |
| `internal/verify/decline_gone_test.go` | 155 | join request 已消失时不无限重试；超时不误 strike；真实权限故障保留重试。 | 是（`CHANGELOG.md` 4.1.1） | **行为**。保留为 typed Telegram error 到 action 状态映射测试。 |
| `internal/verify/delivery_owner_test.go` | 37 | 旧投递完成不能把消息 ID 写进新 pending。 | 未证实 | **行为**。改测 challenge ID/epoch 条件 attach；这是 Store 条件更新的必要回归。 |
| `internal/verify/duplicate_arrival_test.go` | 88 | 重投同一 join request 保留可见挑战；真正重新申请替换挑战。 | 未证实 | **行为**。改测 `challenge_open` 唯一索引、重复插入和 supersede 语义。 |
| `internal/verify/settlement_giveup_test.go` | 27 | 无法修复的 Telegram 失败立即转人工，不循环重试。 | 是 | **行为**。移到 action worker retry 分类测试。 |
| `internal/verify/kernel_shapes_test.go` | 101 | 真实命令输出、terminal 包装和不同 compiler banner 的内核判定一致。 | 未证实 | **行为**。整体移到 `rules` 单元测试。 |
| `internal/store/settings_test.go` | 907 | 稀疏 override、版本迁移、冲突、注册、写失败和旧 antispam 导入。 | 未证实 | **混合**。保留来源、校验、冲突、迁移输入；改为 `settings` YAML 升级和 DB 设置事务测试，删除旧版本 JSON round-trip。 |
| `internal/store/state_root_compat_test.go` | 184 | 根级 `settings.json`/antispam 兼容 fixture。 | 否（兼容 fixture） | **实现锁定**。作为一次性迁移输入保留，断言目标数据库/settings 结果。 |
| `internal/store/json_test.go` | 317 | 原子写、损坏备份、并发 writer、临时文件回收、父目录同步。 | 否（旧文件介质） | **实现锁定**。保留“无半写、并发不回退、损坏不覆盖”的性质，重写为数据库事务/迁移故障测试；丢弃 temp 文件名断言。 |
| `internal/store/reconcile_test.go` | 84 | config 与运行时设置漂移时不悄悄重启验证或注销群。 | 未证实 | **行为**。改为默认值、群 override、provisioning 三层优先级测试。 |
| `internal/tg/errors_join_test.go` | 138 | Telegram error 分类、告警去重、私聊不清理、audit 不去重、目的地故障不算永久项失败。 | 是 | **混合**。保留分类结果；改测 connector 的 typed error 与 queue 行状态，不断言 telego error 字符串散布在业务层。 |
| `internal/tg/redact_test.go` | 66 | Bot token 不出现在 API 错误和日志，writer 返回调用者原长度。 | 是（`CHANGELOG.md` 4.5.6） | **行为**。整体迁至 `status`；保留日志边界测试。 |
| `internal/tg/tg_test.go` | 547 | HTML/rich fallback、清理、告警、管理员 cache、处罚和文本限长。 | 是（多项 Telegram 修复） | **混合**。保留可观察发送/缓存/处罚语义；telego request 字段测试移到 connector 合约测试，队列行为独立测试。 |
| `internal/tg/delete_retry_test.go` | 69 | delete 的瞬态重试、已不存在不重试、次数上限和零 ID no-op。 | 是（`CHANGELOG.md` 4.5.5） | **行为**。改为 queue/outbox 的删除 action 重试测试，不使用 timer sleep。 |
| `internal/panel/panel_test.go` | 55 | 群语言、`/autodel` 参数、帮助文案来自 catalogue。 | 未证实 | **混合**。语言/参数和 catalogue 一致性保留；旧命令测试移到 Telegram 命令面或 console DTO。 |
| `internal/panel/settings_controls_test.go` | 1052 | callback 控件只改目标群、重渲染正确、旧 revision 被拒绝。 | 否（旧 callback UI 实现） | **实现锁定**。重写为 `PATCH /settings` 的目标群、版本冲突和字段级契约测试。 |
| `internal/panel/settings_integration_test.go` | 394 | 旧管理命令持久化到目标群、运行时群可用、fresh admin、写失败反馈。 | 未证实 | **混合**。保留群隔离、fresh admin、错误反馈；从 Telegram 命令/JSON settings 改为 console API 和数据库。 |
| `internal/panel/settings_panel_behavior_test.go` | 1244 | session 防重放、callback codec、草稿、题库、频道、权限降级和 stale revision。 | 否（主要为旧 Telegram UI 协议） | **实现锁定**。把安全语义重写为 console 会话/CSRF/OIDC、HTTP revision、规则导入和 API 测试；不保留 callback 长度、ForceReply 与 chat picker 优先级。 |
| `internal/panel/admin_support_test.go` | 121 | Panel 测试用 fake Bot API、handler 启动辅助。 | 否（测试支撑） | 不单独迁移；按新 API/connector test helper 重建，承载它的面板行为测试不能丢。 |
| `internal/lookup/pkgs_test.go` | 461 | 发行版家族、snapshot/9999/snap 排序、Repology 可用性、overlay 截断和 cache 失败保留旧数据。 | 是 | **行为**。移到拆分后的 lookup 纯函数/HTTP fake 测试。 |
| `internal/lookup/releaseinfo_test.go` | 166 | Debian/Ubuntu release metadata 成功/失败 TTL、stable/testing/EOL 标签。 | 是 | **行为**。整体留在 `lookup`。 |
| `internal/lookup/use_matches_test.go` | 45 | 多命中排序、建议当前可用命令、主页标签本地化。 | 未证实 | **混合**。保留排序/本地化；edition 命令前缀改为单一命令或群能力配置断言。 |
| `internal/lookup/use_test.go` | 188 | USE_EXPAND 展示、来源可用性和未找到/暂不可用区分。 | 未证实 | **行为**。解析保留在 lookup；HTML 断言改为 `tgfmt` 输出/结构化 view model。 |
| `internal/lookup/version_test.go` | 255 | Gentoo 版本自然排序、命令参数、主树可用性和 package cache。 | 是 | **行为**。整体保留在 `lookup`。 |
| `internal/lookup/wiki_test.go` | 100 | transient 搜索不报未找到、语言偏好去重、来源提示。 | 是 | **行为**。整体保留。 |
| `internal/lookup/armpkgs_test.go` | 150 | Debian Madison、AUR、Fedora、Gentoo arm64 状态的“未知”与“缺失”区别。 | 是 | **行为**。整体保留。 |
| `internal/lookup/bug_test.go` | 130 | Bugzilla 查找状态、故障文案、枚举本地化。 | 未证实 | **混合**。解析与状态保留；Telegram 文案转 `tgfmt/i18n`。 |
| `internal/lookup/handlers_test.go` | 461 | 全部 lookup handler 的参数、目录文案、限流和清理。 | 未证实 | **实现锁定**。保留命令到用例的覆盖；telegohandler/清理调用改为 `telegram/updates` + queue 合约。 |
| `internal/lookup/http_test.go` | 166 | 请求者语言链、运行时群、HTTP 状态/体积/并发限制、私聊频率限制。 | 未证实 | **混合**。整体按 lookup transport 和 settings 测试重组。 |
| `internal/lookup/news_test.go` | 134 | 新闻缓存可用性和渲染时的暂时故障/结果区别。 | 未证实 | **混合**。缓存行为留 lookup；展示移 tgfmt。 |
| `internal/lookup/parser_fixtures_test.go` | 487 | 真实上游固定样本：新闻、IUSE、metadata、包搜索、overlay、Repology、Bugzilla、release metadata。 | 是 | **行为**。fixture 原样保留，按解析器文件拆分；禁止改成网络测试。 |
| `internal/lookup/pkg_test.go` | 43 | `/pkg` plain/rich 的 keyword legend。 | 未证实 | **实现锁定**。改为 `tgfmt` 视图模型/文本合约。 |
| `internal/lookup/arm_test.go` | 95 | arm64 keyword 选择和查询失败不伪称无支持。 | 是 | **行为**。整体保留。 |
| `internal/lookup/repology_test.go` | 55 | Repology 家族归并、未知家族计数、项目名输入边界。 | 未证实 | **行为**。整体保留。 |
| `internal/lookup/cve_test.go` | 74 | CVE JSON、未知记录、ID 形状和 rune 截断。 | 未证实 | **行为**。整体保留。 |
| `internal/lookup/kernel_test.go` | 118 | kernel.org 格式拒绝、缓存命中和上游失败区别。 | 未证实 | **行为**。整体保留。 |
| `internal/lookup/manpage_test.go` | 74 | manpage 解析、名称形状和 404/故障区别。 | 未证实 | **行为**。整体保留。 |
| `internal/moderate/antispam_test.go` | 178 | 频道 ID 规范化、控制群限制、白名单边界、发送者过滤。 | 是 | **混合**。保留策略；telego transport 调用改为 Gateway fake。 |
| `internal/moderate/bantime_test.go` | 60 | 处罚时长语法、永久/有限文本、mute 边界。 | 未证实 | **混合**。保留语法与边界；文本由 `i18n/tgfmt` 或 console 格式化。 |
| `internal/moderate/moderate_test.go` | 971 | 权限预检、fail-closed 管理员检查、warn/ban/mute 处置顺序和失败告警。 | 是 | **混合**。保留 use-case 行为；Update/telego 调用改为 moderation 端口测试。 |
| `internal/moderate/state_test.go` | 254 | warning 跨重启、旧 JSON fixture、读写失败、上限驱逐。 | 是 | **混合**。保留 warning 语义；fixture 改为 JSON 导入到 database/audit 的结果测试。 |
| `internal/bot/help_coverage_test.go` | 190 | 帮助、菜单、DM 命令和 catalogue 的一致性；edition 身份文案。 | 否（含已废弃 edition 协议） | **混合**。保留命令注册表与帮助一致性；删除 Gentoo/generic 身份断言，改测不出现社区硬编码。 |
| `internal/bot/settings_integration_test.go` | 158 | lookup 自动删除、菜单 alias、私聊默认回复和实时频率设置。 | 未证实 | **混合**。按 `settings`、`telegram/updates`、lookup 分拆。 |
| `internal/bot/bot_test.go` | 1022 | handler 顺序、菜单 scope、DM 容量、全局分发和公共验证入口边界。 | 未证实 | **实现锁定**。保留路由优先级/唯一处理行为，改测领域事件映射；不锁 telegohandler 内部 route 数组。 |
| `internal/bot/edition_dispatch_test.go` | 54 | 旧 build edition 的命令前缀能到达 handler。 | 否（已废弃 edition 协议） | 重写为单一命令注册或每群 capability 开关的 dispatch 测试。 |
| `internal/bot/dm_test.go` | 39 | 私聊命令不被自动回复吞掉。 | 未证实 | **行为**。整体迁至 `telegram/updates`。 |
| `internal/config/config_test.go` | 634 | 旧 JSON 配置的默认、legacy schema、校验、时长边界、已知 chat 和可信群。 | 否（旧配置 schema） | **实现锁定**。保留值域与默认意图，重写为 `defaults.yaml`、环境覆盖和 `configupgrade` 的 v1/v2/v3 输入测试。 |
| `internal/config/antispam_default_test.go` | 25 | `block_channel_senders` 缺省为开，避免未配置群失去保护。 | 是 | **行为**。保留为 `settings` 出厂默认测试。 |
| `internal/edition/deploy_test.go` | 48 | 两个旧 edition 的 unit、安装器和二进制名一致。 | 否（已废弃双产品部署） | 重写为单一 `cmd/bot`、服务单元、配置目录和迁移启动路径的一致性测试。 |
| `internal/feed/feed_fix_test.go` | 654 | backlog cursor、投递失败分类、deadline、tracked 编辑、长文本和永久错误推进。 | 是 | **混合**。保留投递/编辑状态机；fake telego 改为 queue Gateway，cursor 存储改 database。 |
| `internal/feed/feed_test.go` | 920 | Bug/news cursor、状态编辑、确认 ping、追踪上限、腐败状态、429 和重开。 | 是 | **混合**。保留所有业务分支；JSON/直接 Bot API 断言改 repository/outbox 合约。 |
| `internal/feed/lifecycle_failure_test.go` | 420 | 首次 baseline、过滤、权限探测、取消 flush、保存失败后重启重投递。 | 是 | **混合**。保留 lifecycle 语义；由 `app` 任务和 database/outbox 测试。 |
| `internal/feed/state_compat_test.go` | 220 | 旧 feed JSON 的状态键、未知字段和迁移。 | 否（兼容 fixture） | **实现锁定**。保留 JSON 作为导入 fixture，断言迁移后的 DB feed/source/tracking 状态。 |
| `internal/feed/upstream_fixtures_test.go` | 88 | Bugzilla 字段、分页、空结果和 tracked 查询 URL 合约。 | 是 | **行为**。保留固定上游 fixture，适配新 feed transport。 |
| `internal/i18n/kernel_example_test.go` | 31 | kernel 示例与 build edition 的后缀一致。 | 否（已废弃 edition 行为） | 重写为示例占位符不会被接受且不含社区名。 |
| `internal/i18n/no_scratch_files_test.go` | 43 | 仓库不携带调试/临时文件。 | 否（仓库卫生） | **行为**。保留为仓库门禁，扩展到新 `web/` 目录。 |
| `internal/i18n/edition_identity_test.go` | 64 | 只有 Gentoo edition 文案可以命名社区。 | 否（已废弃 edition 行为） | 重写为所有面向用户文本不含特定社区/群硬编码。 |
| `internal/i18n/edition_prefix_test.go` | 36 | catalogue 的 Gentoo 命令必须带 edition token。 | 否（已废弃 edition 协议） | 重写为所有命令占位符都由目标命令注册表满足，或移除该 token。 |
| `internal/i18n/escape_guard_test.go` | 43 | locale JSON 没有双重转义字面量。 | 否（数据质量） | **行为**。整体保留，涵盖 Go 和前端 locale。 |
| `internal/i18n/invariants_test.go` | 140 | 用户中文只在 locale、locale 文件可加载、typed catalog 形状一致。 | 否（架构门禁） | **行为**。保留并扩展为 Go/前端的文案边界门禁。 |
| `internal/i18n/catalog_test.go` | 301 | locale 解析、catalog 完整性、恢复文案、格式占位符跨语言一致。 | 否（数据质量） | **行为**。整体保留，删除 edition token 断言。 |
| `internal/i18n/consistency_test.go` | 256 | 术语、简繁脚本、Gentoo 固有词保持原文。 | 否（数据质量） | **混合**。保留术语/脚本检查；移除产品不再支持的 Gentoo identity 规则。 |
| `cmd/vestibule/sd_notify_test.go` | 201 | systemd no-op、ready/watchdog 时序、poll 进度和卡住调用下的关闭。 | 未证实 | **行为**。移到 `app/status/telegram` 的生命周期测试。 |
| `cmd/vestibule/main_test.go` | 605 | 意外 poll 结束重启、先注册再拉取、缓冲更新 drain、并发上限、超过 retention 的 outage 提示、关闭顺序。 | 是 | **混合**。保留生命周期结果；重写为 `app.Run` 和 task supervisor 测试，不锁 main 函数。 |
| `cmd/vestibule/registration_test.go` | 1644 | owner claim、enrollment、未知群离开、同群串行、运行时注册、持久化失败。 | 是 | **混合**。删除全局 owner/enrollment 的业务断言；重写为 bot 加入/移出写 `chat`、每群独立管理员、幂等和事务一致性测试。 |
| `cmd/vestibule/runtime_registration_integration_test.go` | 132 | 注册后无需重建配置即可激活验证/管理/lookup。 | 是 | **行为**。改为 bot 加入后创建 chat/settings，服务立即按该群配置工作。 |

## 三、跨包重复

| 重复逻辑 | 现在的位置与差异 | 目标归属 |
|---|---|---|
| 可变设置的默认/覆盖解析 | `config/config.go` 直接解析全局与群配置；`store/baseline.go` 用 JSON 字段存在性补来源；`store/settings.go` 再合并 runtime override；`verify/service.go`、`moderate/service.go`、`lookup/http.go`、`panel/*.go` 各自读 effective 值。旧 global/control-group 规则渗入业务包。 | `settings` 唯一计算 `defaultValue`、`overrideValue`、`effectiveValue`、`source`、`locked`。`verification`/`moderate`/`lookup` 只通过 Store/DTO 得到群设置。 |
| JSON 状态持久化和“读坏不覆盖” | `store/json.go` 是通用原子文件层；`verify/state.go:123–593` 管四种状态；`moderate/state.go` 管 warning；`feed/feed.go:343–380` 管 cursor；`cmd/main.go`、`registration.go` 另有 runtime 文件状态。每条路径的不可读、保存失败和恢复语义不同。 | `database` 的迁移、事务和 repository；`verification` 用条件转换，`feed`/`moderate` 各有 repository。旧 JSON 只作为一次性导入输入。 |
| Telegram 发送、删除、429 与重试 | `tg/tg.go:75–229` 有 HTML fallback、删除 timer retry；`verify/service.go`/`state.go` 直接发送挑战、告警和删除；`feed/feed.go:381–400,525–625` 自己节流/分类编辑；`moderate/service.go`、`panel/*.go`、`registration.go` 直接 `SendMessage`。 | `telegram/queue` 是唯一发送路径：durable outbox、每群 FIFO、全局限流、429 持久化退避。业务包只写动作；connector 负责非消息 API 的有界调用。 |
| 外部 HTTP 调用、超时和体积限制 | `lookup/http.go:62,250–355` 已集中 HTTP client、并发槽、状态码和 body 上限；`lookup` 的各数据源自行给不同 timeout/limit；`feed/feed.go:235–302` 复用 `lookup.GetJSON`。它不是重复实现，但跨 feature 的依赖未被明确成端口。 | 不新造 `util`/`common` 包。`lookup` 保留有界 HTTP transport 并暴露窄接口给 `feed`；每个查询仍拥有自己的 body 上限和失败语义；`status` 只记录指标/断路状态。 |
| 进程内 timer、重试和长期循环 | `verify/state.go:632–782` 用 per-pending timer；`tg/tg.go:179–229` 用 delete timer；`feed/feed.go:771–976` 用轮询 ticker；`cmd/main.go` 管 polling retry；`registration.go` 管未知群离开 deadline；`panel/session.go` 管 session/tombstone TTL。 | `app` 统一注册/停止长期任务；`verification` 的到期由数据库扫描领取；`telegram/queue` 处理消息可重试动作；`console/auth` 处理 session 到期；不把业务 deadline 留在内存。 |
| 有界缓存与 ID 驱动 map | `tg/tg.go` 有 admin、linked chat、alert、cleanup map；`lookup` 有 pkg/version/news/release/kernel cache；`verify` 有 `pend`、recent pass、cooldown、agent tally；`panel/session.go` 有 session/token；各自有不同 TTL、容量和淘汰方式。 | `verification` 不再以内存保存 challenge；`telegram` 保留有界短 TTL 的平台查询缓存，但敏感写入必须 fresh；`lookup` 保留有界结果缓存；其余持久事实进入 `database`。每个缓存必须有容量、TTL、指标和失效行为。 |
| 时长解析和面向用户的时长文案 | `config/config.go` 有秒数/Telegram 边界；`verify/service.go:624–666,2541–2585` 有验证时长/ban 文案；`verify/state.go:1073–1098` 有 outage 文案；`moderate/service.go:530–579` 有处罚语法/文案；`panel/settings_input.go:797–833` 又解析输入。 | 值域和单位转换归 `settings`/`rules`；业务返回结构化 duration/reason；Telegram 文案由 `i18n` + `telegram/tgfmt`，控制台由前端 locale 格式化。 |
| Telegram 标识转换 | 多处裸 `int64`：`tg/tg.go` 到 `tu.ID`，`verify` 在 callback/string 中手工转换，`panel/codec.go` 编码有符号整数，`moderate/antispam.go` 解析 `-100`/`t.me/c`，`feed` 拼 chat ID 文件名。 | `telegram/ids` 统一 ChatID/UserID/MessageID 与 Telegram wire 互转；`verification` 用自己的 opaque domain ID，禁止在业务代码拼 callback 或裸传平台整数。 |
| 用户可见文案和 HTML 拼接 | `verify/service.go`、`verify/state.go`、`verify/kernel.go`、`moderate/service.go`、`lookup/*.go`、`feed/feed.go`、`panel/*.go` 都直接 `i18n.Render`、`fmt.Sprintf`、`html.EscapeString` 或构造 telego markup。 | `i18n` 只保存文本；`telegram/tgfmt` 负责 Telegram HTML/长度/转义；`console` 返回 DTO，前端渲染。`verification` 与 `rules` 只返回结构化原因/结果。 |
| 管理员身份检查 | `tg.Client` 同时提供 Cached/Fresh；`verify/service.go`、`moderate/service.go`、`panel/panel.go`、`panel/settings_input.go`、`registration.go` 各自决定何时 fresh。旧 panel 中同一操作有 cached/fresh 两种路径。 | `telegram` 提供成员查询；`console/auth` 校验身份；所有敏感写入统一 fresh admin；`verification` 只接收已授权的 actor 和领域请求。 |

## 四、目标包的填充度

预计行数只计生产 Go，不计测试、生成的 OpenAPI/TypeScript、`web/dist` 或 SQL 文件。以下估算均为 `[推断]`，依据现有职责分配和目标架构，不是把当前行数机械相加。

| 目标包 | 由现有哪些代码填充 | 缺什么需要新写 | 预计行数 |
|---|---|---|---:|
| `cmd/bot` | `cmd/vestibule/main.go` 的 flag/version 入口。 | 只保留 CLI 解析和 `app.Run` 调用；移除业务装配。 | ~80 |
| `internal/app` | `main.go` 生命周期、`sd_notify.go` 的部分 systemd 协调、`bot/bot.go` 装配意图。 | 依赖图、启动顺序、后台任务注册表、两阶段停止、任务 supervision。 | ~600 |
| `internal/verification` | `verify/service.go` 的 join/answer/settle/postjoin 行为，`state.go` 的处罚/恢复语义，`kernel.go` 的流程部分。 | 领域类型、ports、Store 条件转换、outbox 动作、超时扫描、管理员控制台共用服务。 | ~2,600 |
| `internal/rules` | `verify/kernel.go:22–232,407–530,838–853`，`moderate/antispam.go` 的纯策略片段，题目校验。 | NFKC 归一化、封闭条件集合、规则 definition、结构信号、线上/试答共用入口。 | ~750 |
| `internal/telegram` | `tg/tg.go` 的 connector/成员操作，`bot/*.go` 的路由，`main.go` 的 polling，`registration.go` 的 membership 更新。 | `verification.Gateway` 实现、Webhook/long-poll lease、Update→领域事件、编译期接口断言。 | ~1,000 |
| `internal/telegram/ids` | `tg/tg.go`、`panel/codec.go`、`moderate/antispam.go` 的分散 ID 转换。 | 强类型 ID、Bot API 互转、chat-link/channel ID 解析。 | ~140 |
| `internal/telegram/tgfmt` | verify/panel/moderate/lookup/feed 中的 HTML、转义、文本限长和 i18n 渲染。 | 按消息方向拆格式化器、结构化结果→Telegram HTML、长度裁切和测试。 | ~650 |
| `internal/telegram/queue` | `tg/tg.go` 删除 retry/清理、`feed/feed.go` 发送节流/429 分支。 | durable outbox、lease、群 FIFO、全局限流、429 jitter、backpressure、drain/metrics。 | ~900 |
| `internal/telegram/store` | 无直接等价代码。 | 适配器独立 migration/version 表、poll/webhook lease、队列辅助持久状态。 | ~250 |
| `internal/console/api` | `panel/settings_input.go` 的设置校验、`settings_panel.go` 的资源操作、`panel.go` 的状态/权限意图。 | OpenAPI 生成接口、路由、DTO、错误码、只调用 `verification`/`status`。 | ~700 |
| `internal/console/auth` | `panel/session.go` 的会话绑定/重放防护，`panel/*.go` 的 fresh admin 意图。 | Mini App `initData` HMAC/replay cache、OIDC PKCE/state/nonce、8 小时会话、写前授权。 | ~600 |
| `internal/console/assets` | 无。 | `go:embed` 前端产物、SPA fallback、无前端产物时仅 API 的启动路径。 | ~60 |
| `internal/settings` | `config/config.go`、`store/baseline.go`、`store/settings.go` 的默认/校验/来源语义。 | `defaults.yaml` embed、YAML load、环境变量机械映射、secret reference、`configupgrade` 复制规则、provisioning 校验。 | ~800 |
| `internal/database` | `store/json.go`，verify/moderate/feed/registration 的文件状态模型。 | dbutil 装配、迁移、SQLite/PostgreSQL dialect、challenge/chat/rule/feed/audit repository、旧 JSON 导入与兼容下限。 | ~1,800 |
| `internal/status` | `tg/redact.go`、`main.go` liveness/outage、`moderate/service.go` 权限预检、verify stats。 | `/livez`、`/readyz`、诊断、结构化日志、指标、redaction、权限/队列/任务健康。 | ~500 |
| `internal/i18n` | 10 个现有 i18n 源文件和 locale 数据。 | 删除 edition token/社区分支；增加 console/frontend 资源一致性校验。 | ~1,300 |
| `internal/lookup` | `lookup/*.go` 的解析、缓存、HTTP transport、查询 use case。 | 按领域拆小文件、显式 transport 注入、从 Telegram handler 中剥离展示。 | ~3,000 |
| `internal/feed` | `feed/feed.go` 的 fetch、cursor、状态编辑、过滤和周期行为。 | DB repository、queue Gateway、每源暂停/恢复、与 app task registry 集成。 | ~950 |
| `internal/moderate` | `moderate/*.go` 的处罚、warning、反频道身份和权限规则。 | 不含 telego 的 use case、audit/database repository、规则引擎调用、控制台 DTO 支持。 | ~700 |
| `internal/panel` | `panel/panel.go` 的少量 Telegram 信息/快捷命令意图。 | 仅保留目标架构仍要求的 Telegram 命令薄层；旧设置面板不在此包重建。 | ~180 |
| `internal/edition` | 两个 edition 文件中的版本元数据意图。 | 单一产品 metadata，取消 build tag、社区身份和命令前缀。 | ~30 |
| `web/`（非 Go） | `panel/settings_panel.go`、`settings_input.go` 的屏幕能力和 `i18n/panel.go` 的文案资产。 | Vite/React、OpenAPI 客户端、页面、React i18n、无后端 mock、构建产物。 | ~6,000 TypeScript/CSS |

非测试文件没有无法判定去向的条目：46 个都有明确处置。

事故来源方面，标记为“未证实”的测试不是被忽略，而是仓库没有足以证明其来自生产事故的出处；它们仍按行为资产保留并重写。

四张表的条目数：

1. 非测试文件：46
2. 测试文件：88
3. 跨包重复：10
4. 目标包填充：22
