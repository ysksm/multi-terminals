#!/usr/bin/env bash
#
# multi-terminals Wails デスクトップ版 ビルド/開発スクリプト
#
# フロントエンド(UI)を埋め込み用ディレクトリに配置してから、Wails の
# デスクトップアプリ(apps/wails)をビルド/起動する。
#
# 使い方:
#   scripts/build-wails.sh [command] [platform]
#
# command:
#   build   デスクトップアプリをビルド（既定）
#   dev     wails dev で開発起動（ホットリロード）
#
# platform（build 時のみ。既定は実行 OS）:
#   省略    実行 OS 向け（macOS=darwin/universal, Windows=windows/amd64）
#   任意    wails の -platform 値をそのまま指定（例: darwin/arm64, windows/amd64）
#
# 注意:
#   - Wails は **クロスコンパイル不可**。Windows 版は Windows 上、macOS 版は
#     macOS 上でのみビルドできる。実行 OS と異なる OS を指定すると警告する。
#   - wails CLI が必要: go install github.com/wailsapp/wails/v2/cmd/wails@latest
#   - 成果物は apps/wails/build/bin/ に生成し、release/ にもコピーする。
#
set -euo pipefail

# リポジトリルートへ移動（どこから呼ばれても動くように）
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT"

if ! command -v go >/dev/null 2>&1; then
  echo "error: go コマンドが見つかりません。Go 1.26 以上をインストールしてください。" >&2
  exit 1
fi

if ! command -v wails >/dev/null 2>&1; then
  echo "error: wails コマンドが見つかりません。" >&2
  echo "  go install github.com/wailsapp/wails/v2/cmd/wails@latest" >&2
  exit 1
fi

COMMAND="${1:-build}"
PLATFORM="${2:-}"
OUTDIR="release"

# --- フロントエンドを本番ビルドして埋め込みディレクトリへ配置 ---
# apps/web/webui が //go:embed all:dist で UI を取り込むため、Wails ビルド前に
# 必ず dist を用意する（用意しないと UI 非組み込みの案内ページになる）。
embed_frontend() {
  if [ ! -d "frontend" ]; then
    echo ">> frontend なし: UI 非組み込みでビルドします" >&2
    return 0
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

# --- 実行 OS の既定 -platform 値を返す ---
default_platform() {
  case "$(uname -s)" in
    Darwin)            echo "darwin/universal" ;;
    MINGW*|MSYS*|CYGWIN*) echo "windows/amd64" ;;
    *)                 echo "" ;;
  esac
}

# --- 指定 platform が実行 OS でビルド可能かを警告する（不可でも続行はしない） ---
assert_buildable() {
  local platform="$1" host
  host="$(uname -s)"
  case "$platform" in
    darwin/*)
      if [ "$host" != "Darwin" ]; then
        echo "error: $platform は macOS 上でのみビルドできます（現在: $host）。" >&2
        echo "  Wails はクロスコンパイル不可です。macOS 環境か CI を使ってください。" >&2
        exit 1
      fi
      ;;
    windows/*)
      case "$host" in
        MINGW*|MSYS*|CYGWIN*) : ;;
        *)
          echo "error: $platform は Windows 上でのみビルドできます（現在: $host）。" >&2
          echo "  Wails はクロスコンパイル不可です。Windows 環境か CI を使ってください。" >&2
          exit 1
          ;;
      esac
      ;;
  esac
}

cmd_dev() {
  embed_frontend
  echo ">> (cd apps/wails && wails dev)"
  (cd apps/wails && wails dev)
}

cmd_build() {
  local platform="$PLATFORM"
  if [ -z "$platform" ]; then
    platform="$(default_platform)"
  fi
  if [ -z "$platform" ]; then
    echo "error: この OS ($(uname -s)) は Wails のビルド対象外です。" >&2
    exit 1
  fi
  assert_buildable "$platform"

  embed_frontend

  echo ">> (cd apps/wails && wails build -platform $platform -clean)"
  (cd apps/wails && wails build -platform "$platform" -clean)

  mkdir -p "$OUTDIR"
  if [ -d "apps/wails/build/bin" ]; then
    echo ">> copy apps/wails/build/bin/* -> $OUTDIR/"
    cp -R apps/wails/build/bin/. "$OUTDIR/"
  else
    echo "error: ビルド成果物 apps/wails/build/bin が見つかりません。" >&2
    exit 1
  fi

  echo ""
  echo "✅ Wails デスクトップ版をビルドしました（platform=$platform）"
  echo "   成果物: $ROOT/apps/wails/build/bin/  （$OUTDIR/ にもコピー済み）"
}

case "$COMMAND" in
  build) cmd_build ;;
  dev)   cmd_dev ;;
  *)
    echo "unknown command: $COMMAND (build|dev)" >&2
    exit 1
    ;;
esac
