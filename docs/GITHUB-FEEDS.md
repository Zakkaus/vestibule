# GitHub 提交订阅配置

GitHub 提交订阅使用公开仓库的 Atom 源，不需要 GitHub 凭据。配置在进程启动时读取；修改配置文件后必须重启进程，运行期间不会并发重配。这个订阅仍属于 Gentoo 模块，因此禁用 `gentoo` 模块时不会启动 GitHub 提交源。

## 最小配置

`github_repos` 位于 `feeds` 条目内。只启用 GitHub 提交时，必须显式关闭原有的 Bugzilla 与新闻源：

```json
{
  "github_atom_base": "",
  "feeds": [
    {
      "chat_id": -1009000000101,
      "bugs": false,
      "news": false,
      "github_repos": [
        {"repo": "gentoo-zh/overlay"},
        {"repo": "Zakkaus/vestibule", "branch": "v5/next"}
      ]
    }
  ]
}
```

省略 `branch` 会跟随仓库当前默认分支。仓库身份是 `(repo, branch)`；同一 `feeds` 条目中不能重复这个二元组。`repo` 必须是两段 `owner/name`，每段只能使用 ASCII 字母、数字、`.`、`_`、`-`，长度为 1 至 100，且段本身不能是 `.` 或 `..`。

加载配置时不会验证 `branch` 是否是 Git ref。`branch` 可以包含 `/`、`@`、`#` 等字符；非法或不存在的分支由上游请求失败处理。`url.PathEscape` 只负责把分支分段编码进请求路径，不是分支合法性校验。

`github_repos` 省略、设为 `null` 或设为空数组，都表示该目的地不订阅 GitHub 提交。旧字段仍保持原语义：`bugs` 与 `news` 缺失或为 `null` 时启用，显式 `false` 时关闭，显式 `true` 时启用。

`silent_bugs: true` 强制所有 Bugzilla 消息静默；缺失、`null` 或 `false` 时，仍按缺陷状态决定是否静默，例如 `UNCONFIRMED` 和首次观察时已解决的缺陷仍不发通知。该字段不影响 GitHub 提交消息，产品与组件过滤也只作用于 Bugzilla。

## Atom 基地址

`github_atom_base` 是用户配置的顶层字段，不是 `resources` 对象的成员。空值使用 `https://github.com`。非空值必须是带 host 的 `http://` 或 `https://` 地址，不能包含 userinfo、query 或 fragment。加载时会移除所有尾部 `/`，并保留路径前缀：

```json
{"github_atom_base": "https://git.example.com/git///"}
```

上述配置的有效基地址为 `https://git.example.com/git`，提交源和提交链接都会在这个前缀下构造。地址不符合这些条件时，配置加载直接失败，不会静默改用另一个地址。

自建 GitHub Enterprise 的版本、匿名权限、路由、重定向及 Atom 返回的提交标识尚未核实。基地址必须指向同时提供 Atom 源和网页提交路径的站点根或挂载前缀；本配置不会把 REST API 地址自动转换为网页地址。
