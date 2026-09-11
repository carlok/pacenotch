GO ?= go
FUZZTIME ?= 10s

.PHONY: test cover cover-html fuzz golden reference build clean

## test: race-enabled unit tests
test:
	$(GO) test -race ./...

## cover: merged unit + end-to-end profile, per-package summary and coverage gate
cover:
	scripts/cover.sh

## cover-html: coverage.html from the merged profile
cover-html: cover
	$(GO) tool cover -html=coverage/merged.out -o coverage.html

## fuzz: short runs of every fuzz target (FUZZTIME=30s in CI)
fuzz:
	scripts/fuzz.sh $(FUZZTIME)

## golden: regenerate testdata/golden (review the diff!)
golden:
	$(GO) test ./internal/tui -run Golden -update

## reference: regenerate testdata/reference.sh from reference/claude-pace.sh
reference:
	scripts/patch-reference.sh

## build: CLI-only cross-compiled binaries in dist/
build:
	scripts/build-cli.sh

clean:
	rm -rf bin dist coverage coverage.out coverage.html
