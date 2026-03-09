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
$sourceRoot = (Resolve-Path (Join-Path $PSScriptRoot ".")).Path

function Invoke-Git {
    param(
        [Parameter(Mandatory=$true)]
        [string[]]$Arguments,

        [string]$WorkingDirectory = $sourceRoot
    )

    & git -C $WorkingDirectory @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "git command failed: git -C $WorkingDirectory $($Arguments -join ' ')"
    }
}

function Export-HfSnapshot {
    param(
        [Parameter(Mandatory=$true)]
        [string]$DestinationRoot
    )

    # Hugging Face Spaces rejects binary objects in regular git history.
    # Export a clean tracked-files snapshot and skip image assets that trigger pre-receive rejection.
    $trackedFiles = & git -C $sourceRoot ls-files
    if ($LASTEXITCODE -ne 0) {
        throw "failed to enumerate tracked files from $sourceRoot"
    }

    foreach ($relativePath in $trackedFiles) {
        if ($relativePath -match '\.(png|jpg|jpeg|gif|webp)$') {
            continue
        }

        $sourcePath = Join-Path $sourceRoot $relativePath
        if (-not (Test-Path $sourcePath -PathType Leaf)) {
            continue
        }

        $targetPath = Join-Path $DestinationRoot $relativePath
        $targetDir = Split-Path $targetPath -Parent
        if ($targetDir -and -not (Test-Path $targetDir)) {
            New-Item -ItemType Directory -Path $targetDir -Force | Out-Null
        }

        Copy-Item $sourcePath $targetPath -Force
    }
}

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

Write-Host "源目录: $sourceRoot" -ForegroundColor Gray

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("CLIProxyAPI-hf-" + [Guid]::NewGuid().ToString("N"))

try {
    New-Item -ItemType Directory -Path $tempRoot -Force | Out-Null
    Export-HfSnapshot -DestinationRoot $tempRoot

    Invoke-Git -WorkingDirectory $tempRoot -Arguments @("init", "-b", "main")

    $gitUserName = (& git -C $sourceRoot config user.name).Trim()
    $gitUserEmail = (& git -C $sourceRoot config user.email).Trim()
    if ($gitUserName) {
        Invoke-Git -WorkingDirectory $tempRoot -Arguments @("config", "user.name", $gitUserName)
    }
    if ($gitUserEmail) {
        Invoke-Git -WorkingDirectory $tempRoot -Arguments @("config", "user.email", $gitUserEmail)
    }

    Invoke-Git -WorkingDirectory $tempRoot -Arguments @("add", ".")
    Invoke-Git -WorkingDirectory $tempRoot -Arguments @("commit", "-m", "Deploy CLIProxyAPI to Hugging Face Spaces")
    Invoke-Git -WorkingDirectory $tempRoot -Arguments @("remote", "add", "hf", $repoUrl)

    Write-Host "`n正在推送 Hugging Face 快照..." -ForegroundColor Cyan
    Invoke-Git -WorkingDirectory $tempRoot -Arguments @("push", "hf", "main:main", "--force")
} finally {
    if (Test-Path $tempRoot) {
        Remove-Item $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Write-Host "`n部署完成!" -ForegroundColor Green
Write-Host "Space 地址: https://huggingface.co/spaces/$HfUsername/$SpaceName" -ForegroundColor Cyan
Write-Host "构建将自动开始，请稍候在 Space 页面查看状态。" -ForegroundColor Gray
