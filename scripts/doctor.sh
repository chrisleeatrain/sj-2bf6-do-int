#!/usr/bin/env bash
# Checks that the tools for this exercise are installed. Run it before the interview.
set -uo pipefail

fail=0
ok()   { printf '  \033[32mok\033[0m    %-10s %s\n' "$1" "$2"; }
bad()  { printf '  \033[31mFAIL\033[0m  %-10s %s\n' "$1" "$2"; fail=1; }
warn() { printf '  \033[33mwarn\033[0m  %-10s %s\n' "$1" "$2"; }

check() {
  local name=$1; shift
  if ! command -v "$name" >/dev/null 2>&1; then
    bad "$name" "not installed"
    return
  fi
  ok "$name" "$("$@" 2>/dev/null | head -1)"
}

echo "Required:"
check docker docker --version
check kind kind version
check kubectl kubectl version --client
check helm helm version --short
check terraform terraform version
if command -v terraform >/dev/null 2>&1; then
  tfv=$(terraform version -json 2>/dev/null | sed -n 's/.*"terraform_version": *"\([^"]*\)".*/\1/p')
  if [[ -n "$tfv" ]] && [[ "$(printf '%s\n' 1.11.0 "$tfv" | sort -V | head -1)" != 1.11.0 ]]; then
    bad "terraform" "$tfv is too old; this exercise needs 1.11 or newer"
  fi
fi
check curl curl --version

if command -v docker >/dev/null 2>&1; then
  if docker info >/dev/null 2>&1; then
    ok "docker" "daemon is running"
  else
    bad "docker" "daemon is not running (start Docker Desktop, colima, etc.)"
  fi
fi

echo "Optional (handy for the scripting task):"
for t in python3 jq go; do
  if command -v "$t" >/dev/null 2>&1; then ok "$t" "$(command -v "$t")"; else warn "$t" "not installed"; fi
done

echo
if ((fail)); then
  echo "Some required tools are missing. Install them, or open this repo in GitHub Codespaces (see README)."
  exit 1
fi
echo "All set."
