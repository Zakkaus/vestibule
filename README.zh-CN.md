[English](README.md) · **简体中文**

# Vestibule

Vestibule 是 Telegram 群组入群验证与管理机器人。一个实例可服务多个群组，
每个群组均由其 Telegram 管理员通过 Web 控制台配置。

该名称取自进门前等候的门厅。启用审批模式时，申请人在群组外等待。

## 状态

**正在重写。** 当前代码树源自 `gentoo-zh-verify-bot` v4.5.6，原样迁入后改名。
新部署默认使用不要求操作系统知识的选择题示例，不再询问 Linux 内核版本。
`docs/PLAN-v5.md` 说明各阶段的验收条件及明确排除的工作。

验证核心位于 `internal/verification`，不导入 Telegram，
并通过根据自身调用点定义的三个端口与外部交互：`Gateway`、`LiveProbe` 与 `Store`。
已完成的阶段仅在一处记录：`docs/PLAN-v5.md` 阶段表的状态列。
本文件曾重复该信息，最后一次记录为“阶段六进行中”。

上一代仍在生产环境中运行，当前代码库就绪之前不会替换。

## 四份参考文档

| 决策事项 | 参阅文档 |
|---|---|
| 控制台取值、界面内容、文案规则 | `web/design.html` |
| 包结构、数据、流程、可靠性 | `web/architecture.html` 与 `docs/ARCHITECTURE.md` |
| 规则：上限、不变量、语言、提交和门禁 | `CONTRIBUTING.md` |
| 软件保存的人员相关数据及实例运营者必须说明的事项 | `docs/PRIVACY.zh-CN.md` |

重写顺序及各阶段的验收标准参见 `docs/PLAN-v5.md`。

两份参考文档均为网页，可通过以下命令在本地打开：

```sh
python3 -m http.server 8787 --bind 127.0.0.1 --directory web
```

## 订阅推送

订阅推送属于群级有效设置。控制台编辑页面和 `GET /api/chats/{id}/feeds` 显示当前值及来源；`PUT /api/chats/{id}/feeds` 通过版本号与 CSRF 校验整份替换订阅覆盖。`config.json` 的 `feeds` 条目仅用于启动时一次性导入。设置 `github_repos` 可选择仓库与分支，再为各仓库启用 `issues` 或 `pulls`；两个开关默认关闭。省略分支时跟随仓库当前默认分支。迁移与投递规则参见 [`docs/GITHUB-FEEDS.md`](docs/GITHUB-FEEDS.md)，PUT 请求示例参见 [`examples/feeds.json`](examples/feeds.json)。RSS、Atom、JSON Feed 来源和富文本控制尚未实现。

## 目标

1. 任何人均可将机器人添加到自己的群组，并由该群组的 Telegram 管理员自行配置。
2. Web 控制台覆盖每项群组设置；进程级配置中的 `modules` 明确启用可选的 `gentoo` 与 `linux` 机器人模块。省略或留空时不启用任何可选模块；旧版 `disabled_modules` 键会被拒绝，必须迁移为 `modules`。
3. 状态存储在数据库中，并发和重启时不会丢失或重复结算。
4. 可通过一条命令完成部署，升级失败时自动回滚。

验收标准只有一句：**删除本社区的记录后，产品仍可正常运行。**

## 部署题库

新群默认使用 `quiz` 模式和内置示例。示例不要求领域知识，也不能有效抵挡自动化答题。
管理员仍可在验证设置中主动选择 `kernel` 或 `mixed`。

原生部署无需修改源码或重新编译即可提供实例题库。在 `config.json` 所在目录放置
`questions.json`，并在 `config.json` 中设置 `"factory_questions_file": "questions.json"`：

```json
{
  "questions": [
    {"q": "请选择“书”字。", "options": ["书", "椅"], "answer": 0}
  ],
  "fallback_questions": [
    {"q": "请输入“书”字。", "answers": ["书"]}
  ]
}
```

文件复用现有题库设置字段，不另设导入格式。至少提供一个题库字段，字段值必须是非空数组。
`answer` 是从零开始的选项编号。
相对路径以配置文件所在目录为准，也可指定绝对路径。进程在启动时读取题库；
未设置路径时使用内置示例，指定的文件不存在或内容无效时拒绝启动。

使用仓库提供的 Compose 部署时，将题库放在宿主机的 `VESTIBULE_STATE_DIRECTORY`
目录内，并设置 `"factory_questions_file": "/var/lib/vestibule/questions.json"`。
该目录已挂载到容器；宿主机上与 `config.json` 相邻的文件不会一并挂载。
题库文件须允许容器中的 UID 65532 读取。

仓库里另有一份面向 Linux 用户的题库，作为**示例**而不是默认值：见
[`examples/questions/`](examples/questions/)，每种语言一份，问的是 kernel.org、
gnu.org 和 vim 怎么存盘退出。把 `factory_questions_file` 指向与群语言相符的那一份即可。

没有自身题库的群继承部署题库，后来注册的新群也一样。题库屏显示实际生效的题目，
并标明出厂默认、配置文件或此群覆盖。还原会删除此群覆盖，重新显示继承的题库，
不会改写部署文件。修改部署题库后须重启；原有的每群题库和文案编辑器继续可用。

## 许可证

参见 `LICENSE`。

发布附件包含 `THIRD-PARTY-LICENSES`。原生安装的文件路径为
`/usr/local/share/doc/vestibule/THIRD-PARTY-LICENSES`，容器内路径为
`/usr/share/doc/vestibule/THIRD-PARTY-LICENSES`。清单包括锁定版本的 Go 运行时与发布依赖、
浏览器运行时依赖，以及仓库内的第三方图标和样式。共享样式缺少上游声明的情况已记入清单，
未补写未知的版权归属。

容器构建会追加实际运行层的 Alpine 软件包清单及许可声明。对应源码归档、Alpine
构建文件与补丁，以及原始许可文件随镜像保存在
`/usr/share/doc/vestibule/alpine-runtime-sources`。

更新依赖或第三方副本后，运行 `python3 scripts/generate-third-party-licenses.py`，
再执行 `python3 scripts/check-third-party-licenses.py`。生成和检查均须联网获取锁定版本的 npm 归档及 Bot API、TDLib 许可声明。
