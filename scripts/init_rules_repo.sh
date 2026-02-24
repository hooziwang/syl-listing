#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
用法：
  scripts/init_rules_repo.sh <repo_name> [target_dir] [--create-remote public|private]

示例：
  scripts/init_rules_repo.sh syl-listing-rules
  scripts/init_rules_repo.sh syl-listing-rules ~/work/syl-listing-rules --create-remote public
EOF
}

if [[ "${1:-}" == "" ]]; then
  usage
  exit 1
fi

REPO_NAME="$1"
TARGET_DIR="${2:-$PWD/$REPO_NAME}"
CREATE_REMOTE="false"
VISIBILITY="public"

if [[ "${3:-}" == "--create-remote" ]]; then
  CREATE_REMOTE="true"
  VISIBILITY="${4:-public}"
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
TEMPLATE_DIR="$ROOT_DIR/templates/rules-repo"

if [[ ! -d "$TEMPLATE_DIR" ]]; then
  echo "模板目录不存在：$TEMPLATE_DIR" >&2
  exit 1
fi

if [[ -e "$TARGET_DIR" ]]; then
  echo "目标目录已存在：$TARGET_DIR" >&2
  exit 1
fi

mkdir -p "$TARGET_DIR"
cp -R "$TEMPLATE_DIR"/. "$TARGET_DIR"/

if [[ ! -d "$TARGET_DIR/.git" ]]; then
  git -C "$TARGET_DIR" init >/dev/null
fi

sed -i.bak "s/__RULES_REPO_NAME__/$REPO_NAME/g" "$TARGET_DIR/README.md"
rm -f "$TARGET_DIR/README.md.bak"

git -C "$TARGET_DIR" add .
git -C "$TARGET_DIR" commit -m "chore(init): 初始化 rules 仓库模板" >/dev/null || true

echo "已初始化规则仓库模板：$TARGET_DIR"

if [[ "$CREATE_REMOTE" == "true" ]]; then
  if ! command -v gh >/dev/null 2>&1; then
    echo "未找到 gh 命令，跳过远程创建。" >&2
    exit 0
  fi
  if ! gh auth status >/dev/null 2>&1; then
    echo "gh 未登录，跳过远程创建。" >&2
    exit 0
  fi
  gh repo create "$REPO_NAME" "--$VISIBILITY" --source "$TARGET_DIR" --remote origin --push
  echo "已创建并推送远程仓库：$REPO_NAME"
fi

