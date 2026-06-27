Param(
    [string]$BuildProfile = "release",
    [string]$OutputDir = "dist",
    [string]$AssetsOut = "releaseassets",
    [string]$FinalName = "mutant",
    [switch]$SkipRustTargetInstall,
    [switch]$HostOnly
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$rustBuildScript = Join-Path $repoRoot "native\rustffi\build_rust.ps1"

$targets = @(
    @{ GoOS = "windows"; GoArch = "amd64"; RustTarget = "x86_64-pc-windows-gnu"; ExeSuffix = ".exe" },
    @{ GoOS = "windows"; GoArch = "arm64"; RustTarget = "aarch64-pc-windows-gnullvm"; ExeSuffix = ".exe" },
    @{ GoOS = "linux"; GoArch = "amd64"; RustTarget = "x86_64-unknown-linux-gnu"; ExeSuffix = "" },
    @{ GoOS = "linux"; GoArch = "arm64"; RustTarget = "aarch64-unknown-linux-gnu"; ExeSuffix = "" },
    @{ GoOS = "darwin"; GoArch = "amd64"; RustTarget = "x86_64-apple-darwin"; ExeSuffix = "" },
    @{ GoOS = "darwin"; GoArch = "arm64"; RustTarget = "aarch64-apple-darwin"; ExeSuffix = "" }
)

$totalSteps = 5
$step = 0

function Show-Step {
    Param(
        [string]$Message,
        [string]$Status = "Running"
    )

    $percent = [int](($step / $totalSteps) * 100)
    Write-Progress -Activity "Mutant Full Build" -Status $Status -PercentComplete $percent -CurrentOperation $Message
    Write-Host "[$step/$totalSteps] $Message" -ForegroundColor Cyan
}

function Start-Step {
    Param([string]$Message)
    $script:step += 1
    Show-Step -Message $Message
}

function Invoke-Checked {
    Param(
        [string]$What,
        [scriptblock]$Command
    )

    & $Command
    if ($LASTEXITCODE -ne 0) {
        throw "$What failed with exit code $LASTEXITCODE"
    }
}

function Resolve-RustLib {
    Param(
        [string]$LibDir
    )

    $candidates = @(
        (Join-Path $LibDir "mutant_rust.lib"),
        (Join-Path $LibDir "libmutant_rust.a")
    )

    foreach ($candidate in $candidates) {
        if (Test-Path $candidate) {
            return $candidate
        }
    }

    throw "Rust static library not found under '$LibDir'. Expected mutant_rust.lib or libmutant_rust.a"
}

function Ensure-RustTargets {
    Param(
        [array]$TargetList
    )

    if ($SkipRustTargetInstall) {
        Write-Host "    Skipping rustup target auto-install (--SkipRustTargetInstall)." -ForegroundColor Yellow
        return
    }

    $rustup = Get-Command rustup -ErrorAction SilentlyContinue
    if (-not $rustup) {
        throw "rustup not found in PATH. Install rustup or re-run with -SkipRustTargetInstall"
    }

    $installed = & rustup target list --installed
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to query installed Rust targets via 'rustup target list --installed'"
    }

    foreach ($target in $TargetList) {
        if ($installed -contains $target.RustTarget) {
            Write-Host "    Rust target already installed: $($target.RustTarget)" -ForegroundColor DarkGray
            continue
        }

        Write-Host "    Installing Rust target: $($target.RustTarget)" -ForegroundColor Yellow
        & rustup target add $target.RustTarget
        if ($LASTEXITCODE -ne 0) {
            throw "Failed to install Rust target '$($target.RustTarget)'"
        }
    }
}

function Get-CrossCompilerCandidates {
    Param(
        [string]$GoOS,
        [string]$GoArch
    )

    if ($GoOS -eq "windows" -and $GoArch -eq "amd64") {
        return @("x86_64-w64-mingw32-gcc")
    }
    if ($GoOS -eq "windows" -and $GoArch -eq "arm64") {
        return @("aarch64-w64-mingw32-gcc")
    }
    if ($GoOS -eq "linux" -and $GoArch -eq "amd64") {
        return @("x86_64-linux-gnu-gcc")
    }
    if ($GoOS -eq "linux" -and $GoArch -eq "arm64") {
        return @("aarch64-linux-gnu-gcc")
    }
    if ($GoOS -eq "darwin" -and $GoArch -eq "amd64") {
        return @("o64-clang")
    }
    if ($GoOS -eq "darwin" -and $GoArch -eq "arm64") {
        return @("oa64-clang")
    }

    return @()
}

function Get-CrossCompilerHint {
    Param(
        [string]$GoOS,
        [string]$GoArch
    )

    if ($GoOS -eq "windows" -and $GoArch -eq "amd64") {
        return "x86_64-w64-mingw32-gcc or clang --target=x86_64-w64-windows-gnu"
    }
    if ($GoOS -eq "windows" -and $GoArch -eq "arm64") {
        return "aarch64-w64-mingw32-gcc or clang --target=aarch64-w64-windows-gnu"
    }
    if ($GoOS -eq "linux" -and $GoArch -eq "amd64") {
        return "x86_64-linux-gnu-gcc or clang --target=x86_64-unknown-linux-gnu"
    }
    if ($GoOS -eq "linux" -and $GoArch -eq "arm64") {
        return "aarch64-linux-gnu-gcc or clang --target=aarch64-unknown-linux-gnu"
    }
    if ($GoOS -eq "darwin" -and $GoArch -eq "amd64") {
        return "o64-clang (osxcross) or clang with an Apple SDK/sysroot"
    }
    if ($GoOS -eq "darwin" -and $GoArch -eq "arm64") {
        return "oa64-clang (osxcross) or clang with an Apple SDK/sysroot"
    }

    return "a target-appropriate cross C compiler"
}

function Assert-ReleaseAssetsDataClean {
    $dataDir = Join-Path $repoRoot "$AssetsOut\data"
    if (-not (Test-Path $dataDir)) {
        throw "Required assets data directory not found: $dataDir"
    }

    $entries = Get-ChildItem -Force $dataDir
    $placeholder = $entries | Where-Object { $_.Name -eq "placeholder.bin" }
    $unexpected = $entries | Where-Object { $_.Name -ne "placeholder.bin" }

    if ($unexpected.Count -gt 0) {
        foreach ($entry in $unexpected) {
            Remove-Item -Force -Recurse $entry.FullName
        }

        Write-Host "    Pruned $dataDir to placeholder.bin only." -ForegroundColor Yellow
    }

    if (-not $placeholder) {
        throw "Expected '$dataDir' to contain placeholder.bin before build actions, but it is missing."
    }
}

Assert-ReleaseAssetsDataClean

New-Item -ItemType Directory -Path (Join-Path $repoRoot $OutputDir) -Force | Out-Null

$exeSuffix = if ($IsWindows) { ".exe" } else { "" }
$bootstrapPath = Join-Path $repoRoot (Join-Path $OutputDir ("mutant-bootstrap" + $exeSuffix))

$rustLibDirByTarget = @{}

$hostInfo = & go env GOHOSTOS GOHOSTARCH
if ($LASTEXITCODE -ne 0 -or -not $hostInfo -or $hostInfo.Count -lt 2) {
    throw "Failed to detect Go host target via 'go env GOHOSTOS GOHOSTARCH'"
}
$goHostOS = $hostInfo[0].Trim()
$goHostArch = $hostInfo[1].Trim()

if ($HostOnly) {
    $targets = $targets | Where-Object { $_.GoOS -eq $goHostOS -and $_.GoArch -eq $goHostArch }
    if (-not $targets -or $targets.Count -eq 0) {
        throw "No host-matching target found for $goHostOS/$goHostArch"
    }
}

Push-Location $repoRoot
try {
    Start-Step "Ensure Rust targets are installed"
    Ensure-RustTargets -TargetList $targets

    Start-Step "Compile Rust static libraries (all targets)"
    foreach ($target in $targets) {
        $targetLabel = "$($target.GoOS)/$($target.GoArch)"
        Write-Host "    Rust => $targetLabel ($($target.RustTarget))" -ForegroundColor DarkGray

        Invoke-Checked -What "Rust build for $targetLabel" -Command {
            & $rustBuildScript -BuildProfile $BuildProfile -Target $target.RustTarget
        }

        $rustLibDir = Join-Path $repoRoot ("native\rustffi\lib\target\$($target.RustTarget)\$BuildProfile")
        $rustLibPath = Resolve-RustLib -LibDir $rustLibDir
        $rustLibDirByTarget[$targetLabel] = $rustLibDir
        Write-Host "      lib: $rustLibPath" -ForegroundColor DarkGray
    }

    Start-Step "Compile Go bootstrap binary"
    Invoke-Checked -What "Go bootstrap build" -Command {
        go build -o $bootstrapPath .
    }
    Write-Host "    Bootstrap binary: $bootstrapPath" -ForegroundColor DarkGray

    Start-Step "Generate embedded release assets"
    Invoke-Checked -What "Release asset generation" -Command {
        & $bootstrapPath gen --release-assets -out $AssetsOut
    }
    Write-Host "    Assets directory: $(Join-Path $repoRoot $AssetsOut)" -ForegroundColor DarkGray

    Start-Step "Recompile final Go binaries with Rust + release assets"
    $oldCGOEnabled = $env:CGO_ENABLED
    $oldCGOLdFlags = $env:CGO_LDFLAGS
    $oldGoos = $env:GOOS
    $oldGoarch = $env:GOARCH
    $oldCC = $env:CC

    try {
        $env:CGO_ENABLED = "1"

        foreach ($target in $targets) {
            $targetLabel = "$($target.GoOS)/$($target.GoArch)"
            $rustLibDir = $rustLibDirByTarget[$targetLabel]
            if (-not $rustLibDir) {
                throw "Missing Rust lib directory for target $targetLabel"
            }

            $isHostTarget = ($target.GoOS -eq $goHostOS -and $target.GoArch -eq $goHostArch)
            $ccVarName = ("MUTANT_CC_{0}_{1}" -f $target.GoOS, $target.GoArch).Replace("-", "_")
            $targetCC = [Environment]::GetEnvironmentVariable($ccVarName)

            if (-not $isHostTarget -and [string]::IsNullOrWhiteSpace($targetCC)) {
                foreach ($candidate in (Get-CrossCompilerCandidates -GoOS $target.GoOS -GoArch $target.GoArch)) {
                    $found = Get-Command $candidate -ErrorAction SilentlyContinue
                    if ($found) {
                        $targetCC = $found.Source
                        Write-Host "      auto-selected CC: $targetCC" -ForegroundColor DarkGray
                        break
                    }
                }
            }

            if (-not $isHostTarget -and [string]::IsNullOrWhiteSpace($targetCC)) {
                $hint = Get-CrossCompilerHint -GoOS $target.GoOS -GoArch $target.GoArch
                throw "Cross-CGO compiler not configured for $targetLabel. Set env '$ccVarName' to $hint, or use -HostOnly."
            }

            $env:GOOS = $target.GoOS
            $env:GOARCH = $target.GoArch
            if ($isHostTarget) {
                $env:CC = $oldCC
            }
            else {
                $env:CC = $targetCC
            }
            if ([string]::IsNullOrWhiteSpace($oldCGOLdFlags)) {
                $env:CGO_LDFLAGS = "-L$rustLibDir"
            }
            else {
                $env:CGO_LDFLAGS = "$oldCGOLdFlags -L$rustLibDir"
            }

            $targetName = "$FinalName-$($target.GoOS)-$($target.GoArch)$($target.ExeSuffix)"
            $finalPath = Join-Path $repoRoot (Join-Path $OutputDir $targetName)

            Write-Host "    Go => $targetLabel" -ForegroundColor DarkGray
            Invoke-Checked -What "Go final build for $targetLabel" -Command {
                go build -tags mutant_rust -o $finalPath .
            }
            Write-Host "      binary: $finalPath" -ForegroundColor DarkGray
        }
    }
    finally {
        $env:CGO_ENABLED = $oldCGOEnabled
        $env:CGO_LDFLAGS = $oldCGOLdFlags
        $env:GOOS = $oldGoos
        $env:GOARCH = $oldGoarch
        $env:CC = $oldCC
    }

    Write-Progress -Activity "Mutant Full Build" -Status "Done" -PercentComplete 100 -Completed
    Write-Host "Build complete." -ForegroundColor Green
    Write-Host "  Final binaries in: $(Join-Path $repoRoot $OutputDir)" -ForegroundColor Green
}
finally {
    Pop-Location
}
