[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$')]
    [string] $Version,

    [Parameter(Mandatory = $true)]
    [string] $ArchivePath,

    [Parameter(Mandatory = $true)]
    [string] $OutputDirectory,

    [switch] $AllowFixtureNotices
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true

function Get-MsiProperty {
    param(
        [Parameter(Mandatory = $true)] $Database,
        [Parameter(Mandatory = $true)] [string] $Name
    )

    $query = "SELECT ``Value`` FROM ``Property`` WHERE ``Property`` = '$Name'"
    $view = $Database.GetType().InvokeMember('OpenView', 'InvokeMethod', $null, $Database, @($query))
    $null = $view.GetType().InvokeMember('Execute', 'InvokeMethod', $null, $view, $null)
    $record = $view.GetType().InvokeMember('Fetch', 'InvokeMethod', $null, $view, $null)
    if ($null -eq $record) {
        throw "MSI property $Name is missing"
    }
    return $record.GetType().InvokeMember('StringData', 'GetProperty', $null, $record, 1)
}

if (-not (Test-Path -LiteralPath $ArchivePath -PathType Leaf)) {
    throw "archive does not exist: $ArchivePath"
}
if (-not (Get-Command wix -ErrorAction SilentlyContinue)) {
    throw 'wix is not installed or is not on PATH'
}

$workingDirectory = Join-Path $env:RUNNER_TEMP "windows-msi-$Version"
$archiveDirectory = Join-Path $workingDirectory 'archive'
$wixDirectory = Join-Path $workingDirectory 'wix'
$extractDirectory = Join-Path $workingDirectory 'installed'
Remove-Item -LiteralPath $workingDirectory -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Path $archiveDirectory, $wixDirectory, $extractDirectory -Force | Out-Null

$archiveEntries = @(& tar -tzf $ArchivePath)
if ($LASTEXITCODE -ne 0 -or $archiveEntries.Count -eq 0) {
    throw 'unable to list release archive'
}
$archiveRoot = "d2-$Version/"
$metadataRoot = "._d2-$Version"
foreach ($entry in $archiveEntries) {
    $normalizedEntry = $entry.Replace('\', '/')
    if ($normalizedEntry.StartsWith('/', [System.StringComparison]::Ordinal) -or
        $normalizedEntry -match '^[A-Za-z]:' -or
        $normalizedEntry -match '(^|/)\.\.(/|$)') {
        throw "release archive contains an unsafe path: $entry"
    }
    $isReleasePath = $normalizedEntry.StartsWith($archiveRoot, [System.StringComparison]::Ordinal)
    $isMetadataPath = $normalizedEntry -eq $metadataRoot -or
        $normalizedEntry.StartsWith("$metadataRoot/", [System.StringComparison]::Ordinal)
    if (-not $isReleasePath -and -not $isMetadataPath) {
        throw "release archive contains a path outside ${archiveRoot}: $entry"
    }
}

& tar -xzf $ArchivePath -C $archiveDirectory
if ($LASTEXITCODE -ne 0) {
    throw 'unable to extract release archive'
}

$releaseDirectory = Join-Path $archiveDirectory "d2-$Version"
$binaryPath = Join-Path $releaseDirectory 'bin/d2.exe'
$noticesPath = Join-Path $releaseDirectory 'THIRD_PARTY_NOTICES.txt'
if (-not (Test-Path -LiteralPath $binaryPath -PathType Leaf)) {
    throw "release archive does not contain $archiveRoot/bin/d2.exe"
}
if (-not (Test-Path -LiteralPath $noticesPath -PathType Leaf)) {
    if (-not $AllowFixtureNotices) {
        throw "release archive does not contain $archiveRoot/THIRD_PARTY_NOTICES.txt"
    }

    # v0.7.1 predates notices in release archives. Its published binary remains the
    # continuity fixture, while current notices exercise the installer component.
    $noticesPath = Join-Path $PSScriptRoot '../../../THIRD_PARTY_NOTICES.txt'
    if (-not (Test-Path -LiteralPath $noticesPath -PathType Leaf)) {
        throw 'fixture notices file is missing'
    }
}

Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'd2.wxs') -Destination $wixDirectory
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'd2.ico') -Destination $wixDirectory
Copy-Item -LiteralPath $binaryPath -Destination (Join-Path $wixDirectory 'd2.exe')
Copy-Item -LiteralPath $noticesPath -Destination (Join-Path $wixDirectory 'THIRD_PARTY_NOTICES.txt')

$msiVersion = $Version.Substring(1).Split('-')[0]
Push-Location $wixDirectory
try {
    & wix build -arch x64 -d "D2Version=$msiVersion" ./d2.wxs
    if ($LASTEXITCODE -ne 0) {
        throw "wix exited with status $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

$msiPath = Join-Path $wixDirectory 'd2.msi'
if (-not (Test-Path -LiteralPath $msiPath -PathType Leaf)) {
    throw 'wix did not produce d2.msi'
}

$installer = New-Object -ComObject WindowsInstaller.Installer
$database = $installer.GetType().InvokeMember(
    'OpenDatabase',
    'InvokeMethod',
    $null,
    $installer,
    @((Resolve-Path -LiteralPath $msiPath).Path, 0)
)

$expectedProperties = @{
    ProductName = 'D2'
    Manufacturer = 'D2'
    UpgradeCode = '{AC84FEE7-EB67-4F5D-A08D-ADEF69538690}'
}
foreach ($property in $expectedProperties.Keys) {
    $actual = Get-MsiProperty -Database $database -Name $property
    if ($actual -cne $expectedProperties[$property]) {
        throw "MSI $property is '$actual', expected '$($expectedProperties[$property])'"
    }
}

$productVersion = Get-MsiProperty -Database $database -Name ProductVersion
if ($productVersion -cne $msiVersion) {
    throw "MSI ProductVersion is '$productVersion', expected $msiVersion"
}

$installArguments = "/a `"$msiPath`" /qn TARGETDIR=`"$extractDirectory`""
$install = Start-Process -FilePath msiexec.exe -ArgumentList $installArguments -Wait -PassThru
if ($install.ExitCode -notin @(0, 3010)) {
    throw "administrative MSI extraction exited with status $($install.ExitCode)"
}

$installedBinaries = @(Get-ChildItem -LiteralPath $extractDirectory -Filter d2.exe -File -Recurse)
$installedNotices = @(Get-ChildItem -LiteralPath $extractDirectory -Filter THIRD_PARTY_NOTICES.txt -File -Recurse)
if ($installedBinaries.Count -ne 1 -or $installedNotices.Count -ne 1) {
    throw 'MSI did not install exactly one binary and one notices file'
}

$sourceBinaryHash = (Get-FileHash -LiteralPath $binaryPath -Algorithm SHA256).Hash
$installedBinaryHash = (Get-FileHash -LiteralPath $installedBinaries[0].FullName -Algorithm SHA256).Hash
if ($sourceBinaryHash -cne $installedBinaryHash) {
    throw 'installed d2.exe does not match the release archive'
}
$sourceNoticesHash = (Get-FileHash -LiteralPath $noticesPath -Algorithm SHA256).Hash
$installedNoticesHash = (Get-FileHash -LiteralPath $installedNotices[0].FullName -Algorithm SHA256).Hash
if ($sourceNoticesHash -cne $installedNoticesHash) {
    throw 'installed THIRD_PARTY_NOTICES.txt does not match the release archive'
}

$versionOutput = (& $installedBinaries[0].FullName --version 2>&1 | Out-String).Trim()
if ($LASTEXITCODE -ne 0 -or $versionOutput -cne $Version) {
    throw "installed d2.exe reported an unexpected version: $versionOutput"
}

New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
$outputPath = Join-Path $OutputDirectory "d2-$Version-windows-amd64.msi"
Copy-Item -LiteralPath $msiPath -Destination $outputPath -Force

$outputHash = (Get-FileHash -LiteralPath $outputPath -Algorithm SHA256).Hash.ToLowerInvariant()
Write-Host "Built $outputPath"
Write-Host "SHA-256: $outputHash"
