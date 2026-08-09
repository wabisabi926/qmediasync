#!/bin/bash

# QMediaSync 发布脚本 — 创建并推送 Git Tag
# 完整构建流程由 GitHub Actions (.github/workflows/release.yml) 自动完成：
#   Windows/Linux 双平台二进制 → 飞牛 FPK → Docker 镜像 → GitHub Release（含 .changes/ 更新日志）

set -e

# 切换到项目根目录
cd "$(dirname "$0")/.."
echo "工作目录：$(pwd)"

# 解析参数
VERSION=""
while getopts "v:h" opt; do
    case $opt in
        v) VERSION="$OPTARG" ;;
        h) echo "用法: $0 -v vX.X.X"; exit 0 ;;
        *) echo "用法: $0 -v vX.X.X"; exit 1 ;;
    esac
done

# 校验版本号格式
if [ -z "$VERSION" ]; then
    echo "错误: 请指定版本号，如: $0 -v v0.16.3"
    exit 1
fi
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "错误: 版本号格式应为 vX.X.X（如 v0.16.3）"
    exit 1
fi

# 检查更新日志是否存在
NOTES=".changes/${VERSION}.md"
if [ ! -f "$NOTES" ]; then
    echo "警告: 未找到 $NOTES"
    echo "      GitHub Release 将只有默认标题，没有中文更新日志"
    read -p "继续推送 tag？(y/N) " confirm
    if [ "$confirm" != "y" ] && [ "$confirm" != "Y" ]; then
        echo "已取消"
        exit 0
    fi
else
    echo "✓ 更新日志: $NOTES"
fi

# 检查 tag 是否已存在
if git rev-parse "$VERSION" >/dev/null 2>&1; then
    echo "错误: tag $VERSION 已存在"
    exit 1
fi

# 推送 main 分支最新代码
echo "拉取并推送 main 分支最新代码..."
git pull origin main
git push origin main

# 创建并推送 tag
echo "创建 tag $VERSION ..."
git tag "$VERSION" -m "Release $VERSION"
git push origin "$VERSION"

echo ""
echo "========================================"
echo "✓ Tag $VERSION 已推送到 GitHub"
echo "  GitHub Actions 开始构建，包括："
echo "    - Windows/Linux 双平台二进制"
echo "    - 飞牛 FPK (amd64 + arm64)"
echo "    - Docker 多架构镜像"
echo "    - GitHub Release（含中文更新日志）"
echo "========================================"
echo "查看进度: https://github.com/wabisabi926/qmediasync/actions"
