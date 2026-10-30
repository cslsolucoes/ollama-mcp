#!/usr/bin/env bash
# Rotate the bearer token used by the nginx Ollama proxy.
set -euo pipefail

if [[ ${EUID} -ne 0 ]]; then
    echo "run as root" >&2
    exit 1
fi

umask 077
NEW_KEY="$(openssl rand -hex 32)"

cat > /etc/nginx/ollama/auth.conf <<'EOF'
# Rotated by ollama-key-rotate
map $http_authorization $ollama_authorized {
    default 0;
    "~*^Bearer[[:space:]]+__KEY__$" 1;
}
EOF
sed -i "s/__KEY__/${NEW_KEY}/" /etc/nginx/ollama/auth.conf
chmod 600 /etc/nginx/ollama/auth.conf

printf '%s\n' "${NEW_KEY}" > /etc/nginx/ollama/api-key
chmod 600 /etc/nginx/ollama/api-key

nginx -t
systemctl reload nginx
# nginx reload is graceful; give the new workers a moment to take over.
sleep 2

echo "New API key: ${NEW_KEY}"
