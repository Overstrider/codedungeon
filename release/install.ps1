[CmdletBinding()]
param(
    [ValidateSet('codex','claude','claude-code','claude-ce')]
    [string]$Provider,
    [string]$Target,
    [switch]$Yes
)

$ErrorActionPreference = 'Stop'

function Normalize-Provider {
    param([string]$Name)
    switch ($Name) {
        'codex' { return 'codex' }
        'claude' { return 'claude' }
        'claude-code' { return 'claude' }
        'claude-ce' { return 'claude' }
        default { throw "provider must be codex or claude" }
    }
}

function Resolve-ProjectTarget {
    param([string]$Requested)
    if ($Requested) {
        $TargetPath = (Resolve-Path -LiteralPath $Requested).Path
    } else {
        $TargetPath = (Get-Location).Path
    }
    $Inside = & git -C $TargetPath rev-parse --is-inside-work-tree 2>$null
    if ($LASTEXITCODE -ne 0 -or $Inside.Trim() -ne 'true') {
        throw "run from a git project or pass -Target <project-root>"
    }
    return $TargetPath
}

$Here = Split-Path -Parent $MyInvocation.MyCommand.Path
if (-not $Provider) {
    if ($Yes) {
        throw "provider required: .\install.ps1 -Provider codex|claude"
    }
    $Provider = Read-Host "Provider [codex/claude]"
}
$Provider = Normalize-Provider $Provider
$Target = Resolve-ProjectTarget $Target

if (-not [Environment]::Is64BitOperatingSystem) {
    throw "unsupported Windows architecture: 32-bit"
}

$Bin = "codedungeon-$Provider.exe"
$Src = Join-Path $Here "bin/$Bin"
if (-not (Test-Path -LiteralPath $Src)) {
    throw "binary missing: $Src"
}

$DestDir = Join-Path $Target ".codedungeon/bin"
New-Item -ItemType Directory -Force -Path $DestDir | Out-Null
$DestBin = Join-Path $DestDir "codedungeon-$Provider.exe"
Copy-Item -LiteralPath $Src -Destination $DestBin -Force
Write-Host "[OK] copied binary -> $DestBin"

Write-Host "[OK] running project-local setup in $Target"
$OldProvider = $env:CODEDUNGEON_PROVIDER
$env:CODEDUNGEON_PROVIDER = $Provider
try {
    & $DestBin setup --target $Target --yes
} finally {
    $env:CODEDUNGEON_PROVIDER = $OldProvider
}

Write-Host ""
Write-Host "=== Install complete ==="
Write-Host ""
Write-Host "Project binary:"
Write-Host "  $DestBin"
if ($Provider -eq 'codex') {
    Write-Host "Codex workflow router is installed under .agents/skills/codedungeon."
} else {
    Write-Host "Claude Code workflow router is available as /codedungeon."
}
