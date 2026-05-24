# Security Policy

## Supported Scope

This repository expects security review for:

- the Go API and shell endpoints
- filesystem mutation, journaling, and recovery logic
- frontend API/WebSocket/SSE interactions
- deployment manifests and supply-chain dependencies

## Reporting

Do not file public GitHub issues for suspected vulnerabilities that could expose user data, filesystem integrity, or deployment credentials.

Instead:

1. Gather the minimal reproduction and affected commit/tag information.
2. Describe impact, exploitability, and any known mitigations.
3. Report privately through the repository security advisory flow or an agreed private channel.

## Triage Expectations

- Acknowledge within 3 business days.
- Confirm severity and next steps after reproduction.
- Prepare a fix, validation evidence, and release note before public disclosure.

## Minimum Security Gates

- `govulncheck` must pass on backend code.
- `npm audit --omit=dev --audit-level=moderate` must pass for production frontend dependencies.
- Branch protection should require the CI and Security workflows before merge.
