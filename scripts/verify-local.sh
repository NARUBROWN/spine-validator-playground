#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
  printf 'Usage: bash scripts/verify-local.sh /path/to/spine\n' >&2
  exit 2
fi
spine_source=$(cd "$1" && pwd -P)
if [ ! -f "$spine_source/contract/validator/contract.go" ]; then
  printf 'The selected Spine source must include contract/validator.\n' >&2
  exit 2
fi
cd "$(dirname "$0")/.."
export GOWORK=off
verification_directory=$(mktemp -d "${TMPDIR:-/tmp}/spine-validator-verify.XXXXXX")
trap 'rm -rf "$verification_directory"' EXIT
verification_modfile="$verification_directory/local.mod"
cp go.mod "$verification_modfile"
if [ -f go.sum ]; then
  cp go.sum "$verification_directory/local.sum"
fi

# 로컬 소스 경로는 임시 모듈 파일에만 넣고 저장소의 go.mod와 go.sum은 바꾸지 않습니다.
go mod edit -modfile="$verification_modfile" -replace "github.com/NARUBROWN/spine=$spine_source"
go mod tidy -modfile="$verification_modfile"
go mod verify -modfile="$verification_modfile"
test -z "$(gofmt -l .)"
go vet -modfile="$verification_modfile" ./...
go test -modfile="$verification_modfile" -race ./... -count=1 -shuffle=on
