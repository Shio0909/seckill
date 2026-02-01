#!/bin/bash
# 推荐服务启动脚本

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}   推荐系统微服务启动脚本${NC}"
echo -e "${GREEN}========================================${NC}"

# 检查Python环境
check_python() {
    if ! command -v python3 &> /dev/null; then
        echo -e "${RED}错误: 未找到 Python3${NC}"
        exit 1
    fi
    
    PYTHON_VERSION=$(python3 -c 'import sys; print(f"{sys.version_info.major}.{sys.version_info.minor}")')
    echo -e "${GREEN}Python版本: ${PYTHON_VERSION}${NC}"
}

# 检查虚拟环境
check_venv() {
    if [ ! -d ".venv" ]; then
        echo -e "${YELLOW}创建虚拟环境...${NC}"
        python3 -m venv .venv
    fi
    
    source .venv/bin/activate
    echo -e "${GREEN}虚拟环境已激活${NC}"
}

# 安装依赖
install_deps() {
    echo -e "${YELLOW}安装Python依赖...${NC}"
    pip install -r requirements.txt -q
    echo -e "${GREEN}依赖安装完成${NC}"
}

# 生成gRPC代码
generate_grpc() {
    echo -e "${YELLOW}生成gRPC代码...${NC}"
    python -m grpc_tools.protoc \
        -I./proto \
        --python_out=./proto \
        --grpc_python_out=./proto \
        ./proto/recommendation.proto
    echo -e "${GREEN}gRPC代码生成完成${NC}"
}

# 检查配置
check_config() {
    if [ ! -f "config/config.yaml" ]; then
        echo -e "${YELLOW}复制配置模板...${NC}"
        cp config/config.example.yaml config/config.yaml
    fi
}

# 检查数据
check_data() {
    if [ ! -f "data/processed/events.csv" ]; then
        echo -e "${YELLOW}警告: 未找到处理后的数据${NC}"
        echo -e "${YELLOW}请先运行: python scripts/convert_movielens.py${NC}"
    fi
}

# 检查模型
check_model() {
    if [ ! -f "src/model/models/lgb_ctr_v1.pkl" ]; then
        echo -e "${YELLOW}警告: 未找到训练好的模型${NC}"
        echo -e "${YELLOW}请先运行: python scripts/train_lgb.py${NC}"
    fi
}

# 启动服务
start_service() {
    echo -e "${GREEN}启动推荐服务...${NC}"
    python -m src.main
}

# 主函数
main() {
    cd "$(dirname "$0")"
    
    check_python
    check_venv
    install_deps
    generate_grpc
    check_config
    check_data
    check_model
    
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}   准备工作完成，启动服务${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    
    start_service
}

main "$@"
