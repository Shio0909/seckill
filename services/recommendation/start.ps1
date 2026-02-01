# 推荐服务启动脚本 (Windows PowerShell)

$ErrorActionPreference = "Stop"

Write-Host "========================================" -ForegroundColor Green
Write-Host "   推荐系统微服务启动脚本" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green

# 切换到脚本目录
Set-Location $PSScriptRoot

# 检查Python环境
function Check-Python {
    try {
        $version = python --version
        Write-Host "Python版本: $version" -ForegroundColor Green
    } catch {
        Write-Host "错误: 未找到 Python" -ForegroundColor Red
        exit 1
    }
}

# 创建/激活虚拟环境
function Setup-Venv {
    if (-not (Test-Path ".venv")) {
        Write-Host "创建虚拟环境..." -ForegroundColor Yellow
        python -m venv .venv
    }
    
    # 激活虚拟环境
    & .\.venv\Scripts\Activate.ps1
    Write-Host "虚拟环境已激活" -ForegroundColor Green
}

# 安装依赖
function Install-Deps {
    Write-Host "安装Python依赖..." -ForegroundColor Yellow
    pip install -r requirements.txt -q
    Write-Host "依赖安装完成" -ForegroundColor Green
}

# 生成gRPC代码
function Generate-Grpc {
    Write-Host "生成gRPC代码..." -ForegroundColor Yellow
    
    # 确保proto目录存在
    if (-not (Test-Path "proto")) {
        New-Item -ItemType Directory -Path "proto" | Out-Null
    }
    
    python -m grpc_tools.protoc `
        -I./proto `
        --python_out=./proto `
        --grpc_python_out=./proto `
        ./proto/recommendation.proto
    
    Write-Host "gRPC代码生成完成" -ForegroundColor Green
}

# 检查配置
function Check-Config {
    if (-not (Test-Path "config/config.yaml")) {
        Write-Host "复制配置模板..." -ForegroundColor Yellow
        Copy-Item "config/config.example.yaml" "config/config.yaml"
    }
}

# 检查数据
function Check-Data {
    if (-not (Test-Path "data/processed/events.csv")) {
        Write-Host "警告: 未找到处理后的数据" -ForegroundColor Yellow
        Write-Host "请先运行: python scripts/convert_movielens.py" -ForegroundColor Yellow
    }
}

# 检查模型
function Check-Model {
    if (-not (Test-Path "src/model/models/lgb_ctr_v1.pkl")) {
        Write-Host "警告: 未找到训练好的模型" -ForegroundColor Yellow
        Write-Host "请先运行: python scripts/train_lgb.py" -ForegroundColor Yellow
    }
}

# 创建必要的目录
function Setup-Directories {
    $dirs = @(
        "data/raw",
        "data/processed", 
        "src/model/models",
        "logs"
    )
    
    foreach ($dir in $dirs) {
        if (-not (Test-Path $dir)) {
            New-Item -ItemType Directory -Path $dir -Force | Out-Null
        }
    }
}

# 启动服务
function Start-Service {
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Green
    Write-Host "   准备工作完成，启动服务" -ForegroundColor Green
    Write-Host "========================================" -ForegroundColor Green
    Write-Host ""
    
    # 设置 PYTHONPATH
    $env:PYTHONPATH = (Get-Location).Path
    
    python -m src.main
}

# 主函数
function Main {
    Check-Python
    Setup-Directories
    Setup-Venv
    Install-Deps
    Generate-Grpc
    Check-Config
    Check-Data
    Check-Model
    Start-Service
}

Main
