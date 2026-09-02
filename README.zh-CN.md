[English](README.md) · **简体中文**

# Vestibule

Telegram 群入群验证与管理机器人，一个实例服务很多群，
每个群由该群自己的管理员通过 Web 控制台配置。

名字取自门厅：进门之前等待的那个房间。审批制下，未通过验证的人就在群外等着。

## 状态

**重写中。** 当前树起于 `gentoo-zh-verify-bot` v4.5.6 原样导入后改名。
到目前为止行为未变：这次重写先做重新分包，每一阶段都只搬代码，不改它判定什么。
`docs/PLAN-v5.md` 记着十一个阶段各自验收什么、又刻意不做什么。

阶段零与阶段一已完成。验证核心现在是 `internal/verification`，其中没有 Telegram，
对外一律经三个从自身调用点推导出的端口：`Gateway`、`LiveProbe` 与 `Store`。
阶段六正按屏分片进行，一屏一支。

上一代仍在生产运行，本仓库达到可用之前不替换它。

## 三份依据

| 要查什么 | 看哪份 |
|---|---|
| 界面取值、屏的内容、文案规则 | `web/design.html` |
| 包结构、数据、流程、稳定性 | `web/architecture.html` 与 `docs/ARCHITECTURE.md` |
| 规则：上限、不变量、语言、提交、门禁 | `CONTRIBUTING.md` |
| 软件保存哪些关于人的数据，运行实例的人要写明什么 | `docs/PRIVACY.zh-CN.md` |

改造次序与每阶段的验收见 `docs/PLAN-v5.md`。

三份文档都是页面，本地打开即可：

```sh
python3 -m http.server 8787 --bind 127.0.0.1 --directory web
```

## 要做成什么

1. 任何人把机器人拉进自己的群即可使用，由该群的 Telegram 管理员自行配置。
2. Web 控制台覆盖每一项群组设定；进程配置中的 `disabled_modules` 列表选择可选的 `gentoo` 与 `linux` 机器人模块。
3. 状态在数据库，并发与重启之下不丢、不重复结算。
4. 一条命令部署，升级失败自动回退。

验收标准一句话：**删掉我们社区那几行配置，产品照常运转。**

## 许可

见 `LICENSE`。
