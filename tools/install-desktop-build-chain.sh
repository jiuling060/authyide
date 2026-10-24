#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

# Update apt package lists
sudo apt-get update

# Install all system packages required by Tauri bundler
sudo apt-get install -y --no-install-recommends \
  libwebkit2gtk-4.1-dev libgtk-3-dev libayatana-appindicator3-dev \
  librsvg2-dev libxdo-dev libssl-dev \
  patchelf xdg-utils rpm \
  gstreamer1.0-plugins-base gstreamer1.0-plugins-good \
  gstreamer1.0-libav gstreamer1.0-alsa

# Install Rust toolchain via rustup if not already present
if ! command -v rustup &>/dev/null; then
  curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain stable
fi

# Ensure the rust-src component is installed for cross-compilation
source "$HOME/.cargo/env"
rustup component add rust-src --toolchain stable

# Install Go from go.dev if not already present
if ! command -v go &>/dev/null; then
  GO_VERSION=$(curl -s https://go.dev/VERSION?m=text | head -n 1)
  curl -L "https://go.dev/dl/${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tar.gz
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf /tmp/go.tar.gz
  export PATH="/usr/local/go/bin:${PATH}"
fi

# Verify all required commands are present
MISSING=()
for cmd in cargo rustc go patchelf rpm node; do
  if ! command -v "$cmd" &>/dev/null; then
    MISSING+=("$cmd")
  fi
done

# Exit with error if any required tool is missing
if [ ${#MISSING[@]} -gt 0 ]; then
  echo "ERROR: Missing required tools: ${MISSING[*]}" >&2
  exit 1
fi

# Print versions of all required tools
cargo --version
rustc --version
go version
patchelf --version
rpm --version
node --version
