[CmdletBinding()]
param(
    [ValidateSet('codex','claude','claude-code','claude-ce')]
    [string]$Provider,
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

$Here = Split-Path -Parent $MyInvocation.MyCommand.Path
if (-not $Provider) {
    if ($Yes) {
        throw "provider required: .\install.ps1 -Provider codex|claude"
    }
    $Provider = Read-Host "Provider [codex/claude]"
}
$Provider = Normalize-Provider $Provider

if (-not [Environment]::Is64BitOperatingSystem) {
    throw "unsupported Windows architecture: 32-bit"
}

$Bin = "codedungeon-$Provider.exe"
$Src = Join-Path $Here "bin/$Bin"
if (-not (Test-Path -LiteralPath $Src)) {
    throw "binary missing: $Src"
}

$DestDir = Join-Path $HOME ".local/bin"
New-Item -ItemType Directory -Force -Path $DestDir | Out-Null
$DestBin = Join-Path $DestDir "codedungeon-$Provider.exe"
Copy-Item -LiteralPath $Src -Destination $DestBin -Force
Write-Host "[OK] copied binary -> $DestBin"

if ($Provider -eq 'claude') {
    $Plug = Join-Path $HOME ".claude/plugins/local/codedungeon"
    $PlugBinDir = Join-Path $Plug "bin"
    $PlugSkillDir = Join-Path $Plug "skills/grimoire-cli"
    $ManifestDir = Join-Path $Plug ".claude-plugin"
    New-Item -ItemType Directory -Force -Path $PlugBinDir, $PlugSkillDir, $ManifestDir | Out-Null

    $PluginBin = Join-Path $PlugBinDir "codedungeon.exe"
    Copy-Item -LiteralPath $Src -Destination $PluginBin -Force
    Write-Host "[OK] plugin binary -> $PluginBin"

    Copy-Item -LiteralPath (Join-Path $Here "skills/grimoire-cli/SKILL.md") -Destination (Join-Path $PlugSkillDir "SKILL.md") -Force
    Write-Host "[OK] skill -> $PlugSkillDir"

    @'
{
  "name": "codedungeon",
  "version": "0.8.0",
  "description": "Deterministic Go CLI for project pipelines: phase state, review, repo discovery, QA, code-review, task decomposition. SQLite FTS5 backend, embedded prompts.",
  "author": { "name": "loldinis" }
}
'@ | Set-Content -LiteralPath (Join-Path $ManifestDir "plugin.json") -Encoding UTF8
    Write-Host "[OK] manifest -> $(Join-Path $ManifestDir "plugin.json")"
} else {
    Write-Host "[OK] Codex provider selected; no global Claude plugin installed"
}

Write-Host ""
Write-Host "=== Install complete ==="
Write-Host ""
Write-Host "Next steps:"
Write-Host "  1. cd into any git project"
Write-Host "  2. codedungeon-$Provider setup"
if ($Provider -eq 'codex') {
    Write-Host "  3. Codex workflow skills become available under .agents/skills"
} else {
    Write-Host "  3. Claude Code slash commands become available:"
    Write-Host "     /one-shot, /side-quest, /main-quest, /code-review"
}

& $DestBin version --human
