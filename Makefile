BINARY := hostapp
PKG := ./...

.PHONY: all build run test lint tidy clean

all: build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/$(BINARY) ./cmd/hostapp

run: build
	./bin/$(BINARY) serve

lint:
	golangci-lint run || true

test:
	go test -race $(PKG)

tidy:
	go mod tidy

clean:
	rm -rf bin
