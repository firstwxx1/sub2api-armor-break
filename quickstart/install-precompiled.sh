#!/usr/bin/env bash
# =============================================================================
# Sub2API armor-break — 免编译一键部署（下载预编译产物）
# 用法: curl -sSL <URL> | sudo bash
# =============================================================================
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "请使用 root 运行: curl -sSL <URL> | sudo bash" >&2
  exit 1
fi

OWNER="firstwxx1"
REPO="sub2api-armor-break"
TAG="v0.2.9-armor1"
ASSET="sub2api-armor-local.tar.gz"

# GitHub 直连，失败自动降级到两个国内可用代理
URLS=(
  "https://github.com/${OWNER}/${REPO}/releases/download/${TAG}/${ASSET}"
  "https://ghfast.top/https://github.com/${OWNER}/${REPO}/releases/download/${TAG}/${ASSET}"
  "https://gh-proxy.com/https://github.com/${OWNER}/${REPO}/releases/download/${TAG}/${ASSET}"
)

WORK_DIR="/tmp/sub2api-precompiled"
mkdir -p "$WORK_DIR"

echo "[*] 开始下载预编译包 (${ASSET}) ..."
SUCCESS=0
for url in "${URLS[@]}"; do
  echo "[*] 尝试源: $url"
  if curl -L --connect-timeout 10 --max-time 600 -o "$WORK_DIR/$ASSET" "$url"; then
    if [ -s "$WORK_DIR/$ASSET" ]; then
      SUCCESS=1
      break
    fi
  fi
  echo "[!] 该源失败，换下一个..."
  rm -f "$WORK_DIR/$ASSET"
done

if [ "$SUCCESS" -ne 1 ]; then
  echo "[!] 所有下载源均失败。" >&2
  echo "    请手动下载 ${ASSET} 并放到 ${WORK_DIR}/" >&2
  exit 1
fi

echo "[*] 解压中..."
tar -xzf "$WORK_DIR/$ASSET" -C "$WORK_DIR"

echo "[*] 开始免编译部署..."
bash "$WORK_DIR/deploy/setup.sh"