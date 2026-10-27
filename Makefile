GO ?= go
COVERAGE_FILE := coverage.out
COVERAGE_THRESHOLD ?= 80

.PHONY: all fmt vet lint test coverage check-coverage tidy

all: fmt vet lint test

fmt:
	gofmt -w .

vet:
	$(GO) vet ./...

lint:
	golangci-lint run

test:
	$(GO) test ./... -race

coverage:
	$(GO) test ./... -coverprofile=$(COVERAGE_FILE) -covermode=atomic
	$(GO) tool cover -func=$(COVERAGE_FILE) | tail -1

check-coverage: coverage
	@total=$$($(GO) tool cover -func=$(COVERAGE_FILE) | tail -1 | awk '{print $$3}' | tr -d '%'); \
	echo "total coverage: $$total%"; \
	awk -v total="$$total" -v min="$(COVERAGE_THRESHOLD)" 'BEGIN { if (total+0 < min+0) { printf "coverage %.1f%% below threshold %d%%\n", total, min; exit 1 } }'

tidy:
	$(GO) mod tidy
