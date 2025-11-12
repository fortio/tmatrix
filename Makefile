# Makefile for local tests, lint
# (release is goreleaser from shared workflows)

test:
	go test -race ./...

.golangci.yml: Makefile
	curl -fsS -o .golangci.yml https://raw.githubusercontent.com/fortio/workflows/main/golangci.yml

lint: .golangci.yml
	golangci-lint $(DEBUG_LINTERS) run $(LINT_PACKAGES)

coverage:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...

.PHONY: lint coverage test
