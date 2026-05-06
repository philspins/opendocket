SHELL := /bin/bash

# Configurable variables
DB ?= opendocket.db
ADDR ?= :8080
CRAWLER_BIN ?= opendocket-crawler
SERVER_BIN ?= opendocket-server
CRAWLER_FLAGS ?= $(FLAGS)

.PHONY: help build build-crawler build-server crawler server test templ clean eol-check eol-fix

help:
	@echo "Available targets:"
	@echo "  make build          Build crawler and server binaries"
	@echo "  make build-crawler  Build crawler binary"
	@echo "  make build-server   Build server binary"
	@echo "  make crawler        Run crawler against DB (DB=$(DB))"
	@echo "                      Examples:"
	@echo "                        make crawler CRAWLER_FLAGS='--provincial'"
	@echo "                        make crawler -- --provincial"
	@echo "  make server         Start web service (DB=$(DB), ADDR=$(ADDR))"
	@echo "  make test           Run all tests"
	@echo "  make eol-check      Fail if any tracked file contains CRLF line endings"
	@echo "  make eol-fix        Convert CRLF endings in tracked files to LF"
	@echo "  make templ          Regenerate templ files"
	@echo "  make clean          Remove built binaries"

build: eol-fix build-crawler build-server

build-crawler:
	go build -o $(CRAWLER_BIN) ./cmd/crawler

build-server:
	go build -o $(SERVER_BIN) ./cmd/server

crawler:
	go run ./cmd/crawler --db $(DB) $(CRAWLER_FLAGS) $(filter-out $@,$(MAKECMDGOALS))

ifneq (,$(filter crawler,$(MAKECMDGOALS)))
%:
	@:
endif

server:
	go run ./cmd/server -db $(DB) -addr $(ADDR)

test: eol-check
	go test -coverprofile=coverage.out ./...
	@grep -v "_templ\.go:" coverage.out > coverage_filtered.out
	@echo "=== Coverage excluding auto-generated *_templ.go files ==="
	@go tool cover -func=coverage_filtered.out | tail -1

eol-check:
	@cr="$$(printf '\r')"; \
	files="$$(git grep -I -z -l "$$cr" -- . | tr '\0' '\n' || true)"; \
	if [ -n "$$files" ]; then \
		echo "CRLF line endings detected in tracked files:"; \
		echo "$$files"; \
		echo "Run: make eol-fix or normalize files to LF before committing."; \
		exit 1; \
	fi; \
	echo "LF line-ending check passed."

eol-fix:
	@cr="$$(printf '\r')"; \
	git grep -I -z -l "$$cr" -- . | xargs -0 -r sed -i 's/\r$$//'
	@echo "Normalized tracked files to LF."

templ:
	go run github.com/a-h/templ/cmd/templ@v0.3.1001 generate

clean:
	rm -f $(CRAWLER_BIN) $(SERVER_BIN)

kill:
	pid=$(lsof -ti:8080) && [ -n "$pid" ] && kill "$pid"
