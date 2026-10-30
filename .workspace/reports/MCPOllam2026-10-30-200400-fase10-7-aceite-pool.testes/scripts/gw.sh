#!/bin/bash
# 10.7 - gateway de teste (127.0.0.1:11599, mcpgw). start <A|B> | stop
W=/var/tmp/aceite107
U=mcpgw-aceite107
case "$1" in
start)
  R=$2; tipo=cpu; [ "$R" = B ] && tipo=remoto
  install -d -m 0700 -o mcpgw -g mcpgw "$W/gw$R"
  cat > "$W/gw$R/backends.json.tmp" <<EOF
{"backends":[
 {"id":"sec-cpu","url":"http://127.0.0.1:11435","tipo":"$tipo","slots":1,"modelos":["qwen3-ia"],"ordem":1},
 {"id":"morto","url":"http://127.0.0.1:11436","tipo":"cpu","slots":1,"ordem":2}
]}
EOF
  install -m 0600 -o mcpgw -g mcpgw "$W/gw$R/backends.json.tmp" "$W/gw$R/backends.json"; rm -f "$W/gw$R/backends.json.tmp"
  if [ ! -s "$W/pw" ]; then umask 077; head -c 18 /dev/urandom | base64 | tr -d '/+=' > "$W/pw"; fi
  tr -d '\n' < "$W/pw" | /usr/local/bin/mcpollama-gateway hash-password -user aceite > "$W/gw$R/admin.tmp" || exit 1
  install -m 0600 -o mcpgw -g mcpgw "$W/gw$R/admin.tmp" "$W/gw$R/admin"; rm -f "$W/gw$R/admin.tmp"
  systemctl reset-failed $U 2>/dev/null
  systemd-run --unit=$U --description="10.7 aceite: gateway de teste 11599" --uid=mcpgw --gid=mcpgw \
    -p NoNewPrivileges=yes -p MemoryMax=1G \
    /usr/local/bin/mcpollama-gateway --listen 127.0.0.1:11599 --ollama 127.0.0.1:11434 \
      --db "$W/gw$R/gw.db" --content-db "$W/gw$R/content.db" \
      --gen-slots 1 --embed-slots 1 --priority-ips 127.0.0.1 --allowed-models "qwen3-ia;nomic-embed-text" \
      --admin-file "$W/gw$R/admin" --start-stagger 300ms --backends "$W/gw$R/backends.json" \
      --overload-wait 3s --overload-queue 2 --overload-hold 10s --health-interval 5s \
      --breaker-failures 3 --breaker-open 20s --gpu-interval 0 --aer-path - --debug
  for i in $(seq 1 20); do curl -s -m 2 -o /dev/null 127.0.0.1:11599/monitor/api/state && break; sleep 0.5; done
  echo "gw$R: ativo=$(systemctl is-active $U) tipo_sec=$tipo"
  ls -l "$W/gw$R" | sed 's/^/   /'
  journalctl -u $U --no-pager -o cat | head -12 | sed 's/^/   /'
  ;;
stop)
  systemctl stop $U 2>/dev/null; systemctl reset-failed $U 2>/dev/null
  echo "gw: parado ($(systemctl is-active $U 2>&1))"
  ;;
esac
