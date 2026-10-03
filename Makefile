.PHONY: all build test clean run

BINARY_NAME=go-scraper
BINARY_PATH=bin/$(BINARY_NAME)

all: build test

build:
	go build -o $(BINARY_PATH) ./cmd/scraper

test:
	go test -v ./...

bench:
	go test -bench=. ./...

clean:
	rm -rf bin/ output/
