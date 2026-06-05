param(
    [string]$Version = "",
    [string]$InstallDir = ""
)

$ErrorActionPreference = "Stop"
$Repo = "iamwavecut/seasond"
$Binary = "modguard.exe"

if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    if (-not [string]::IsNullOrWhiteSpace($env:INSTALL_DIR)) {
        $InstallDir = $env:INSTALL_DIR
    }
}

if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $DefaultInstallDir = Join-Path $env:LOCALAPPDATA "Programs\seasond\bin"
    $Answer = Read-Host "Install directory [$DefaultInstallDir]"
    if ([string]::IsNullOrWhiteSpace($Answer)) {
        $InstallDir = $DefaultInstallDir
    } else {
        $InstallDir = $Answer
    }
}

if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = $env:VERSION
}

if ([string]::IsNullOrWhiteSpace($Version)) {
    $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
    $Version = $Release.tag_name
}

switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { $Arch = "x64" }
    "x86" { $Arch = "x86" }
    default { throw "Unsupported Windows architecture: $env:PROCESSOR_ARCHITECTURE" }
}

$Archive = "seasond_${Version}_windows_${Arch}.zip"
$Url = "https://github.com/$Repo/releases/download/$Version/$Archive"
$Temp = Join-Path ([System.IO.Path]::GetTempPath()) ("seasond-install-" + [System.Guid]::NewGuid())
New-Item -ItemType Directory -Path $Temp | Out-Null

try {
    $ArchivePath = Join-Path $Temp $Archive
    Invoke-WebRequest -Uri $Url -OutFile $ArchivePath
    Expand-Archive -Path $ArchivePath -DestinationPath $Temp -Force

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -Path (Join-Path $Temp $Binary) -Destination (Join-Path $InstallDir $Binary) -Force

    Write-Host "Installed modguard $Version to $(Join-Path $InstallDir $Binary)"
    $PathEntries = $env:PATH -split ";"
    if ($PathEntries -notcontains $InstallDir) {
        Write-Host "Note: $InstallDir is not in PATH."
    }
}
finally {
    Remove-Item -Path $Temp -Recurse -Force -ErrorAction SilentlyContinue
}
