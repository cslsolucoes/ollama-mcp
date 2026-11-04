#!/bin/bash
# Fase 9.6 - etapa 1 (preparo, sem mudar o trafego): usuario, diretorios e binario.
# Uso: bash /root/deploy96/stage1.sh <sha256 do arquivo SHA256SUMS, calculado no PC de origem>
# Seguranca (revisao 2026-10-30): pasta so do root (0700); manifesto SHA-256 de TODOS os
# arquivos, ancorado no hash passado como argumento; o binario e conferido de novo ja
# copiado (.new), antes do mv, para nao haver troca entre a conferencia e a instalacao.
set -euo pipefail
EXP_SUMS="$1"; D=/root/deploy96; B=/root/srv-backup-20261028/fase96-$(date +%Y%m%d-%H%M%S)
[ "$(id -u)" -eq 0 ] || { echo "rode como root"; exit 1; }
[ "$(stat -c '%U %a' "$D")" = "root 700" ] || { echo "$D precisa ser root 0700"; exit 1; }
for f in "$D"/*; do [ "$(stat -c '%U' "$f")" = "root" ] || { echo "dono errado: $f"; exit 1; }; done
[ "$(sha256sum "$D/SHA256SUMS" | cut -d' ' -f1)" = "$EXP_SUMS" ] || { echo "SHA256SUMS nao confere"; exit 1; }
(cd "$D" && sha256sum --quiet -c SHA256SUMS) || { echo "algum arquivo nao confere com o SHA256SUMS"; exit 1; }
EXP_BIN=$(awk '$2=="mcpollama-gateway"{print $1}' "$D/SHA256SUMS")
echo "boot: $(uptime -s)"
mkdir -p "$B"
id mcpgw >/dev/null 2>&1 || useradd --system --no-create-home --home-dir /var/lib/mcpollama-gateway --shell /sbin/nologin mcpgw
install -d -m 0750 -o mcpgw -g mcpgw /var/lib/mcpollama-gateway
install -d -m 0750 -o root -g mcpgw /etc/mcpollama-gateway

# Fase 12 (C-4.3): chave HMAC dos dados de treino. Gerada UMA vez; NUNCA sobrescrita; NUNCA impressa (so o key_id).
# 32 bytes aleatorios em hex (64 caracteres). O gateway roda como mcpgw e o hmackey.Load recusa qualquer permissao de
# grupo/outros, entao o arquivo e mcpgw:mcpgw 0600 (nao root:mcpgw 0640). Fica FORA do backup comum (C-6.8): copie-a
# so por canal protegido (a mesma chave vai para o gateway de fallback, C-4.3a). Trocar a chave quebra os grupos
# antigos (C-4.4): so com o owner.
KEY=/etc/mcpollama-gateway/doc.key
key_id() { python3 -c 'import sys,hashlib,binascii; print(hashlib.sha256(binascii.unhexlify(sys.stdin.read().strip())).hexdigest()[:16])' < "$KEY"; }
if [ -e "$KEY" ]; then
  # existente: so conserta dono/modo; o conteudo nao e tocado (nem lido para a tela)
  chown mcpgw:mcpgw "$KEY"; chmod 0600 "$KEY"
  echo "doc.key: ja existia (mantida)"
else
  TMPK=$(umask 077; mktemp /etc/mcpollama-gateway/.doc.key.XXXXXX)
  head -c 32 /dev/urandom | od -An -vtx1 | tr -d ' \n' > "$TMPK"
  chown mcpgw:mcpgw "$TMPK"; chmod 0600 "$TMPK"
  # ln falha se o destino ja existir: nunca sobrescreve (mesmo numa corrida)
  if ln "$TMPK" "$KEY" 2>/dev/null; then echo "doc.key: GERADA"; else echo "doc.key: ja existia (mantida)"; fi
  rm -f "$TMPK"
fi
[ "$(stat -c '%U:%G %a' "$KEY")" = "mcpgw:mcpgw 600" ] || { echo "doc.key com dono/modo errado"; exit 1; }
[ "$(wc -c < "$KEY" | tr -d ' ')" -ge 64 ] || { echo "doc.key curta demais (precisa de 64 caracteres hex); nada foi alterado"; exit 1; }
echo "doc.key key_id=$(key_id)  (confira que e o mesmo em todo gateway das mesmas estacoes)"

[ -e /usr/local/bin/mcpollama-gateway ] && cp -p /usr/local/bin/mcpollama-gateway "$B/" || true
install -m 0755 -o root -g root "$D/mcpollama-gateway" /usr/local/bin/mcpollama-gateway.new
[ "$(sha256sum /usr/local/bin/mcpollama-gateway.new | cut -d' ' -f1)" = "$EXP_BIN" ] || { rm -f /usr/local/bin/mcpollama-gateway.new; echo "binario copiado nao confere"; exit 1; }
mv -f /usr/local/bin/mcpollama-gateway.new /usr/local/bin/mcpollama-gateway
sync
/usr/local/bin/mcpollama-gateway --version
echo "backup em $B"
echo "ETAPA1_OK"
