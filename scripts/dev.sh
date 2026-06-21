#!/usr/bin/env bash
#
# multi-terminals 開発用実行スクリプト
#
# 使い方:
#   scripts/dev.sh <command>
#
# コマンド:
#   test     全パッケージのテストを実行
#   cover    カバレッジ付きでテストを実行（coverage.out を生成）
#   vet      go vet を実行
#   build    全パッケージをビルド
#   check    build + vet + test をまとめて実行（CI 相当）
#   web      Web アダプタを起動（apps/web が存在する場合）
#   wails    Wails アプリを開発モードで起動（apps/wails が存在する場合）
#   help     このヘルプを表示
#
set -euo pipefail

# リポジトリルートへ移動（どこから呼ばれても動くように）
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT"

# go の存在確認
if ! command -v go >/dev/null 2>&1; then
  echo "error: go コマンドが見つかりません。Go 1.26 以上をインストールしてください。" >&2
  exit 1
fi

cmd_test() {
  echo ">> go test ./..."
  go test ./...
}

cmd_cover() {
  echo ">> go test -coverprofile=coverage.out ./..."
  go test -coverprofile=coverage.out ./...
  echo ">> カバレッジ概要:"
  go tool cover -func=coverage.out | tail -n 1
  echo "HTML レポート: go tool cover -html=coverage.out"
}

cmd_vet() {
  echo ">> go vet ./..."
  go vet ./...
}

cmd_build() {
  echo ">> go build ./..."
  go build ./...
}

cmd_check() {
  cmd_build
  cmd_vet
  cmd_test
  echo ">> check: OK"
}

cmd_web() {
  if [ ! -d "apps/web" ]; then
    echo "apps/web はまだ存在しません（Web アダプタは今後の実装計画です）。" >&2
    exit 1
  fi
  echo ">> go run ./apps/web"
  go run ./apps/web "$@"
}

cmd_wails() {
  if [ ! -d "apps/wails" ]; then
    echo "apps/wails はまだ存在しません（Wails アプリは今後の実装計画です）。" >&2
    exit 1
  fi
  if ! command -v wails >/dev/null 2>&1; then
    echo "error: wails コマンドが見つかりません。" >&2
    echo "  go install github.com/wailsapp/wails/v2/cmd/wails@latest" >&2
    exit 1
  fi
  echo ">> (cd apps/wails && wails dev)"
  (cd apps/wails && wails dev "$@")
}

cmd_help() {
  # 先頭のシェバング行を飛ばし、連続するコメント行（ヘッダー）だけを表示する
  awk 'NR==1{next} /^#/{sub(/^# ?/,""); print; next} {exit}' "${BASH_SOURCE[0]}"
}

main() {
  local sub="${1:-help}"
  shift || true
  case "$sub" in
    test)  cmd_test "$@" ;;
    cover) cmd_cover "$@" ;;
    vet)   cmd_vet "$@" ;;
    build) cmd_build "$@" ;;
    check) cmd_check "$@" ;;
    web)   cmd_web "$@" ;;
    wails) cmd_wails "$@" ;;
    help|-h|--help) cmd_help ;;
    *)
      echo "unknown command: $sub" >&2
      echo "" >&2
      cmd_help >&2
      exit 1
      ;;
  esac
}

main "$@"
