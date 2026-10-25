$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot

$taskVersionMatch = [regex]::Match((Get-Content -LiteralPath 'version.go' -Raw), 'appVersion = "([0-9.]+)"')
if (-not $taskVersionMatch.Success) { throw 'Cannot read appVersion from version.go' }
$taskVersion = $taskVersionMatch.Groups[1].Value
$taskName = "StayCalc-v$taskVersion-windows-amd64"
$taskOutput = Join-Path $PSScriptRoot 'release'
$taskStage = Join-Path $taskOutput $taskName
if (Test-Path -LiteralPath $taskStage) { throw "Output already exists: $taskStage" }

$env:CGO_ENABLED = '1'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
if (-not $env:CC -and (Test-Path -LiteralPath 'C:\msys64\ucrt64\bin\gcc.exe')) {
    $env:CC = 'C:\msys64\ucrt64\bin\gcc.exe'
    $env:PATH = 'C:\msys64\ucrt64\bin;' + $env:PATH
}
New-Item -ItemType Directory -Path (Join-Path $taskStage 'data') -Force | Out-Null
go build -trimpath -buildvcs=false -ldflags '-H=windowsgui -s -w' -o (Join-Path $taskStage 'StayCalc.exe') .
if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }

# Explicit allowlist: never copy profiles, settings, or the working data directory.
Copy-Item -LiteralPath 'countries_default.json' -Destination (Join-Path $taskStage 'data/countries.json')
foreach ($taskDoc in @('README.md', 'CHANGELOG.md', 'RULES_RU.md', 'RELEASE_NOTES.md')) {
    Copy-Item -LiteralPath $taskDoc -Destination $taskStage
}
$taskLicenses = Join-Path $taskStage 'THIRD_PARTY_LICENSES'
New-Item -ItemType Directory -Path $taskLicenses | Out-Null
$taskModules = @(go list -m -f '{{.Path}}|{{.Dir}}' all)
if ($LASTEXITCODE -ne 0) { throw 'Cannot list dependency licenses' }
foreach ($taskModule in $taskModules) {
    $taskParts = $taskModule -split '\|', 2
    if ($taskParts.Count -ne 2 -or -not $taskParts[1]) { continue }
    $taskModuleName = $taskParts[0] -replace '[^a-zA-Z0-9._-]', '_'
    foreach ($taskLicense in Get-ChildItem -LiteralPath $taskParts[1] -File | Where-Object { $_.Name -match '^(LICENSE|COPYING|NOTICE)' }) {
        Copy-Item -LiteralPath $taskLicense.FullName -Destination (Join-Path $taskLicenses "$taskModuleName-$($taskLicense.Name)")
    }
}
$taskGoRoot = go env GOROOT
if ($LASTEXITCODE -ne 0) { throw 'Cannot locate Go license' }
Copy-Item -LiteralPath (Join-Path $taskGoRoot 'LICENSE') -Destination (Join-Path $taskLicenses 'Go-LICENSE')
$taskArchive = Join-Path $taskOutput "$taskName.zip"
Compress-Archive -LiteralPath $taskStage -DestinationPath $taskArchive
$taskHash = (Get-FileHash -LiteralPath $taskArchive -Algorithm SHA256).Hash.ToLowerInvariant()
[IO.File]::WriteAllText((Join-Path $taskOutput 'SHA256SUMS.txt'), "$taskHash  $taskName.zip`n", (New-Object Text.UTF8Encoding($false)))
Write-Output "Release archive: $taskArchive"
Write-Output "SHA-256: $taskHash"
