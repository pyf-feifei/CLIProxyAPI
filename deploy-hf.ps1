# Hugging Face Spaces 部署脚本
# 使用前请先登录: huggingface-cli login 或 hf auth login
# 或设置环境变量: $env:HF_TOKEN = "your_token"

param(
    [Parameter(Mandatory=$true)]
    [string]$HfUsername,
    
    [string]$SpaceName = "CLIProxyAPI"
)

$ErrorActionPreference = "Stop"
$repoUrl = "https://huggingface.co/spaces/$HfUsername/$SpaceName"

Write-Host "=== CLIProxyAPI Hugging Face Spaces 部署 ===" -ForegroundColor Cyan
Write-Host "目标: $repoUrl" -ForegroundColor Gray

# 检查是否已登录
$whoami = hf auth whoami 2>$null
if (-not $whoami) {
    Write-Host "`n错误: 未登录 Hugging Face" -ForegroundColor Red
    Write-Host "请先运行: huggingface-cli login" -ForegroundColor Yellow
    Write-Host "或设置: `$env:HF_TOKEN = 'your_read_token'" -ForegroundColor Yellow
    exit 1
}

Write-Host "已登录: $whoami" -ForegroundColor Green

# 添加或更新 HF 远程
$existing = git remote get-url hf 2>$null
if ($existing) {
    git remote set-url hf $repoUrl
    Write-Host "已更新 HF 远程: $repoUrl" -ForegroundColor Gray
} else {
    git remote add hf $repoUrl
    Write-Host "已添加 HF 远程: $repoUrl" -ForegroundColor Gray
}

# 推送
Write-Host "`n正在推送到 Hugging Face Spaces..." -ForegroundColor Cyan
git push hf main

Write-Host "`n部署完成!" -ForegroundColor Green
Write-Host "Space 地址: https://huggingface.co/spaces/$HfUsername/$SpaceName" -ForegroundColor Cyan
Write-Host "构建将自动开始，请稍候在 Space 页面查看状态。" -ForegroundColor Gray
