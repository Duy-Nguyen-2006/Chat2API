# Chat2API — common development tasks.
#
# Quick reference:
#   make build      compile chat2api into ./bin/
#   make test       run all unit tests (no race detector)
#   make race       run all unit tests with -race
#   make vet        go vet ./...
#   make lint       alias for vet (gofmt -l ./... is also run)
#   make run        build + run with current .env
#   make docker     build container image tagged chat2api:dev
#   make clean      remove ./bin/ and test caches
#   make tidy       go mod tidy

BINARY   := bin/chat2api
IMAGE    := chat2api:dev
PKGS     := ./...
GOFLAGS  := -trimpath -ldflags="-s -w"

.PHONY: all build test race vet lint fmt run docker clean tidy

all: vet test build

build:
	@mkdir -p bin
	go build $(GOFLAGS) -o $(BINARY) ./cmd/chat2api

test:
	go test -count=1 $(PKGS)

race:
	go test -race -count=1 $(PKGS)

vet:
	go vet $(PKGS)

lint: vet
	@test -z "$$(gofmt -l . 2>/dev/null | grep -v vendor/)" \
	  || { echo "gofmt found differences above"; exit 1; }

fmt:
	gofmt -w .

run: build
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi; ./$(BINARY)

docker:
	docker build -t $(IMAGE) .

tidy:
	go mod tidy

clean:
	rm -rf bin/
	go clean -testcache