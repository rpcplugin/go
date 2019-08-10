#!/bin/bash

# We do not run protoc under go:generate because we want to ensure that all
# dependencies of go:generate are "go get"-able for general dev environment
# usability.

set -eu

SOURCE="${BASH_SOURCE[0]}"
while [ -h "$SOURCE" ]; do SOURCE="$(readlink "$SOURCE")"; done
DIR="$(cd -P "$(dirname "$SOURCE")" && pwd)"

cd "$DIR"

protoc -I ./ countplugin1.proto --go_out=plugins=grpc:./
