# dev_start.ps1
Write-Host "正在启动 K8s 环境端口转发..." -ForegroundColor Green

# 启动 MySQL 转发 (转发到本地 3307)
Start-Process powershell -ArgumentList "kubectl port-forward svc/mysql-service 3307:3306" -WindowStyle Minimized

# 启动 Redis 转发 (转发到本地 6379)
Start-Process powershell -ArgumentList "kubectl port-forward svc/redis-service 6379:6379" -WindowStyle Minimized

# 启动 RabbitMQ 转发 (转发到本地 6672 和 15672)
Start-Process powershell -ArgumentList "kubectl port-forward svc/rabbitmq-service 6672:5672 15672:15672" -WindowStyle Minimized

Write-Host "✅ 所有服务已在后台启动！"
Write-Host "MySQL: 3307"
Write-Host "Redis: 6379"
Write-Host "RabbitMQ: 6672 / 15672"