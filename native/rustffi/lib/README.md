# Mutant Rust Anti-Tamper Static Library

This crate builds a static library consumed by Go cgo bindings in
native/rustffi.

## Exposed C ABI

- mutant_rust_probe(const char* request) -> char*
- mutant_rust_free(char* ptr)

The request and response payloads are JSON strings matching the Go envelope
schema in native/rustffi/probe.go.

## Build

### Local host target

cargo build --release

Output library path example:

- Windows (MSVC): target/release/mutant_rust.lib
- Linux/macOS: target/release/libmutant_rust.a

### Cross target

cargo build --release --target <RUST_TARGET>

Examples:

- x86_64-pc-windows-msvc
- x86_64-unknown-linux-gnu
- aarch64-unknown-linux-gnu
- x86_64-apple-darwin
- aarch64-apple-darwin
