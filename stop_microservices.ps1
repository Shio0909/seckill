# stop_microservices.ps1
# 停止所有微服务

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  停止 DaMai-Go 微服务" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# 查找并停止 Go 进程
Write-Host "正在查找运行中的服务..." -ForegroundColor Yellow

$goProcesses = Get-Process | Where-Object { $_.ProcessName -like "*go*" -or $_.MainWindowTitle -like "*services*" }

if ($goProcesses.Count -eq 0) {
    Write-Host "  未找到运行中的服务" -ForegroundColor Gray
} else {
    Write-Host "  找到 $($goProcesses.Count) 个进程" -ForegroundColor Green
    
    foreach ($proc in $goProcesses) {
        try {
            Write-Host "  停止: $($proc.ProcessName) (PID: $($proc.Id))" -ForegroundColor Yellow
            Stop-Process -Id $proc.Id -Force
        } catch {
            Write-Host "  ⚠️  无法停止进程 $($proc.Id)" -ForegroundColor Red
        }
    }
}

Write-Host ""
Write-Host "✅ 所有服务已停止" -ForegroundColor Green
Write-Host ""
