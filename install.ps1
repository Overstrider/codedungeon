[CmdletBinding()]
param(
    [ValidateSet('codex','claude','claude-code','claude-ce')]
    [string]$Provider,
    [switch]$Yes
)

$ErrorActionPreference = 'Stop'
$Here = Split-Path -Parent $MyInvocation.MyCommand.Path
$Installer = Join-Path $Here 'release/install.ps1'

$ArgsList = @()
if ($Provider) {
    $ArgsList += @('-Provider', $Provider)
}
if ($Yes) {
    $ArgsList += '-Yes'
}

& $Installer @ArgsList
exit $LASTEXITCODE
