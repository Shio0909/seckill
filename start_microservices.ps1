# start_microservices.ps1
# 微服务架构启动脚本

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  DaMai-Go 微服务架构启动脚本" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# 检查 Consul 是否运行
Write-Host "[1/6] 检查 Consul 服务..." -ForegroundColor Yellow
$consulRunning = $false
try {
    $response = Invoke-WebRequest -Uri "http://127.0.0.1:8500/v1/status/leader" -TimeoutSec 2 -ErrorAction SilentlyContinue
    if ($response.StatusCode -eq 200) {
        $consulRunning = $true
        Write-Host "  ✅ Consul 已运行" -ForegroundColor Green
    }
} catch {
    Write-Host "  ❌ Consul 未运行，请先启动 Consul" -ForegroundColor Red
    Write-Host "     启动命令: consul agent -dev" -ForegroundColor Gray
    Write-Host ""
    $startConsul = Read-Host "是否现在启动 Consul? (y/n)"
    if ($startConsul -eq "y") {
        Start-Process powershell -ArgumentList "consul agent -dev" -WindowStyle Normal
        Write-Host "  等待 Consul 启动..." -ForegroundColor Yellow
        Start-Sleep -Seconds 5
    } else {
        Write-Host "  跳过 Consul 启动，服务注册将失败" -ForegroundColor Yellow
    }
}

# 检查基础设施服务
Write-Host ""
Write-Host "[2/6] 检查基础设施服务 (MySQL/Redis/RabbitMQ)..." -ForegroundColor Yellow

$infraReady = $true

# 检查 MySQL
try {
    $null = Test-NetConnection -ComputerName localhost -Port 3307 -InformationLevel Quiet -WarningAction SilentlyContinue
    Write-Host "  ✅ MySQL (3307)" -ForegroundColor Green
} catch {
    Write-Host "  ❌ MySQL 未就绪" -ForegroundColor Red
    $infraReady = $false
}

# 检查 Redis
try {
    $null = Test-NetConnection -ComputerName localhost -Port 6379 -InformationLevel Quiet -WarningAction SilentlyContinue
    Write-Host "  ✅ Redis (6379)" -ForegroundColor Green
} catch {
    Write-Host "  ❌ Redis 未就绪" -ForegroundColor Red
    $infraReady = $false
}

# 检查 RabbitMQ
try {
    $null = Test-NetConnection -ComputerName localhost -Port 6672 -InformationLevel Quiet -WarningAction SilentlyContinue
    Write-Host "  ✅ RabbitMQ (6672)" -ForegroundColor Green
} catch {
    Write-Host "  ❌ RabbitMQ 未就绪" -ForegroundColor Red
    $infraReady = $false
}

if (-not $infraReady) {
    Write-Host ""
    Write-Host "  提示: 运行 .\dev_start.ps1 启动基础设施服务" -ForegroundColor Yellow
    $continue = Read-Host "是否继续启动微服务? (y/n)"
    if ($continue -ne "y") {
        exit
    }
}

# 启动微服务
Write-Host ""
Write-Host "[3/6] 启动微服务..." -ForegroundColor Yellow

# User Service (端口 50051)
Write-Host "  启动 User Service (gRPC:50051)..." -ForegroundColor Cyan
Start-Process powershell -ArgumentList "cd '$PSScriptRoot'; go run services/user/main.go" -WindowStyle Normal

Start-Sleep -Seconds 2

# Product Service (端口 50052)
Write-Host "  启动 Product Service (gRPC:50052)..." -ForegroundColor Cyan
Start-Process powershell -ArgumentList "cd '$PSScriptRoot'; go run services/product/main.go" -WindowStyle Normal

Start-Sleep -Seconds 2

# Order Service (端口 50053)
Write-Host "  启动 Order Service (gRPC:50053)..." -ForegroundColor Cyan
Start-Process powershell -ArgumentList "cd '$PSScriptRoot'; go run services/order/main.go" -WindowStyle Normal

Start-Sleep -Seconds 2

# Seckill Service (端口 50054)
Write-Host "  启动 Seckill Service (gRPC:50054)..." -ForegroundColor Cyan
Start-Process powershell -ArgumentList "cd '$PSScriptRoot'; go run services/seckill/main.go" -WindowStyle Normal

Start-Sleep -Seconds 2

# API Gateway (端口 8080)
Write-Host "  启动 API Gateway (HTTP:8080)..." -ForegroundColor Cyan
Start-Process powershell -ArgumentList "cd '$PSScriptRoot'; go run services/gateway/main.go" -WindowStyle Normal

Write-Host ""
Write-Host "[4/6] 等待服务启动..." -ForegroundColor Yellow
Start-Sleep -Seconds 5

# 检查服务健康状态
Write-Host ""
Write-Host "[5/6] 检查服务健康状态..." -ForegroundColor Yellow

$services = @(
    @{Name="User Service"; Port=50051},
    @{Name="Product Service"; Port=50052},
    @{Name="Order Service"; Port=50053},
    @{Name="Seckill Service"; Port=50054},
    @{Name="API Gateway"; Port=8080}
)

foreach ($svc in $services) {
    try {
        $connection = Test-NetConnection -ComputerName localhost -Port $svc.Port -InformationLevel Quiet -WarningAction SilentlyContinue
        if ($connection) {
            Write-Host "  ✅ $($svc.Name) ($($svc.Port))" -ForegroundColor Green
        } else {
            Write-Host "  ⚠️  $($svc.Name) ($($svc.Port)) - 端口未监听" -ForegroundColor Yellow
        }
    } catch {
        Write-Host "  ❌ $($svc.Name) ($($svc.Port)) - 检查失败" -ForegroundColor Red
    }
}

# 显示访问信息
Write-Host ""
Write-Host "[6/6] 启动完成！" -ForegroundColor Green
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  服务访问信息" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "📡 微服务 (gRPC):" -ForegroundColor White
Write-Host "  • User Service:    localhost:50051" -ForegroundColor Gray
Write-Host "  • Product Service: localhost:50052" -ForegroundColor Gray
Write-Host "  • Order Service:   localhost:50053" -ForegroundColor Gray
Write-Host "  • Seckill Service: localhost:50054" -ForegroundColor Gray
Write-Host ""
Write-Host "🌐 API Gateway (HTTP):" -ForegroundColor White
Write-Host "  • Gateway:         http://localhost:8080" -ForegroundColor Gray
Write-Host "  • Swagger 文档:    http://localhost:8080/swagger/index.html" -ForegroundColor Gray
Write-Host ""
Write-Host "🔧 服务治理:" -ForegroundColor White
Write-Host "  • Consul UI:       http://localhost:8500" -ForegroundColor Gray
Write-Host ""
Write-Host "💾 基础设施:" -ForegroundColor White
Write-Host "  • MySQL:           localhost:3307" -ForegroundColor Gray
Write-Host "  • Redis:           localhost:6379" -ForegroundColor Gray
Write-Host "  • RabbitMQ:        localhost:6672 (AMQP)" -ForegroundColor Gray
Write-Host "  • RabbitMQ UI:     http://localhost:15672 (guest/guest)" -ForegroundColor Gray
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "💡 提示:" -ForegroundColor Yellow
Write-Host "  • 所有服务已在独立窗口中启动" -ForegroundColor Gray
Write-Host "  • 关闭窗口即可停止对应服务" -ForegroundColor Gray
Write-Host "  • 查看 Consul UI 可监控服务注册状态" -ForegroundColor Gray
Write-Host ""
