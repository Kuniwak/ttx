#!/usr/bin/env bash
#
# install-ttx.sh — 会話用プロセス用 CLI（ttx-human / ttx-ai-send / ttx-ai-recv）を
# 取得・配置する冪等インストーラです。Go 環境は不要で、GitHub Release の
# ビルド済みバイナリ（goreleaser アセット）から OS/arch に合うものを取得し、
# PATH 上のディレクトリへ配置します。リポジトリが private なため取得には gh CLI を使います。
#
# 使い方:
#   bash scripts/install-ttx.sh
#
# 環境変数（主にテスト・上書き用。接頭辞は TTX_ に統一）:
#   TTX_REPO         取得元 GitHub リポジトリ（既定: Kuniwak/ttx）
#   TTX_VERSION      取得する Release タグ（既定: 最新 Release）
#   TTX_INSTALL_DIR  配置先ディレクトリ（既定: PATH 上の書込可ディレクトリ、無ければ ~/.local/bin）
#
# 終了コード: 0=成功（配置済み or 既に最新）、非0=失敗（原因と対処を標準エラーへ出力）。
set -euo pipefail

REPO="${TTX_REPO:-Kuniwak/ttx}"
BINARIES=(ttx-human ttx-ai-send ttx-ai-recv)

log() { printf '%s\n' "$*" >&2; }
die() { printf 'エラー: %s\n' "$*" >&2; exit 1; }

# main で mktemp する一時ディレクトリ。EXIT トラップで掃除するため関数 local ではなく
# グローバルにする（local だとトラップ実行時にスコープ外となり set -u に触れて失敗する）。
tmp=""
cleanup() {
  [ -n "${tmp:-}" ] && rm -rf "$tmp"
  return 0
}
trap cleanup EXIT

# gh が無ければ OS に応じて自動導入を試みる。導入できなければ手順を示して停止する。
ensure_gh() {
  if command -v gh >/dev/null 2>&1; then
    return
  fi
  log "gh (GitHub CLI) が見つかりません。自動導入を試みます…"
  case "$(uname -s)" in
    Darwin)
      command -v brew >/dev/null 2>&1 || die "Homebrew が無いため gh を自動導入できません。https://cli.github.com/ から gh を導入してください"
      brew install gh || die "brew install gh に失敗しました"
      ;;
    Linux)
      if command -v apt-get >/dev/null 2>&1; then
        { sudo apt-get update && sudo apt-get install -y gh; } || die "apt-get での gh 導入に失敗しました"
      elif command -v dnf >/dev/null 2>&1; then
        sudo dnf install -y gh || die "dnf での gh 導入に失敗しました"
      elif command -v brew >/dev/null 2>&1; then
        brew install gh || die "brew install gh に失敗しました"
      else
        die "対応パッケージマネージャが見つかりません。https://cli.github.com/ から gh を導入してください"
      fi
      ;;
    MINGW* | MSYS* | CYGWIN*)
      command -v winget >/dev/null 2>&1 || die "winget が無いため gh を自動導入できません。'winget install --id GitHub.cli' などで gh を導入してください"
      winget install --id GitHub.cli -e --source winget || die "winget での gh 導入に失敗しました"
      ;;
    *)
      die "この OS では gh を自動導入できません。https://cli.github.com/ から gh を導入してください"
      ;;
  esac
  command -v gh >/dev/null 2>&1 || die "gh の導入に失敗しました"
}

# gh が認証済み（private repo にアクセス可能）であることを確認する。
ensure_gh_auth() {
  gh auth status >/dev/null 2>&1 || die "gh が未認証です。'gh auth login' で ${REPO%%/*} にアクセスできる状態にしてから再実行してください"
}

detect_os() {
  case "$(uname -s)" in
    Darwin) echo darwin ;;
    Linux) echo linux ;;
    MINGW* | MSYS* | CYGWIN*) echo windows ;;
    *) die "未知の OS: $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    arm64 | aarch64) echo arm64 ;;
    x86_64 | amd64) echo amd64 ;;
    *) die "未知の arch: $(uname -m)" ;;
  esac
}

# 取得する Release タグを決める。TTX_VERSION 指定があればそれ、無ければ最新 Release。
# prerelease:auto 運用のため releases/latest（prerelease を除外）は使わず最新1件を取る。
# 未公開 draft を誤って拾わないよう --exclude-drafts で除外する（prerelease は残す）。
resolve_version() {
  if [ -n "${TTX_VERSION:-}" ]; then
    printf '%s' "$TTX_VERSION"
    return
  fi
  local tag
  tag="$(gh release list -L 1 --exclude-drafts --repo "$REPO" --json tagName -q '.[0].tagName' 2>/dev/null || true)"
  [ -n "$tag" ] || die "公開済み Release が見つかりません（${REPO}）。先に Release を作成してください（docs/HOW_TO_RELEASE.md）"
  printf '%s' "$tag"
}

# 配置先を決める。既に PATH 上にある HOME 配下の書込可ディレクトリを優先し、
# ~/.local/bin が PATH 上にあれば最優先。どれも無ければ ~/.local/bin（作成して警告）。
choose_install_dir() {
  if [ -n "${TTX_INSTALL_DIR:-}" ]; then
    printf '%s' "$TTX_INSTALL_DIR"
    return
  fi
  local localbin="$HOME/.local/bin"
  case ":$PATH:" in
    *":$localbin:"*)
      printf '%s' "$localbin"
      return
      ;;
  esac
  local IFS=: d
  for d in $PATH; do
    case "$d" in
      "$HOME"/*)
        if [ -d "$d" ] && [ -w "$d" ]; then
          printf '%s' "$d"
          return
        fi
        ;;
    esac
  done
  printf '%s' "$localbin"
}

# tag の先頭 'v' と空白を除いた版番号。version.Version との比較に使う。
normalize_version() { printf '%s' "$1" | tr -d 'v[:space:]'; }

# 配置先 $dir に 3 ツールが揃い、--version が tag と一致するか（冪等判定）。
# PATH 全体ではなく $dir を見るのは、$dir が PATH 外でも再実行で再取得しないため。
already_installed() {
  local dir="$1" tag="$2" b got
  for b in "${BINARIES[@]}"; do
    [ -x "$dir/$b" ] || return 1
    got="$("$dir/$b" --version 2>/dev/null | head -n1 || true)"
    [ "$(normalize_version "$got")" = "$(normalize_version "$tag")" ] || return 1
  done
  return 0
}

# OS/arch に一致するアーカイブアセットを Release から取得し、tmp へ展開する。
# goreleaser の name_template にハードコード依存せず、アセット名のトークンでマッチする。
download_and_extract() {
  local tag="$1" os="$2" arch="$3" tmp="$4"
  local assets name lname matched=()
  assets="$(gh release view "$tag" --repo "$REPO" --json assets -q '.assets[].name' 2>/dev/null || true)"
  [ -n "$assets" ] || die "Release ${tag} が見つからないか、アセットがありません（${REPO}）"

  while IFS= read -r name; do
    [ -n "$name" ] || continue
    lname="$(printf '%s' "$name" | tr '[:upper:]' '[:lower:]')"
    # OS トークン（darwin の別名 macos/apple/osx も許容）
    case "$lname" in
      *"$os"*) ;;
      *)
        if [ "$os" = darwin ]; then
          case "$lname" in *macos* | *apple* | *osx*) ;; *) continue ;; esac
        else
          continue
        fi
        ;;
    esac
    # arch トークン（別名 aarch64 / x86_64 / x64 も許容）
    case "$lname" in
      *"$arch"*) ;;
      *)
        if [ "$arch" = amd64 ]; then
          case "$lname" in *x86_64* | *x64*) ;; *) continue ;; esac
        elif [ "$arch" = arm64 ]; then
          case "$lname" in *aarch64*) ;; *) continue ;; esac
        else
          continue
        fi
        ;;
    esac
    # アーカイブ拡張子
    case "$lname" in
      *.tar.gz | *.tgz | *.zip) matched+=("$name") ;;
    esac
  done <<EOF
$assets
EOF

  [ "${#matched[@]}" -gt 0 ] || die "OS=${os} arch=${arch} に一致するアセットが Release ${tag} にありません（候補: $(printf '%s' "$assets" | tr '\n' ' ')）"

  for name in "${matched[@]}"; do
    gh release download "$tag" --repo "$REPO" --pattern "$name" --dir "$tmp" --clobber
    case "$name" in
      *.zip)
        command -v unzip >/dev/null 2>&1 || die "zip アーカイブの展開に unzip が必要です。unzip を導入してから再実行してください"
        (cd "$tmp" && unzip -oq "$name")
        ;;
      *.tar.gz | *.tgz) tar -xzf "$tmp/$name" -C "$tmp" ;;
    esac
  done
}

main() {
  ensure_gh
  ensure_gh_auth

  local os arch tag dir
  os="$(detect_os)"
  arch="$(detect_arch)"
  tag="$(resolve_version)"
  dir="$(choose_install_dir)"

  if already_installed "$dir" "$tag"; then
    log "ttx-* は既に最新 ($tag) です。再取得しません。"
    print_summary "$dir"
    return 0
  fi

  tmp="$(mktemp -d)"

  log "ttx-* $tag を $os/$arch 向けに取得し $dir へ配置します…"
  download_and_extract "$tag" "$os" "$arch" "$tmp"

  mkdir -p "$dir"
  local b found
  for b in "${BINARIES[@]}"; do
    found="$(find "$tmp" -type f \( -name "$b" -o -name "$b.exe" \) 2>/dev/null | head -n1)"
    [ -n "$found" ] || die "アーカイブ内に $b が見つかりません"
    install -m 0755 "$found" "$dir/$b" 2>/dev/null || {
      cp "$found" "$dir/$b" && chmod 0755 "$dir/$b"
    }
  done

  # 検証: 配置した各バイナリの --version が tag と一致するか。
  for b in "${BINARIES[@]}"; do
    local got
    got="$("$dir/$b" --version 2>/dev/null | head -n1 || true)"
    [ "$(normalize_version "$got")" = "$(normalize_version "$tag")" ] ||
      die "$b の検証に失敗（期待 $(normalize_version "$tag") / 実際 ${got:-無し}）"
  done

  log "完了: $dir に ttx-human / ttx-ai-send / ttx-ai-recv ($tag) を配置しました。"
  print_summary "$dir"
}

# AI が起動コマンドの提示に使えるよう、最終行に機械可読な要約を出す。
# 配置先が PATH 上なら素のコマンド、PATH 外なら絶対パスを起動コマンドとして示す。
print_summary() {
  local dir="$1"
  printf 'TTX_INSTALL_DIR=%s\n' "$dir"
  case ":$PATH:" in
    *":$dir:"*) printf 'TTX_LAUNCH_CMD=%s\n' "ttx-human" ;;
    *)
      log "警告: $dir は PATH 上にありません。別窓で素の 'ttx-human' を使うには、新しいシェルで PATH へ $dir を追加してください（例: echo 'export PATH=\"$dir:\$PATH\"' >> ~/.zshrc）。"
      printf 'TTX_LAUNCH_CMD=%s\n' "$dir/ttx-human"
      ;;
  esac
}

main "$@"
