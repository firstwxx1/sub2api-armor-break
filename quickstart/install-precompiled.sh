#!/usr/bin/env bash
# =============================================================================
# Sub2API armor-break — 免编译一键部署
#
#   下载预编译产物，目标机只解包 + 起两个容器，不做任何编译。
#   用法：
#     curl -sSL https://raw.githubusercontent.com/firstwxx1/sub2api-armor-break/main/quickstart/install-precompiled.sh | sudo bash
# =============================================================================
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "请使用 root 运行: curl -sSL <URL> | sudo bash" >&2
  exit 1
fi

TAG="v0.2.9-armor1"
ASSET="sub2api-armor-local.tar.gz"
SHA256="f908943324f657c87e84a4086795fc8521b2026efc8dfa26d9a679f7ec513c8a"
GH="https://github.com/firstwxx1/sub2api-armor-break/releases/download/${TAG}/${ASSET}"

# 按实测速度排序：gh-proxy.com 最快，直连最慢
SOURCES=(
  "https://gh-proxy.com/${GH}"
  "https://ghfast.top/${GH}"
  "${GH}"
)

WORK_DIR="/tmp/sub2api-precompiled"
mkdir -p "$WORK_DIR"
cd "$WORK_DIR"
rm -f "$ASSET"

echo "======================================================="
echo " Sub2API armor-break 免编译部署 (${TAG})"
echo " 目标机不会编译任何代码"
echo "======================================================="

echo "[*] 下载 ${ASSET} (38 MB) ..."
OK=0
for url in "${SOURCES[@]}"; do
  host=$(echo "$url" | awk -F/ '{print $3}')
  echo "[*] 尝试: $host"
  if curl -fL --connect-timeout 10 --retry 2 --retry-delay 2 \
       --speed-time 20 --speed-limit 4096 \
       -o "${ASSET}.part" "$url"; then
    mv -f "${ASSET}.part" "$ASSET"
    OK=1
    break
  fi
  echo "[!] $host 失败或速度过慢，切换下一个源"
  rm -f "${ASSET}.part"
done

if [ "$OK" -ne 1 ]; then
  echo "[错误] 所有下载源均失败。" >&2
  echo "       请手动下载 ${ASSET} 后放到 ${WORK_DIR}/ 再重跑本脚本。" >&2
  exit 1
fi

echo "[*] 校验 SHA256 ..."
ACTUAL=$(sha256sum "$ASSET" | awk '{print $1}')
if [ "$ACTUAL" != "$SHA256" ]; then
  echo "[错误] 校验失败，产物可能不完整或被篡改。" >&2
  echo "       期望: $SHA256" >&2
  echo "       实际: $ACTUAL" >&2
  rm -f "$ASSET"
  exit 1
fi
echo "[成功] SHA256 校验通过"

echo "[*] 解压 ..."
rm -rf "$WORK_DIR/deploy"
tar -xzf "$ASSET"

# 残留的 /app/data 会顶掉 /opt/sub2api/data 的配置探测，导致跳过 AUTO_SETUP
if [ -e /app/data ]; then
  echo "[!] 检测到 /app/data 残留，移开以免抢占配置路径"
  mv /app/data "/app/data.bak.$(date +%Y%m%d%H%M%S)"
fi

echo "[*] 执行免编译部署 ..."
exec bash "$WORK_DIR/deploy/setup.sh"