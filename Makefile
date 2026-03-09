BIN     := paige
MODULE  := github.com/emtb/paige
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build run tui serve test lint clean install

## build: compile the binary to ./paige
build:
	go build $(LDFLAGS) -o $(BIN) ./cmd/paige

## run: build and open the TUI
run: build
	./$(BIN) tui

## serve: build and start the daemon
serve: build
	./$(BIN) serve

## tui: alias for run
tui: run

## test: run all tests
test:
	go test ./...

## lint: run golangci-lint (requires golangci-lint installed)
lint:
	golangci-lint run ./...

## clean: remove build artifacts
clean:
	rm -f $(BIN)
	rm -rf dist/

## install: install to GOPATH/bin
install:
	go install $(LDFLAGS) ./cmd/paige

## tidy: tidy go modules
tidy:
	go mod tidy

help: Makefile
	@echo ""
	@echo "Paige — cron-based AI job orchestrator for OpenCode"
	@echo ""
	@sed -n 's/^## //p' $< | column -t -s ':' | sed -e 's/^/ /'
	@echo ""
