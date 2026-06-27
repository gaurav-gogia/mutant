#!/usr/bin/env bash
set -euo pipefail

BUILD_PROFILE="release"
OUTPUT_DIR="dist"
ASSETS_OUT="releaseassets"
FINAL_NAME="mutant"
SKIP_RUST_TARGET_INSTALL=0
HOST_ONLY=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --build-profile)
      BUILD_PROFILE="$2"
      shift 2
      ;;
    --output-dir)
      OUTPUT_DIR="$2"
      shift 2
      ;;
    --assets-out)
      ASSETS_OUT="$2"
      shift 2
      ;;
    --final-name)
      FINAL_NAME="$2"
      shift 2
      ;;
    --skip-rust-target-install)
      SKIP_RUST_TARGET_INSTALL=1
      shift
      ;;
    --host-only)
      HOST_ONLY=1
      shift
      ;;
    -h|--help)
      cat <<'EOF'
Usage: ./scripts/build.sh [options]

Options:
  --build-profile <name>  Rust cargo profile (default: release)
  --output-dir <dir>      Output directory for binaries (default: dist)
  --assets-out <dir>      Release assets output directory (default: releaseassets)
  --final-name <name>     Final binary name (default: mutant)
  --skip-rust-target-install  Skip automatic rustup target install step
  --host-only             Build only GOHOSTOS/GOHOSTARCH target
EOF
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUST_CRATE_DIR="$REPO_ROOT/native/rustffi/lib"
OUTPUT_PATH="$REPO_ROOT/$OUTPUT_DIR"
BOOTSTRAP_BIN="$OUTPUT_PATH/mutant-bootstrap"

TARGETS=(
  "windows amd64 x86_64-pc-windows-gnu .exe"
  "windows arm64 aarch64-pc-windows-gnullvm .exe"
  "linux amd64 x86_64-unknown-linux-gnu ''"
  "linux arm64 aarch64-unknown-linux-gnu ''"
  "darwin amd64 x86_64-apple-darwin ''"
  "darwin arm64 aarch64-apple-darwin ''"
)

declare -A RUST_LIB_DIR_BY_TARGET

resolve_tool() {
  local tool_name="$1"
  local resolved

  if resolved="$(command -v "$tool_name" 2>/dev/null)"; then
    printf '%s\n' "$resolved"
    return 0
  fi

  if command -v cmd.exe >/dev/null 2>&1; then
    resolved="$(cmd.exe /c where "$tool_name" 2>/dev/null | tr -d '\r' | head -n 1 || true)"
    if [[ -n "$resolved" ]]; then
      printf '%s\n' "$resolved"
      return 0
    fi
  fi

  return 1
}

ps_quote() {
  local value="$1"
  value="${value//\'/\'\'}"
  printf "'%s'" "$value"
}

to_windows_path() {
  local value="$1"
  if [[ "$value" =~ ^/mnt/([a-zA-Z])/(.*)$ ]]; then
    local drive="${BASH_REMATCH[1]^^}"
    local rest="${BASH_REMATCH[2]//\//\\}"
    printf '%s:\%s\n' "$drive" "$rest"
    return 0
  fi

  printf '%s\n' "$value"
}

run_tool() {
  local tool_path="$1"
  shift

  if [[ "$tool_path" =~ ^[A-Za-z]:\\ || "$tool_path" =~ \.exe$ ]]; then
    if [[ -z "$POWERSHELL_BIN" ]]; then
      echo "PowerShell not found. Cannot execute Windows tool path: $tool_path" >&2
      return 1
    fi

    local ps_prefix=""
    local ps_command=""
    local env_name
    for env_name in CGO_ENABLED GOOS GOARCH CC CGO_LDFLAGS; do
      if [[ -n "${!env_name-}" ]]; then
        ps_prefix+="\$env:$env_name = $(ps_quote "${!env_name}"); "
      fi
    done

    ps_command="$ps_prefix& $(ps_quote "$(to_windows_path "$tool_path")")"
    for arg in "$@"; do
      ps_command+=" $(ps_quote "$(to_windows_path "$arg")")"
    done
    "$POWERSHELL_BIN" -NoProfile -Command "$ps_command"
    return $?
  fi

  "$tool_path" "$@"
}

GO_BIN="$(resolve_tool go || true)"
RUSTUP_BIN="$(resolve_tool rustup || true)"
CARGO_BIN="$(resolve_tool cargo || true)"
POWERSHELL_BIN="$(resolve_tool powershell.exe || true)"

if [[ -z "$POWERSHELL_BIN" ]]; then
  POWERSHELL_BIN="$(resolve_tool pwsh || true)"
fi

if [[ -z "$GO_BIN" ]]; then
  echo "go toolchain not found. Install Go or make sure the Go shim is visible to bash." >&2
  exit 1
fi

if [[ -z "$CARGO_BIN" ]]; then
  echo "cargo not found. Install Rust or make sure the Cargo shim is visible to bash." >&2
  exit 1
fi

if [[ -z "$RUSTUP_BIN" && "$SKIP_RUST_TARGET_INSTALL" -eq 0 ]]; then
  echo "rustup not found. Install Rustup or use --skip-rust-target-install." >&2
  exit 1
fi

GO_HOST_OS="$(run_tool "$GO_BIN" env GOHOSTOS)"
GO_HOST_ARCH="$(run_tool "$GO_BIN" env GOHOSTARCH)"

if [[ "$GO_HOST_OS" == "windows" ]]; then
  BOOTSTRAP_BIN+=".exe"
fi

if [[ "$HOST_ONLY" -eq 1 ]]; then
  FILTERED=()
  for target in "${TARGETS[@]}"; do
    read -r GOOS GOARCH _ _ <<<"$target"
    if [[ "$GOOS" == "$GO_HOST_OS" && "$GOARCH" == "$GO_HOST_ARCH" ]]; then
      FILTERED+=("$target")
    fi
  done

  if [[ "${#FILTERED[@]}" -eq 0 ]]; then
    echo "No host-matching target found for $GO_HOST_OS/$GO_HOST_ARCH" >&2
    exit 1
  fi

  TARGETS=("${FILTERED[@]}")
fi

GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
RESET='\033[0m'

TOTAL_STEPS=5
CURRENT_STEP=0

draw_progress() {
  local current="$1"
  local total="$2"
  local width=30
  local filled=$(( current * width / total ))
  local empty=$(( width - filled ))
  printf "${YELLOW}["
  printf "%0.s#" $(seq 1 "$filled")
  printf "%0.s-" $(seq 1 "$empty")
  printf "] %d/%d${RESET}\n" "$current" "$total"
}

run_step() {
  CURRENT_STEP=$((CURRENT_STEP + 1))
  local msg="$1"
  echo -e "${CYAN}[$CURRENT_STEP/$TOTAL_STEPS] $msg${RESET}"
  draw_progress "$CURRENT_STEP" "$TOTAL_STEPS"
}

resolve_rust_lib() {
  local candidates=(
    "$RUST_LIB_DIR/libmutant_rust.a"
    "$RUST_LIB_DIR/mutant_rust.lib"
  )
  for c in "${candidates[@]}"; do
    if [[ -f "$c" ]]; then
      echo "$c"
      return 0
    fi
  done
  return 1
}

ensure_rust_targets() {
  if [[ "$SKIP_RUST_TARGET_INSTALL" -eq 1 ]]; then
    echo "    Skipping rustup target auto-install (--skip-rust-target-install)."
    return 0
  fi

  if [[ -z "$RUSTUP_BIN" ]]; then
    echo "rustup not found. Install rustup or use --skip-rust-target-install" >&2
    return 1
  fi

  local installed
  installed="$(run_tool "$RUSTUP_BIN" target list --installed)"

  for target in "${TARGETS[@]}"; do
    read -r _ _ rust_target _ <<<"$target"
    if grep -qx "$rust_target" <<<"$installed"; then
      echo "    Rust target already installed: $rust_target"
      continue
    fi

    echo "    Installing Rust target: $rust_target"
    run_tool "$RUSTUP_BIN" target add "$rust_target"
  done
}

mkdir -p "$OUTPUT_PATH"

assert_releaseassets_data_clean() {
  local data_dir="$REPO_ROOT/$ASSETS_OUT/data"
  if [[ ! -d "$data_dir" ]]; then
    echo "Required assets data directory not found: $data_dir" >&2
    exit 1
  fi

  local entries
  mapfile -t entries < <(find "$data_dir" -mindepth 1 -maxdepth 1 -printf '%f\n' | sort)
  local has_placeholder=0
  local unexpected=()

  for entry in "${entries[@]}"; do
    if [[ "$entry" == "placeholder.bin" ]]; then
      has_placeholder=1
    else
      unexpected+=("$entry")
    fi
  done

  if [[ "${#unexpected[@]}" -gt 0 ]]; then
    for entry in "${unexpected[@]}"; do
      rm -rf -- "${data_dir:?}/$entry"
    done
    echo "    Pruned $data_dir to placeholder.bin only."
  fi

  if [[ "$has_placeholder" -ne 1 ]]; then
    echo "Expected '$data_dir' to contain placeholder.bin before build actions, but it is missing." >&2
    exit 1
  fi
}

cd "$REPO_ROOT"

run_step "Ensure Rust targets are installed"
ensure_rust_targets

assert_releaseassets_data_clean

run_step "Compile Rust static library"
for target in "${TARGETS[@]}"; do
  read -r GOOS GOARCH RUST_TARGET _ <<<"$target"
  TARGET_LABEL="$GOOS/$GOARCH"

    echo "    Rust => $TARGET_LABEL ($RUST_TARGET)"
  (
    cd "$RUST_CRATE_DIR"
      run_tool "$CARGO_BIN" build --profile "$BUILD_PROFILE" --target "$RUST_TARGET"
  )

  RUST_LIB_DIR="$RUST_CRATE_DIR/target/$RUST_TARGET/$BUILD_PROFILE"
  if ! RUST_LIB_PATH="$(RUST_LIB_DIR="$RUST_LIB_DIR" resolve_rust_lib)"; then
    echo "Rust static library not found in $RUST_LIB_DIR" >&2
    exit 1
  fi

  RUST_LIB_DIR_BY_TARGET["$TARGET_LABEL"]="$RUST_LIB_DIR"
  echo "      lib: $RUST_LIB_PATH"
done

run_step "Compile Go bootstrap binary"
run_tool "$GO_BIN" build -o "$BOOTSTRAP_BIN" .
echo "    Bootstrap binary: $BOOTSTRAP_BIN"

run_step "Generate embedded release assets"
run_tool "$BOOTSTRAP_BIN" gen --release-assets -out "$ASSETS_OUT"
echo "    Assets directory: $REPO_ROOT/$ASSETS_OUT"

run_step "Recompile final Go binaries with Rust + release assets"
OLD_CGO_ENABLED="${CGO_ENABLED-}"
OLD_CGO_LDFLAGS="${CGO_LDFLAGS-}"
OLD_GOOS="${GOOS-}"
OLD_GOARCH="${GOARCH-}"
OLD_CC="${CC-}"

export CGO_ENABLED=1

for target in "${TARGETS[@]}"; do
  read -r T_GOOS T_GOARCH _ T_EXE_SUFFIX <<<"$target"
  TARGET_LABEL="$T_GOOS/$T_GOARCH"
  TARGET_RUST_LIB_DIR="${RUST_LIB_DIR_BY_TARGET[$TARGET_LABEL]}"

  if [[ -z "$TARGET_RUST_LIB_DIR" ]]; then
    echo "Missing Rust lib dir for $TARGET_LABEL" >&2
    exit 1
  fi

  export GOOS="$T_GOOS"
  export GOARCH="$T_GOARCH"

  IS_HOST_TARGET=0
  if [[ "$T_GOOS" == "$GO_HOST_OS" && "$T_GOARCH" == "$GO_HOST_ARCH" ]]; then
    IS_HOST_TARGET=1
  fi

  CC_VAR_NAME="MUTANT_CC_${T_GOOS}_${T_GOARCH}"
  CC_VAR_NAME="${CC_VAR_NAME//-/_}"
  TARGET_CC="${!CC_VAR_NAME-}"

  if [[ "$IS_HOST_TARGET" -eq 0 && -z "$TARGET_CC" ]]; then
    echo "Cross-CGO compiler not configured for $TARGET_LABEL. Set env $CC_VAR_NAME or use --host-only" >&2
    exit 1
  fi

  if [[ "$IS_HOST_TARGET" -eq 1 ]]; then
    export CC="$OLD_CC"
  else
    export CC="$TARGET_CC"
  fi

  TARGET_RUST_LIB_DIR_LDFLAGS="$TARGET_RUST_LIB_DIR"
  if [[ "$T_GOOS" == "windows" ]]; then
    TARGET_RUST_LIB_DIR_LDFLAGS="$(to_windows_path "$TARGET_RUST_LIB_DIR")"
  fi

  if [[ -n "$OLD_CGO_LDFLAGS" ]]; then
    export CGO_LDFLAGS="$OLD_CGO_LDFLAGS -L$TARGET_RUST_LIB_DIR_LDFLAGS"
  else
    export CGO_LDFLAGS="-L$TARGET_RUST_LIB_DIR_LDFLAGS"
  fi

  FINAL_BIN="$OUTPUT_PATH/$FINAL_NAME-$T_GOOS-$T_GOARCH$T_EXE_SUFFIX"
  echo "    Go => $TARGET_LABEL"
  run_tool "$GO_BIN" build -tags mutant_rust -o "$FINAL_BIN" .
  echo "      binary: $FINAL_BIN"
done

export CGO_ENABLED="$OLD_CGO_ENABLED"
export CGO_LDFLAGS="$OLD_CGO_LDFLAGS"
export GOOS="$OLD_GOOS"
export GOARCH="$OLD_GOARCH"
export CC="$OLD_CC"

echo -e "${GREEN}Build complete.${RESET}"
echo -e "${GREEN}  Final binaries in: $OUTPUT_PATH${RESET}"
