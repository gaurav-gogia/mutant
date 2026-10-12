#!/usr/bin/env sh
#
# Build the QUILLDROP evidence (Linux / macOS).
#
#   ./examples/workshop/evidence/build_evidence.sh                       everything
#   ./examples/workshop/evidence/build_evidence.sh --topic timestomp,ads  only these
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
# With no --topic, or --topic all, all four are built. The case tree already
# holds what ads and registry make, so after a case tree that built they are
# skipped; after one that did not they are built into case/ so dfrws/2a, 2b and
# 3 still have their input.
#
# Requires fsagen on PATH. No version is pinned: the script asks fsagen which
# features it has and stops, naming them, if one the playbooks use is missing.
#   go install github.com/aoiflux/fsagen@latest
#
# Mark-of-the-Web and the alternate data stream are NTFS features, so any volume
# without named streams needs --on-unsupported=skip, which is passed below for
# the case tree. Beat 3's ADS line then reports nothing. A second cost is less
# obvious and depends on the volume rather than on the operating system: where
# the file system has no birth time the modelled timeline comes back with every
# crtime at 0, and beat 6's "older than its own birth" finding cannot fire.
# Measured on WSL ext4, where it was 0 for all 135 rows; APFS keeps a birth time
# and is unmeasured, so this script does not guess -- it checks the timeline it
# just wrote and says so. Prefer NTFS, or take the pre-built corpus:
# evidence/README.md, under "Where the pre-built corpus comes from", says what
# it is and how to check it. The disk image is the same everywhere: fsagen
# writes its NTFS itself. The ads and registry topics need nothing from the
# volume and are the same everywhere too.

set -e

SEED=88412
HERE=$(cd "$(dirname "$0")" && pwd)
WORKSHOP=$(dirname "$HERE")
TOPICS="$HERE/topics"
PLAYBOOK="$HERE/quilldrop.playbook.yaml"
ROOT="$WORKSHOP/case"
BODYFILE="$WORKSHOP/case_quilldrop.body"
IMAGE="$WORKSHOP/case_quilldrop.img"
KNOWN="timestomp case ads registry"

TOPIC=all
while [ $# -gt 0 ]; do
    case "$1" in
        --topic) [ $# -ge 2 ] || { echo "--topic needs a list: all, case, timestomp, ads, registry"; exit 2; }
                 TOPIC="$2"; shift 2 ;;
        --topic=*) TOPIC="${1#--topic=}"; shift ;;
        -h|--help) sed -n '2,25p' "$0"; exit 0 ;;
        *) echo "build_evidence: unknown option $1"; exit 2 ;;
    esac
done

WANTED=""
for t in $(printf '%s' "$TOPIC" | tr ',' ' ' | tr '[:upper:]' '[:lower:]'); do
    case " all $KNOWN " in
        *" $t "*) ;;
        *) echo "unknown topic: $t"; echo "  topics: all, case, timestomp, ads, registry"; exit 2 ;;
    esac
    if [ "$t" = all ]; then WANTED="$KNOWN"; break; fi
    WANTED="$WANTED $t"
done
if [ -z "$WANTED" ]; then
    echo "--topic names nothing: all, case, timestomp, ads, registry"
    exit 2
fi
wants() {
    case " $WANTED " in *" $1 "*) return 0 ;; esac
    return 1
}

# What the playbooks and the dfrws/ programs rely on, by the names
# `fsagen features` lists.
NEEDED="
field:history.visits.redirect
format:chrome_cookies
format:regf
image.fs:ntfs
image.partition:gpt
image.ntfs.usn_journal
model.stomp_changes_ctime
playbook:subsecond_jitter
"

if ! command -v fsagen >/dev/null 2>&1; then
    echo "fsagen not found on PATH."
    echo "  go install github.com/aoiflux/fsagen@latest"
    exit 1
fi

# --json matters: a release without the features command reads a bare
# `fsagen features` as an output directory and writes a corpus into it.
if ! HAVE=$(fsagen features --json 2>/dev/null); then
    echo "This fsagen cannot list its features, so it is older than the playbooks."
    echo "  go install github.com/aoiflux/fsagen@latest"
    exit 1
fi
MISSING=""
for f in $NEEDED; do
    printf '%s\n' "$HAVE" | grep -qF "\"$f\"" || MISSING="$MISSING $f"
done
if [ -n "$MISSING" ]; then
    echo "This fsagen lacks what the playbooks use:"
    for f in $MISSING; do echo "  $f"; done
    echo "  go install github.com/aoiflux/fsagen@latest"
    exit 1
fi
fsagen --version
echo "topics:$WANTED"

BUILT=""
CASE_RC=""

# The image first: nothing scans a file inside it, so an antivirus refusal of
# the case tree's PE cannot cost dfrws/1 its disk.
if wants timestomp; then
    echo "timestomp: generating disk image (seed $SEED)..."
    fsagen --seed "$SEED" --playbook "$TOPICS/timestomp.playbook.yaml" --clean \
           --image "$IMAGE" --image-fs ntfs --image-size 64M --partition gpt \
           --image-time 2026-03-12T06:00:00Z
    BUILT="$BUILT
  timestomp : $IMAGE"
fi

if wants case; then
    if [ -d "$ROOT" ]; then
        echo "removing previous corpus: $ROOT"
        rm -rf "$ROOT"
    fi
    # The records of earlier runs describe files that were just removed.
    rm -rf "$ROOT.fsagen" "$WORKSHOP/case.ads.fsagen" "$WORKSHOP/case.registry.fsagen"
    echo "case: generating the case tree (seed $SEED)..."
    # --on-unsupported=skip is a no-op on NTFS, where nothing here is
    # unsupported. Elsewhere it is the difference between a corpus and no corpus
    # at all; every skip is recorded in "$ROOT".fsagen/run-manifest.json.
    # The status is kept rather than left to set -e, so the topics below can
    # still fill in what dfrws needs.
    CASE_RC=0
    fsagen --seed "$SEED" --playbook "$PLAYBOOK" --on-unsupported=skip \
           --timeline "$BODYFILE" --timeline-format bodyfile --timeline-source modelled \
           "$ROOT" || CASE_RC=$?
    if [ "$CASE_RC" -ne 0 ]; then
        echo ""
        echo "The case tree is incomplete: fsagen stopped with status $CASE_RC."
    else
        BUILT="$BUILT
  case      : $ROOT"
    fi
fi

# ads and registry write into case/ at the paths the case tree has them, so
# the dfrws programs read them unchanged. --into-existing merges, overwriting
# the file if it is there; --meta keeps each run's records apart from the
# case's.
for t in ads registry; do
    wants "$t" || continue
    if [ "$CASE_RC" = 0 ]; then
        echo "$t: the case tree just built holds it already"
        continue
    fi
    echo "$t: generating into $ROOT (seed $SEED)..."
    fsagen --seed "$SEED" --playbook "$TOPICS/$t.playbook.yaml" \
           --into-existing --meta "$WORKSHOP/case.$t.fsagen" "$ROOT"
    BUILT="$BUILT
$(printf '  %-9s : %s' "$t" "$ROOT")"
done

echo ""
echo "evidence ready$BUILT"

if [ "$CASE_RC" = 0 ]; then
    echo "  files     : $(find "$ROOT" -type f | wc -l)"
    echo "  bodyfile  : $BODYFILE ($(wc -l < "$BODYFILE") rows)"
    # Beat 6 asks whether any file's mtime precedes its crtime. Ask the timeline
    # the same question: if nothing answers yes, the beat will find nothing and
    # the attendee should be told now rather than wondering later. This tests
    # the output rather than guessing from the platform, so it is right on ReFS,
    # on FAT32 and on a network share too.
    if [ "$(awk -F'|' '$9 < $11' "$BODYFILE" | wc -l)" -eq 0 ]; then
        echo ""
        echo "NOTE: no row in this timeline has a modification time earlier than its"
        echo "      creation time, so beat 6's \"older than its own birth\" finding"
        echo "      will not fire here. That finding needs crtime, which is NTFS."
        echo "      The pre-built corpus keeps it: evidence/README.md, under"
        echo "      \"Where the pre-built corpus comes from\"."
    fi
    echo ""
    echo "Now run the first step:"
    echo "  ./examples/workshop/run.sh 01_scene"
else
    DIVES=""
    case "$BUILT" in *"timestomp :"*) DIVES="$DIVES 1_timestomp" ;; esac
    case "$BUILT" in *"ads       :"*) DIVES="$DIVES 2a_ad_redirects 2b_ad_cookies" ;; esac
    case "$BUILT" in *"registry  :"*) DIVES="$DIVES 3_run_keys" ;; esac
    if [ -n "$DIVES" ]; then
        echo ""
        echo "Now run the deep dives this built:"
        for d in $DIVES; do echo "  ./examples/workshop/run.sh dfrws/$d"; done
    fi
fi

if [ -n "$CASE_RC" ] && [ "$CASE_RC" -ne 0 ]; then
    exit "$CASE_RC"
fi
