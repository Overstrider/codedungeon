[CmdletBinding()]
param(
    [ValidateSet('codex','claude','claude-code','claude-ce')]
    [string]$Provider,
    [string]$Target,
    [switch]$Yes
)

$ErrorActionPreference = 'Stop'
$Here = Split-Path -Parent $MyInvocation.MyCommand.Path
$Installer = Join-Path $Here 'release/install.ps1'

if ($Provider) {
    & $Installer -Provider $Provider -Target $Target -Yes:$Yes
} else {
    & $Installer -Target $Target -Yes:$Yes
}
exit $LASTEXITCODE
