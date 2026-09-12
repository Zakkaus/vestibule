# GitHub 提交、issue 与 pull request 订阅配置

GitHub 提交订阅使用 Atom 源，issue 与 pull request 订阅使用 REST API。订阅是群级运行期设置；控制台页面只读，群级 API 可读取并保存。配置文件中的 `feeds` 只在启动时作为一次性导入输入，之后不再作为运行期来源。GitHub 订阅仍属于 Gentoo 模块，因此禁用 `gentoo` 模块时不会启动这些来源。

## 运行期设置与首次导入

群级订阅的出厂默认值为：`interval_seconds=300`，`bugs=false`、`news=false`，其余字符串与 `github_repos` 为空。`GET /api/chats/{id}/feeds` 返回有效值、每项来源与群设置版本号。`PUT /api/chats/{id}/feeds` 要求运维会话、CSRF 令牌和完整请求体；它以 `expected_revision` 做条件更新，并整份替换该群的订阅覆盖：

```json
{
  "expected_revision": 0,
  "lang": "en",
  "interval_seconds": 300,
  "bugs": false,
  "news": false,
  "bug_product": "",
  "bug_component": "",
  "silent_bugs": false,
  "github_repos": [
    {
      "repo": "gentoo-zh/overlay",
      "branch": "master",
      "issues": true,
      "pulls": true
    }
  ]
}
```

[`examples/feeds.json`](../examples/feeds.json) 是可提交到该 PUT 路由的请求体。所有字段均为必填；用空字符串或空数组显式清除对应覆盖。版本冲突返回 `409 settings_conflict`，校验失败返回字段错误且不改变版本号。

旧 `config.json` 的 `feeds` 与单数 `feed` 只作启动迁移输入。Store 仅为已管理、尚无订阅覆盖的群导入一次；文件状态使用原子替换，数据库状态使用版本比较交换。任一未管理群 ID、持久化错误或比较交换冲突都会终止启动，不会发布半完成快照。导入成功后应从配置文件删除这些条目，后续修改通过群设置接口持久化。为保持旧版行为，迁移条目中缺失或为 `null` 的 `bugs`、`news` 会导入为 `true`；运行期 PUT 不接受缺失或 `null` 的布尔值。

`issues` 和 `pulls` 分别控制新建 issue 与新建 pull request 推送。运行期请求必须显式传布尔值；首次导入时，省略、设为 `null` 或设为 `false` 均表示关闭。提交推送继续随仓库配置启用，不受这两个开关影响。

省略 `branch` 会跟随仓库当前默认分支。仓库身份是 `(repo, branch)`，开关不同不会形成另一个身份；同一群不能重复这个二元组。`repo` 必须是两段 `owner/name`，每段只能使用 ASCII 字母、数字、`.`、`_`、`-`，长度为 1 至 100，且段本身不能是 `.` 或 `..`。

加载配置时不会验证 `branch` 是否是 Git ref。`branch` 可以包含 `/`、`@`、`#` 等字符；非法或不存在的分支由上游请求失败处理。`url.PathEscape` 只负责把分支分段编码进请求路径，不是分支合法性校验。

空 `github_repos` 表示该群不订阅 GitHub。`silent_bugs: true` 强制所有 Bugzilla 消息静默；设为 `false` 时，仍按缺陷状态决定是否静默，例如 `UNCONFIRMED` 和首次观察时已解决的缺陷仍不发通知。该字段不影响 GitHub 提交消息，产品与组件过滤也只作用于 Bugzilla。

## GitHub 基地址

`github_atom_base` 是用户配置的顶层字段，不是 `resources` 对象的成员。空值使用 `https://github.com`。非空值必须是带 host 的 `http://` 或 `https://` 地址，不能包含 userinfo、query 或 fragment。加载时会移除所有尾部 `/`，并保留路径前缀：

```json
{"github_atom_base": "https://git.example.com/git///"}
```

上述配置的有效基地址为 `https://git.example.com/git`，提交源和提交链接都会在这个前缀下构造。地址不符合这些条件时，配置加载直接失败，不会静默改用另一个地址。

`github_api_base` 是独立的 REST API 基地址，空值使用 `https://api.github.com`。它与 `github_atom_base` 使用相同的地址校验与尾部 `/` 规范化规则，但不会改变 Atom 请求或消息链接。启用任一事件开关时，每个本轮访问的仓库只请求一次 `/repos/{owner}/{repo}/issues`；该响应同时提供 issue 与 pull request。

自建 GitHub Enterprise 的版本、匿名权限、路由、重定向、Atom 返回的提交标识及 REST 路径尚未核实。`github_atom_base` 必须指向同时提供 Atom 源和网页提交路径的站点根或挂载前缀；配置不会在两个基地址之间自动转换。

## 轮询、状态与失败处理
每个目的地每轮最多发送 10 条 GitHub 消息。仓库从 `next_repo` 开始，按配置顺序访问，每个仓库一轮最多访问一次；每个仓库按提交、issue、pull request 的固定顺序共用剩余预算。完整访问一圈后，下一轮从最后访问仓库的下一个位置开始；预算耗尽后也从该位置开始，未发送的条目保留在游标之后。普通 Atom 或 REST 失败只影响相应来源，后面的仓库仍会尝试；REST 失败不会阻止同仓库的提交推送，也不会推进 issue 或 pull request 游标。

Telegram 发送的永久拒绝会消耗一条预算并推进对应游标。其他发送失败不会跨过未送达的消息。Telegram `429` 会停止该目的地本轮的全部 GitHub 投递，并保留当前仓库为下一轮起点；本实现不等待 `retry_after`，也不保证收到 `429` 后至少等待 60 秒。取消轮询会停止后续 HTTP 请求，并在普通轮次保存已经推进的状态。

提交继续使用原有基线和游标规则。issue 与 pull request 各有独立的首次启用标记和编号游标；某类首次启用时以当前页内该类的最高编号建立基线，不补发历史。后续条目按编号升序发送，状态变为关闭、合并或重新打开时不会再次发送新建消息。

REST 每页最多 30 条，issue 与 pull request 共用编号。满页时，如果某类页内最低编号仍大于该类游标，系统记录一次 `WARNING` 并把游标推进到该类最高编号。如果满页没有该类条目，但全页最低编号仍大于该类游标，系统同样记录一次 `WARNING`，并以全页最高编号推进该类游标。未满页或页面编号与游标重叠时，不会对缺席的类别告警或推进游标。

Atom 的根元素、`entry`、`id`、标题类型和页内唯一性不符合解析契约时，整页拒绝并保留原状态。REST 中任一条目的正整数 `number`、仓库内 `html_url`、非空 `title`、非空 `user.login`、RFC 3339 `created_at`、`open` 或 `closed` 状态以及 pull request 的可空 RFC 3339 `merged_at` 不符合契约时，也会拒绝整页并保留两个事件游标。REST 标题会删除控制字符并截为 200 个字符；两类响应均限制为 4 MiB。

状态键是 `repo@branch`，只按完整键查找，不按仓库序号绑定。提交、issue 与 pull request 游标在每次发送成功或永久拒绝后分别在内存中推进；完成该目的地的本轮轮询后，状态快照统一落盘。`repos` 中值为 `null` 的键会删除，不会变成已初始化的空游标；空表在保存时省略整个 `github` 字段。移除的仓库键会保留一个轮询周期后再剪枝；重新加入时仍可沿用未剪枝游标。整个目的地移出配置时，运行时保留并刷写其状态；重新加入后重置为立即到期并按旧游标继续。

每个状态文件由一个运行进程单写；进程内写入使用原子替换，但多个进程同时写同一目的地时，后写入的完整快照可能覆盖先写入的状态。消息先发送后保存，进程在发送成功与保存之间崩溃时，重启可能重复发送。旧版本读取新状态后重写会丢弃未知的 `github` 字段；因此回滚期间的提交可能漏收，重新升级后会重新建立基线。

只发布新建事件；评论、关闭、合并、重新打开与 review 事件均不支持，也不会编辑已发送消息。GitHub 源仍受 `gentoo` 模块门控。GitHub Enterprise 的版本、匿名权限、路由、重定向、Atom 标识及 REST 路径尚未端到端核实。
