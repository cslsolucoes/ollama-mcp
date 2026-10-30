#!/bin/bash
# 10.7 - testes Linux pendentes, como mcpgw, numa pasta temporaria.
W=/var/tmp/aceite107
B0=8886b162-c7be-4322-8782-6235db648193
echo "== inicio $(date '+%F %T') boot=$(uptime -s) boot_id_ok=$([ "$(cat /proc/sys/kernel/random/boot_id)" = "$B0" ] && echo sim || echo NAO)"
install -d -m 0700 -o mcpgw -g mcpgw "$W/tests" "$W/tests/tmp"
install -m 0700 -o mcpgw -g mcpgw "$W/up/bin/pool.test" "$W/up/bin/content.test" "$W/up/bin/joblog.test" "$W/tests/"
cd "$W/tests" || exit 1
run() { # nome regex|"" extra...
  local n=$1 rx=$2; shift 2
  local args=(-test.count=1 -test.v)
  [ -n "$rx" ] && args+=("-test.run=$rx")
  runuser -u mcpgw -- env -i PATH=/usr/bin:/bin TMPDIR="$W/tests/tmp" HOME="$W/tests" "./$n.test" "${args[@]}" "$@"
}
for spec in \
  "pool|^(TestAdv5S4FileModesLinux|TestRegistryWrongModeIsRefused)$" \
  "content|^(TestAdvLPermissions|TestRegress10LExistingFileBecomesPrivate)$" \
  "joblog|^(TestAdvLKilledMidStream|TestAdvLKillChild)$"; do
  n=${spec%%|*}; rx=${spec#*|}
  run "$n" "$rx" > "$W/tests/$n-sel.log" 2>&1; rc=$?
  echo "== $n selecionados rc=$rc"
  grep -E "^(=== RUN|--- |PASS|FAIL|ok)|VEC\|" "$W/tests/$n-sel.log" | sed 's/^/   /'
done
for n in pool content joblog; do
  run "$n" "" -test.short > "$W/tests/$n-full.log" 2>&1; rc=$?
  echo "== $n suite completa (-short) rc=$rc: PASS=$(grep -c -- '^--- PASS' "$W/tests/$n-full.log") FAIL=$(grep -c -- '--- FAIL' "$W/tests/$n-full.log") SKIP=$(grep -c -- '--- SKIP' "$W/tests/$n-full.log")"
  grep -E -- "--- (FAIL|SKIP)" -A2 "$W/tests/$n-full.log" | sed 's/^/   /' | head -30
  tail -1 "$W/tests/$n-full.log" | sed 's/^/   /'
done
echo "== quem rodou: $(stat -c '%U %a' "$W/tests/tmp")"
echo "== fim $(date '+%F %T') boot=$(uptime -s)"
