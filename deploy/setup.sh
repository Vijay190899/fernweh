#!/usr/bin/env bash
#
# One-shot provisioning for a fresh Ubuntu host: Oracle Cloud, Hetzner, or any
# other VM with a public address. ARM and x86 both work; nothing here or in the
# Dockerfile pins an architecture.
#
#   curl -fsSL https://raw.githubusercontent.com/Vijay190899/fernweh/main/deploy/setup.sh | bash
#
# Idempotent: safe to re-run after a failure, and safe to re-run to redeploy.
#
# The step nobody expects is the firewall, and it is host-specific. Oracle's
# Ubuntu images ship iptables rules that DROP everything except SSH, on top of
# the cloud-side Security List; open one and not the other and the instance
# answers nothing, with no error to read. Hetzner ships neither, so there is
# nothing to undo. Both cases are handled by detection rather than by asking
# which provider this is, because the wrong answer to that question is silent.
set -euo pipefail

REPO="${REPO:-https://github.com/Vijay190899/fernweh.git}"
DIR="${DIR:-$HOME/fernweh}"

# Hetzner hands you root; Oracle hands you an unprivileged ubuntu user. Resolve
# it once rather than sprinkling sudo through a script that then breaks on the
# minimal images where sudo is not installed.
if [ "$(id -u)" -eq 0 ]; then SUDO=""; else SUDO="sudo"; fi

# Which cloud this is changes what can be wrong, and therefore what the failure
# message should tell someone to go and look at. Read from DMI rather than
# asked, because a prompt in a piped-to-bash script cannot be answered.
read_dmi() { cat "/sys/class/dmi/id/$1" 2>/dev/null || true; }
case "$(read_dmi chassis_asset_tag)$(read_dmi sys_vendor)" in
*OracleCloud*) CLOUD="oracle" ;;
*Hetzner*) CLOUD="hetzner" ;;
*) CLOUD="unknown" ;;
esac

say() { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
warn() { printf '\033[33m    ! %s\033[0m\n' "$*"; }

# ---------------------------------------------------------------- packages ---
say "Installing Docker"
if ! command -v docker >/dev/null 2>&1; then
	$SUDO apt-get update -qq
	$SUDO apt-get install -y -qq ca-certificates curl git
	$SUDO install -m 0755 -d /etc/apt/keyrings
	$SUDO curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
	$SUDO chmod a+r /etc/apt/keyrings/docker.asc
	echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] \
https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" |
		$SUDO tee /etc/apt/sources.list.d/docker.list >/dev/null
	$SUDO apt-get update -qq
	$SUDO apt-get install -y -qq docker-ce docker-ce-cli containerd.io \
		docker-buildx-plugin docker-compose-plugin
	$SUDO usermod -aG docker "$USER"
else
	echo "    already installed: $(docker --version)"
fi

# ------------------------------------------------------------------- swap ---
# The Go build is the memory peak, not the running stack, which idles under
# 100 MB. On a 6 GB shape a build can still spike hard enough to have the OOM
# killer take out the compiler, which surfaces as a build that "just stops".
if [ ! -f /swapfile ]; then
	say "Adding 2G swap so the build cannot be OOM-killed"
	$SUDO fallocate -l 2G /swapfile
	$SUDO chmod 600 /swapfile
	$SUDO mkswap /swapfile >/dev/null
	$SUDO swapon /swapfile
	echo '/swapfile none swap sw 0 0' | $SUDO tee -a /etc/fstab >/dev/null
fi

# --------------------------------------------------------------- firewall ---
say "Opening 80 and 443 on the host"
if command -v netfilter-persistent >/dev/null 2>&1; then
	# -I at position 6, not -A. Oracle's chain ends in a catch-all REJECT, so an
	# appended rule sits below it and never matches, while iptables -L displays
	# a rule that reads perfectly correctly. That failure looks like a cloud
	# firewall problem and sends people to the wrong console for an hour.
	for port in 80 443; do
		if ! $SUDO iptables -C INPUT -p tcp --dport "$port" -j ACCEPT 2>/dev/null; then
			$SUDO iptables -I INPUT 6 -p tcp --dport "$port" -m conntrack --ctstate NEW -j ACCEPT
		fi
	done
	$SUDO netfilter-persistent save >/dev/null
	echo "    iptables rules saved"
elif command -v firewall-cmd >/dev/null 2>&1; then
	$SUDO firewall-cmd --permanent --add-port=80/tcp --quiet || true
	$SUDO firewall-cmd --permanent --add-port=443/tcp --quiet || true
	$SUDO firewall-cmd --reload --quiet || true
	echo "    firewalld rules saved"
else
	echo "    no host firewall to open"
fi

if [ "$CLOUD" = "oracle" ]; then
	warn "That was only the host half. Oracle also filters at the VCN Security"
	warn "List, which this script cannot reach. Both must allow 80 and 443."
	warn "See docs/ORACLE.md."
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
		echo "# --- added by deploy/setup.sh ---"
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
$SUDO docker compose \
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
	warn "the stack is not answering locally yet; check: docker compose logs"
fi

if curl -fsS --max-time 20 "https://$host/healthz" >/dev/null 2>&1; then
	printf '\n\033[32m    Live: https://%s\033[0m\n\n' "$host"
else
	warn "not reachable from outside yet."
	case "$CLOUD" in
	oracle)
		warn "Almost always the VCN Security List: add ingress rules for TCP 80"
		warn "and 443 from 0.0.0.0/0 in the Oracle console, then retry shortly."
		;;
	hetzner)
		warn "Hetzner leaves ports open unless a Cloud Firewall is attached to"
		warn "the server. If you added one, allow TCP 80 and 443 inbound."
		;;
	*)
		warn "Check whatever filters traffic in front of this host allows TCP"
		warn "80 and 443 inbound."
		;;
	esac
	warn "Certificates also take up to a minute to issue on first boot."
	printf '\n    Expected URL once open: https://%s\n\n' "$host"
fi
