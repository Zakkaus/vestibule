# 架构

一个 Telegram 群入群验证与管理机器人，加一个 Web 控制台。同一份代码服务很多群，
每个群由该群自己的 Telegram 管理员配置。

本文件是实施依据。界面取值见 `web/design.html`，贡献方式与门禁见 `CONTRIBUTING.md`。

> 本文件与 `web/architecture.html` 内容一致，两者同时更新。
> 界面相关的规定一律放 `web/design.html`，不在这里重复。
> 章节顺序：不变量 · 部署拓扑 · 包结构 · 群的两种模式 · 流程 · 数据模型 ·
> 状态机与动作执行 · 规则引擎 · 权限 · 配置 · 接口 · 前端 · 稳定性 ·
> 可观测与维护 · 实施。


## 0. 设计目标与不变量

不变量在每一个阶段都必须成立，任何一条被破坏即为该阶段未完成。

| # | 不变量 | 破坏时的表现 |
|---|---|---|
| 1 | Web 控制台与 Telegram 更新调用同一个 `verification.Service` | 后台放行与群内答题走出两套规则 |
| 2 | `internal/verification` 不引用 telego，不出现 Telegram 类型 | 验证逻辑无法在无网络条件下测试 |
| 3 | 失败朝拒绝方向倒 | 上游抖动时陌生人被放行入群 |
| 4 | 状态转换是带条件的更新，读取后不再写回 | 并发路径重复结算 |
| 5 | 先落库，再对外可见 | 挑战已发出但系统不知道，重启后无人结算 |
| 6 | 迁移失败即退出 | 以旧结构接收流量，写坏数据 |
| 7 | 代码中不存在针对特定群的分支 | 通用性只是说法，删掉我们的配置就无法运行 |
| 8 | 机器人令牌不进日志、界面、仓库 | 令牌泄露等于全部身份可被伪造 |

验收标准一句话：**删掉我们社区那几行配置，产品照常运转。**

## 1. 不许写成面条

**每一次改动都按完整架构落地，不允许先塞进去以后再整理。**
「以后再拆」在这个仓库里没有发生过一次，它只是把成本推给下一个人。

### 硬性上限

| 项 | 上限 | 超了怎么办 |
|---|---|---|
| 单个文件 | 600 行 | 按职责拆成多个文件，不是按行数切 |
| 单个函数 | 80 行 | 抽出命名清楚的子函数，名字要说明它做什么 |
| 函数圈复杂度 | 15 | 通常意味着分支该换成查表或多态 |
| 一次提交涉及的职责 | 1 个 | 加功能与改结构分两次提交 |

上限是触发讨论的阈值，不是可以逼近的目标。**接近上限时先问是不是分错了包。**

### 新代码必须有归属

新增的代码只能落在架构书已经声明的包里。**需要一个新包时，先改架构书**，
说明它管什么、允许什么、禁止什么，再写代码。

不允许出现这几种形态：

- 名为 `util`、`common`、`helper`、`misc` 的包。它们没有边界，
  因此任何内容都能进去，最后成为第二个面条团。
- 一个函数同时做取数、判断和渲染。三件事分给三处。
- 在业务包里直接拼接面向用户的文案。文案归 `i18n`，业务返回结构化结果。
- 复制一段逻辑改两行。第二次出现时抽出来，第三次出现说明抽错了地方。

### 怎么检查

门禁里带文件行数、函数行数与圈复杂度三项，超限即失败。
包边界用导入检查：验证包不许引用 telego，规则包不许引用数据库与网络。

**这些检查在第一次实质改动之前就要能执行。** 先有尺子，再动手。

## 2. 部署拓扑

三个组件，一次部署。

```
                    ┌──────────────────────────────┐
   管理员浏览器 ────▶│  app（单个 Go 二进制）        │
   （Telegram 内）   │                              │
                    │  ├ adminhttp   :8080         │
                    │  ├ verification              │
                    │  ├ telegram                  │
                    │  └ web/dist（go:embed）      │
                    └───────┬──────────────┬───────┘
                            │              │
                     ┌──────▼─────┐   ┌────▼──────────┐
                     │ 数据库      │   │ telegram-bot- │
                     │ SQLite 或   │   │ api（容器）    │
                     │ PostgreSQL  │   │ :8081         │
                     └────────────┘   └────┬──────────┘
                                           │ MTProto
                                           ▼
                                      Telegram
```

- **app** 同时承载机器人与控制台，前端构建产物编进同一个二进制，线上不运行 Node。
- **telegram-bot-api** 自建，不可省略：直连 api.telegram.org 实测 707 ms，本地 1.3 ms。
  官方不提供静态二进制与镜像，因此在发布 CI 编译一次并发布按 commit 固定的镜像，部署机只拉取。
- **数据库** 自托管默认 SQLite 单文件，我们的公开实例使用 PostgreSQL。
  同一份迁移，只有个别 DDL 分方言。

8081 不向宿主机发布。app 与 bot-api 通过容器网络通信。

## 3. 包结构

```
cmd/bot                     进程入口，解析命令行，调用 app.Run
  │
internal/app                配置加载、数据库打开与迁移、依赖组装、生命周期、健康检查
  ├──▶ internal/adminhttp   HTTP 路由、认证、DTO
  ├──▶ internal/verification  验证状态机、策略、超时、处罚
  └──▶ internal/telegram    SDK 调用、轮询或 webhook、Update 转领域事件
```

依赖方向单向向下，`verification` 不依赖任何兄弟包。

| 包 | 允许 | 禁止 |
|---|---|---|
| `app` | 引用全部 | 承载业务判断 |
| `adminhttp` | 调用 `verification.Service` | 直接调用 telego；直接执行 SQL |
| `verification` | 定义并使用 `Gateway`、`Store` 接口 | 引用 telego；出现 `tgbotapi.*` 类型 |
| `telegram` | 实现 `verification.Gateway` | 决定谁通过谁拒绝 |

保留的既有包按原职责继续存在：`i18n`、`lookup`、`feed`、`moderate`、`panel`、`edition`。
其中 `lookup` 与 `feed` 是可选功能模块，由每个群自行启用。

### 接口定义在使用方

```go
// internal/verification/ports.go
package verification

type Gateway interface {
    SendChallenge(ctx context.Context, c Challenge) (MessageRef, error)
    DeleteMessage(ctx context.Context, chat ChatID, msg MessageID) error
    ApproveJoin(ctx context.Context, chat ChatID, user UserID) error
    DeclineJoin(ctx context.Context, chat ChatID, user UserID) error
    RestrictMember(ctx context.Context, chat ChatID, user UserID, until time.Time) error
    RemoveMember(ctx context.Context, chat ChatID, user UserID, banUntil time.Time) error
    IsMember(ctx context.Context, chat ChatID, user UserID) (bool, error)
    IsAdmin(ctx context.Context, chat ChatID, user UserID) (bool, error)
}

type Store interface {
    CreateChallenge(ctx context.Context, c *Challenge) error
    AttachMessages(ctx context.Context, id ChallengeID, refs []MessageRef) error
    Transition(ctx context.Context, id ChallengeID, from, to State) (bool, error)
    Get(ctx context.Context, id ChallengeID) (*Challenge, error)
    FindOpen(ctx context.Context, chat ChatID, user UserID) (*Challenge, error)
    ClaimExpired(ctx context.Context, now time.Time, limit int) ([]*Challenge, error)
    Settings(ctx context.Context, chat ChatID) (*GroupSettings, error)
}

type Clock interface{ Now() time.Time }

type Service struct {
    gw    Gateway
    store Store
    clock Clock
}
```

`Transition` 返回 `(bool, error)`：`false` 表示状态已被其他路径改变，
调用方按已结算处理，不重试也不报错。这是不变量 4 在接口上的体现。

## 4. 数据模型

使用 `go.mau.fi/util/dbutil`：一套代码同时支持 SQLite 与 PostgreSQL，
统一写 `$1` 占位符，SQLite 执行前自动转 `?1`。迁移经 `embed.FS` 注册。

```go
//go:embed migrations/*.sql
var migrationFS embed.FS

var migrations = dbutil.BuildUpgradeTable().WithFSPath(migrationFS, "migrations").Finish()
```

`migrations/00-latest.sql` 建立当前完整结构，新安装一步到位；
后续变更新增 `01-*.sql`、`02-*.sql`。版本表带兼容下限，
**旧二进制连接新结构会被拒绝启动**，因此结构不兼容时禁止直接回退二进制。

### 表

```sql
-- 群。bot 所在的每个群一行。没有全局默认，缺省值即出厂默认。
CREATE TABLE chat (
    id            BIGINT PRIMARY KEY,          -- Telegram chat id
    title         TEXT   NOT NULL,
    joined_at     BIGINT NOT NULL,
    left_at       BIGINT,                      -- 非空表示 bot 已被移出，数据待清理
    settings      TEXT   NOT NULL DEFAULT '{}' -- 仅存与出厂默认不同的项
);

-- 待验证与已结算的挑战。等待队列、操作记录、统计都从这张表出。
CREATE TABLE challenge (
    id            TEXT   PRIMARY KEY,
    chat_id       BIGINT NOT NULL REFERENCES chat(id) ON DELETE CASCADE,
    user_id       BIGINT NOT NULL,
    user_name     TEXT   NOT NULL DEFAULT '',
    state         TEXT   NOT NULL,             -- pending|approved|declined|banned|expired|superseded
    kind          TEXT   NOT NULL,             -- rule|pow|captcha|membership
    payload       TEXT   NOT NULL DEFAULT '{}',-- 题面、诱饵、nonce、难度
    attempts      INTEGER NOT NULL DEFAULT 0,
    created_at    BIGINT NOT NULL,
    expires_at    BIGINT NOT NULL,
    settled_at    BIGINT,
    settled_by    BIGINT,                      -- 管理员结算时记录其 user_id
    reason        TEXT   NOT NULL DEFAULT '',
    epoch         INTEGER NOT NULL DEFAULT 0   -- 掉线恢复重发时递增，旧定时器据此失效
);
CREATE UNIQUE INDEX challenge_open ON challenge (chat_id, user_id) WHERE state = 'pending';
CREATE INDEX challenge_due   ON challenge (expires_at) WHERE state = 'pending';
CREATE INDEX challenge_recent ON challenge (chat_id, created_at DESC);

-- 挑战发出的消息。删除时按此逐条撤回。
CREATE TABLE challenge_message (
    challenge_id  TEXT   NOT NULL REFERENCES challenge(id) ON DELETE CASCADE,
    chat_id       BIGINT NOT NULL,
    message_id    BIGINT NOT NULL,
    PRIMARY KEY (challenge_id, chat_id, message_id)
);

-- 规则集合。题库与自动回复共用同一张表，用 collection 区分。
CREATE TABLE rule (
    id            TEXT   PRIMARY KEY,
    chat_id       BIGINT NOT NULL REFERENCES chat(id) ON DELETE CASCADE,
    collection    TEXT   NOT NULL,             -- challenge|autoreply
    ordinal       INTEGER NOT NULL,
    enabled       BOOLEAN NOT NULL DEFAULT TRUE,
    definition    TEXT   NOT NULL,             -- 题面、条件、回复内容，三语
    updated_at    BIGINT NOT NULL,
    updated_by    BIGINT
);
CREATE INDEX rule_lookup ON rule (chat_id, collection, ordinal);

-- 订阅源。
CREATE TABLE feed_source (
    id            TEXT   PRIMARY KEY,
    chat_id       BIGINT NOT NULL REFERENCES chat(id) ON DELETE CASCADE,
    url           TEXT   NOT NULL,
    format        TEXT   NOT NULL,             -- rss|atom|jsonfeed
    interval_s    INTEGER NOT NULL,
    filters       TEXT   NOT NULL DEFAULT '[]',
    last_ok_at    BIGINT,
    fail_count    INTEGER NOT NULL DEFAULT 0,
    paused        BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE TABLE feed_seen (
    source_id     TEXT   NOT NULL REFERENCES feed_source(id) ON DELETE CASCADE,
    entry_id      TEXT   NOT NULL,             -- 条目标识，不用标题
    seen_at       BIGINT NOT NULL,
    PRIMARY KEY (source_id, entry_id)
);

-- 操作记录。所有写入都留痕，可撤销的记录保留撤销所需数据。
CREATE TABLE audit (
    id            BIGSERIAL PRIMARY KEY,
    chat_id       BIGINT NOT NULL,
    actor_id      BIGINT NOT NULL,
    action        TEXT   NOT NULL,
    target        TEXT   NOT NULL DEFAULT '',
    before_value  TEXT,
    after_value   TEXT,
    undo_until    BIGINT,
    created_at    BIGINT NOT NULL
);
CREATE INDEX audit_recent ON audit (chat_id, created_at DESC);
```

`challenge_open` 这个部分唯一索引是不变量 4 的落点：
同一个人在同一个群同时只能有一条待验证记录，重复到达在数据库层被拒绝，
不依赖内存中的已见集合。

`epoch` 保留现有语义：掉线恢复后重发挑战时递增，恢复前排队的定时器凭旧 epoch 结算会被拒绝。

### 写入顺序

```
BEGIN
  INSERT INTO challenge (... state='pending' ...)      -- 先落库
COMMIT
SendChallenge()                                        -- 再对外可见
BEGIN
  INSERT INTO challenge_message (...)                  -- 回填消息标识
COMMIT
```

`SendChallenge` 失败时删除该条记录。顺序颠倒会留下没有主的挑战，
这是扫描中发现的一类实际缺陷。

## 5. 状态机

```
                    ┌──────────┐
   join request ───▶│ pending  │
                    └────┬─────┘
        答对 / 管理员放行 │ 超时      │ 管理员拒绝    │ 重复到达
                    ┌────▼─────┐ ┌───▼──────┐ ┌────▼─────┐ ┌──────────┐
                    │ approved │ │ expired  │ │ declined │ │superseded│
                    └──────────┘ └────┬─────┘ └────┬─────┘ └──────────┘
                                      │ 配置了拒绝后封禁
                                 ┌────▼─────┐
                                 │  banned  │
                                 └──────────┘
```

每一次转换都是一条带条件的更新：

```sql
UPDATE challenge
   SET state = $2, settled_at = $3, settled_by = $4, reason = $5
 WHERE id = $1 AND state = 'pending';
```

影响 0 行表示已被其他路径结算。**这不是错误**，调用方按已结算处理并返回成功。
扫描发现的「刚通过验证的人被进群后验证又抓一次」正是缺少这一层导致的。

超时不使用进程内定时器。扫描器按 `challenge_due` 索引领取到期记录：

```sql
UPDATE challenge SET state = 'expired', settled_at = $1
 WHERE id IN (SELECT id FROM challenge
               WHERE state = 'pending' AND expires_at <= $1
               ORDER BY expires_at LIMIT $2)
RETURNING id, chat_id, user_id;
```

代价是精度由秒级变为扫描间隔。收益是重启不丢、多实例不重复。

## 6. 配置

配置分三层，各有各的位置。**不写成一份文件。** 判断一项属于哪一层，看谁改它、多久改一次。

| 层 | 谁改 | 放在哪 | 例 |
|---|---|---|---|
| 进程配置 | 运维，很少改 | 文件加环境变量 | 令牌、监听地址、数据库连接、Bot API 地址、日志 |
| 每群配置 | 该群管理员，随时改 | 数据库 | 验证方式、超时、封禁时长、提示文案 |
| 声明式资源 | 自托管者，进版本控制 | provisioning 目录 | 题库、自动回复、订阅源 |

### 进程配置：默认内嵌，用户文件只写差异

```
/etc/vestibule/
├── config.yaml                 只写与默认不同的项
└── provisioning/
    ├── rules/{challenges,autoreply}.yaml
    └── feeds/*.yaml

internal/settings/
├── defaults.yaml               go:embed，完整、带注释、含全部默认值
├── config.go                   struct 与 Validate
├── upgrade.go                  旧路径到当前路径的复制规则
└── load.go                     Do → Unmarshal → Validate
```

- **分节**：`server`、`database`、`telegram`、`web`、`log` 各管一段，不是平铺的表。
- **环境变量按机械映射覆盖**：`database.uri` 对应 `VT_DATABASE_URI`，规则可推导。
- **密钥只写引用**：`token: $file{/run/secrets/bot_token}` 或 `$env{BOT_TOKEN}`。
  明文密钥不进配置文件，因为配置文件会被贴进工单、提交进仓库、发到群里求助。
- **升级由 configupgrade 处理**：以当前 `defaults.yaml` 为模板重建，按白名单复制旧值，
  不维护版本链。代价是拼错的未知键会在写回时消失，因此每个字段都要有示例项与复制规则。

### 每群配置：存空表示继承

数据库只存与出厂默认不同的项。空值表示继承，不保存副本，
否则默认值改动之后无法传播。既有的入群验证方案采用同一做法。

**接口返回来源，不只返回最终值。** 每一项返回六个字段：

| 字段 | 含义 |
|---|---|
| `defaultValue` | 出厂默认 |
| `overrideValue` | 本群设定，未设定时为空 |
| `effectiveValue` | 当前实际生效的值 |
| `pendingValue` | 本次未保存的改动 |
| `source` | `default` · `group` · `provisioning` |
| `locked` | 由文件管理时为真，界面只读 |

界面三态各有各的样子：继承、已覆盖、待保存。已覆盖不只靠颜色区分，
同时给圆点与文字，并提供只看已覆盖的筛选和恢复默认。

### 声明式资源：可由文件管理的部分

题库、自动回复、订阅源既可在控制台修改，也可放进 `provisioning/` 由文件管理，
启动时应用。自托管者据此把这些内容纳入版本控制，一套配置也可复制到另一台。

- 与控制台导入**共用同一份实现和同一套校验**，不为文件另写一条路径。
- 由文件管理的资源在控制台**只读并标出来源**，避免界面改完后被重启覆盖却查不出原因。
- 文件解析失败时拒绝启动并指出是哪个文件第几条，不做部分应用。

## 7. HTTP 层

### 认证与授权

两步，缺一不可：

1. **身份**：Telegram Mini App 的 `initData`，校验 HMAC 签名、`auth_date`，
   并把已用过的签名记入短期缓存，有效期内不接受第二次。
   需要在普通浏览器打开时，改用当前 Telegram Login 的 OIDC，
   校验 ID Token 的签名、`iss`、`aud`、`exp`，配合一次性 `state`、PKCE 与 `nonce`。
   **不使用已归档的旧 iframe Login Widget。**
2. **授权**：每次敏感写入前，用来访者的数字 ID 调用本地 Bot API 的 `getChatMember`，
   只接受 `creator` 与 `administrator`。会话最长 8 小时，
   非写入路径每 60 秒复查一次。

`initData` 只证明来访者是谁，不证明他能管这个群。**两步不可互相替代。**
被操作对象的管理员身份同样现查，不使用缓存。

### 路由

```
GET    /livez                     进程事件循环存活即 200，不探测依赖
GET    /readyz                    配置校验完成、数据库已迁移、Telegram 通道建立才 200
POST   /api/session               校验 initData 或 OIDC 回调，签发会话
GET    /api/chats                 该管理员可管理的群，由 getChatMember 求交集得出
GET    /api/chats/{id}/overview   首页四层所需数据，一次返回
GET    /api/chats/{id}/queue      等待队列
POST   /api/chats/{id}/queue/{cid}/approve
POST   /api/chats/{id}/queue/{cid}/decline
GET    /api/chats/{id}/settings   带每项来源：出厂默认或本群设定
PATCH  /api/chats/{id}/settings   只提交改动过的字段，带版本号做冲突检测
GET    /api/chats/{id}/rules      collection=challenge|autoreply
PUT    /api/chats/{id}/rules      整份替换，用于导入
POST   /api/chats/{id}/rules/test 试答，调用线上同一份判定代码
GET    /api/chats/{id}/audit      操作记录
POST   /api/chats/{id}/audit/{aid}/undo
GET    /api/chats/{id}/stats
GET    /api/chats/{id}/diagnostics
GET    /verify/{token}            入群验证页，面向陌生访问者，无会话
POST   /verify/{token}/answer     提交答案、工作量证明结果或人机验证凭据
```

`/verify/*` 是唯一的公开面。它不复用管理会话，令牌一次性、带签名与有效期。

### 错误

三层，照搬 bridgev2 的做法：

1. Go 侧 sentinel 与 `%w` 包装，支持 `errors.Is/As`；
2. 稳定的 API 错误码：`VERIFICATION_NOT_FOUND`、`INVALID_STATE_TRANSITION`、
   `TELEGRAM_RATE_LIMITED`、`FORBIDDEN`、`CONFLICT`、`INTERNAL_ERROR`；
3. 响应只带公开消息与 request id，内部原因留在日志。

### 契约

单一 OpenAPI 文件生成 Go 服务端接口与 TypeScript 客户端类型，
CI 中重新生成并比对差异。两端各自编译通过不代表契约同步。

## 8. 前端

**Vite + React + React Router**，组件层用 shadcn/ui（`base-nova`、`neutral`、CSS variables）
+ Tailwind v4 + lucide，与既有生产项目保持同一套设计系统。
构建产物 `dist/` 经 `//go:embed` 进入二进制。

### 为什么不用 Next.js

既有生产项目使用 Next.js，但它的价值集中在服务端：RSC、ISR、图片优化、middleware。
本控制台位于登录之后，不需要 SEO、不需要首屏服务端渲染、不需要增量再生成。
若采用静态导出，这些能力全部关闭，剩下的是一个 React 构建工具加文件路由，
同时要接受两项限制：路由不能使用路径参数，`go:embed` 必须记得写 `all:` 前缀
以免下划线开头的目录被静默排除。为用不到的能力付这两笔代价不成立。

核实过既有项目的实际用法：没有 middleware、没有 server actions、
没有 `cookies()` 与 `headers()`，`next/image` 零引用，页面几乎全部是客户端组件。
它当前虽运行在 Node 进程上，该进程实际不承担服务端职责。

### 什么保持一致

设计系统与框架无关，因此一致的部分不受影响：

| 项 | 取值 |
|---|---|
| 组件库 | shadcn/ui，`base-nova`，`neutral`，CSS variables，官方支持 Vite |
| 样式 | Tailwind v4 |
| token | 同一份 oklch 语义 token 与 `.dark` 机制 |
| 图标 | lucide |
| 表单 | react-hook-form + zod |
| 组件契约 | StatusBadge 查表映射、Promise 式 ConfirmDialog、PageHeader、Section、EmptyState |

只更换一项：多语言由 next-intl 改为 react-i18next。
需要保持的是约定而非实现：顶层 key 按域划分、缺失翻译回落到源语言、
按语言校验复数类别、变量占位符与源串逐一比对。

### 前后端分离

**分离是架构上的，恒定成立；内嵌只是两种部署方式之一。** 前端是纯 SPA，
后端是一组 JSON 接口，之间只有 OpenAPI 这一份契约。没有服务端渲染的 HTML，
没有模板与 Go 结构体的耦合。

| 部署方式 | 怎么做 | 代价 |
|---|---|---|
| 内嵌（默认） | 构建产物 `go:embed` 进二进制，`/` 提供静态文件，`/api` 提供接口 | 无。版本天然对齐，一条命令部署 |
| 分开部署 | 前端放 CDN 或 nginx，后端只提供接口，前端用 `VITE_API_BASE` 指向后端 | 需处理跨域与版本协商。前后端可能不同步，因此接口返回自身版本，不匹配时前端提示刷新 |

后端不假设静态文件一定存在：没有内嵌产物时它就是纯接口服务，正常启动。
两种方式共用同一个二进制，切换不改代码。

开发时前端运行自己的开发服务器，把 `/api` 代理到本机后端或一份假数据。
**前端不依赖后端即可运行**，这是分离是否真正成立的判断标准。

### 收益

- 动态路由可直接使用 `/groups/:id`，不必改写为查询参数。
- 构建产物为 `dist/assets/`，不含下划线开头的目录，`go:embed` 无须特殊前缀。
- 构建时间与依赖数量明显低于 Next.js。

主题、token、组件契约见 `web/design.html`。

## 9. 并发与内存

Telegram 侧上限为全局每秒 30 条、同群每分钟约 20 条，吞吐不是瓶颈。
需要保证的是并发与重启之下不丢状态、不重复结算。

- 发送经统一队列，按群与按整机两层限流；收到 429 按 `retry_after` 退避，
  并把该信号上报，不在各调用点各自重试。
- 状态全在数据库后应用进程无状态，可起多份。剩余约束：主动拉取更新只能有一个实例执行；
  需要多实例时改用 webhook。**数据模型现在即按此设计。**
- 内存不随群数增长：待验证记录不在内存排队，题库与配置按群缓存但有上限与过期。
  基线为线上实测常驻 58 MB、峰值 62 MB，目标是**一百个群常驻不超过 150 MB**。

## 10. 可观测

- 日志用结构化字段，固定为 `component`、`request_id`、`update_id`、`chat_id`、
  `user_id`、`challenge_id`、`state_from`、`state_to`。生产输出 JSON。
  **令牌、验证答案、cookie、完整 Authorization 头不得进入日志。**
- 健康检查两个：`/livez` 不探测依赖，避免依赖抖动引发重启风暴；`/readyz` 探测全部依赖。
  探针成功不写访问日志，失败必须记录状态与 request id。
- 指标只保留能驱动告警或容量决策的：待验证数、超时数、状态转换失败数、
  Telegram 429 数、更新处理延迟、通过率。不把高基数字段放入标签。

通过率是最有价值的一个。线上当前为 2/71，说明来访基本是批量账号；
若突然升至八成，说明验证被绕过或正在拦截真实用户。

## 11. 分阶段实施

不做一次性重写。每一阶段独立通过全部门禁并可发布。

| 阶段 | 内容 | 验收 |
|---|---|---|
| 一 | 同包纯移动：`service.go` 2859 行按七组职责拆成多个文件，签名不变 | 两个构建标签均编译通过，测试全绿，diff 只有移动 |
| 二 | 抽出 `Gateway` 与 `Store` 接口，现有实现原样满足 | 行为零变更，新增假实现的单元测试 |
| 三 | 数据层换 `dbutil`，JSON 状态一次性迁入数据库 | 迁移可重放，旧二进制连新库被拒绝启动 |
| 四 | 配置换 `configupgrade` | 现有三个版本的配置都能升级且保留用户改过的值 |
| 五 | `adminhttp` 与前端，`go:embed` 进同一二进制 | 一条命令部署，健康检查通过 |
| 六 | 多租户：去掉全局默认，配置按群隔离 | 删除我们社区的配置后产品照常运转 |

**第一阶段不动锁。** `v.mu` 是跨文件共享状态的边界，按新文件拆锁会引入死锁。
同理 `state`、`kernel`、`pending` 不拆成独立服务，它们共享同一份状态。
测试首轮不随源文件移动，否则无法用测试证明移动本身没有改变行为。

## 12. 哪里不该做什么

- **不在 `adminhttp` 里判断业务。** 它只做认证、参数校验与调用。
  「这个人能不能被放行」属于 `verification`。
- **不在 `telegram` 里查数据库。** 它把 Update 转成领域事件即结束。
- **不在 `verification` 里格式化面向用户的文案。** 它返回结构化结果，
  由 `i18n` 与调用方渲染。
- **不给不变量加开关。** 界面偏好可以配置，不变量不可以。
- **不为单个群加分支。** 需要差异时加配置项，并给出出厂默认。
