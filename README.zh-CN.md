[English](README.md) · **简体中文**

# Vestibule

Telegram 群入群验证与管理机器人，一个实例服务很多群，
每个群由该群自己的管理员通过 Web 控制台配置。

名字取自门厅：进门之前等待的那个房间。审批制下，未通过验证的人就在群外等着。

## 状态

**重写中。** 当前树是 `gentoo-zh-verify-bot` v4.5.6 原样导入后改名的结果，
功能与它一致；`docs/PLAN-v5.md` 里的八个阶段尚未开始。

上一代仍在生产运行，本仓库达到可用之前不替换它。

## 三份依据

| 要查什么 | 看哪份 |
|---|---|
| 界面取值、屏的内容、文案规则 | `web/design.html` |
| 包结构、数据、流程、稳定性 | `web/architecture.html` 与 `docs/ARCHITECTURE.md` |
| 构建、提交、PR 前检查、代码风格 | `CONTRIBUTING.md` 与 `docs/development.md` |

改造次序与每阶段的验收见 `docs/PLAN-v5.md`。

三份文档都是页面，本地打开即可：

```sh
python3 -m http.server 8787 --bind 127.0.0.1 --directory web
```

## 要做成什么

1. 任何人把机器人拉进自己的群即可使用，由该群的 Telegram 管理员自行配置。
2. Web 控制台覆盖每一项设定。
3. 状态在数据库，并发与重启之下不丢、不重复结算。
4. 一条命令部署，升级失败自动回退。

验收标准一句话：**删掉我们社区那几行配置，产品照常运转。**

## 许可

见 `LICENSE`。
