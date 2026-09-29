<div align="center">

<img src="assets/logo.svg" alt="Sub2API Logo" width="128" />

# Sub2API 破甲版

**在 AI API 网关层面注入人格（破甲）的 sub2api 二次开发版**

[![Go](https://img.shields.io/badge/Go-1.27.0-00ADD8.svg)](https://golang.org/)
[![Vue](https://img.shields.io/badge/Vue-3.4+-4FC08D.svg)](https://vuejs.org/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-18-336791.svg)](https://www.postgresql.org/)
[![Docker](https://img.shields.io/badge/Docker-一键部署-2496ED.svg)](quickstart/)

</div>

基于 [Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api) 二次开发。在原版"订阅额度分发网关"能力之上，新增**破甲（人格注入）系统**：在网关边缘自动改写发往上游模型的 system prompt，任何客户端只要调用本网关配置的模型，即可直接以指定人格运行——客户端无需任何补丁或额外配置。

## 破甲功能

- **网关边缘注入**：中间件挂载在全部 6 条网关链路（`/v1`、`/v1beta`、Codex、根路径别名、Antigravity ×2）上，按协议自动识别四种 system 形态并改写：
  - Anthropic `/v1/messages` 顶层 `system`（字符串/数组）
  - OpenAI `chat/completions` 的 `messages` 系统消息
  - OpenAI Responses API 的 `instructions`
  - Gemini `generateContent` 的 `systemInstruction`
- **两种模式**：`replace`（人格完全替换原 system）/ `prepend`（人格在前，保留原 system）
- **人格库**：仓库 `personas/` 目录即人格库（`.txt` / `.md`），面板在线预览、上传、编辑、删除、切换
- **管理面板**：`/admin/armor-break` 页面可视化开关、换人格、选模式；也有完整 REST API
- **失败开放**：破甲关闭、人格缺失、非 JSON 请求等情况一律原样透传，不影响正常转发
- **OAuth 兼容**：对 Claude OAuth 账号，人格经下游 mimicry 层以 `[System Instructions]` 形式送达模型；API Key 账号则 `system` 即为人格原文

详细设计见 [docs/armor-break.md](docs/armor-break.md)。

## 一键安装

```bash
curl -sSL https://raw.githubusercontent.com/firstwxx1/sub2api-armor-break/main/quickstart/install.sh | sudo bash
```

终端面板菜单：

```
  1) 一键安装 / 重新安装      — 检测装 Docker → 生成密钥 → 源码构建 → 健康等待 → 打印凭据
  2) 绑定域名（自动 HTTPS）    — Caddy 容器自动签发/续期证书
  3) 卸载                     — 可选清数据卷、清源码
  4) 状态检测                  — 容器/健康/登录/破甲状态/人格库/网关链路/域名 HTTPS
  5) 查看日志
  6) 破甲开关 / 切换人格        — 免开面板，终端直接切
  7) 更新到最新代码             — git pull + 重建
```

安装细节见 [quickstart/README.md](quickstart/README.md)。

## 使用流程

1. 浏览器打开 `http://<服务器IP>:8080`（或绑定的域名），用安装时打印的管理员账号登录
2. **添加上游账号**（Claude / Gemini / OpenAI / Antigravity 等；大陆服务器给 Google 系账号配代理）
3. **创建分组与 API key**
4. **开启破甲**：面板「破甲」页或安装菜单第 6 项，选人格、选模式
5. 客户端 `base_url` 指向 `http(s)://<地址>/v1`，带上 key —— 出来的就是人格在说话

## 与上游的关系

本仓库保留上游完整提交历史，`upstream` 远程指向 Wei-Shaw/sub2api，可随时合并上游更新。原版完整文档：

- 中文文档：[README_CN.md](README_CN.md)
- English：[docs/](docs/) · 上游 [README](https://github.com/Wei-Shaw/sub2api/blob/main/README.md)
- 日本語：[README_JA.md](README_JA.md)

## 破甲相关改动一览

| 位置 | 内容 |
|---|---|
| `backend/internal/service/armor_break.go` | 人格库（目录扫描/缓存/防穿越）+ 四协议形态改写 + 设置服务 |
| `backend/internal/server/middleware/armor_break.go` | 网关中间件（挂载于全部网关链路） |
| `backend/internal/server/routes/armor_break.go` | 管理 API：`/api/v1/admin/armor-break` |
| `frontend/src/views/admin/ArmorBreakView.vue` | 管理面板页面 |
| `personas/` | 人格库（25 张卡片） |
| `docs/armor-break.md` | 破甲架构与行为文档 |
| `quickstart/` | 一键安装面板 |

## 声明

本项目仅供技术学习与研究使用。使用本项目可能违反上游服务商（Anthropic 等）的服务条款，请在使用前自行评估风险；请遵守所在国家或地区的法律法规。因使用本项目产生的任何直接或间接后果，由使用者自行承担。
