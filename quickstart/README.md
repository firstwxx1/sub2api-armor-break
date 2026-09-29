# sub2api 破甲版 · 一键安装

破甲（人格注入）版 sub2api 的终端面板安装器。一个脚本完成安装、域名绑定、卸载、状态检测。

## 一行安装（全新服务器）

预编译二进制部署（推荐，目标机不编译，1 GB 内存也能跑）：

```bash
curl -sSL https://raw.githubusercontent.com/firstwxx1/sub2api-armor-break/main/quickstart/install-precompiled.sh | sudo bash
```

从源码构建部署（内存建议 ≥ 2 GB，构建期峰值可能超过 1 GB）：

```bash
curl -sSL https://raw.githubusercontent.com/firstwxx1/sub2api-armor-break/main/quickstart/install.sh | sudo bash
```

脚本会：检测/安装 Docker → 克隆源码 → 引导配置端口与管理员账号 → 从源码构建镜像（前端 + 后端嵌入）→ 启动 postgres / redis / sub2api → 等待健康检查 → 自动完成管理员合规确认 → 可选立即启用破甲 → 打印面板地址和凭据。

## 已克隆仓库

```bash
cd quickstart
sudo bash install.sh          # 打开菜单
sudo bash install.sh install  # 或直接执行某项
```

## 菜单

```
  1) 一键安装 / 重新安装
  2) 绑定域名（自动 HTTPS）      — Caddy 容器自动签发/续期证书，需域名已解析到本机、放行 80/443
  3) 卸载                        — 可选是否删除数据卷与源码目录
  4) 状态检测                    — 容器 / 健康检查 / 管理员登录 / 破甲状态 / 人格库数量 / 网关 401 探测 / 域名 HTTPS
  5) 查看日志
  6) 破甲开关 / 切换人格          — 免登面板，直接终端切换
  7) 更新到最新代码               — git pull + 重建
```

## 安装后

1. 浏览器打开 `http://<服务器IP>:<端口>`（或绑定的域名），用安装时打印的管理员邮箱/密码登录
2. **上游账号**：面板添加账号（Google 系 OAuth 在大陆服务器需要先在「代理」里配一个可用代理）
3. **API key**：面板创建 key，客户端 `base_url` 指向 `http(s)://<地址>/v1`
4. **破甲**：菜单第 6 项或面板「破甲」页开关；开启后所有经过网关的请求自动注入所选人格

凭据保存在 `quickstart/.env`（权限 600）。管理员 API 的首次合规确认已由安装脚本自动完成。

## 注意

- 大陆 IP + 无备案域名：80 端口 HTTP 会被运营商/云厂商劫持，请始终使用 `https://` 入口
- postgres 数据卷挂载点已按 postgres:18 的新布局处理（`PGDATA=/var/lib/postgresql/data`），down/up 不会丢数据
- 人格库来自仓库 `personas/` 目录，只读挂载进容器；增删人格文件后在面板刷新即可
