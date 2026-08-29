#!/usr/bin/env bash
# sdkz 一键安装脚本（Linux / macOS）
# 用法:
#   curl -fsSL https://raw.githubusercontent.com/bigwg/sdkz/main/scripts/install.sh | bash
# 环境变量:
#   SDKZ_INSTALL_DIR  安装目录（默认 ~/.local/bin）
set -euo pipefail

REPO="bigwg/sdkz"
BINARY="sdkz"
INSTALL_DIR="${SDKZ_INSTALL_DIR:-$HOME/.local/bin}"

err() { echo "错误: $*" >&2; exit 1; }

# 1. 探测平台
OS="$(uname -s 2>/dev/null | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m 2>/dev/null)"
case "$ARCH" in
  x86_64|amd64)  ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) err "不支持的架构: $ARCH" ;;
esac
case "$OS" in
  linux|darwin) : ;;
  *) err "不支持的系统: $OS（仅支持 Linux / macOS；Windows 请用 install.ps1）" ;;
esac

ASSET="sdkz-${OS}-${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"
echo "检测平台: ${OS}/${ARCH}"

# 2. 下载
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
echo "下载 $URL"
if command -v curl >/dev/null 2>&1; then
  curl -fSL "$URL" -o "$TMP/$ASSET" || err "下载失败（请确认已发布 release: https://github.com/${REPO}/releases）"
elif command -v wget >/dev/null 2>&1; then
  wget -O "$TMP/$ASSET" "$URL" || err "下载失败"
else
  err "需要 curl 或 wget"
fi

# 3. 解压
tar -xzf "$TMP/$ASSET" -C "$TMP"
NEW="$TMP/$BINARY"
[ -x "$NEW" ] || err "解压后未找到可执行文件 $BINARY"

# 4. 安装目录（无写权限时回退系统目录）
if [ ! -d "$INSTALL_DIR" ]; then
  mkdir -p "$INSTALL_DIR" 2>/dev/null || INSTALL_DIR="/usr/local/bin"
fi
if [ ! -w "$INSTALL_DIR" ]; then
  if [ -w "$(dirname "$INSTALL_DIR")" ]; then
    INSTALL_DIR="/usr/local/bin"
  else
    err "无写入权限: $INSTALL_DIR（可设置 SDKZ_INSTALL_DIR 指向可写目录）"
  fi
fi
install -m 0755 "$NEW" "$INSTALL_DIR/$BINARY" || err "安装失败"
echo "已安装到 $INSTALL_DIR/$BINARY"

# 5. PATH 检查
case ":$PATH:" in
  *":$INSTALL_DIR:"*) : ;;
  *) echo "提示: 请将 $INSTALL_DIR 加入 PATH（如 export PATH=\"$INSTALL_DIR:\$PATH\"）" ;;
esac

# 6. shell 集成
"$INSTALL_DIR/$BINARY" init >/dev/null 2>&1 || true

echo "完成！运行 'sdkz list java' 开始使用（已安装版本升级用 'sdkz selfupdate'）。"
