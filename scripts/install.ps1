# sdkz 一键安装脚本 (Windows)
# 用法:
#   irm https://raw.githubusercontent.com/bigwg/sdkz/main/scripts/install.ps1 | iex
$ErrorActionPreference = "Stop"

$Repo   = "bigwg/sdkz"
$Arch   = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
$Asset  = "sdkz-windows-$Arch.tar.gz"
$Url    = "https://github.com/$Repo/releases/latest/download/$Asset"
$InstallDir = Join-Path $env:LOCALAPPDATA "sdkz"

Write-Host "检测架构: $Arch"
Write-Host "下载 $Url"

$tmp = New-Item -ItemType Directory -Path (Join-Path $env:TEMP "sdkz-install") -Force
Invoke-WebRequest -Uri $Url -OutFile (Join-Path $tmp $Asset)
tar.exe -xzf (Join-Path $tmp $Asset) -C $tmp | Out-Null

$dest = Join-Path $InstallDir "sdkz.exe"
New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
Copy-Item -Path (Join-Path $tmp "sdkz.exe") -Destination $dest -Force
Write-Host "已安装到 $dest"

# 加入用户 PATH（避免重复）
$paths = [Environment]::GetEnvironmentVariable("Path", "User") -split ";"
if ($paths -notcontains $InstallDir) {
  [Environment]::SetEnvironmentVariable("Path", ($paths + $InstallDir) -join ";", "User")
  Write-Host "已将 $InstallDir 加入用户 PATH，请重启终端使其生效。"
}

& $dest init | Out-Null
Write-Host "完成！运行 sdkz list java 开始使用（已安装版本升级用 sdkz selfupdate）。"
