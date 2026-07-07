#!/usr/bin/env bash
#
# multi-terminals macOS 向けリリースビルドスクリプト
#
# フロントエンド(UI)を組み込んだ単一実行ファイルを macOS 向けに生成する。
# Windows 版リリースと同じく release/ に成果物とチェックサムを出力する。
#
# 使い方:
#   scripts/build-mac.sh [arch]
#
# arch:
#   both       arm64 と amd64 の両方 + universal を生成（既定）
#   arm64      Apple Silicon 向けのみ
#   amd64      Intel Mac 向けのみ
#   universal  arm64 + amd64 をまとめた universal バイナリのみ
#
# 生成物 (release/):
#   multi-terminals-darwin-arm64
#   multi-terminals-darwin-amd64
#   multi-terminals-darwin-universal   （lipo がある場合）
#   SHA256SUMS-darwin.txt
#
set -euo pipefail

# リポジトリルートへ移動（どこから呼ばれても動くように）
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT"

if ! command -v go >/dev/null 2>&1; then
  echo "error: go コマンドが見つかりません。Go 1.26 以上をインストールしてください。" >&2
  exit 1
fi

ARCH="${1:-both}"
OUTDIR="release"
BASENAME="multi-terminals-darwin"
PKG="./apps/web/cmd"

# --- フロントエンドを本番ビルドしてサーバーバイナリに組み込む ---
embed_frontend() {
  if [ -d "frontend" ]; then
    if ! command -v npm >/dev/null 2>&1; then
      echo "error: npm コマンドが見つかりません。Node.js 20 以上をインストールしてください。" >&2
      echo "  https://nodejs.org/  または  brew install node" >&2
      exit 1
    fi
    if [ ! -d "frontend/node_modules" ]; then
      echo ">> (cd frontend && npm install)"
      (cd frontend && npm install)
    fi
    echo ">> (cd frontend && npm run build)"
    (cd frontend && npm run build)
    echo ">> embed frontend/dist -> apps/web/webui/dist"
    rm -rf apps/web/webui/dist
    mkdir -p apps/web/webui/dist
    touch apps/web/webui/dist/.gitkeep
    cp -R frontend/dist/. apps/web/webui/dist/
  else
    echo ">> frontend なし: UI 非組み込みでビルドします"
  fi
}

# --- 指定アーキテクチャ向けにクロスコンパイル ---
build_arch() {
  local goarch="$1"
  local out="$OUTDIR/$BASENAME-$goarch"
  echo ">> GOOS=darwin GOARCH=$goarch go build -o $out $PKG"
  CGO_ENABLED=0 GOOS=darwin GOARCH="$goarch" \
    go build -trimpath -ldflags "-s -w" -o "$out" "$PKG"
}

# --- universal バイナリを作成（lipo が必要） ---
build_universal() {
  local out="$OUTDIR/$BASENAME-universal"
  if ! command -v lipo >/dev/null 2>&1; then
    echo ">> lipo が見つからないため universal バイナリはスキップします" >&2
    return 0
  fi
  echo ">> lipo -create -> $out"
  lipo -create \
    "$OUTDIR/$BASENAME-arm64" \
    "$OUTDIR/$BASENAME-amd64" \
    -output "$out"
}

# --- release/ 内の darwin バイナリの SHA-256 を出力 ---
write_checksums() {
  local sumfile="$OUTDIR/SHA256SUMS-darwin.txt"
  echo ">> チェックサム生成: $sumfile"
  (
    cd "$OUTDIR"
    # macOS は shasum、それ以外は sha256sum を使う
    if command -v shasum >/dev/null 2>&1; then
      shasum -a 256 "$BASENAME"-* >"SHA256SUMS-darwin.txt"
    else
      sha256sum "$BASENAME"-* >"SHA256SUMS-darwin.txt"
    fi
  )
  cat "$sumfile"
}

mkdir -p "$OUTDIR"
embed_frontend

case "$ARCH" in
  arm64)
    build_arch arm64
    ;;
  amd64)
    build_arch amd64
    ;;
  universal)
    build_arch arm64
    build_arch amd64
    build_universal
    ;;
  both)
    build_arch arm64
    build_arch amd64
    build_universal
    ;;
  *)
    echo "unknown arch: $ARCH (both|arm64|amd64|universal)" >&2
    exit 1
    ;;
esac

write_checksums

echo ""
echo "✅ macOS 向け成果物を $ROOT/$OUTDIR に生成しました（UI 組み込みの単一バイナリ）"
echo "   起動例: ./$OUTDIR/$BASENAME-arm64   （その後 http://localhost:8080 を開く）"
