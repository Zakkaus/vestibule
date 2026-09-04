[English](README.md) · **简体中文**

# Vestibule

Vestibule 是 Telegram 群组入群验证与管理机器人。一个实例可服务多个群组，
每个群组均由其 Telegram 管理员通过 Web 控制台配置。

该名称取自进门前等候的门厅。启用审批模式时，申请人在群组外等待。

## 状态

**正在重写。** 当前代码树源自 `gentoo-zh-verify-bot` v4.5.6，原样迁入后改名。
目前行为未变：本次重写先调整包结构，每个阶段仅迁移代码，不改变其判定逻辑。
`docs/PLAN-v5.md` 说明十二个阶段的验收条件及明确排除的工作。

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

## 目标

1. 任何人均可将机器人添加到自己的群组，并由该群组的 Telegram 管理员自行配置。
2. Web 控制台覆盖每项群组设置；进程级配置中的 `disabled_modules` 用于选择可选的 `gentoo` 与 `linux` 机器人模块。
3. 状态存储在数据库中，并发和重启时不会丢失或重复结算。
4. 可通过一条命令完成部署，升级失败时自动回滚。

验收标准只有一句：**删除本社区的记录后，产品仍可正常运行。**

## 许可证

参见 `LICENSE`。
