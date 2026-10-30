# Plano — Fase 9: fila de espera na retaguarda (gateway) e monitor `/monitor` com SQLite

> **Estado:** **9.1–9.5 CONCLUÍDAS** (9.5, revisão adversarial, em 2026-10-29 00:15: 12 achados, 0 Alto, todos corrigidos; report `.workspace/reports/MCPOllam2026-10-28-235633-revisao-adversarial-fase9.report.md`). **Próximo: 9.6 (implantação, com autorização)**, que só vem depois da 8.9 da Fase 8.
> **Decisões do owner (AskUserQuestion, 22:43):**
> - **D9-1:** `/monitor` **só na LAN, ver sem login**. As **ações** (promover, despromover, remover, cancelar) **exigem login** do administrador.
> - **D9-2:** **Ollama só no loopback** (127.0.0.1); todo o tráfego passa pela fila.
> - **D9-6:** **desenvolver em paralelo** à 8.9 e **implantar depois** da escolha do modelo.
> - **D9-8:**
>   - **cancelar pedido em execução: SIM**;
>   - **o cliente pode pedir prioridade `alta`, mas só de IPs autorizados** (lista configurável). De outro IP, o pedido é **ignorado** e tratado como `normal`, sem erro;
>   - **um único administrador** (a opção de vários com nome não foi escolhida).
> - **D9-3, D9-4, D9-5, D9-7 e D9-9:** valores recomendados do §9 (64/16/10 min/embed 2; 90 d/7 d; `modernc.org/sqlite`; escalonamento de 300 ms; allowlist de modelos também no gateway).
> **Origem:** pedido do owner em 2026-10-28:
> - 22:38: "fazer um pool de espera em fila na retaguarda e gerar http:/<ip servidor ia>:8443/monitor para mostrar a fila, pode usar SQLite para banco de dados — faça o plano";
> - 22:39: "com possibilidade de promover ou remover ou despromover na fila".
>
> **Contexto que motiva a fase:**
> - o critério do owner para a produção é **multiagentes trabalhando ao mesmo tempo, com qualidade impecável** (Fase 8, memória `criterio-modelo-multiagente`);
> - o nginx já aceita **12 conexões por IP** e 600 pedidos/min com rajada de 60, desde 22:36;
> - o Ollama, porém, tem fila **interna e invisível** (`OLLAMA_MAX_QUEUE=8`): passando disso, responde 503, e ninguém vê quem espera nem quanto;
> - o servidor tem a **fonte de 850 W no limite** (memória `servidor-ia-reboots-causa-fonte`), e uma fila que **controla quantos pedidos começam ao mesmo tempo** também atenua os picos de energia.

---

## 1. Finalidade

1. **Fila de espera na retaguarda (pool):**
   - quando todos os slots do Ollama estão ocupados, o pedido **espera numa fila controlada**, em vez de receber 429/503;
   - quem espera recebe a vaga na ordem justa;
   - o cliente (mcpollama, OpenClaude/`ia-local`) não precisa mudar para se beneficiar.
2. **Monitor web em `https://ia.cslsolucoes.com.br:8443/monitor`**, mostrando ao vivo:
   - quem está **em execução** (slot, cliente, tool, modelo, tempo decorrido);
   - quem está **na fila** (posição, prioridade, tempo de espera);
   - os **concluídos** recentes;
   - as estatísticas de espera e de vazão;
   - o **estado das 4 GPUs** (W, VRAM, temperatura, uso), o contador AER e o horário do boot.
3. **Ações do administrador na fila:**
   - **promover** (subir de prioridade ou de posição);
   - **despromover** (baixar);
   - **remover** (cancelar um pedido que espera, e o cliente recebe uma resposta clara).

   Cada ação fica **auditada** com quem fez, quando, o quê e o antes/depois.
4. **Histórico em SQLite:** cada pedido e cada ação viram um registro **só de metadados**, sem o texto dos documentos nem as respostas. Isso permite medir a espera e a vazão por dia, cliente e modelo, e cruzar com os reboots.

**Fora do escopo da v1:** balanceamento entre vários servidores; fila persistente que sobreviva a reboot (pedido em espera num reboot é perdido, porque o cliente já perdeu a conexão); cobrança ou cota por cliente.

## 2. Arquitetura

```
clientes (mcpollama em N agentes/estações, OpenClaude ia-local, curl)
   │ HTTPS :8443 (Bearer)                         LAN ou Cloudflare Tunnel
   ▼
nginx (TLS, Bearer, limit_conn 12, 403 nos endpoints administrativos)
   ├── /api/*            ──► gateway 127.0.0.1:11500 ──► Ollama 127.0.0.1:11434
   ├── /monitor          ──► gateway (HTML)                 (D9-2: Ollama só no loopback)
   └── /monitor/api/*    ──► gateway (JSON, SSE e ações; login próprio, D9-1)
                               │
                               ├── fila em memória (fonte da verdade do "agora")
                               ├── gravador assíncrono ──► SQLite (WAL) /var/lib/mcpollama-gateway/gateway.db
                               └── coletor de GPU (nvidia-smi a cada 5 s + AER de 00:1b.4 + boot id)
```

- **Novo binário Go:** `mcpollama-gateway` (Linux), no **mesmo módulo** `github.com/cslsoftwares/mcpollama`.
  - O código fica em `src/gateway/` (pacotes `queue`, `proxy`, `store`, `monitor`, `gpu`).
  - O entrypoint fica em `src/cmd/mcpollama-gateway/`.
  - Segue o molde CSL: flags com fallback para variável de ambiente, stdout/stderr para log, `-ldflags -X main.Version`.
  - O binário é gerado em `D:\MCP\mcpollama-gateway` e copiado para `/usr/local/bin/` no servidor.
- **Sem cgo:** SQLite por **`modernc.org/sqlite`**, driver Go puro (D9-5), coerente com a decisão da Fase 7 (go-pdfium em WASM, sem cgo).
- **Reaproveitamento (SSOT):**
  - o padrão store + manager + laço de limpeza com TTL de `referencias/MCPNas/truenas-mcp/tasks/` é o molde (só leitura, nada copiado);
  - os mapas de erro e de redação do token são os de `src/ollama/` e `src/redact/`;
  - o teste de carga é o da Fase 8 (`src/cmd/bakeoff`, suíte S7 e a 8.9).
- **O gateway confia no nginx:**
  - escuta **só em 127.0.0.1**;
  - o Bearer continua validado no nginx;
  - o IP do cliente vem de `X-Real-IP`, posto pelo nginx; qualquer `X-Real-IP` vindo de fora é sobrescrito.

## 3. Fila: semântica

| Item | Regra proposta |
| --- | --- |
| **Faixas (lanes)** | `geracao` (`/api/generate` e `/api/chat`) e `embed` (`/api/embed`), com slots independentes: um embed de ~1 s não espera atrás de uma extração de 40 s. Os outros endpoints de leitura (`/api/tags`, `/api/ps`, `/api/show`, `/api/version`) passam **direto**, sem fila |
| **Slots** | `geracao` = `OLLAMA_NUM_PARALLEL` (o valor sai da medição 8.9 da Fase 8, ver D9-3); `embed` = 2. Cada slot é liberado quando a resposta termina (fim do stream), em erro ou quando o cliente desconecta |
| **Prioridade** | 3 níveis: `alta`, `normal` (padrão) e `baixa`. O cliente pode pedir pelo header `X-Mcpollama-Priority`, limitado a `normal`/`baixa`: `alta` só vem do administrador (D9-8) |
| **Ordem** | Por prioridade e, dentro dela, **justiça por cliente**: rodízio entre os clientes, por IP + `X-Mcpollama-Client`. Um agente com 20 pedidos não trava os outros. Dentro de um mesmo cliente, FIFO |
| **Envelhecimento** | Um pedido `baixa` que espera mais que N min (padrão 5) sobe para `normal`, o que evita espera eterna |
| **Limites** | Fila total: 64 pedidos. Por cliente: 16. Espera máxima: 10 min, depois **504**. Fila cheia: **503 + `Retry-After`**. Os valores sugeridos vão para D9-3 |
| **Desconexão** | Se o cliente desiste enquanto espera (o contexto HTTP é cancelado), o pedido sai da fila e é registrado como `cancelado`, sem ocupar slot |
| **Escalonamento de partida** | Entre o início de dois pedidos, no mínimo X ms (padrão 300 ms, D9-7). Assim os 4 prefills não começam no mesmo milissegundo, o que **atenua os picos de energia** que derrubam a fonte. Hipótese a medir na 9.7 |
| **Ações do administrador** | Ver §4.3 |
| **Respostas ao cliente** | Headers `X-Request-Id`, `X-Queue-Ms` (quanto esperou) e `X-Queue-Position` (posição na entrada). O mcpollama passa a mostrar a espera real no `[uso] queue_ms` (9.4) |
| **Streaming** | Repasse transparente (`httputil.ReverseProxy`, `FlushInterval: -1`). Os contadores do Ollama (`prompt_eval_count`, `eval_count`, `load_duration`, `total_duration`) são lidos **só do último chunk** (`done:true`), e o texto não é guardado |
| **Modelos** | Opcional (D9-9): lista de modelos permitidos também no gateway, como 2ª barreira, além do `--allowed-models` do mcpollama |

## 4. Monitor `/monitor`

### 4.1 Página
- **Um único HTML embutido** no binário (`go:embed`), sem CDN e sem dependências externas, já que o servidor fica na LAN.
- Atualiza ao vivo por **SSE** (`/monitor/api/stream`), com polling de 2 s como reserva.
- Quadros:
  - **Cartões:** em execução / na fila / concluídos hoje / erros hoje / espera p50 e p95 (15 min) / tokens por segundo somados.
  - **Em execução:** slot, id, cliente, tool, modelo, prioridade, tempo decorrido, tokens já gerados (quando o stream informa).
  - **Fila:** posição, id, cliente, tool, modelo, prioridade, **esperando há**, com os botões **▲ promover**, **▼ despromover** e **✕ remover**.
  - **Concluídos recentes:** os últimos 50, com status, espera, execução e tokens.
  - **GPUs:** 4 cartões (W, VRAM, temperatura, uso), mais AER de `00:1b.4`, boot atual e uptime.
  - **Gráficos simples** (SVG inline) da última hora: tamanho da fila, vazão e espera.
  - **Auditoria:** as últimas ações do administrador.
- **Todo dado vindo do cliente é escapado** (nome do cliente, tool, modelo). Isso previne XSS a partir de um header malicioso.

### 4.2 API JSON (base para a página e para scripts)

| Rota | Método | Uso |
| --- | --- | --- |
| `/monitor/api/state` | GET | foto atual: slots, fila, GPUs |
| `/monitor/api/stream` | GET (SSE) | eventos ao vivo |
| `/monitor/api/history?desde=&ate=&cliente=&modelo=` | GET | histórico do SQLite, paginado |
| `/monitor/api/stats?janela=15m\|1h\|24h\|7d` | GET | p50/p95 de espera e execução, vazão, erros por classe |
| `/monitor/api/queue/{id}/promote` | POST | sobe um nível de prioridade; já em `alta`, vai ao topo da faixa |
| `/monitor/api/queue/{id}/demote` | POST | desce um nível; já em `baixa`, vai ao fim da faixa |
| `/monitor/api/queue/{id}/remove` | POST | tira da fila; o cliente recebe **409** com o motivo "removido pelo administrador" e o pedido fica `removido` |
| `/monitor/api/audit` | GET | ações do administrador |

### 4.3 Ações na fila (promover, despromover, remover)
- **Valem só para pedidos que estão esperando.** Um pedido em execução não é interrompido na v1. Cancelar em execução é opcional (D9-8): o Ollama não tem cancelamento por id, só fechando a conexão.
- **Atômicas sob o lock da fila:** se o pedido já saiu da fila (começou, terminou ou o cliente desistiu), a ação responde **409 "não está mais na fila"**.
- **Auditadas:** cada ação grava `audit(ts, usuario, ip, acao, request_id, antes, depois, motivo)`. O motivo é opcional e informado no botão.
- **Protegidas:**
  - exigem login de administrador (D9-1);
  - exigem token **CSRF**, porque o login é por cookie;
  - só aceitam `POST`;
  - a ação de **remover** pede confirmação na página.
- **Promover respeita a justiça:** o pedido sobe de nível, mas não fura um `alta` que já esperava antes dele.

## 5. SQLite: esquema (só metadados; WAL; um único gravador assíncrono)

```sql
requests(id TEXT PRIMARY KEY, client_ip TEXT, client_label TEXT, tool TEXT, endpoint TEXT, model TEXT,
         lane TEXT, priority_in TEXT, priority_final TEXT, bytes_in INTEGER,
         enqueued_at INTEGER, started_at INTEGER, finished_at INTEGER,       -- epoch ms
         wait_ms INTEGER, run_ms INTEGER,
         status TEXT,            -- ok | erro | cancelado | removido | rejeitado | expirado
         http_status INTEGER, error_class TEXT,
         prompt_eval_count INTEGER, eval_count INTEGER, load_ms INTEGER, boot_id TEXT);
audit(id INTEGER PRIMARY KEY, ts INTEGER, usuario TEXT, ip TEXT, acao TEXT, request_id TEXT,
      antes TEXT, depois TEXT, motivo TEXT);
gpu_samples(ts INTEGER, gpu INTEGER, bus_id TEXT, power_w REAL, mem_used_mib INTEGER,
            util_pct INTEGER, temp_c INTEGER, aer_corr_1b4 INTEGER, boot_id TEXT);
events(ts INTEGER, kind TEXT, detail TEXT);   -- partida/parada do gateway, mudança de config, boot novo detectado
```

- **Índices:** `requests(enqueued_at)`, `requests(client_label, enqueued_at)` e `gpu_samples(ts)`.
- **Retenção (D9-4):**
  - `requests`: 90 dias;
  - `gpu_samples`: 7 dias;
  - `audit` e `events`: sem expiração;
  - limpeza diária, com `VACUUM` semanal.
- **Banco** em `/var/lib/mcpollama-gateway/gateway.db`, dono `mcpgw:mcpgw`, permissão 0600.
- **O banco nunca fica no caminho quente:**
  - a fila vive em memória;
  - o gravador assíncrono usa canal com buffer e insere em lote;
  - se o disco travar, a fila segue funcionando e os registros perdidos são contados num evento.
- **Boot id** (`/proc/sys/kernel/random/boot_id`) em cada registro. Assim, os pedidos de cada boot ficam ligados a ele, e os **reboots aparecem no histórico**, cruzados com as amostras de GPU anteriores à queda.
- **Sigilo:**
  - nenhum prompt, documento, resposta ou token é gravado;
  - um teste planta uma sequência-sentinela no prompt e prova que ela não aparece nos bytes do arquivo `.db` nem do `-wal`.

## 6. Segurança (entra na revisão adversarial 9.5)
- O gateway só escuta em 127.0.0.1. Com a D9-2, o Ollama também passa a escutar só em 127.0.0.1, e ninguém da LAN contorna a fila pela porta 11434.
- O nginx continua com o Bearer e com o 403 de `/api/pull|push|create|delete|copy|blobs`. Por defesa em profundidade, o gateway também recusa essas rotas.
- **`/monitor`, conforme a D9-1:**
  - acesso restrito à LAN no nginx (`allow 10.100.0.0/16; deny all`);
  - login próprio do gateway: usuário e senha com hash bcrypt num arquivo 0600, cookie `HttpOnly; Secure; SameSite=Strict` e expiração de 8 h;
  - CSRF nas ações;
  - o login também tem limite de tentativas.
- **Headers do cliente** (`X-Mcpollama-Client`, `-Tool`, `-Priority`): validados por tamanho e por conjunto de caracteres, e escapados na página. Um `alta` vindo do cliente é rebaixado para `normal`.
- **Unit systemd endurecida:**
  - `User=mcpgw`, `NoNewPrivileges`, `ProtectSystem=strict`, `ReadWritePaths=/var/lib/mcpollama-gateway`, `PrivateTmp`, `MemoryMax`;
  - acesso ao `nvidia-smi` só de leitura (é preciso conferir o acesso a `/dev/nvidia*` com `ProtectSystem`);
  - leitura do AER em `/sys`.
- **Ataques a cobrir:**
  - smuggling, com cabeçalhos `Transfer-Encoding`/`Content-Length` duplicados (o nginx normaliza, mas se testa);
  - um cliente lotando a fila (o limite por cliente cobre);
  - rajada de SSE (limite de conexões no `/monitor/api/stream`);
  - crescimento do banco (a retenção cobre);
  - XSS pelo nome do cliente;
  - CSRF nas ações;
  - id de pedido adivinhável (usar aleatório de 128 bits).

## 7. Mudanças no mcpollama (9.4)
- Envia `X-Mcpollama-Client`, com padrão `<hostname>/<pid>` e flag `--client-label` para nomear o agente, e `X-Mcpollama-Tool` (nome da tool). Sem conteúdo.
- Lê `X-Queue-Ms` e põe o valor real em `queue_ms` na linha `[uso]`, que já existe.
- **503 com `Retry-After` (fila cheia):** a tool devolve um erro claro ("fila do servidor cheia, tente em N s") e faz **no máximo 1 nova tentativa** automática, se couber no `--timeout`. Cobre também o **409** de "removido pelo administrador", com mensagem clara e sem nova tentativa.
- **Sem mudança no `tools/list`** (o budget está no limite) nem no `TOOLS.md`.

## 8. Etapas

| Etapa | Conteúdo | Muda o servidor? | Modelo do agente |
| --- | --- | --- | --- |
| 9.0 | Decisões D9 com o owner; levantar o SSOT (truenas `tasks/`, `src/ollama` e `src/redact`) e conferir o `ollama.conf` do nginx e as permissões de `nvidia-smi` sob systemd | não (só leitura) | loop principal |
| 9.1 | `src/gateway/queue`: faixas, slots, prioridade, justiça, envelhecimento, limites, desconexão, escalonamento e ações. Testes de unidade + `-race` + fuzz das ações concorrentes | não | sonnet |
| 9.2 | `src/gateway/proxy`: repasse com streaming, leitura dos contadores do último chunk, headers de resposta, rotas diretas e bloqueadas. Testes com Ollama falso (`httptest`), incluindo desconexão no meio do stream | não | sonnet |
| 9.3 | `src/gateway/store` (SQLite com `modernc.org/sqlite`, WAL, gravador em lote, retenção e teste da sentinela) e `src/gateway/gpu` (coletor de `nvidia-smi`/AER/boot id, com binário falso nos testes) | não | sonnet |
| 9.4 | `src/gateway/monitor`: HTML embutido, API JSON, SSE, login, CSRF, auditoria e ações. Testes de rota, escape e CSRF. **Também** as mudanças do mcpollama (§7) e seus testes | não | sonnet |
| 9.5 | **Revisão adversarial** (§6) com vetores executáveis, correções, testes de regressão e fuzz de 10 min por alvo | não | **opus** |
| 9.6 | **Implantação** (com autorização):<br>- usuário `mcpgw`, `/var/lib`, binário, unit e arquivo de senha;<br>- nginx: `/api/` → 11500, `location /monitor` com allow/deny;<br>- Ollama em 127.0.0.1 (D9-2) e `NUM_PARALLEL`/`MAX_QUEUE` (D9-3).<br>Backups com `test -s`, gravação atômica, `nginx -t`. **Rollback ensaiado:** voltar o `/api/` para 11434 com 1 linha + reload | **sim — autorização** | loop principal |
| 9.7 | **Aceite ao vivo:**<br>- (a) 2 → 5 → 10 agentes reais pela fila (reaproveita a 8.9), com o monitor mostrando a fila;<br>- (b) promover, despromover e remover durante a carga, auditados;<br>- (c) prova por 2ª via: consulta `sqlite3` no servidor × monitor × `calls.jsonl` do cliente;<br>- (d) sentinela ausente no `.db`;<br>- (e) o gateway reiniciado no meio da carga não derruba o Ollama;<br>- (f) com e sem escalonamento de partida, medir o pico de W (hipótese da fonte). Para no 1º reboot | carga | **opus** (coordena) |
| 9.8 | Documentação: `CONEXAO-IA.md` (local + servidor, md5 igual), `README.md`, `CLAUDE.md` e report final. Commits por etapa, com gate verde e verificação em worktree | não | sonnet + loop principal |

**Gate de cada etapa de código:**
- `go build ./...`, `go vet ./...`, `gofmt -l` vazio, `go test -count=1 ./...`, `go test -race` (gcc de `deps\`);
- build Linux (`GOOS=linux`) do gateway;
- verificação do commit num worktree limpo, lição da Fase 7.

## 9. Decisões para o owner (AskUserQuestion, antes da 9.1)

- **D9-1, acesso ao `/monitor`:**
  - (a) **LAN + login do gateway** (recomendado);
  - (b) só LAN, sem login (mas as ações exigiriam login de qualquer jeito);
  - (c) também pela internet (Cloudflare) com login.

  Pelo IP, o navegador avisa sobre o certificado (Cloudflare Origin CA). O certo é usar o nome `ia.cslsolucoes.com.br:8443` e importar a CA no Windows das estações de administração.
- **D9-2, forçar todo o tráfego pela fila:** Ollama só em 127.0.0.1 (recomendado) ou manter `0.0.0.0:11434` (a LAN pode contornar a fila). O `ollama-warm` usa o loopback e continua funcionando, fora da fila.
- **D9-3, slots e limites:**
  - slots de `geracao` = o `NUM_PARALLEL` medido na 8.9 para o modelo escolhido;
  - fila de 64; 16 por cliente; espera máxima de 10 min;
  - `embed` = 2.
- **D9-4, retenção:** `requests` 90 dias, `gpu_samples` 7 dias, auditoria sem expiração.
- **D9-5, dependência nova:** `modernc.org/sqlite`, Go puro e sem cgo (recomendado). O `mattn/go-sqlite3` exige cgo e foi descartado.
- **D9-6, ordem:** desenvolver a 9.1–9.5 **em paralelo** ao teste de carga 8.9 da Fase 8 e implantar (9.6) **depois** da escolha do modelo (recomendado). A alternativa é implantar antes, para que a 8.9 já passe pela fila.
- **D9-7, escalonamento de partida:** ligar com 300 ms (recomendado) e medir na 9.7, com e sem.
- **D9-8, poderes do administrador:**
  - promover, despromover e remover pedidos em espera (pedido do owner);
  - além disso, **cancelar em execução** (opcional; fecha a conexão com o Ollama);
  - aceitar ou não `alta` vinda do cliente (recomendado: não aceitar, `alta` só pelo administrador);
  - usuários: um único administrador, ou vários com nome (o nome vai para a auditoria).
- **D9-9:** lista de modelos permitidos também no gateway (recomendado: sim, a mesma do mcpollama).

## 10. Riscos

| Risco | Mitigação |
| --- | --- |
| O gateway vira ponto único de falha | Unit com `Restart=always`; rollback de 1 linha no nginx; o `ollama-warm` independe dele |
| Latência extra | Proxy em loopback (< 1 ms); medida na 9.7 |
| Stream preso segura o slot | Slot liberado em erro ou desconexão; timeout de execução igual ao do Ollama; teste da 9.2 |
| Número de slots acima do que a VRAM comporta (Ollama em CPU ou OOM) | Slots = valor medido na 8.9; o monitor mostra `100% GPU`/VRAM e alerta se cair para CPU |
| Picos de energia com N alto (fonte de 850 W) | Escalonamento de partida + slots limitados + 100 W; teste da 9.7 com parada no 1º reboot |
| Vazamento de conteúdo jurídico no banco | Só metadados; teste da sentinela; revisão adversarial |
| Ações indevidas na fila | Login, CSRF, só POST, confirmação ao remover, auditoria |

## 11. Extensão: faixa CPU/RAM com roteamento por prioridade (pedido do owner, 2026-10-28 22:59)

> Pergunta do owner (22:57): "com a implementação da fila, podemos usar também a CPU e a RAM balanceando por prioridade o
> que vai para GPU/VRAM e o que vai para CPU/RAM?". Resposta: sim, por **pedido inteiro**. Owner (22:59): "implemente isso
> no plano, bem detalhado".
> **Ordem:** começa **depois** da 9.7 (a fila da GPU implantada e aceita). As medições exigem o servidor livre, depois da 8.9.
> Nada aqui muda o que já foi decidido para a GPU.

### 11.1 Por que funciona e quais são os limites
- **O servidor tem 93 GB de RAM** e CPU ociosa. A VRAM (32 GB) é o recurso escasso.
- **Uma instância do Ollama não escolhe CPU ou GPU por pedido.** Pedir o mesmo modelo com outro `num_gpu` faz o Ollama
  **recarregar** o modelo e tirar o de produção da VRAM, como aconteceu na 8.8. Então a faixa CPU é uma **segunda
  instância do Ollama, só de CPU**, em `127.0.0.1:11435`, com os próprios modelos residentes na RAM.
- **Um pedido não se divide nem migra durante a execução.** A escolha da faixa é feita uma vez, quando o pedido começa.
  Um pedido que ainda espera pode trocar de faixa (11.4, regra R8).
- **Prefill na CPU é ordens de grandeza mais lento.** Estimativa a medir: ~50–300 tok/s, contra ~5.000 na GPU.
  Resumos e extrações de documentos são quase todo prefill, então **documento grande não vai para a CPU**, a menos que o
  administrador mande.
- **Rende bem na CPU:** embeddings (`nomic`, 137M), perguntas e gerações curtas, rascunhos e trabalho de fundo `baixa`.
- **O modelo certo para CPU é MoE** (`qwen3-ia` = 30B com só 3B ativos por token). Os densos (`gemma4:12b`, `qwen3.5:9b`)
  ficam lentos. Se a GPU passar a usar o `gemma4` (mais fiel) e a CPU o `qwen3-ia`, **a qualidade passa a depender da
  faixa**, o que choca com a "primazia perfeita". Por isso a política padrão é **o mesmo modelo nas duas faixas**, e a troca
  de modelo na CPU só existe como opção explícita (D9-C3).
- **Energia:** a CPU em carga total soma ~100–150 W **na mesma fonte de 850 W**, já no limite com 4 GPUs a 100 W (memória
  `servidor-ia-reboots-causa-fonte`). Isso é medido antes de ligar (11.3) e limitado por cota de CPU (11.2).

### 11.2 Instância `ollama-cpu` (servidor)
- **Unit nova** `/etc/systemd/system/ollama-cpu.service`, cópia da `ollama.service` com:
  - `Environment="OLLAMA_HOST=127.0.0.1:11435"`;
  - `Environment="CUDA_VISIBLE_DEVICES=-1"` + `Environment="OLLAMA_LLM_LIBRARY=cpu_avx2"` (ou `cpu_avx512`, conforme o
    `lscpu` da 9.9), para garantir que **nenhuma** GPU é tocada;
  - `OLLAMA_NUM_PARALLEL=<c>` (medido), `OLLAMA_MAX_LOADED_MODELS=2`, `OLLAMA_KEEP_ALIVE=-1`, `OLLAMA_CONTEXT_LENGTH`
    (medido; padrão 16384 na CPU) e `OLLAMA_MAX_QUEUE=8`;
  - **mesmo diretório de modelos** (`OLLAMA_MODELS`, blobs compartilhados, sem cópia em disco). Os modelos **só são
    baixados pela instância principal**: a `ollama-cpu` não recebe `/api/pull`, e o gateway bloqueia;
  - **isolamento de recursos:**
    - `CPUAffinity=` reservando núcleos para o SO e para o runner da GPU, que também usa CPU (sampling, cópia de buffers).
      Padrão: deixar 4 threads livres;
    - `CPUQuota=` (teto de potência indireto);
    - `MemoryMax=` (padrão 60 GB; o resto fica para o SO, o page cache e a instância GPU);
    - `Nice=10` e `IOSchedulingClass=idle`;
  - `After=ollama.service` (a instância GPU sobe primeiro) e um `ollama-cpu-warm` próprio, que carrega os modelos da
    faixa CPU no boot.
- Backups, `test -s`, gravação atômica e `sync`, como em toda mudança de servidor de hoje.
- **Rollback:** `systemctl disable --now ollama-cpu` + gateway com `--cpu-lane=off`, de 1 flag.

### 11.3 Medição antes de ligar (spike, sem rotear nada)
Executada com a produção em horário combinado, **uma coisa por vez**, monitor de potência a 1 s e parada no 1º reboot:
1. **Inventário do hardware:**
   - `lscpu` (modelo, núcleos/threads, AVX2/AVX-512, NUMA);
   - `free -g`;
   - banda de memória (`mbw`, se houver, ou inferida pela taxa de geração);
   - sensores (`sensors`) e potência do pacote da CPU por RAPL (`/sys/class/powercap/intel-rapl*/energy_uj`). Se for
     AMD, `amd_energy` ou `zenpower`, conforme o que existir.
2. **Velocidade na CPU**, por modelo (`qwen3-ia`, `nomic-embed-text`, `gemma4:12b`, `qwen3.5:9b`):
   - prefill com 1k, 4k e 8k tokens;
   - geração de 500 tokens;
   - carga a frio e RAM ocupada;
   - variando `num_thread` (½, ¾ e todas as threads livres).
3. **Qualidade na CPU:** S2 (22 seções de b01) e S3 JSON, comparadas com a mesma suíte na GPU. **Com o mesmo modelo e a
   mesma quantização, o resultado tem de ser equivalente.** Diferença acima do ruído barra o modelo na CPU.
4. **Interferência:** com a GPU em carga multiagente (os degraus da 8.9), rodar a CPU em carga e medir:
   - **(a)** a queda de vazão da GPU, porque o runner da GPU depende de CPU;
   - **(b)** a temperatura da CPU;
   - **(c)** a potência somada. Com isso se fixam `CPUAffinity`, `CPUQuota` e `num_thread`.
5. **Energia:** pico somado de GPU (nvidia-smi) + CPU (RAPL) com tudo em carga. **Teto proposto: GPU + CPU ≤ ~650 W
   medidos**, para sobrar margem dos 850 W ao resto da máquina e aos picos de milissegundos. Se passar, baixar a
   `CPUQuota`.
6. **Saída:** report com a tabela "modelo × faixa × velocidade × qualidade × W", que alimenta as decisões D9-C e os
   parâmetros do roteador (11.4).

### 11.4 Roteamento por prioridade no gateway
**Novas entradas por pedido:**
- prioridade (`alta`, `normal` ou `baixa`, já com a regra dos IPs autorizados);
- endpoint (`generate`/`chat`/`embed`);
- modelo pedido;
- **tokens estimados do prompt** (`bytes_in / 3,5`, calibrado na 9.9);
- `num_predict` pedido;
- **espera estimada na GPU** = posição × média móvel (EWMA) da duração por faixa;
- **tempo estimado na CPU** = tokens ÷ prefill medido + `num_predict` ÷ geração medida;
- header opcional `X-Mcpollama-Lane: gpu|cpu|auto`.

**Regras, avaliadas em ordem e todas configuráveis por flag:**

| # | Regra | Padrão |
| --- | --- | --- |
| R1 | `embed` → **CPU** se a D9-C2 aprovar o `nomic` na CPU (libera ~0,8 GB de VRAM); senão, GPU | conforme a D9-C2 |
| R2 | Modelo pedido **não disponível na CPU** → GPU | sempre |
| R3 | `alta` → **sempre GPU**, nunca CPU | sempre |
| R4 | `X-Mcpollama-Lane: cpu\|gpu` vale **só de IP autorizado** (a mesma lista `--priority-ips`); de outro IP é ignorado e fica `auto` | sempre |
| R5 | Prompt estimado > `--cpu-max-prompt-tokens` → GPU (documento grande na CPU levaria minutos) | 4.000 tokens (ajustado pela medição) |
| R6 | `baixa` → **CPU** se o tempo estimado na CPU ≤ `--cpu-max-est` (padrão 120 s); senão, fila `baixa` da GPU | ligado |
| R7 | `normal` → GPU. **Transborda para a CPU só se:** espera estimada na GPU > `--overflow-wait` (padrão 60 s) **e** tempo estimado na CPU < espera estimada na GPU **e** o modelo é o mesmo nas duas faixas (ou a D9-C3 permitir outro) | ligado |
| R8 | **Roubo de trabalho:** quando um slot da GPU fica livre e a fila da GPU está vazia, o gateway pega o pedido **mais antigo que ainda espera** na fila da CPU e o leva para a GPU. Pedido já iniciado na CPU não migra | ligado |
| R9 | A faixa CPU fora do ar (instância parada ou erro de conexão) → tudo vai para a GPU, com registro de evento e alerta no monitor | sempre |

- **Justiça e limites** (por IP, já corrigidos na 9.1) valem **por faixa e no total**: um IP não ocupa as duas faixas inteiras.
- **Escalonamento de partida** (300 ms) vale entre as duas faixas juntas, porque o pico de energia é da máquina toda.
- **Resposta:** header `X-Lane: gpu|cpu` + `X-Routing-Reason: R6` etc. O mcpollama mostra `lane=` e `rota=` na linha `[uso]`.
- **Ações do administrador** no monitor, além das da §4.3: **mover para GPU** e **mover para CPU**, só para pedidos em
  espera, auditadas. Nenhuma das duas fura a justiça.

### 11.5 Monitor e SQLite
- **Monitor:**
  - coluna **Faixa** em todas as tabelas;
  - filtro GPU/CPU/todas;
  - cartão **CPU**: uso %, load, RAM usada/livre, temperatura, W do pacote (RAPL) e modelos residentes na `ollama-cpu`;
  - cartão **Energia total**: GPU + CPU, com alerta visual acima do teto da 11.3;
  - estatísticas por faixa: espera p50/p95, vazão, transbordos e roubos por hora.
- **SQLite:**
  - em `requests`: `routing_rule`, `est_gpu_wait_ms`, `est_cpu_ms`, `lane_moved_by` (`r8`/`admin`/nulo);
  - tabela nova `cpu_samples(ts, util_pct, load1, ram_used_mib, ram_avail_mib, temp_c, rapl_w, boot_id)`, com retenção de 7 dias;
  - eventos `cpu_lane_down`/`cpu_lane_up`.

### 11.6 Etapas da extensão

| Etapa | Conteúdo | Muda o servidor? | Modelo do agente |
| --- | --- | --- | --- |
| 9.9 | **Inventário + spike de medição** (11.3, itens 1–3) com uma `ollama-cpu` **temporária**, sem tráfego de produção; report | **sim — autorização** | opus (coordena; risco de energia) |
| 9.10 | **Interferência e energia** (11.3, itens 4–5) durante a carga multiagente da GPU; fixa `CPUAffinity`, `CPUQuota`, `num_thread` e o teto de W | carga | opus |
| 9.11 | Decisões D9-C com o owner, a partir dos reports da 9.9/9.10 | não | loop principal |
| 9.12 | **Código:** faixa CPU no gateway (backend 2, R1–R9, EWMA, estimador de tokens, roubo de trabalho, R9 de queda), monitor (11.5), store (11.5), mcpollama (`lane`/`rota` no `[uso]`). Testes com **dois Ollama falsos** (gpu e cpu), um teste por regra, fuzz da decisão + concorrência, `-race` | não | sonnet |
| 9.13 | **Revisão adversarial** da faixa CPU: falsificação de lane/prioridade, inanição entre faixas, roubo de trabalho em corrida, queda da `ollama-cpu` no meio de um pedido, estimador manipulável (prompt pequeno em bytes e grande em tokens) | não | opus |
| 9.14 | **Implantação:** `ollama-cpu.service` + `ollama-cpu-warm` definitivos (11.2), gateway com `--cpu-lane=on`; backups e rollback ensaiado (`--cpu-lane=off` + `disable --now`) | **sim — autorização** | loop principal |
| 9.15 | **Aceite ao vivo:**<br>- (a) carga mista: 10 agentes `normal` na GPU + 10 tarefas `baixa` de fundo;<br>- (b) o monitor mostra as duas faixas;<br>- (c) transbordo R7 e roubo R8 observados e auditados;<br>- (d) a qualidade das respostas da CPU é igual à da GPU (S2/S3);<br>- (e) a `ollama-cpu` derrubada no meio → R9 manda para a GPU, sem erro ao cliente que ainda esperava;<br>- (f) potência dentro do teto;<br>- (g) prova por 2ª via no SQLite. Para no 1º reboot | carga | opus |
| 9.16 | Documentação (`CONEXAO-IA.md` local + servidor, `README`, `CLAUDE.md`) e report final | não | sonnet |

### 11.7 Decisões D9-C (para a 9.11, com os números da medição)
- **D9-C1:** ligar a faixa CPU? Só se a 9.9/9.10 mostrarem ganho real sem roubar vazão da GPU e dentro do teto de energia.
- **D9-C2:** `nomic-embed-text` na CPU (R1)? Recomendação prévia: sim, se a latência do embed na CPU ficar abaixo de ~1 s.
- **D9-C3:** modelo da faixa CPU: **o mesmo da GPU** (padrão, pela "primazia perfeita") ou um MoE mais rápido na CPU com
  qualidade diferente, restrito a `baixa`.
- **D9-C4:** limiares `--cpu-max-prompt-tokens`, `--cpu-max-est`, `--overflow-wait`, slots da CPU e núcleos reservados.
- **D9-C5:** quem pode forçar a faixa (`X-Mcpollama-Lane`): a mesma lista de IPs da prioridade `alta` (padrão) ou uma lista própria.

### 11.8 Riscos da extensão

| Risco | Mitigação |
| --- | --- |
| A carga da CPU derruba a fonte | Medição de energia antes (9.10), teto de W, `CPUQuota`, parada no 1º reboot |
| A CPU ocupada atrasa o runner da GPU | `CPUAffinity` reservando núcleos + medição de interferência (9.10) |
| Qualidade diferente entre faixas | Mesmo modelo por padrão (D9-C3) + S2/S3 na CPU (9.9, 9.15) |
| Pedido preso numa faixa lenta | R5/R6 por tempo estimado, R8 (roubo pela GPU), ação "mover para GPU" |
| Estimador enganado (poucos bytes, muitos tokens) | Teto pelo `bytes_in` real + correção pela razão bytes/token medida + revisão 9.13 |
| RAM esgotada (modelos + KV + page cache) | `MemoryMax` na `ollama-cpu`, `MAX_LOADED_MODELS=2`, monitor de RAM |
| Atualização do Ollama esquecida numa das instâncias | As duas units usam o mesmo binário `/usr/local/bin/ollama`; checklist no `CONEXAO-IA.md` |

### 11.9 Sobrecarga: repasse para um Ollama secundário (pool de backends) — pedido do owner, 2026-10-28 23:01
> Owner: "e em caso de sobrecarga, podendo repassar para o ollama secundário". A §11 tem uma faixa CPU fixa; aqui o gateway
> passa a ter um **pool de backends**: o primário (GPU local), a `ollama-cpu` local e **N secundários remotos**
> (outro PC ou servidor com Ollama **na LAN da CSL**; somente modelos locais, nunca nuvem). Para onde vai cada pedido
> depende da prioridade, da sobrecarga e da saúde de cada backend.

**Registro de backends** (arquivo de configuração `--backends /etc/mcpollama-gateway/backends.json`, 0600):

| Campo | Exemplo | Uso |
| --- | --- | --- |
| `id` | `gpu-local`, `cpu-local`, `sec-ws01` | nome no monitor e no SQLite |
| `url` | `http://127.0.0.1:11434`, `https://10.100.x.y:8443` | destino |
| `tipo` | `gpu`, `cpu`, `remoto` | entra nas regras |
| `token_file` | `/etc/mcpollama-gateway/sec-ws01.token` | Bearer **próprio** do secundário (o token do cliente nunca é repassado); lido do arquivo, nunca logado |
| `tls_ca` | caminho do PEM | TLS do remoto (mesmo padrão `SystemCertPool` + CA do mcpollama) |
| `slots` / `ctx` | `2` / `32768` | concorrência e contexto no secundário |
| `modelos` | `["qwen3-ia"]` | o que ele serve; o **digest** é conferido por `/api/tags` |
| `classes` | `["alta","normal","baixa"]` | prioridades que ele aceita |
| `max_prompt_tokens` | `8000` | teto (CPU e máquinas fracas) |
| `ordem` | `1, 2, 3…` | preferência no repasse |

**O que é sobrecarga no primário** (qualquer um dos itens, todos configuráveis):
- **S1:** espera estimada na fila da GPU > `--overload-wait` (padrão 60 s). É a mesma EWMA da §11.4;
- **S2:** fila da GPU > `--overload-queue` (padrão 75% do `--queue-max`);
- **S3:** **primário sem saúde:**
  - `/api/version` falha;
  - o modelo não está residente;
  - `ollama ps` mostra que o modelo saiu de "100% GPU" (caiu para CPU por falta de VRAM);
  - 3 erros seguidos (5xx, conexão, timeout) → **disjuntor aberto** por 30 s, depois meio-aberto com 1 pedido de prova;
- **S4:** **alerta de energia**: potência somada acima do teto da §11.3 por mais de X s. O gateway segura novos inícios no primário (protege a fonte) e repassa ao secundário.

**Regras de repasse** (somam-se às R1–R9 da §11.4; avaliadas quando o pedido **ainda não começou**):

| # | Regra |
| --- | --- |
| F1 | Sem sobrecarga → primário (ou CPU pelas R1/R6). |
| F2 | Com sobrecarga → o 1º backend saudável, em `ordem`, que **sirva o mesmo modelo com o mesmo digest**, aceite a classe de prioridade e caiba em `max_prompt_tokens`, com slot livre ou com espera estimada menor que a do primário. |
| F3 | `alta` **pode** ir a um secundário **remoto com GPU e o mesmo modelo** (D9-B3), mas **nunca** à CPU. |
| F4 | **Mesmo modelo, mesmo digest** é obrigatório por padrão ("primazia perfeita"). Modelo diferente só com permissão explícita (D9-B2) e só para `baixa`. |
| F5 | Nenhum secundário elegível → o pedido **continua esperando** no primário (sem erro), até a espera máxima. |
| F6 | **Volta automática:** quando a sobrecarga termina (histerese: S1 e S2 abaixo de 50% do limiar por 30 s), os pedidos novos voltam ao primário. Os que já estão no secundário terminam lá. |
| F7 | **Durante a execução não há repasse.** Se o backend falhar no meio, o cliente recebe erro claro, e o mcpollama refaz **1 vez**, se couber no timeout: as tools são de leitura e o reprocessamento é seguro. |
| F8 | **Justiça e limites por IP valem no pool inteiro.** Um IP não ganha slots extras por causa do repasse. |

**Limite estrutural: o gateway roda no mesmo servidor.** Se o servidor de IA reiniciar (o problema da fonte), o gateway cai junto e não consegue repassar. Por isso:
- **mcpollama com fallback no cliente:** `--ollama-url-fallback` (lista), um secundário ou o gateway de outra máquina;
  - em **erro de conexão** ou **503 de sobrecarga** do primário, o mcpollama tenta o fallback **uma vez**;
  - a linha `[uso]` registra `backend=fallback`;
  - o token do fallback vem do próprio arquivo dele, com o mesmo cuidado de redação;
- **Opcional, depois:** um gateway também no secundário, para que os dois se cubram.

**Monitor e SQLite:**
- quadro **Backends**: estado (ok, sobrecarregado, disjuntor aberto, fora), slots usados/livres, fila, latência p50, modelo e digest, último erro;
- a coluna **Backend** nas tabelas;
- os eventos `overload_on/off`, `backend_down/up` e `breaker_open/close`;
- ação do administrador: **"drenar backend"** (não recebe pedidos novos; os que estão rodando terminam), para manutenção;
- SQLite: `requests.backend_id`, `requests.failover_rule`, tabela `backend_samples(ts, backend_id, estado, slots_usados, fila, p50_ms)` com 7 dias.

**Segurança e sigilo:**
- Os secundários ficam **só na LAN da CSL**, com TLS + token próprio.
- Os documentos jurídicos **não saem da rede**, e nunca vão para nuvem: o owner decidiu "somente modelos locais".
- Um backend remoto só entra no `backends.json` com aprovação do owner (D9-B1).
- A revisão adversarial (9.13) cobre:
  - SSRF pelo `url` (o arquivo é do root e não aceita URL vinda do cliente);
  - vazamento do token do secundário;
  - disjuntor manipulável por erros forçados;
  - inanição do primário por histerese mal calibrada.

**Etapas (encaixam na extensão):**
- **9.12:** inclui o código do pool: registro de backends, saúde, disjuntor, S1–S4, F1–F8, "drenar".
  - Testes com **três Ollama falsos** (gpu, cpu e remoto) cobrindo:
    - um por regra;
    - disjuntor;
    - histerese;
    - digest diferente recusado.
  - Inclui também o `--ollama-url-fallback` do mcpollama.
- **9.13:** inclui os vetores acima.
- **9.14:** um secundário real (D9-B1), com token próprio.
- **9.15:** aceite com:
  - (h) sobrecarga provocada (10 agentes + fila cheia) → repasse ao secundário, visível no monitor;
  - (i) o primário parado de propósito (`systemctl stop ollama`) → disjuntor + repasse;
  - (j) a volta automática pela histerese;
  - (k) o fallback do mcpollama com o gateway parado.

**Decisões D9-B:**
- **D9-B1, qual é o secundário remoto:** que máquina da LAN, com qual GPU, VRAM e modelo; ou **só a `ollama-cpu` local** por enquanto. Sem secundário remoto, a §11.9 fica restrita a "sobrecarga → CPU local" mais o fallback do cliente.
- **D9-B2:** permitir modelo diferente no secundário para `baixa`. Padrão: não.
- **D9-B3:** `alta` pode ir a um secundário remoto com GPU e o mesmo modelo. Padrão: sim.
- **D9-B4:** limiares S1/S2/S4 e histerese. Os padrões acima são ajustados pela medição.
- **D9-B5:** fallback no mcpollama (lado do cliente), ligado por padrão quando houver secundário.

## 12. Changelog

- 2026-10-28 22:40 — plano criado (estado: PLANEJADO) pelo loop principal (Opus 5.5), a partir dos pedidos do owner das 22:38 ("pool de espera em fila na retaguarda… `/monitor`… SQLite… faça o plano") e das 22:39 ("possibilidade de promover ou remover ou despromover na fila").
  - Contexto do mesmo dia: nginx liberado para 12 conexões por IP às 22:36; produção com 4 GPUs a 100 W às 22:11; Fase 8 aguardando o teste de carga 8.9 (`gemma4:12b` × `qwen3.5:9b`, depois do `think:false`, etapa 8.8 em andamento).
  - Levantamento do SSOT: o `truenas-mcp/tasks/` guarda o estado em memória com TTL (molde de padrão, sem fila nem persistência); nenhuma referência usa SQLite.
  - Nenhuma execução.
- 2026-10-28 22:44 — **decisões do owner registradas** (ver o cabeçalho):
  - D9-1: LAN, ver sem login, ações com login;
  - D9-2: loopback;
  - D9-6: paralelo, implantar depois da 8.9;
  - D9-8: cancelar em execução + `alta` do cliente só de IPs autorizados (de outro IP é ignorado) + 1 administrador;
  - as demais com os valores recomendados.

  **Efeitos no desenho:**
  - **§3, prioridade:** `X-Mcpollama-Priority: alta` vale só quando o IP do cliente está na lista `--priority-ips` (CIDR). De outro IP, vira `normal` em silêncio, e isso fica registrado em `requests.priority_in`/`priority_final`.
  - **§4.2:** acrescenta a rota `POST /monitor/api/running/{id}/cancel` (fecha a conexão com o Ollama, libera o slot, `status=cancelado_admin`, auditado; o cliente recebe erro claro).
  - **§4.1/§6:** a página e as rotas GET dispensam login. O nginx continua com `allow 10.100.0.0/16; deny all` em `/monitor`. As rotas POST exigem sessão do administrador + CSRF.
  - **§6, CSRF com GET sem login:** as rotas GET não podem ter efeito colateral, e isso é testado na 9.5.
- 2026-10-28 23:01 — **Extensão "faixa CPU/RAM" acrescentada (§11)**, a pedido do owner.
  - Pergunta das 22:57: "podemos usar também a CPU e a RAM balanceando por prioridade…". Pedido das 22:59: "implemente isso no plano então, bem detalhado".
  - **Desenho:**
    - segunda instância `ollama-cpu` (127.0.0.1:11435, sem GPU, blobs compartilhados, `CPUAffinity`/`CPUQuota`/`MemoryMax`);
    - roteamento por pedido inteiro, com as regras R1–R9 (alta sempre GPU; baixa para a CPU se couber; normal transborda só por espera estimada; roubo de trabalho pela GPU; queda da CPU → GPU);
    - monitor e SQLite por faixa;
    - medição de velocidade, qualidade, interferência e **energia** antes de ligar (teto GPU+CPU ≈ 650 W, por causa da fonte de 850 W).
  - **Etapas novas 9.9–9.16, depois da 9.7.** Decisões D9-C1..C5. Riscos em §11.8.
  - **Numeração:** a extensão virou §11 e o Changelog passou a §12. As referências a "§11 Changelog" em mensagens anteriores a este registro apontam para o §12.
  - Nenhuma execução.
- 2026-10-28 23:03 — **§11.9 "Sobrecarga: repasse para Ollama secundário"** acrescentada, a pedido do owner (23:01: "e em caso de sobrecarga, podendo repassar para o ollama secundário").
  - **Desenho:**
    - pool de backends (`backends.json`: gpu-local, cpu-local, remotos da LAN com token e CA próprios);
    - detecção de sobrecarga S1–S4 (espera, fila, saúde/disjuntor/saída de 100% GPU, **alerta de energia**);
    - regras F1–F8 (mesmo modelo e digest obrigatório; `alta` nunca vai à CPU; sem repasse durante a execução; volta com histerese);
    - **fallback no mcpollama** (`--ollama-url-fallback`), porque o gateway roda no mesmo servidor e cai junto num reboot;
    - monitor (quadro Backends, "drenar backend");
    - SQLite (`backend_id`, `failover_rule`, `backend_samples`).
  - Encaixado nas etapas 9.12–9.15. Decisões D9-B1..B5. A D9-B1 (qual máquina é o secundário) é a que destrava o remoto.
  - Nenhuma execução.
- 2026-10-28 22:55 — **9.1 `src/gateway/queue` CONCLUÍDA** (agente sonnet, worktree isolado; commit "feat: Fase 9.1").
  - **Arquivos:** `src/gateway/queue/queue.go`, `queue_test.go`, `fuzz_test.go`.
  - **O que foi feito:**
    - faixas `geracao`/`embed` com slots independentes; prioridades `alta|normal|baixa`; justiça por cliente (IP + label) por "quem foi servido há mais tempo", FIFO dentro do cliente; envelhecimento `baixa`→`normal` (calculado na hora de escolher, o `priority_final` do registro guarda o nível efetivo na partida);
    - limites: total (503 `ErrFull`), por cliente (503 `ErrClientFull`), espera máxima (`ErrWaitTimeout`, 504), saída por contexto cancelado (registro `cancelado`);
    - escalonamento de partida (padrão do entrypoint 300 ms) **só na faixa `geracao`** (embed é leve e curto; atrasá-lo não ajuda a fonte);
    - ações atômicas sob o lock: `Promote`, `Demote`, `Remove` (o cliente recebe `ErrRemoved`, 409) e `Cancel` em execução (D9-8: cancela o contexto com causa `ErrAdminCanceled` e libera o slot **na hora**; o registro `cancelado_admin` sai no `Release` do proxy, uma única vez);
    - semântica de promover/despromover: sobe/desce um nível e entra no **fim** do novo nível (não fura quem já esperava); já em `alta`, "topo" (pin com contador decrescente); já em `baixa`, "fim" (contador crescente);
    - ids de 128 bits (`crypto/rand`, 32 hex); `Snapshot()` consistente com a ordem simulada de saída; ganchos `OnRecord`/`OnChange` chamados **fora do lock**.
  - **O que deu certo:** primeira compilação limpa; as 24 provas de unidade passam; o fuzz `FuzzConcurrentActions` (4 goroutines, ações aleatórias, invariantes verificados durante a corrida e dreno ao final) rodou 20 s com 77.837 execuções sem falha; `TestRandomActions` (40 rodadas semeadas) roda no gate normal.
  - **O que não deu certo / ajustes:** a 1ª versão dos arquivos foi recusada pela ferramenta (comando composto); recriados por Write, sem perda. Nenhum bug encontrado nos testes.
  - **Gate (números):** `go build ./...` ok; `go vet ./...` ok; `gofmt -l .` vazio; `go test -count=1 ./...` 12 pacotes ok (queue 1,8 s); `-race -count=3 ./gateway/...` ok (6,2 s); build Linux `GOOS=linux CGO_ENABLED=0 go build ./gateway/...` ok. `go.mod`/`go.sum` **não** alterados nesta etapa (o SQLite entra na 9.3).
  - **Estado:** 9.1 concluída.
  - **Pendências (para a 9.5):** (1) `Retry-After` é fixo em 5 s (ajustar por vazão real?); (2) fairness considera só o último início por cliente; avaliar se um cliente com muitos pedidos em execução deveria perder preferência; (3) `Snapshot` é O(n²) por faixa (n ≤ 64, aceitável); (4) rodar o fuzz por 10 min.
- 2026-10-28 22:58 — **9.1 correção de segurança (alerta MEDIUM do coordenador): limite e justiça ancorados no IP** (commit "fix: Fase 9.1 - limite/justica por IP (label nao falsifica)").
  - **Achado:** `clientKey = ip+"|"+label`; o label vem do header `X-Mcpollama-Client`, controlado pelo cliente. Variando o label, um IP furava `--queue-per-client` e ganhava mais fatias no rodízio.
  - **Correção (`src/gateway/queue/queue.go`):**
    1. rodízio em **dois níveis**: primeiro entre IPs (o servido há mais tempo; empate pelo pedido mais antigo), depois entre os labels do IP vencedor (sub-filas), depois FIFO; o `Snapshot` simula a mesma ordem;
    2. novo teto **por IP** (`Config.PerIP`, padrão = `PerClient`; flag `--queue-per-ip` no entrypoint, 9.4) somando todos os labels, além do teto por IP+label;
    3. limite de **labels distintos por IP** (`Config.LabelsPerIP`, padrão 16; flag `--labels-per-ip`): acima disso o label vira `"<ip>/outros"` sem rejeitar o pedido (o registro guarda o label normalizado); a contagem usa referências dos pedidos ativos e é liberada em remoção, término, cancelamento e abandono;
    4. invariantes novos em `checkInvariants`: `perIP` coerente e ≤ `PerIP`, labels por IP ≤ `LabelsPerIP`+1, referências de label = pedidos ativos.
  - **Testes de regressão (`ip_test.go`):** 1 IP com 50 labels diferentes → exatamente 5 em espera (teto por IP 5) e 45 rejeições; o teto de um IP não afeta outro; 50 labels com `LabelsPerIP=4` viram 5 labels (46 em `outros`); 1 IP com 20 labels × 1 IP com 1 label: 20 partidas alternam com diferença ≤ 1. O fuzz agora usa 2 IPs, 16 labels, `PerIP=4`, `LabelsPerIP=3` e verifica os invariantes novos.
  - **Ajustes de teste:** `TestLimits` ganhou `PerIP: 10` e `TestSweep` espera 2 entradas de memória (IP + IP|label).
  - **Gate:** `go vet`/`gofmt` limpos; `go test -count=1 ./gateway/...` ok; `-race -count=3` ok (6,7 s); fuzz 20 s, 35.408 execuções, sem falha.
  - **Pendência nova:** o proxy (9.2) e o entrypoint (9.4) devem repassar `--queue-per-ip` e `--labels-per-ip`.
- 2026-10-28 23:08 — **9.2 `src/gateway/proxy` CONCLUÍDA** (commit "feat: Fase 9.2").
  - **Arquivos:** `src/gateway/proxy/proxy.go`, `proxy_test.go`; `queue.Request` ganhou `ID` opcional (o proxy gera o id antes, para que até a rejeição 503 leve `X-Request-Id`).
  - **O que foi feito:**
    - `httputil.ReverseProxy` com `Rewrite`, `FlushInterval: -1`, transporte sem proxy de ambiente; o `Rewrite` remove `Authorization`, `Cookie`, `X-Real-IP`, `X-Forwarded-*` e os `X-Mcpollama-*` antes de falar com o Ollama;
    - `/api/generate`, `/api/chat` → `geracao`; `/api/embed` e `/api/embeddings` → `embed`; `/api/tags|ps|show|version` diretos; `/api/pull|push|create|delete|copy|blobs` → 403 (qualquer método); qualquer outro caminho → 404; método ≠ POST nas rotas da fila → 405;
    - **extensão consciente:** `/v1/chat/completions`, `/v1/completions`, `/v1/embeddings` (fila) e `/v1/models` (direto), porque o OpenClaude (`ia-local`) provavelmente fala o dialeto OpenAI; sem isso ele quebraria com o Ollama só no loopback (D9-2). Os contadores aceitam `prompt_tokens`/`completion_tokens`. **A confirmar na 9.6** se o nginx realmente roteia `/v1/` ao gateway;
    - contadores só do **último chunk**: o corpo da resposta passa por um `countingBody` que guarda só os últimos 16–32 KiB em memória; na saída, lê só a última linha por regex (aspas escapadas dentro do texto do modelo não casam). Texto nunca é guardado nem logado. Teste com `context` de 140 KB cobre o caso em que a linha final é maior que o buffer;
    - headers `X-Request-Id` (128 bits), `X-Queue-Ms`, `X-Queue-Position`; `Retry-After` no 503; 504 e 409 mapeados;
    - IP do cliente: `X-Real-IP` só vale se o par TCP for loopback (nginx local) e se houver exatamente 1 valor válido; senão vale o IP do par;
    - headers `X-Mcpollama-Client` (`[A-Za-z0-9._:/@+-]{1,64}`, senão vira `invalido`; ausente vira `anonimo`) e `-Tool` (`[a-z0-9_.-]{1,32}`); `-Priority`: `alta` só de IP em `PriorityNets`, de outro vira `normal` em silêncio (`priority_in` x `priority_final`);
    - allowlist opcional de modelos (aceita `:latest`): 403 sem tocar o Ollama; JSON inválido → 400; corpo acima do teto (padrão 32 MiB) → 413;
    - **cancelar em execução (D9-8):** antes dos headers do Ollama, o cliente recebe 409 "cancelado pelo administrador"; no meio do stream, o `countingBody` fecha o NDJSON com uma linha `{"error":"cancelado pelo administrador"}`, que o `streamNDJSON` do mcpollama já mostra como "o servidor reportou erro durante a geração";
    - desconexão do cliente no meio do stream: o `ReverseProxy` aborta (`ErrAbortHandler`), o `defer` do handler faz `Release`, a conexão com o Ollama fecha e o slot volta (registro `cancelado`/`client_gone`).
  - **O que não deu certo e solução:** (1) o teste `TestAdminCancelBeforeResponseHeaders` travou em `httptest.Server.Close`: um Ollama falso que não lê o corpo nunca vê a desconexão (o servidor Go só inicia a leitura de fundo depois de consumir o corpo). Solução: o falso passa a ler o corpo, como o Ollama real; (2) `go vet` acusou `t.Fatal` em goroutine: criado `postBg` sem `t`.
  - **Gate (números):** `gofmt -l` vazio; `go build ./...` e `go vet ./...` ok; `go test -count=1 ./...` 12 pacotes ok (proxy 0,7 s, queue 1,9 s); `-race -count=5 ./gateway/...` ok (proxy 4,3 s, queue 10,5 s); build Linux `./gateway/...` ok. 20 testes do proxy + `TestParseCounters` (6 casos).
  - **Estado:** 9.2 concluída.
  - **Pendências (9.5):** (1) sem timeout de execução próprio (depende da desconexão do cliente e do nginx); avaliar `--run-timeout`; (2) `X-Queue-Position` é a posição na entrada, não atualizada; (3) 502 do Ollama reinício: o cliente recebe `Ollama indisponível`, sem `Retry-After`; (4) confirmar rotas `/v1/*` na 9.6; (5) smuggling (CL/TE duplicados) hoje depende do `net/http` e do nginx: criar vetor executável.
- 2026-10-28 23:12 — **9.3 `src/gateway/store` + `src/gateway/gpu` CONCLUÍDA** (commit "feat: Fase 9.3").
  - **Arquivos:** `src/gateway/store/store.go`, `store_test.go`, `sentinel_test.go`; `src/gateway/gpu/gpu.go`, `gpu_test.go`; `src/go.mod`/`go.sum`.
  - **Dependência nova (autorizada, D9-5):** `modernc.org/sqlite v1.61.0` (Go puro, sem cgo) com 7 indiretas (`libc`, `mathutil`, `memory`, `go-humanize`, `go-isatty`, `go-strftime`, `bigfft`). O `go get` **subiu `golang.org/x/sys` de v0.48.0 para v0.49.0** (exigência do sqlite); a suíte inteira, inclusive OCR/Windows, passou depois da subida.
  - **O que foi feito (store):**
    - banco criado com 0600 antes do SQLite abrir (o `-wal`/`-shm` herdam a permissão); DSN com `busy_timeout(5000)`, `journal_mode(WAL)`, `synchronous(NORMAL)`; esquema do §5 (requests, audit, gpu_samples, events) + `meta` e os índices `requests(enqueued_at)`, `requests(client_label, enqueued_at)`, `gpu_samples(ts)` (+ audit/events por `ts`);
    - **gravador assíncrono em lote** (canal de 4096, lote de 200 ou 250 ms, uma transação por lote); com o canal cheio o item é descartado e **contado**; a contagem vira o evento `registros_descartados` na próxima transação; `Flush()` é uma barreira (nunca descartada); `Close()` drena;
    - auditoria **síncrona** (`AddAudit`), por ser rara e não poder se perder;
    - leituras para o monitor: `History` (filtros cliente/modelo/período, paginação, tudo parametrizado), `Stats` (p50/p95 de espera e execução, totais, por status e classe, vazão/min, tokens/s), `GPUSince`, `Events`, `Audits`;
    - retenção: `Prune` (requests 90 d, gpu 7 d; audit e events nunca expiram), `Vacuum`, `Maintain` com datas de vencimento em `meta` (sobrevivem a reinício: poda diária, VACUUM semanal) e `RunMaintenance` horário;
    - `BootID` lê `/proc/sys/kernel/random/boot_id`; na abertura grava `gateway_inicio` e, se o boot id mudou desde a última vez, `boot_novo` (os reboots aparecem no histórico).
  - **O que foi feito (gpu):** `Collector` com `nvidia-smi --query-gpu=index,pci.bus_id,power.draw,memory.used,utilization.gpu,temperature.gpu --format=csv,noheader,nounits` a cada 5 s (timeout 4 s por chamada), `ParseCSV` (`[N/A]` vira 0), `ReadAER` de `/sys/bus/pci/devices/0000:00:1b.4/aer_dev_correctable` (lê `TOTAL_ERR_COR`, senão soma); `AERPath` configurável (`-` desliga); `Runner` injetável; `Latest()` para o monitor.
  - **Teste-sentinela (`sentinel_test.go`):** prompt, system, resposta e token com strings únicas passam pelo proxy real; depois de `Flush`, os bytes de `.db`, `-wal` e `-shm` **não** contêm nenhuma delas, nem depois do `Close`. Há **controle positivo**: o label do cliente (metadado gravado) é encontrado em `.db` ou `-wal`, o que prova que a varredura enxerga os dados.
  - **O que não deu certo / ajustes:** o primeiro desenho do `Stats` misturava rejeitados no cálculo de espera; a espera agora só conta pedidos que chegaram a um slot (`started_at > 0`). Removido um teste de inundação que dependia da velocidade da máquina (o descarte é coberto de forma determinística por `TestDropCounter`, com canal sem consumidor: 5 itens, capacidade 2, 3 descartados).
  - **Gate (números):** `gofmt -l` vazio; `go build`/`go vet ./...` ok; `go test -count=1 ./...` 14 pacotes ok (store 0,6 s, gpu 0,1 s); `-race -count=2 ./gateway/... ./ollama ./tools` ok (gpu 3,2 s, proxy 2,4 s, queue 5,0 s, store 4,1 s, ollama 2,5 s, tools 6,0 s); build e vet Linux (`GOOS=linux CGO_ENABLED=0`) de `./gateway/...` ok.
  - **Estado:** 9.3 concluída.
  - **Pendências (9.5):** (1) `Stats` lê até 200 mil linhas na memória por chamada (ok p/ 90 d de uso modesto; medir); (2) o descarte por buffer cheio só foi provado por teste determinístico, não sob carga real; (3) permissão do `nvidia-smi` sob `ProtectSystem=strict` (9.6); (4) o AER ausente vira `Err` "aer: ..." no `Snapshot` (ruído em máquina sem o dispositivo).
- 2026-10-28 23:25 — **9.4 `src/gateway/monitor` + `src/cmd/mcpollama-gateway` + mudanças no mcpollama CONCLUÍDA** (commit "feat: Fase 9.4").
  - **Arquivos novos:** `src/gateway/monitor/{monitor.go,admin.go,index.html,monitor_test.go}`; `src/cmd/mcpollama-gateway/{main.go,config.go,main_test.go}`; `src/tools/gateway_client_test.go`; `src/mcpollama-mcp/client_label_test.go`. **Alterados:** `src/ollama/{api,client,errors,types}.go`, `src/tools/{generate,chat,embed,filetools,registry}.go`, `src/mcpollama-mcp/config.go`. `TOOLS.md` e as descrições das tools **não** mudaram (`TestToolsListBudget*` e `TestCatalogInSync` passam).
  - **Monitor:**
    - HTML único embutido (`go:embed`), sem CDN; SSE em `/monitor/api/stream` (reenvia ao mudar a fila, com debounce de 150 ms, e a cada 2 s; limite de 20 painéis → 503) e polling de 2 s como reserva;
    - quadros do §4.1: cartões, em execução (botão **Cancelar**), fila (**▲ ▼ ✕**), concluídos recentes (50), GPUs + AER + boot id + uptime, 3 gráficos SVG da última hora (fila, vazão, espera média; amostrados a cada 5 s em memória), auditoria e eventos;
    - API do §4.2 mais `POST /monitor/api/running/{id}/cancel`; `/monitor/api/session`, `/series`, `/login`, `/logout`;
    - **todo dado vindo de cliente é escapado:** a página monta o DOM só com `textContent` (o teste proíbe `innerHTML`, `eval(`, URLs externas), o JSON sai do `encoding/json` (escapa `<`, `>`, `&`, e as quebras de linha ficam `\n` dentro de um único evento SSE) e há **CSP com hash** do script e do estilo inline (`default-src 'none'`, sem `unsafe-inline`). Teste de XSS com label `<img onerror>`, `</script>` e `\n\ndata: forged` passa;
    - **GET sem efeito colateral:** teste executa todos os GET (inclusive as URLs de ação, que devem dar 405/404) e confere que fila, auditoria e sessões não mudaram.
  - **Acesso (D9-1) e a escolha do hash:** GET aberto; todo POST exige sessão do único administrador **e** `X-CSRF-Token`. **Hash: PBKDF2-HMAC-SHA256 da stdlib (`crypto/pbkdf2`, Go 1.24+), 600.000 iterações, em vez de bcrypt**, porque evita mais uma dependência (o SQLite já é uma) e atende bem a um único administrador com senha longa; o formato da linha (`usuario:pbkdf2-sha256$iter$salt$hash`) é autodescritivo, então trocar por bcrypt depois é simples. Arquivo `--admin-file` com permissão de grupo/outros é recusado em Unix; subcomando `mcpollama-gateway hash-password -user admin` (senha lida do **stdin**, nunca da linha de comando; mínimo 8 caracteres). O `Verify` calcula o hash sempre e compara em tempo constante (usuário inexistente e senha errada têm a mesma resposta).
  - **Sessão e CSRF:** cookie `mcpgw_session` com 32 bytes aleatórios, `HttpOnly; Secure; SameSite=Strict; Path=/monitor`, 8 h; no máximo 8 sessões ativas; o token CSRF é por sessão e só é entregue a quem tem o cookie; camada extra: `Sec-Fetch-Site` diferente de `same-origin/none` ou `Origin` de outro host → 403; login só aceita JSON; **limite de tentativas: 5 falhas por IP (e 30 no total) em 15 min → 429**, mesmo com a senha certa. Cada ação grava na auditoria (usuário, IP, ação, id, antes, depois, motivo ≤ 200 caracteres).
  - **Entrypoint:** flags → env (`--listen 127.0.0.1:11500`, `--ollama`, `--db`, `--gen-slots`, `--embed-slots`, `--queue-max`, `--queue-per-client`, `--queue-per-ip`, `--labels-per-ip`, `--max-wait`, `--age-after`, `--start-stagger`, `--priority-ips`, `--allowed-models`, `--max-body-bytes`, `--admin-file`, `--gpu-interval`, `--nvidia-smi`, `--aer-path`, `--retention-requests`, `--retention-gpu`, `--version`, `--debug`); `--listen` fora do loopback é recusado (precisa de `--non-loopback-listen`); `GET /healthz`; encerramento por SIGINT/SIGTERM com 30 s de tolerância; grava os eventos `gateway_config`, `gateway_inicio` e `gateway_parada`. **Padrão de `--gen-slots` = 2** (o valor final sai da 8.9/D9-3 e é fixado na unit da 9.6). Build Linux: `GOOS=linux CGO_ENABLED=0`, 18 MB.
  - **Mudanças no mcpollama (§7):**
    - envia `X-Mcpollama-Client` (padrão `<hostname>/<pid>`, `--client-label`/`MCPOLLAMA_CLIENT_LABEL`; caracteres fora de `[A-Za-z0-9._:/@+-]` viram `_`, máx. 64) e `X-Mcpollama-Tool` (o nome da tool, via `ollama.WithTool` no registro, sem tocar nas definições);
    - lê `X-Queue-Ms` (`Usage.QueueMS`, `EmbedResponse.QueueMS`) e soma ao `queue_ms` do `[uso]` (generate, chat, embed e as tools de arquivo);
    - **503 com `Retry-After`** (só a resposta do gateway; um 503 puro do Ollama não): erro claro "a fila do servidor está cheia; tente de novo em ~N s" e **no máximo 1 nova tentativa**, apenas se sobrar pelo menos o dobro da espera no `--timeout`; **409** do administrador: "o administrador do servidor removeu ou cancelou este pedido…", sem nova tentativa; cancelamento no meio do stream chega como linha `{"error":"cancelado pelo administrador"}` e já aparece como erro do stream.
  - **O que não deu certo e solução:** (1) a camada de exibição desta sessão reescreveu a sequência de escape unicode do JSON nos arquivos de teste, e a asserção do XSS falhou em falso; solução: montar o texto procurado por bytes (`string([]byte{92,117,48,48,51,99})`), sem a sequência literal; (2) `readSmallJSON` exigia `Content-Type` até em ação sem corpo (415); agora só exige JSON quando há corpo; (3) um teste usava `r.Header.Set("Host")`, que não altera `r.Host`; corrigido no helper; (4) o aviso "eval()" do hook de segurança foi falso positivo (o teste procura `eval(` no HTML para provar que não existe).
  - **Gate (números):** `gofmt -l .` vazio; `go build ./...` e `go vet ./...` ok; `go test -count=1 ./...` 17 pacotes ok (monitor 0,8 s, gateway-cmd 5,6 s, tools 4,0 s); `-race -count=2` em `./gateway/... ./ollama ./tools ./cmd/mcpollama-gateway` ok (monitor 3,9 s, proxy 2,5 s, queue 4,9 s, store 5,1 s, tools 10,9 s, cmd 12,8 s); build **e** vet Linux do gateway ok; `TestToolsListBudget`, `TestToolsListBudgetWithFileTools` e `TestCatalogInSync` PASS sem regenerar o `TOOLS.md`.
  - **Estado:** 9.4 concluída; o código de 9.1–9.4 está pronto para a revisão adversarial (9.5).
  - **Pendências (para a 9.5):**
    1. o teste de ponta a ponta mcpollama→gateway real não existe (as duas metades têm teste próprio com Ollama/cliente falsos); montar na 9.5/9.7;
    2. o `Secure` do cookie impede o login por HTTP puro: o `/monitor` só funciona por HTTPS (nginx 8443); decidir se haverá modo de desenvolvimento;
    3. a página não foi aberta num navegador real (só testada por Go); validar visualmente e com o Chrome DevTools na 9.7;
    4. SSE: o limite de 20 painéis é global, não por IP;
    5. `Stats`/`History` de 7 d sobre 200 mil linhas na memória (medir);
    6. `/v1/*` (dialeto OpenAI) está no proxy; confirmar na 9.6 o `location` do nginx;
    7. sem `--run-timeout` no gateway; um stream pendurado só termina pela desconexão do cliente;
    8. o teste de ponta a ponta do entrypoint leva ~5 s para encerrar (a tolerância de desligamento); investigar se é o `Shutdown` esperando conexões ociosas;
    9. `golang.org/x/sys` foi de v0.48.0 para v0.49.0 (exigência do SQLite): reconferir o OCR no Windows real (`go test -tags ocr`) antes do merge.

- 2026-10-28 23:24 → 2026-10-29 00:15 — **9.5 revisão adversarial CONCLUÍDA** (subagente opus, worktree `agent-a5589ffef1a3fdd1b`; commits `3146afd` e `8814d7a` de correção + o commit "docs: Fase 9.5" do report).
  - **Report:** `.workspace/reports/MCPOllam2026-10-28-235633-revisao-adversarial-fase9.report.md`. Ataques, logs e fuzz em `….testes/`, como na Fase 7. As regressões ficam nos pacotes (`fase95_regress_test.go`).
  - **Método:**
    - leitura de todo o código da 9.1–9.4;
    - 30 vetores executáveis (`VEC|id|sev|arquivo:linha|resultado`) em 6 pacotes, rodados no `315f996` (`antes.log`) e no código corrigido (`depois.log`, `s2-depois.log`);
    - uma regressão por achado; prova de que ela falha antes, numa cópia do `315f996` (`git archive`, sem tocar o repositório principal);
    - fuzz de 10 min por alvo e gate completo.
  - **O que deu certo (sem achado, 18 vetores OK):**
    - `X-Real-IP` só de loopback com 1 valor (8 casos);
    - smuggling: CL+CL, CL/TE com espaço e CL negativo → 400; `chunked, identity` e TE duplicado → 501; CL+TE → o contrabandeado vira 2º pedido e recebe 403. O Ollama só viu o corpo exato;
    - 17 variantes de rota bloqueada (`/api/pull/`, `//api/pull`, `/api/%70ull`, `/API/pull`, `/v1/../api/pull`, `OPTIONS`/`HEAD`…): 0 chegou ao Ollama;
    - contadores não enganados por `"done":true` no texto nem por `tool_call`;
    - vazamento de slot: 0 nos 3 modos de falha;
    - inanição de `baixa`: o envelhecimento funciona;
    - escalonamento: quem desiste sai;
    - corrida das ações: 1.800 pedidos = 1.800 registros com `-race`;
    - SQLi no `/history`: parametrizado;
    - sentinela nos erros: o `error_class` só tem texto fixo;
    - XSS, CSRF (11 casos), fixação de sessão e exposição de segredo: OK;
    - `hash-password`: não ecoa em stdout/stderr;
    - ponta a ponta com `--debug`: token, prompt e resposta fora do log, do `/monitor` e do `.db`;
    - cliente mcpollama: nova tentativa só em 503 + `Retry-After` inteiro, nunca depois de 409; `TOOLS.md` e descrições intactos.
  - **O que não deu certo (12 achados, 0 Alto, todos corrigidos):**
    - **Médios:**
      - **M1** TOCTOU do login, achado do coordenador: 40 paralelas → 40 PBKDF2;
      - **M2** bloqueio global, achado do coordenador: 40 IPs travavam o administrador;
      - **M3** SSE só com teto global (pendência 4 da 9.4);
      - **P6** sem `--run-timeout`: slot preso para sempre (pendências 9.2-1 e 9.4-7);
      - **S2** histórico sem teto de linhas: ~864 mil/dia possíveis por 90 d.
    - **Baixos:**
      - **P2** rotação IPv6 dentro do /64;
      - **C1** `--priority-ips ::ffff:a.b.c.d` aceito e sem efeito. A hipótese inicial, "casa com todo IPv4", foi **refutada pelo spike**;
      - **Q1** registro perdido em Cancel × abandono;
      - **S1** `Stats` sem índice (48 ms por chamada com 207 mil linhas, 2 vezes a cada 2 s com painel aberto) e truncamento silencioso;
      - **S3** um evento de descarte por lote, numa tabela que nunca expira;
      - **M8** `/stats` sem cache;
      - **C5** SIGTERM com painel aberto esperava 30 s.
  - **Solução:**
    - **M1:** reserva atômica `tryBegin`/`end`/`abort` + semáforo de 2 PBKDF2;
    - **M2:** teto global retirado; 5 por IP + backoff exponencial por IP (15 min → 24 h). Não houve necessidade de `--admin-ips`;
    - **M3:** 4 streams por IP, 32 no total;
    - **P6:** `--run-timeout` (`MCPGW_RUN_TIMEOUT`), **padrão = `--max-wait` (10 min)**. O motivo é a simetria: ninguém segura um slot mais do que alguém pode esperar por ele; a folga sobre os 120 s do mcpollama e os ~87 s medidos é grande; só uma geração patológica do contexto inteiro (~11 min) é cortada. Antes dos headers → 504; no stream → linha NDJSON de erro; registro `erro/run_timeout`;
    - **S2:** `--max-request-rows` (`MCPGW_MAX_REQUEST_ROWS`, 3.000.000), `Store.Cap` por `rowid` a cada `Maintain` (de hora em hora), evento `teto_pedidos`;
    - **P2:** `fairIP` agrupa o IPv6 por /64;
    - **C1:** `parseNets` com `net/netip` (o mapeado vira IPv4; < /96 e zona são recusados);
    - **Q1:** o `abandon` emite o registro;
    - **S1:** índice `requests(finished_at)` (0,56 ms) + campo `truncated`;
    - **S3:** no máximo 1 evento por minuto, sem perder a contagem (o `Close` grava o resto);
    - **M8:** cache de 5 s por janela;
    - **C5:** `RegisterOnShutdown(mon.Close)` (6 ms);
    - **endurecimento H1:** o `Verify` compara o SHA-256 do usuário.
  - **Medida da pendência 9.3-1/9.4-5 (`Stats` com 200 mil linhas, banco sintético de 90 dias, 207 mil linhas):**
    - antes: 15m 48,2 ms, 7d 70,8 ms (4 MiB), 90d 305,6 ms (49 MiB, truncado);
    - depois: 15m 0,56 ms, 7d 33,6 ms;
    - a janela de 90d não é exposta e agora avisa o truncamento.
  - **Gate (números):**
    - `go build`/`go vet ./...` exit 0; `gofmt -l .` vazio;
    - `go test -count=1 ./...`: 16 pacotes `ok`;
    - `-race -count=1 ./...`: 16 `ok` (doctext 127 s);
    - **OCR com a DLL real** (SHA-256 conferido, `tessdata\best`): ocr, doctext, tools e mcpollama-mcp `ok`, **385 PASS, 0 SKIP**. O `x/sys` v0.49.0 está validado (pendência 9.4-9);
    - build Linux do gateway: 18.077.009 bytes; vet Linux ok;
    - `go mod tidy`: **sem diff** (o `modernc.org/sqlite` já era direto; o aviso do editor estava velho);
    - os 2 commits de correção foram verificados em exportação limpa (`git archive … src TOOLS.md`): tudo `ok`.
  - **Fuzz de 10 min por alvo** (23:55:32 → 00:05:36, paralelo, `-parallel 3`), todos PASS:
    - `FuzzConcurrentActions`: 213.938 execuções;
    - `FuzzFairIP`: 6.972.864;
    - `FuzzCountersFromModelText`: 3.282.757;
    - `FuzzParseNets`: 3.729.010;
    - `FuzzLoginLimiter`: 3.165.797.
  - **Erros de processo (corrigidos, detalhe no report §3.2 e §6):**
    - a 1ª versão do vetor Q4 usava prazo de 0 ms;
    - a 1ª regressão do Q1 passava no código antigo por causa de um `Release` final; isso foi provado na cópia do `315f996` e corrigido;
    - a 1ª regressão de SSE travou a suíte por 400 s por causa da ordem `defer` × `Cleanup`;
    - um caso do O1 foi mal nomeado;
    - um BOM do PowerShell 5.1 entrou num arquivo de teste;
    - a 1ª exportação limpa ficou sem o `TOOLS.md`.
  - **Estado:** 9.5 concluída.
  - **Pendências (numeradas; detalhe no report §7):**
    1. `hash-password` ecoa a senha no terminal: documentar `read -rs` na 9.6.
    2. **Decisão do owner:** o gateway não policia `options` (`num_ctx`, `num_gpu`, `keep_alive:0`), que podem forçar recarga ou descarga e prefill gigante.
    3. O limite de login e o de SSE usam o IP completo, não o /64. Basta enquanto o `/monitor` for só da LAN IPv4.
    4. INFO: um cliente que desiste antes de entrar não gera registro.
    5. 9.6: confirmar `proxy_request_buffering on` (o gateway não tem prazo de leitura do corpo).
    6. 9.6: confirmar `X-Real-IP` em todo `location`, e não pôr `127.0.0.1` em `--priority-ips`.
    7. 9.7: validar o teto de 3 milhões de linhas e o `--run-timeout` de 10 min com o volume real.
    8. Pendência 9.4-8 diagnosticada: o gateway encerra em 5 ms; os ~5 s são do ambiente do worktree (0,35 s em exportação limpa).
    9. Herdadas e ainda abertas:
       - 9.1-1 e 9.1-2;
       - 9.2-2, 9.2-3 e 9.2-4;
       - 9.3-2, 9.3-3 e 9.3-4;
       - 9.4-1, 9.4-2 e 9.4-3.
- 2026-10-29 16:08 — **9.6a políticas do gateway** (Sonnet 5.5, sem commit; decisão do owner de 2026-10-29: "políticas antes, instalar uma vez só"). Estado: **concluída no código, aguardando revisão e commit**.
  - **Escopo:** as políticas rodam no proxy, **antes de enfileirar**, nas rotas `/api/generate`, `/api/chat`, `/api/embed`, `/api/embeddings`, `/v1/chat/completions`, `/v1/completions` e `/v1/embeddings` (a `/api/embeddings` foi incluída por ser a forma antiga da `/api/embed`).
  - **Desenho da reescrita** (`gateway/proxy/policy.go`, função `applyPolicy`, pura e testável):
    1. o corpo vai para `map[string]json.RawMessage` (o `options` também). Os valores ficam como vieram (nada do prompt é interpretado), e só as chaves citadas são tocadas. `json.Unmarshal` valida o documento inteiro: JSON inválido, `null`, array, texto solto depois do objeto ou `model` que não é string dão **400** com mensagem fixa (sem eco do corpo), sem chegar ao Ollama e sem entrar na fila;
    2. **se nada mudou, o corpo original segue byte a byte.** Se mudou, o objeto é remontado com as chaves em ordem alfabética e os valores intactos, sem escape de HTML (`json.Marshal` de `RawMessage` escaparia `<`, `>` e `&`; montei à mão). O `ContentLength` é o do corpo novo; o `BytesIn` do registro continua sendo o tamanho recebido;
    3. as chaves são casadas **sem diferenciar maiúsculas**, como o decoder do Ollama (`Keep_Alive`, `OPTIONS`, `Num_Ctx`, `\u006beep_alive`, e o sinal Kelvin `\u212a`, que dobra para `k`, todos casam). Duas grafias da mesma chave de política (`model`/`Model`, `think`/`THINK`, `keep_alive`/`Keep_Alive`, `options`/`Options`) **dão 400**: depois do decode em mapa a ordem se perde e não dá para saber qual o Ollama usaria; o caso `model`/`Model` fecha um desvio da allowlist;
    4. **opções:** `keep_alive` de topo sai (salvo `--allow-client-keepalive`); `options.num_ctx` sai (salvo `--allow-client-num-ctx`); saem também as de `--strip-options`. As rotas de embedding passam pela mesma limpeza;
    5. **raciocínio, API nativa:** `think` ausente ou `null` vira `think:<--default-think>`; valor explícito (`true`, `false`, `"high"`) é respeitado (a chave mantém a grafia do cliente). **`/v1`:** sem `reasoning_effort` nem `reasoning` (ou ambos `null`), injeta `"reasoning_effort":"none"` quando o resultado é "desligado"; `think:false` no corpo do `/v1` **não conta** (o Ollama o ignora lá). Os valores `none` explícitos contam como desligado no registro;
    6. **precedência: corpo explícito > header `X-Mcpollama-Think` > nome virtual `-think` > `--default-think`.** O header é validado (`on`/`off`, sem diferenciar maiúsculas, espaços aparados; outro valor é ignorado) e **removido antes de ir ao Ollama**. O nome `<base>-think` (sufixo exato, base não vazia, só nas rotas de geração) é reescrito para a base; na nativa liga `think:true`, no `/v1` não injeta nada (fica o padrão do modelo, que raciocina);
    7. a **allowlist (D9-9)** é checada no nome BASE, depois da reescrita (`foo-think` com `foo` fora da lista dá 403; só um sufixo é retirado, então `qwen3-ia-think-think` dá 403). O registro/monitor mostram o nome base.
  - **Rótulo `think` no registro** (só metadado): `on`, `off`, `modelo` (padrão do modelo: `/v1` com `--default-think=true` e nada injetado) ou vazio (embeddings; linhas antigas). No `/v1`, header `on`/nome `-think` rotulam `on` mesmo sem injetar nada, porque foi pedido explícito de raciocínio.
  - **Armazenamento:** coluna `think TEXT` em `requests`; migração idempotente `migrate()` em `store.Open` (olha `PRAGMA table_info` e faz `ALTER TABLE ... ADD COLUMN` só se faltar). `queue.Request/Record/Item` ganharam `Think`; o `store.Row` também. **Monitor:** coluna "Raciocínio" (ligado / desligado / padrão do modelo / "-") nas três tabelas (rodando, esperando, recentes).
  - **Flags (`config.go`, env `MCPGW_*`, documentadas no `--help`):**

    | Flag | Env | Padrão |
    | --- | --- | --- |
    | `--default-think` | `MCPGW_DEFAULT_THINK` | `false` |
    | `--think-suffix` | `MCPGW_THINK_SUFFIX` | `-think` (`none` desliga; com espaço ou aspas é recusado) |
    | `--allow-client-keepalive` | `MCPGW_ALLOW_CLIENT_KEEPALIVE` | `false` |
    | `--allow-client-num-ctx` | `MCPGW_ALLOW_CLIENT_NUM_CTX` | `false` |
    | `--strip-options` | `MCPGW_STRIP_OPTIONS` | `num_gpu,main_gpu,low_vram,use_mmap,use_mlock,numa` (`none` = nenhuma; `num_ctx` sai por `--allow-client-num-ctx`, não por esta lista) |

    O evento `gateway_config` do SQLite passou a registrar as cinco.
  - **O que deu certo:**
    - a reescrita sem escape de HTML e com corpo intocado byte a byte quando não há o que mudar (teste com espaçamento estranho, `<b>&</b>` e `\u00e9`);
    - a grafia original da chave `think`/`reasoning_effort` do cliente é mantida quando ele é respeitado, então não há chave duplicada;
    - a regra "duas grafias = 400" simplificou o desenho e fechou o desvio por caixa.
  - **O que não deu certo (erros de processo meus, corrigidos):**
    1. dois scripts Python de edição escreveram um `\t\r\n` literal dentro de uma string Go em `config.go` (não compilava); corrigi com a ferramenta de edição;
    2. a 1ª versão do teste de `Keep_Alive` + `keep_alive` esperava 200; o código, corretamente, devolve 400 (duas grafias). Ajustei o teste ao desenho, não o contrário;
    3. um heredoc grande no bash falhou por aspas; passei a gravar os arquivos com a ferramenta de escrita.
  - **Arquivos:** novos `gateway/proxy/policy.go`, `gateway/proxy/policy_test.go` (30 casos de tabela nativo/`/v1`/embedding + 13 testes + `FuzzPolicyRewrite`), `gateway/store/think_test.go`, `gateway/store/sentinel_policy_test.go`. Alterados `gateway/proxy/proxy.go`, `gateway/queue/queue.go`, `gateway/store/store.go`, `gateway/monitor/index.html`, `gateway/monitor/monitor_test.go`, `cmd/mcpollama-gateway/config.go`, `main.go`, `main_test.go`. Nenhum commit; `src/files`, `src/doctext`, `src/ocr`, `D:\MCP`, servidor e `.mcp.json` intocados.
  - **Testes novos:** corpo antes e depois nativo e `/v1` (cada política); `-think` (nativo liga, `/v1` sem `reasoning_effort` injetado); header on/off e corpo explícito vencendo o header; allowlist no nome base; JSON inválido (9 formas × 6 rotas) = 400 e 0 chamadas ao Ollama, sem eco do prompt e sem entrar na fila; `keep_alive`/`num_ctx`/`num_gpu` removidos nas 7 rotas e `--allow-*` religando a passagem; truques de caixa e de escape; `ContentLength`; limite de corpo (413 como antes); migração sobre um banco antigo aberto 2 vezes; flags, env e `--help`; ponta a ponta por `run()` com o histórico do monitor; coluna no HTML.
  - **Sentinela:** `TestSentinelSurvivesThePolicies` (prompt reescrito, `/v1`, 400, 403 e gêmeas de caixa): a string única não aparece no `.db`/`-wal`/`-shm`, nem aberto nem fechado, nem no log, e os 400 não ecoam o corpo (controle positivo: o rótulo do cliente aparece no banco). O teste da 9.3/9.5 (`TestSentinelNeverReachesTheDatabase`) segue passando.
  - **Gate (dentro de `src/`):** `go build ./...` ok; `go vet ./...` limpo; `gofmt -l .` vazio; `go test -count=1 ./...` **16 pacotes ok** (1 sem testes: `mcp`); `go test -race -count=1 ./gateway/... ./cmd/mcpollama-gateway` **6 pacotes ok** (cmd 7,0 s, monitor 3,9 s, store 3,7 s, proxy 3,0 s, queue 3,0 s, gpu 2,1 s); build Linux amd64 com `CGO_ENABLED=0` ok (18.131.894 bytes).
  - **Fuzz `FuzzPolicyRewrite` (5 min, `-parallel 3`): PASS, 7.029.893 execuções, 504 entradas no corpus.** Invariantes: nunca entra em pânico; se aceito, o resultado é um objeto JSON válido, sem `keep_alive`, sem `options.num_ctx/num_gpu/main_gpu/low_vram/use_mmap/use_mlock/numa` (sem diferenciar caixa), com o raciocínio definido (nativa: chave `think` não nula; `/v1`: modo definido, e `off` implica `reasoning_effort` ou `reasoning`; embedding: sem modo), o `model` do corpo igual ao nome base e a 2ª passada sem sufixo não muda nada.
  - **Ataques da 9.5 reexecutados** (arquivos de `...fase9.testes\` copiados para os pacotes, `go test -count=1 -v -run TestAdv ./gateway/... ./cmd/mcpollama-gateway ./tools`, depois removidos de `src`): **30 vetores `VEC|...|OK`, 0 diferentes de OK, 0 FAIL**, todos os pacotes `ok` (store 176,9 s por causa do S2 com 3,15 milhões de linhas). Os 29 ids do `depois.log` da 9.5 estão todos presentes, e o 30º é o S2 (que na 9.5 estava em `s2-depois.log`). Log: `%TEMP%\ataques-9.6a.log`.
  - **Estado:** 9.6a concluída no código.
  - **Pendências (numeradas):**
    1. **Validar ao vivo no Ollama v0.40.8** (uma chamada de cada vez, depois da implantação): `think:false` e `reasoning_effort:"none"` em modelo que NÃO raciocina (se o Ollama recusar, a política precisa saber o que o modelo suporta; hoje assume o `qwen3-ia`), `qwen3-ia-think` na nativa e no `/v1`, e que o `options` limpo não recarrega o modelo (`/api/ps` antes e depois).
    2. **Clientes que mandam `num_ctx` de propósito** (conferir se o mcpollama manda, em `src/ollama`) passam a ser ignorados em silêncio; se a 9.7 mostrar uso legítimo, usar `--allow-client-num-ctx`. Hoje a única defesa de prefill gigante é `--max-body-bytes`.
    3. Duas grafias de uma chave de política dão 400; um cliente legítimo nunca faz isso. Se algum cliente reclamar, registrar o caso.
    4. A reescrita ordena as chaves do objeto quando muda algo (o Ollama não depende da ordem). O corpo intocado segue idêntico.
    5. O modelo `<base>-think` **não existe no Ollama**: `/api/tags` e `/v1/models` não o listam. Se algum cliente validar o nome contra a lista, não vai achá-lo; documentar na 9.6b (nginx/implantação).
    6. `think` em `/api/embed` e o sufixo em embedding **não são aplicados** (por desenho); o rótulo fica vazio.
    7. Herdadas: pendências 1, 3 a 9 da 9.5 continuam como estavam (a 2 foi resolvida aqui: o gateway agora policia `options` e `keep_alive`).
- 2026-10-30 12:20 — **9.6 IMPLANTAÇÃO CONCLUÍDA** (loop principal, com autorização do owner: "prossiga com as 1, 2 e 3").
  - **Decisões do owner (2026-10-29/30):**
    - `--priority-ips` = toda a LAN `10.100.2.0/24`;
    - senha digitada pelo owner;
    - políticas antes de instalar, numa instalação única (9.6a, `6d51d48`);
    - fechar a porta 11434. Uma máquina `…da01::4` chamava a porta direto, sem token: 541 `/api/generate` em 27–28/10.
  - **Instalador e pacote:** `deploy/fase9-gateway/`, com `Instalar-Gateway.ps1`, etapas 1/2, rollback, script de senha e `MANUAL.md` (15 seções, 31 flags conferidas).
    - Commits `2cb599d`, `7e47707` e `f2d0162`, mais a correção da corrida do reload neste registro.
  - **O que deu certo:**
    - etapa 1 (usuário `mcpgw`, pastas, binário `0.9.0-2cb599d` com SHA-256 conferido);
    - integridade provada por 3 recusas: arquivo adulterado, hash errado e pasta 0755;
    - etapa 2 às 12:12:40: gateway `active`/`enabled`, Ollama em `127.0.0.1:11434`, nginx → 11500 com `nginx -t` ok.
    - **Conferência pela estação 10.100.2.30:**
      - `10.100.2.2:11434` → sem conexão (porta fechada); sem token → 401; `/api/pull` → 403; `/monitor` → 200;
      - `CF-Connecting-IP: 203.0.113.77` forjado da LAN → registrado como `10.100.2.30` (correção do nginx funcionando);
      - `/v1` padrão → "pong", 2 tokens, sem raciocínio; `qwen3-ia-think` → raciocínio de 687 caracteres;
      - a tool MCP `generate` desta sessão passou pela fila (`queue_ms` 0);
      - histórico do SQLite com `client_ip`, `label`, `think` e `status`.
  - **O que não deu certo, e as soluções:**
    1. **N1:** os scripts gravavam a senha com `0640`, e o gateway recusa (`admin.go`: só 0600). Corrigido para `mcpgw:mcpgw 0600` e provado no servidor (0640 → código 2; 0600 → `/monitor` 200).
    2. **Senha:** o owner digitou 10 caracteres e o script exige 12, então recusou ("senha curta"). O script passou a mostrar a contagem.
       - O owner deixou a senha em `/root/mcpgw_password.txt` (0600). O hash foi gerado desse arquivo **sem exibir a senha**.
       - O gateway aceita a partir de 8. Recomendação: trocar por uma de 12 ou mais (`-Modo Senha`) e apagar o `.txt`.
    3. **Corrida do reload:** o 1º teste da etapa 2 caiu num worker antigo do nginx, ainda direto ao Ollama (resposta com raciocínio e sem headers da fila), e o `grep` encerrou o script **depois** da ativação, sem rollback.
       - Os workers novos começaram às 12:13:02; nas conferências seguintes tudo passou pela fila.
       - Solução: `sleep 5` depois do reload, e o `grep` dos headers deixa de ser fatal.
    4. **Erro meu de processo:** o instalador ficou parado pedindo a senha nesta sessão sem terminal. Encerrei com `TaskStop` às 11:31, sem efeito no servidor.
  - **Backups:** `/root/srv-backup-20261028/fase96-ativacao-20261030-121239`. **Rollback:** `bash <pasta>/rollback.sh <pasta>`.
  - **Pendências:**
    1. 9.7: aceite ao vivo, com 2 → 5 → 10 agentes, ações no `/monitor` (login, promover, remover, cancelar) e a página aberta num navegador real.
    2. Trocar a senha por uma de 12 ou mais e apagar `/root/mcpgw_password.txt`.
    3. Identificar a máquina `2804:2904:706:da01::4`, que perdeu o acesso direto e deve passar a usar :8443 com token.
    4. As extensões §11 (faixa CPU e pool) continuam planejadas.
- 2026-10-30 12:25 — **9.6, ajustes depois da implantação** (loop principal).
  1. **`/monitor` sem estilo e parado em "conectando…"** (visto pelo owner no navegador): o CSP com hash bloqueava o script e o estilo.
     - Causa: o binário foi gerado de uma cópia de trabalho com `index.html` em CRLF. O gateway fazia o hash dos bytes brutos, mas o navegador faz o hash do texto já convertido para LF.
     - Correção `7a86350`: o HTML embutido é normalizado para LF antes do CSP e de servir. Regressão em `csp_newline_test.go` (hash calculado como o navegador). Provado com uma entrada CRLF: o código antigo não casa e o novo casa.
     - Gateway `0.9.1-7a86350` instalado pelo instalador (`-Modo Atualizar`). No servidor, os 2 hashes conferem.
     - Conferido no Chrome do owner: página renderizada, com dados ao vivo.
  2. **Instalador:** no `-Modo Atualizar`, o PowerShell 5.1 removia as aspas duplas do comando remoto e o `curl -w` se desfazia. Corrigido em `1e5e70c`.
  3. **Senha do admin trocada pelo owner** (15 caracteres, no arquivo `/root/mcpgw_password.txt`):
     - hash gravado sem exibir a senha, com backup do hash anterior; gateway reiniciado;
     - login certo → 200; senha errada → 401;
     - **o owner decidiu manter o arquivo em texto puro** (root 0600).
  4. **Erro meu de processo:** um `cat >>` sem entrada travou um comando por 2 min e criou um arquivo vazio em `.workspace/plans/`. Apagado e refeito; sem efeito no servidor.
  - **Pendências:**
    1. 9.7: aceite com carga, mais as ações do `/monitor` feitas pelo owner no navegador.
    2. Identificar a máquina `2804:2904:706:da01::4`.
    3. Opcional: importar a CA do Cloudflare nas estações e usar o nome `ia.cslsolucoes.com.br` em vez do IP, para tirar o aviso "Não seguro".
- 2026-10-30 12:40 — **Conferência do NAT (MK-ESCRITORIO-02) e do Cloudflare, pedida pelo owner** (loop principal; só leitura no roteador até a decisão).
  - **Achado grave, corrigido (`2a16888`):** o `/monitor` respondia 200 pela internet.
    - Causa: o túnel `cloudflared` entrega como `::1`, e a `location /monitor` liberava o loopback, o que viola a D9-1.
    - Pelo mesmo motivo, todos os clientes do túnel apareciam para a fila como `::1`.
    - Correção: 403 em `/monitor` para qualquer pedido com `CF-Connecting-IP`, e o loopback fora da LAN no `geo`.
    - Prova: internet `/monitor` 200 → 403; `/api/tags` com token pela internet 200; LAN 200. Backup do nginx no servidor.
  - **NAT regra 0** (WAN:8443 → servidor, aberta a todos, usada só por varredores que recebiam 400/401 do nginx): o owner decidiu **restringir a `PERMITIDOS`**. Aplicado às 12:38, com o export do NAT salvo antes.
  - **O IPv6 `…da01::4` é a TV LG Bancada** (`10.100.2.55`).
  - **Pendências:**
    1. Atualizar o `cloudflared` (2026.10.3 → 2026.10.6).
    2. O owner verificar o app de IA na TV LG.
    3. Endurecimento opcional do nginx: responder 403 antes do 401 para quem está fora da LAN e do Cloudflare.
