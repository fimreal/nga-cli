#!/usr/bin/env bash
# install-skills.sh - 一键安装 NGA CLI Agent Skills 到当前工作区
#
# 使用方式:
#   1. 安装全部 Skill:
#      curl -sSL https://raw.githubusercontent.com/fimreal/nga-cli/main/scripts/install-skills.sh | bash
#   2. 安装指定 Skill (例如 nga-cli 或 nga-thread-monitor):
#      curl -sSL https://raw.githubusercontent.com/fimreal/nga-cli/main/scripts/install-skills.sh | bash -s -- nga-cli
#   3. 指定安装目录:
#      TARGET_DIR=/path/to/.agents/skills bash install-skills.sh

set -euo pipefail

SKILL_SELECTION="${1:-all}"
TARGET_BASE="${TARGET_DIR:-.agents/skills}"
RAW_BASE_URL="${REPO_RAW_URL:-https://raw.githubusercontent.com/fimreal/nga-cli/main}"

ALL_SKILLS=("nga-cli" "nga-thread-monitor")

echo "================================================================="
echo "  🚀 NGA CLI Agent Skill 安装程序"
echo "================================================================="

# 确定需要安装的 Skill 列表
SKILLS_TO_INSTALL=()
if [ "$SKILL_SELECTION" = "all" ] || [ -z "$SKILL_SELECTION" ]; then
    SKILLS_TO_INSTALL=("${ALL_SKILLS[@]}")
else
    # 检查指定的 Skill 是否合法
    MATCHED=false
    for s in "${ALL_SKILLS[@]}"; do
        if [ "$s" = "$SKILL_SELECTION" ]; then
            MATCHED=true
            SKILLS_TO_INSTALL+=("$s")
            break
        fi
    done
    if [ "$MATCHED" = false ]; then
        echo "❌ 错误: 未知的 Skill 名称 '$SKILL_SELECTION'"
        echo "   可用 Skills: ${ALL_SKILLS[*]}"
        exit 1
    fi
fi

# 检查下载工具
DOWNLOAD_TOOL=""
if command -v curl >/dev/null 2>&1; then
    DOWNLOAD_TOOL="curl"
elif command -v wget >/dev/null 2>&1; then
    DOWNLOAD_TOOL="wget"
fi

download_file() {
    local url="$1"
    local dest="$2"

    if [ "$DOWNLOAD_TOOL" = "curl" ]; then
        curl -fsSL "$url" -o "$dest"
    elif [ "$DOWNLOAD_TOOL" = "wget" ]; then
        wget -q "$url" -O "$dest"
    else
        echo "❌ 错误: 系统中未找到 curl 或 wget，无法完成远程下载"
        return 1
    fi
}

echo "📂 安装目标目录: ${TARGET_BASE}"
echo "📦 待安装 Skill 列表: ${SKILLS_TO_INSTALL[*]}"
echo ""

SUCCESS_COUNT=0

for skill in "${SKILLS_TO_INSTALL[@]}"; do
    DEST_DIR="${TARGET_BASE}/${skill}"
    DEST_FILE="${DEST_DIR}/SKILL.md"
    mkdir -p "$DEST_DIR"

    echo -n "  ⏳ 正在安装 [${skill}] ... "

    # 优先检测当前目录是否存在本地版本 (本地克隆仓库场景)
    LOCAL_SOURCE="./docs/skills/${skill}/SKILL.md"
    if [ -f "$LOCAL_SOURCE" ]; then
        cp "$LOCAL_SOURCE" "$DEST_FILE"
        echo "✅ 完成 (本地同步: ${DEST_FILE})"
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
        continue
    fi

    # 远程下载
    REMOTE_URL="${RAW_BASE_URL}/docs/skills/${skill}/SKILL.md"
    if download_file "$REMOTE_URL" "$DEST_FILE"; then
        echo "✅ 完成 (远程拉取: ${DEST_FILE})"
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    else
        echo "❌ 失败 (拉取 $REMOTE_URL 异常)"
    fi
done

echo ""
echo "================================================================="
if [ "$SUCCESS_COUNT" -gt 0 ]; then
    echo "🎉 安装成功！共装载 $SUCCESS_COUNT 个 Skill 至: ${TARGET_BASE}"
    echo "💡 AI Agent (如 Multica / Codex) 将自动扫描装载此目录下已配置的 Skill。"
else
    echo "⚠️ 未能成功安装任何 Skill，请检查网络连接或权限。"
    exit 1
fi
echo "================================================================="
