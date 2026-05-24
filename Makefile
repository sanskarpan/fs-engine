.PHONY: build test race run dev lint format img docker-build docker-build-web perf perf-go perf-k6

GO_PACKAGES := $(shell go list ./... | grep -v '/web/node_modules/')

build:
	go build -o bin/server ./cmd/server

test:
	go test $(GO_PACKAGES) -v -count=1

race:
	go test $(GO_PACKAGES) -race -count=1

run:
	go run ./cmd/server

dev:
	@echo "Starting backend and frontend..."
	go run ./cmd/server &
	cd web && npm run dev

lint:
	go vet $(GO_PACKAGES)

format:
	gofmt -w .
	cd web && npx prettier --write "src/**/*.{ts,tsx}"

img:
	dd if=/dev/zero of=testdata/disk.img bs=1M count=64 2>/dev/null
	@echo "Created testdata/disk.img"

docker-build:
	docker build -t fs-engine/backend:local .

docker-build-web:
	docker build -t fs-engine/frontend:local ./web

perf: perf-go

perf-go:
	go test ./internal/... -run '^$$' -bench . -benchmem -count=1

perf-k6:
	k6 run perf/k6/api-smoke.js
