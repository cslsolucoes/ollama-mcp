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
