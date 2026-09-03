# 架构

<!-- 由 scripts/gen-arch-md.py 从 web/architecture.html 生成，不要手改。 -->

## 0. 不变量

每一阶段都必须成立。任何一条被破坏，即为该阶段未完成。

| 不变量 | 破坏时会发生什么 |
|---|---|
| 控制台与 Telegram 更新调用同一个 Service | 后台放行与群内答题走出两套规则 |
| 验证包不引用 telego | 验证逻辑无法在无网络条件下测试 |
| 失败朝拒绝方向倒 | 上游抖动时陌生人被放行入群 |
| 状态转换是带条件的更新 | 并发路径重复结算 |
| 先落库，再对外可见 | 挑战已发出而系统不知道，重启后无人结算 |
| 迁移失败即退出 | 以旧结构接收流量，写坏数据 |
| 代码中没有针对特定群的分支 | 通用性只是说法，删掉我们的配置就无法运行 |
| 机器人令牌不进日志、界面、仓库 | 令牌泄露等于全部身份可被伪造 |

验收标准：删掉我们社区那几行配置，产品照常运转。

## 1. 不许写成面条

**每一次改动都按完整架构落地，不允许先塞进去以后再整理。** 「以后再拆」在这个仓库里没有发生过一次，它只是把成本推给下一个人。

### 硬性上限

| 项 | 上限 | 超了怎么办 |
|---|---|---|
| 单个文件 | 600 行 | 按职责拆成多个文件，不是按行数切 |
| 单个函数 | 80 行 | 抽出命名清楚的子函数，名字要说明它做什么 |
| 函数圈复杂度 | 15 | 通常意味着分支该换成查表或多态 |
| 一次提交涉及的职责 | 1 个 | 加功能与改结构分两次提交 |

上限是触发讨论的阈值，不是可以逼近的目标。**接近上限时先问是不是分错了包。**

### 新代码必须有归属

新增的代码只能落在本文已经声明的包里。**需要一个新包时，先改本文**， 说明它管什么、允许什么、禁止什么，再写代码。

- **不许出现 `util`、`common`、`helper`、 `misc` 这类包。**它们没有边界，因此任何内容都能进去， 最后成为第二个面条团。
- **一个函数不同时做取数、判断和渲染。**三件事分给三处。
- **业务包里不拼接面向用户的文案。**文案归 `i18n`， 业务返回结构化结果。
- **不复制一段逻辑改两行。**第二次出现时抽出来， 第三次出现说明抽错了地方。

### 怎么检查

门禁里带文件行数、函数行数与圈复杂度三项，超限即失败。 包边界用导入检查：验证包不许引用 telego，规则包不许引用数据库与网络。

**这些检查在第一次实质改动之前就要能执行。**先有尺子，再动手。

## 2. 部署拓扑

三个组件，一次部署。

```text
管理员浏览器（Telegram 内）
        │
        ▼
┌─────────────────────────────────────┐
│  app  ·  单个 Go 二进制              │
│                                     │
│   console        :8080              │
│   verification                      │
│   telegram                          │
│   web/dist       go:embed           │
└──────────┬──────────────┬───────────┘
           │              │
           ▼              ▼
   ┌───────────────┐  ┌──────────────────┐
   │ 数据库         │  │ telegram-bot-api │
   │ SQLite 或 PG   │  │ 容器 · :8081     │
   └───────────────┘  └────────┬─────────┘
                               │ MTProto
                               ▼
                          Telegram
```

- **app** 同时承载机器人与控制台，前端构建产物编进同一个二进制，线上不运行 Node。
- **telegram-bot-api** 自建，不可省略：直连实测 707 ms，本地 1.3 ms。 官方不提供静态二进制与镜像，因此在发布流水线编译一次并发布按 commit 固定的镜像，部署机只拉取。
- **数据库** 自托管默认 SQLite 单文件，公开实例使用 PostgreSQL。 同一份迁移，只有个别 DDL 分方言。

## 3. 包结构

形状取自 [mautrix-telegram](https://github.com/mautrix/telegram)， 它是同类型的 Go 程序，也把一个外部协议接进自己的核心。四条做法直接照抄。

| 照抄的做法 | 它怎么做 | 我们怎么用 |
|---|---|---|
| 按领域概念分文件 | `portal.go`、`user.go`、`login.go`、 `queue.go` 各一个文件，不按技术层切 | `join.go`、`answer.go`、`settle.go`、 `timeout.go` |
| 标识转换独立成包 | `connector/ids` 只做标识互转 | 同名包。避免整套代码到处传裸整数 |
| 每个方向一个格式化包 | `telegramfmt` 与 `matrixfmt` 分开 | `tgfmt` 只负责把我们的消息渲染成 Telegram 格式 |
| 适配器自带存储与独立版本表 | `connector/store` 用 `db.Child()` 取得独立版本表 | 适配器的表与核心的表分开升级，互不牵制 |

```text
cmd/bot/                    入口，解析命令行，调用 app.Run

internal/
├── app/                    装配、生命周期、后台任务注册
├── verification/           核心
│   ├── ports.go            Gateway 与 Store 接口定义在这里
│   ├── join.go             入群申请
│   ├── answer.go           答案判定
│   ├── settle.go           批准、拒绝、封禁
│   ├── timeout.go          到期扫描
│   └── postjoin.go         进群后验证
├── rules/                  规则引擎
│   ├── normalize.go        匹配前归一化
│   ├── condition.go        封闭的条件类型集合
│   └── signals.go          结构信号计分
├── telegram/               适配器
│   ├── connector.go        实现 verification.Gateway
│   ├── updates.go          轮询或 webhook，Update 转领域事件
│   ├── ids/                标识转换
│   ├── tgfmt/              消息渲染
│   ├── queue/              发送队列、按群限流、429 退避
│   └── store/              适配器自己的表，独立版本表
├── console/                Web 后台
│   ├── api/                路由与数据传输结构
│   ├── auth/               initData 校验与运维令牌会话
│   └── assets/             前端产物 go:embed
├── settings/               配置，configupgrade
├── database/               dbutil 装配与迁移
└── status/                 健康、替换状态与按需发布查询

web/                        前端源码，Vite + React
migrations/                 编号 SQL
testdata/                   样本与固定装置
```

| 包 | 允许 | 禁止 |
|---|---|---|
| app | 引用全部 | 承载业务判断 |
| console | 调用 verification 与 status | 直接调用 telegram，直接执行 SQL |
| verification | 使用自己定义的接口，调用 rules | 引用 telego，出现 Telegram 类型，格式化面向用户的文案 |
| rules | 纯函数，无副作用 | 访问数据库或网络 |
| telegram | 实现 verification.Gateway | 决定谁通过谁拒绝，查核心的表 |

### 端口的签名

这一节以前只说了「接口定义在 `ports.go`」而没有给签名， 结果是照着它做的人只能自己编。签名按上一代实际用到的调用面定， **不按想象中该有的样子定**。

```text
type Gateway interface {
    // 投递。富文本被平台拒绝时退回简版；瞬时失败绝不重试，
    // 因为第一条可能已经送到了。这条是上一代用重复消息换来的。
    DeliverChallenge(ctx, *Challenge, Delivery, rendered Rendered) (Delivered, error)
    DeliverResult(ctx, ChatID, rich, simpler string) (MessageID, error)
    Notify(ctx, ChatID, text string, ttlSeconds int)
    Alert(ctx, logChat ChatID, text string)
    FailAlert(ctx, logChat, group ChatID, text string)
    Audit(ctx, logChat ChatID, text string)
    Retract(ctx, []MessageRef) error

    // 交互应答。一次交互只能调用其中一个。
    AckFast(ctx, InteractionID) error
    AckResult(ctx, InteractionID, AckResult) error

    // 成员动作。时长用秒，与平台一致；在边界上换算不损失精度，
    // 而换成 time.Time 会引入一个没人要求的取整决定。
    ApproveJoin(ctx, ChatID, UserID) error
    DeclineJoin(ctx, ChatID, UserID) error
    Ban(ctx, ChatID, UserID, seconds int, revoke bool) error
    Unban(ctx, ChatID, UserID, onlyIfBanned bool) error
    Mute(ctx, ChatID, UserID, seconds int) error
    Unmute(ctx, ChatID, UserID) error

    // 权限。两个，不是一个：读路径用缓存的，写之前必须用现查的。
    CachedAdmin(ctx, ChatID, UserID) (bool, error)
    FreshAdmin(ctx, ChatID, UserID) (bool, error)

    // 成员状态。布尔不够：只解除本次验证下的那次禁言、不撤销管理员已有的封禁、
    // 申请消失后判断人到底进没进群，都要看具体是哪一种。
    Member(ctx, ChatID, UserID) (Membership, error)
}

type Store interface {
    LoadPending(string) ([]PendingRecord, error)
    InsertPending(string, PendingRecord) (bool, error)
    UpdatePending(string, PendingRef, PendingRecord) (bool, error)
    TransitionChallenge(string, ChallengeTransition) (bool, error)
    DeletePending(string, PendingRef) (bool, error)

    // 扫描器在一个有界事务中领取已到期记录，并把 deadline 前推到 claimUntil。
    // now、claimUntil 和 limit 由调用点传入；不引入 Clock，也不让一次领取无限增长。
    ClaimExpired(namespace string, now, claimUntil int64, limit int) ([]PendingRecord, error)

    // 动作领取以可过期的 claim 保护；进程崩溃后会由另一名 worker 重试。
    ClaimActions(namespace, owner string, now, claimUntil int64, limit int) ([]PendingAction, error)
    CompleteAction(namespace, id, owner string, completedAt int64, followups []ActionIntent) (bool, error)
    RetryAction(namespace, id, owner string, attempts int, nextTryAt int64, detail string) (bool, error)
    FailAction(namespace, id, owner string, failedAt int64, detail string) (bool, error)

    LoadFailures(string) ([]FailureRecord, error)
    SaveFailures(string, func() []FailureRecord) error
    LoadAgents(string) (AgentTally, error)
    SaveAgents(string, func() AgentTally) error
    LoadHeartbeat(string) (HeartbeatRecord, error)
    SaveHeartbeat(string, HeartbeatRecord) error
}
```

`ClaimExpired` 是扫描器的领取操作，不是读取后由调用方逐条更新。 它以 `deadline ≤ now` 选择 `pending` 行，条件更新 `epoch` 与 `deadline=claimUntil`，再返回实际领取到的记录。 因为同一事务完成选择和标记，多个进程只会有一个领取者；因为 `limit` 固定， 慢查询不会占用无界事务。

时间保存为 Unix 秒。到期扫描间隔为 5 秒，领取 lease 为 30 秒，每批最多 32 条。 因此超时结算的额外延迟通常不超过一个扫描间隔，加上一次数据库与动作调度；重启后不会遗漏。 秒级精度是这里刻意接受的取舍，避免为每条挑战维持进程内定时器。

每一处形状都有理由，而且每一条都是上一代踩过的：

| 形状 | 为什么不是更直白的写法 |
|---|---|
| `DeliverChallenge` 收一份投递意图，返回**实际送到了哪几处** | 挑战按每群配置发到群里、私聊或两者。写成返回单个消息引用就表达不了部分成功， 而部分成功是常态：机器人无法主动私聊没有交互过的人 |
| `Retract` 收一组消息引用 | 结算时要收掉这次挑战贴出的全部消息。逐条删就会删一半失败一半， 留下半截现场 |
| 应答是独立一项，**不并进结算**，并且分成 `AckFast` 与 `AckResult` 两个方法 | 上一代按钮转两秒，原因是回调应答排在三四次串行往返之后。 并进结算就必然又排到后面。但也不能一律提前： 申请人那条路上，应答的文案就是结果。见「回调应答分两种，不是一条规则」 |
| `Mute` 与 `Unmute` 成对； `Kick` 与 `Ban` 分开 | 禁言档的验证通过后要抬走自己下的那次禁言，没有解除就是过了还是哑的。 踢和封是两件事，合成一个带时限的调用会让调用方无法表达「只踢不封」 |
| `Membership` 一次返回身份与权限 | 是否在群、是否管理员来自同一次查询。拆成两个方法就是两次往返， 而每次往返在我们的部署位置上约半秒 |
| `TransitionChallenge` 同时收状态转换与动作意图 | 挑战状态和第一项外部动作写入同一个事务。分两次调用时，进程在中间退出会留下 「已判定但没有动作」；外部 API 调用不在事务内，由动作 worker 在提交后执行 |
| `TransitionChallenge` 以 `PendingRef.epoch` 条件更新 | 扫描器领取到期记录时递增 epoch。旧领取者即使继续执行，也不能结算被新领取者接管的挑战 |
| `InsertPending` 返回 `bool` 而不是靠错误区分 | 重复到达由唯一索引拒绝，那不是异常，是预期内的一种结果 |
| `ClaimExpired(now, claimUntil, limit)` 返回实际领取到的记录 | `now` 让测试直接固定时间；`claimUntil` 让进程崩溃后的工作可重新领取； `limit` 限制一次事务。三者共同避免 `Clock` 接口和无界扫描 |

### 端口用到的类型

上一版只给了方法签名，**签名里的每一个类型都没有定义**。 派去做这一片的助手因此停下来：它无法在不自拟约定的前提下完成一次无行为变更的搬移， 而自拟约定正是被禁止的。类型补在这里。

```text
type ChatID int64
type UserID int64
type MessageID int
type ChallengeID string
type InteractionID string          // Telegram 的 callback query id

type State string                  // pending approved declined banned expired superseded
type Kind  string                  // rule pow captcha membership
type Gate  string                  // request  申请制    mute  先入群后限制

type MessageRef struct { Chat ChatID; Message MessageID }

type Delivery struct {             // 要送到哪几处，来自每群设定
    Group   bool
    Private bool
}
type Delivered struct {            // 实际送到了哪几处
    GroupMsg   MessageID           // 0 表示没送到
    PrivateMsg MessageID
    Any        bool                // 两处都没送到时为假，到期自动拒绝
}

type Challenge struct {
    ID        ChallengeID
    Chat      ChatID
    User      UserID
    Gate      Gate
    Kind      Kind
    State     State
    Invited   bool                 // 是别人拉进来的，通知里让管理员可以担保
    Held      bool                 // 本次验证下过禁言，通过时要抬走
    HoldUntil int64                // 那次禁言的到期时间，用来认出是不是自己下的
    Passing   bool                 // 已答对，重试只能走批准，不能改判拒绝
    Attempts  int
    Nonce     string               // 认出过期的旧事件
    Epoch     uint32               // 掉线恢复后递增，拒绝上一轮的结算
    Lang      string
    Payload   string               // 题面、诱饵、难度，按 Kind 解释
    Delivered Delivered
    ExpiresAt int64
    DeferredSince int64            // 管理员手工挂起的起点，有 48 小时上限
}

type Rendered struct {             // 核心渲染好的文本与实体，适配器不再加工
    GroupRich   string
    GroupPlain  string             // 平台拒绝标记时退回这一份
    DMRich      string
    DMPlain     string
    Buttons     []Button
}

type Membership struct {
    Status        string           // member creator administrator restricted left kicked
    IsAdmin       bool
    CanRestrict   bool
    RestrictedTil int64            // 认出这次禁言是不是本次验证下的
}

type Action string                 // approve decline ban kick mute unmute retract
type AckResult struct {            // 回调应答；文案本身可能就是结果
    Text  string
    Alert bool                     // 弹窗还是气泡
}
```

### 这份契约是从八个接口反推的，不是想出来的

第一版 `Gateway` 有十个方法，是照着「核心大概需要什么」写的， 只核对过我碰巧查到的那几处。派去实施的助手连着指出七组缺口， 最后一组把问题的性质说清楚了：**`internal/verify` 声明的不是两个接口，是八个。**

| 它声明的 | 是什么 | 去哪 |
|---|---|---|
| `verifyTransport`（10 个方法） | **上一代自己的端口，而且分得很好**：投递、清理、告警、审计、 封禁解禁、禁言解禁 | 去掉平台类型后**就是** `Gateway` 的主体 |
| `adminTransport` | `CachedAdmin` 与 `FreshAdmin` 两个查询 | 两个都留。合成一个会丢掉「写前现查」与六十秒缓存的区分， 而那是 v3.6.7 量出来的 |
| `verifyBot`、`modBot`、`Telegram` | 直接暴露 telego 参数结构体的三层包装 | 不保留。我们的不变量不允许核心出现平台类型 |
| `liveProbe`、`heartbeatBot` | `GetMe` 探活 | 落地时留成了核心的第三个端口 `LiveProbe`，只有 `Probe` 一个方法。原打算归 `app`， 但停机暂停、恢复重发和重启后的停机窗口估算都还在核心手里， 搬走探活就要连这些一起搬，那是阶段三的事 |
| `botUnwrapper` | 取出底层客户端的逃生口 | 不保留。它存在的理由是包装层太多 |

**教训写在这里：这类契约要自下而上从被替换的代码反推，不能自上而下想。** 想出来的版本每一轮都会被实施推翻一次，而每一次推翻都是一整次派发。

被推翻的具体几处：漏掉退回简版的投递、 把审计定成本地存储，而它发到日志群、漏掉带清理时限的通知、 漏掉解封、把两个权限查询合成一个、时长用了 `time.Time` 而平台收秒。

### 第四轮：五处只有动手搬才会暴露

又一位助手停在这里，五条都带行号，都成立。**它们有一个共同点： 隔着代码读不出来，只有真去搬才撞得到。**

| 撞到什么 | 改成 |
|---|---|
| `DeliverChallenge` 只收 ID 与投递意图， **挑战的内容无处传**。核心按申请人姓名、语言、群模式、nonce、 恢复窗口、频道提示、管理员按钮构造消息，而适配器持有的只是客户端与队列； 让它按 ID 回查核心状态又违反包边界 | 收整条 `*Challenge` 与一份**核心已经渲染好**的 `Rendered`：群内与私聊各一对富文本与退化文本，加按钮。 **适配器不再加工文案**，这与「业务包不拼接标记」是同一条 |
| `Membership` 在正文里被论证过，**接口里却没有**； 而且只回一个布尔不够 | 补进接口，返回状态而非布尔。只解除本次验证下的那次禁言、 不撤销管理员已有的封禁、申请消失后判断人到底进没进群， 都要看具体是哪一种 |
| `Store` **没有非终态写回**。 首次样本提示、fallback 换题、批准失败后保留 `passing` 重试， 都是在未结算时落盘的 | `Update`，带 `epoch` 的条件写入。 少了它，重启后的判定、重试与 nonce 语义都会变 |
| 管理员应答次序，**文档与实现冲突** | 按实现改文档。写之前必须现查管理员身份，这一步绕不过去； 而应答文案要区分「不是你的」「已经处理过了」「做完了」。 先应答就没有结果可说 |
| 审计的归属，**同一份文档自相矛盾**： 接口里有 `Audit`，正文又说它进本地存储 | 经 `Gateway` 发日志群，与实现一致。 「进本地存储」是我想当然写下的 |

**方法本身也要改。**这份契约被实施推翻了四轮，缺口数是 6、3、5，没有收敛。 原因不是核得不够仔细，是**隔着代码写签名这件事本身不成立**： 端口的形状由调用点决定，而调用点要搬到眼前才看得全。

因此这一节从今往后**是「搬的时候必须承载什么」的清单，不是照着抄的签名表**。 做搬移的人按不变量推导接口：核心里不出现平台类型；然后把推导出来的形状写回这里。 **文档记录已经成立的事实，不预先规定尚未验证的形状。**

### 第八轮：搬完了，这是推导出来的形状

改成清单之后的那一次派发把这一片搬完了。下面是**从调用点推导出来**的三个端口， 不是写在前面等人照抄的。

| 端口 | 方法数 | 为什么是这个数 |
|---|---|---|
| Gateway | 18 | 上一代的 `verifyTransport` 与 `adminTransport` 加起来只有十个。多出来的八个是当年**绕过自己那两个端口、直接调机器人**的地方： 直接发送、富文本被拒后的退化链、失败告警、完整成员状态， 以及两种应答。这八个正是隔着代码看不见的部分 |
| LiveProbe | 1 | 只有 `Probe`。原打算把探活整个归 `app`， 但停机暂停、恢复重发和重启后的停机窗口估算都还在核心， 所以这一片只把平台身份挡在外面 |
| Store | 8 | 四份 JSON 快照各一读一写。这一片不换介质、不动文件名与字段、 不调整加锁顺序；那些是阶段三的事，所以接口是现状的形状， 不是数据库的形状 |

编译期断言写在各自的实现旁边：`telegram.VerificationGateway`、 `database.VerificationJSONStore`、`app.outageAwareBot`。 `internal/verification` 里没有 telego， `scripts/boundaries.txt` 守着这一条。

**那八个多出来的方法说明了改成清单是对的。** 前七轮都试图先写签名，而这八个方法在旧代码里没有任何接口声明过它们： 它们只以「直接调机器人」的形式存在。隔着代码读，读到的是那两个端口； 搬到眼前，才看得见绕过端口的那些调用。

### Store 要覆盖核心实际持有的状态

第一版只有五个方法，覆盖不了 `internal/verify` 真正写在盘上的那几份状态。 逐个查过它读写的四个文件之后，**其中两个根本不属于这一层**：

| 文件 | 存什么 | 归谁 |
|---|---|---|
| `pending.json` | 待验证记录本体 | `verification`。`Create` 到 `ClaimExpired` 已覆盖 |
| `verifyfail.json` | `{group_id, user_id, count, last}`：某人在某群连续失败几次、最后一次什么时候。 驱动冷却与自动封禁，上限五万条 | **`verification`，原契约里缺**。补三个方法 |
| `agents.json` | `{total, counts}`：诱饵命中时按对方自称的模型计数 | **`status`**。这是统计，不参与任何判定， 放进验证的 Store 会让那个接口同时管两件事。 **这是阶段三的去处**：阶段一第三片不换介质也不动锁次序， 落地的 `Store` 仍完整承载这四份 JSON |
| `heartbeat.json` | `{last_online}`：重启后据此估算停机了多久 | **`app`**。与「连接中断由装配层处理」是同一个决定： 核心不需要知道离线这件事，驱动扫描的那一层才需要 |

第二轮自下而上核对又找出三处。**每一次核对都仍有新的缺口， 这本身就是「凭想写契约」不成立的证据**：

| 核心实际做的 | 原契约 | 改成 |
|---|---|---|
| `recordKernelTry` 同时校验 nonce 与「是否已结算」， 再加计数并返回剩余次数 | `Attempted` 不收 nonce | **收 nonce 并返回是否接受**。不收就挡不住一条过期的回调 白白消耗掉申请人一次机会 |
| `pruneVerifyFails` 按每群的重试窗口清理失败记录 | 三个失败方法里没有清理 | `PruneFails`。窗口按群取，因此传一个取窗口的函数进去， 而不是一个固定时长。没有它，记录会一路涨到五万条上限 |
| `recordDecision` 与 `Stats` 每日通过/拒绝计数 | 没有 | `Tally`。**阶段三之后它应当变成对已结算记录的一次查询**， 而不是另存一份计数；这一片不换介质，所以先照搬 |

另外三处形状的理由：

| 方法 | 为什么是这个形状 |
|---|---|
| `OpenByUser` 按人查，不按群 | 同一个人可能同时在几个群等待。跨群看他，才能判断这是不是一次批量注册 |
| `AttachDelivery` 返回 `bool` | 投递结果回填时，那条记录可能已经被超时结算掉了。 **条件写入，落空不是错误**，是「来晚了」 |
| `Attempted` 单独一个方法 | 答错一次要加一次计数，而这不是一次状态转换， 记录仍然待验证。塞进 `Settle` 会让那个方法名不副实 |

### 三处签名被实现推翻了

把契约拿去实施时，有三处与现有行为不符。**照实改契约，不是照契约改行为。** 这一片的规矩是不许改判定。

| 原来写的 | 与什么不符 | 改成 |
|---|---|---|
| `Ban(ctx, chat, user, until)` | 自动封禁不撤回历史消息，管理员封禁撤回。 `internal/verify/service.go:2703` 传 `false`， `:2750` 传 `true`。签名里没有这个参数， **取任一固定值都改变行为** | `Ban(ctx, chat, user, until, revoke bool)` |
| **一次交互只应答一次，且 `AckFast` 与 `AckResult` 只能调用其中一个** | Telegram 一个回调只能应答一次，**而应答的文案有时就是结果**： 「不是你的」「已经处理过了」。所以次序按交互类型定，不是一律提前， 见下一节。`internal/verify/service.go:1781-1799` （上一代路径） | 分两种，见下 |
| 只有 `DeliverChallenge` | 核心还要发结果私聊、失败通知、管理员告警与审计记录， 各有各的清理与去重语义，塞进同一个方法会让它无法解释 | 按语义分开，见下 |

### 回调应答分两种，不是一条规则

上一代 v3.6.7 专门改过这里：管理员点「踢出」「通过」时按钮要转两秒， 原因是应答排在三四次串行往返之后。修法不是「一律先应答」，而是分情况：

| 谁点的 | 次序 | 为什么 |
|---|---|---|
| 管理员按钮 | **先校验 nonce，再现查管理员身份，最后带文案应答** | 写之前必须现查，这一步绕不过去；而应答的文案要区分 「不是你的」「已经处理过了」「做完了」。 v3.6.7 优化的是**把这几次调用从串行改成尽早 ack 之外的部分并行**， 不是把应答提到判定之前；提前就没有结果可说了 |
| 申请人答题 | **先判定，再应答**，应答带结果文案 | 那个提示就是他唯一能看到的结果。先应答就等于把结果扔了 |

因此端口给两个方法：`AckFast(ctx, InteractionID)` 不带文案、用于管理员按钮；`AckResult(ctx, InteractionID, AckResult)` 带文案、用于申请人。**一次交互只能调用其中一个**，这一条要在实现里断言。

### 投递按语义分开

| 方法 | 送什么 | 它自己的语义 |
|---|---|---|
| `DeliverChallenge` | 挑战 | 按 `Delivery` 送到群内、私聊或两者，返回实际送达处。 两处都没送到不是错误，是一种结果 |
| `DeliverResult` | 通过或拒绝的通知 | 私聊送不到就算了，不重试。群内那条有清理时限 |
| `Alert` | 管理员告警 | **带去重与节流**：同一件事在窗口内只发一次。 没有配置告警群时退回到群内，并带更短的清理时限 |
| `Retract` | 收回本次挑战贴出的消息 | 收一组引用，逐条删除失败只记录不阻断结算 |

审计经 `Gateway` 发到日志群，与上一代一致 （`internal/verify/service.go:2147`）。 早先这里写过「审计进本地存储」，那是想当然，实现里不是。

### 连接中断由装配层处理，不进端口

上一代用 `GetMe` 心跳判断能不能连上 Telegram，连不上时暂停验证超时， 避免把机器人自己的掉线判成申请人超时 （`internal/verify/state.go:609`、`:892`）。

**这件事不需要端口方法。**超时由扫描器领取，而扫描器由 `app` 驱动： 平台不可达时 `app` 停止驱动它，恢复后再开始，并把 `epoch` 递增， 使掉线前那一轮的结算无法生效。**核心不需要知道「离线」这个概念**， 它只知道没有人来叫它扫描。少一个端口方法，也少一处两边状态可能不一致的地方。

### 接口上要写清不许做什么

上面那张表说的是每个方法为什么长这样，**但没说实现时不许做什么**。 mautrix 的接口注释两样都写：某个方法「不允许返回错误，连接错误走另一条通道」， 某个方法「在全局缓存锁里被调用，因此不能做慢操作」。 约束写在接口旁边，实现的人不必去读调用方才知道。补上我们的：

| 约束 | 违反会怎样 |
|---|---|
| **不许在数据库事务里调用 `Gateway` 的任何方法** | 每一次调用都是一趟网络往返，在我们的部署位置上约半秒。 事务被这样撑住，锁的持有时间从毫秒变成秒 |
| `Store` 的实现不许发起网络调用 | 它是数据层。一旦它自己去问 Telegram， 「先写库再执行动作」这条次序就无从保证 |
| 应答不排在几次串行往返之后 | 排在后面按钮就转好几秒，上一代正是这么慢下来的。 但申请人那条路要先判定再应答，因为文案就是结果； 约束是「不串在后面」，不是「一律最先」 |
| `ClaimExpired` 必须受 `limit` 约束， 实现不得忽略它 | 积压之后一次领走全部到期记录，一轮扫描要发成百上千条消息， 直接撞上限流 |
| `Gateway` 的实现不许决定谁通过谁拒绝 | 那是 `verification` 的事。适配器一旦开始判断， 就有两处地方在做同一个决定 |

### 为什么不做可选能力接口

mautrix 把必须实现的部分压得很小，其余能力各自一个可选接口， 核心用类型断言探测。`bridgev2/networkinterface.go` 里 `NetworkConnector` 只有十个方法，`NetworkAPI` 九个， 而整个文件有**八十四个接口**。

这个模式解决的是**多个平台各自支持不同能力**：连接器实现哪个接口就支持哪个能力， 新增一种能力不会让已有的连接器编译不过。代价是那份契约无法一次读完， 要知道有哪些可选能力，只能通读整个文件。

**我们只有一个平台，所以不取。**十个方法写在一个接口里， 一眼看得完，测试替身实现十个方法也不算负担。 **触发条件写在这里**：真的出现第二个平台时，按它的形状拆， 而不是那时才临时想一个。

与上一代的分歧记在这里：它的 `verifyBot` 直接暴露 telego 的参数结构体， `internal/verify/service.go:55`。那样省掉一层转换， 代价是核心包里出现平台类型，我们的不变量不允许。 **转换的成本落在适配器，换来核心可以脱离平台测试。**

**用编译期断言固定接口。**`var _ verification.Gateway = (*Connector)(nil)` 写在实现旁边，接口变更在编译时暴露，这一条同样取自 mautrix。

`rules` 是纯函数包，因此可以脱离一切依赖测试， 也可以被控制台的试答直接调用，不必绕一圈网络。

### 进程骨架的选择

三种常见骨架都能监督后台任务，但它们表达退出顺序的能力不同。

| 模式 | 优点 | 本项目的代价 |
|---|---|---|
| `run.Group` 的成对 actor | 实现小，任一关键 actor 返回即可触发整组退出 | 六个任务并不对称。更新入口必须先停，发送器必须最后排空， 顺序仍隐藏在注册位置 |
| 后台服务 registry | 统一为 `Run(context.Context) error`，可表达依赖、状态与禁用 | 六个固定任务不需要动态依赖图，引入 manager、反射命名与服务状态会增加 生命周期本身的复杂度 |
| 显式 `App.Start/Stop` | 组装、启动与反向退出顺序直接可见 | 必须自行补齐部分启动失败回滚、统一超时与关键组件监督 |

**选择显式 `App` 编排。**`internal/app.App` 是唯一生命周期所有者；`app.New` 只升级并校验配置、迁移数据库和组装依赖， 不启动 goroutine。`App.Run` 按固定顺序启动组件，任何关键组件在非退出阶段返回 都进入同一关闭路径。后台任务统一实现 `Run(context.Context) error`， 但不引入动态 registry 或 `run.Group`。

`cmd/bot` 只处理进程参数、信号 context 与退出码， 不保存业务对象，也不直接执行 SQL。

### 拉取更新时必须声明要哪几种

平台默认**不投递** `chat_join_request`。 权限给满、群里开关也开了，只要拉取时没有声明这一项， 加群申请一条都收不到，而且没有任何报错。上一代把五种一次声明清楚， `cmd/gentoo-zh-verify-bot/main.go:368`：

```text
chat_join_request    加群申请，验证的唯一触发点
chat_member          成员变化，用于恢复期核对与踢出后清理
my_chat_member       机器人自己被加入或移出群
message              入群后验证、反垃圾、命令
callback_query       按钮作答
```

**这一项归启动前的自检管。**拉取时声明了什么是自己的配置， 能不能收到还取决于群的设置和机器人的权限。三者缺任一， 表现都是「什么也没发生」，因此要在启动时逐项报，不能等第一个申请进来才发现。

| 查什么 | 不满足时的表现 |
|---|---|
| 拉取时声明了 `chat_join_request` | 没有申请事件 |
| 群是超级群且开了「需管理员批准」 | 平台不产生申请事件 |
| 机器人是管理员，且有邀请、封禁、删除三项 | 收得到但结算不了 |

三条都通过才算这个群「可以验证」， 在控制台的群列表里就要这么标。**标成可用却什么都不做，比标成不可用更糟。**

### 装配与生命周期

启动顺序照 mautrix：**先起连接，再起会话，最后异步做启动后的收尾**。 关闭顺序是它更值得抄的部分，逐条都有理由。

```text
1  读配置、打开数据库并执行迁移       失败退出
2  组装 Telegram、verification 与数据库 Store
3  原子领取 update_poll_lease       未持有则继续运行，但不注册更新 handler
4  启动 feed、心跳、到期扫描与动作 worker
5  仅持有租约的进程启动 Telegram long polling
6  handler 注册随 long polling 创建；lookup 预热异步执行
```

```text
1  关闭 root context，停止新工作
2  取消 Telegram long polling，并按 holder 释放 update_poll_lease
3  等待已开始的 update handler 与注册任务
4  等待心跳、到期扫描器和动作 worker 退出
5  刷新 verification 与 feed 状态
6  最后关数据库                它是所有人的依赖，必须最后关
```

| 照抄的做法 | 为什么 |
|---|---|
| 以固定单行原子领取更新拉取租约 | Telegram 每个 token 只能有一个更新流；行只在租约过期时可被接管，避免两个 handler 并存 |
| 续租只接受当前 `holder` | 续租失败立即取消 long polling。失去所有权的进程保留其他后台任务，但不再消费更新 |
| 停止时先取消 polling 并释放匹配 holder 的租约，再等待 handler | 新实例可尽早领取；旧实例不能在等待过程中继续拉取，也不能删除继任者的租约 |
| 未持有租约时不注册 Telegram handler | 进程保持存活以执行非更新任务，但没有任何路径可消费重复的 update |
| 数据库最后关 | 扫描器、动作 worker 与状态刷新都依赖数据库，提前关闭会使已开始的写入失败 |

#### 优雅退出预算

默认总预算为 30 秒，可在 10 至 120 秒之间配置。各阶段都从同一个关闭起点计算 **累计绝对 deadline**，不是五段独立超时相加；前一阶段提前完成， 余量自动留给后续阶段。

| 累计 deadline | 阶段 | 正常动作 | 到期后 |
|---|---|---|---|
| T+2s | 关闭入口 | `ready=false`；停止 Telegram admission 与管理 HTTP listener | 关闭底层 transport 或 listener，继续下一阶段 |
| T+10s | 等待在途操作 | 等待 update worker、HTTP handler 与已开始的数据库事务，不再分配新业务工作 | 取消 handler context；未提交事务回滚，durable inbox 留待重放 |
| T+15s | 停止周期生产者 | 按注册顺序的逆序取消并等待，此后不再产生新 outbox | 取消剩余 I/O；由 lease 或版本条件保证下次可重试 |
| T+27s | 排空发送器 | 只完成已在途和当前已到期的 outbox，不等待未来延期、429 暂停或打开的断路器 | 取消发送请求，释放 lease 或等待 lease 到期 |
| T+30s | 关闭资源 | 刷新日志与指标，最后关闭数据库 | 记录未完成阶段并返回关闭错误；第二次终止信号可强制退出 |

某一阶段失败或超时后仍继续执行后续清理，不能因前一项失败而跳过数据库关闭。 发送器是消费者，不随周期生产者一起停止；生产者停止后，它保留到 `T+27s` 排空。业务状态没有 write-behind 内存缓冲， inbox 与 outbox 始终以数据库为准。

### 后台任务

六个长期任务都在 `app.New` 构造期固定注册，不提供运行时插件 registry。 发送器与更新入口是具有特殊启停顺序的显式组件；其余四项放进固定的周期任务 slice， 由同一个 managed-task 适配器提供独立 context、`Done`、 `Err` 与有超时的停止方法。

| 任务 | 注册位置 | 启动 | 关闭 | 失败处理 |
|---|---|---|---|---|
| 更新拉取 | `App` 的显式入口组件 | 取到租约后，最后开放入口 | 最先停止 admission 并释放租约 | 网络错误有限退避；令牌或协议配置错误返回进程级错误 |
| 发送队列 | `App` 的显式消费者组件 | 先于所有生产者 | 最后排空并停止 | 429 持久化延期；网络与 5xx 退避；永久 4xx 进入 dead-letter 或禁用群 |
| 到期扫描 | `verification.RunExpiryScanner` | 启动即扫描，此后每 5 秒领取至多 32 条；领取 lease 为 30 秒 | 取消 context 后不再开始结算 | 查询失败只记录并等待下一轮；已领取记录仍由 lease 到期后重试 |
| 动作执行 | `verification.RunPendingActions` | 启动即领取 ready 动作，此后每 5 秒运行；领取 lease 为 30 秒 | 取消 context 后不再开始外部调用 | 临时错误按退避重试；永久或耗尽错误记录为 `failed` |
| 权限同步 | 固定周期任务 slice | 与其他周期任务一同启动 | 按 slice 逆序停止 | 单个故障域暂停；全局认证错误返回进程级错误 |
| 订阅抓取 | 固定周期任务 slice | 与其他周期任务一同启动 | 按 slice 逆序停止 | 单个 feed 失败只影响该 feed；任务循环意外返回属于进程级错误 |

每个周期任务同步完成一次迭代后才重置 timer，同一任务不重叠； 一次执行过慢后，不并发执行积压周期。新增长期任务时，必须同时补入固定注册表、 启动与退出表、就绪和指标定义，以及生命周期测试。

**恢复只放在单项处理边界。**记录定位字段和 stack 后，把该项写成可重试或终态， 再继续下一项。顶层任务循环、发送调度器与 supervisor 不恢复；这些位置的 panic 或意外返回可能已经破坏锁、事务、堆或在途标记，必须让进程退出。

### 发送队列

消息先在业务事务内写入 durable outbox。内存 channel、最小堆和缓存只保存有上限的唤醒信息， 数据库行才是权威状态。队列提供至少一次发送，不宣称 Telegram 不支持的恰好一次语义。

```text
send_outbox(
  id, dedupe_key UNIQUE, chat_id, chat_kind, method,
  payload, state, attempt, available_at,
  lease_owner, lease_until, telegram_message_id,
  last_error, created_at, completed_at
)
INDEX(state, available_at, id)
INDEX(chat_id, state, id)
```

`dedupe_key` 防止同一业务事件重复创建 outbox。claim 使用有期限的 lease； 进程退出后，`lease_until` 到期即可重领。Telegram 已接收消息而返回值尚未落库时 仍可能重发，因此该窄窗口接受重复，并在已有 `message_id` 的操作中优先改用 edit。

```text
chatBucket {
  chat_id, chat_kind,
  head, has_head, in_flight,
  next_send_at, blocked_until, last_used_at,
  heap_index, breaker_key
}

Sender {
  global_limiter,
  buckets map[chat_id]*chatBucket,
  ready_min_heap,
  work[2 * workers], result[2 * workers],
  wake[1]
}
```

每个 bucket 只装载一条 durable 队首。单一 scheduler 按 `max(outbox.available_at, next_send_at, blocked_until)` 排序； 到期后先取得全局令牌，再把该群标为 `in_flight` 并交给有界 worker。 结果返回前不派发同群第二条，因此同群保持 FIFO，不同群仍可并行。

| 约束 | 初始值 | 达到上限时 |
|---|---|---|
| 全局发送 | 29 条/秒，burst 1 | 等待全局令牌 |
| 群组发送 | 每 3 秒一条，burst 1 | 队首留在最小堆 |
| 私聊发送 | 每秒一条，burst 1 | 队首留在最小堆 |
| 内存 bucket | 2,048 个活跃群 | 空 bucket 按最后使用时间驱逐，其他 due 群留在数据库 |
| outbox | 全局 100,000 条，每群 500 条 | 事务返回 backpressure 错误，不静默丢弃 |

收到 429 后不占用 worker 等待。当前行的 `available_at` 与对应 bucket 的 `blocked_until` 一起持久化为 `retry_after` 加 0 至 250 毫秒抖动， worker 随即释放。所有 Telegram 消息发送必须经过 `Sender`；权限查询等非消息 API 可有独立并发上限，但仍必须设置请求超时并处理 429。

Telegram 限额与退避字段见 [Bot FAQ](https://core.telegram.org/bots/faq#my-bot-is-hitting-limits-how-do-i-avoid-this) 和 [ResponseParameters](https://core.telegram.org/bots/api#responseparameters)。

### 断路器

断路器按**故障域**取键，不把同一个上游故障机械复制成每群一次探测。 消息发送仍按群分桶；共享 Bot API 的网络与 5xx 故障使用 `bot|upstream`， 独立 provider 使用 `group_id|provider`，订阅抓取使用 feed。 每个 bucket 只保存 `breaker_key`，由有上限的 breaker 表取得状态。

最简状态只需要 `state`、`generation`、`failures`、 `probe`、`open_for` 与 `open_until`。 `generation` 使旧请求的完成回调无法覆盖新一代状态。

| 当前状态 | 事件 | 转换 |
|---|---|---|
| closed | 成功 | 连续失败数清零 |
| closed | 第 5 次连续可重试失败 | open 2 分钟 |
| open | `open_until` 未到 | 本地拒绝，不访问上游 |
| open | `open_until` 已到 | half-open，只允许一次 probe |
| half-open | probe 成功 | closed，等待恢复为 2 分钟 |
| half-open | probe 失败 | 重新 open，等待翻倍，最多 30 分钟 |

只把 transport error 与 5xx 计为可重试失败。429 使用自己的 `retry_after` 暂停，不计断路失败；明确永久 400/403 直接禁用对应群或资源， 也不进入 half-open 探测。状态按故障域持久化，进程重启或内存项驱逐后恢复， 避免重启清空暂停期。

当前状态机不引入通用断路器库。只有出现并发 probe、滑动窗口统计或跨进程协调时， 才重新评估成熟实现。

故障域取键依据：

## 4. 群的两种模式

同一个机器人在两种群里的行为不一样，因为 Telegram 给的入口本身就是两条。 **这是群的属性，不是我们的设定项**，控制台必须显示当前是哪一种， 因为它决定了未通过验证的人此刻在群里还是群外。

控制台在群列表与首页各显示一次当前模式。处于第二种模式时给出一句说明， 并指出开启批准的位置，但**不替管理员改群设置**，那是他的群。

此外还有两类群直接跳过：机器人不在其中的群， 以及配置为只发消息不做验证的频道与群。频道、管理员、机器人账号本身也不触发验证。

## 5. 流程

五条主要流程。**每条都写明失败时倒向哪一边**，因为这决定陌生人进不进得来。

### 一、审批制的入群验证

```text
申请人            机器人                     数据库              群
   │                 │                          │                 │
   │─ 申请加入 ─────▶│                          │                 │
   │                 │─ 检查免验证来源 ────────▶│                 │
   │                 │  命中 → 直接批准，结束                        │
   │                 │                          │                 │
   │                 │─ 写入待验证记录 ────────▶│                 │
   │                 │  唯一索引冲突 → 重复到达，保持原挑战，结束      │
   │                 │                          │                 │
   │◀─ 按本群设定发出 │                          │                 │
   │   挑战：群内、私 │                          │                 │
   │   聊或两者      │                          │                 │
   │                 │  一处都没送达 → 群内说明未送达，到期自动拒绝        │
   │                 │─────────────────────────────── 合并入口消息 ▶│
   │                 │                          │                 │
   │─ 提交答案 ─────▶│                          │                 │
   │                 │─ 条件更新 pending→approved ▶│               │
   │                 │  影响 0 行 → 已被结算，返回成功                │
   │                 │─ 批准加群 ──────────────────────────────────▶│
   │                 │─ 删除挑战消息 ──────────────────────────────▶│
```

**发到哪几处是每群的设定，不是流程写死的。**默认两者都发： 群内那条让在场的人看得见有人在验证，私聊那条是申请人真正作答的地方。 机器人无法主动私聊没有交互过的人，**所以私聊送不到是常态而不是故障**， 群内那条这时就是唯一的通路。三种取值下「一处都没送达」的处理相同： 在群内说明未送达，到期自动拒绝，不当作通过。

### 二、先入群后限制

差别只在两端：开头多一步收回发言权限，结尾把批准换成恢复权限。 中间的挑战、判定、结算完全相同，**因为它们在同一个 Service 里，只有一份实现**。

```text
新成员            机器人                     数据库              群
   │─ 已进入群 ─────▶│                          │                 │
   │                 │─ 收回发言权限 ──────────────────────────────▶│
   │                 │─ 写入待验证记录 ────────▶│                 │
   │◀─ 按本群设定发出 │                          │                 │
   │─ 提交答案 ─────▶│                          │                 │
   │                 │─ 条件更新 ──────────────▶│                 │
   │                 │─ 恢复发言权限 ──────────────────────────────▶│
```

### 三、超时

不用进程内定时器。扫描器按到期时间领取记录，一次一批。

```text
每 N 秒
   │
   ├─ 条件更新：pending 且已到期 → expired，一次取一批
   │     领到的才处理。没领到说明别的实例先领了
   │
   ├─ 按该群配置处置：拒绝 · 移出 · 移出并封禁
   │
   └─ 删除挑战消息，写入操作记录
```

### 四、掉线恢复

机器人连不上 Telegram 期间，待验证记录仍在到期。 **掉线是我们的问题，不能记在申请人头上。**

- 心跳探测不通时**暂停结算**，不产生拒绝，也不记失败次数。
- 恢复后给一个完整的新窗口，并重新发出挑战。`epoch` 递增， 恢复前排队的旧定时器凭旧 `epoch` 结算会被拒绝。
- 重新发出前先确认这个人还在群里或申请还在，已经不在的直接清理，不重发。
- **积压的更新按时间丢弃。**收到的更新距当前超过一定时长， 直接判为过期并处置，不再生成题目，此时用户早已离开，出题没有意义。 这条取自既有的入群验证方案。

### 五、管理员在控制台处置

```text
浏览器            console                  verification        Telegram
   │─ 放行请求 ─────▶│                          │                 │
   │                 │─ 校验会话签名 ───────────│                 │
   │                 │─ 现查操作者是否仍是管理员 ──────────────────▶│
   │                 │  否 → 403，不执行                              │
   │                 │─ 现查被操作对象的身份 ──────────────────────▶│
   │                 │─────────────────────────▶│                 │
   │                 │                          │─ 条件更新 ──────│
   │                 │                          │─ 批准加群 ──────▶│
   │                 │                          │─ 写操作记录 ────│
```

`console` 只做认证、参数校验与调用。 「这个人该不该被放行」的判断在 `verification` 里， 与群内答题走的是同一个方法。

## 6. 数据模型

用 `dbutil`：一套代码同时支持 SQLite 与 PostgreSQL，统一写 `$1` 占位符， SQLite 执行前自动转 `?1`。迁移经 `embed.FS` 注册，版本表带兼容下限， **旧二进制连接新结构会被拒绝启动**。

```text
-- 群。没有全局默认，settings 只存与出厂默认不同的项。
CREATE TABLE chat (
    id         BIGINT PRIMARY KEY,
    title      TEXT   NOT NULL,
    left_at    BIGINT,          -- 非空表示 bot 已被移出，数据待清理
    settings   TEXT   NOT NULL DEFAULT '{}',
    settings_revision BIGINT NOT NULL DEFAULT 0  -- migrations/01-settings.sql；控制台写入靠它检冲突
);

CREATE TABLE challenge (
    id         TEXT   PRIMARY KEY,
    chat_id    BIGINT NOT NULL REFERENCES chat(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL,
    state      TEXT   NOT NULL,   -- pending|approved|declined|banned|expired|superseded
    kind       TEXT   NOT NULL,   -- rule|pow|captcha|membership（判定机制，非界面选项）
                                    -- membership 只用于「去关注再回来」这一类；
                                    -- 免验证在任何挑战之前求值，命中即跳过，不建记录
    payload    TEXT   NOT NULL,   -- 题面、诱饵、nonce、难度
    delivery   TEXT   NOT NULL,   -- 实际送到哪几处，各自的消息 id
    attempts   INTEGER NOT NULL DEFAULT 0,
    reason     TEXT,             -- 只在 declined 时非空：wrong_answer|rejected|external_unmet
    expires_at BIGINT NOT NULL,
    settled_at BIGINT,
    settled_by BIGINT,          -- 管理员结算时记录其 user_id
    epoch      INTEGER NOT NULL DEFAULT 0  -- 掉线恢复重发时递增
);

-- 挑战的外部副作用。首项动作与状态转换同一事务写入，后续动作由完成前项时写入。
CREATE TABLE pending_action (
    id           TEXT    PRIMARY KEY,
    challenge_id TEXT    NOT NULL REFERENCES challenge(id) ON DELETE CASCADE,
    kind         TEXT    NOT NULL,
    payload      TEXT    NOT NULL,
    state        TEXT    NOT NULL DEFAULT 'pending',
    attempts     INTEGER NOT NULL DEFAULT 0,
    next_try_at  BIGINT  NOT NULL,
    claim_owner  TEXT,
    claim_until  BIGINT,
    done_at      BIGINT,
    failed_at    BIGINT,
    last_error   TEXT
);
CREATE INDEX pending_action_due
    ON pending_action (next_try_at) WHERE state = 'pending';

-- Telegram 每个 bot token 只能有一个更新流。固定单行足以表达这一个资源。
CREATE TABLE update_poll_lease (
    singleton  INTEGER PRIMARY KEY CHECK (singleton = 1),
    holder     TEXT    NOT NULL,
    expires_at BIGINT  NOT NULL
);

-- 同一人在同一群同时只能有一条待验证记录。
-- 重复到达在数据库层被拒绝，不依赖内存中的已见集合。
CREATE UNIQUE INDEX challenge_open
    ON challenge (chat_id, user_id) WHERE state = 'pending';
CREATE INDEX challenge_due
    ON challenge (expires_at) WHERE state = 'pending';

-- 题库、自动回复、显示名黑名单、反垃圾共用一张表，用 collection 区分。
-- 条件类型只有一套，因此匹配与导出导入各只有一份实现。
CREATE TABLE rule (
    id         TEXT   PRIMARY KEY,
    chat_id    BIGINT NOT NULL REFERENCES chat(id) ON DELETE CASCADE,
    collection TEXT   NOT NULL,   -- challenge|autoreply|namefilter|antispam
    ordinal    INTEGER NOT NULL,
    enabled    BOOLEAN NOT NULL DEFAULT TRUE,
    definition TEXT   NOT NULL    -- 题面、条件、回复内容，三语
);

-- 以下四张表承载上一代那四份 JSON 状态。它们不在本节最初的设计里，
-- 是阶段三第一片换介质时按现有状态的实际形状定下来的。
CREATE TABLE verification_failure (
    chat_id BIGINT  NOT NULL REFERENCES chat(id) ON DELETE CASCADE,
    user_id BIGINT  NOT NULL,
    count   INTEGER NOT NULL,
    last_at BIGINT  NOT NULL,   -- 冷却窗口从这里算
    PRIMARY KEY (chat_id, user_id)
);

CREATE TABLE agent_tally (        -- 诱饵命中时对方自称的模型，只是统计
    model TEXT    PRIMARY KEY,
    count INTEGER NOT NULL
);

CREATE TABLE verification_runtime (  -- 单值运行状态：命中总数、上次在线时间
    key   TEXT   PRIMARY KEY,
    value BIGINT NOT NULL
);

CREATE TABLE warning_counter (    -- 群与用户双键，有界驱逐仍在内存里做
    chat_id BIGINT  NOT NULL REFERENCES chat(id) ON DELETE CASCADE,
    user_id BIGINT  NOT NULL,
    count   INTEGER NOT NULL,
    PRIMARY KEY (chat_id, user_id)
);
```

### 动作与更新拉取的持久边界

| 字段 | 理由 |
|---|---|
| `pending_action.id` | 动作的稳定身份；完成、重试和失败都按此条件更新 |
| `challenge_id` | 关联触发动作的挑战；挑战删除时级联删除不再有意义的动作 |
| `kind`、`payload` | 执行器根据种类反序列化固定 JSON 负载；Store 不解释业务参数 |
| `state`、`attempts`、`next_try_at` | 表达待执行、完成、失败和下一次可执行时间；`pending_action_due` 只索引可领取行 |
| `claim_owner`、`claim_until` | 一个 worker 在 lease 内独占动作；进程崩溃后 lease 到期即可重领 |
| `done_at`、`failed_at`、`last_error` | 保留终态时刻和最后错误，区别成功、永久失败与仍可重试 |
| `update_poll_lease.singleton` | 固定为 `1`，因为系统只有一个 Telegram 更新流资源，不引入无用的通用命名空间 |
| `update_poll_lease.holder`、`expires_at` | 标识当前进程及其失效时间；续租和释放必须同时匹配 holder，过期后才允许接管 |

挑战状态转换与首项 `pending_action` 在同一个数据库事务提交。 动作执行、Telegram 调用和后续动作写入发生在事务外；完成当前动作与写入后续动作仍在同一事务。 临时错误按退避重试，永久或耗尽错误写为 `failed`。自动清理只删除群内挑战消息， 私聊题目和结果不会被自动删除。

### 这四张表是搬来的，不是设计出来的

本节最初只画了 `chat`、`challenge` 和 `rule`。 真去换介质时，上一代那四份 JSON 还得有地方放：失败计数与冷却、 诱饵命中的模型计数、两个单值运行状态、以及警告计数。

`agent_tally` 与 `warning_counter` 的最终去处不在这里： 前者是统计、该归 `status`，后者归 `moderate` 自己的那一层。 放在这里是因为阶段三第一片只换介质、不动归属， 搬家和拆包一起做就分不清失败来自哪一件。

还有一张 `version` 表，由 `dbutil` 自己建和维护， 记结构版本与兼容下限。**旧二进制连新库被拒绝**就是靠它： 迁移不声明兼容下限，`dbutil` 便把下限设成目标版本， 更老的程序收到 `ErrUnsupportedDatabaseVersion` 并拒绝启动。

### 为什么退回要分原因

`state` 已经把超时和拒绝分成 `expired` 与 `declined`。 再加 `reason`，是因为 `declined` 里混着三件不同的事： 答错、答对却触发否决条件、外部条件查不到。三者对应的处置和文案都不同， 合成一个词就统计不出来。

上一代把这三种写进了日志那一行（`internal/verify/service.go:2765`）， 却没有落进任何一张表，所以控制台无从查起，也无从统计。 它另有一处并得更死：没关注必关频道的人被记成 `timeout` （`internal/verify/state.go:682`）， 和从没回来答的人写成同一个词；这一类正是 `external_unmet` 要分出来的。 这一列存在的目的，就是让统计屏那行「各验证方式拦截量」有数可依。 见设计文档「上一代七天里的真实分布」。

### 写入顺序

顺序固定，颠倒会留下没有主的挑战。这是扫描中发现的一类实际缺陷。

```text
1. INSERT challenge (state='pending')      提交
2. SendChallenge()                        对外可见
3. INSERT challenge_message (...)         回填消息标识
```

## 7. 状态机与动作执行

每一次转换都是一条带条件的更新。**影响 0 行不是错误**， 表示已被其他路径结算，调用方据此返回成功。 扫描发现的「刚通过验证的人被进群后验证再抓一次」正是缺少这一层导致的。

```text
UPDATE challenge
   SET state = $2, settled_at = $3, settled_by = $4
 WHERE id = $1 AND state = 'pending';
```

### 超时与结算不依赖进程内定时器

扫描器启动时立即执行，之后每 5 秒按 `challenge_due` 领取到期记录。 一次领取最多 32 条，并把记录的 deadline 前推 30 秒作为 lease。时间以 Unix 秒保存； 因此实际结算可比 deadline 晚至一个扫描间隔与一次调度，但进程重启和多实例不会丢失或重复结算。

```text
1. ClaimExpired(now, claimUntil, 32)  条件领取 due challenge
2. TransitionChallenge(...)             条件变更 state，插入首项 pending_action；同一事务提交
3. ClaimActions(...)                    worker 以 30 秒 lease 领取 ready action
4. Telegram 调用                        成功则完成动作并写后续动作；失败则重试或记录 failed
```

动作执行使用指数退避，受 Telegram 的 `retry_after` 约束时采用更晚的时间。 永久错误与重试耗尽写入终态，而不是静默丢弃。自动删除只针对群内挑战消息；私聊中的题目和结果保留。

## 8. 规则引擎

一套条件类型，三个作用对象：验证答案、消息文本、显示名与个人简介。 **纯函数包，无副作用**，因此控制台的试答直接调用它，与线上判定不可能不一致。

### 先归一化，再匹配

真实垃圾信息不会把关键词原样写出来。归一化只用于匹配，不改动原文； 界面展示、消息删除、操作记录用的都是原始内容。

```text
// NFKC 必须最先执行，否则全角分隔符无法被后续步骤识别。
func Normalize(in string) string {
    s := norm.NFKC.String(in)      // 全角转半角，兼容字符归一
    s = dropInvisible(s)           // U+200B-200F, U+2060-2064, U+FEFF, 变体选择器
    s = foldCJKSeparators(s)       // 折叠汉字之间的 · * - _ . 与多余空白
    return strings.ToLower(s)
}
```

### 条件类型是封闭集合

加类型要改代码，加规则不用。这条边界保证规则可以导出成文件、 可以被不信任地导入，也可以被界面完整表达。

| 类型 | 参数 |
|---|---|
| equals | 值、是否忽略大小写 |
| one_of | 值的集合 |
| contains | 子串、是否要求井号前缀 |
| regex | 正则，带长度与执行时长上限 |
| number_range | 上下界 |
| version_range | 上下界、归一化方式 |
| membership | 群或频道、查询失败时的方向 |

### 配置包是数据，不是代码

规则集合可以打包分发。**包的内容是封闭集合，且不含任何可执行形式。** 这不是保守，是责任边界：数据包最坏的情况是内容不合适，可以停用； 代码插件最坏的情况是任意代码运行在别人的群里，那时隔离、审核与责任都要重做。

| 可以装 | 不可以装 |
|---|---|
| 规则集合的三种：题库、自动回复、名单 | 任何形式的可执行内容 |
| 反垃圾规则与分值 | 密钥与凭据 |
| 订阅源与过滤条件 | 群标识与用户标识 |
| 消息文案，三语 | 统计数据 |
| 已有能力的开关建议值 | 密钥、凭据与任何指向外部的写操作 |
| **声明式命令**：取数、挑字段、渲染，见下一节 | **需要写代码才能表达的命令。**判据是它能不能用取数加模板说清楚， 说不清楚的就是代码，包里装不了 |

### 声明式命令：不引入运行时

包可以注册命令，但只能是**取数、挑字段、渲染**这一类。 现有命令的形状支持这个划分：`repology`、`kernel`、 `manpage`、`cve` 各一百余行，都是这个形状； 而 `packages` 一千五百余行，做的是版本自然排序、 多源合并与十几处条件渲染，声明不出来。

**因此不需要沙箱、脚本引擎或 WASM。**执行一条声明式命令用的全是既有部件： 有界的取数通道、响应解析、模板渲染。**没有一行使用者提供的代码被执行。**

| 控制点 | 做法 |
|---|---|
| 出网 | 只允许 https；走机器人自己的取数通道， 沿用既有的并发上限、排队上限、超时与响应体积上限 |
| 凭据 | 模板无法访问任何密钥。声明里也不允许出现请求头形式的凭据 |
| 名字冲突 | 与内置命令或已装命令同名时整份拒绝，不做静默改名 |
| 失败语义 | 没找到与上游故障分开表达。把故障说成没找到会让人以为查错了 |
| 缓存 | 按声明的时长缓存，失败使用较短的负缓存，避免故障期反复打上游 |

**不发明表达式语言。**字段提取限于按路径取值，渲染限于占位替换。 一旦开始支持条件、循环与运算，就等于在维护一门小语言， 而它会以「再加一个功能」的形式长大，最终没有人愿意调试它。 需要逻辑的命令写 Go，走合并请求。

### 导入路径与手工导入共用一份实现

包的导入、单份规则的导入、`provisioning` 目录的声明式加载， **走同一份解析与校验**。三条路径各写一份校验，迟早有一条宽一些， 而宽的那条就是被利用的那条。

- 正则受同样的长度与执行时长上限，**不因为来自包就放宽**。
- 每条落库的规则记录来源包与版本，因此可以整包停用或移除。
- **不自动更新。**上游改版只产生一条提示，是否装入由人决定。 自动更新等于把改变他人群规则的权力交给包的作者。
- 校验失败整份拒绝并指出第几条，不做部分导入。

包的分发暂不做索引与市场：在有若干真正可用的包之前， 索引只是一个空货架加一批要长期维护的机制。到「找不到合适的包」成为真问题时再设计， 届时要解决的是署名、版本与举报。

### 富文本按实体存，不按标记语言存

回复内容、题面、进出提示都可以带格式。**存储与传输的是实体数组**： 文字一份，样式一份，样式记录起始偏移与长度。

| 做法 | 代价 |
|---|---|
| 标记语言加解析模式 | 要转义。用户输入里的一个尖括号或下划线就能让整条消息发送失败， 或者产生错位的格式。转义与反转义必须两端一致，容易长期不一致 |
| **实体数组** | 需要一个编辑器把选区转成偏移，但**复制到的文字等于屏幕上的文字**， 且与平台的表示一致，不必自己维护一套转义规则 |

实体类型是封闭集合：加粗、斜体、下划线、删除线、剧透、行内代码、代码块、 引用、可折叠引用、带文字的链接。

**两类明确排除。**自定义表情要求发送方持有会员资格， 换一台机器部署即无法发送，规则文件因此不可移植； 可点击的用户提及让自动回复能点名任何人，是现成的骚扰通道。

偏移按 UTF-16 码元计，与平台一致。 **不要按字节或按 rune 计**：中文与表情会算错位置， 而错位的实体不会报错，只会把样式套在别的字上。

### 链接预览与按钮

- **链接预览用新的选项对象**，三态：关、小图、大图， 另可选显示在文字上方。**默认关**：自动回复一天触发多次， 每次拖一张大图会让群难以阅读。
- **只发链接按钮。**不使用回调按钮：回调需要一套编码协议、 一份状态和一条鉴权路径，而自动回复只负责发言。 这条边界一旦松开，它会长成一个任何人都能在群内编写的脚本引擎。
- 保存前校验每个网址：协议、可解析、按钮文字非空。 **不允许存进一个点了没反应的按钮。**

渲染归 `telegram/tgfmt`： 业务包返回结构化结果与实体，由它转成平台调用。 **业务包不拼接标记，也不直接构造键盘。**

### 结构信号

关键词表落后于改词，**规避手法本身是更稳的信号**。以下不看内容只看形态，累加计分。

| 信号 | 取自 | 说明 |
|---|---|---|
| 词中间的不可见字符 | 归一化前后的差异 | 归一化删掉了字符即说明原文嵌了隐藏字符。**正常输入不产生这种差异** |
| 私有邀请链接数量 | Telegram 消息实体 | 从平台解析好的实体读取，不使用自写的正则扫描全文 |
| 提及的用户名数量 | Telegram 消息实体 | 同上 |
| 链接总数 | Telegram 消息实体 | 同上 |
| 入群时长 | 成员记录 | 新成员首条含链接的权重高于长期成员 |

实体由平台解析，不受显示层的字符插入影响， 因此比自写的全文扫描可靠。

处置分四档：仅记录、删除、删除并禁言、删除并移出。每次自动处置写入操作记录且可撤销。 **默认最低档**，先观察命中情况再提档。

线上样本进 `testdata/` 作测试用例，不进出厂默认规则： 默认规则要对所有群成立，具体广告词只对特定时期的特定骗局成立。

## 9. 权限

两层，各解决一个问题。**只有其中一层都不成立。**

### 定时同步

周期性调用 `getChatAdministrators`， 把结果写进本地权限表。控制台的群列表、导航过滤、界面显示都读这份缓存。 **解决的是「不能为了画一个列表打十次接口」。**

### 写前现查

每次敏感写入前用 `getChatMember` 单独查一次， 只接受群主与管理员。**解决的是「缓存里他还是管理员，实际已经被撤了」。** 被操作对象的身份同样现查。

| 用途 | 数据来源 | 过期怎么办 |
|---|---|---|
| 能看到哪些群 | 本地权限表 | 差一个周期。多显示一个群没有危害，因为写入时会被拒绝 |
| 导航项是否出现 | 本地权限表 | 同上。**加载完成前按无权限处理**，先显示再收回更糟 |
| 能否执行写入 | 现查 | 不允许过期。查不到即拒绝 |
| 能否处置某个人 | 现查被操作对象 | 不允许过期。刚被提升为管理员的人不应被处置 |

### 同步的触发时机

- 固定周期，按群错开，避免所有群在同一秒发起请求。
- 收到该群的管理员变动更新时立即同步这一个群。
- 管理员打开该群的控制台时同步这一个群，因为他接下来大概率要操作。
- **同步失败不清空已有记录。**清空会让所有人瞬间失去访问， 而失败通常只是一次网络抖动。记录失败时间，在诊断屏显示。

### 运维与群管理员是两回事

群管理员管自己的群，运维管这台机器。**两者的边界要写死，不能混。**

| 能力 | 群管理员 | 运维 |
|---|---|---|
| 本群的配置与队列 | 可 | 不可，除非他也是该群管理员 |
| 导出本群数据 | 可 | 同上 |
| 删除本群数据 | 可 | 同上 |
| 整机诊断与指标 | 不可 | 可 |
| 查看其他群的内容 | **不可** | **不可** |

**运维不能读别人群里的内容。**他能看到整机的计数与延迟， 看不到具体某个群的队列与文案。这条要落在接口上，不是靠界面隐藏。

自托管的人同时是运维和管理员，两个身份重合， 但代码里仍然是两套判断。这样公开实例与自托管共用同一份实现。

### 同步的成本

一百个群定时同步就是一百次接口调用。**按群错开并延长周期**， 活跃群同步更频繁，长期没有申请的群降低频率。 收到管理员变动的更新时立即同步该群，这比定时更准。

### 写权限怎么定

不是所有管理员都该能改配置。**以 Telegram 自己的权限位为准，不另立一套角色。** 具备限制成员权限的管理员可写，其余只读。群主始终可写。 这样管理员在 Telegram 里被降权，控制台的权限跟着降，不需要第二处维护。

既有的入群验证方案在这里踩过一个坑： 群主状态存了一个字段，能力判断却读另一个字段，两者不一致。 **权限只有一个来源**，不存两份。

## 10. 配置

配置分三类，各有独立的所有者和持久位置。进程配置由运维在启动前提供； 每群配置由该群管理员修改；声明式资源由自托管者纳入版本控制。

| 类型 | 谁改 | 放在哪 | 例 |
|---|---|---|---|
| 进程配置 | 运维，很少改 | `config.json` 与环境变量 | 数据库连接、Bot API 地址、日志、外部服务令牌 |
| 每群配置 | 该群管理员，随时改 | `chat.settings` 与 `chat.settings_revision` | 验证方式、超时、封禁时长、反垃圾规则 |
| 声明式资源 | 自托管者 | `provisioning/` | 题库、自动回复、订阅源 |

### 设置实现：默认值、加载与数据库存取分开

```text
internal/settings/
├── defaults.yaml               go:embed，完整注释与全部出厂默认值
├── defaults.go                 解析默认值，展开文件管理值，保留来源
├── config.go                   进程配置类型、归一化与严格校验
├── load.go                     configupgrade 白名单与四条升级路径
└── store.go                    每群读取、稀疏覆盖、revision 与快照

internal/database/settings.go   chat.settings 的 Repository
migrations/01-settings.sql      settings_revision
```

- **没有全局默认记录，也没有控制群。**新群从内嵌 factory 取得默认值； 用户文件中的旧式顶层值只展开到文件列出的群，不会传播给后来注册的群。
- **进程级凭据不进入每群设置。**`BOT_TOKEN`、 `VT_DATABASE_TYPE`、`VT_DATABASE_URI` 与 `TELEGRAM_API_URL` 在进程装配边界读取。未认领时，进程随机生成 名为 `SETUP_TOKEN` 的认领口令；部署者不能指定。仅将该口令的哈希保存到 `STATE_DIRECTORY/claim.json`；认领后，Bot token 以 `0600` 权限保存到同一文件，并删除该哈希。
- **设置升级由 configupgrade 处理。**加载器以当前结构为模板， 只复制白名单字段；未知字段和废弃字段不进入规范表示。升级规则不按版本串联。
- **所有当前字段都必须同时进入默认文件和复制规则。**反射测试核对 `GroupOverrides`，防止新增字段只在一种加载路径生效。

### 每群配置：空覆盖继承 factory

`chat.settings` 只保存稀疏覆盖。空对象表示完整继承； 恢复默认就是删除对应字段，不写一份默认值副本。两个群的记录、revision 与快照互不影响。

**读取接口同时返回值与来源。**`Store.Settings(chatID)` 返回不可变 `GroupView`；每个 getter 返回 `Setting[T]{Value, Source}`。 来源只有三种：

| 来源 | 含义 | 恢复方式 |
|---|---|---|
| factory default | `defaults.yaml` 的出厂值 | 无须恢复 |
| user file | 启动文件为该群管理的值 | 修改文件并重启 |
| chat override | `chat.settings` 的运行时覆盖 | 删除该稀疏字段 |

`Store.Update(chatID, expectedRevision, overrides)` 接收完整稀疏记录。 Store 先解析整份有效设置并执行值域校验，再以 compare-and-swap 写数据库。 revision 冲突、校验失败或数据库写入失败都不发布新快照。

### 声明式资源：可由文件管理的部分

题库、自动回复、订阅源既可以在控制台里改，也可以放进 `provisioning/` 由文件管理，启动时应用。 这让自托管者能把这些内容纳入版本控制，也让一套配置可以复制到另一台。

- **与控制台导入是同一份实现、同一套校验。**不为文件另写一条路径。
- 由文件管理的资源在控制台里**只读并标出来源**，避免有人在界面上改完， 下次重启被文件覆盖却不知道原因。
- 文件解析失败时拒绝启动并指出是哪个文件第几条，不做部分应用。

## 11. 接口

### 两步，缺一不可

#### 一、身份

**两类使用者，两个入口，都不用密码。**

| 谁 | 怎么进 | 为什么是这个 |
|---|---|---|
| 群管理员 | Telegram 里的 Mini App。每次请求带 `initData`， 校验 HMAC 与签发时间，记录已用过的签名，有效期内不接受第二次 | 他要管的群在 Telegram 里，人也在。**每次请求都重新证明一次**， 比一张长期票据强 |
| 运维 | 机器人发一条一次性链接，浏览器打开即换成会话。 **整条链与 Telegram 的登录服务无关** | 他要看的是诊断、版本、证书续期，**而这些恰恰在出事时才看**。 那时 Telegram 可能正好不通 |

**不做 Telegram Login 的 OIDC。**它存在的理由是「在普通浏览器里打开控制台」， 但真正需要普通浏览器的是运维，而 OIDC 在最需要它的那一刻依赖 Telegram 可达。 一次性链接换会话这套机制安装时已经有了，复用它，少一个协议、少一个回调、 少一处要配的凭据。代价写清楚：**群管理员不能在普通浏览器里打开控制台**， 桌面版 Telegram 里可以。

policr-mini 独立走到了同一形状： 群管理员那一面走 Mini App 校验， `lib/policr_mini_web/console_v2/tma_auth.ex`； 管理面走与 Telegram 无关的令牌 cookie， `lib/policr_mini_web/admin_v2/token_auth.ex`。

#### 二、授权

**不把权限镜像进库。**每次敏感写入前用来访者的数字 ID 调用本地 Bot API 的 `getChatMember`，只接受 `creator` 与 `administrator`。会话最长 8 小时，非写入路径每 60 秒复查。 **被操作对象的管理员身份同样现查，不使用缓存。**

policr-mini 选了另一条：把 Telegram 的权限镜像进 `permissions` 表，`lib/policr_mini/schema/permission.ex`， 字段里既有从 Telegram 抄来的 `tg_is_owner`、 `tg_can_restrict_members`，也有它自己的 `readable`、 `writable`。**两种代价都摆出来**：镜像的好处是快、离线也能判， 坏处是会过期：在 Telegram 里被撤职的人，直到下一次同步之前仍然进得来， 它为此专门有一条同步链。我们选现查，代价是每次判定多一趟往返， 在我们的部署位置上约半秒；换来的是**没有过期窗口，也不需要同步机制**。

`initData` 只证明来访者是谁，不证明他能管这个群。**两步不可互相替代。**

**运维与群管理员是两类主体，不是同一类的两个档位。** 运维看这台机器：诊断、版本、升级、证书。群管理员看他自己的群。 **运维不因为是运维就能读某个群的数据**，除非他本来就是那个群的管理员。 界面上不做「简单版与专业版」：两者看到的差别来自服务端的授权结果，不是模式开关。

### 路由

| 路径 | 说明 |
|---|---|
| GET /livez | 进程事件循环存活即 200，不探测依赖，避免依赖抖动引发重启风暴 |
| GET /readyz | 配置校验完成、数据库已迁移、Telegram 通道建立才 200 |
| POST /api/session | 校验 `initData`，签发群管理员会话 |
| GET /api/instance | 把这台执行个体的机器人用户名交给还没有会话的浏览器。入口屏要说清该去开哪个机器人，而那个名字每台部署都不同，编进前端包里就会把别人的部署名字端给所有人。未被认领时返回空字符串，入口屏据此改说「还没有被认领」，不提任何机器人。机器人用户名在 Telegram 里本来就是公开的，因此这条不需要会话 |
| GET /api/session | 把当前会话的 CSRF 令牌交给已经持有 Cookie 的浏览器。**运维那条路需要它**：一次性链接只写 Cookie 再跳转，令牌没有别的出口，于是那种会话能读却不能写。签发令牌只在这一处，不另设可读 Cookie |
| GET /enter/{token} | 运维入口。机器人发的一次性链接落在这里，换成会话后重定向。**令牌用过即失效**，整条链与 Telegram 的登录服务无关 |
| GET /api/chats | 群与频道屏。该管理员可管理的群，由 getChatMember 求交集得出，不存租户表 |
| GET /api/chats/{id}/overview | 首页四层所需数据，一次返回 |
| GET /api/chats/{id}/queue | 等待队列 |
| POST /api/chats/{id}/queue/{cid} | 人工结算一条：放行、拒绝、封禁。 **带当前状态做条件更新**，已被超时或他人结算过的返回冲突，不覆盖 |
| PATCH /api/chats/{id} | 这个群开不开验证、是不是只发消息不验证 |
| GET /api/chats/{id}/settings | 验证方式、管理与处罚、功能三屏共用。带每项来源：出厂默认、本群设定或由文件管理 |
| PATCH /api/chats/{id}/settings | 只提交改动过的字段，带版本号做冲突检测 |
| GET · PUT /api/chats/{id}/rules | 题库、消息与文案、免验证来源三屏共用，`collection` 区分题库、自动回复、显示名黑名单与反垃圾。PUT 整份替换用于导入 |
| POST /api/chats/{id}/rules/test | 试答，调用线上同一份判定代码 |
| GET /api/chats/{id}/audit | 操作记录 |
| POST /api/chats/{id}/audit/{aid}/undo | 撤销一条。 只有可逆的才给这个入口，删掉的消息回不来 |
| GET · PUT /api/chats/{id}/feeds | 订阅推送。PUT 整份替换用于导入 |
| GET /api/chats/{id}/stats | 统计屏。区间与粒度由查询参数给，服务端聚合，不把明细发给前端 |
| GET /api/chats/{id}/packages | 已装的配置包与可装的包 |
| POST /api/chats/{id}/packages | 装一个包。先返回它将改动哪些项，确认后才落库 |
| GET · PATCH /api/me/preferences | 看的人自己的偏好，不属于任何群 |
| GET /api/status | 诊断屏与版本屏。健康、当前版本、设置持久化、Bot API 探测、回退条件读数和宿主替换状态。**只有运维可见** |
| GET /api/status/release | 运维明确操作后，按需读取固定 GitHub 仓库的最新正式发布、变更说明与目标结构清单；失败不影响本地状态。同上，只有运维 |
| GET /api/process/settings | 实例级设置的只读视图，每项带来源（出厂默认 / 用户文件 / 群覆盖）。**只有运维可见**，没有写入路由 |
| POST /api/status/upgrade | 发起升级，只写目标版本，执行在宿主侧。同上，只有运维 |
| GET /verify/{token} | **长期存在的唯一公开面。**令牌一次性、带签名与有效期，不复用管理会话 |
| GET · POST /setup/{token} | **只在认领之前存在。**安装脚本打印的一次性链接落在这里， 用来填写 Bot token，再给 Telegram 部署者一次性绑定链接；网页不接收或显示绑定口令。 **认领成功后这条路由不再注册**，之后任何人访问都是 404，不是隐藏 |

**这张表是穷举的。**界面上多一个屏，这里就要多一行； 没有对应行的屏是还没设计，不是省略。两份文档的一致性照这条核对： 设计文档里的每一个屏，都要能在这里找到它取数与写入的那一行。

### 错误

三层：Go 侧 sentinel 与 `%w` 包装；稳定的 API 错误码 `VERIFICATION_NOT_FOUND`、`INVALID_STATE_TRANSITION`、 `TELEGRAM_RATE_LIMITED`、`FORBIDDEN`、`CONFLICT`； 响应只带公开消息与 request id，内部原因留在日志。

**契约由单一 OpenAPI 文件生成两端类型，流水线中重新生成并比对差异。** 两端各自编译通过不代表契约同步。

## 12. 前端

**Vite + React + React Router**，组件层用 shadcn/ui（`base-nova`、 `neutral`、CSS variables）+ Tailwind v4 + lucide， 与既有生产项目同一套设计系统。构建产物 `dist/` 经 `go:embed` 进入二进制。

### 为什么不用 Next.js

Next.js 的价值集中在服务端：RSC、增量再生成、图片优化、middleware。 本控制台位于登录之后，不需要搜索引擎收录，不需要首屏服务端渲染。 采用静态导出会把这些能力全部关闭，剩下的是一个 React 构建工具加文件路由， 同时要接受两项限制：路由不能使用路径参数；`go:embed` 必须写 `all:` 前缀，否则下划线开头的目录被静默排除，构建通过而页面白屏。

核实过既有项目的实际用法：没有 middleware、没有 server actions、 没有 `cookies()` 与 `headers()`、`next/image` 零引用， 页面几乎全部是客户端组件。它虽运行在 Node 进程上，该进程不承担服务端职责。

### 前后端分离

**分离是架构上的，恒定成立；内嵌只是两种部署方式之一。** 两者不冲突：前端是一个纯 SPA，后端是一组 JSON 接口， 之间只有 OpenAPI 这一份契约。没有服务端渲染的 HTML， 没有模板与 Go 结构体的耦合，前端可以对着假数据独立开发。

| 部署方式 | 怎么做 | 代价 |
|---|---|---|
| 内嵌 默认 | 构建产物 `go:embed` 进二进制，后端在 `/` 提供静态文件，接口在 `/api` | 无。前后端版本天然对齐，一条命令部署 |
| 分开部署 | 前端放 CDN 或 nginx，后端只提供接口。 前端用 `VITE_API_BASE` 指向后端地址 | 需要处理跨域与版本协商。**前后端可能不同步**， 因此接口要返回自身版本，前端不匹配时提示刷新 |

后端不假设静态文件一定存在：没有内嵌产物时它就是一个纯接口服务，正常启动。 这样两种方式共用同一个二进制，切换不需要改代码。

开发时前端运行自己的开发服务器，把 `/api` 代理到本机后端或一份假数据。**前端不依赖后端即可运行**， 这是分离是否真正成立的判断标准。

### 迁移面有多大

设计系统与框架无关，因此风格完全一致，不是近似。

| 项 | 处数 | 换成 |
|---|---|---|
| `components/ui` 全部 25 个组件 | 0 | 原样使用，其中没有一个引用 Next |
| next-intl | 58 | react-i18next。key 结构与回落约定不变 |
| next/navigation | 19 | react-router 的 useLocation 与 useNavigate |
| next/link | 18 | react-router 的 Link |
| next/dynamic | 7 | React.lazy 加 Suspense |
| next-themes | 1 | 本身是纯 React 库，直接沿用 |

样式层一行不改：oklch token、`.dark` 变体、 `[data-slot]` 覆盖、状态色、圆角推导全部原样。 `components.json` 只需去掉 `rsc`。字体从 npm 包换为自托管。

收益：动态路由可直接使用 `/groups/:id`；产物为 `dist/assets/`，不含下划线目录；构建时间与依赖数量明显下降。

### 目录：按屏组织，不按文件类型组织

形状取自 Grafana：**实现、故事、测试同目录，同寿命**； 页面按功能划分目录，只从目录根导出对外入口。

```text
app/                     壳：路由表、布局、Provider、错误边界
features/                一个屏一个目录
├── queue/
│   ├── QueueScreen.tsx
│   ├── QueueTable.tsx
│   ├── QueueTable.test.tsx
│   ├── api.ts           只放这一屏的请求
│   └── index.ts         对外只导出这一个文件声明的内容
├── verification/
├── rules/
├── messages/
└── stats/
components/
├── ui/                  shadcn 生成物，不手改
└── …                    自写共用件：StatusBadge、ConfirmDialog、PageHeader、EmptyState
lib/
├── api/                 由 OpenAPI 生成，不手写
├── auth.ts  i18n.ts  utils.ts
styles/globals.css       token 层，全项目唯一可以写颜色的地方
```

| 规则 | 为什么 |
|---|---|
| features 之间不互相引用 | 需要共用就上移到 `components` 或 `lib`。 横向引用会使删除一个屏需要先做一次全项目排查 |
| 只从 `index.ts` 导出 | 目录内部随便改，对外只有一个面。这条要有检查，否则第一天就会被绕过 |
| `components/ui` 不手改 | 它是生成物，升级会覆盖。需要改行为时在外面包一层 |
| 测试与组件同目录 | 删组件时测试跟着删，不会留下针对已删除组件的测试 |

### 状态分两种，不要混

| 种类 | 放哪 | 例 |
|---|---|---|
| 服务端状态 | 查询库缓存，带失效与重取 | 队列、设置、统计 |
| 界面状态 | 组件内部，或 URL | 展开的分组、筛选条件、当前群 |

**不要把服务端数据塞进全局存储。**它一旦进入全局存储，就需要自行处理失效、 重取、并发请求与错误，而这些查询库已经做完了。

筛选条件与当前群放 URL，**这样一个链接可以直接发给同事**， 刷新也不丢。存在组件里会两样都做不到。

### 一个屏崩了不带崩后台

每个 feature 外面包一层错误边界，出错时只有这一屏显示错误与重试， 导航与其他屏照常。这与后端「一个群出问题不影响其他群」是同一条原则的两侧。

按 feature 懒加载，首屏只下载首页需要的部分。 十几个屏全部打进一个包会让第一次打开明显变慢。

### 文档拆成两份，不是九份

Grafana 把前端指南拆成九份，**按维护者任务分**，不按组件分： 全局代码约定、样式实现手法、token 语义、可访问性各一份， 单个组件的用法放在组件旁边的文档里。

我们只有一个维护者，按同样的维度缩小到两份：

| 文件 | 管什么 |
|---|---|
| [设计语言](design.html) | token 语义、视觉规则、组件契约、文案规则 |
| `web/README.md` | 目录约定、命名、导出边界、状态放哪、怎么加一个屏 |

**暂不引入组件展示工具。**十几个屏、一个维护者，它的收益不抵维护成本。 改用一条只在开发构建里存在的路由，把所有共用件排在一页上， 同一份组件、同一套 token。规模上来再换。

### 三种约束载体各管各的

这条同样取自 Grafana，它把约束明确分给三处：

| 载体 | 负责 | 我们的落点 |
|---|---|---|
| 文档 | 解释语义、选择条件、为什么这样选。工具推断不出来的部分 | 设计语言与本页 |
| 门禁 | 可判定的不变量 | 类型检查、lint、构建、生成物无漂移、可访问性零违规 |
| 目录结构 | 让实现、测试、文档同寿命，新文件自动被工具扫到 | 共置与 `index.ts` 边界 |

**文档中除稳定的命令名之外不写具体标识。**Grafana 的可访问性指南至今还在描述一个 已经不存在的检查作业，把贡献者引向不存在的流程。 因此这里只引用稳定的命令名，不复制作业名与阈值；改门禁时同一次提交里改文档。

### 前后端契约

一份 OpenAPI 文件生成 Go 服务端接口与 TypeScript 客户端类型， 门禁中重新生成并比对差异，理由见接口一节。

`lib/api` 全部是生成物，不手写。 手写一个请求函数很快，但它会与契约无声分叉，而且分叉时两边都是绿的。

## 13. 稳定性

公开服务的核心要求：**一个群出问题，其他群不受影响；进程重启不丢状态。** 这一节把隔离、兜底、并发、内存、备份收在一起，因为它们服务同一个目标。

### 并发与内存

## 14. 可观测与维护

### 日志

结构化字段固定为 `component`、`request_id`、`update_id`、 `chat_id`、`user_id`、`challenge_id`、 `state_from`、`state_to`。生产输出 JSON，开发输出可读格式。

**令牌、验证答案、会话凭据、完整的授权头不得进入日志。** 这条要有一个写入前的过滤器兜底，不能只靠调用点自觉。

访问日志记录方法、路径、耗时、状态码与响应大小， 按状态码分级。**探针成功不写日志**，否则日志里九成是健康检查。

### 健康检查

| 探针 | 检查什么 | 为什么这样分 |
|---|---|---|
| /livez | 进程事件循环仍在运行 | **不探测数据库与 Telegram。**依赖短暂故障不应触发重启， 重启解决不了外部依赖的问题，只会放大故障 |
| /readyz | 配置已校验、数据库已迁移、Telegram 通道已建立 | 未就绪时不接流量，但进程继续存活等待依赖恢复 |

### 指标

只保留能驱动告警或容量决策的。**不把群标识放进标签**，那是高基数字段， 群多了会把指标系统撑爆；按群的数据在诊断屏用查询得出。

| 指标 | 用来判断 |
|---|---|
| 待验证数量 | 队列是否积压 |
| 各终态计数 | 通过率变化 |
| 状态转换失败数 | 并发竞争是否异常 |
| 待执行动作积压与最长等待 | Telegram 调用是否卡住 |
| 429 计数 | 是否触及速率上限 |
| 更新处理延迟 | 是否跟得上 |

**通过率是最有价值的一个。**线上当前为 2/71，说明来访基本是批量账号。 若突然升至八成，说明验证被绕过，或者正在拦截真实用户，两者都要立即排查。

### 让架构不腐化

口头约定会失效，因此每条边界都要有机器能查的手段。

| 边界 | 怎么查 |
|---|---|
| 验证包不引用 telego | 检查该包的导入列表，出现即失败 |
| 规则包无副作用 | 禁止导入数据库与网络相关的包 |
| 接口实现完整 | 编译期断言写在实现旁边 |
| 两个构建标签都能编 | 门禁同时跑两次 |
| 文案是书面语 | 中文检查脚本 |
| 没有针对特定群的分支 | 检索固定的群标识与主群一类的命名 |

**新增一条规则时，同时给出它失败时会变红的检查。** 没有检查的规则只是偏好，半年后没有人记得。

第 02 节的允许与禁止矩阵是机器检查的输入，不只是评审提示。 模块外边界由 Go 的 `internal` 目录强制；模块内边界由依赖 linter 按目录和完整 module import path 拒绝。只检查 import cycle 不够， 因为无环的错误依赖仍能编译。

| 边界 | 机器约束 |
|---|---|
| 模块外部不得依赖实现 | 运行包全部位于 `internal/`， 由 Go 工具链拒绝模块外导入 |
| 模块内部不得越层 | 依赖边界规则逐目录拒绝第 02 节矩阵中的禁止边 |
| `rules` 保持纯函数 | 导入检查拒绝数据库、网络与具体协议包 |
| 接口实现必须完整 | 编译期接口断言随接口签名一起编译 |
| 所有构建配置都有效 | 每套受支持的构建标签分别执行静态检查、构建与测试 |
| 不出现单群特例 | 静态检查拒绝固定群标识和主群一类的业务命名 |

新增边界时必须同时增加一个能因违规而失败的检查，并用故意违规证明失败发生在目标断言。 检查的稳定合同是“拒绝哪条边”，不是当前脚本、命令或作业的名字。

### 兼容表面

| 表面 | 稳定性 | 维护合同 |
|---|---|---|
| Go 包与内部接口 | 内部 | 不承诺模块外兼容；一次变更内迁移全部调用方后直接删除旧接口， 不保留永久 shim |
| 已发布的管理 HTTP API | 公共且版本化 | 请求、响应与错误码保持兼容；破坏性删除只进入 major release |
| 实验 HTTP API | 不稳定 | 只能位于显式实验入口或受功能开关保护，文档必须说明可能变更 |
| YAML 配置 | 公共 | 字段改名与移动由配置升级器单向迁移；运行时只读取规范的新字段 |
| 数据库 schema | 持久兼容表面 | 已发布 migration 的标识、顺序与 SQL 冻结；修正只能追加新的 forward migration |
| Telegram update 与 callback | 外部协议 | 兼容 Telegram 协议；SDK 类型只留在 `telegram`， 不成为本项目的公共 Go API |

稳定性必须由目录、路由版本、迁移历史与兼容固定装置体现， 不能只在文字中标为 internal 或 public。

### 弃用与向后兼容

以下流程只适用于已发布的 HTTP、配置和持久数据表面。 内部 Go 接口采用一次变更内的完整切换。

- 在 issue 中记录旧项、替代项、使用范围、停止接受的版本与删除版本。
- 先发布替代项。配置升级只从旧路径写入新路径，业务代码只读取新字段， 不长期保留两套实现。
- 启动日志或 HTTP 响应只警告一次，同时给出替代项与删除版本。
- 同一变更加入旧配置、旧 HTTP 请求或旧数据库固定装置， 证明升级后只产生规范的新表示。
- 小型配置项至少保留两个 minor release 且不少于 90 天； 稳定 HTTP 字段保留到下一 major。
- 只有可关闭功能才在删除前经历一个默认关闭的 minor； 普通字段改名不人为增加关闭期。
- 删除运行时代码后仍保留旧配置迁移固定装置与全部已发布 migration， 使跨版本升级继续可验证。

### 版本与发布

应用版本采用 SemVer：用户可见的兼容功能进入 minor，兼容缺陷修复进入 patch， 稳定 HTTP API 的破坏性修改进入 major；内部包重构本身不决定版本号。 有用户可见内容时，每六周最多发布一个 minor；没有内容则跳过， patch 与安全版本按需发布。当前 v5 的阶段合并与一次性发布次序仍以 `docs/PLAN-v5.md` 为准。

发布说明只记录使用者能观察到或必须采取行动的变化：破坏性变化、功能、缺陷修复、 安全、配置与数据库。内部重构、构建清理和机械调整不进入发布说明。

应用 SemVer 与 schema version 独立。schema 使用单调递增版本和兼容下限； 旧二进制发现数据库兼容下限高于自身能力时拒绝启动。已发布 migration 不修改、不重排、 不删除，错误由新的 forward migration 修正。降级只保证到兼容下限允许的版本； 破坏性迁移执行前必须备份。

发布验证至少覆盖空库升级、上一发布版本数据库升级，以及兼容范围内的一次降级启动。 这些是场景合同，不把当前执行命令或流水线作业名写进架构书。

### 文档与代码的一致

本项目继续同时维护 `docs/ARCHITECTURE.md` 与 `web/architecture.html`；两份文档表达同一结论，并在同一次变更中更新。

影响包职责、允许的导入边、稳定 HTTP 表面、配置升级、schema 兼容或启动关闭顺序的变更， 必须同步更新两份架构文档，或在评审中明确记录为何架构没有变化。 每六个月只检查断链、已删除符号和失效流程，不按日历重写仍然有效的架构决定。

正文只保存不变量、边界、选择理由与稳定的场景合同。 可判定的约束由 lint、编译和行为固定装置执行；具体命令名、作业名与实现行号不写入正文， 避免流程调整后文档仍指向旧入口。

界面相关规定仍只放在 [设计语言](design.html)，不在这里重复。

## 15. 安装与更新

### 一条命令装完

**脚本只处理机器上的事，一个密钥都不问。** 令牌与凭据全部在浏览器里填：命令行历史留得住，终端回滚看得见， 而装的人不会想到这两件事。脚本问的每一项都有默认值，一路回车能走完。 同一个脚本管安装、升级、回退、卸载与查看状态。

这条边界要在实现上守住： **脚本里不出现读取令牌的分支，也不接受用环境变量传令牌**。 留一个「非交互时可以从环境变量拿」的口子，等于把这条规则作废。

### 脚本要先认这台机器

目标是多个发行版都能装。**按能力探测，不按发行版名字分支**： 判断有没有某个命令、某个目录是否可写，而不是判断这是哪个发行版。 发行版的名字会变，会有衍生版，探测出来的能力不会。

| 探什么 | 怎么用 |
|---|---|
| 进程管理器 | 决定装哪种单元文件。识别不出来时装成前台进程并说清没有开机自启， **不假装装好了** |
| 容器运行时 | 容器是默认部署方式。机器上没有运行时时问要不要装， **不问就装是越权**；不装就退回原生服务，不是装不了 |
| 端口是否被占 | 装之前就要报，不能起服务失败之后才让人自己去查是谁占了 |
| 架构与库 | 决定下载哪个二进制。静态构建，不依赖机器上的运行库 |

### 路径与证书

默认路径按 Linux 惯例分开放，**因为卸载和备份要按这条线区分**： 配置和数据要留，缓存可以丢。

```text
/etc/vestibule/          配置与 provisioning     卸载时单独问要不要删
/var/lib/vestibule/      数据库与状态           卸载时单独问要不要删
/usr/local/bin/          二进制                 升级替换这里，保留上一版
                         日志                   交给进程管理器，不自己写文件轮转
```

**证书要在安装当场配好，不留给人装完自己去查怎么弄。** 这一块照 3x-ui 的做法，它把没有域名的人也覆盖到了：

| 情况 | 怎么做 | 结果是什么 |
|---|---|---|
| 填了域名 | 用 ACME 客户端以独立模式签发，占用 80 端口那一下。 已有且未过期的证书直接复用，**不重复签发** | 浏览器认的证书，自动续期 |
| 没有域名 | 探测本机公网地址，签发面向 IP 的短期证书。 探测走多个来源，**互相不一致就停下来问，不猜** | 浏览器认的证书。有效期短，因此续期任务必须在装的时候一起装好 |
| 自己有证书 | 给现成的证书与私钥路径，不签发 | 用自己的那份 |
| 不使用证书 | 仅监听回环地址；控制台使用 HTTP，并给出通过端口转发访问控制台的命令 | 不暴露到公网，无需证书 |

ACME 客户端不在机器上时当场装，**不因为缺一个工具就让整个安装停下来**。 私钥权限 `600`，证书链 `644`。 签发完由脚本直接写进配置并重载，不需要人再去改一个文件。

**短期证书的续期是一条要长期活着的链。** 安装时一并写入续期任务，并且把「上次续期成功是什么时候」放进诊断屏。 续期悄悄停掉、直到证书过期那天才发现，是这种做法唯一的真风险。 这一条是我们比它多做的：它只装 cron，不回报状态。

装完打印的是**可以直接打开的完整地址**， 域名或探测到的公网地址、端口、随机路径、以及认领链接，拼好一条。 让人自己拼地址就等于让一半的人卡在这里。

### 重跑同一条命令是升级

安装脚本必须可以重复执行。**已经装过就走升级，不是重新装一遍**： 已有的配置和数据保持原样，只替换二进制与单元文件。 装到一半失败时，删除这一次新建的文件与单元再退出， **不留一个半装的状态让下一次执行去猜**。

同类项目的安装脚本有一千两百余行， 其中很大一部分是内嵌的双语消息表。**取它一个脚本管完整生命周期的形状， 不取把界面文案编进脚本的做法**：脚本只输出必要的进度与错误，翻译留给文档。

```text
install.sh                安装：探测、问机器上的四件事、起服务、打印认领链接
install.sh --upgrade      升级：下载、校验、原子替换、重启、探活，不健康自动回退
install.sh --rollback     回退到上一版
install.sh --status       版本、健康、上次升级结果
install.sh --uninstall    卸载，数据目录是否一并删除单独确认
```

### 从 3x-ui 取什么，不取什么

它是同一形态的现成实践：一条命令装完、自带网页面板、脚本管完整生命周期。 **参考它的做法，不照搬它的判断。**

| 它的做法 | 我们 | 为什么 |
|---|---|---|
| 没有终端时自动转非交互，判据是 `[ ! -t 0 ]` | **取** | 不必额外加一个标志，管道执行时行为自然正确 |
| 面板端口在 1024–62000 间随机，网页路径是一段随机字符 | **取** | 控制台不落在人人都知道的地址上，挡掉按固定端口批量扫描的那一类。 代价是装完必须把地址完整告诉人，不能靠默认值记忆 |
| 装完把结果写进一个 `600` 权限的文件供自动化读取 | **取** | 批量部署的人需要机器可读的产物。我们写进去的是认领链接，不是凭据 |
| 已装过就停服务、备份 `bin/`、解包后把管理员放的文件放回去 | **取** | 正是「重跑是升级」需要的形状：区分随版本走的和人放进去的 |
| 按 `/etc/os-release` 的 `ID` 分支，列了十六个发行版名字 | **不取** | 名单外的衍生版直接装不了。我们按能力探测， 代价是每一项要多写一段判断，换来的是不用维护这份名单 |
| 自动生成面板用户名与密码，装完打印，并写进结果文件 | **不取** | 我们没有面板密码。身份来自 Telegram 绑定， **没有可以泄露的默认凭据，也没有一份写着密码的文件** |
| 接受 `XUI_USERNAME` 与 `XUI_PASSWORD` 环境变量 | **不取** | 这正是我们禁掉的那扇门。留一个「非交互时可以从环境变量拿凭据」的口子， 「密钥不进脚本」这条规则就作废了 |
| 无域名时签发短期 IP 证书并用 cron 续期 | **取，并补一处** | 没有域名的人也能得到浏览器认可的证书，这一段是它最值得抄的。 补的是回报：短期证书靠一条定时任务活着， **它只装任务不报状态**，续期停掉要到过期那天才发现。 我们把上次续期结果放进诊断屏 |

发现的问题记在这里：它把密码写进 `/etc/x-ui/install-result.env` 并在终端打印一遍， 两处都会被回滚看到或被备份带走。**凭据一旦生成就必然要落到某处**， 这也是我们选择不生成凭据的理由之一。

### 更新期间能保证什么

**不是零中断。**只有一个实例可以拉取更新，因此升级必然有几秒的重启窗口， 声称完全不中断是不诚实的。可以做到的是这三条：

| 保证 | 靠什么 |
|---|---|
| **不丢更新** | 拉取中断期间，平台侧会保留未确认的更新并在恢复后重发 |
| **不丢进行中的验证** | 待验证记录与到期时间都在数据库，超时由扫描器领取而不是进程内定时器 |
| **不重复处理** | 重复到达由唯一索引拒绝，结算是带条件的更新 |

控制台在升级期间显示「正在更新」并自动重试， 而不是抛出一个连接错误让人以为坏了。

### 发布查询不跟页面加载绑定

`GET /api/status` 只读本机的当前版本、宿主单元和上次替换结果。 打开版本屏不会访问外部网络；运维按下检查更新后， `GET /api/status/release` 才查询固定的 GitHub 仓库与最新正式发布。 发布源不可用时，只有新版区块进入可重试状态，不能把失败写成「已是最新」， 也不能遮住已经读到的本地状态。

发布响应同时带变更说明和目标标签的 `vestibule-schema-manifest`。 当前二进制内嵌的迁移历史不知道未来版本的兼容下限，所以回退判断必须使用目标发布自己的 `minimum_rollback_schema_version`，再与当前保留版本支持的 schema 比较。 接口返回结构版本和原因；界面负责把它写成人能理解的说明。

### 更新流程

```text
1  运维明确检查发布源，与当前版本比较
2  取目标发布的结构清单，先判断能不能回退
3  下载到临时文件，校验摘要与签名
4  原子替换，保留上一版
5  由进程管理器重启
6  探活；不健康则换回上一版并记录原因
```

**第 2 步是关键，而且必须在下载发布二进制之前。**目标发布的结构清单声明兼容下限， 决定当前保留版本能不能回退。装完才发现回不去，等于没有回退。

### 谁能点这个按钮

**只有运维，不是群管理员。**版本导航、发布查询和升级端点都遵守这条边界。 升级影响整台机器上的所有群，而群管理员只对自己的群负责； 后端在读取或写入前拒绝群管理员，不能只靠界面隐藏。

### 不需要终端也能更新

更新可以中断服务，但**不应该要求有人去开一个终端**。 同时，应用不能改写自己正在执行的文件，因为那需要给它对自身可写的权限， 会削弱进程管理器的加固。两条约束一起决定了机制： **应用发起，另一个单元执行。**

```text
1  运维看清目标版本与回退条件，再次确认升级
2  应用往数据目录写一个请求文件      它只需要对自己的数据目录可写
3  一个监视该路径的单元被触发         一次性任务，有替换二进制的权限
4  下载 → 校验 → 原子替换 → 重启主单元 → 探活
5  结果写回数据目录                  应用重启后读它，界面显示成功或已回退
```

这样点击更新不需要任何 shell 访问，而应用自身不持有多余的权限。 **请求文件里只有目标版本，没有下载地址**：地址由升级单元自己决定， 否则一个能写数据目录的人就能指定从哪里下载。

### 容器是默认部署方式

默认走容器：**装的人不必先把运行时依赖弄对**，回退是换回上一个镜像标签， 比替换二进制干净。原生服务保留给不想装容器运行时的人，两条路的操作面一致。

这带来一个必须正面解决的问题： **容器里的进程不能更新自己**。要替换镜像就得能操作容器运行时， 那等同于取得宿主的最高权限。把 `docker.sock` 挂进应用容器， 等于让一个对着公网的 Web 服务持有宿主 root，为一个更新按钮交出这个权限不成比例。

### 两种部署共用同一套更新机制

结论是不为容器另设一条路径，而是把原生那条推广过去： **应用只写意图，宿主侧的单元执行，容器始终拿不到运行时**。 安装脚本本来就以 root 在宿主上跑过一次，那一次顺带装下这个单元。

| 步骤 | 容器 | 原生服务 |
|---|---|---|
| 应用做什么 | 往自己的数据目录写一个只含目标版本的请求文件。 **不含下载地址**，地址由执行方决定，否则能写数据目录的人就能指定从哪里下载 |
| 谁执行 | 宿主上监视该路径的单元 | 宿主上监视该路径的单元 |
| 怎么换 | 拉新镜像，用新标签重建容器 | 下载、校验、原子替换二进制 |
| 怎么回退 | 换回上一个镜像标签 | 换回保留的上一版二进制 |
| 探活失败 | 自动回到上一版，把原因写回数据目录， 应用重启后读它并在界面上说明 |

**一套机制，两种落地。**之前把容器写成「不给按钮、给一条可复制的命令」是错的： 那让默认部署方式失去主要功能，而问题本身有解。

还有一种情况给不了按钮： 有人不用安装脚本，自己写 compose 起容器，宿主上就没有那个单元。 这时界面显示有新版并给出该执行的命令，**并说明为什么这里不同**， 不是让人以为功能坏了。判断依据是那个单元在不在，不是猜部署方式。

### 哪里不该做什么

- **不在 console 里判断业务。**它只做认证、参数校验与调用。
- **不在 telegram 里查数据库。**它把 Update 转成领域事件即结束。
- **不在 verification 里格式化面向用户的文案。**它返回结构化结果。
- **不给不变量加开关。**界面偏好可以配置，不变量不可以。
- **不为单个群加分支。**需要差异时加配置项，并给出出厂默认。
