GO ?= go
BIN := bin/aios
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 1.0.0)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/ArsinShaabani/AIOS/internal/version.Version=$(VERSION) \
	-X github.com/ArsinShaabani/AIOS/internal/version.Commit=$(COMMIT) \
	-X github.com/ArsinShaabani/AIOS/internal/version.Date=$(DATE)

.PHONY: help build run test race cover cover-gate lint fmt vet tidy docker release clean seed doctor migrate e2e load

help:
	@echo "AIOS targets:"
	@echo "  make build     - build bin/aios"
	@echo "  make run       - run the server (config.yaml or defaults)"
	@echo "  make test      - unit + e2e tests"
	@echo "  make race      - tests with the race detector"
	@echo "  make cover     - coverage report; make cover-gate enforces the threshold"
	@echo "  make lint      - gofmt/go vet checks"
	@echo "  make docker    - build the container image"
	@echo "  make release   - cross-compile linux amd64/arm64/armv7"

build:
	@mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/aios

run:
	$(GO) run ./cmd/aios server

test:
	$(GO) test ./internal/... ./tests/e2e/...

race:
	$(GO) test -race ./internal/... ./tests/e2e/...

cover:
	./scripts/coverage.sh coverage.out

cover-gate: cover
	./scripts/coverage-gate.sh

e2e:
	$(GO) test -count=1 -v ./tests/e2e/...

load:
	$(GO) test -tags load -count=1 -v ./tests/load/...

lint:
	@test -z "$$(gofmt -l internal cmd tests)" || (echo "gofmt needed:"; gofmt -l internal cmd tests; exit 1)
	$(GO) vet ./internal/... ./cmd/... ./tests/...

fmt:
	gofmt -w internal cmd tests

vet:
	$(GO) vet ./...

tidy:
	$(GO) mod tidy

docker:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg DATE=$(DATE) -t aios:$(VERSION) .

release:
	@mkdir -p dist
	@for target in linux/amd64 linux/arm64 linux/arm; do \
		os=$${target%/*}; arch=$${target#*/}; \
		suffix=$$arch; [ "$$arch" = "arm" ] && suffix=armv7; \
		echo "building $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch GOARM=7 $(GO) build -trimpath -ldflags "$(LDFLAGS)" \
			-o dist/aios-$(VERSION)-$$os-$$suffix ./cmd/aios || exit 1; \
	done
	cd dist && sha256sum aios-* > checksums.txt
	@echo "artifacts in dist/"

seed:
	$(GO) run ./cmd/aios seed

doctor:
	$(GO) run ./cmd/aios doctor

migrate:
	$(GO) run ./cmd/aios migrate

clean:
	rm -rf bin dist coverage.out
