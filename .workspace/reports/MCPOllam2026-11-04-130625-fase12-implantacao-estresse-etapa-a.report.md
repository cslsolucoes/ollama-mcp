# Report: Fase 12, implantação e estresse da etapa A (só o que roda hoje)

> **Data:** 2026-11-04, das 13:02 às 13:06. **Executor:** coordenador (Opus 5.5).
> **Escopo (owner, 12:59):** finalizar a Fase 12 em produção, com rollback garantido, e fazer o estresse em duas etapas que não se misturam:
> - **A:** só o que roda hoje: `qwen3-ia`, `--gen-slots 3`, gateway 0.13.0;
> - **B:** a Fase 13, com o gemma4, num report à parte.

## 1. Implantação

| Item | Antes | Depois |
| --- | --- | --- |
| gateway (servidor) | `0.12.6-a155513` | **`0.13.0-5372f4c`** |
| unit | flags | + `MCPGW_TREINO_DB`, `MCPGW_DOC_KEY_FILE`, `MCPGW_ROLE=primario` (variáveis: rollback à 0.12.6 continua subindo) |
| chave HMAC | — | `/etc/mcpollama-gateway/doc.key` gerada, `mcpgw:mcpgw` 0600, **`key_id=6db878cdc922021d`** |
| `treino.db` | — | `/var/lib/mcpollama-gateway/treino.db` (+ `-wal`/`-shm`), `mcpgw:mcpgw` 0600 |
| mcpollama (`D:\MCP`) | `0.9.2-f367b35` | **`0.10.0-5372f4c`** (vale nas sessões reiniciadas) |

**Ordem cumprida:** gateway antes do mcpollama (C-2.11).

**Backups para rollback:**
- `/root/srv-backup-20261028/fase12-pre-20261104-130246`: binário 0.12.6, unit e cópia do `gateway.db`;
- `.../fase96-20261104-130258`: binário anterior, feito pela etapa 1;
- `.../fase96-unit-20261104-130259`: unit anterior;
- `D:\MCP\mcpollama-mcp.exe.bak-0.9.2`.

**Conferências depois da implantação:**
- `active`;
- `/api/version`, `/monitor` e `/monitor/api/treino` responderam 200;
- `POST /api/feedback` com `{}` deu **400**;
- o log mostrou "chave HMAC ... carregada (key_id 6db878cdc922021d)" e "registro de erros ligado (treino.db, role primario)";
- slots `geracao 3` e `embed 2`;
- a pendência "provar 0600 no Linux" foi **fechada**.

**Remoção do `X-Mcpollama-Doc` antes do Ollama:** coberta pelos testes de unidade e de ponta a ponta da 12.3 e pelo vetor P3 da 12.9. Não houve prova ao vivo, porque o Ollama não registra cabeçalhos.

## 2. Estresse, etapa A

**Método:** `stress12.py` rodou no servidor contra `127.0.0.1:11500`, com a tool e o cliente marcados como `stress-fase12`.
- Degraus de 1, 3, 5, 10, 20 e 30 pedidos `/api/generate` simultâneos (`qwen3-ia`, `think:false`, `num_predict 48`).
- Depois, 20 `POST /api/feedback` válidos ao mesmo tempo, apontando para jobs do próprio teste.
- Amostra a cada 1 s do `/monitor/api/state` (latência e fila) e do `nvidia-smi` (watts por GPU).
- Havia tráfego real ao mesmo tempo (Cursor/Codex).

| Simultâneos | Parede | Códigos | Latência p50 | Latência máx | Tokens gerados |
| --- | --- | --- | --- | --- | --- |
| 1 | 0,7 s | 200×1 | 0,74 s | 0,74 s | 26 |
| 3 | 2,2 s | 200×3 | 1,38 s | 2,18 s | 88 |
| 5 | 4,6 s | 200×5 | 2,45 s | 4,59 s | 180 |
| 10 | 8,0 s | 200×10 | 4,39 s | 7,98 s | 304 |
| 20 | 17,1 s | 200×20 | 8,75 s | 17,11 s | 662 |
| 30 | 25,9 s | 200×30 | 13,29 s | 25,90 s | 989 |

**Resultados:**
- **69/69 pedidos com 200, 0 erros.**
- A parede cresce **linearmente** (~0,85 s por pedido curto). É a confirmação, sob a 0.13.0, de que o `qwen3-ia` gera **um por vez**, mesmo com `--gen-slots 3`/`NUM_PARALLEL=3`.
- **`/api/feedback`:** 20/20 com 200 e no máximo **30 ms**, cada um com `estado=nova` e `amostras_copiadas=1` (mesma origem 127.0.0.1, sem estação).
- **Monitor sob carga:** 74 amostras, latência p50 **4 ms** e máxima **16 ms**, 0 erros. Não houve travamento (padrão 0.12.6).
- **GPUs:** a soma máxima foi **202,6 W**, e o maior valor por placa foi 52,4 / 48,0 / 44,5 / 58,4 W. Uma placa por vez no pico, longe do limite de 100 W.

**Limpeza:**
- os 20 relatos de teste foram apagados do `treino.db` (`limpa12.py`, que só apaga se todas as ocorrências forem do teste; `secure_delete` e checkpoint TRUNCATE);
- o `treino.db` ficou vazio e as permissões continuaram 0600;
- os 69 jobs continuam no log normal (tool `stress-fase12`) e expiram em 7 dias.

## 3. O que falta para fechar a Fase 12

1. **Reiniciar as sessões** do Claude Code e do Cursor, para elas carregarem o mcpollama 0.10.0.
2. **Aceite ao vivo:** um relato real pela tool `reportar_erro` numa sessão reiniciada, a confirmação na aba "Erros (treino)" e uma exportação.
3. **12.11:** tirar o aviso "Instrução futura" do `docs/RELATAR-ERROS-IA.md` e aplicar a §9.1 do plano no `~\.claude\CLAUDE.md` global.

## 4. Etapa B (Fase 13)

Fica num report separado: gemma4 junto com o qwen3.5, medindo VRAM, energia e vazão, para responder as questões da §8 do plano da Fase 13. Nada da etapa B foi feito.
