#!/usr/bin/env bash
#
# multi-terminals 初回セットアップスクリプト
#
# クローン直後に一度実行すると、ビルドに必要な依存物を一括で揃える。
# 各ビルドスクリプト（build-all.sh / build-wails.sh / dev.sh build）も
# 不足分を自動導入するが、まとめて先に済ませたい場合はこれを使う。
#
# 使い方:
#   scripts/init.sh [--no-wails]
#
# やること:
#   1. go / node / npm の存在チェック（無ければ導入方法を案内）
#   2. Go モジュールのダウンロード (go mod download)
#   3. フロントエンド依存のインストール (cd frontend && npm install)
#   4. wails CLI のインストール（未導入の場合。--no-wails でスキップ）
#
set -euo pipefail

# リポジトリルートへ移動（どこから呼ばれても動くように）
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT"

WITH_WAILS=1
for arg in "$@"; do
  case "$arg" in
    --no-wails) WITH_WAILS=0 ;;
    -h|--help)
      awk 'NR==1{next} /^#/{sub(/^# ?/,""); print; next} {exit}' "${BASH_SOURCE[0]}"
      exit 0
      ;;
    *)
      echo "unknown option: $arg (--no-wails|--help)" >&2
      exit 1
      ;;
  esac
done

# --- 1. ツールチェーンの確認 ------------------------------------------------

# go: PATH に無ければ公式インストーラ/Homebrew の既定位置も探す
if ! command -v go >/dev/null 2>&1; then
  for cand in /usr/local/go/bin /opt/homebrew/bin; do
    if [ -x "$cand/go" ]; then
      export PATH="$PATH:$cand"
      break
    fi
  done
fi
if ! command -v go >/dev/null 2>&1; then
  {
    echo "error: go コマンドが見つかりません。Go 1.26 以上をインストールしてください。"
    echo "  https://go.dev/dl/  または  brew install go"
  } >&2
  exit 1
fi
echo ">> go:   $(go version)"

# node / npm（フロントエンドのビルドに必要）
if ! command -v npm >/dev/null 2>&1; then
  {
    echo "error: npm コマンドが見つかりません。Node.js 20 以上をインストールしてください。"
    echo "  https://nodejs.org/  または  brew install node"
  } >&2
  exit 1
fi
echo ">> node: $(node --version) / npm $(npm --version)"
node_major="$(node --version | sed 's/^v//' | cut -d. -f1)"
if [ "$node_major" -lt 20 ] 2>/dev/null; then
  echo "warning: Node.js 20 以上を推奨します（現在: $(node --version)）。" >&2
fi

# --- 2. Go モジュール --------------------------------------------------------

echo ">> go mod download"
go mod download

# --- 3. フロントエンド依存 ----------------------------------------------------

if [ -d "frontend" ]; then
  echo ">> (cd frontend && npm install)"
  (cd frontend && npm install)
else
  echo ">> frontend なし: スキップ"
fi

# --- 4. wails CLI（デスクトップ版のビルドに必要） ------------------------------

# PATH → $GOBIN → $(go env GOPATH)/bin の順で探索する
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

if [ "$WITH_WAILS" = "1" ]; then
  if resolve_wails; then
    echo ">> wails: 導入済み ($WAILS)"
  else
    echo ">> go install github.com/wailsapp/wails/v2/cmd/wails@latest"
    go install github.com/wailsapp/wails/v2/cmd/wails@latest
    if resolve_wails; then
      echo ">> wails: インストールしました ($WAILS)"
      case ":$PATH:" in
        *":$(dirname "$WAILS"):"*) : ;;
        *)
          echo "   note: $(dirname "$WAILS") が PATH にありません。直接 wails を使う場合は追加してください:"
          echo "         export PATH=\"\$PATH:$(dirname "$WAILS")\""
          echo "   （scripts/ 配下のビルドスクリプトは PATH に無くても自動探索します）"
          ;;
      esac
    else
      echo "error: wails のインストールに失敗しました。" >&2
      exit 1
    fi
  fi
else
  echo ">> wails: --no-wails 指定のためスキップ"
fi

# --- 完了 ---------------------------------------------------------------------

echo ""
echo "✅ セットアップ完了。次の一歩:"
echo "   ./scripts/dev.sh build        # UI 組み込みの単一バイナリ (bin/multi-terminals)"
echo "   ./scripts/build-wails.sh      # デスクトップ版（Wails）をビルド"
echo "   ./scripts/build-all.sh        # web + wails を一括ビルド (release/)"
echo "   ./scripts/dev.sh help         # 開発コマンド一覧"
