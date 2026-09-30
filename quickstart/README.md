# sub2api 破甲版 · 一键安装

破甲（人格注入）版 sub2api 的终端面板安装器。一个脚本完成安装、域名绑定、卸载、状态检测、更新。

## 一行安装（全新服务器）

预编译二进制部署（推荐，目标机不编译，1 GB 内存也能跑）：

```bash
curl -sSL https://gh-proxy.com/https://raw.githubusercontent.com/firstwxx1/sub2api-armor-break/main/quickstart/install-precompiled.sh | sudo bash
```

从源码构建部署（内存建议 ≥ 2 GB，构建期峰值可能超过 1 GB，1 GB 机器容易 OOM 断连）：

```bash
curl -sSL https://gh-proxy.com/https://raw.githubusercontent.com/firstwxx1/sub2api-armor-break/main/quickstart/install.sh | sudo bash
```

预编译脚本会：检测 Docker → 通过加速源下载发布包 → 校验 SHA256 → 解包到 `/opt/sub2api` → 拉取 postgres/redis 镜像并启动 → 安装 systemd 单元 → 等待健康检查 → 打印面板地址和凭据。全程不在目标机编译。

源码脚本会：检测/安装 Docker → 克隆源码 → 引导配置端口与管理员账号 → 从源码构建镜像（前端 + 后端嵌入）→ 启动 postgres / redis / sub2api → 等待健康检查 → 自动完成管理员合规确认 → 可选立即启用破甲 → 打印面板地址和凭据。

## 两种安装模式的区别

| | 预编译部署 | 源码构建部署 |
|---|---|---|
| 目标机编译 | 不需要 | 需要（前端 + Go） |
| 内存要求 | 1 GB 可跑 | 构建期建议 ≥ 2 GB |
| 应用运行方式 | 宿主机 systemd (`sub2api`) | docker 容器 (`sub2api`) |
| 容器 | 只有 postgres / redis | postgres / redis / sub2api / caddy |
| 目录 | `/opt/sub2api` | `/opt/sub2api-armor-break` |
| 更新方式 | 下载并校验新发布包，换二进制后重启 | git pull + 重建镜像 |

## 预编译部署后怎么进管理菜单

预编译部署不落地源码树，装完只有 `/opt/sub2api`。管理菜单照样能用（脚本会自动识别预编译模式）：

```bash
# 打开交互菜单
curl -sSL https://gh-proxy.com/https://raw.githubusercontent.com/firstwxx1/sub2api-armor-break/main/quickstart/install.sh | sudo bash

# 只做状态检测
curl -sSL https://gh-proxy.com/https://raw.githubusercontent.com/firstwxx1/sub2api-armor-break/main/quickstart/install.sh | sudo bash -s status

# 直接更新（等价菜单第 7 项）
curl -sSL https://gh-proxy.com/https://raw.githubusercontent.com/firstwxx1/sub2api-armor-break/main/quickstart/install.sh | sudo bash -s update
```

## 已克隆仓库

```bash
cd quickstart
sudo bash install.sh          # 打开菜单
sudo bash install.sh install  # 或直接执行某项
```

## 菜单

```
  1) 一键安装 / 重新安装
  2) 绑定域名（自动 HTTPS）      — 源码模式用 Caddy 容器自动签发；预编译模式打印 Caddy/Nginx 反代片段
  3) 卸载                        — 可选是否删除数据卷（预编译模式还可选删除 /opt/sub2api）
  4) 状态检测                    — 容器 / systemd / 健康检查 / 管理员登录 / 破甲状态 / 人格库数量 / 网关 401 探测
  5) 查看日志                    — 预编译模式走 journalctl -u sub2api -f
  6) 破甲开关 / 切换人格          — 免登面板，直接终端切换
  7) 更新到最新版本               — 预编译: 下载+校验新发布包，换二进制后重启 systemd（不编译、不动 app.env / 数据卷）
                                  源码: git pull + 重建镜像
```

## 安装后

1. 浏览器打开 `http://<服务器IP>:<端口>`（或绑定的域名），用安装时打印的管理员邮箱/密码登录
2. **上游账号**：面板添加账号（Google 系 OAuth 在大陆服务器需要先在「代理」里配一个可用代理）
3. **API key**：面板创建 key，客户端 `base_url` 指向 `http(s)://<地址>/v1`
4. **破甲**：菜单第 6 项或面板「破甲」页开关；开启后所有经过网关的请求自动注入所选人格

凭据位置：预编译模式在 `/opt/sub2api/app.env`，源码模式在 `quickstart/.env`（权限均为 600）。管理员 API 的首次合规确认已由安装脚本自动完成。

更新不会覆盖 `app.env`：安装器检测到已有 `app.env` 会复用其中的数据库/Redis/JWT/管理员密钥，只替换二进制与人格库。脚本还会在升级前后对 `app.env` 做 SHA256 比对，万一被改写会自动还原并重启。

## 注意

- 大陆 IP + 无备案域名：80 端口 HTTP 会被运营商/云厂商劫持，请始终使用 `https://` 入口
- `raw.githubusercontent.com` 从大陆服务器常超时；上面所有 URL 都套了 `gh-proxy.com` 加速前缀
- postgres 数据卷挂载点已按 postgres:18 的新布局处理（`PGDATA=/var/lib/postgresql/data`），down/up 不会丢数据
- 重复安装会自动沿用既有 `.env` 中的数据库/Redis/会话密钥（postgres 数据卷只认首次初始化时的密码，重生命钥会导致应用崩循环）；要彻底重置请先用菜单 3 卸载并清数据卷
- 人格库：源码模式来自仓库 `personas/`；预编译模式解包到 `/opt/sub2api/personas`，更新时随发布包刷新
