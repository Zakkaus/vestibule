# v5 方案计划书

从单体机器人改造为「一个机器人服务多个群 + Web 控制台」。
**本文件规定做什么、按什么次序做、以及每步做完的判据。** 其余依据：

| 要查什么 | 看哪份 |
|---|---|
| 界面取值、屏的内容、文案规则 | `web/design.html` |
| 包结构、数据、流程、稳定性 | `web/architecture.html` 或 `docs/ARCHITECTURE.md` |
| 构建、提交、PR 前检查、代码风格 | `CONTRIBUTING.md` |

## 0. 目标与范围

### 语言

提交信息用英文。`README.md` 用英文，另给 `README.zh-CN.md`。
面向使用者的文档与界面文案用中文，过书写检查。代码注释用英文。

### 全程质量限制

**每次改动均按完整架构实现，不采用临时实现后再整理。**
文件 600 行、函数 80 行、圈复杂度 15 是硬性上限，一次提交只涉及一组职责。
新代码必须落在架构书已声明的包里；需要新包时先改架构书。
禁止出现 `util`、`common`、`helper`、`misc` 这类无边界的包。

上限是触发讨论的阈值，不是可以逼近的目标。**接近上限时先问是不是分错了包。**
细则见 `docs/ARCHITECTURE.md`，检查在阶段零建立。

### 目标

1. 任何人把机器人拉进自己的群即可使用，由该群的 Telegram 管理员自行配置。
2. 有 Web 控制台，覆盖当前 Telegram 面板与配置文件里的每一项设定。
3. 状态在数据库，并发与重启之下不丢、不重复结算。
4. 一条命令部署，升级失败自动回退。

### 功能范围

这一轮要做的功能，按屏列出。每一项的界面规定见 `web/design.html`，
结构规定见 `docs/ARCHITECTURE.md`。

| 屏 | 内容 |
|---|---|
| 首页 | 四层：概况四个数字、需要注意、近期趋势、常用配置入口 |
| 等待队列 | 正在等待的申请，放行、拒绝、封禁 |
| 验证方式 | 投递渠道与五种挑战类型；被邀请入群三选一；超时、失败上限、封禁时长 |
| 题库 | 规则式题目的增删改、内置模板、试答、三语、导出导入 |
| 反垃圾 | 四条出厂默认规则加自定义规则、分数累加、四档处置、先观察不处置 |
| 免验证来源 | 信任群、必关频道、白名单，查询失败时的方向 |
| 群与频道 | 机器人所在的群、当前群模式、拥有者绑定 |
| 管理与处罚 | 警告上限、反频道马甲、处罚记录去向 |
| 消息与文案 | 自动回复、进出提示、验证全流程文案，留空即用默认 |
| 订阅推送 | RSS 与 Atom 与 JSON Feed、过滤、静默、失败暂停 |
| 统计 | 结果趋势、通过率、各方式拦截量 |
| 诊断 | 权限预检、接口延迟、心跳、一致性检查 |
| 功能 | 能力开关，默认全关，另给两个预设按钮 |
| 偏好 | 标记、名字、强调色、标题图标、保存方式 |

### 五种验证方式

| 方式 | 备注 |
|---|---|
| 入群选择 | **已被攻破**，此备注必须显示在后台的选项旁边 |
| 入群问答 · 输入 | 答案可被检索或传播后失效 |
| 入群问答 · 规则 | 内核版本号一题即此类，靠不存在的诱饵筛选 |
| 工作量证明 | 仅网页。难度按二进制位计，不按十六进制字符计 |
| 人机验证 | 仅网页。依赖外部服务，失败方向需配置 |

条件类型是封闭集合，作用于三处：验证答案、消息文本、显示名与个人简介。
新增条件类型应通过配置实现，而非增加专用代码分支。

### 不在本轮范围

- 付费与配额计费。
- 移动端原生应用。
- 不把 `lookup` 与 `feed` 拆成独立服务。它们保持在同一进程内；模块开关是实例级配置，因为 Telegram 的处理器注册和默认命令菜单属于进程级表面。
- 多实例水平扩容的实际部署。数据模型按它设计，但本轮仍单实例运行。

### 验收标准

**删除本社区的专用配置后，产品仍应正常运行。** 每个阶段结束时均须满足该条件。

### 持续测试环境

编译和自动化测试不能覆盖机器人在群中的实际行为。**从首个可执行版本开始，
每个阶段均在该环境中实际运行一次**，不将实机验收集中到最后。

| 用什么 | 在哪 |
|---|---|
| 测试机器人 | `@NvidiaH200Bot`，令牌在 `/scratch/ssd/verify-bot-test/bot.env`，权限 `600`。不进仓库，不进日志，不在任何地方回显 |
| 测试群 | `t.me/zakkbottest`，超级群，`chat_id` 为 `-1004330294599`。补 `-100` 前缀，超级群的 id 才是这个形状 |
| 正式机器人 | `@VestibuleBot`，留到切换那一步，开发期间不用 |

这套环境与生产完全隔开：**测试期间不碰任何生产群，也不碰上一代的令牌与状态目录。**

能在这里验的，就必须在这里验。验证流程要真发起一次入群申请，看挑战有没有发出、
超时有没有结算、通过之后禁言有没有抬走。这些没有一条能靠单元测试证明。

**三项前置已全部满足**，验证可以在这里实机执行。它们共同决定一个群能不能验证，
而三者失效时的表现完全一样：什么也不发生。

| 前置 | 状态 | 缺了会怎样 |
|---|---|---|
| 群是超级群且开了「需要管理员批准」 | 已开 | 平台不产生加群申请事件 |
| 机器人是管理员，有邀请、封禁、删除三项 | 已给满 | 收得到但结算不了 |
| 拉取时声明 `chat_join_request` | 由我们的代码保证 | 一条申请都收不到，且不报错 |

前两项只能由群主在 Telegram 里点，第三项是我们自己的配置。
提为管理员之后隐私模式那一条也一并解决：管理员身份的机器人收得到群里全部消息，
反垃圾才有得测。

程序在启动时就要检查这两项并把缺的逐条报出来，
而不是安静地跑着、等到有人申请进群时才发现什么都没发生。

## 1. 阶段划分

一个阶段一个分支一个 PR。每阶段结束时线上可发布，不存在「做到一半的中间态」。

| 阶段 | 分支 | 内容 | 状态 |
|---|---|---|---|
| 零 | `v5/gates` | 先立尺子：行数、复杂度、包边界检查与基线冻结 | 完成 |
| 一 | `v5/skeleton` | 建目标包结构，按清点结果把代码搬进去，接口定义在使用方 | 完成 |
| 二 | `v5/rules` | 规则引擎独立成纯函数包：归一化、条件类型、结构信号 | 完成 |
| 三 | `v5/database` | 数据层换 dbutil，状态入库，待执行动作表，实例租约 | 完成 |
| 四 | `v5/config` | 配置换 configupgrade，分成三层 | 完成 |
| 五 | `v5/console-api` | 接口契约、`internal/console`、Mini App 认证与授权 | 完成 |
| 六 | `v5/console-*` | 前端骨架与一条通路：进入、选群、看队列、放行一个人。按屏分片，见该节 | 完成 |
| 七 | `v5/console-screens` | 其余各屏 | 完成 |
| 八 | `v5/multitenant` | 去掉全局默认，配置按群隔离 | 完成 |
| 九 | `v5/deploy` | 一次部署、健康检查、失败自动回退 | 完成 |
| 十 | `v5/cutover` | 从上一代迁移、并行观察、随时可退回 | 未开始 |
| 十一 | `v5/asked-for` | 维护者指定的功能：主题、两种新语言、拥有者绑定、控制群、试答、日报、结构信号 | 未开始 |

**上一阶段合入 `main` 之前不开始下一阶段。** 每阶段结束时线上可发布。

**分支一列是这个阶段作为单支交付时用的名字。** 一个阶段常常拆成几片，
每片一支描述性分支、一个 PR。已交付的部分里只有 `v5/gates`、`v5/rules`、
`v5/config`、`v5/console-api` 真的以表里这个名字合入，其余都是按片命名的。
所以这一列定的是归属，不是承诺只开一支。
详细节里若另有说明（阶段七写明一屏一支），以详细节为准，
`scripts/check-docs.py` 会拒绝两处给出互相矛盾的单一名字。

**状态列随合入更新。** 待决表为每项记录决策阶段；
若该阶段已经完成，`scripts/check-docs.py` 会拒绝仍未处理的决策。
合入条件包括质量门禁通过、阶段验收脚本通过，以及新增检查先验证失败路径。
任一条件不满足时，须停止合入并报告具体阻塞项。

### 阶段零 · 建立质量门禁

**分支** `v5/gates`

阶段零在实质改动前建立度量和边界检查。

| 检查 | 内容 |
|---|---|
| 文件行数 | 超过 600 行即失败，列出超限文件与行数 |
| 函数行数 | 超过 80 行即失败 |
| 圈复杂度 | 超过 15 即失败 |
| 包边界 | 验证包不许引用 telego；规则包不许引用数据库与网络 |
| 中文 | 面向使用者的文案过书写检查 |

阶段零实施时，`internal/verify/service.go` 有 2,859 行，现有代码必然超过限制。
因此，该阶段还建立了基线清单，逐项记录并冻结既有超限项；
检查只禁止新增和恶化，不要求一次修复全部问题。后续阶段分别处理所属范围内的条目。

每项检查都先验证失败路径，再恢复通过，具体流程见工作流第 06 节。

**验收结果**：`scripts/lint.sh` 可执行；将文件扩展至 601 行时，
脚本在文件行数检查中失败；基线条目在冻结时均可追溯至具体文件和行号。

「在冻结那一刻」是后加的，因为原文读起来像一条持续成立的性质，而它做不到：
基线是阶段零的快照，之后的重写把每个文件都搬过一遍。现在数出来是
117 行里 36 行仍然命中、23 行的文件已不存在、58 行的行号不再落在那个函数上。
这与 `docs/INVENTORY.md` 是同一类冻结文档，参见那一节。
**其中 23 行可以移出基线**：`CONTRIBUTING.md` 写着行可以离开基线、不可以加入，
而一个文件已经不存在的违规不再是违规。移除时要同时下调 `scripts/held.txt`，
还没做。

原验收命令是 `make lint`，但仓库始终没有 Makefile，
因此该命令无法执行。实际入口是 `scripts/lint.sh`，
贡献指南的门禁清单也使用该命令。

**不做**：不在这个阶段修任何超限项。建尺子与用尺子分开。

### 阶段一 · 建目标包结构

**分支** `v5/skeleton`

#### 分片依据

清点的 46 个来源文件中有 13 个标为 `重写`。验证、设置和持久状态同时改变依赖方向与状态模型；
为区分搬移、端口边界和状态转换中的问题，阶段一分为三片：

| 分片 | 边界 |
|---|---|
| 1A · 无状态边界 | 迁移 `lookup`、`i18n`、日志脱敏、标识与格式化职责；不改变持久状态和外部动作时序。 |
| 1B · Telegram 与装配边界 | 将 Update 路由、Gateway 实现、命令注册和生命周期装配迁至 `telegram`、`app` 与 `cmd/bot`；核心仅通过端口调用外部服务。 |
| 1C · 核心端口边界 | 将验证流程迁至 `verification`，通过 `Store` 和 `Gateway` 两个端口隔离既有状态；数据库介质由阶段三更换。 |

每片完成后，两个构建标签均可编译，两套测试均通过。

下列 `现路径` 均是 `docs/INVENTORY.md` 的来源追溯。阶段一搬移后，后续阶段修改对应目标路径；同一来源文件因后续重写再次出现时，行数是该阶段的接触面，不能将各阶段合计为全仓行数。

**命名已定：`console`。** 架构图和流程原本把这条 HTTP 边界叫 `adminhttp`，
而包结构声明的是 `internal/console`，同一件事两个词。统一到后者：
那个包装着认证与内嵌的前端产物，不只是一个 HTTP 监听器，`adminhttp` 只命名了其中最小的一块；
而「控制台」是产品在设计文档里通篇用的词。架构文档四处已改，**不存在 `adminhttp` 这个包**。

**这是一次重排，不是原地整理。** 旧版仍在生产运行，本仓库另起一份，
因此直接朝目标架构搬，不做「先同包拆再搬第二次」。

阶段一依据 `docs/INVENTORY.md` 的逐文件去向，建立架构文档声明的包：

```
internal/app  verification  rules  telegram  console  settings  database  status
```

`verification/ports.go` 定义 `Gateway` 与 `Store`，
由 `telegram` 和数据层实现；接口位于使用方。
编译期断言用于约束实现。

架构文档的端口章节记录必须承载的能力，不预先指定未经实现验证的签名。
阶段一根据调用点和「核心不包含平台类型」等不变量推导接口，
再将实际签名写回架构文档。

没有 `Clock`：`ClaimExpired` 直接收 `now`，少一个接口，测试传值即可。

测试随代码迁移。25,569 行测试中包含由生产问题形成的行为规格；
仅依赖具体实现的断言改写为行为断言，没有直接删除。

**范围限制**：阶段一未改变判定逻辑和锁结构，未删除测试覆盖的行为，
也未建立架构文档未声明的包。

**验收结果**：两个构建标签均可编译，两套测试均通过；
`grep -r telego internal/verification` 无结果；
每个目标包均有编译期接口断言，阶段零的四项检查没有新增超限项。

#### 文件处置

**46 个文件，21,434 行非测试 Go 代码。** 表按清点处置分组；逐段去向和测试资产细节仍以 `docs/INVENTORY.md` 为准。

| 现路径 | 处置 | 目标位置 |
|---|---|---|
| `internal/tg/redact.go` | 移入 | `internal/status` |
| `internal/i18n/{lookup_content,lookup_distros,lookup_packages,moderate,feed,doc}.go` | 移入 | `internal/i18n` |
| `internal/verify/kernel.go` | 拆分 | `internal/rules`、`internal/verification`、`internal/settings`、`internal/telegram/updates.go`、`internal/telegram/tgfmt` |
| `internal/tg/{tg,errors}.go` | 拆分 | `internal/telegram/connector.go`、`internal/telegram/tgfmt`、`internal/telegram/queue` |
| `internal/panel/panel.go` | 拆分 | `internal/telegram/updates.go`、`internal/status`、保留的 `internal/panel` 命令薄层 |
| `internal/lookup/{repology,content,cve,distros,http,kernel,manpage,packages}.go` | 拆分 | `internal/lookup`、`internal/telegram/updates.go`、`internal/telegram/tgfmt`、`internal/settings` |
| `internal/moderate/{antispam,service}.go` | 拆分 | `internal/moderate`、`internal/telegram/ids`、`internal/telegram/updates.go`、`internal/settings`、`internal/status` |
| `internal/bot/{bot,commands,dm}.go` | 拆分 | `internal/telegram/updates.go`、`internal/telegram/tgfmt`、`internal/app`、`internal/status`、`internal/settings` |
| `internal/feed/feed.go` | 拆分 | `internal/feed`、`internal/database`、`internal/telegram/queue`、`internal/telegram/tgfmt`、`internal/status`、`internal/app` |
| `internal/i18n/{bot,catalog,verification,panel}.go` | 拆分 | `internal/i18n`；控制台文案迁至 `web/` locale |
| `cmd/vestibule/sd_notify.go` | 拆分 | `internal/app`、`internal/status`、`internal/telegram` |
| `internal/verify/{service,state}.go` | 重写 | `internal/verification`、`internal/database`、`internal/telegram/tgfmt`、`internal/telegram/queue`、`internal/status` |
| `internal/store/{baseline,settings}.go` | 重写 | `internal/settings`、`internal/database` |
| `internal/panel/{session,settings_input,settings_panel}.go` | 重写 | `internal/console/{auth,api}`、`internal/settings`、`internal/rules`、`web/` |
| `internal/moderate/state.go` | 重写 | `internal/moderate`、`internal/database` |
| `internal/config/config.go` | 重写 | `internal/settings`、`internal/rules` |
| `internal/edition/{edition_gentoo,edition_generic}.go` | 重写 | `internal/edition` |
| `cmd/vestibule/{main,registration}.go` | 重写 | `cmd/bot`、`internal/{app,status,telegram,database,settings}` |
| `internal/store/json.go` | 删除 | 无；由 `internal/database` 的事务、迁移和导入承接 |
| `internal/panel/codec.go` | 删除 | 无；由 `internal/console/{auth,api}` 的会话和 DTO 校验承接 |
| `internal/bot/edition.go` | 删除 | 无；功能改由每群配置控制 |

#### 必须保住的行为

- 两种入群模式、可信群绕过、冷却、群内与私聊投递、确认送达后才计时、nonce/epoch 防旧事件、管理员结算、失败不误罚、挑战清理和不重复验证。
- 真实 `uname` 输出与命令回显的判定、跨群 fallback 隔离、三次尝试、归一化后的规则命中，以及结构信号不依赖 Telegram 或数据库。
- 已删除消息视为成功、敏感操作现查管理员、恢复群默认权限、发送经队列并遵守 `429` 退避；日志在所有调用点都不得泄露 bot token。

**私聊消息的清理是有意反转的，不是保住的行为。** 上一代结算时把群内和私聊的挑战都删掉
（`~/code/refs/gentoo-zh-verify-bot/internal/verify/service.go:2173-2176`，
断言在 `~/code/refs/gentoo-zh-verify-bot/internal/verify/verify_test.go:1232`）。
这一代只删群内那条：群内消息是公开的挑战证据，删掉它是为了不把某个人被拦下这件事留在群里；
私聊里的题目和结果是申请人自己那一份记录，删掉它等于替他抹掉发生过什么。
架构文档记着这个决定（`docs/ARCHITECTURE.md:802`、`:849`）。
该行为变化在此单独记录，供生产切换前对照，
并同步影响隐私说明中申请人私聊记录的描述。
- 设置的来源、稀疏覆盖、revision 冲突与整份校验；控制台和 Telegram 命令的目标群隔离、过期与重放防护、写入 fail closed。
- lookup 的“未找到”与上游故障区分、有界 HTTP 与缓存、版本排序和固定上游样本；moderation 与 feed 的处罚、cursor、投递失败和暂停语义。
- 生命周期的路由优先级、单个 Update 只进入预期 handler、先注册后拉取、关闭顺序和运行时群加入/移出语义；所有用户可见文本继续经 `i18n`。

#### 依赖

依赖：阶段零的门禁、基线清单和两个构建标签均已可执行，第一片开始前先以它们冻结当前行为。


### 阶段二 · 规则引擎

**分支** `v5/rules`

阶段二将 `internal/rules` 建为纯函数包，包含归一化流水线和封闭的条件类型集合。
包边界检查禁止该包导入数据库和网络相关包。

验证答案、消息文本、显示名与个人简介共用同一套条件类型。

阶段二独立交付，使控制台试答可以直接调用生产判定入口，
避免维护另一份可能与线上行为不一致的实现。

**验收结果**：包边界检查通过；`testdata/spam` 中每种规避方式均有样本，
并断言归一化后可以命中；判定入口保持为纯函数。

阶段二依据原 README 中的清单补齐了 `testdata/spam` 样本。
结构信号不属于该阶段，现已归入阶段十一。

#### 文件处置

**2 个来源文件。** 原清单还将 `state.go:21–177` 误列为结构信号：

- 该段代码在两代实现中均为 **AI tripwire 的自称模型计数**，属于 `agents.json` 状态，
  已归入阶段三。
- **结构信号在两代代码中均不存在**，属于架构文档新增设计，现已归入阶段十一。

| 现路径 | 处置 | 目标位置 |
|---|---|---|
| `internal/verification/kernel.go` | 拆分 | `internal/rules/{normalize,condition}.go`、`internal/verification`、`internal/telegram/updates.go`、`internal/telegram/tgfmt` |
| `internal/moderate/antispam.go` | 拆分 | `internal/moderate`、`internal/telegram/ids`、`internal/telegram/updates.go` |

`antispam.go` 中读取群配置的代码最终迁入阶段四建立的 `internal/settings`；
阶段二未提前建立该包。

#### 必须保住的行为

- 内核版本题仍接受真实命令输出并剥离回显；Windows 和 macOS 转入 fallback；全角分钟证明、nonce 绑定和三次尝试不变。
- 同一用户跨群使用 fallback 不误扣次数，同题库可复用、不同题库不串题；线上判定和控制台试答调用同一纯函数入口。
- 归一化后仍能识别规避写法。结构信号不属于本阶段，现已归入阶段十一。
- 反频道身份策略只影响当前群，保留 4,096 项上限、linked channel 例外、白名单边界和解除白名单后的 unban。

#### 阶段三之后要补的一件：数据库错误不是文件缺失（**已做完**）

一次只读检视量出来的，当时定为落在阶段三之后、部署之前必须补。
这一节现在是记录，不是待办：下面五条逐条核过，都已落地。

问题是换介质时把 JSON 的 best-effort 语义一起带了过来。`LoadPending` 出错直接返回，
遗留注释中的「损坏文件已备份」仅适用于文件存储。
数据库读取失败通常表示连接瞬断，而 `pending` 记录仍然存在：
新申请因 `challenge_open` 冲突无法发出挑战，旧按钮又因内存中没有 nonce 无法结算，
导致申请人无法继续验证。同类问题当时还存在于 `LoadFailures`、`LoadAgents` 和
`LoadWarnings` 出错同样退化为空状态，而随后的快照写入会先 `DELETE` 整张表再写回
本进程看到的那些，历史冷却、失败次数和警告计数**静默消失**；三个 `Save` 的错误被丢弃；
`verification.New` 与 `moderate.New` 无法返回恢复错误，装配层只能继续启动。

现在的样子：

| 当时的问题 | 现在 |
|---|---|
| `LoadPending` 出错静默退化为空 | `internal/verification/state_restore.go:19` 先 `disablePendingState`，再把错误包着返回 |
| 三个 `Save` 的错误被丢弃 | `internal/verification/state.go:108`、`:298`、`:754` 都走 `retryStoreWrite`，失败落日志 |
| `moderate` 的 `LoadWarnings` 同形状 | `internal/moderate/state.go:36` 记下 `loadErr` 并把错误返回 |
| 构造函数无法返回恢复错误 | `internal/verification/service.go:193` 与 `internal/moderate/service.go:55` 都返回 `error` |
| 装配层只能继续启动 | `internal/app/app.go:89-92` 在 `newBaseServices` 返回错误时终止启动 |

「出错」与「条件不匹配影响 0 行」的区分由
`internal/database/verification_store.go:198` 的 `changedRow` 承担：读不出受影响行数是错误，
受影响 0 行只是「已经是那个状态」。`internal/verification/state_write_failure_test.go`
证明的旧文件语义仍在，没有被这几处改动动过。

#### 依赖

依赖：阶段一第三片已把验证核心置于端口之后，核心中不出现平台类型。
`internal/rules` 由阶段二创建；阶段一第三片没有建立或承诺建立该包。
`scripts/boundaries.txt` 已为其预留纯函数边界，包创建后立即生效。


### 阶段三 · 数据层

**分支** `v5/database`

阶段三引入 `go.mau.fi/util/dbutil`，建立 `migrations/00-latest.sql`，
并将既有 JSON 状态一次性迁入数据库。表结构见 `docs/ARCHITECTURE.md` 第 6 节「数据模型」。

阶段三同时完成以下五项：

1. `challenge_open` 使用部分唯一索引，在数据库层拒绝重复记录。
2. 所有状态转换改为带条件更新，影响 0 行时按已结算处理。
3. 扫描器领取到期记录，替代进程内定时器。
4. 状态转换与动作意图在同一事务中写入 `pending_action`，再由执行器完成动作。
5. 拉取更新的实例租约写入数据库，而非内存。

#### 分片依据

这五项存在依赖顺序：先建立数据库，再实现带条件更新，最后以扫描器替代定时器。
阶段三按介质、判定、时间与动作分片，以便定位验收失败。

| 分片 | 边界 |
|---|---|
| 3A · 介质 | 引入 `dbutil`、`migrations/00-latest.sql`、仓储与一次性导入命令。读写从 JSON 换成数据库，**判定与时序不变**：定时器还在进程里，动作仍直接执行。 |
| 3B · 判定落库 | 上表第 1、2 件：`challenge_open` 部分唯一索引，以及全部状态转换改为带条件的更新，影响 0 行按已结算处理。 |
| 3C · 时间与动作 | 上表第 3、4、5 件：扫描器领取到期记录并删除进程内定时器、`pending_action` 表与执行器、实例租约入库。 |

3C 实施前，架构文档尚未定义 `pending_action` 和实例租约的表结构。
阶段三根据实际调用点推导表结构，再写回 `docs/ARCHITECTURE.md` 第 6 节。
`scripts/check-docs.py` 比对迁移和该节的表名，防止只更新一处。

3A 至 3C 均在两个构建标签下编译成功，并通过两套测试。
阶段三的整体验收在 3C 完成后执行；分片仅用于定位问题，不降低验收标准。

一次性导入命令支持重复执行且结果一致，并在导入前备份原 JSON 文件，
导入后校验记录数量和关键字段。

**验收结果**：迁移可以重放；旧二进制连接新数据库时拒绝启动；
使用旧数据导入后，等待队列与操作记录数量与导入前一致。
测试群实机验收覆盖状态入库、扫描领取超时记录和带条件结算。

#### 文件处置

**8 个文件，7,395 行**，其中 `internal/tg/tg.go` 那一行看起来已由阶段一第二片做完：
该来源文件不存在，它的三个去处都已就位，HTML fallback 与管理员缓存在 `connector.go`、
删除重试在 `telegram/queue/delete.go`。动手前先确认这一行还剩什么，
剩下的行数按实际接触面算，不要照搬这里的七千余行。

| 现路径 | 处置 | 目标位置 |
|---|---|---|
| `internal/tg/tg.go` | 拆分 | `internal/telegram/connector.go`、`internal/telegram/tgfmt`、`internal/telegram/queue` |
| `internal/feed/feed.go` | 拆分 | `internal/feed`、`internal/database`、`internal/telegram/queue`、`internal/telegram/tgfmt`、`internal/status`、`internal/app` |
| `internal/moderate/service.go` | 拆分 | `internal/moderate`、`internal/telegram`、`internal/status`、`internal/settings` |
| `internal/verify/{service,state}.go` | 重写 | `internal/verification`、`internal/database`、`internal/telegram/queue`、`internal/telegram/tgfmt`、`internal/status` |
| `internal/moderate/state.go` | 重写 | `internal/moderate`、`internal/database` |
| `cmd/vestibule/registration.go` | 重写 | `internal/telegram/updates.go`、`internal/database`、`internal/settings` |
| `internal/store/json.go` | 删除 | 无；迁移、事务和仓储位于 `internal/database` |

#### 必须保住的行为

- 两种入群模式、可信群、冷却和挑战投递保持原有判定；待验证记录只在可见动作前落库，同一群同一人不重复打开挑战，旧 nonce/epoch 不能结算新挑战。
- 临时故障有上限重试，群已不可达或动作永久失败不循环；频道查询或掉线不得计为申请人失败，恢复前核验成员状态，48 小时后不再延期。
- 代办动作的投递、删除和处罚保留成功、瞬态失败、永久失败三种结果；私聊不自动删除，已不存在消息成功，权限恢复使用群默认权限。
- warning 的群与用户双键、确定性有界驱逐、处罚成功后再删证据、目标管理员保护和失败告警跨重启仍成立。
- feed cursor 不越过未投递项，状态编辑、首次 baseline、确认 ping、永久错误推进、瞬态错误保留和每源失败暂停不变。
- bot 加入、移出和标题更新幂等；同群转移串行，持久化成功后服务立即按该群配置工作。

#### 依赖

依赖：阶段二已提供纯规则判定，阶段一的 Gateway、Store 与任务装配边界已稳定，数据库迁移才可替换现有状态介质。


### 阶段四 · 配置

**分支** `v5/config`

阶段四改用 `go.mau.fi/util/configupgrade`，将既有 schema 折叠为
「旧路径 → 当前路径」复制规则，并删除逐版本分支。

迁移覆盖四条路径。重写前的 `internal/store/settings.go:374-386` 包含：
没有 `version` 字段的版本 0、v1、v2 和当前版本 v3。
阶段四还新增了带完整注释的 `internal/settings/defaults.yaml`。

**验收结果**：四种版本各有一份配置文件并分别执行升级；
用户修改值均得到保留，新字段采用默认值。

`testdata/state/settings.json` 是没有 `version` 字段的版本 0 样例；
阶段四依据 `internal/store/settings.go` 中各版本的结构补齐另外三份，
每份均包含用户修改值。

**已知代价**：拼写错误但语法合法的未知键会在写回时消失。
评审要求每个 struct 字段同时具有示例项和复制规则。

#### 文件处置

**3 个文件，2,500 行。**

| 现路径 | 处置 | 目标位置 |
|---|---|---|
| `internal/config/config.go` | 重写 | `internal/settings`、`internal/rules` |
| `internal/store/baseline.go` | 重写 | `internal/settings` |
| `internal/store/settings.go` | 重写 | `internal/settings`、`internal/database` |

#### 必须保住的行为

- 时长溢出保护、Telegram 处罚边界、模式、语言、题目、feed 间隔和反垃圾默认值继续经过同等严格的值域校验。
- 默认值夹取、显式来源、每群稀疏覆盖、空 override 继承出厂默认和群间隔离不变。
- revision 冲突、整份校验和写入失败时不发布半个新快照不变；旧配置升级保留用户改值，新字段采用默认值。

**这一项归本阶段，不归阶段八。** 架构文档第 10 节与 `docs/INVENTORY.md` 都写着
出厂默认加 `chat.settings`，不存在全局默认或 control group，而阶段八原本也把这项切换
列为自己的工作。**配置模型是这一层的属性，不是事后的迁移**：建新的设置层时不可能
同时保留一个全局默认，那等于先把旧模型再实现一遍、再拆掉它。
阶段八留下的是真正属于多租户的部分，见该阶段。

#### 依赖

依赖：阶段三的 `chat.settings`、迁移和事务已可用。


### 阶段五 · 接口与认证

**分支** `v5/console-api`

阶段五将 `internal/panel` 拆分为 `internal/console/{api,auth}`：
`auth` 负责身份验证，`api` 负责参数校验和服务调用。
群管理员通过 Telegram Mini App 访问，每次请求携带 `initData`；
服务端校验 HMAC 和签发时间，并记录已使用的签名。
运维通过机器人发送的一次性链接换取会话，不依赖 Telegram 登录服务。

接口层完成认证和参数校验后调用 `verification.Service`，
不直接写入 SQL，也不绕过领域层。每次敏感写入前，
服务端均使用访问者的数字 ID 调用 `getChatMember`。

阶段五实现以下端点：

- `POST /api/session` 与 `GET /enter/{token}`
- `GET /api/chats`、`GET /api/chats/{id}/queue` 和
  `POST /api/chats/{id}/queue/{cid}`
- `GET /livez` 与 `GET /readyz`

其余端点按所属屏在阶段七实现。

#### 文件处置

**4 个文件，1,780 行**（清点时是 1,832；阶段四改过其中几处）。

阶段五实施前的 `internal/panel` 是 Telegram 内联键盘设置面板，而非 HTTP 面板。
`OnPing`、`OnStart` 和 `OnStats` 均为 Telegram handler；
会话按用户保存，令牌通过 callback data 传递，当时仓库中尚无 HTTP 服务。
阶段五建立了控制台层，并保留 Telegram 侧仍需承担的职责。

各文件在阶段五的实际接触面如下：

| 文件 | 本阶段 |
|---|---|
| `panel.go` 416 行 | 拆：Telegram 命令（`/ping`、`/start`、`/stop`、`/stats`）留在 `internal/panel`；会话与授权那部分进 `internal/console/auth` |
| `session.go` 337 行 | 重写为 `console/auth`：Mini App 的 `initData` 校验、运维一次性链接换会话、过期与重放 |
| `codec.go` 199 行 | 删除：它编码的是 callback data，控制台不用回调按钮 |
| `settings_input.go` 828 行 | **本阶段基本不动。** 它的 26 个函数是题库、fallback、频道、确认这些屏的输入流程，跟着各自的屏在阶段七落地 |

| 现路径 | 处置 | 目标位置 |
|---|---|---|
| `internal/panel/panel.go` | 拆分 | `internal/console/api`、`internal/console/auth`、`internal/telegram/updates.go`、`internal/status`、保留的 `internal/panel` |
| `internal/panel/session.go` | 重写 | `internal/console/{auth,api}` |
| `internal/panel/settings_input.go` | 重写 | `internal/console/api`、`internal/settings`、`internal/rules` |
| `internal/panel/codec.go` | 删除 | 无；由接口契约、会话、CSRF/state、DTO 校验和 revision 控制替代 |

#### 必须保住的行为

- 身份与群绑定、会话过期、单用户会话和并发重放仅成功一次；权限失效或成员查询失败时拒绝写入。
- 题库、fallback、频道和白名单仍做整份校验；确认删除、目标群隔离和 stale revision 冲突继续可观察。
- 信息命令不要求管理员；写入命令维持群范围限制和 fail closed，不能直接写 SQL 或绕过 `verification.Service`。

**验收结果**：Mini App 的 `initData` 可以换取会话，过期签名和重放均被拒绝；
写入端点缺少 `X-CSRF-Token` 时返回 `csrf_invalid`；
不在授权列表中的群 ID 返回 `chat_not_found`，而非空集合；
接口层不包含判定逻辑，结算均调用 `verification.Service`。

该验收条款在阶段完成后补记，内容来自本节必须保住的行为和接口层职责，
均为阶段五已经交付且有测试覆盖的性质，不构成新增要求。

#### 后续补充：为运维会话提供写入凭据（**已完成**）

原实现中，`GET /enter/{token}` 仅写入 HttpOnly 会话 Cookie，再以 `303` 重定向到首页
（`internal/console/api/server.go:246-263`）；CSRF 令牌仅由
`POST /api/session` 的 JSON 响应返回（`internal/console/api/server.go:223-243`），
而结算要求 `X-CSRF-Token`（`internal/console/auth/manager.go:291-297`）。

因此，通过一次性链接进入的运维可以读取群和队列，但无法执行写入；
当时也没有端点供浏览器获取当前会话的 CSRF 令牌。

当前实现按照架构文档第 11 节的路由表提供 `GET /api/session`，
向已持有会话 Cookie 的浏览器返回 CSRF 令牌。
令牌仍由单一位置签发，没有增加可读的 CSRF Cookie。

#### 后续补充：读取路径使用缓存复查（**已完成**）

原实现中，`AccessibleChats` 对每个已配置群串行执行一次实时 `getChatMember`。
架构文档规定写入路径每次实时检查，读取路径定期复查，
被操作对象始终实时检查。

列出五个群需要五次串行 Telegram 往返，在当前部署位置约需 2.5 秒；
上一代 v3.6.7 已修复过同类问题。

当前实现由读取路径调用 `CachedAdmin`（30 秒 TTL），
写入路径继续调用 `FreshAdmin`，避免权限已撤销的管理员利用缓存继续写入。

#### 依赖

依赖：阶段三的 `verification.Service` 与动作状态机、阶段四的设置读取和 revision 语义均已完成，接口才能只做认证、校验与调用。


### 阶段六 · 前端与一条通路

**分支** 按屏分片，每片一支一个 PR：`v5/console-entry`、`v5/console-groups`、
`v5/console-queue`，其余屏依此类推。

**分片依据**：阶段六按屏交付，每个屏形成独立通路且不共享状态，
因此采用多个分支和 PR，使各屏可以独立验收与回退。
阶段一按依赖方向分片，阶段六按屏分片。

阶段六先确定契约，再依次完成后端和前端。
首条通路是**进入 → 选择群 → 查看等待队列 → 放行申请人**。

进入屏不提供登录表单。群管理员从 Telegram 打开 Mini App，
身份来自每次请求携带的签名数据；运维通过机器人发送的一次性链接换取会话。
该屏覆盖会话换取及失败时的五种状态，详见设计文档「打不开的时候看到什么」。

阶段六还完成了 Mini App 身份校验、写入前实时检查管理员、权限定期同步、
前端目录约定和错误边界。

**验收结果**：架构文档第 11 节的路由表与实现一致；
前端可以在没有后端时使用假数据启动；
端到端用例在真实浏览器中从读取已有会话或通过 Mini App 换取会话执行至成功放行。

改动过的路由均在两个宽度、每个主题和最宽语言下完成渲染检查，
并通过键盘完成整条操作路径。

#### 验收现状

| 条款 | 状态 |
|---|---|
| 最小通路完整可用 | 已达成 |
| 端到端用例在真实浏览器中通过 | 已达成。浏览器优先读取既有会话；仅当 Mini App 提供 `initData` 且没有会话时换取会话 |
| 每条路由 × 两个宽度 × 三个主题 × 最宽语言，并完成键盘操作 | 已达成并已自动化；`web/e2e/render-gate.spec.ts` 由 CI 执行 |
| 一条命令部署后健康检查通过 | 阶段九已完成自动化验收；受 Bot API 凭据配置影响的真实部署仍列为 `EXEMPT` |
| **实机**：控制台连接测试环境数据库并放行测试群中的真实申请 | 阶段九仍将干净机器、域名、证书和浏览器路径列为 `EXEMPT` |

前三项由阶段六完成；后两项属于部署环境验收，阶段九已记录自动化结果和真实环境豁免。

#### 文件处置

**1 个来源文件的首段，472 行。**

| 现路径 | 处置 | 目标位置 |
|---|---|---|
| `internal/panel/settings_panel.go:1–472` | 重写 | `internal/console/api` 与 `web/` 的会话、选群、队列和放行通路 |

#### 必须保住的行为

- 取得会话后只可选择已授权的群，等待队列和放行操作始终带目标群边界。
- 放行仍调用阶段三的同一 `verification.Service`；写入前现查管理员，失败方向为拒绝。
- 浏览器不继承 Telegram callback、ForceReply、64 字节 payload 或 `telego.InlineKeyboardMarkup` 协议。

#### 依赖

依赖：阶段五的接口契约、Mini App 与运维一次性链接身份校验和写入前授权已冻结，阶段三的队列读取与结算接口已可调用。


### 阶段七 · 其余各屏

**分支** 按屏分片，一屏一支一个 PR：`v5/screen-<屏名>`。

设计文档的「各屏职责」列了 15 个屏。阶段六完成其中 2 个：等待队列和群与频道；
阶段七完成偏好、操作记录、管理与处罚、验证方式、免验证来源、题库、
订阅推送、统计、消息与文案、功能、诊断和首页；阶段九随后完成版本屏，现还剩 0 个。
版本屏归阶段九，因为当前版本、可用新版、回退条件和升级入口均依赖该阶段的部署机制。
阶段六还建立了进入屏，但该屏不在上述 15 个屏中：
表中的首页显示概况、需要注意和趋势；进入屏则显示无法进入控制台时的状态。
`scripts/check-docs.py` 根据设计文档的表和 `web/src/app/App.tsx` 路由计算数量，
并拒绝手工记录与计算结果不一致。
阶段七沿用阶段六的按屏分片方式，各屏完成后依据 `web/design.html` 核对取值。

**范围限制**：阶段七没有在屏实现中直接修改接口契约；
所需契约变更通过独立分支完成。

等待队列与操作记录使用不同端点：`GET .../queue` 仅返回等待中的记录，
已结算记录来自 `GET .../audit`，撤销通过 `POST .../audit/{aid}/undo` 完成。
阶段六建立等待队列屏时使用测试数据，实际端点由阶段七实现。
撤销属于操作记录屏，而非等待队列屏。因此，阶段七在操作记录屏实现撤销，
同时删除队列屏测试数据中 `banned` 行的撤销按钮，
并解除 `scripts/check-phase-seams.py` 中对应的阶段接缝限制。

#### 实施顺序：设置端点先于八个屏

阶段七实施前逐屏核对 13 个屏的数据来源，由此确定设置端点必须优先完成。
核对时，控制台共有七条路由，均不支持设置读写
（`/livez` `/readyz` `GET/POST /api/session` `/enter/` `/api/chats` `/api/chats/`）。
当前顶层分发已包含 `GET · POST /setup/{token}`、`GET /api/process/settings`、
`GET /api/status`、`GET /api/status/release` 和 `POST /api/status/upgrade`
（`internal/console/api/server.go:136-184`）；`/api/chats/` 也已按群展开为
`queue`、`audit`、`stats`、`settings`、`rules` 五组
（`internal/console/api/server.go:285-310`）。
阶段七已经完成八个屏所依赖的设置端点。

底层读写能力来自 `internal/settings/store.go:339` 的 `Settings(chatID)` 和
`:389` 的 `Update(groupID, expectedRevision, next)`。实现以按群授权、
写入前实时检查管理员和可区分的 revision 冲突为不变量，
再根据 `Store` 的调用面确定接口契约并写回文档。

其余 5 屏各自的情况：

| 屏 | 它要的数据在哪 |
|---|---|
| 操作记录 | 判定的来龙去脉在 `challenge` 的 `state`/`reason`/`settled_at`/`settled_by`；**「谁改了设置」没有任何表**，那半需要一张新表，归引入设置写入的那一阶段 |
| 统计 | 数据在 `challenge` 表里，缺的是聚合端点；时区已有 `process.stats_timezone` |
| 首页 | 概况与趋势是上面几屏的聚合，最后做，否则要为它单独造一遍 |
| 诊断 | `/livez`、`/readyz` 与只供运维读取的 `GET /api/status` 已有；后者返回 health、设置持久化状态（含上次错误）和成功 Bot API 探测的心跳及延迟。权限预检和查询缓存还没有状态；`private_query_per_min` 是按群设置，不进实例端点 |
| 版本 | **归阶段九，不归本阶段。** 现已通过 `GET /api/status` 显示当前版本和宿主替换状态；运维明确操作后，`GET /api/status/release` 才查询固定 GitHub 仓库的最新正式发布、变更说明和结构清单。屏已建成，所以「还剩 0 个」与路由计数一致 |

#### 进程级设置与按群设置的边界

设置端点落地之后，把剩下 9 屏要的字段对着
`internal/console/api/settings.go` 的响应体又数了一遍。结论：

| 屏 | 字段 |
|---|---|
| 免验证来源、题库、消息与文案、功能、诊断 | 全部在按群的设置里，可以直接做 |
| **订阅推送** | `feeds`、`news_url`、`overlays` 一个都不在 |
| **统计** | `stats_timezone` 不在 |

缺的这四个不属于群，属于进程层：`internal/settings/defaults.yaml` 把它们放在
`process` 与 `resources` 两节，而按群的那对端点承载的是 `GroupOverrides`。
这些字段属于进程级设置，而非按群设置。
订阅推送和统计因此使用单独的进程级设置接口；
接口形状根据实际调用点确定。

消息与文案屏还需要规则表接口。该屏负责五项内容，
其中显示名剧透、自动撤回（含延时）和富文本位于设置中；
自动回复规则位于 `rule` 表
（`migrations/00-latest.sql:58-65`：`id`、`chat_id`、`collection`、`ordinal`、
`enabled`、`definition`），进出提示与验证全流程文案也不属于设置字段。
因此，该屏使用规则表读写接口，而非扩展设置端点。

该分片同时涉及存储层，范围大于复用 `Store.Settings` 和 `Store.Update` 的设置端点分片。

顺带澄清一处容易读混的说法：本文件后面写「三种问答共用 `rule`」，
说的是 `challenge.kind` 这个判定机制的名字，不是这张表。
题库屏编辑的 `questions` 与 `fallback_questions` 是设置字段，
验证时也确实读它们（`internal/verification/kernel.go:59`、`91`、`148`），
和 `rule` 表没有关系。

**统计屏还另外缺聚合**：数据在 `challenge` 表里，
但没有任何端点把「入群结果趋势、通过率、各验证方式拦截量」算出来。
那是它自己的一片，不是设置层的事。

**文案与语言在这一阶段一起做完，不留到最后。**
每加一屏，三种语言的词条同时补齐，不先写中文再统一翻译。
留到最后会变成一次几百条的批量翻译，那种翻译没人逐条核对。

- 界面上出现的每一个字都来自词条表，代码里不留字面量
- 源语言词条缺失是编译错误；译文允许不全，缺的当场回落到源语言
- 每条译文的占位符集合必须与源文一致。这一项最容易漏，
  漏了会把 `{{name}}` 原样显示给用户，而词条数量对得上，看不出来
- 带数量的句子走平台的复数机制，复数分类按语言核对

**验收结果**：每条路由均在两个宽度、三个主题和实测最宽语言下渲染；
页面没有横向溢出或未替换占位符，各主题取值有效，可聚焦控件具有焦点环。

该验收由 `web/e2e/render-gate.spec.ts` 自动执行，
新增屏会自动进入断言范围。全部语言与全部屏的定时渲染检查归阶段十一。
#### 文件处置

**2 个文件或来源段，1,193 行。**

| 现路径 | 处置 | 目标位置 |
|---|---|---|
| `internal/i18n/panel.go` | 拆分 | Telegram 命令文案保留在 `internal/i18n`；控制台文案迁至 `web/` 语言资源 |
| `internal/panel/settings_panel.go:473–1381` | 重写 | `internal/console/api` 与 `web/` 的其余设置、规则、频道、反垃圾和统计屏 |

#### 必须保住的行为

- 各屏继续只读写目标群，并展示设置来源、恢复默认、整份校验、确认删除和 revision 冲突。
- 题库、频道、白名单、反垃圾和诊断屏复用阶段二、三、四的领域结果，不在前端重写规则或状态转换。
- 控制台文案继续由语言资源提供；不恢复旧回调编码、内联键盘或 ForceReply 交互。

#### 依赖

依赖：阶段六的最小通路已验证，阶段五的接口契约保持不变；阶段七所需契约变更已通过独立分支完成。


### 阶段八 · 多租户

**分支** `v5/multitenant`

配置模型本身在阶段四就已是「出厂默认加每群取值」，读取路径统一走
`Store.Settings(chatID)`。**这一阶段不再重做那件事**，做的是它之外仍然全局的部分：
代码里对某个特定群的分支、以及按 edition 决定功能的那一套。

这一阶段把 Gentoo 与 Linux 相关的查询和订阅拆成可关闭的模块：
命令表由模块声明而不是写死在分发处，关掉的模块既不注册命令也不出现在帮助里。
这两块合起来接近一万行，是「按群隔离」之外最大的一处全局假设。

**验收（已达成）**：把我们社区的群配置全部删除，机器人与控制台照常工作；
关掉全部可选模块后二进制照常启动，命令表里不残留它们的条目；
`grep -rn '主群\|mainGroup\|isMainChat' internal cmd` 无业务分支。

另一个同类问题不在上述 grep 的范围内：**测试曾包含真实生产群 ID**。
`config.example.json` 使用虚构 ID，但若干测试文件曾直接使用五个生产群 ID，
其中一处还暴露群之间的信任关系。群 ID 不是凭据，但测试断言不依赖具体值，
因此公开仓库不应保留生产拓扑。

阶段八实施前重新统计了这五个完整 ID，三个目标测试文件合计 21 处，
并非旧快照记录的 43 处。Go 测试和 `testdata/` 中的群 ID 已迁至 `-1009` 虚构号段，
`scripts/check-test-chat-ids.py` 会拒绝号段外 ID、缺失的扫描目标和零覆盖。
`internal/feed/state_compat_test.go` 与 `internal/settings/defaults.yaml` 中原有的虚构 ID 未修改。

上述 grep 仍无结果，因为前序阶段迁移时已经删除对应业务分支；该命令作为回归检查保留。
本阶段处理了原来由构建标签二选一的两个 `internal/edition` 文件及其九处读取：

| 原读取位置 | 处置结果 |
|---|---|
| telegram 包的 edition 命令前缀适配层 | 删除适配层；`internal/telegram/commands.go` 只声明无前缀命令 |
| `internal/lookup/packages.go` 的 `Name` 读取 | 保留单一 `edition.Name`，继续作为 User-Agent |
| `internal/lookup/packages.go` 的 `CommandPrefix` 读取 | 提示文本固定使用 `/use` |
| `internal/verification/kernel.go` 的 `KernelExampleSuffix` 读取 | 占位示例固定为通用的 `X.Y.Z` |
| `internal/i18n/catalog.go` 的 `CommandPrefix` 读取 | 删除运行时替换；三语目录直接保存无前缀命令 |
| `internal/i18n/catalog.go` 的 `KernelExampleSuffix` 读取 | 删除运行时替换；三语目录直接保存通用示例 |
| `internal/i18n/bot.go` 的 `IsGentoo` 读取 | 删除版本分支，只保留中性的 `Identity` 文案 |
| verification 文案的 `IsGentoo` 读取 | 删除版本分支；fallback 题迁入 `internal/rules/provisioning/fallback_questions.json` |
| `cmd/bot/main.go` 的 `Name` 读取 | 保留单一 `edition.Name`，继续拼接默认配置路径 |

`gentoo` 构建标签现在只用于兼容性回归，不再选择源码或产品行为。
**实机验收豁免**：将测试群和另一个临时群同时连接到同一进程，
验证双方配置互不影响。自动化测试已覆盖前三项；该项需要使用生产机器人账号，
不在无人值守操作的授权范围内。

#### 文件处置

**12 个文件，4,600 行。** 前序阶段已将这些来源迁入目标包；这里的行数表示移除全局模型时复查的接触面。

| 现路径 | 处置 | 目标位置 |
|---|---|---|
| `internal/bot/{commands,dm}.go` | 复查 | `internal/telegram/updates.go`、`internal/telegram/tgfmt`、`internal/settings` |
| `internal/i18n/{bot,catalog,verification}.go` | 复查 | `internal/i18n`、`internal/settings`、出厂 rules/provisioning |
| `cmd/vestibule/registration.go` | 复查 | `internal/telegram/updates.go`、`internal/database`、`internal/settings` |
| `internal/store/{baseline,settings}.go` | 复查 | `internal/settings`、`internal/database` |
| `internal/config/config.go` | 复查 | `internal/settings` |
| `internal/edition/{edition_gentoo,edition_generic}.go` | 重写 | 单一的 `internal/edition` |
| `internal/bot/edition.go` | 已删除 | 无；不再由 edition 决定群功能。前序阶段已完成，此行是回归项 |

#### 必须保住的行为

- bot 加入、移出、标题更新和同群串行保持幂等；加入后即创建该群可用的状态，移出后保留待清理标记。
- 每群只继承出厂默认或本群 override，配置、队列、授权和规则互不串群；删除社区配置后产品仍可运行。
- 每次敏感写入仍现查管理员；不保留全局 owner、enrollment、未知群延迟离开或 build edition 的特权路径。
- 命令菜单保留语言 scope 与运行时群更新；私聊命令不被自动回复吞掉；版本元数据保留，社区身份、命令前缀和 edition fallback 题被移除。
- typed catalogue、占位符和三语一致性保持；内置 fallback 题改为出厂 rules/provisioning，而非按 build tag 选择。

**配置模型已于阶段四完成切换。** 因此，这 12 个来源在阶段八仅需复查，
确认移除全局默认后，不再有代码假设某个特定群存在。

#### 依赖

依赖：阶段三的群记录、阶段四的设置模型与阶段五的按群授权均已完成。
### 阶段九 · 安装与更新

**分支** `v5/deploy`

三个组件一次部署：应用、数据库、自建 Bot API。

**容器是默认部署方式。** 原生服务保留给未安装容器运行时的环境。
两条路径共用安装、升级、回退、卸载、状态和宿主替换单元。

**「无需凭据的一条命令启动三个组件」不成立。** 上游 Telegram Bot API 要求
`TELEGRAM_API_ID` 与 `TELEGRAM_API_HASH`。安装器在容器路径开始前检查
`/etc/vestibule/bot-api.env`；缺少任一变量即失败，不生成占位值，也不从浏览器认领流程读取。
当前没有获批准的凭据配置路径，因此不能将该前提表述为无需配置即可部署。
应用的 `BOT_TOKEN` 仍仅通过浏览器认领写入 `bot.env`，安装器不读取该值。

安装器按 `curl` 或 `wget`、`systemctl`、容器运行时与机器架构等能力选择路径，不按发行版名称分支。
重跑同一部署方式是升级；失败事务只恢复本次变更的文件与单元，不删除已有 `bot.env`、配置、状态或无关文件。
容器路径为首次安装生成 PostgreSQL 凭据与 `container.env`，升级保留它们。原生路径保留
`DynamicUser` 服务及原有五项生命周期。

**原生安装器保留升级下载前的回退预检。** 脚本先获取 `SHA256SUMS` 与
`vestibule-schema-manifest`，校验清单后，再把目标版本的
`minimum_rollback_schema_version` 与本机保存的当前
`target_schema_version` 比较。结构不兼容或缺少当前清单时，脚本在请求二进制之前退出；
结构版本相同，或者当前版本不低于目标版本声明的回退下限时，才继续下载。

`migrations.AssessRollback` 与 `FetchAfterRollbackCheck` 仍供 Go 代码使用，shell 脚本不调用它们。
发布侧由 `cmd/schema-manifest` 生成两项结构元数据；版本库中的
`deploy/vestibule-schema-manifest` 仍是受检副本。发布流程把原生单元、安装器拆分的运行时文件、
`compose.yaml` 与宿主替换单元作为发布资产，全部列进 `SHA256SUMS`。
安装成功后，脚本把已校验的结构清单与版本号保存在 `/etc/vestibule/`，
供下一次原生升级在下载二进制前判断。

**发布来源真实性仍有明确缺口。** `SHA256SUMS` 与被校验资产来自同一个 GitHub release，
当前发布流程没有提供独立签名。校验只能证明资产与摘要一致，不能证明发布者身份；
脚本会输出警告，并在结果文件写入
`checksum_authenticity=unverified_same_release`，不会将校验成功表述为来源可信。
独立签名或其他可信摘要来源属于阶段九验收豁免后的后续工作。

**应用只写升级意图，宿主单元执行替换。** `POST /api/status/upgrade` 只接受运维角色和
CSRF 令牌。应用将目标版本原子写入数据目录的 `replacement-request`；文件只有一行版本号，
字符集限于 `[A-Za-z0-9._-]`，不接受 URL、镜像地址、键值对或额外行。请求中的版本由运维选择，
原生路径的下载地址和容器路径的 `ghcr.io/zakkaus/vestibule:<version>` 均由宿主执行器固定决定。
因此能写数据目录的主体不能把升级来源改成任意地址。

`deploy/vestibule-replace.path` 只监视该请求文件，触发
`deploy/vestibule-replace.service`。服务以 root 运行
`/usr/local/libexec/vestibule-replace`：原生部署调用已校验的安装器，容器部署更新固定应用镜像标签，
由宿主调用 Docker Compose。应用容器没有 Docker 运行时凭据或 `docker.sock`；`compose.yaml` 的
应用、数据库和 Bot API 三个服务也不会把该 socket 挂入应用。

执行器完成替换后依次探测 `/livez` 与 `/readyz`。两者通过时向数据目录写入
`replacement-result.env` 的 `status=applied` 与 `reason=complete`。任一探针失败时自动恢复上一版，
再次探测；成功恢复写入 `status=rolled_back` 与 `reason=healthcheck_failed`，恢复也失败则写入
`status=rollback_failed`。结果文件为 `0600`，所以结果和失败原因在应用重启后仍可读取。

安装器在数据目录写入 `replacement-unit.env` 的 `available=yes`，仅在替换单元已经安装时写入。
应用通过状态服务读取此事实及最后的宿主结果；`GET /api/status` 同时返回构建注入的当前版本和
`replacement`。版本屏只向运维显示，群管理员既看不到导航，也会被后端拒绝读取状态、查询发布或发起升级。

**发布查询按需执行，不在页面打开时对外请求。** 运维选择「检查更新」后，
`GET /api/status/release` 才查询固定仓库 `Zakkaus/vestibule` 的 GitHub 最新正式发布，
并读取该标签下固定名称的 `vestibule-schema-manifest`。查询失败只让「可用新版」区块进入可重试状态；
当前版本、宿主单元状态和上次替换结果继续显示。这样既不把打开本地状态页变成外部依赖，
也不会把「发布源不可用」误报成「已是最新」。

目标发布的结构清单由目标版本自己提供。当前二进制内嵌的迁移历史无法预知未来版本的
`minimum_rollback_schema_version`，所以不能只调用当前版本的 `migrations.AssessRollback` 推断未来发布。
应用严格解析下载到的两项清单，再以当前保留版本的 schema 计算结构化回退结果。
不兼容时，界面会同时显示目标 schema、最低回退 schema 和当前保留版本支持的 schema，不只显示布尔值。

宿主单元存在且回退预检通过时，升级入口先要求确认，再沿现有 CSRF 传输写入版本意图；
页面在应用重启期间重试本地状态端点，并显示最终宿主结果。宿主单元不存在时完全不显示升级按钮，
改为说明应用不能替换宿主进程或容器，并给出 compose 镜像设置和宿主命令。判断只看
`unit_available`，不猜部署方式。

首次安装仍通过临时 `setup.env` 接收原始认领值，进程只把哈希写进其自己创建的
`StateDirectory`。`systemctl restart` 成功后，安装器删除临时文件。完整认领链接写入
`/etc/vestibule/install-result.env`，权限为 `600`；它包含操作、版本、部署方式、认领链接、
回退可用性与摘要校验状态，不写机器人令牌或其他凭据。

无特权测试把目标路径放入 `VESTIBULE_ROOT`，以 `VESTIBULE_FETCH`、`VESTIBULE_SYSTEMCTL` 和
`VESTIBULE_DOCKER` 替换外部依赖。`scripts/test-install.sh` 在临时根执行原生与容器的安装、
升级、状态、回退和卸载。`scripts/test-replacement.sh` 用假安装器、Docker Compose、健康探针执行
两种部署的同一宿主替换机制，并驱动 URL 请求、Docker socket 注入、探针失败自动回退和回退失败的反例。

**验收结果**：三个组件的凭据前置条件使第一项仍为 `EXEMPT`；
脚本执行 `/livez` 与 `/readyz` 探针，验证失败替换自动回退并保留结果，
在结构不兼容时于下载前阻止升级，并检查版本、发布查询、运维权限和回退原因的 Go 契约。
干净机器、域名、证书、浏览器和 Bot API 凭据配置路径仍为 `EXEMPT`。
`scripts/accept-phase9.sh` 按七项顺序输出：第一项和第七项保留真实环境豁免，中间五项实际执行。
浏览器门禁另行覆盖群管理员隐藏、无宿主单元时无升级按钮、回退原因、确认升级、重连读取结果和查询失败重试。

#### 文件处置

| 现路径 | 处置 | 目标位置 |
|---|---|---|
| `deploy/install.sh` | 已重写为部署调度入口 | 同名。默认容器，`--native` 选择原生；安装、升级、回退、卸载与状态共用入口 |
| `deploy/install-common.sh`、`deploy/install-native.sh`、`deploy/install-container.sh` | 新增 | 安装器的已校验运行时文件；分别承载事务与发布资产、原生生命周期、容器生命周期 |
| `deploy/vestibule-replace`、`.service`、`.path` | 新增 | 同名。宿主监视版本请求，执行两条部署路径并写回结果 |
| `internal/status/release.go`、`internal/console/api/release.go` | 新增 | 按需读取固定发布源及目标结构清单，并以运维权限返回结构化回退结果 |
| `web/src/features/version/` | 新增 | 最后一张控制台屏；显示当前版本、发布说明、回退原因、宿主结果及升级或手动命令 |
| `deploy/Dockerfile`、`Dockerfile.bot-api`、`compose.yaml` | 新增 | `deploy/` 下的容器文件。构建应用与固定 Bot API 提交的镜像，并定义不挂载 Docker socket 的三个组件 |
| `deploy/vestibule.service` | 沿用 | 同名。保留全部既有加固项，作为受摘要校验的发布资产 |
| `deploy/gentoo-zhbot.service` | 已删除 | 无。阶段八移除构建标签选择后，`--generic` 与 `--gentoo` 不再选择版本，第二个单元没有安装目标 |

**阶段九验收豁免的后续工作**：为 Telegram Bot API 确定获批准的 `TELEGRAM_API_ID` 与 `TELEGRAM_API_HASH` 配置路径，
在干净机器上验证域名、证书和浏览器路径，并为发布摘要提供独立认证。

#### 必须保住的行为

以下行为必须保住：

- 二进制、结构清单、systemd 单元、安装器运行时文件、替换执行器与 `compose.yaml` 均按发布的
  `SHA256SUMS` 校验后才安装。
- **不覆盖已有的 `bot.env` 与 `bot-api.env`**，因此升级不会替换应用令牌或 Bot API 凭据。
- ~~两个版本名字全程分开，一台机器上可以同时装（`--generic` 与 `--gentoo`）~~
  **这一条被阶段八取代。** 版本由构建标签二选一的机制没了，一份二进制服务所有社区；
  「一台机器上装两份」原本是为了同时运行 Gentoo-zh 与通用两套，而多租户让一个实例
  就能服务多个群。安装脚本的 `--generic` / `--gentoo` 与第二份单元一并删除，
  `internal/edition/deploy_test.go` 改为断言单元、安装脚本与二进制用同一个产品名。
- 单元的加固逐条保留：`DynamicUser`、`ProtectSystem=strict`、
  `CapabilityBoundingSet=` 空、`SystemCallFilter=@system-service`、
  `RestrictAddressFamilies` 只留三种、`UMask=0077`、`StateDirectoryMode=0700`。
- `Type=notify` 与 `WatchdogSec=120s` 要求进程主动报告状态，systemd 负责处理无响应进程。

#### 依赖

依赖阶段五提供的 `/livez` 与 `/readyz`。`Live` 仅读取原子标志；
`Ready` 要求配置校验完成、Telegram 通道建立，并在探测时确认数据库可用。
替换执行器依次探测这两个端点。

自建 Bot API 已接入容器部署：应用使用 `TELEGRAM_API_URL=http://bot-api:8081`，
Bot API 容器从独立的 `bot-api.env` 读取上游必需凭据。应用代码继续把
`TelegramAPIURL` 传给 `telego.WithAPIServer`。


### 阶段十 · 切换到生产

上一代仍在几个真实的群里运行，最后一步是把它们换过来。
这一步风险最高，因为它不是写代码，是拿正在服务几百人的群做替换。
所以它单独成一个阶段，而不是发布日当天临场决定。

**先迁数据，再谈切换。** 上一代的状态在几份 JSON 文件里，
迁移写成一条只读旧文件、只写新库的单向命令，可以重复执行，
第二次执行不产生重复记录。

上一代的生产代码持久化**七份** JSON（`agents`、`antispam`、`heartbeat`、`pending`、
`settings`、`verifyfail`、`warns`；其余八份只出现在它的测试里）。这七份分两条路过来，
**不是一条命令包办**，下表按实际的路写：

| 迁移内容 | 路径 | 原因 |
|---|---|---|
| 警告计数（`warns.json`） | `cmd/import-state` | 丢了等于给被警告的人清零重来 |
| 验证失败计数（`verifyfail.json`） | `cmd/import-state` | 丢了会让冷却与自动封禁从头计 |
| 自动化代理计数（`agents.json`） | `cmd/import-state` | 反垃圾的历史依据 |
| 心跳（`heartbeat.json`） | `cmd/import-state` | 决定恢复后要不要重发挑战 |
| 每群设置与文案、题库、自动回复（`settings.json`） | **设置层直接读**（`internal/settings/store.go:305` 的对账） | 丢了会回到出厂默认，群管理员未必立刻发现；是手写内容，无法重建 |
| 反垃圾旧格式（`antispam.json`） | **设置层直接读**（`internal/settings/store_init.go:115`） | 上一代单独存了一份，不迁会静默回到默认 |
| 进行中的待验证记录（`pending.json`） | `cmd/import-state -pending carry\|drop` | 是否迁移由维护者在切换前决定 |

`cmd/import-state` 要求显式提供 `-pending carry` 或 `-pending drop`；
缺少该参数时，命令会在备份和写入前拒绝执行。`carry` 会将旧机器人仍可能结算的记录写入新库；
`drop` 则将这些记录留给仍在响应的旧机器人。命令不提供默认选项，
最终选择由维护者在阶段十执行前确认。

原表还列有「已通过验证的名单」，并假定丢失该名单会要求已有成员重新验证。
上一代不持久化此类名单：七份文件中没有该数据，成员身份由 Telegram 保存，
且验证仅对入群申请触发。因此，该行已删除。

**迁移要在旧机器人停下之后做，而这一条由命令自己拦着。** 导入会把它管的每张表
整批删除重建，所以对着一个正在被轮询的库执行，会用上一代的快照替掉此刻进行中的验证。
判据是 `update_poll_lease` 里那条未过期的租约：有持有者就拒绝并把持有者名字打出来，
过期的持有者算停机、不拦。对着冷库重复执行不受影响，那正是本阶段验收要的那一条。

迁完先核对：两边各自数一遍上表每一类的条数，数目不一致就停下来查，不凭印象判断。

**用新令牌并行运行，不复用旧令牌。**
复用旧令牌的切换是原子的，但退回去要重新配置，而且中间那段两边都不在线。
新令牌的代价是群里短期有两个机器人，换来的是任何时刻都能退回：
旧的一直在，出问题就把新的移出群。
并行期间新机器人只记录判断、不发出任何动作，确认它判断得和旧的一致，再让它接手。

**只观察是一个运行模式，不是「先不给权限」。已经写好了。**
Bot API 规定，机器人必须持有 `can_invite_users` 管理员权限才会收到
`chat_join_request`（`telego@v1.11.1/types.go:108`）。
因为收得到和做得了是同一个权限，所以不给权限，它一条申请都收不到，无从比较；
给了权限，approve 与 decline 就都在它手上。只观察只能由代码保证。

它由 `internal/verification/observe_only.go:68` 的 `ApplyObservationMode` 装在网关外面：
读全部转给真网关，**每一次对外写入都换成一条落库的观察**，合成的消息号是负数，
不可能指向真实的 Telegram 消息。
开关是配置里的 `observe_only`（`internal/settings/config.go:255`），
由 `internal/app/app.go:202` 把它接上 `ObserveOnly`。
观察写不进去时它**不报成功**，否则「没发出去」和「发了但没记下」会长得一样。

这个模式拦下的是全部对外写入，不只是 approve 与 decline。
两个机器人收到同一条申请，如果新的照常发验证消息，申请人会收到两份题目，
观察本身就改变了被观察的对象。所以新机器人只把它会做的判断写进自己的库，
一个 Telegram 调用都不发；验收里「并行期两边判断一致」比的是这批记录与旧机器人的日志。

**退回的条件写死在这里，不到当天再判断。**
出现下面任意一条就退回，不讨论：

- 误拒真实用户，一天内两起。
  读数：`GET /api/status` 的 `rollback_observations.rejections`。`window_seconds` 固定为
  `86400`，`by_reason` 按 `reason` 汇总最近窗口内 `declined` 与 `banned` 的结算数。
  `human_review_required` 始终为 `true`：这些是核查原料，不自动认定误拒。
- 验证消息发送失败或重复发送，持续超过十分钟。
  读数：`rollback_observations.challenge_delivery`。`streak.problem_span_seconds` 是当前未被成功
  发送打断的首末问题事件间隔；只有超过 `600` 秒时 `streak.exceeds_threshold` 才为 `true`。
  `failed_deliveries` 与 `duplicate_deliveries` 分别是这个连续段里的发送失败和重复发送数；
  单次失败的间隔为 `0`，不触发。
- 控制台换不到会话，且十分钟内没有恢复。
  读数：`rollback_observations.console_access`。它记录群访问验证返回
  `ErrAccessUnavailable` 的连续段；任一成功验证会清零。读 `streak.problem_span_seconds` 与
  `streak.exceeds_threshold`，同样以超过 `600` 秒为界。
  该读数原本仅由 `AuthorizeChat` 记录群访问验证失败，
  未覆盖会话换取整体失败。当前由 `redemptionUnavailable`
  （`internal/console/auth/manager.go:252`）记录应签发但未签发会话的两种情况：
  会话表已满或无法获得凭据熵；成功换取会话时记录恢复。
  链接无效、过期或已兑换不计入该指标，避免重复访问旧链接触发回退。
- 数据库写入失败率超过百分之一。
  读数：`rollback_observations.database_writes`。`scope` 是 `retry_store_write`，
  `window_seconds` 固定为 `600`；`total_writes` 是窗口内完成的逻辑写入数，
  `failed_writes` 是三次重试都失败的写入数，`failure_rate_percent` 是两者的百分比，
  `exceeds_one_percent` 表示严格超过百分之一。

退回的动作是：把新机器人的权限收回，旧机器人的权限恢复，
然后写清是哪一条触发的。旧机器人在整个观察期内不停机，
所以退回不需要重新部署。

**观察期至少七天，且要跨过一个周末。**
垃圾注册的流量在周末和工作日不是一回事，只看工作日会得出错误的结论。

**验收**：迁移命令重复执行结果一致；并行期两边判断一致；
按上面任一条件演练过一次退回，退回后旧机器人照常工作；
观察期满且期间没有触发退回条件。

**不做**：不在观察期内改新机器人的规则。观察的是它现在这套，
边观察边改等于没有观察。


## 2. 扫描结果的处置

2026-08-31 全仓库扫描共 81 条。分类与处置：

| 类别 | 条数 | 处置 |
|---|---|---|
| 拆分方案 | 12 | 采纳，作为阶段一与阶段二的依据 |
| 通用化 | 13 | 采纳，阶段六落实；其中最小可用集 5 条为阶段六验收项 |
| 代码缺陷 | 22 | 高与中优先在对应阶段顺带修复，低优先另开分支 |
| 文档漂移 | 26 | 阶段五之后统一修订，因为届时文档结构会变 |
| 部署与流程 | 8 | 阶段五与发布准备阶段处理 |

**明确驳回的**：无。全部条目要么采纳，要么排入后续，理由记在对应 issue。

### 阶段十一 · 维护者指定的功能

**分支** `v5/asked-for`

2026-09-02，维护者处理了积压的待决项。以下功能不属于 v5 重写范围；
重写完成上一代能力和控制台，这些新增功能排在生产切换之后。
如需提前实施，应单独调整阶段顺序。

**主题。** 默认使用 Graphite，另支持 Catppuccin Frappé、Catppuccin Macchiato、
Catppuccin Mocha、Tokyo Night Storm 和 Tokyo Night。
六套配色取代偏好屏原定的强调色选择，并通过既有对比度与主题泄漏检查。

**新增日语和俄语。** 语言目录由三份增加到五份。
俄语复数类别不止 `one` 和 `other`，现有复数键需按语言补齐类别。

**渲染门禁覆盖所有语言，但分两档执行。** 维护者原话是「可以都试试看但是不用太频繁」。
每个 PR 测试实测最宽语言；定时任务测试全部语言与全部屏。
该决定解决了「逐一测试全部语言或仅测试最宽语言」的待决项。

**每个群仅绑定一个拥有者。** `ClaimOwner`
（`internal/settings/store_registration.go:96`）仅在尚无拥有者时接受认领，
并拒绝第二个人使用同一领取码。阶段十一还需在接口中返回拥有者，
并在群与频道屏中支持显示和改绑。

**实现控制群。** 该功能属于管理与处罚屏。

**实现试答。** 该功能属于题库屏，对应 `POST /api/chats/{id}/rules/test`，
并调用生产路径使用的同一判定代码。该项曾误列为阶段三必须保留的行为；
上一代没有 HTTP 服务，因此它是新增功能。

**支持每日推送设备状态，并允许关闭。** 诊断数据已经存在，阶段十一增加投递和开关。

**增加结构信号。** 架构文档定义的消息实体计分在两代代码中均不存在，
维护者已将其归入阶段十一。

**不在本阶段实现**题库的内置模板和偏好屏的标题图标。
维护者尚未指定这两项；后续需要单独排期或从设计文档移除。

**验收要求**：六套配色分别在两个宽度下渲染，无溢出且对比度达标；
五种语言的目录键集和占位符一致，俄语复数类别完整；
定时任务覆盖全部语言与全部屏；每个群仅能绑定一个拥有者；
控制群、试答和日报开关分别具有端到端用例；
结构信号具有样本和断言，关闭后不影响其他反垃圾规则。

#### 文件处置

| 现路径 | 处置 | 目标位置 |
|---|---|---|
| `web/src/styles/tokens.css` | 不改 | 它是 vendored 副本。配色作为新增层加在 `web/src/app/app.css`，不动来源 |
| `web/src/features/preferences` | 扩写 | 原地。主题选择取代原先设想的强调色 |
| `web/src/i18n/locales` | 增两份 | 同名目录，加 `ja.json`、`ru.json` |
| `web/e2e/render-gate.spec.ts` | 扩写 | 原地，按频次拆成两档 |
| `internal/rules` | 扩写 | 原地，结构信号是纯函数 |

#### 必须保住的行为

- 六套配色都要满足既有的主题不泄漏与对比度检查，默认那一套的观感不因为多了五套而改变。
- 五种语言下现有的十五屏不出现截断、横向滚动、按钮被挤出容器。
- 拥有者绑定不改变现有授权：拥有者是一个人，不是一种绕过权限检查的身份。
- 结构信号是加分项不是替代项，关掉它，现有反垃圾规则的判定一个字不变。

#### 依赖

依赖阶段十完成：这些是加法，加在一个已经与上一代等价并且切过去的系统上。
主题与语言只依赖阶段七的十五屏，可以更早做；其余几条都要先有控制台的写入路径。

### 已在对应阶段修复的高优先项

| 条目 | 阶段 |
|---|---|
| 进群后禁言的归属记录不耐崩溃，且与待验证记录不在同一事务 | 三 |
| `recentlyPassed` 在进程重启后丢失 | 三 |
| 群内挑战可见后才写入完整待验证状态 | 三 |
| 自动删除可保存为开启而延时为 0 | 五（保存时整份校验） |
| 处罚目标的管理员状态使用缓存 | 五（授权改为现查） |
| Gentoo 版构建命令实际生成通用版 | 一（构建脚本，顺带） |

## 3. 发布

- 阶段零到十全部合入 `main` 并稳定运行后，一次发布 v5.0.0。阶段十一是切换之后的加法，不拦发布。
  发布要人点头，依据同样是 `CONTRIBUTING.md`「What may happen without asking」。

**什么时候算可以替换生产。** 下面每一条都能当场验证，
不满足就不发，不用「差不多了」这种判断：

| 条件 | 怎么验 |
|---|---|
| 阶段零到十各自的验收都过了 | 逐条对，不补记 |
| 门禁在两个构建标签下都是绿的 | 完整跑一遍，不看缓存 |
| 一条端到端用例在真浏览器里通过 | 从换取会话走到放行成功 |
| 三种语言各跑过一遍全部屏 | 最长的那种语言下无截断、无横向滚动 |
| 迁移命令在生产数据的副本上跑通 | 条数逐类核对一致 |
| 演练过一次退回 | 退回后旧机器人照常工作 |
| 观察期满七天且跨过周末 | 期间未触发任何退回条件 |
- **不在过程中连续打 tag。** 阶段合入只进 `main`，不发版本。
- 发布前：`CHANGELOG` 与 tag 一致性校验、最终二进制冒烟测试、
  两个构建标签的完整门禁。

## 4. 待决

已定的不再列在这里：仓库是公开的 `vestibule`，放个人账号；控制台暂用这个名字。

### 要维护者定

每项均注明必须作出决定的阶段，避免已完成阶段仍保留未处理的决策。

| 问题 | 决策阶段 | 影响 |
|---|---|---|
| 控制台域名 | 实机部署前 | 代码已通过 `CONSOLE_URL` 支持配置；留空时不投递链接，Mini App 登录接口仍可使用。域名仅阻塞证书签发和 Mini App 实机配置 |
| 从配置删除群时是否保留进行中的验证 | 阶段十 | 实测中，删除群配置并重启会将该群的 `pending` challenge 标记为 `superseded`；数据库记录仍保留且不跨群可见，但重新添加群后不会恢复进行中的队列 |
| ~~申请人应答是否提前到动作之前~~ **已于 2026-09-02 确定：提前** | 已确定 | 阶段三第三片已经使动作和状态转换在同一事务中落库并由执行器重试；文案需表述为「已判定通过」 |
| 是否迁移进行中的待验证记录 | 阶段十 | `cmd/import-state -pending carry\|drop` 要求显式选择并在备份前校验。`carry` 可能使两个机器人处理同一申请；`drop` 将记录留给仍在运行的旧机器人。最终选择由维护者确认 |
| 公开实例使用哪个机器人账号 | 阶段十 | `@GentooZhVerifyBot` 包含社区名称，不适合作为通用账号；更换账号涉及既有群迁移 |

### 未排期与已确定事项

| 问题 | 现状 |
|---|---|
| 控制台域名 | **阶段九已完成，代码不再等它；剩下的只有实机。** 实现把它做成了配置（`CONSOLE_URL`，留空即不投递链接，Mini App 登录接口照常），所以代码不需要答案。需要它的是那台真机：证书要签给某个域名，Mini App 也要填一个。阶段九剩的两条 `EXEMPT` 都卡在这里 |
| 申请人应答顺序 | **已于 2026-09-02 确定提前应答。** 阶段三第三片已具备事务落库和执行器重试前提；文案表述为「已判定通过」，而非「已经进群」 |
| 四屏欠的六条职责 | **四条已定 2026-09-02，归阶段十一**：强调色改为整套主题、拥有者绑定（唯一）、控制群、试答。**两条仍待决**：题库的内置模板、偏好的标题图标 | 实测：把设计文档「各屏职责」表里每屏的特征词拿去比三份语言目录（控制台文案检查保证凡是屏上能看见的字都在目录里），四屏存在缺口：**题库**缺「试答」与「内置模板」，**偏好**缺「强调色」与「标题图标」，**群与频道**缺「拥有者绑定」，**管理与处罚**缺「控制群」。逐个打开确认过不是换了说法：题库屏 18 个文案键里没有任何试答入口，群与频道屏 14 个键里没有拥有者。其中「试答」还被阶段三的「必须保住的行为」写成了要保住的既有行为，而**上一代根本没有 HTTP 服务**（`grep -rln 'http.Server\|ListenAndServe' ~/code/refs/gentoo-zh-verify-bot` 为空），控制台试答是新功能不是遗产 —— 这与清点表里那次「新功能被写进搬移表」是同一个错。要么补做，要么从设计文档里去掉；两种都要维护者点头 |
| 路由表与实现的差异要不要收敛 | 阶段十与阶段十一 | 实测，单位是**表格行**（一行可能写着两个方法）：架构文档第 11 节现在 26 行，18 行已实现、8 行没有。`POST /api/status/upgrade` 已由阶段九的宿主替换机制实现；`GET /verify/{token}` 属阶段十。其余七行上一代都没有，`rules/test` 已归阶段十一。七行是：`GET /api/chats/{id}/overview`、`GET /api/chats/{id}/packages`、`POST /api/chats/{id}/packages`、`GET · PATCH /api/me/preferences`、`GET · PUT /api/chats/{id}/feeds`、`PATCH /api/chats/{id}`、`POST /api/chats/{id}/rules/test`。要么补进某一阶段，要么从路由表里去掉。这份清单由 `scripts/check-console-routes.py` 打印，不是手数的 |
| ~~全部语言或仅最宽语言参与渲染门禁~~ **已于 2026-09-02 确定：分两档执行** | 每个 PR 测试实测最宽语言，定时任务测试全部语言与全部屏；`scripts/check-locale-catalogues.py` 静态检查目录键集和占位符 |
| 结构信号 | **已于 2026-09-02 确定归阶段十一。** 该功能是架构文档新增设计，两代代码均未实现 |

阶段表的状态列用于阻止已完成阶段继续保留未处理的待决项；
`scripts/check-docs.py` 会检查决策阶段与阶段状态是否一致。

### 隐私说明：维护者已确定

**这是个开源项目，公开实例并不固定。** 因此这份说明的主体不是「我们这个服务怎么处理你的数据」，
而是**软件保存的数据**；该内容适用于每份部署，并可依据 schema 核对。
运行某一份拷贝的人是持有数据的那一方，实例专属的部分由他自己填。

落在 `docs/PRIVACY.md` 与 `docs/PRIVACY.zh-CN.md`，两个 README 都指向它。
第 2 节那张表由 `scripts/check-privacy-tables.py` 看着：
schema 里凡是带 `user_id` 或 `chat_id` 的表，两种语言的说明都必须写到，漏一个即报红。
**一份悄悄不完整的隐私说明比没有更糟**，因为人会照着它做判断。

界面上怎么摆是阶段七的事，设计文档已有一条：
保存的是他人群组的数据这一事实要置于显著位置，不能只写在说明的第七段。

### 根据证据确定的两项原待决事项

这两项决定了阶段五和阶段六的接口形状，现已按下列证据确定；
维护者仍可明确修改决定。

| 问题 | 决定 | 依据 |
|---|---|---|
| 控制台的访问入口 | 两个面：群管理员走 Telegram 里的 Mini App，每次请求校验 `initData`；运维走机器人发的一次性链接，在普通浏览器里换成会话。**去掉 Telegram Login 的 OIDC** | OIDC 存在的理由是「在普通浏览器里打开」，但真正需要普通浏览器的是运维，而它在最需要的那一刻依赖 Telegram 可达。一次性链接换会话安装时已经有了。policr-mini 独立走到同一形状：`console_v2/tma_auth.ex` 走 Mini App，`admin_v2/token_auth.ex` 走与 Telegram 无关的令牌 |
| 受众模型 | **两类主体**。运维看这台机器，群管理员看他的群；运维不因为是运维就能读某个群的数据。界面不做简单版与专业版，差别来自服务端授权结果 | 架构书原本就是这么定的，设计书那句「收敛为群管理员」是后写的。policr-mini 同样分成两面 |

代价写在文档里：**群管理员不能在普通浏览器里打开控制台**，桌面版 Telegram 里可以。

第三条，同样按证据定：**「撤销」不属于等待队列屏，它属于操作记录屏。**

设计书「各屏职责」把等待队列写成「正在等待的申请，放行、拒绝、封禁」，
把撤销写在操作记录那一行，和「谁改了什么、每次判定的来龙去脉」并列。
`web/src/features/queue/fixtures.ts` 曾为 `banned` 行提供 `revoke` 动作，
但没有对应端点。该动作现已删除；`scripts/check-phase-seams.py` 会拒绝队列功能
再次使用 `/audit` 或 `revoke`。本段记录决定依据，不是待办。

上一代给出了这样分屏的理由，写在 `internal/verify/service.go:733-735`：

> 解封是危险的那一半。刚刚封了这个人的管理员会发现封禁被悄悄解除，
> 所以 Telegram 已报为封禁或已离开的人一律原样不动 —— 把他放进去的那个人比这次结算更有权。

一个按钮要能安全地解封，就必须知道这条封禁是谁下的；操作记录知道，等待队列不知道。
所以这不是「端点还没建」，是**控件建在了错的屏上**。

因此：队列那一片不接这个按钮，也不为它发明端点；`banned` 行在队列里只显示状态。
撤销连同 `GET .../audit` 与 `POST .../audit/{aid}/undo` 一起归阶段七的操作记录屏，
并且那一屏必须先能显示这条封禁的来源，撤销才允许出现。

一句话就能推翻：如果维护者要在队列里直接撤销自己刚下的封禁，
那就把它限定为本人在本次会话内下的封禁，而不是对所有 `banned` 行都显示。

### 设计文档与架构文档的冲突：已解决

一次一致性检查发现十二项冲突。每项确定唯一结果并写入对应文档，
修改后重新检查两份文档。

| 冲突 | 决定 |
|---|---|
| 令牌录入位置 | 脚本一个密钥都不问，全部在浏览器里填，脚本里不留读令牌的分支 |
| 容器更新按钮 | 两种部署共用一套机制，都有按钮。应用只写目标版本，宿主侧的单元执行 |
| 配置包能不能带命令 | 架构书自相矛盾，已改：判据是能不能用取数加模板说清楚，说不清楚的就是代码 |
| 首次认领路由 | 有这条路由，`/setup/{token}`，**认领成功后不再注册**，之后一律 404。`/verify/{token}` 仍是长期存在的唯一公开面 |
| 设置值的来源 | 来源与「未保存」分开显示。来源三种：出厂默认、本群设定、由文件管理；由文件管理的置为只读并标出来源 |
| 群模式的展示 | 群列表每行与首页顶部都要显示。缺三项前置中任一项时，这个群标成不可验证 |
| `membership` 的归属 | **看命中之后做什么**。「去关注再回来」是挑战，有记录有到期；「已在主群就免验证」在任何挑战之前求值，命中即跳过，不建记录。同一种条件类型两种用法 |
| 挑战发送位置 | 按每群设定发到群内、私聊或两者，默认两者。`challenge` 表加 `delivery` 记录实际送到哪几处 |
| 反垃圾算不算规则集合 | 算。`rule.collection` 加 `antispam`。`definition` 本来就不透明，结构上零成本 |
| 屏与接口的覆盖 | 路由表补到二十三条并声明**它是穷举的**。宣称之后当场核对，发现四个屏没有写入口：等待队列能放行却没有结算路由、操作记录写着含撤销却没有、订阅推送整屏没有、群与频道只有读。补齐后由 `check-docs.py` 逐屏核对 |

顺带发现一处检查没列出来的：设计书写五种验证方式，架构书 `challenge.kind` 只有四种。
**六种是使用者看到的选项，四种是判定机制**，三种问答共用 `rule`。两边都写明了这层对应。

## 5. 禁止事项

- 上一阶段未合入 `main` 时，不开始下一阶段。
- 纯移动阶段不附带修复。
- 不为单个群增加代码分支。
- 不为不变量增加开关。
- 合并、创建 PR 和推送的授权以 `CONTRIBUTING.md` 的「What may happen without asking」为准。
  该节是仓库规则的唯一声明，本文件不复述，以免规则副本过期。
