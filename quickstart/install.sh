#!/usr/bin/env bash
# =============================================================================
# sub2api 破甲版 · 一键安装面板
# 用法:
#   bash install.sh            # 交互菜单
#   bash install.sh install    # 直接执行某项: install/domain/uninstall/status/logs/update
#   curl -sSL https://raw.githubusercontent.com/firstwxx1/sub2api-armor-break/main/quickstart/install.sh | sudo bash
# =============================================================================
set -euo pipefail

# ---------- 常量 ----------
REPO_URL="https://github.com/firstwxx1/sub2api-armor-break.git"
REPO_SLUG="firstwxx1/sub2api-armor-break"
REPO_BRANCH="main"
CLONE_BASE="/opt/sub2api-armor-break"
PRECOMPILED_DIR="/opt/sub2api"
PRECOMPILED_SCRIPT_URL="https://raw.githubusercontent.com/${REPO_SLUG}/${REPO_BRANCH}/quickstart/install-precompiled.sh"
# 下载加速源，按实测速度排序；最后一项空串 = 直连 github.com
GH_PROXIES=("https://gh-proxy.com/" "https://ghfast.top/" "")
GH_API="https://api.github.com/repos/${REPO_SLUG}/contents"
COMPLIANCE_PHRASE_ZH="我已阅读、理解并同意 Sub2API 部署与运营合规承诺"

# ---------- 颜色 ----------
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; CYAN='\033[0;36m'; NC='\033[0m'
p_info() { echo -e "${BLUE}[信息]${NC} $*"; }
p_ok()   { echo -e "${GREEN}[成功]${NC} $*"; }
p_warn() { echo -e "${YELLOW}[警告]${NC} $*"; }
p_err()  { echo -e "${RED}[错误]${NC} $*" >&2; }
hr()     { echo -e "${CYAN}==================================================${NC}"; }

is_tty() { [ -e /dev/tty ] && [ -r /dev/tty ] && [ -w /dev/tty ]; }
ask()    { local _v; read -rp "$1" _v < /dev/tty || true; echo "${_v:-$2}"; }
ask_yn() { local _v; read -rp "$1 [y/N]: " _v < /dev/tty || true; [[ "${_v:-}" =~ ^[Yy]$ ]]; }

# ---------- 环境 ----------
SUDO=""
if [ "$(id -u)" -ne 0 ]; then
  if command -v sudo >/dev/null 2>&1; then SUDO="sudo"; else
    p_err "需要 root 或 sudo 权限运行"; exit 1
  fi
fi

# 定位 compose 目录: 脚本在仓库内 → 本目录; curl|bash → clone 后的 quickstart/
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}" 2>/dev/null)" 2>/dev/null && pwd || pwd)"
COMPOSE_DIR=""
# INSTALL_MODE: source = 源码构建; precompiled = 预编译二进制 + 容器只跑数据库
INSTALL_MODE=""
resolve_compose_dir() {
  INSTALL_MODE=""
  if [ -f "$SCRIPT_DIR/docker-compose.yml" ] && [ -d "$SCRIPT_DIR/../backend" ]; then
    COMPOSE_DIR="$SCRIPT_DIR"; INSTALL_MODE="source"
  elif [ -f "$CLONE_BASE/quickstart/docker-compose.yml" ]; then
    COMPOSE_DIR="$CLONE_BASE/quickstart"; INSTALL_MODE="source"
  elif [ -f "$PRECOMPILED_DIR/app.env" ] && [ -f "$PRECOMPILED_DIR/db-compose.yml" ]; then
    COMPOSE_DIR="$PRECOMPILED_DIR"; INSTALL_MODE="precompiled"
  fi
}

compose() {
  local files=()
  [ -f "$COMPOSE_DIR/db-compose.yml" ] && files=(-f "$COMPOSE_DIR/db-compose.yml")
  if docker compose version >/dev/null 2>&1; then
    (cd "$COMPOSE_DIR" && $SUDO docker compose "${files[@]}" "$@")
  elif command -v docker-compose >/dev/null 2>&1; then
    (cd "$COMPOSE_DIR" && $SUDO docker-compose "${files[@]}" "$@")
  else
    p_err "未找到 docker compose 插件"; exit 1
  fi
}

# 从 GitHub raw / 加速源 / jsDelivr 下载文本文件
# - 加速源常把 raw 的旧版本缓存住，所以带 _cb 时间戳
# - 仍返回过期内容时（例如缺少预期标记）自动换源
url_host() { local h="${1#*://}"; echo "${h%%/*}"; }

# 解析重定向，拿到真正的对象存储直链（绕过对 raw 的缓存）
resolve_redirect_url() {
  local url="$1"
  curl -fsSI --connect-timeout 8 --max-time 15 "$url" 2>/dev/null \
    | tr -d '\r' | awk 'tolower($1)=="location:"{print $2}' | tail -1
}

download_from_url() { # <url> <out> [validator-regex]
  local url="$1" out="$2" validator="${3:-}"
  if ! curl -fL --connect-timeout 10 --retry 1 --retry-delay 2 \
       --speed-time 20 --speed-limit 2048 --max-time 120 -o "$out.part" "$url"; then
    rm -f "$out.part"; return 1
  fi
  if [ -n "$validator" ] && ! grep -qE "$validator" "$out.part" 2>/dev/null; then
    p_warn "$(url_host "$url") 返回了过期/不完整内容，换源"
    rm -f "$out.part"; return 1
  fi
  mv -f "$out.part" "$out"; return 0
}

# 从 GitHub Contents API 拉文件（无 CDN，永远是最新 commit）
download_from_api() { # <repo-path> <ref> <out> [validator-regex]
  local rpath="$1" ref="$2" out="$3" validator="${4:-}"
  local base64_file="$out.b64"
  if ! curl -fsSL --connect-timeout 10 --max-time 60 \
       -H 'Accept: application/vnd.github.raw' -o "$base64_file" \
       "${GH_API}/${rpath}?ref=${ref}"; then
    rm -f "$base64_file"; return 1
  fi
  if [ -n "$validator" ] && ! grep -qE "$validator" "$base64_file" 2>/dev/null; then
    rm -f "$base64_file"; return 1
  fi
  mv -f "$base64_file" "$out"; return 0
}

os_download() { # 对象存储直链优先；失效则跳回 raw
  local out="$1" api_path="$2" ref="$3" validator="${4:-}"
  local raw="https://raw.githubusercontent.com/${REPO_SLUG}/${ref}/${api_path}"
  local direct="" host
  for prefix in "${GH_PROXIES[@]}"; do
    direct="$(resolve_redirect_url "${prefix}${raw}")" && [ -n "$direct" ] && break
  done
  if [ -n "$direct" ]; then
    host="$(url_host "$direct")"
    p_info "下载源: $host (对象存储直链)"
    if download_from_url "$direct" "$out" "$validator"; then return 0; fi
  fi
  for prefix in "${GH_PROXIES[@]}"; do
    host="${prefix#https://}"; host="${host%/}"
    [ -n "$host" ] || host="github.com(直连)"
    p_info "下载源: $host"
    if download_from_url "${prefix}${raw}?_cb=$(date +%s%N)" "$out" "$validator"; then return 0; fi
  done
  p_info "下载源: cdn.jsdelivr.net"
  if download_from_url "https://cdn.jsdelivr.net/gh/${REPO_SLUG}@${ref}/${api_path}" "$out" "$validator"; then return 0; fi
  p_info "下载源: api.github.com (无 CDN)"
  download_from_api "$api_path" "$ref" "$out" "$validator"
}

fetch_url() { # <最终URL> <输出路径> [validator-regex] —— 更新 install-precompiled.sh 用
  local raw="$1" out="$2" validator="${3:-}"
  local direct host prefix
  if [[ "$raw" == "https://raw.githubusercontent.com/${REPO_SLUG}/"* ]]; then
    local rest="${raw#https://raw.githubusercontent.com/${REPO_SLUG}/}"
    local ref="${rest%%/*}"; local api_path="${rest#*/}"
    os_download "$out" "$api_path" "$ref" "$validator"; return $?
  fi
  for prefix in "${GH_PROXIES[@]}"; do
    host="${prefix#https://}"; host="${host%/}"
    [ -n "$host" ] || host="github.com(直连)"
    p_info "下载源: $host"
    if download_from_url "${prefix}${raw}?_cb=$(date +%s%N)" "$out" "$validator"; then return 0; fi
  done
  return 1
}

# 预编译部署凭据在 app.env，源码部署在 .env
# 两个文件都是 600 且由 root 持有，非 root 运行时要提权读
env_get() {
  local f="$COMPOSE_DIR/.env"
  [ -f "$COMPOSE_DIR/app.env" ] && f="$COMPOSE_DIR/app.env"
  $SUDO grep -E "^$1=" "$f" 2>/dev/null | head -1 | cut -d= -f2-
}

app_url() { echo "http://127.0.0.1:$(env_get SERVER_PORT || echo 8080)"; }

wait_healthy() {
  local url; url="$(app_url)/health"
  p_info "等待服务就绪 ($url) ..."
  for i in $(seq 1 90); do
    if curl -sf --max-time 3 "$url" >/dev/null 2>&1; then p_ok "服务已就绪"; return 0; fi
    sleep 2
  done
  p_err "服务 90 次探测后仍未就绪，请用菜单 [5] 查看日志"; return 1
}

admin_login() {
  local email pass resp
  email="$(env_get ADMIN_EMAIL)"; pass="$(env_get ADMIN_PASSWORD)"
  resp=$(curl -sf --max-time 10 -X POST "$(app_url)/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$email\",\"password\":\"$pass\"}" 2>/dev/null) || return 1
  echo "$resp" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p'
}

# 首次使用管理员 API 需确认合规承诺（幂等）
accept_compliance() {
  local token="$1"
  curl -sf --max-time 10 -X POST "$(app_url)/api/v1/admin/compliance/accept" \
    -H "Authorization: Bearer $token" -H "Content-Type: application/json" \
    -d "{\"phrase\":\"$COMPLIANCE_PHRASE_ZH\",\"language\":\"zh\"}" >/dev/null 2>&1 || true
}

# ---------- 依赖安装 ----------
ensure_docker() {
  if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then return 0; fi
  p_warn "未检测到 Docker / compose 插件"
  if is_tty && ask_yn "是否自动安装 Docker（官方脚本 get.docker.com）？"; then
    curl -fsSL https://get.docker.com | $SUDO sh
    $SUDO systemctl enable --now docker 2>/dev/null || true
  else
    p_err "请先手动安装 Docker 后重试"; exit 1
  fi
}

ensure_repo() {
  resolve_compose_dir
  if [ -n "$COMPOSE_DIR" ]; then return 0; fi
  p_info "未在仓库内运行，克隆源码到 $CLONE_BASE ..."
  if ! command -v git >/dev/null 2>&1; then
    $SUDO apt-get update -qq && $SUDO apt-get install -y -qq git || { p_err "git 安装失败"; exit 1; }
  fi
  $SUDO git clone --depth 1 -b "$REPO_BRANCH" "$REPO_URL" "$CLONE_BASE"
  $SUDO chown -R "$(id -u):$(id -g)" "$CLONE_BASE" 2>/dev/null || true
  resolve_compose_dir
  [ -n "$COMPOSE_DIR" ] || { p_err "源码获取失败"; exit 1; }
}

rand_hex() { openssl rand -hex "$1" 2>/dev/null || head -c "$(( $1 * 2 ))" /dev/urandom | od -An -tx1 | tr -d ' \n' | head -c "$(( $1 * 2 ))"; }

# ---------- 1. 安装 ----------
cmd_install() {
  hr; echo -e "${CYAN}  一键安装 sub2api 破甲版${NC}"; hr
  resolve_compose_dir
  if [ "$INSTALL_MODE" = "precompiled" ]; then
    cmd_install_precompiled
    return
  fi
  ensure_docker
  ensure_repo

  if [ -f "$COMPOSE_DIR/.env" ]; then
    p_warn "检测到已有安装（.env 存在）"
    if is_tty && ask_yn "覆盖配置并重新安装？（数据卷保留）"; then :; else
      p_info "改为重建并重启服务..."
      compose up -d --build
      wait_healthy && cmd_status
      return
    fi
  fi

  echo ""
  p_info "基础配置（直接回车用括号内默认值）"
  local port email password persona enable_armor
  port=$(ask "服务端口 [8080]: " "8080"); port=${port:-8080}
  email=$(ask "管理员邮箱 [admin@sub2api.local]: " "admin@sub2api.local"); email=${email:-admin@sub2api.local}
  password=$(ask "管理员密码 [留空自动生成]: " "")
  if [ -z "$password" ]; then password=$(openssl rand -hex 12 2>/dev/null || head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n' | head -c 24); fi
  echo ""
  enable_armor="n"; persona="R.txt"
  if is_tty && ask_yn "安装完成后立即启用破甲？"; then
    enable_armor="y"
    persona=$(ask "默认人格文件名 [R.txt]: " "R.txt"); persona=${persona:-R.txt}
  fi

  p_info "生成密钥与 .env ..."
  cat > "$COMPOSE_DIR/.env" <<EOF
SERVER_PORT=$port
BIND_HOST=0.0.0.0
ADMIN_EMAIL=$email
ADMIN_PASSWORD=$password
POSTGRES_USER=sub2api
POSTGRES_PASSWORD=$(rand_hex 16)
POSTGRES_DB=sub2api
REDIS_PASSWORD=$(rand_hex 16)
JWT_SECRET=$(rand_hex 32)
TOTP_ENCRYPTION_KEY=$(rand_hex 32)
TZ=Asia/Shanghai
EOF
  chmod 600 "$COMPOSE_DIR/.env"

  p_info "构建镜像并启动（首次构建约 5-15 分钟，请耐心）..."
  compose up -d --build

  wait_healthy

  # 合规确认 + 可选启用破甲
  local token=""
  token=$(admin_login || true)
  if [ -n "$token" ]; then
    accept_compliance "$token"
    if [ "$enable_armor" = "y" ]; then
      p_info "启用破甲: persona=$persona mode=replace"
      curl -sf --max-time 10 -X PUT "$(app_url)/api/v1/admin/armor-break" \
        -H "Authorization: Bearer $token" -H "Content-Type: application/json" \
        -d "{\"enabled\":true,\"persona\":\"$persona\",\"mode\":\"replace\"}" >/dev/null \
        && p_ok "破甲已启用" || p_warn "破甲启用失败，可稍后通过菜单 [6] 重试"
    fi
  fi

  echo ""
  hr; p_ok "安装完成"; hr
  echo -e "  面板地址:   ${GREEN}http://<服务器IP>:$port${NC}"
  echo -e "  管理员邮箱: ${GREEN}$email${NC}"
  echo -e "  管理员密码: ${GREEN}$password${NC}"
  echo -e "  凭据备份:   $COMPOSE_DIR/.env (权限 600)"
  echo ""
  p_warn "首次登录管理员 API 的合规确认已自动完成"
  p_info "下一步: 面板里添加上游账号 → 创建 API key → 即可使用"
  p_info "绑定域名走菜单第 2 项; 状态检测走第 4 项"
  cmd_status || true
}

# 预编译部署的“重新安装” = 重跑免编译安装器（不编译、不覆盖 app.env、不动数据卷）
cmd_install_precompiled() {
  p_warn "检测到预编译部署: $PRECOMPILED_DIR"
  p_info "重装 = 下载最新发布包并覆盖二进制/人格库，app.env 与数据卷保持不变"
  if is_tty; then
    ask_yn "继续重新安装？" || { p_info "已取消"; return 0; }
  fi
  update_precompiled
}

# ---------- 2. 绑定域名 ----------
cmd_domain() {
  hr; echo -e "${CYAN}  绑定域名（Caddy 自动 TLS）${NC}"; hr
  resolve_compose_dir
  { [ -n "$COMPOSE_DIR" ] && { [ -f "$COMPOSE_DIR/.env" ] || [ -f "$COMPOSE_DIR/app.env" ]; }; } \
    || { p_err "尚未安装，请先执行安装"; exit 1; }

  if [ "$INSTALL_MODE" = "precompiled" ]; then
    local pport; pport="$(env_get SERVER_PORT)"; pport="${pport:-8080}"
    p_warn "预编译部署只有 postgres/redis 容器，不含内置 Caddy，无法自动签发证书"
    p_info "请在宿主机已有反向代理里配置，把域名转发到 http://127.0.0.1:$pport"
    echo "  Caddyfile 片段:"
    echo "    <你的域名> {"
    echo "        reverse_proxy 127.0.0.1:$pport"
    echo "    }"
    echo "  Nginx 片段:"
    echo "    location / { proxy_pass http://127.0.0.1:$pport; proxy_set_header Host \$host; proxy_set_header X-Forwarded-Proto \$scheme; }"
    p_info "改完后执行: sudo systemctl reload caddy  （或 nginx -s reload）"
    return 0
  fi

  is_tty || { p_err "绑定域名需要交互终端: bash install.sh domain"; exit 1; }

  local domain
  domain=$(ask "请输入域名（需已解析到本机 IP）: " "")
  [ -n "$domain" ] || { p_err "域名不能为空"; exit 1; }

  p_info "写入 Caddyfile ..."
  cat > "$COMPOSE_DIR/Caddyfile" <<EOF
$domain {
    reverse_proxy sub2api:8080
}
EOF

  p_info "启动 Caddy（占用 80/443 端口，自动签发证书）..."
  compose --profile domain up -d caddy

  p_info "验证 https://$domain ..."
  local ok=""
  for i in $(seq 1 15); do
    if curl -sf --max-time 5 "https://$domain/health" >/dev/null 2>&1; then ok=1; break; fi
    sleep 2
  done
  if [ -n "$ok" ]; then
    p_ok "域名已生效: https://$domain"
  else
    p_warn "暂未通过 https 检测: 证书可能仍在签发，或 80/443 未在防火墙/安全组放行"
    p_info "排查: 域名解析是否指向本机; 云厂商控制台放行 80/443; 稍后重跑菜单 [4] 检测"
  fi
}

# ---------- 3. 卸载 ----------
cmd_uninstall() {
  hr; echo -e "${CYAN}  卸载 sub2api 破甲版${NC}"; hr
  resolve_compose_dir
  [ -n "$COMPOSE_DIR" ] || { p_warn "未找到安装"; return 0; }

  if [ "$INSTALL_MODE" = "precompiled" ]; then
    is_tty && ! ask_yn "确认停止并停用 sub2api 服务？" && { p_info "已取消"; return 0; }
    p_info "停用 systemd 服务 ..."
    $SUDO systemctl disable --now sub2api >/dev/null 2>&1 || true
    $SUDO rm -f /etc/systemd/system/sub2api.service
    $SUDO systemctl daemon-reload >/dev/null 2>&1 || true
    compose down || true
    if is_tty && ask_yn "是否同时删除数据库/Redis 数据卷（数据将丢失）？"; then
      compose down -v || true
      p_ok "数据卷已删除"
    fi
    if [ -n "$PRECOMPILED_DIR" ] && [ "$PRECOMPILED_DIR" != "/" ] \
       && is_tty && ask_yn "是否删除应用目录 $PRECOMPILED_DIR（含 app.env 与人格库）？"; then
      $SUDO rm -rf "$PRECOMPILED_DIR"
      p_ok "应用目录已删除"
    fi
    p_ok "卸载完成"
    return 0
  fi

  is_tty && ! ask_yn "确认停止并移除容器？" && { p_info "已取消"; return 0; }

  compose --profile domain down || true
  if is_tty && ask_yn "是否同时删除数据卷（数据库/Redis 数据将丢失）？"; then
    compose --profile domain down -v || true
    p_ok "数据卷已删除"
  fi
  if [ "$COMPOSE_DIR" = "$CLONE_BASE/quickstart" ] && is_tty && ask_yn "是否删除源码目录 $CLONE_BASE？"; then
    $SUDO rm -rf "$CLONE_BASE"
    p_ok "源码目录已删除"
  fi
  p_ok "卸载完成"
}

# ---------- 4. 状态检测 ----------
cmd_status() {
  hr; echo -e "${CYAN}  运行状态检测${NC}"; hr
  resolve_compose_dir
  { [ -n "$COMPOSE_DIR" ] && { [ -f "$COMPOSE_DIR/.env" ] || [ -f "$COMPOSE_DIR/app.env" ]; }; } \
    || { p_err "尚未安装"; exit 1; }
  local env_file="$COMPOSE_DIR/.env"
  [ -f "$COMPOSE_DIR/app.env" ] && env_file="$COMPOSE_DIR/app.env"

  echo ""; p_info "容器状态:"
  (cd "$COMPOSE_DIR" && $SUDO docker ps --filter name=sub2api --format '  {{.Names}}\t{{.Status}}' 2>/dev/null) || true

  if [ "$INSTALL_MODE" = "precompiled" ]; then
    echo ""; p_info "应用服务:"
    if systemctl is-active --quiet sub2api; then
      p_ok "systemd sub2api → $(systemctl is-active sub2api) / $(systemctl is-enabled sub2api 2>/dev/null || echo unknown)"
    else
      p_err "systemd sub2api → 未运行（排查: systemctl status sub2api）"
    fi
  fi

  echo ""; p_info "健康检查:"
  if curl -sf --max-time 5 "$(app_url)/health" >/dev/null 2>&1; then
    p_ok "$(app_url)/health → ok"
  else
    p_err "$(app_url)/health → 无响应"; return 1
  fi

  p_info "管理员登录 + 破甲状态:"
  local token; token=$(admin_login || true)
  if [ -z "$token" ]; then
    p_err "管理员登录失败（密码被改过？见 $env_file）"
  else
    accept_compliance "$token"
    local state
    state=$(curl -sf --max-time 10 "$(app_url)/api/v1/admin/armor-break" -H "Authorization: Bearer $token" 2>/dev/null || echo "{}")
    echo "$state" | grep -q '"enabled":true' \
      && p_ok "破甲: 已启用 ($(echo "$state" | sed -n 's/.*"persona":"\([^"]*\)".*/\1/p'), $(echo "$state" | sed -n 's/.*"mode":"\([^"]*\)".*/\1/p'))" \
      || p_warn "破甲: 未启用（菜单 [6] 可开启）"
    local n
    n=$(echo "$state" | grep -o '"name"' | wc -l | tr -d ' ')
    p_ok "人格库: ${n} 个人格文件"
  fi

  p_info "网关链路探测（无 key 应返回 401）:"
  local code
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 8 -X POST "$(app_url)/v1/messages" \
    -H "Content-Type: application/json" -d '{"model":"x","max_tokens":1,"messages":[]}' 2>/dev/null || echo 000)
  [ "$code" = "401" ] && p_ok "/v1/messages → 401（链路正常）" || p_warn "/v1/messages → $code（异常）"

  if [ -f "$COMPOSE_DIR/Caddyfile" ]; then
    local d; d=$(head -1 "$COMPOSE_DIR/Caddyfile" | awk '{print $1}')
    p_info "域名 https://$d :"
    curl -sf --max-time 8 "https://$d/health" >/dev/null 2>&1 && p_ok "https 可达" || p_warn "https 不可达"
  fi
  echo ""
}

# ---------- 5. 日志 ----------
cmd_logs() {
  resolve_compose_dir; [ -n "$COMPOSE_DIR" ] || { p_err "尚未安装"; exit 1; }
  if [ "$INSTALL_MODE" = "precompiled" ]; then
    # 预编译模式应用跑在宿主机 systemd 上，日志在 journald
    $SUDO journalctl -u sub2api -f -n 200
  else
    compose logs -f --tail=200 sub2api
  fi
}

# ---------- 6. 破甲开关 ----------
cmd_armor() {
  hr; echo -e "${CYAN}  破甲开关${NC}"; hr
  resolve_compose_dir
  { [ -n "$COMPOSE_DIR" ] && { [ -f "$COMPOSE_DIR/.env" ] || [ -f "$COMPOSE_DIR/app.env" ]; }; } \
    || { p_err "尚未安装"; exit 1; }
  is_tty || { p_err "需要交互终端"; exit 1; }
  local token; token=$(admin_login || true)
  [ -n "$token" ] || { p_err "管理员登录失败"; exit 1; }
  accept_compliance "$token"

  local base; base="$(app_url)/api/v1/admin/armor-break"
  local state; state=$(curl -sf --max-time 10 "$base" -H "Authorization: Bearer $token")
  echo "  当前: $(echo "$state" | grep -q '"enabled":true' && echo '已启用' || echo '未启用')"
  echo "  人格库:"
  echo "$state" | sed -n 's/.*"personas":\[//;s/\].*//p' | tr '}' '\n' | sed -n 's/.*"name":"\([^"]*\)".*/    - \1/p' | head -30
  echo ""
  if ask_yn "切换 启用/关闭 状态？"; then
    local persona mode enable
    if echo "$state" | grep -q '"enabled":true'; then
      enable=false; persona=$(echo "$state" | sed -n 's/.*"persona":"\([^"]*\)".*/\1/p')
      mode=$(echo "$state" | sed -n 's/.*"mode":"\([^"]*\)".*/\1/p')
    else
      enable=true
      persona=$(ask "人格文件名 [R.txt]: " "R.txt"); persona=${persona:-R.txt}
      mode=$(ask "注入模式 replace/prepend [replace]: " "replace"); mode=${mode:-replace}
    fi
    curl -sf --max-time 10 -X PUT "$base" -H "Authorization: Bearer $token" -H "Content-Type: application/json" \
      -d "{\"enabled\":$enable,\"persona\":\"$persona\",\"mode\":\"${mode:-replace}\"}" >/dev/null \
      && p_ok "已更新: enabled=$enable persona=$persona" || p_err "更新失败"
  fi
}

# ---------- 7. 更新 ----------
update_source() {
  local repo_dir; repo_dir="$(cd "$COMPOSE_DIR/.." && pwd)"
  if [ -d "$repo_dir/.git" ]; then
    p_info "拉取最新代码 ($repo_dir) ..."
    git -C "$repo_dir" pull --ff-only || p_warn "拉取失败（本地有改动？），仍将继续重建"
  fi
  p_info "重建镜像并重启 ..."
  compose up -d --build
  wait_healthy && p_ok "更新完成"
}

# 预编译模式更新：重新拉取免编译安装器 → 下载并校验发布包 → 换二进制/人格库 → 重启 systemd
# 全程不编译；app.env 原样保留；数据库数据卷不动
update_precompiled() {
  local work="/tmp/sub2api-precompiled-update"
  local env_file="$PRECOMPILED_DIR/app.env"
  local env_backup="$work/app.env.backup"
  local before="" after=""

  rm -rf "$work"; mkdir -p "$work"

  p_info "拉取最新免编译安装器 ..."
  if ! fetch_url "$PRECOMPILED_SCRIPT_URL" "$work/install-precompiled.sh" 'install-precompiled|sub2api-armor'; then
    p_err "所有下载源均失败，更新中止（现有版本继续运行）"; exit 1
  fi
  if ! grep -q 'SHA256=' "$work/install-precompiled.sh" || ! bash -n "$work/install-precompiled.sh"; then
    p_err "下载到的安装器不完整（疑似代理劫持），更新中止"; exit 1
  fi

  if [ -f "$env_file" ]; then
    $SUDO cp -a "$env_file" "$env_backup"
    before="$($SUDO sha256sum "$env_file" | awk '{print $1}')"
  fi

  p_info "执行免编译升级（下载约 38 MB 发布包并校验 SHA256）..."
  $SUDO bash "$work/install-precompiled.sh"

  if [ -f "$env_file" ]; then
    after="$($SUDO sha256sum "$env_file" | awk '{print $1}')"
    if [ "$before" != "$after" ]; then
      p_warn "升级过程改动了 app.env，已从备份还原（避免换密钥导致旧数据不可读）"
      $SUDO cp -a "$env_backup" "$env_file"
      $SUDO chmod 600 "$env_file"
      $SUDO systemctl restart sub2api
    fi
  fi
  rm -f "$env_backup"

  wait_healthy && p_ok "更新完成（app.env 与数据卷未改动）"
  cmd_status || true
}

cmd_update() {
  hr; echo -e "${CYAN}  更新到最新版本${NC}"; hr
  resolve_compose_dir; [ -n "$COMPOSE_DIR" ] || { p_err "尚未安装"; exit 1; }
  if [ "$INSTALL_MODE" = "precompiled" ]; then
    update_precompiled
  else
    update_source
  fi
}

# ---------- 菜单 ----------
menu() {
  while true; do
    echo ""
    hr
    echo -e "${CYAN}   sub2api 破甲版 · 管理面板${NC}"
    hr
    echo "    1) 一键安装 / 重新安装"
    echo "    2) 绑定域名（自动 HTTPS）"
    echo "    3) 卸载"
    echo "    4) 状态检测"
    echo "    5) 查看日志"
    echo "    6) 破甲开关 / 切换人格"
    echo "    7) 更新到最新版本"
    echo "    0) 退出"
    echo ""
    local c; c=$(ask "请选择 [0-7]: " "0")
    case "$c" in
      1) cmd_install ;;
      2) cmd_domain ;;
      3) cmd_uninstall ;;
      4) cmd_status ;;
      5) cmd_logs ;;
      6) cmd_armor ;;
      7) cmd_update ;;
      0|q|exit) echo "再见"; exit 0 ;;
      *) p_warn "无效选择" ;;
    esac
  done
}

case "${1:-}" in
  install)   cmd_install ;;
  domain)    cmd_domain ;;
  uninstall) cmd_uninstall ;;
  status)    cmd_status ;;
  logs)      cmd_logs ;;
  armor)     cmd_armor ;;
  update)    cmd_update ;;
  menu|"")   menu ;;
  -h|--help)
    echo "用法: bash install.sh [install|domain|uninstall|status|logs|armor|update|menu]"
    ;;
  *) p_err "未知命令: $1"; exit 1 ;;
esac
