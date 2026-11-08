# install-skills.ps1 - 一键安装 NGA CLI Agent Skills 到当前工作区 (PowerShell)
#
# 使用方式:
#   1. 安装全部 Skill:
#      irm https://raw.githubusercontent.com/fimreal/nga-cli/main/scripts/install-skills.ps1 | iex
#   2. 安装指定 Skill:
#      & ([scriptblock]::Create((irm https://raw.githubusercontent.com/fimreal/nga-cli/main/scripts/install-skills.ps1))) -Skill nga-cli
#   3. 本地执行:
#      .\scripts\install-skills.ps1 -Skill all

[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [string]$Skill = "all",

    [Parameter(Position = 1)]
    [string]$TargetDir = ".agents/skills",

    [string]$RawBaseUrl = "https://raw.githubusercontent.com/fimreal/nga-cli/main"
)

$ErrorActionPreference = "Stop"

$allSkills = @("nga-cli", "nga-thread-monitor")

Write-Host "=================================================================" -ForegroundColor Cyan
Write-Host "  🚀 NGA CLI Agent Skill 安装程序 (PowerShell)" -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan

$skillsToInstall = @()
if ($Skill -eq "all" -or [string]::IsNullOrWhiteSpace($Skill)) {
    $skillsToInstall = $allSkills
} else {
    if ($allSkills -contains $Skill) {
        $skillsToInstall = @($Skill)
    } else {
        Write-Host "❌ 错误: 未知的 Skill 名称 '$Skill'" -ForegroundColor Red
        Write-Host "   可用 Skills: $($allSkills -join ', ')" -ForegroundColor Yellow
        exit 1
    }
}

Write-Host "📂 安装目标目录: $TargetDir"
Write-Host "📦 待安装 Skill 列表: $($skillsToInstall -join ', ')"
Write-Host ""

$successCount = 0

foreach ($s in $skillsToInstall) {
    $destDir = Join-Path $TargetDir $s
    $destFile = Join-Path $destDir "SKILL.md"

    if (-not (Test-Path $destDir)) {
        New-Item -ItemType Directory -Path $destDir -Force | Out-Null
    }

    Write-Host -NoNewline "  ⏳ 正在安装 [$s] ... "

    # 优先检测本地仓库是否存在对应文件
    $localSource = Join-Path "docs\skills" (Join-Path $s "SKILL.md")
    if (Test-Path $localSource) {
        Copy-Item -Path $localSource -Destination $destFile -Force
        Write-Host "✅ 完成 (本地同步: $destFile)" -ForegroundColor Green
        $successCount++
        continue
    }

    # 远程下载
    $remoteUrl = "$RawBaseUrl/docs/skills/$s/SKILL.md"
    try {
        Invoke-RestMethod -Uri $remoteUrl -OutFile $destFile
        Write-Host "✅ 完成 (远程拉取: $destFile)" -ForegroundColor Green
        $successCount++
    } catch {
        Write-Host "❌ 失败 (拉取 $remoteUrl 异常: $_)" -ForegroundColor Red
    }
}

Write-Host ""
Write-Host "=================================================================" -ForegroundColor Cyan
if ($successCount -gt 0) {
    Write-Host "🎉 安装成功！共装载 $successCount 个 Skill 至: $TargetDir" -ForegroundColor Green
    Write-Host "💡 AI Agent (如 Multica / Codex) 将自动扫描装载此目录下已配置的 Skill。" -ForegroundColor Gray
} else {
    Write-Host "⚠️ 未能成功安装任何 Skill，请检查网络连接。" -ForegroundColor Red
    exit 1
}
Write-Host "=================================================================" -ForegroundColor Cyan
