#!/usr/bin/env bash
set -euo pipefail

KIND_VERSION=v0.33.0
arch=$(dpkg --print-architecture)
curl -fsSLo /tmp/kind "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-${arch}"
sudo install -m 0755 /tmp/kind /usr/local/bin/kind

sudo apt-get update -qq && sudo apt-get install -y -qq jq >/dev/null

make doctor
