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
$installRoot = $null
$installRootCreated = $false
$cleanupVerified = $false
$result = $null
$outputCreated = $false
$MaxArchiveBytes = 32 * 1024 * 1024
$MaxEntryUncompressedBytes = 16 * 1024 * 1024
$MaxTotalUncompressedBytes = 32 * 1024 * 1024
$MaxCompressionRatio = 200

function Assert-NoReparseComponents {
    param([string]$Path, [string]$Label)

    $full = [IO.Path]::GetFullPath($Path)
    $current = [IO.Path]::GetPathRoot($full)
    $relative = $full.Substring($current.Length)
    foreach ($part in ($relative -split '[\\/]' | Where-Object { $_ })) {
        $current = Join-Path $current $part
        if ([IO.File]::Exists($current) -or [IO.Directory]::Exists($current)) {
            $attributes = [IO.File]::GetAttributes($current)
            if (($attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                throw "$Label contains a reparse point"
            }
        }
    }
}

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
    Assert-NoReparseComponents $resolved $Label
    return $resolved
}

function Resolve-ContainedOutput {
    param([string]$Root, [string]$Path)

    $resolved = [IO.Path]::GetFullPath($Path)
    $prefix = $Root.TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
    if (-not $resolved.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw 'output is outside candidate directory'
    }
    Assert-NoReparseComponents ([IO.Path]::GetDirectoryName($resolved)) 'output parent'
    return $resolved
}

function Test-DirectoryEmpty {
    param([string]$Path)

    $enumerator = [IO.Directory]::EnumerateFileSystemEntries($Path).GetEnumerator()
    try {
        return -not $enumerator.MoveNext()
    }
    finally {
        $enumerator.Dispose()
    }
}

function Get-ZipCentralCRCs {
    param([string]$Path)

    $data = [IO.File]::ReadAllBytes($Path)
    $minimum = [Math]::Max(0, $data.Length - 65557)
    $eocd = -1
    for ($index = $data.Length - 22; $index -ge $minimum; $index--) {
        if ($data[$index] -eq 0x50 -and $data[$index + 1] -eq 0x4b -and
            $data[$index + 2] -eq 0x05 -and $data[$index + 3] -eq 0x06) {
            $eocd = $index
            break
        }
    }
    if ($eocd -lt 0) { throw 'archive central directory is malformed' }
    $entryCount = [BitConverter]::ToUInt16($data, $eocd + 10)
    $centralSize = [BitConverter]::ToUInt32($data, $eocd + 12)
    $centralOffset = [BitConverter]::ToUInt32($data, $eocd + 16)
    $commentLength = [BitConverter]::ToUInt16($data, $eocd + 20)
    if ([BitConverter]::ToUInt16($data, $eocd + 4) -ne 0 -or
        [BitConverter]::ToUInt16($data, $eocd + 6) -ne 0 -or
        [BitConverter]::ToUInt16($data, $eocd + 8) -ne $entryCount -or
        $entryCount -ne $expectedMembers.Count -or
        ([Int64]$centralOffset + [Int64]$centralSize) -ne $eocd -or
        ($eocd + 22 + $commentLength) -ne $data.Length) {
        throw 'archive central directory is malformed'
    }
    $crcs = New-Object 'System.Collections.Generic.Dictionary[string,uint32]' ([StringComparer]::OrdinalIgnoreCase)
    $position = [int]$centralOffset
    for ($entryIndex = 0; $entryIndex -lt $entryCount; $entryIndex++) {
        if ($position + 46 -gt $eocd -or [BitConverter]::ToUInt32($data, $position) -ne 0x02014b50) {
            throw 'archive central directory is malformed'
        }
        $flags = [BitConverter]::ToUInt16($data, $position + 8)
        $compressedSize = [BitConverter]::ToUInt32($data, $position + 20)
        $uncompressedSize = [BitConverter]::ToUInt32($data, $position + 24)
        $nameLength = [BitConverter]::ToUInt16($data, $position + 28)
        $extraLength = [BitConverter]::ToUInt16($data, $position + 30)
        $entryCommentLength = [BitConverter]::ToUInt16($data, $position + 32)
        $next = $position + 46 + $nameLength + $extraLength + $entryCommentLength
        if (($flags -band 1) -ne 0 -or $compressedSize -eq [uint32]::MaxValue -or
            $uncompressedSize -eq [uint32]::MaxValue -or $next -gt $eocd) {
            throw 'archive central directory is malformed'
        }
        $encoding = if (($flags -band 0x0800) -ne 0) { [Text.Encoding]::UTF8 } else { [Text.Encoding]::ASCII }
        $name = $encoding.GetString($data, $position + 46, $nameLength)
        if ($crcs.ContainsKey($name)) {
            throw 'archive contains duplicate members'
        }
        $crcs.Add($name, [BitConverter]::ToUInt32($data, $position + 16))
        $position = $next
    }
    if ($position -ne $eocd) { throw 'archive central directory is malformed' }
    return ,$crcs
}

try {
    $candidateRoot = [IO.Path]::GetFullPath($CandidateDirectory)
    if (-not [IO.Directory]::Exists($candidateRoot)) {
        throw 'candidate directory is absent'
    }
    Assert-NoReparseComponents $candidateRoot 'candidate directory'
    $outputPath = Resolve-ContainedOutput $candidateRoot $Output
    if ([IO.Path]::IsPathRooted($Archive) -or [IO.Path]::GetFileName($Archive) -cne $Archive) {
        throw 'archive must be a basename'
    }
    $archivePath = Resolve-ContainedFile $candidateRoot $Archive 'archive'
    $checksumPath = Resolve-ContainedFile $candidateRoot 'SHA256SUMS' 'SHA256SUMS'
    $archiveLength = (New-Object IO.FileInfo($archivePath)).Length
    if ($archiveLength -le 0 -or $archiveLength -gt $MaxArchiveBytes) {
        throw 'archive exceeds bounded size'
    }

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
    Add-Type -TypeDefinition @'
public static class AOCommandCandidateCrc32
{
    public static uint Update(uint crc, byte[] buffer, int count)
    {
        for (int index = 0; index < count; index++)
        {
            crc ^= buffer[index];
            for (int bit = 0; bit < 8; bit++)
                crc = (crc & 1) != 0 ? 0xedb88320u ^ (crc >> 1) : crc >> 1;
        }
        return crc;
    }

    public static uint Finish(uint crc) { return crc ^ 0xffffffffu; }
}
'@
    $expectedMembers = @(
        'ao-command.exe',
        'LICENSE',
        'functional-smoke.json',
        'help-smoke.txt',
        'provenance.json',
        'sbom.json',
        'version-readback.json'
    )
    $centralCRCs = Get-ZipCentralCRCs $archivePath
    $seen = New-Object 'System.Collections.Generic.HashSet[string]' ([StringComparer]::OrdinalIgnoreCase)
    $actualMembers = New-Object 'System.Collections.Generic.List[string]'
    $entries = New-Object Collections.ArrayList
    $declaredTotal = [Int64]0
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
            if ($entry.Length -gt $MaxEntryUncompressedBytes) {
                throw 'archive member exceeds bounded size'
            }
            $declaredTotal += $entry.Length
            if ($declaredTotal -gt $MaxTotalUncompressedBytes) {
                throw 'archive exceeds bounded expanded size'
            }
            if ($entry.Length -gt 0 -and
                ($entry.CompressedLength -eq 0 -or ([double]$entry.Length / [double]$entry.CompressedLength) -gt $MaxCompressionRatio)) {
                throw 'archive member exceeds compression ratio limit'
            }
            $actualMembers.Add($name)
            [void]$entries.Add($entry)
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
        if ([IO.Directory]::Exists($installRoot)) {
            Assert-NoReparseComponents $installRoot 'candidate install root'
        }
        else {
            [void][IO.Directory]::CreateDirectory($installRoot)
            $installRootCreated = $true
        }
        $installDirectory = Join-Path $installRoot ([Guid]::NewGuid().ToString('D'))
        [void][IO.Directory]::CreateDirectory($installDirectory)

        $buffer = New-Object byte[] (64 * 1024)
        $expandedTotal = [Int64]0
        foreach ($entry in $entries) {
            $destination = Join-Path $installDirectory $entry.FullName
            $inputStream = $null
            $outputStream = $null
            try {
                $inputStream = $entry.Open()
                $outputStream = New-Object IO.FileStream(
                    $destination,
                    [IO.FileMode]::CreateNew,
                    [IO.FileAccess]::Write,
                    [IO.FileShare]::None
                )
                $entryWritten = [Int64]0
                $entryCrc = [uint32]::MaxValue
                while (($read = $inputStream.Read($buffer, 0, $buffer.Length)) -gt 0) {
                    $entryWritten += $read
                    $expandedTotal += $read
                    if ($entryWritten -gt $MaxEntryUncompressedBytes -or
                        $expandedTotal -gt $MaxTotalUncompressedBytes -or
                        $entryWritten -gt $entry.Length) {
                        throw 'archive extraction exceeds bounded size'
                    }
                    $entryCrc = [AOCommandCandidateCrc32]::Update($entryCrc, $buffer, $read)
                    $outputStream.Write($buffer, 0, $read)
                }
                if ($entryWritten -ne $entry.Length) {
                    throw 'archive member declared length mismatch'
                }
                if ([AOCommandCandidateCrc32]::Finish($entryCrc) -ne $centralCRCs[$entry.FullName]) {
                    throw 'archive member CRC mismatch'
                }
            }
            finally {
                if ($null -ne $outputStream) { $outputStream.Dispose() }
                if ($null -ne $inputStream) { $inputStream.Dispose() }
            }
        }
    }
    finally {
        $zip.Dispose()
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
        limits = [ordered]@{
            max_archive_bytes = $MaxArchiveBytes
            max_entry_uncompressed_bytes = $MaxEntryUncompressedBytes
            max_total_uncompressed_bytes = $MaxTotalUncompressedBytes
            max_compression_ratio = $MaxCompressionRatio
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
    if ($installRootCreated -and [IO.Directory]::Exists($installRoot)) {
        if (Test-DirectoryEmpty $installRoot) {
            [IO.Directory]::Delete($installRoot, $false)
        }
        if ([IO.Directory]::Exists($installRoot) -and (Test-DirectoryEmpty $installRoot)) {
            throw 'script-created candidate install root cleanup could not be verified'
        }
    }
}

$result.cleanup_verified = $true
$outputParent = [IO.Path]::GetDirectoryName($outputPath)
if (-not [IO.Directory]::Exists($outputParent)) {
    [void][IO.Directory]::CreateDirectory($outputParent)
}
Assert-NoReparseComponents $outputParent 'output parent'
$json = $result | ConvertTo-Json -Depth 5
$reportStream = $null
$reportWriter = $null
try {
    $reportStream = New-Object IO.FileStream(
        $outputPath,
        [IO.FileMode]::CreateNew,
        [IO.FileAccess]::Write,
        [IO.FileShare]::None
    )
    $outputCreated = $true
    $reportWriter = New-Object IO.StreamWriter($reportStream, (New-Object Text.UTF8Encoding($false)))
    $reportStream = $null
    $reportWriter.Write($json + [Environment]::NewLine)
    $reportWriter.Flush()
    $reportWriter.Dispose()
    $reportWriter = $null
}
catch {
    $writeError = $_
    try { if ($null -ne $reportWriter) { $reportWriter.Dispose(); $reportWriter = $null } } catch {}
    try { if ($null -ne $reportStream) { $reportStream.Dispose(); $reportStream = $null } } catch {}
    if ($outputCreated -and [IO.File]::Exists($outputPath)) {
        try { [IO.File]::Delete($outputPath) } catch {}
    }
    throw $writeError
}
finally {
    if ($null -ne $reportWriter) { $reportWriter.Dispose() }
    if ($null -ne $reportStream) { $reportStream.Dispose() }
}
