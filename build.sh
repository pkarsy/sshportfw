#!/bin/sh
# Build sshportfw as a statically linked binary.
#
# CGO_ENABLED=0 removes the dependency on the host glibc, so the resulting
# binary also runs on older distributions and on musl systems (Alpine).
# -trimpath keeps the build reproducible, -s -w drops the symbol table and
# debug information.
set -eu

cd "$(dirname "$0")"

CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o sshportfw .
