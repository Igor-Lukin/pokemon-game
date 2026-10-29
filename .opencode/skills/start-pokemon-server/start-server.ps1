[CmdletBinding()]
param(
    [switch]$Stop,
    [switch]$Status
)

$ErrorActionPreference = "Stop"
$repo = (Resolve-Path (Join-Path $PSScriptRoot "..\..\..")).Path
$tmp = Join-Path $env:TEMP "opencode"
New-Item -ItemType Directory -Force -Path $tmp | Out-Null
$exe = Join-Path $tmp "pokemon-server.exe"
$cmdPath = Join-Path $tmp "run-pokemon.cmd"
$outLog = Join-Path $tmp "srv.out"
$errLog = Join-Path $tmp "srv.err"

function Get-Listener {
    Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue |
        Select-Object -First 1
}

if ($Stop) {
    $listener = Get-Listener
    if (-not $listener) {
        "server already stopped"
        return
    }
    Stop-Process -Id $listener.OwningProcess -Force
    Start-Sleep -Milliseconds 300
    if (Get-Listener) { "server still running" } else { "server stopped" }
    return
}

if ($Status) {
    $listener = Get-Listener
    if ($listener) {
        "server up pid=$($listener.OwningProcess) http://localhost:8080"
    } else {
        "server down"
    }
    return
}

$listener = Get-Listener
if ($listener) {
    "server already running pid=$($listener.OwningProcess) http://localhost:8080"
    return
}

Push-Location $repo
try {
    & go build -o $exe ./server
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }
} finally {
    Pop-Location
}

$cmdContent = "@echo off`r`n" +
    "set SESSION_KEY=local-test-key`r`n" +
    "cd /d `"$repo`"`r`n" +
    "`"$exe`" > `"$outLog`" 2> `"$errLog`"`r`n"
[System.IO.File]::WriteAllText($cmdPath, $cmdContent, (New-Object System.Text.ASCIIEncoding))

$res = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{
    CommandLine = "cmd.exe /c `"$cmdPath`""
}
if ($res.ReturnValue -ne 0) {
    throw "Win32_Process.Create failed: $($res.ReturnValue)"
}

for ($i = 0; $i -lt 10; $i++) {
    Start-Sleep -Milliseconds 200
    $listener = Get-Listener
    if ($listener) {
        "server up pid=$($listener.OwningProcess) http://localhost:8080"
        return
    }
}

"server launch requested (cmd pid=$($res.ProcessId)), port 8080 is not listening yet."
if (Test-Path $errLog) {
    "--- stderr ---"
    Get-Content $errLog -Encoding UTF8 -Tail 20
}
