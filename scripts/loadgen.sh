#!/usr/bin/env bash
# Sends a mix of good and bad requests to txn-api so the metrics have something in them.
# Usage: scripts/loadgen.sh [base_url] [seconds]
set -euo pipefail

BASE_URL="${1:-http://localhost:8080}"
DURATION="${2:-60}"
CURRENCIES=(USD USD USD EUR GBP USDC USDC JPY)

if ! curl -fsS -o /dev/null "$BASE_URL/healthz"; then
  echo "can't reach $BASE_URL/healthz (is the port-forward running?)" >&2
  exit 1
fi

echo "sending traffic to $BASE_URL for ${DURATION}s (ctrl-c to stop)"
end=$((SECONDS + DURATION))
ids=()
while ((SECONDS < end)); do
  cur=${CURRENCIES[RANDOM % ${#CURRENCIES[@]}]}
  amount=$((RANDOM % 50000 - 500))
  body=$(curl -sS -X POST "$BASE_URL/api/transactions" \
    -H 'Content-Type: application/json' \
    -d "{\"amount_cents\": $amount, \"currency\": \"$cur\"}" || true)
  id=$(sed -n 's/.*"id": *"\([^"]*\)".*/\1/p' <<<"$body")
  [[ -n "$id" ]] && ids+=("$id")

  # Look up an existing transaction now and then, and occasionally one that doesn't exist.
  if ((${#ids[@]} > 0 && RANDOM % 2 == 0)); then
    curl -sS -o /dev/null "$BASE_URL/api/transactions/${ids[RANDOM % ${#ids[@]}]}" || true
  fi
  if ((RANDOM % 10 == 0)); then
    curl -sS -o /dev/null "$BASE_URL/api/transactions/txn_doesnotexist" || true
  fi
done
echo "done"
