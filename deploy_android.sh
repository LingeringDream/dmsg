#!/bin/bash

set -e

# --- 配置区域 ---
BINARY_NAME="dmsg"
BUILD_DIR="./build"
OUTPUT_BINARY="${BUILD_DIR}/${BINARY_NAME}_android"
REMOTE_DIR="/data/local/tmp"
REMOTE_PATH="${REMOTE_DIR}/${BINARY_NAME}"

# --- 颜色输出 ---
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${YELLOW}🚀 开始 Android 部署流程...${NC}"

# 1. 编译 (交叉编译为 Android arm64 架构)
echo -e "1️⃣ 正在交叉编译 Android 版本 (GOOS=android GOARCH=arm64)..."
mkdir -p ${BUILD_DIR}
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -o ${OUTPUT_BINARY} ./cmd/dmsg/

echo -e "   ${GREEN}✅ 编译成功 -> ${OUTPUT_BINARY}${NC}"

# 2. 检查 ADB 设备
echo -e "2️⃣ 正在检查 ADB 连接..."
DEVICE_COUNT=$(adb devices | grep -c "device$")
if [ "$DEVICE_COUNT" -eq 0 ]; then
    echo -e "   ${RED}❌ 未检测到设备。请确保手机已连接并开启 USB 调试。${NC}"
    exit 1
fi
echo -e "   ${GREEN}✅ 设备已连接${NC}"

# 3. 推送文件
echo -e "3️⃣ 正在推送二进制文件到 /data/local/tmp/dmsg ..."
adb push ${OUTPUT_BINARY} ${REMOTE_PATH}

# 4. 赋予执行权限
echo -e "4️⃣ 设置执行权限 (chmod +x)..."
adb shell chmod 755 ${REMOTE_DIR}/${BINARY_NAME}

# 5. 启动服务
echo -e "5️⃣ 启动服务..."
# 停止旧进程 (如果正在运行)
adb shell "pkill -f dmsg > /dev/null 2>&1 || true"

# 后台静默运行并输出日志到手机本地文件
adb shell "nohup ${REMOTE_DIR}/${BINARY_NAME} > ${REMOTE_DIR}/dmsg.log 2>&1 &"

echo -e "${GREEN}🎉 部署完成！${NC}"
echo -e "   📱 程序已在后台运行。"
echo -e "   📝 查看日志: adb shell tail -f ${REMOTE_DIR}/dmsg.log"
echo -e "   🛑 停止服务: adb shell pkill -f dmsg"

