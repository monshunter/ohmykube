.PHONY: build clean test

BINARY_NAME=ohmykube
VERSION=0.1.0
BUILD_TIME=$(shell date +%FT%T%z)
GO_VERSION=$(shell go version | awk '{print $$3}')

LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.GoVersion=${GO_VERSION}"

build:
	go build ${LDFLAGS} -o ${BINARY_NAME} ./cmd/ohmykube

install: build
	mv ${BINARY_NAME} $(GOPATH)/bin/

clean:
	go clean
	rm -f ${BINARY_NAME}

test:
	go test ./...

lint:
	golangci-lint run

deps:
	go mod tidy
	go mod verify

.DEFAULT_GOAL := build
