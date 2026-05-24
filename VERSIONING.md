# Versioning and Releases

This repository follows semantic versioning:

- `MAJOR`: incompatible API or behavior changes
- `MINOR`: backward-compatible features
- `PATCH`: backward-compatible fixes and hardening

## Tag Format

- Release tags must use `vX.Y.Z`
- Example: `v1.4.2`

## Release Process

1. Ensure CI, security scans, and benchmarks are green.
2. Update release notes with user-visible and operational changes.
3. Create and push a semantic version tag.
4. GitHub Actions builds server binaries, frontend bundle archives, and checksums.
5. Publish the GitHub release and attach artifacts.
