# 推荐系统微服务 Dockerfile
# Python 3.11

FROM python:3.11-slim as builder

WORKDIR /app

# 安装构建依赖
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# 复制依赖文件
COPY requirements.txt .

# 安装Python依赖
RUN pip install --no-cache-dir --user -r requirements.txt

# 生产镜像
FROM python:3.11-slim

WORKDIR /app

# 复制Python包
COPY --from=builder /root/.local /root/.local

# 确保pip安装的包在PATH中
ENV PATH=/root/.local/bin:$PATH

# 创建数据目录
RUN mkdir -p /app/data/raw /app/data/processed /app/src/model/models /app/logs

# 复制应用代码
COPY proto/ /app/proto/
COPY src/ /app/src/
COPY scripts/ /app/scripts/
COPY config/ /app/config/

# 生成gRPC代码
RUN python -m grpc_tools.protoc \
    -I./proto \
    --python_out=./proto \
    --grpc_python_out=./proto \
    ./proto/recommendation.proto

# 环境变量
ENV PYTHONPATH=/app
ENV PYTHONUNBUFFERED=1
ENV LOG_LEVEL=INFO
ENV GRPC_PORT=50052

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD python -c "import grpc; channel = grpc.insecure_channel('localhost:50052'); grpc.channel_ready_future(channel).result(timeout=5)" || exit 1

# 暴露端口
EXPOSE 50052

# 启动命令
CMD ["python", "-m", "src.main"]
