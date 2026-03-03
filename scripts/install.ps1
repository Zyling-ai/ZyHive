# ZyHive (引巢) — Windows 一键安装脚本 (PowerShell)
# ─────────────────────────────────────────────────────────────────────────
# 通用安装命令（自动识别平台）：
#   Windows:      irm https://install.zyling.ai/install | iex
#   Linux/macOS:  curl -sSL https://install.zyling.ai/install | bash
#
# 直接指定 PS1：
#   irm https://install.zyling.ai/zyhive.ps1 | iex
#
# 需要 PowerShell 5.1+ 或 PowerShell 7+
# 安装系统服务需要管理员权限（脚本会自动提权）
# ─────────────────────────────────────────────────────────────────────────

param(
    [string]$Port   = "8080",
    [string]$Domain = "",
    [switch]$NoService,      # 不安装 Windows 服务（仅复制二进制）
    [switch]$Uninstall       # 卸载
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ── 颜色输出 ────────────────────────────────────────────────────────────────
function Write-Info    { param($Msg) Write-Host "  ℹ  $Msg" -ForegroundColor Cyan }
function Write-Ok      { param($Msg) Write-Host "  ✅ $Msg" -ForegroundColor Green }
function Write-Warn    { param($Msg) Write-Host "  ⚠  $Msg" -ForegroundColor Yellow }
function Write-Err     { param($Msg) Write-Host "  ❌ $Msg" -ForegroundColor Red; exit 1 }

# ── 自动提权（非管理员时重新以管理员身份运行）─────────────────────────────
$IsAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $IsAdmin) {
    Write-Host ""
    Write-Host "  ZyHive 安装需要管理员权限，正在请求提权..." -ForegroundColor Yellow
    # 管道执行（irm|iex）时 MyCommand 是 ScriptBlock，没有 Path 属性
    # 严格模式下直接访问会抛出 PropertyNotFoundException，用 try/catch 规避
    $ScriptPath = try { $MyInvocation.MyCommand.Path } catch { $null }
    if ($ScriptPath) {
        # 本地脚本文件 → 直接提权重启
        $ArgList = "-NoProfile -ExecutionPolicy Bypass -File `"$ScriptPath`" -Port $Port"
        if ($Domain)     { $ArgList += " -Domain $Domain" }
        if ($NoService)  { $ArgList += " -NoService" }
        Start-Process powershell -Verb RunAs -ArgumentList $ArgList -Wait
        exit
    } else {
        # 管道运行（irm | iex）→ 下载到临时文件再提权重启
        $TmpScript = Join-Path $env:TEMP "zyhive-install.ps1"
        $InstallBase = "https://install.zyling.ai"
        try {
            Invoke-WebRequest -Uri "$InstallBase/zyhive.ps1" -OutFile $TmpScript -UseBasicParsing
        } catch {
            Invoke-WebRequest -Uri "https://raw.githubusercontent.com/Zyling-ai/zyhive/main/scripts/install.ps1" -OutFile $TmpScript -UseBasicParsing
        }
        $ArgList = "-NoProfile -ExecutionPolicy Bypass -File `"$TmpScript`" -Port $Port"
        if ($Domain)     { $ArgList += " -Domain $Domain" }
        if ($NoService)  { $ArgList += " -NoService" }
        Start-Process powershell -Verb RunAs -ArgumentList $ArgList -Wait
        Remove-Item $TmpScript -Force -ErrorAction SilentlyContinue
        exit
    }
}

# ══════════════════════════════════════════════════════════════════════════
# 以下代码以管理员身份运行
# ══════════════════════════════════════════════════════════════════════════

$ServiceName  = "zyhive"
$InstallDir   = "C:\Program Files\ZyHive"
$ConfigDir    = "C:\ProgramData\ZyHive"
$AgentsDir    = "$ConfigDir\agents"
$BinaryPath   = "$InstallDir\zyhive.exe"
$ConfigFile   = "$ConfigDir\zyhive.json"
$InstallBase  = "https://install.zyling.ai"
$GithubBase   = "https://github.com/Zyling-ai/zyhive/releases/download"

# ── 卸载模式 ───────────────────────────────────────────────────────────────
if ($Uninstall) {
    Write-Host ""
    Write-Host "  正在卸载 ZyHive..." -ForegroundColor Red
    $svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($svc) {
        if ($svc.Status -eq "Running") { Stop-Service $ServiceName -Force }
        sc.exe delete $ServiceName | Out-Null
        Write-Ok "Windows 服务已删除"
    }
    if (Test-Path $InstallDir) { Remove-Item $InstallDir -Recurse -Force; Write-Ok "二进制已删除" }
    Write-Ok "卸载完成（配置文件保留在 $ConfigDir）"
    exit
}

# ── 检测架构 ───────────────────────────────────────────────────────────────
$Arch = if ([System.Environment]::Is64BitOperatingSystem) {
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
} else {
    Write-Err "ZyHive 不支持 32 位 Windows"
}

Write-Host ""
Write-Host "  🚀 正在安装 ZyHive (引巢 · AI 团队操作系统)…" -ForegroundColor Blue
Write-Host ""
Write-Info "操作系统：Windows / $Arch"
Write-Info "安装目录：$InstallDir"
Write-Info "配置目录：$ConfigDir"
Write-Host ""

# ── 获取最新版本 ───────────────────────────────────────────────────────────
Write-Info "查询最新版本…"
$Latest = $null
try {
    $resp = Invoke-RestMethod -Uri "$InstallBase/latest" -TimeoutSec 8 -UseBasicParsing
    $Latest = $resp.version
} catch {}
if (-not $Latest) {
    Write-Info "CF 镜像不可用，回退到 GitHub API…"
    try {
        $resp = Invoke-RestMethod -Uri "https://api.github.com/repos/Zyling-ai/zyhive/releases/latest" -TimeoutSec 10
        $Latest = $resp.tag_name
    } catch {}
}
if (-not $Latest) { Write-Err "无法获取最新版本，请检查网络连接" }
Write-Info "最新版本：$Latest"

$FileName   = "zyhive-windows-$Arch.exe"
$DownloadCF = "$InstallBase/dl/$Latest/$FileName"
$DownloadGH = "https://mirror.ghproxy.com/https://github.com/Zyling-ai/zyhive/releases/download/$Latest/$FileName"
$TmpBin     = Join-Path $env:TEMP $FileName

# ── 检测是否已安装（更新流程）─────────────────────────────────────────────
if (Test-Path $BinaryPath) {
    $Current = $null
    try { $Current = (& "$BinaryPath" --version 2>$null) -replace '.*?(v[\d.]+).*','$1' } catch {}
    if (-not $Current) { $Current = "（未知版本）" }

    Write-Host ""
    Write-Host "  检测到已安装的 ZyHive：" -NoNewline -ForegroundColor Yellow
    Write-Host $Current -ForegroundColor White
    Write-Host "  最新版本：" -NoNewline -ForegroundColor Cyan
    Write-Host $Latest -ForegroundColor White
    Write-Host ""

    if ($Current -eq $Latest) {
        Write-Host "  ✅ 已是最新版本，无需更新。" -ForegroundColor Green
        Write-Host ""
        exit 0
    }

    $Confirm = Read-Host "  是否更新 $Current → $Latest？[Y/n]"
    if ($Confirm -eq "" ) { $Confirm = "Y" }
    if ($Confirm -notmatch "^[Yy]$") {
        Write-Host ""
        Write-Info "已取消，当前版本 $Current 保持不变。"
        exit 0
    }

    Write-Host ""
    Write-Info "开始更新 $Current → $Latest…"

    # 停止服务
    $OldSvc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($OldSvc -and $OldSvc.Status -eq "Running") {
        Stop-Service $ServiceName -Force
        Start-Sleep 2
        Write-Info "服务已停止"
    }

    # 下载
    Write-Info "下载 $FileName…"
    $Downloaded = $false
    try {
        Invoke-WebRequest -Uri $DownloadCF -OutFile $TmpBin -UseBasicParsing -TimeoutSec 120
        $Downloaded = $true
    } catch { Write-Info "CF 镜像下载失败，回退到 GitHub…" }
    if (-not $Downloaded) {
        try {
            Invoke-WebRequest -Uri $DownloadGH -OutFile $TmpBin -UseBasicParsing -TimeoutSec 120
            $Downloaded = $true
        } catch {}
    }
    if (-not $Downloaded) { Write-Err "下载失败。" }

    # 替换二进制
    Remove-Item $BinaryPath -Force -ErrorAction SilentlyContinue
    Copy-Item $TmpBin $BinaryPath -Force
    Remove-Item $TmpBin -Force -ErrorAction SilentlyContinue
    Write-Ok "二进制已更新至 $BinaryPath"

    # 重启服务
    if ($OldSvc) {
        Start-Service $ServiceName -ErrorAction SilentlyContinue
        Write-Ok "服务已重启"
    }

    Write-Host ""
    Write-Host "╔══════════════════════════════════════════════╗" -ForegroundColor Green
    Write-Host "║  ✅  ZyHive 更新成功！$Current → $Latest" -ForegroundColor Green
    Write-Host "╚══════════════════════════════════════════════╝" -ForegroundColor Green
    Write-Host ""
    exit 0
}

# ── 以下为全新安装流程 ─────────────────────────────────────────────────────

# ── 创建目录 ───────────────────────────────────────────────────────────────
New-Item -ItemType Directory -Path $InstallDir, $ConfigDir, $AgentsDir -Force | Out-Null

# ── 下载二进制 ─────────────────────────────────────────────────────────────
Write-Info "下载 zyhive $Latest (windows/$Arch)…"
$Downloaded = $false
try {
    Invoke-WebRequest -Uri $DownloadCF -OutFile $TmpBin -UseBasicParsing -TimeoutSec 120
    $Downloaded = $true
} catch {
    Write-Info "CF 镜像下载失败，回退到 GitHub…"
}
if (-not $Downloaded) {
    try {
        Invoke-WebRequest -Uri $DownloadGH -OutFile $TmpBin -UseBasicParsing -TimeoutSec 120
        $Downloaded = $true
    } catch {}
}
if (-not $Downloaded) { Write-Err "下载失败。CF: $DownloadCF`nGitHub: $DownloadGH" }

# 停止旧服务再替换二进制
$OldSvc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($OldSvc -and $OldSvc.Status -eq "Running") {
    Write-Info "停止旧版本服务…"
    Stop-Service $ServiceName -Force
    Start-Sleep 2
}
Copy-Item $TmpBin $BinaryPath -Force
Remove-Item $TmpBin -Force -ErrorAction SilentlyContinue
Write-Ok "二进制已安装至 $BinaryPath"

# ── 生成默认配置 ───────────────────────────────────────────────────────────
$ShowToken = $null
if (-not (Test-Path $ConfigFile)) {
    $AdminToken = -join ((48..57) + (97..102) | Get-Random -Count 32 | ForEach-Object {[char]$_})
    $BindMode   = if ($Domain) { "localhost" } else { "lan" }
    $Config = @{
        gateway = @{ port = [int]$Port; bind = $BindMode }
        agents  = @{ dir  = $AgentsDir.Replace("\", "/") }
        models  = @{ primary = "anthropic/claude-sonnet-4-6" }
        auth    = @{ mode = "token"; token = $AdminToken }
    } | ConvertTo-Json -Depth 5
    $Config | Out-File -FilePath $ConfigFile -Encoding utf8 -Force
    Write-Host ""
    Write-Host "  🔑 管理员 Token：" -NoNewline -ForegroundColor Yellow
    Write-Host $AdminToken -ForegroundColor Green
    Write-Host "     已保存至 $ConfigFile，请妥善保存"
    $ShowToken = $AdminToken
}

# ── 安装 Windows 服务 ──────────────────────────────────────────────────────
if (-not $NoService) {
    Write-Host ""
    Write-Info "注册 Windows 服务…"

    # 删除旧服务
    if ($OldSvc) {
        sc.exe delete $ServiceName | Out-Null
        Start-Sleep 1
    }

    # sc create
    $BinPathQuoted = "`"$BinaryPath`" --config `"$ConfigFile`""
    sc.exe create $ServiceName binPath= $BinPathQuoted start= auto DisplayName= "ZyHive AI Team OS" | Out-Null
    sc.exe description $ServiceName "ZyHive (引巢) — AI 团队操作系统 | https://github.com/Zyling-ai/zyhive" | Out-Null
    sc.exe failure $ServiceName reset= 60 actions= restart/3000/restart/5000/restart/10000 | Out-Null

    Start-Service $ServiceName
    Start-Sleep 2

    $svc = Get-Service -Name $ServiceName
    if ($svc.Status -eq "Running") {
        Write-Ok "Windows 服务已启动：$ServiceName"
    } else {
        Write-Warn "服务启动异常，请手动检查：sc query $ServiceName"
    }
}

# ── 添加到 PATH ────────────────────────────────────────────────────────────
$MachPath = [System.Environment]::GetEnvironmentVariable("Path", "Machine")
if ($MachPath -notlike "*$InstallDir*") {
    [System.Environment]::SetEnvironmentVariable("Path", "$MachPath;$InstallDir", "Machine")
    Write-Info "已将 $InstallDir 加入系统 PATH（重新打开终端生效）"
}

# ── 获取访问地址 ───────────────────────────────────────────────────────────
$LocalIP = (Get-NetIPAddress -AddressFamily IPv4 -InterfaceIndex (
    (Get-NetRoute -DestinationPrefix "0.0.0.0/0" | Sort-Object RouteMetric | Select-Object -First 1).InterfaceIndex
) -ErrorAction SilentlyContinue | Select-Object -First 1).IPAddress

$PublicIP = $null
try { $PublicIP = (Invoke-RestMethod "https://api.ipify.org" -TimeoutSec 5) } catch {}

# ── 完成 ──────────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "  ╔══════════════════════════════════════════════╗" -ForegroundColor Green
Write-Host ("  ║  ✅  ZyHive 安装成功！版本: {0,-17}║" -f $Latest) -ForegroundColor Green
Write-Host "  ╚══════════════════════════════════════════════╝" -ForegroundColor Green
Write-Host ""
if ($Domain) {
    Write-Host "  🌐 访问地址：  https://$Domain" -ForegroundColor Blue
} else {
    Write-Host "  📍 本地访问：  http://localhost:$Port" -ForegroundColor Blue
    if ($LocalIP)  { Write-Host "  🏠 内网访问：  http://${LocalIP}:$Port" -ForegroundColor Blue }
    if ($PublicIP) { Write-Host "  🌐 公网访问：  http://${PublicIP}:$Port" -ForegroundColor Blue }
}
if ($ShowToken) {
    Write-Host ""
    Write-Host "  🔑 管理员 Token：" -NoNewline -ForegroundColor Yellow
    Write-Host $ShowToken -ForegroundColor Green
}
Write-Host ""
Write-Host "  📄 配置文件：  $ConfigFile"
Write-Host "  🗂  成员目录：  $AgentsDir"
Write-Host "  📦 二进制：    $BinaryPath"
Write-Host ""
Write-Host "  常用命令：" -ForegroundColor Yellow
Write-Host "    查看状态：  sc query $ServiceName"
Write-Host "    停止服务：  sc stop $ServiceName"
Write-Host "    重启服务：  sc stop $ServiceName; sc start $ServiceName"
Write-Host "    查看日志：  Get-EventLog -LogName Application -Source $ServiceName -Newest 20"
Write-Host "    CLI 管理：  zyhive  (需要管理员终端)"
Write-Host ""
