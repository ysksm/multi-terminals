#!/usr/bin/env bash
#
# multi-terminals 全アプリ一括ビルドスクリプト
#
# 使い方:
#   scripts/build-all.sh [target]
#
# target:
#   all    web + wails（既定）
#   web    web サーバ版バイナリのみ（darwin/windows をクロスビルド）
#   wails  Wails デスクトップ版のみ（実行 OS 向け）
#
# 注意: Wails はクロスコンパイル不可。Windows 版は Windows 上、macOS 版は
#       macOS 上でのみビルドできる。非対象 OS は警告してスキップする。
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT"

TARGET="${1:-all}"
OUTDIR="release"
mkdir -p "$OUTDIR"

if ! command -v go >/dev/null 2>&1; then
  echo "error: go コマンドが見つかりません。" >&2
  exit 1
fi

HOST_OS="$(uname -s)"

# --- フロントエンドをビルドして埋め込みディレクトリへ配置 ---
build_frontend() {
  if [ ! -d "frontend" ]; then
    echo ">> frontend なし: UI 非組み込みでビルドします"
    return 0
  fi
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
}

# --- web サーバ版バイナリ（クロスビルド可能） ---
build_web() {
  echo ">> web: クロスビルド開始"
  local targets=(
    "darwin arm64 multi-terminals-darwin-arm64"
    "darwin amd64 multi-terminals-darwin-amd64"
    "windows amd64 multi-terminals-windows-amd64.exe"
    "windows arm64 multi-terminals-windows-arm64.exe"
  )
  local t goos goarch out
  for t in "${targets[@]}"; do
    read -r goos goarch out <<<"$t"
    echo "   - $goos/$goarch -> $out"
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
      go build -trimpath -ldflags "-s -w" -o "$OUTDIR/$out" ./apps/web/cmd
  done
  # macOS 上なら universal も作る
  if command -v lipo >/dev/null 2>&1; then
    echo "   - darwin/universal (lipo)"
    lipo -create \
      "$OUTDIR/multi-terminals-darwin-arm64" \
      "$OUTDIR/multi-terminals-darwin-amd64" \
      -output "$OUTDIR/multi-terminals-darwin-universal"
  fi
}

# --- wails CLI を解決する（PATH → $GOBIN → $(go env GOPATH)/bin の順） ---
# `go install` 直後で GOPATH/bin が PATH に無くても見つけられるようにする。
WAILS=""
resolve_wails() {
  if command -v wails >/dev/null 2>&1; then
    WAILS="$(command -v wails)"
    return 0
  fi
  local gobin
  gobin="$(go env GOBIN 2>/dev/null)"
  if [ -n "$gobin" ] && [ -x "$gobin/wails" ]; then
    WAILS="$gobin/wails"
    return 0
  fi
  local gopath
  gopath="$(go env GOPATH 2>/dev/null)"
  if [ -n "$gopath" ] && [ -x "$gopath/bin/wails" ]; then
    WAILS="$gopath/bin/wails"
    return 0
  fi
  return 1
}

# --- Wails デスクトップ版（実行 OS 向けのみ） ---
build_wails() {
  # 初回ビルド対応: wails CLI が無ければ自動インストールを試みる
  if ! resolve_wails; then
    echo ">> wails CLI 未導入のためインストールします"
    echo ">> go install github.com/wailsapp/wails/v2/cmd/wails@latest"
    go install github.com/wailsapp/wails/v2/cmd/wails@latest || true
  fi
  if ! resolve_wails; then
    echo ">> [skip] wails CLI を導入できなかったため Wails ビルドをスキップします。" >&2
    echo "   手動導入: go install github.com/wailsapp/wails/v2/cmd/wails@latest" >&2
    echo "   まとめてセットアップするには scripts/init.sh を実行してください。" >&2
    return 0
  fi
  case "$HOST_OS" in
    Darwin)
      echo ">> wails: darwin/universal をビルド ($WAILS)"
      (cd apps/wails && "$WAILS" build -platform darwin/universal -clean)
      # 生成物 (apps/wails/build/bin/*.app) を release/ へコピー
      if [ -d "apps/wails/build/bin" ]; then
        cp -R apps/wails/build/bin/. "$OUTDIR/"
      else
        echo "error: apps/wails/build/bin が見つかりません。Wails ビルドが失敗した可能性があります。" >&2
        exit 1
      fi
      ;;
    MINGW*|MSYS*|CYGWIN*)
      echo ">> wails: windows/amd64 をビルド ($WAILS)"
      (cd apps/wails && "$WAILS" build -platform windows/amd64 -clean)
      if [ -d "apps/wails/build/bin" ]; then
        cp -R apps/wails/build/bin/. "$OUTDIR/"
      else
        echo "error: apps/wails/build/bin が見つかりません。Wails ビルドが失敗した可能性があります。" >&2
        exit 1
      fi
      ;;
    *)
      echo ">> [skip] この OS ($HOST_OS) では Wails ビルド対象外です。" >&2
      echo "   Windows 版は Windows 上、macOS 版は macOS 上で実行するか CI を使ってください。" >&2
      ;;
  esac
}

# --- チェックサム ---
write_checksums() {
  echo ">> チェックサム生成: $OUTDIR/SHA256SUMS.txt"
  (
    cd "$OUTDIR"
    files=$(find . -maxdepth 1 -type f ! -name 'SHA256SUMS*' ! -name 'NOTES.md' -print | sed 's|^\./||' | sort)
    : >SHA256SUMS.txt
    if [ -n "$files" ]; then
      if command -v shasum >/dev/null 2>&1; then
        echo "$files" | xargs shasum -a 256 >SHA256SUMS.txt
      else
        echo "$files" | xargs sha256sum >SHA256SUMS.txt
      fi
    fi
  )
  cat "$OUTDIR/SHA256SUMS.txt" || true
}

case "$TARGET" in
  web)
    build_frontend
    build_web
    ;;
  wails)
    build_frontend
    build_wails
    ;;
  all)
    build_frontend
    build_web
    build_wails
    ;;
  *)
    echo "unknown target: $TARGET (all|web|wails)" >&2
    exit 1
    ;;
esac

write_checksums

echo ""
echo "✅ ビルド完了: $ROOT/$OUTDIR"
ls -lh "$OUTDIR" || true
