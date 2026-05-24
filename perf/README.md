# Performance Tooling

This repository includes two benchmark paths:

- Go benchmarks for core backend paths
- a `k6` scenario for end-to-end API pressure

## Go Benchmarks

```bash
go test ./internal/... -run '^$' -bench . -benchmem -count=1
```

## k6 API Scenario

Start the backend first:

```bash
go run ./cmd/server -addr 127.0.0.1:8080 -img ./testdata/perf.img -format
```

Run the scenario:

```bash
k6 run perf/k6/api-smoke.js
```

The default script exercises:

- file write
- file read
- stat
- directory list
