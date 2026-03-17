#!/usr/bin/env bash

set -e
echo "" > coverage.txt

go get github.com/boumenot/gocover-cobertura
go test -race -coverprofile=coverage.txt -covermode=atomic $* ./...
go run github.com/boumenot/gocover-cobertura < coverage.txt > coverage.xml
go mod tidy
