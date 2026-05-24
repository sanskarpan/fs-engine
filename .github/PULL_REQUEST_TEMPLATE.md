## Summary

- What changed
- Why it changed
- Related issues

## Validation

- [ ] `go test ./... -timeout 8m`
- [ ] `go test ./api ./internal/fs ./internal/cache ./internal/disk -race -timeout 10m`
- [ ] `go test ./internal/integration ./internal/journal ./internal/ops ./internal/shell -race -timeout 10m`
- [ ] `cd web && npm ci && npm run build`
- [ ] `cd web && npm run test:e2e`

## Risk Review

- [ ] API contract reviewed
- [ ] Crash/recovery behavior reviewed
- [ ] Observability impact reviewed
- [ ] Security impact reviewed

## Release Notes

- User-visible impact
- Operational impact
