# 排序服务 Dockerfile (精简版)
FROM python:3.11-slim

WORKDIR /app

# 安装依赖
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# 复制代码
COPY *.py ./
COPY models/ ./models/

# 生成gRPC代码
COPY ranking.proto .
RUN python -m grpc_tools.protoc -I. --python_out=. --grpc_python_out=. ranking.proto

ENV PYTHONUNBUFFERED=1
ENV MODEL_PATH=/app/models/lgb_ctr_v1.pkl

EXPOSE 50052

CMD ["python", "server.py"]
