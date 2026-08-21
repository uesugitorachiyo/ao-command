[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$CandidateDirectory,

    [Parameter(Mandatory = $true)]
    [string]$Archive,

    [Parameter(Mandatory = $true)]
    [string]$ExpectedSourceCommit,

    [Parameter(Mandatory = $true)]
    [string]$ExpectedVersion,

    [Parameter(Mandatory = $true)]
    [string]$Output
)

$ErrorActionPreference = 'Stop'
$installDirectory = $null
$cleanupVerified = $false
$result = $null

function Resolve-ContainedFile {
    param([string]$Root, [string]$Path, [string]$Label)

    $resolved = [IO.Path]::GetFullPath((Join-Path $Root $Path))
    $prefix = $Root.TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
    if (-not $resolved.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "$Label is outside candidate directory"
    }
    if (-not [IO.File]::Exists($resolved)) {
        throw "$Label is absent"
    }
    return $resolved
}

function Resolve-ContainedOutput {
    param([string]$Root, [string]$Path)

    $resolved = [IO.Path]::GetFullPath($Path)
    $prefix = $Root.TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
    if (-not $resolved.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw 'output is outside candidate directory'
    }
    if ([IO.File]::Exists($resolved)) {
        throw 'output already exists'
    }
    return $resolved
}

try {
    $candidateRoot = (Resolve-Path -LiteralPath $CandidateDirectory).Path
    $outputPath = Resolve-ContainedOutput $candidateRoot $Output
    if ([IO.Path]::IsPathRooted($Archive) -or [IO.Path]::GetFileName($Archive) -cne $Archive) {
        throw 'archive must be a basename'
    }
    $archivePath = Resolve-ContainedFile $candidateRoot $Archive 'archive'
    $checksumPath = Resolve-ContainedFile $candidateRoot 'SHA256SUMS' 'SHA256SUMS'

    $checksumText = [IO.File]::ReadAllText($checksumPath, [Text.Encoding]::UTF8)
    $escapedArchive = [Regex]::Escape($Archive)
    $match = [Regex]::Match($checksumText, "\A([0-9a-f]{64})  $escapedArchive\r?\n\z", [Text.RegularExpressions.RegexOptions]::CultureInvariant)
    if (-not $match.Success) {
        throw 'SHA256SUMS must contain exactly one canonical archive entry'
    }
    $expectedDigest = $match.Groups[1].Value
    if ($PSVersionTable.PSEdition -eq 'Desktop') {
        Import-Module (Join-Path $PSHOME 'Modules\Microsoft.PowerShell.Utility') -ErrorAction Stop
    }
    else {
        Import-Module Microsoft.PowerShell.Utility -ErrorAction Stop
    }
    $actualDigest = (Microsoft.PowerShell.Utility\Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualDigest -cne $expectedDigest) {
        throw 'archive checksum mismatch'
    }

    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $expectedMembers = @(
        'ao-command.exe',
        'LICENSE',
        'functional-smoke.json',
        'help-smoke.txt',
        'provenance.json',
        'sbom.json',
        'version-readback.json'
    )
    $seen = New-Object 'System.Collections.Generic.HashSet[string]' ([StringComparer]::OrdinalIgnoreCase)
    $actualMembers = New-Object 'System.Collections.Generic.List[string]'
    $zip = [IO.Compression.ZipFile]::OpenRead($archivePath)
    try {
        foreach ($entry in $zip.Entries) {
            $name = $entry.FullName
            $segments = $name -split '[\\/]'
            if ([string]::IsNullOrEmpty($name) -or $name.EndsWith('/') -or $name.EndsWith('\') -or
                [IO.Path]::IsPathRooted($name) -or $name.Contains(':') -or $segments -contains '..' -or
                $segments.Count -ne 1) {
                throw 'archive contains unsafe member'
            }
            if (-not $seen.Add($name)) {
                throw 'archive contains duplicate members'
            }
            $actualMembers.Add($name)
        }
    }
    finally {
        $zip.Dispose()
    }
    if ($actualMembers.Count -ne $expectedMembers.Count) {
        throw 'archive exact inventory mismatch'
    }
    foreach ($name in $expectedMembers) {
        if (-not $actualMembers.Contains($name)) {
            throw 'archive exact inventory mismatch'
        }
    }

    $installRoot = Join-Path $env:TEMP 'AO Command Candidate Install With Spaces'
    [void](New-Item -ItemType Directory -Path $installRoot -Force)
    $installDirectory = Join-Path $installRoot ([Guid]::NewGuid().ToString('D'))
    [void](New-Item -ItemType Directory -Path $installDirectory)
    Expand-Archive -LiteralPath $archivePath -DestinationPath $installDirectory

    $installedNames = @([IO.Directory]::EnumerateFiles($installDirectory, '*', [IO.SearchOption]::AllDirectories) | ForEach-Object {
        $_.Substring($installDirectory.Length + 1).Replace('\', '/')
    })
    if ($installedNames.Count -ne $expectedMembers.Count) {
        throw 'installed exact inventory mismatch'
    }
    foreach ($name in $expectedMembers) {
        if (-not ($installedNames -ccontains $name)) {
            throw 'installed exact inventory mismatch'
        }
    }

    $binary = Join-Path $installDirectory 'ao-command.exe'
    $versionJSON = & $binary version --json
    if ($LASTEXITCODE -ne 0) {
        throw 'version diagnostic failed'
    }
    $version = ($versionJSON -join "`n") | ConvertFrom-Json
    if ($version.schema_version -cne 'ao.command.version.v0.1' -or
        $version.version -cne $ExpectedVersion -or
        $version.source_commit -cne $ExpectedSourceCommit -or
        $version.provider_calls -ne $false) {
        throw 'version diagnostic identity mismatch'
    }

    $sourceRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
    $missionFixture = Join-Path $sourceRoot 'examples\mission\command-status.ready.json'
    Push-Location $sourceRoot
    try {
        $missionJSON = & $binary mission status --status $missionFixture --json
        if ($LASTEXITCODE -ne 0) {
            throw 'Mission status diagnostic failed'
        }
    }
    finally {
        Pop-Location
    }
    $mission = ($missionJSON -join "`n") | ConvertFrom-Json
    if ($mission.command_schema_version -cne 'ao.command.v0.1' -or
        $mission.status -cne 'ready' -or
        $mission.operator_mode -cne 'read_only' -or
        $mission.safe_to_execute -ne $false -or
        $mission.executes_work -ne $false -or
        $mission.approves_work -ne $false -or
        $mission.mutates_repositories -ne $false) {
        throw 'Mission status diagnostic authority mismatch'
    }

    $result = [ordered]@{
        schema_version = 'ao.command.windows-candidate-qualification.v0.1'
        status = 'passed'
        archive = $Archive
        archive_sha256 = $actualDigest
        source_commit = $ExpectedSourceCommit
        version = $ExpectedVersion
        powershell_edition = [string]$PSVersionTable.PSEdition
        powershell_version = [string]$PSVersionTable.PSVersion
        install_path_contains_spaces = $installDirectory.Contains(' ')
        doctor = [ordered]@{
            status = 'not_applicable'
            replacement_diagnostic = 'version_and_mission_status'
        }
        provider_calls = $false
        cleanup_verified = $false
        authority = [ordered]@{
            safe_to_execute = $false
            executes_work = $false
            approves_work = $false
            mutates_repositories = $false
            releases_or_deploys = $false
        }
    }
}
finally {
    if ($null -ne $installDirectory) {
        try {
            Remove-Item -LiteralPath $installDirectory -Recurse -Force -ErrorAction Stop
        }
        catch {
            # The existence check below is the cleanup authority.
        }
        $cleanupVerified = -not (Test-Path -LiteralPath $installDirectory)
    }
    else {
        $cleanupVerified = $true
    }
    if (-not $cleanupVerified) {
        throw 'candidate install cleanup could not be verified'
    }
}

$result.cleanup_verified = $true
$outputParent = [IO.Path]::GetDirectoryName($outputPath)
if (-not [IO.Directory]::Exists($outputParent)) {
    [void][IO.Directory]::CreateDirectory($outputParent)
}
$json = $result | ConvertTo-Json -Depth 5
[IO.File]::WriteAllText($outputPath, $json + [Environment]::NewLine, (New-Object Text.UTF8Encoding($false)))
