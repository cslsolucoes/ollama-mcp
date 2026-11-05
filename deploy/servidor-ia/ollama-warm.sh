#!/usr/bin/env bash
# Load qwen3.8:27b (96K context, 100% GPU) at boot so the first caller does not wait (64K until 2026-11-05 12:23).
# 2026-11-05 (owner): real model name, no alias (was qwen3-ia = qwen3.5:9b).
# 2026-10-26: was qwen3.8:27b-64k (removed - only the models tuned for this hardware remain).
set -euo pipefail

for _ in $(seq 1 60); do
    if curl -sf http://127.0.0.1:11434/api/version >/dev/null; then
        break
    fi
    sleep 2
done

curl -sf http://127.0.0.1:11434/api/generate \
    -H 'Content-Type: application/json' \
    -d '{"model":"qwen3.8:27b","prompt":"hi","stream":false,"think":false,"options":{"num_ctx":98304,"num_predict":1}}' \
    >/dev/null
