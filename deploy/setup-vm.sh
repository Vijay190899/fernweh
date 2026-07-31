#!/usr/bin/env bash
# One-shot setup for a fresh Ubuntu 24.04 VM (Oracle Always Free ARM or any
# small VPS). Run as a sudo-capable user:
#   curl -fsSL https://raw.githubusercontent.com/Vijay190899/fernweh/main/deploy/setup-vm.sh | bash
set -euo pipefail

if ! command -v docker >/dev/null; then
  curl -fsSL https://get.docker.com | sh
  sudo usermod -aG docker "$USER"
fi

sudo mkdir -p /opt && cd /opt
if [ ! -d fernweh ]; then
  sudo git clone https://github.com/Vijay190899/fernweh.git
  sudo chown -R "$USER" fernweh
fi
cd fernweh && git pull

if [ ! -f .env ]; then
  cp .env.example .env
  cat <<'EOF' >> .env

# --- production ---
DOMAIN=
JAEGER_BASIC_AUTH=
EOF
  echo ">>> Edit /opt/fernweh/.env: set OPENROUTER_API_KEY, DOMAIN, and"
  echo ">>> JAEGER_BASIC_AUTH (docker run --rm caddy:2-alpine caddy hash-password --plaintext 'yourpass')"
  echo ">>> then re-run this script."
  exit 0
fi

docker compose -f docker-compose.yml -f deploy/compose.prod.yml up -d --build
docker compose ps
echo "Fernweh is up. Point your DNS A record at this VM; Caddy handles TLS."
