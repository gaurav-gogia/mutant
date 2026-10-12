# Build the QUILLDROP evidence (Windows / PowerShell).
#
#   .\examples\workshop\evidence\build_evidence.ps1                       everything
#   .\examples\workshop\evidence\build_evidence.ps1 -Topic timestomp,ads  only these
#
# Run from the repository root. One seed and four playbooks, one per topic:
#
#   case       examples/workshop/case/ and case_quilldrop.body, the whole case
#              tree and its timeline; the ten beats read them.
#              Playbook: quilldrop.playbook.yaml
#   timestomp  examples/workshop/case_quilldrop.img, a GPT disk image laid out
#              as Windows lays out a system disk; dfrws/1 reads it.
#              Playbook: topics/timestomp.playbook.yaml
#   ads        the clerk's Chrome History (the redirect hops) and Cookies, in
#              case/; dfrws/2a and 2b read them.
#              Playbook: topics/ads.playbook.yaml
#   registry   the clerk's NTUSER.DAT (the Run key), in case/; dfrws/3 reads it.
#              Playbook: topics/registry.playbook.yaml
#
# With no -Topic, or -Topic all, all four are built. The case tree already holds
# what ads and registry make, so after a case tree that built they are skipped;
# after one that did not -- Defender refusing its generated PE is the usual
# reason -- they are built into case/ so dfrws/2a, 2b and 3 still have their
# input. Neither holds an executable, and the image is not scanned file by
# file, so -Topic timestomp,ads,registry builds on a machine with Defender on.
#
# Requires fsagen on PATH. No version is pinned: the script asks fsagen which
# features it has and stops, naming them, if one the playbooks use is missing.
#   go install github.com/aoiflux/fsagen@latest
#
# Prefer NTFS for the case tree: Mark-of-the-Web and the alternate data stream
# are NTFS features, and beat 3 reads both. Add a Defender exclusion for
# examples/workshop first, and keep it outside Windows Search indexing.

param(
    [string[]]$Topic = @('all')
)

$ErrorActionPreference = 'Stop'

$Seed     = 88412
$Here     = Split-Path -Parent $MyInvocation.MyCommand.Path
$Workshop = Split-Path -Parent $Here
$Topics   = Join-Path $Here 'topics'
$Playbook = Join-Path $Here 'quilldrop.playbook.yaml'
$Root     = Join-Path $Workshop 'case'
$Bodyfile = Join-Path $Workshop 'case_quilldrop.body'
$Image    = Join-Path $Workshop 'case_quilldrop.img'

# -Topic timestomp,ads arrives as two strings; "timestomp,ads" in quotes as one.
$known  = @('timestomp', 'case', 'ads', 'registry')
$wanted = @($Topic | ForEach-Object { $_ -split ',' } | ForEach-Object { $_.Trim().ToLowerInvariant() } |
    Where-Object { $_ })
if ($wanted -contains 'all') { $wanted = $known }
$unknown = @($wanted | Where-Object { $known -notcontains $_ })
if ($unknown.Count -gt 0 -or $wanted.Count -eq 0) {
    Write-Host "unknown topic: $($unknown -join ', ')" -ForegroundColor Red
    Write-Host "  topics: all, $($known -join ', ')"
    exit 2
}

# What the playbooks and the dfrws/ programs rely on, by the names
# `fsagen features` lists.
$Needed = @(
    'field:history.visits.redirect',  # dfrws/2a: the ad's redirect chain
    'format:chrome_cookies',          # dfrws/2b: the cookie jar
    'format:regf',                    # dfrws/3: the hive with the Run key
    'image.fs:ntfs',
    'image.partition:gpt',            # dfrws/1: find the volume on the disk
    'image.ntfs.usn_journal',         # the journal records the real times
    'model.stomp_changes_ctime',      # a stomp leaves the third clock behind
    'playbook:subsecond_jitter'       # times off the whole second, as on a real disk
)

if (-not (Get-Command fsagen -ErrorAction SilentlyContinue)) {
    Write-Host 'fsagen not found on PATH.' -ForegroundColor Red
    Write-Host '  go install github.com/aoiflux/fsagen@latest'
    exit 1
}

# --json matters: a release without the features command reads a bare
# `fsagen features` as an output directory and writes a corpus into it.
$features = $null
try { $features = (fsagen features --json 2>$null | Out-String) | ConvertFrom-Json } catch { }
if (-not $features) {
    Write-Host 'This fsagen cannot list its features, so it is older than the playbooks.' -ForegroundColor Red
    Write-Host '  go install github.com/aoiflux/fsagen@latest'
    exit 1
}
$missing = @($Needed | Where-Object { $features.features -notcontains $_ })
if ($missing.Count -gt 0) {
    Write-Host "fsagen $($features.version) lacks what the playbooks use:" -ForegroundColor Red
    $missing | ForEach-Object { Write-Host "  $_" }
    Write-Host '  go install github.com/aoiflux/fsagen@latest'
    exit 1
}
Write-Host "fsagen $($features.version), generator version $($features.generator_version)"
Write-Host "topics: $($wanted -join ', ')"

$built  = @()
$caseRc = $null

# The image first: nothing scans a file inside it, so an antivirus refusal of
# the case tree's PE cannot cost dfrws/1 its disk.
if ($wanted -contains 'timestomp') {
    Write-Host "timestomp: generating disk image (seed $Seed)..." -ForegroundColor Cyan
    fsagen --seed $Seed --playbook (Join-Path $Topics 'timestomp.playbook.yaml') --clean `
           --image $Image --image-fs ntfs --image-size 64M --partition gpt `
           --image-time 2026-03-12T06:00:00Z
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    $built += "timestomp : $Image"
}

if ($wanted -contains 'case') {
    if (Test-Path $Root) {
        Write-Host "removing previous corpus: $Root"
        Remove-Item -Recurse -Force $Root
    }
    # The records of earlier runs describe files that were just removed.
    foreach ($meta in @("$Root.fsagen", (Join-Path $Workshop 'case.ads.fsagen'), (Join-Path $Workshop 'case.registry.fsagen'))) {
        if (Test-Path $meta) { Remove-Item -Recurse -Force $meta }
    }
    Write-Host "case: generating the case tree (seed $Seed)..." -ForegroundColor Cyan
    # --on-unsupported=skip is a no-op on NTFS, where nothing here is unsupported.
    # On any other volume it is the difference between a corpus and no corpus at
    # all; every skip is recorded in $Root.fsagen/run-manifest.json.
    fsagen --seed $Seed --playbook $Playbook --on-unsupported=skip `
           --timeline $Bodyfile --timeline-format bodyfile --timeline-source modelled `
           $Root
    $caseRc = $LASTEXITCODE
    if ($caseRc -ne 0) {
        Write-Host ''
        Write-Host 'The case tree is incomplete. If the line above says the file "contains a' -ForegroundColor Yellow
        Write-Host '  virus", Defender refused a generated PE and fsagen stopped there.' -ForegroundColor Yellow
        Write-Host "  Exclude $Workshop in Defender and run this again." -ForegroundColor Yellow
    } else {
        $built += "case      : $Root"
    }
}

# ads and registry write into case/ at the paths the case tree has them, so the
# dfrws programs read them unchanged. --into-existing merges, overwriting the
# file if it is there; --meta keeps each run's records apart from the case's.
foreach ($t in @('ads', 'registry')) {
    if ($wanted -notcontains $t) { continue }
    if ($caseRc -eq 0) {
        Write-Host "${t}: the case tree just built holds it already"
        continue
    }
    Write-Host "${t}: generating into $Root (seed $Seed)..." -ForegroundColor Cyan
    fsagen --seed $Seed --playbook (Join-Path $Topics "$t.playbook.yaml") `
           --into-existing --meta (Join-Path $Workshop "case.$t.fsagen") $Root
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    $built += "{0,-9} : $Root" -f $t
}

Write-Host ''
Write-Host 'evidence ready' -ForegroundColor Green
$built | ForEach-Object { Write-Host "  $_" }

if ($caseRc -eq 0) {
    $files = Get-ChildItem -Recurse -File $Root
    $rows  = (Get-Content $Bodyfile).Count
    Write-Host "  files    : $($files.Count)"
    Write-Host "  bodyfile : $Bodyfile ($rows rows)"
    # Beat 6 asks whether any file's mtime precedes its crtime. Ask the timeline
    # the same question: NTFS answers yes, and a volume without crtime answers no
    # for every row, which would cost the beat its finding with no other sign.
    $backdated = @(Get-Content $Bodyfile | Where-Object {
        $f = $_ -split '\|'
        $f.Count -ge 11 -and [int64]$f[8] -lt [int64]$f[10]
    }).Count
    if ($backdated -eq 0) {
        Write-Host ''
        Write-Host 'NOTE: no row in this timeline has a modification time earlier' -ForegroundColor Yellow
        Write-Host "      than its creation time, so beat 6's 'older than its own" -ForegroundColor Yellow
        Write-Host "      birth' finding will not fire here. It needs crtime, which" -ForegroundColor Yellow
        Write-Host '      the volume has. The pre-built corpus keeps it:' -ForegroundColor Yellow
        Write-Host '      evidence/README.md, under "Where the pre-built' -ForegroundColor Yellow
        Write-Host '      corpus comes from".' -ForegroundColor Yellow
    }
    Write-Host ''
    Write-Host 'Now run the first step:' -ForegroundColor Yellow
    Write-Host '  .\examples\workshop\run.ps1 01_scene'
} else {
    $dives = @{ timestomp = @('1_timestomp'); ads = @('2a_ad_redirects', '2b_ad_cookies'); registry = @('3_run_keys') }
    $ready = @($built | ForEach-Object { ($_ -split ' ')[0] } | Where-Object { $dives.ContainsKey($_) })
    if ($ready.Count -gt 0) {
        Write-Host ''
        Write-Host 'Now run the deep dives this built:' -ForegroundColor Yellow
        $ready | ForEach-Object { $dives[$_] } | ForEach-Object { Write-Host "  .\examples\workshop\run.ps1 dfrws/$_" }
    }
}

if ($caseRc) { exit $caseRc }
