#!/bin/bash
# Fase 9.6 (2026-10-30): define a senha do administrador do /monitor (usuario "admin").
# A senha e lida sem eco, nao vai para o historico nem para argumentos de comando;
# so o hash PBKDF2 e gravado em /etc/mcpollama-gateway/admin (root:mcpgw, 0640).
set -euo pipefail
umask 077
F=/etc/mcpollama-gateway/admin
[ "$(id -u)" -eq 0 ] || { echo "rode como root"; exit 1; }
[ -x /usr/local/bin/mcpollama-gateway ] || { echo "falta /usr/local/bin/mcpollama-gateway (etapa 1)"; exit 1; }
read -rs -p "Senha do admin do /monitor (12+ caracteres): " PW; echo
read -rs -p "Repita a senha: " PW2; echo
[ "$PW" = "$PW2" ] || { unset PW PW2; echo "as senhas nao conferem; nada foi gravado"; exit 1; }
unset PW2
[ ${#PW} -ge 12 ] || { unset PW; echo "senha curta (minimo 12); nada foi gravado"; exit 1; }
printf "%s\n" "$PW" | /usr/local/bin/mcpollama-gateway hash-password -user admin > "$F.new"
unset PW
test -s "$F.new"
chown root:mcpgw "$F.new"; chmod 0640 "$F.new"; mv -f "$F.new" "$F"; sync
echo "ok: $F gravado ($(wc -c < "$F") bytes, so o hash). Usuario: admin"
