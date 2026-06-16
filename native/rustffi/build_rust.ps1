Param(
    [string]$Target = "",
    [string]$BuildProfile = "release"
)

$ErrorActionPreference = "Stop"
$crateDir = Join-Path $PSScriptRoot "lib"

Push-Location $crateDir
try {
    if ([string]::IsNullOrWhiteSpace($Target)) {
        cargo build --profile $BuildProfile
    }
    else {
        cargo build --profile $BuildProfile --target $Target
    }
}
finally {
    Pop-Location
}
