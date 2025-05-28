.PHONY: build install clean test

BINARY_NAME=ohmykube
VERSION=0.1.0
BUILD_TIME=$(shell date +%FT%T%z)
GO_VERSION=$(shell go version | awk '{print $$3}')
GOPATH ?= $(shell go env GOPATH)

LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.GoVersion=${GO_VERSION}"

build:
	mkdir -p bin/
	go build ${LDFLAGS} -o bin/${BINARY_NAME} ./cmd/ohmykube

install:
	mkdir -p $(GOPATH)/bin/
	go build ${LDFLAGS} -o $(GOPATH)/bin/${BINARY_NAME} ./cmd/ohmykube

clean:
	go clean
	rm -f bin/${BINARY_NAME}
	rm -f ${BINARY_NAME}

test:
	go test ./...

lint:
	golangci-lint run

deps:
	go mod tidy
	go mod verify

.DEFAULT_GOAL := build
