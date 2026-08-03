#!/usr/bin/env bash
#
# One-shot provisioning for a fresh Oracle Cloud Ubuntu ARM instance.
#
#   curl -fsSL https://raw.githubusercontent.com/Vijay190899/fernweh/main/deploy/oracle-setup.sh | bash
#
# Idempotent: safe to re-run after a failure, and safe to re-run to redeploy.
#
# The step nobody expects is the firewall. Oracle's Ubuntu images ship with
# iptables rules that DROP everything except SSH, on top of the cloud-side
# Security List. Open one and not the other and the instance answers nothing,
# with no error to read. That has cost more people more hours than every other
# part of this setup combined, so it is handled here explicitly and verified at
# the end.
set -euo pipefail

REPO="${REPO:-https://github.com/Vijay190899/fernweh.git}"
DIR="${DIR:-$HOME/fernweh}"

say() { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
warn() { printf '\033[33m    ! %s\033[0m\n' "$*"; }

# ---------------------------------------------------------------- packages ---
say "Installing Docker"
if ! command -v docker >/dev/null 2>&1; then
	sudo apt-get update -qq
	sudo apt-get install -y -qq ca-certificates curl git
	sudo install -m 0755 -d /etc/apt/keyrings
	sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
	sudo chmod a+r /etc/apt/keyrings/docker.asc
	echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] \
https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" |
		sudo tee /etc/apt/sources.list.d/docker.list >/dev/null
	sudo apt-get update -qq
	sudo apt-get install -y -qq docker-ce docker-ce-cli containerd.io \
		docker-buildx-plugin docker-compose-plugin
	sudo usermod -aG docker "$USER"
else
	echo "    already installed: $(docker --version)"
fi

# ------------------------------------------------------------------- swap ---
# The Go build is the memory peak, not the running stack, which idles under
# 100 MB. On a 6 GB shape a build can still spike hard enough to have the OOM
# killer take out the compiler, which surfaces as a build that "just stops".
if [ ! -f /swapfile ]; then
	say "Adding 2G swap so the build cannot be OOM-killed"
	sudo fallocate -l 2G /swapfile
	sudo chmod 600 /swapfile
	sudo mkswap /swapfile >/dev/null
	sudo swapon /swapfile
	echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab >/dev/null
fi

# --------------------------------------------------------------- firewall ---
say "Opening 80 and 443 in the INSTANCE firewall"
warn "This is only half of it. The other half is the VCN Security List in the"
warn "Oracle console, which this script cannot touch. See docs/ORACLE.md."
if command -v netfilter-persistent >/dev/null 2>&1; then
	# Insert above the catch-all REJECT that Oracle's image ends the chain with.
	for port in 80 443; do
		if ! sudo iptables -C INPUT -p tcp --dport "$port" -j ACCEPT 2>/dev/null; then
			sudo iptables -I INPUT 6 -p tcp --dport "$port" -m conntrack --ctstate NEW -j ACCEPT
		fi
	done
	sudo netfilter-persistent save >/dev/null
	echo "    iptables rules saved"
elif command -v firewall-cmd >/dev/null 2>&1; then
	sudo firewall-cmd --permanent --add-port=80/tcp --quiet || true
	sudo firewall-cmd --permanent --add-port=443/tcp --quiet || true
	sudo firewall-cmd --reload --quiet || true
	echo "    firewalld rules saved"
else
	warn "No recognised firewall tool; check manually that 80 and 443 are open."
fi

# ------------------------------------------------------------------ source ---
say "Fetching the application"
if [ -d "$DIR/.git" ]; then
	git -C "$DIR" pull --ff-only
else
	git clone --depth 1 "$REPO" "$DIR"
fi
cd "$DIR"

# --------------------------------------------------------------- settings ---
IP="$(curl -fsS --max-time 10 https://api.ipify.org || true)"
[ -n "$IP" ] || IP="$(hostname -I | awk '{print $1}')"
DEFAULT_HOST="$(echo "$IP" | tr '.' '-').sslip.io"

if [ ! -f .env ]; then
	say "Writing .env"
	cp .env.example .env
	{
		echo ""
		echo "# --- added by deploy/oracle-setup.sh ---"
		echo "SITE_ADDRESS=${SITE_ADDRESS:-$DEFAULT_HOST}"
		echo "ACME_EMAIL=${ACME_EMAIL:-}"
		echo "JAEGER_UI_URL=https://${SITE_ADDRESS:-$DEFAULT_HOST}/jaeger"
	} >>.env
	warn "OPENROUTER_API_KEY is empty. The demo runs fully without it on its"
	warn "deterministic paths; add it to .env and re-run to enable the model."
else
	echo "    .env already exists, leaving it alone"
fi

# ------------------------------------------------------------------ deploy ---
say "Building and starting the stack (first build takes a few minutes)"
sudo docker compose \
	-f docker-compose.yml \
	-f deploy/compose.prod.yml \
	-f deploy/compose.caddy.yml \
	up -d --build

# ------------------------------------------------------------------ verify ---
say "Verifying"
host="$(grep -E '^SITE_ADDRESS=' .env | cut -d= -f2-)"
for i in $(seq 1 30); do
	if curl -fsS --max-time 5 http://localhost/healthz >/dev/null 2>&1; then break; fi
	sleep 4
done

printf '\n'
if curl -fsS --max-time 5 http://localhost/healthz >/dev/null 2>&1; then
	echo "    stack answers locally"
else
	warn "the stack is not answering locally yet; check: sudo docker compose logs"
fi

if curl -fsS --max-time 20 "https://$host/healthz" >/dev/null 2>&1; then
	printf '\n\033[32m    Live: https://%s\033[0m\n\n' "$host"
else
	warn "not reachable from outside yet."
	warn "Almost always the VCN Security List: add ingress rules for TCP 80"
	warn "and 443 from 0.0.0.0/0 in the Oracle console, then retry in a minute."
	warn "Certificates also take up to a minute to issue on first boot."
	printf '\n    Expected URL once open: https://%s\n\n' "$host"
fi
