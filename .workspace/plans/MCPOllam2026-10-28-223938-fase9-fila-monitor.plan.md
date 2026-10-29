# Plano — Fase 9: fila de espera na retaguarda (gateway) e monitor `/monitor` com SQLite

> **Estado:** **DECIDIDO, desenvolvimento 9.1–9.5 a iniciar** (2026-10-28 22:44). A implantação (9.6) só vem depois da 8.9 da Fase 8.
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

## 11. Changelog

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
