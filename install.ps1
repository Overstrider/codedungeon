[CmdletBinding()]
param(
    [ValidateSet('codex','claude','claude-code','claude-ce')]
    [string]$Provider,
    [switch]$Yes
)

$ErrorActionPreference = 'Stop'
$Here = Split-Path -Parent $MyInvocation.MyCommand.Path
$Installer = Join-Path $Here 'release/install.ps1'

if ($Provider) {
    & $Installer -Provider $Provider -Yes:$Yes
} else {
    & $Installer -Yes:$Yes
}
exit $LASTEXITCODE
