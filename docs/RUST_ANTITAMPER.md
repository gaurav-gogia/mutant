# Rust Anti-Tamper Integration

This document describes the Rust static library integration used by Mutant
security builtins.

## Go Side

- Go package: native/rustffi
- Build tags for Rust-backed provider: cgo and mutant_rust
- Default behavior without tags/toolchain: stub provider returns unavailable

## Rust Side

- Crate location: native/rustffi/lib
- Crate type: staticlib
- Exported symbols:
  - mutant_rust_probe
  - mutant_rust_free

## Build Commands

### Host build

pwsh ./native/rustffi/build_rust.ps1

### Cross-target examples

pwsh ./native/rustffi/build_rust.ps1 -Target x86_64-pc-windows-msvc pwsh
./native/rustffi/build_rust.ps1 -Target x86_64-unknown-linux-gnu pwsh
./native/rustffi/build_rust.ps1 -Target aarch64-unknown-linux-gnu pwsh
./native/rustffi/build_rust.ps1 -Target x86_64-apple-darwin pwsh
./native/rustffi/build_rust.ps1 -Target aarch64-apple-darwin

## Enable Rust Probes at Runtime

Set:

- MUTANT_ENABLE_RUST_ANTITAMPER=1

When disabled or unavailable, builtins remain advisory and return
rust_enabled=false with empty rust_signals.

## Strict Release Preconditions

The release asset generator supports strict precheck env flags:

- MUTANT_REQUIRE_RUST_STATICLIB=1
- MUTANT_RUST_STATICLIB_PATH=<path to static library>
- MUTANT_RUST_RELEASE_REQUIRE_CGO=1

If enabled and prerequisites are missing, release asset generation fails fast.
