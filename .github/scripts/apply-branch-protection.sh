#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <owner/repo> [branch]"
  exit 1
fi

repo="$1"
branch="${2:-main}"

gh api \
  --method PUT \
  -H "Accept: application/vnd.github+json" \
  "/repos/${repo}/branches/${branch}/protection" \
  -F required_status_checks.strict=true \
  -F enforce_admins=true \
  -F required_pull_request_reviews.dismiss_stale_reviews=true \
  -F required_pull_request_reviews.required_approving_review_count=1 \
  -F restrictions= \
  -f required_status_checks.contexts[]="Backend Test" \
  -f required_status_checks.contexts[]="Backend Race" \
  -f required_status_checks.contexts[]="Frontend Build" \
  -f required_status_checks.contexts[]="Browser E2E"
