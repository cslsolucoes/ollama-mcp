# Relatório — Fase 11 reestruturada: entendimento completo para revisão por outra IA

| Campo | Valor |
| --- | --- |
| Data/hora | 2026-11-04 17:59 (hora local, America/Sao_Paulo) |
| Autor | Claude Opus 5.5 (Claude Code 2.1.296, extensão no Cursor), sessão de coordenação do projeto mcpollama |
| Pedido do owner | "crie um report com todo esse bate-papo para pedir uma segunda avaliação de outra IA, com todos os detalhes ('até mesmo se é verde ou roxo'), detalhamento máximo do seu entendimento primeiro, antes de fazer o plano" |
| Estado | **Entendimento, duas avaliações externas e o tratamento (atualizado em 2026-11-05 12:00).** Nenhum código nem mudança no servidor por causa deste assunto. A Fase 11 continua **congelada** (desde 2026-10-30 23:50). O plano **já foi reescrito** (2026-11-04 18:25, atualizado em 2026-11-05) no próprio arquivo congelado; o que vale hoje está na §0, na §15 e no plano. Pendentes: aprovação 11.0 e prova 11.1b |
| Plano antigo (a ser reescrito) | `.workspace/plans/congelados/MCPOllam2026-10-30-224500-fase11-websocket-jobs-assincronos.plan.md` |
| Spike já feito | `.workspace/reports/MCPOllam2026-10-30-234600-fase11-1-spike-espera-longa.report.md` (commit `f8c723e`) |

## 0. Como usar este relatório (para a IA revisora)

> **Situação em 2026-11-05 11:40 (leia primeiro).** Este documento tem três camadas, nesta ordem de validade:
> 1. **§15 (resposta do coordenador, decisões do owner e correções) e o plano reescrito** (`.workspace/plans/congelados/MCPOllam2026-10-30-224500-fase11-websocket-jobs-assincronos.plan.md`) — **é o que vale hoje**;
> 2. **§14 (1ª avaliação, Codex, 2026-11-04 18:11)** — as recomendações dela já foram decididas ou incorporadas na §15;
> 3. **§1–§13 (entendimento original de 2026-11-04 17:59)** — histórico; as §7–§9 estão marcadas como **superadas em parte** e os trechos errados com ⛔.
> A 2ª avaliação (GPT-6, 2026-11-05 10:25) e o tratamento dela estão na §15.7.

1. A §1 a §4 descrevem **o sistema como ele era em 2026-11-04 17:59** (fotografia daquele momento, não do estado atual), com números medidos e caminhos de código. A §5 resume a Fase 11 original
   (congelada). A §6 é a **cronologia literal da conversa** de 2026-11-04 (16:57–17:51) que originou a reestruturação. A §7 é o
   **desenho proposto**. A §8 compara o desenho novo com as decisões antigas (D11-x) e aponta **conflitos**. A §9 lista as
   **decisões em aberto**. A §10 traz riscos; a §11 o que **não foi verificado**; a §12 as **perguntas específicas** para você.
2. Marcação de confiança usada no texto: **[medido]** = executado e lido nesta sessão ou em relatório citado; **[código]** = lido no
   fonte do repositório hoje; **[doc]** = documentação do fornecedor, não testado; **[estimativa]** = conta/inferência minha;
   **[não verificado]** = hipótese.
3. Critério do owner que deve guiar a revisão: **qualidade e economia de IA paga acima de tempo de execução** (owner, 2026-11-04
   15:19: "o tempo de execução para mim não tem relevância alta, o foco é economizar com outras IA pagas; se necessitar de algo mais
   urgente uso as IA's pagas"). E **escopo estrito**: nada além do pedido sem perguntar.
4. A avaliação independente do Codex foi acrescentada na §14 em 2026-11-04 18:11. Ela não altera decisões do owner, não substitui
   os dados brutos do bake-off e não reescreve o plano congelado.

## 1. Contexto do sistema (estado em 2026-11-04 17:59)

### 1.1 Componentes e caminho de um pedido

```
Claude Code (extensão no Cursor; o owner NÃO usa a IA do Cursor)
   │ stdio, JSON-RPC 2.0 (MCP)
mcpollama-mcp.exe 0.10.0-5372f4c  (D:\MCP, na estação Windows; Go; sem SDK MCP externo)
   │ HTTPS + Authorization: Bearer; cabeçalho X-Mcpollama-Station (máquina, usuário, domínio, MAC — declarados, não autenticados)
nginx :8443 (LAN, TLS Cloudflare Origin CA)  |  Cloudflare Tunnel (internet, mesmo host sem porta; a partir da LAN dá timeout: split-DNS)
   │ X-Real-IP = $ollama_client_ip (remote_addr na LAN, CF-Connecting-IP no túnel); limit_conn 80/IP, burst 100
mcpollama-gateway 0.13.1-8f9cf6a  (127.0.0.1:11500; fila, /monitor, pool de backends, log de jobs, Fase 12 treino)
   │
Ollama 0.40.13 (somente 127.0.0.1:11434)
```

- Servidor: `ia.cslsolucoes.com.br` = 10.100.2.2, CentOS (`/dev/mapper/cs-root`, 217 GB, **173 GB livres**) [medido].
- GPUs: **4× NVIDIA GeForce RTX 3070, 8 GB cada (32 GB no total)**, limitadas a **100 W cada** e com clock travado em 210–1400 MHz,
  porque a **fonte de 850 W não aguenta as 4 placas** (causa de reinícios; owner 2026-10-28). Picos somados medidos hoje: 202–239 W
  [medido].
- Clientes conhecidos: Claude Code (owner e sessões do projeto), Codex (`Treinamento-Codex`, 10.100.2.30), `ia-local.ps1`
  (OpenClaude, via `/v1`).
- Repositório: GitHub privado `cslsolucoes/ollama-mcp`. Projeto em `E:\MCPOllam`; fontes em `src/`.

### 1.2 Configuração atual relevante [medido/código]

| Item | Valor | Fonte |
| --- | --- | --- |
| `OLLAMA_NUM_PARALLEL` | 1 (desde 14:46 de hoje; passou por 2, 5 e 3 ao longo do dia) | `/etc/systemd/system/ollama.service.d/10-performance.conf` |
| `OLLAMA_MAX_LOADED_MODELS` | **3 no servidor**, 2 na cópia do repositório (divergência pendente) | idem / `deploy/servidor-ia/ollama-10-performance.conf` |
| `OLLAMA_CONTEXT_LENGTH` | 65536 | idem |
| `OLLAMA_KEEP_ALIVE` | -1 (modelo carregado não expira) | idem |
| Gateway `--gen-slots` | **1** (um pedido por vez entra no Ollama; ordem do owner 14:45 "coloque somente 1 por vez") | unit `mcpollama-gateway.service` |
| Gateway `--run-timeout` | 10 min | unit |
| Gateway `--max-wait` | 10 min (padrão; espera na fila, depois 504 `expirado`) | `src/cmd/mcpollama-gateway/config.go:38` |
| Fila | 64 por cliente/IP e 128 no total (produção); prioridade + idade (`--age-after`) | CLAUDE.md do projeto |
| Gateway `--allowed-models` | `qwen3-ia;nomic-embed-text` (hoje **temporariamente** com o gemma4 em teste, ver §1.4) | unit |
| Gateway remove do cliente | `keep_alive`, `num_ctx`, `num_gpu` etc. (salvo `--allow-client-num-ctx`/`--allow-client-keepalive`) | `src/gateway/proxy/policy.go:13,277` |
| mcpollama `--timeout` | **120 s** por chamada (fila + execução) | `src/mcpollama-mcp/config.go` |
| mcpollama fallback | `--ollama-url-fallback` desligado; tentado 1 vez em erro de conexão ou 503 de fila cheia, **não em timeout** | CLAUDE.md |
| `think` | desligado por padrão; liga por argumento `think` nas tools ou pelo nome virtual `qwen3-ia-think` | CLAUDE.md |

### 1.3 Bancos do gateway (`/var/lib/mcpollama-gateway/`, dono `mcpgw`) [código]

- **`gateway.db`** — metadados, sem texto. Tabela `requests` (`src/gateway/store/store.go:39`):
  `id, client_ip, client_label, tool, endpoint, model, lane, priority_in, priority_final, bytes_in, enqueued_at, started_at,
  finished_at, wait_ms, run_ms, status, http_status, error_class, prompt_eval_count, eval_count, load_ms, boot_id, think,
  responded, resp_bytes, backend, failover_rule, station_computer, station_user, station_domain, station_mac, doc_hash, doc_key_id`.
  Outras tabelas: `audit`, `gpu_samples`, `job_gpu_samples`, `cpu_samples`, `backend_samples`, `events`, `meta`.
- **Valores de `status`** (`src/gateway/queue/queue.go:105–121`): `ok`, `erro`, `cancelado` (o cliente desistiu), `cancelado_admin`,
  `removido` (admin tirou da fila), `rejeitado` (fila cheia), `expirado` (`--max-wait`), `na_fila`, `em_execucao` (estados ao vivo
  do log 10.L), `interrompido_reboot` (boot_id mudou), `interrompido_gateway` (processo morreu, mesmo boot).
- **`content.db`** (Fase 10.L, `src/gateway/content/content.go`) — texto completo de cada job, arquivo **0600**, servido no monitor
  **só ao admin logado**. Tabela `job_content(job_id PK, created_at, updated_at, prompt BLOB, prompt_bytes, prompt_sha256,
  prompt_truncated, response BLOB, resp_bytes, resp_truncated, resp_final, prompt_json BLOB, prompt_json_truncated, tem_imagem)`.
  Limites: **4 GiB no total** (acima, apaga os mais antigos e emite evento), **16 MiB por parte** (acima, corta e marca truncado),
  **guarda de 7 dias**, gravação `synchronous=FULL` com flush da resposta a cada 2 s durante o stream (sobrevive a queda de energia).
  O texto está em **BLOB** (bytes UTF-8), não `TEXT`.
- **`treino.db`** (Fase 12) — ocorrências de erro do modelo relatadas (`reportar_erro`), amostras, histórico, retratos, embeddings,
  cotas, índices; chave HMAC em `/etc/mcpollama-gateway/doc.key`.
- Monitor (`/monitor`, **só LAN**; pelo túnel devolve 403): GET livres, POST exigem sessão admin + CSRF. Rotas de job:
  `GET /monitor/api/jobs/{id}` (dados do job), `GET /monitor/api/jobs/{id}/content` (texto; só admin), `GET /monitor/api/jobs/export`,
  `POST /monitor/api/jobs/{id}/marcar-erro`; estado ao vivo em `/monitor/api/state` e `/monitor/api/stream` (SSE) [código].

### 1.4 Situação transitória do servidor neste momento (para a revisora não se confundir)

- Está em curso um **bake-off de modelos para a Fase 13** (escolha de modelo), autorizado pelo owner. Para isso:
  - o `qwen3-ia` (alias de `qwen3.5:9b`, o modelo de produção) e o `gemma4:12b` foram **apagados do disco** por ordem do owner
    ("só deixar os que serão usados agora nos testes"); **a produção está sem o `qwen3-ia`** até a restauração (estações recebem erro);
  - o gateway aceita temporariamente o modelo em teste (`--allowed-models` com `gemma4:31b`); backup da unit em
    `/root/stress13/unit.bak-g13`, restauração por `/root/stress13/g13off.sh`;
  - modelos no disco agora: `gemma4:31b` (20 GB) e `nomic-embed-text`.
- Correção pronta e **não implantada**: gateway 0.13.2 (falso alarme "degradado (fora da GPU)" com o gemma4:26b — `/api/ps` conta um
  buffer `CUDA_Host` de ~0,45–0,6 GiB em `size` e não em `size_vram`, com 31/31 camadas na GPU). Regra nova em
  `src/gateway/models/models.go` `Processor`: diferença ≤ 1 GiB **e** ≤ 25% do tamanho = "100% GPU". Testes de ataque das revisões
  9, 10.5 e 12 passaram (138 PASS, 0 FAIL; log `.workspace/reports/MCPOllam2026-11-04-1645-ataques-gateway-0.13.2.log`).
- O `ollama-warm` falhou no último boot (16:34:37) porque tenta carregar o `qwen3-ia`, que não existe agora.

## 2. Prioridades e regras do owner que se aplicam ao desenho

1. **Qualidade/economia acima do tempo** (§0.3). Consequência direta: raciocínio (`think`) e esperas longas são aceitáveis; o que não
   pode é a IA paga ter de refazer trabalho porque o local errou ou expirou.
2. **Cliente-alvo: Claude Code** (owner 17:26: "só uso o cursor com o Claude Code, não uso a IA do cursor"). Codex é secundário.
3. **Dados de cliente não saem da rede local** (o servidor é self-hosted; documentos de clientes nunca para git/chat/logs externos).
4. **Escopo estrito** (regra global 04/11): "revisar/ver/analisar" é somente leitura; ir além exige pergunta detalhada.
5. **Rigor empírico**: nada é "funciona" sem rodar e ler; 2ª via independente.
6. **Reusar o que existe (SSOT)**, não duplicar mecanismos.
7. **Commit por caminho**, varredura de segredos bloqueando, sem dependência de terceiros sem decisão.
8. O mcpollama **não grava em disco** salvo flag (regra do projeto); stdout só JSON-RPC.

## 3. Problema que motivou a conversa (evidência de hoje)

### 3.1 A fila é síncrona e o timeout corta trabalho [código + medido]

- Fluxo atual: a tool do mcpollama abre um HTTP para o gateway e **fica esperando**; o gateway enfileira; com `--gen-slots 1` só um job
  roda no Ollama; ao terminar, a resposta volta **pela mesma conexão** à IA que pediu. Não há "pegar depois".
- Se a IA desiste (timeout de 120 s do mcpollama, ou cancelamento), a conexão cai e o gateway **cancela o job** (sai da fila, ou o
  pedido ao Ollama é cancelado; nginx registra 499; status `cancelado`). **O trabalho feito se perde.** Não sobra resultado "órfão".
- Não há nova tentativa automática em timeout. Pela regra global do owner, erro/timeout no nível local → **sobe para IA paga
  (haiku)**, exatamente o custo que se quer evitar.

### 3.2 Medição de hoje que mostra o efeito [medido]

Carga de 10 agentes × 3 tarefas (S2 numeração / S3 JSON alternados), todos pelo gateway com `--gen-slots 1`, modelo `gemma4:26b`,
timeout de 300 s por chamada no bake-off (`-server-timeout 300s`):

- `ok=25/30`, **`timeout=5`** (cada um parou em ~300.015 ms), p50 = **228,2 s**, p95 = 276,6 s, máx. = 279,2 s,
  janela 744,2 s, 111.996 tokens de entrada e 11.401 gerados (`out-f13-26b/run-gemma4_26b-multi.log`).
- Entre as tarefas que terminaram, qualidade S2 = 100% (176/176 itens; os 2 S2 que faltam para 220 são os que expiraram).
- Ou seja: com 10 agentes e jobs de ~40 s cada, **a espera na fila sozinha passa de 120 s** (o padrão de produção). Com `think`
  ligado (10–30× mais lento [medido na Fase 8]), seria pior.

### 3.3 Contexto da Fase 13 (escolha de modelo) que interage com a Fase 11 [medido hoje]

Bake-off de qualidade, `think:false`, contexto 64k (qwen) / 32k (gemma4 em dupla), `NUM_PARALLEL=1`:

| Teste | qwen3-ia (qwen3.5:9b) | gemma4:e4b (dupla) | gemma4:12b (dupla) | gemma4:26b (sozinho) | gemma4:31b |
| --- | --- | --- | --- | --- | --- |
| S2 números / número+título | 86,4% / 84,5–85,5% | 86,4% / 81,8% | **100% / 100%** | **100% / 100%** | em curso |
| S3 extração | 100% | 94,4% (1 JSON inválido) | 100% | 100% | em curso |
| S6 conta direta | 0/15 | 3/15 | 6/15 | **9/15** | em curso |
| S6 conta passo a passo | 10/10 | 10/10 | 10/10 | 10/10 | em curso |
| S6 contagem direta / por lista | 3/6 · 3/6 | 0/6 · 3/6 | 4/6 · 3/6 | 3/6 · 4/6 | em curso |

- Observações: os gemma4 devolvem JSON dentro de cerca de código (S2 estrito 0/5); os três modelos "inventam" os mesmos itens
  `1.2, 1.2.1–1.2.3` no S2 (suspeita de gabarito incompleto, **[não verificado]**); o passo a passo acerta 10/10 em todos — sinal de que
  **raciocínio** melhora conta/contagem, que é onde todos erram.
- Tabelas de **tokens por job** (pedido do owner hoje): `.workspace/bakeoff/out-f13-{e4b,12b,26b}/tokens-por-job.md`.
- Relevância para a Fase 11: se a Fase 13 escolher um modelo mais lento (26b/31b) e/ou ligar `think`, **o tempo por job sobe** e o
  modelo síncrono com 120 s fica inviável; o assíncrono é o que torna "qualidade acima do tempo" praticável.

## 4. O que o Claude Code oferece hoje (relevante ao desenho)

- **[medido nesta sessão]** Comandos `Bash` com `run_in_background` e **subagentes em segundo plano** notificam o modelo quando
  terminam; o modelo é **reativado sozinho** com o resultado (foi assim que os bake-offs de 10–40 min e o subagente dos testes de
  ataque avisaram hoje, sem o owner pedir). A espera em si **não consome tokens** (o modelo não roda enquanto a ferramenta roda).
- **[medido no spike 11.1, `claude -p`, 2.1.296]** Tool MCP que bloqueia 170 s e 300 s volta normal com o resultado (sem
  auto-background em `-p`). Com `CLAUDE_AUTO_BACKGROUND_TASKS=1`, aos ~120 s a chamada MCP vai para segundo plano e o modelo recebe
  "você receberá uma notificação"; **se essa notificação acorda o modelo no uso interativo não foi provado** (o `-p` terminou antes).
- **[doc, 2026-10-30]** No interativo, chamada MCP > 2 min vai para segundo plano; `CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS` muda o limite
  (0 desliga); a tarefa não sobrevive ao fim da sessão; ocioso de 30 min para stdio; `MCP_TOOL_TIMEOUT` padrão ~28 h. MCP *tasks*
  assíncronas do protocolo exigem Claude Code **≥ 2.1.304**; instalado hoje: **2.1.296** [medido 17:58].
- **[medido]** Hooks do Claude Code já em uso pelo owner: `UserPromptSubmit` injeta data/hora no contexto a cada mensagem;
  `SessionStart` injeta contexto no início da sessão. São o mecanismo para "avisar lotes prontos" sem custo de modelo.
- Avisos MCP servidor→cliente (`notifications/progress`, `notifications/message`, `resources/updated`, `tools/list_changed`) chegam
  ao **cliente**, não fazem o **modelo** iniciar uma rodada [doc + entendimento; não testado no 2.1.296].

## 5. A Fase 11 original (congelada em 2026-10-30 23:50) — resumo fiel

- Pedido original do owner (22:40 de 30/10): "um websocket para ter comunicação bidirecional, sendo o servidor da IA o servidor e a
  estação o MCP; o MCP não fica esperando o response; quando a IA finalizar o job, envia para a estação solicitante".
- Decisões do owner: **D11-1** tools enviar/resultado; **D11-2** guardar e entregar ao voltar (não cancela na queda); **D11-3** só o
  mcpollama usa WebSocket; **D11-4** LAN e internet, mesmo Bearer; **D11-5** resultado de job não terminado responde na hora com o
  estado (opcional esperar N s); **D11-6** **todas** as tools viram assíncronas (`generate`, `chat`, `summarize_file`, `extract`,
  `translate`, `embed`; **D11-9** embed também); **D11-7** quem busca = mesma máquina/usuário + ID.
- Técnicas: **D11-8** `list_models` síncrona; **D11-10** biblioteca `github.com/coder/websocket` (nova dependência, registrar no NOTICE);
  **D11-11** segredo aleatório por job, servidor guarda só o hash, segredo em `%LOCALAPPDATA%\mcpollama\jobs.json` (ACL do usuário);
  **D11-12** resultado guardado **30 min** após o fim para entrega (memória + SQLite), entregue = apagado da área de entrega;
  **D11-13** mesma fila do gateway; **D11-14** sem túnel, fallback HTTP em segundo plano local com o mesmo ID; **D11-15** map-reduce
  como **job composto local** na estação; **D11-16** servidor empurra o `result` assim que conclui; **D11-17** toda resposta de tool
  leva "jobs concluídos aguardando leitura: N"; **D11-18** notificação do Windows (toast via PowerShell/WinRT, sem conteúdo);
  **D11-19** `aguardar_jobs(ids[], timeout_s)` com ciclo (owner pediu 5 min; spike recomendou **100 s, máx. 110 s** para voltar antes
  do auto-background de 120 s — **decisão pendente**); **D11-20** fallback HTTP com garantia menor; **D11-21** limpeza do `jobs.json`;
  **D11-22** composto interrompido = "reenvie"; **D11-23** entrega "pelo menos uma vez", `ack_result` quando o Claude lê, reenvio na
  reconexão, idempotência por ID, **nenhum conteúdo em disco na estação**; **D11-24** cancelar composto; **D11-25** fallback com no
  máximo 2 HTTP simultâneos por processo e 10 min por parte; **D11-26** cancelamento idempotente; **D11-27** esperas sobre o registro
  local; **D11-28** `listar_jobs` só pendentes/não lidos, máx. 20; **D11-29** justiça por IP real + rótulo também no WebSocket.
- Protocolo rascunhado: `wss://…:8443/api/ws` e `wss://…/api/ws`; mensagens `submit/status/resume/cancel/ack_result/ping` e
  `accepted/rejected/progress/result/cancel_result/pong`; ping a cada 25 s (Cloudflare corta WS ocioso ~100 s); backoff 1→30 s.
- Tools previstas: as atuais devolvem `{job_id, estado, posicao}`; novas `resultado_job(job_id, esperar_s≤60)`,
  `aguardar_jobs(job_ids[], timeout_s)`, `listar_jobs()`, `cancelar_job(job_id)`.
- Versões-alvo de então: gateway 0.13.0 e mcpollama 0.10.0 — **esses números já foram usados pela Fase 12**; a Fase 11 precisará de
  novos (ex.: gateway 0.14.0, mcpollama 0.11.0).
- Revisões externas anteriores: duas rodadas de outra IA (pontos 1–11) já incorporadas (D11-19…D11-29).

## 6. Cronologia literal da conversa de 2026-11-04 (16:57–17:51)

> Formato: **pergunta do owner** (hora) → resumo fiel da minha resposta. Todas as respostas foram "só conversa, sem ação".

1. **(16:56)** "qual será o comportamento da fila agora, a IA envia o job, e a próxima assim que concluir disponibiliza o resultado
   para a IA que enviou pegar?" → Não há "pegar depois": fila síncrona, resposta pela mesma conexão; `--gen-slots 1`; a IA fica
   bloqueada; se desistir, o job é descartado; o `--timeout` de 120 s pesa; sugeri subir o `--timeout` nas estações; o assíncrono era a
   Fase 11.
2. **(16:57)** "e no caso de timeout?" → Tabela dos três limites (`--timeout` 120 s do mcpollama; `--max-wait` 10 min → 504
   `expirado`; `--run-timeout` 10 min); quem vence primeiro é o de 120 s; sem retry; regra global manda subir para haiku; sugestão de
   `--timeout` ~600 s, ressalvando que não medi esperas de 5–10 min no cliente.
3. **(16:58)** "não tem como colocar um webhook para deixar assíncrono?" → Webhook não alcança a IA (stdio; estação atrás de NAT;
   túnel só de ida); o que serve é job + consulta (Fase 11); webhook como opcional para integrações com servidor (n8n, ERP);
   alternativa barata só para timeout: avisos de progresso MCP (não sei se o Claude Code reinicia o timeout com eles).
4. **(17:07)** "e websocket, resolveria?" → Sim, resolve o transporte (conexão parte da estação; passa NAT e Cloudflare; sem HTTP
   aberto esperando), mas a IA continua sem ser "acordada": o resultado chega ao mcpollama e a IA pega com `job_resultado(id)`; é o
   desenho da Fase 11; cuidado do keepalive < 100 s.
5. **(17:07)** "e quanto ao raciocínio?" → Pelo critério do owner vale testar; ganho provável em conta/contagem; custos: 10–30× tempo,
   ~100× tokens gerados (do servidor), segura a fila de 1 slot, risco de resposta vazia se o raciocínio esgotar `num_predict` (Fase 8:
   gemma4, 300 s e vazio); o bake-off **não tem** opção `-think` hoje [código: nenhum "think" em `cmd/bakeoff/main.go`/`suites.go`].
6. **(17:18)** "tem alguma forma de viabilizar? o MCP ou um agente notificar a IA? hoje uso o cursor" → Nem MCP nem agente "acorda" a
   IA do Cursor; opções: (1) enviar + consulta com espera curta (universal); (2) subagente que espera (Claude Code); (3) processo em
   segundo plano (Claude Code); MCP *tasks* exigem cliente com suporte.
7. **(17:26)** "só uso o cursor com o Claude Code, não uso a IA do cursor" → Então (2) e (3) funcionam; desenho: `job_enviar` →
   espera em segundo plano → aviso → resultado. Guardado na memória.
8. **(17:29)** "deixando um agente monitorando a cada 60 s consome token enquanto ele estiver ativo?" → Sim se for o **modelo**
   consultando (uma rodada por consulta, contexto relido, quase todo em cache); não se for uma **ferramenta** bloqueando; tabela:
   agente consultando = tokens por consulta; subagente com 1 espera bloqueante = só início e fim; **processo em segundo plano + aviso = zero
   até o aviso** (recomendado).
9. **(17:36)** "hoje com os ID do job, consigo consultar o status e a resposta?" → Em parte: só pelo monitor (LAN; texto só admin);
   não serve como assíncrono porque não há tool MCP, o job é cancelado se a IA sai, não há controle de dono, e só LAN.
10. **(17:38)** "como uma tool do MCP?" → Esboço: `job_enviar` (ou `assincrono:true` nas tools atuais), `job_resultado(id,
    aguardar_s)`, `job_cancelar`; CLI `mcpollama-mcp.exe --esperar <id>` para espera sem tokens; no gateway: job desacoplado,
    `/api/jobs/{id}` só para o dono, espera longa, WebSocket opcional; ponto difícil: map-reduce hoje é feito no mcpollama — propus
    movê-lo para o gateway (opção b); segurança: dono, 7 dias, limites, revisão adversarial.
11. **(17:39)** "então posso enviar um lote de jobs e depois buscar os resultados de cada job mesmo no dia seguinte?" → Sim, com
    quatro peças: achar os jobs no dia seguinte (`lote_enviar` com ID de lote, `job_listar` por data/rótulo/situação, rótulo livre);
    **fila persistente em disco** (hoje a fila é só memória; reinício perde os pendentes) com re-enfileiramento; **lote em prioridade
    baixa** com limite próprio; prazo de guarda configurável, só o dono lê; texto lido no envio; economia máxima (IA paga só envia e
    confere).
12. **(17:41)** "aí no dia seguinte, fala para a IA ver o resultado do lote, certo?" → Sim; e um **hook `SessionStart`** pode rodar
    `mcpollama-mcp.exe --lotes-prontos` e injetar "2 lotes prontos: …" no contexto, sem a IA rodar.
13. **(17:42)** "onde armazenaremos as informações do lote?" → No **servidor**, banco novo **`lotes.db`** (0600, `mcpgw`), ao lado de
    `gateway.db`/`content.db`/`treino.db`; campos de lote (ID, rótulo, dono por hash do token + estação/usuário, datas, prazo,
    contadores) e de job (ID, lote, ordem, situação, tool, modelo, `think`, entrada, resultado, `[uso]`, erro, tempos); banco próprio
    porque guarda e acesso diferem do `content.db`; texto duplicado conscientemente; 5–20 MB por lote de 200; limpeza automática;
    backup é decisão do owner.
14. **(17:43)** "hoje todos os jobs estão armazenados sem distinção de concluiu/cancelou/erro?" → Não: lista dos `status` (§1.3);
    texto fica no `content.db`; para lotes entrariam `pronto_nao_buscado` e `buscado`, e `interrompido_*` voltariam à fila.
15. **(17:46)** "conseguimos implementar esse histórico completo, inclusive com o contexto enviado e o resultado em campos no formato
    Text?" → Sim, boa parte existe (`job_content`); falta `TEXT` em vez de `BLOB` (ou view com `CAST`), juntar `requests` + `job_content`
    numa consulta/exportação sem fundir os arquivos (separação decidida em 30/10), e rever prazo/limites (7 dias, 4 GiB, 16 MiB).
16. **(17:47)** "sem perder nenhuma informação, log de 100% do que entra e 100% do que sai com dados de projeto/sessão/usuário/modelo…"
    → Tabela de 9 lacunas (corte por tamanho; apagamento automático; pedido original da IA não chega ao gateway; partes de map-reduce
    soltas; resultado final entregue à IA não é visto pelo gateway; raciocínio não registrado; recusados sem texto; imagens só com
    marcador; sem backup fora do servidor) e o conteúdo de cada registro (quem, o quê, entrada, saída, como foi); um registro por
    chamada da IA com jobs filhos; volume estimado; decisões de LGPD e prova de não adulteração.
17. **(17:49)** "Imagens (OCR) armazenando os metadados no servidor" → **Decisão do owner:** imagens **só metadados** no servidor;
    lista (SHA-256 do arquivo e por página, nome/caminho/tamanho, tipo/páginas totais/lidas, dimensões/DPI/megapixels, motor e versão
    Tesseract 5.3.3 + idioma + tessdata, confiança média e tempo por página, texto extraído por página, se a imagem foi direto ao
    modelo); o OCR roda na estação, então o mcpollama manda os metadados no envelope; limite: sem a imagem não se refaz OCR só no
    servidor.
18. **(17:51)** "dá para reestruturar a fase 11 com esse nosso novo bate-papo?" → Sim, propus as etapas da §7.12. **Owner:**
    "reescreve o antigo atualizando data e hora" + "crie um report com todo esse bate-papo para pedir uma segunda avaliação de outra IA
    … antes de fazer o plano me passe o nome do report".

## 7. Desenho proposto (meu entendimento completo)

> ⚠️ **SUPERADO EM PARTE (2026-11-05).** Esta seção é o **entendimento original de 2026-11-04 17:59, mantido como histórico**. Onde ela diverge das §15.2–§15.7 e do plano reescrito, **valem a §15 e o plano**. Trechos sabidamente superados estão marcados com ⛔.

### 7.1 Objetivo da Fase 11 reestruturada

**"Jobs assíncronos, lotes e registro integral"**: a IA paga (Claude Code) envia trabalho ao modelo local **sem ficar bloqueada e
sem timeout**, pode enviar **lotes** e buscar no dia seguinte, é **avisada sem gastar tokens na espera**, e **100% do que entra e
sai** fica registrado com metadados completos, sem perda. O WebSocket é o **transporte**, não o objetivo.

### 7.2 Contrato com a IA (tools do mcpollama)

| Tool | Entrada | Saída | Observação |
| --- | --- | --- | --- |
| `job_enviar` **ou** `assincrono:true` nas tools atuais | o mesmo pedido de `generate/chat/summarize_file/extract/translate/embed`, + `think`, `prioridade`, `rotulo` | **na hora**: `job_id`, posição, espera estimada | escolha em aberto (§9, decisão 1) |
| `job_resultado` | `job_id`, `aguardar_s` (ex.: 60; teto < 120 s por causa do auto-background) | resultado + linha `[uso]`, **ou** "na fila, posição N" / "rodando há X s" | espera sobre registro, sem erro espúrio |
| `job_listar` | filtros: data, rótulo, situação, lote | lista curta (teto, ex.: 20) | resolve "dia seguinte" sem guardar IDs |
| `job_cancelar` | `job_id` (ou `lote_id`) | confirmação idempotente | |
| `lote_enviar` | lista de pedidos + `rotulo` | `lote_id` + IDs dos jobs | prioridade baixa por padrão |
| `lote_resultado` | `lote_id` (paginado) | resultados prontos + situação dos demais | |

- CLI no mesmo executável (sem tokens de espera no Claude Code):
  - `mcpollama-mcp.exe --esperar <job_id|lote_id>` → bloqueia até concluir (ou falhar) e imprime um resumo curto; a IA dispara com
    `Bash run_in_background` e o Claude Code a reativa no fim;
  - `mcpollama-mcp.exe --lotes-prontos` → imprime os lotes concluídos e não buscados do dono; usado por um hook `SessionStart`.
- As descrições das tools ensinam o fluxo: enviar → seguir trabalhando → esperar em segundo plano → ler; `TOOLS.md` regenerado por
  `src/cmd/dumptools`.

### 7.3 Gateway

1. **Job desacoplado da conexão**: pedido assíncrono **não é cancelado** quando o cliente sai; roda até o fim; resultado guardado.
2. **Fila persistente**: pedidos assíncronos pendentes gravados em disco; na subida do gateway voltam à fila; jobs que estavam
   rodando num reinício (`interrompido_reboot`/`interrompido_gateway`) **voltam para a fila** em vez de só registrar.
3. **Prioridade baixa para lotes** + limite próprio (não ocupar os 64/128 da fila interativa); envelhecimento (`--age-after`) já existe.
4. **Consulta por dono**: endpoint novo (ex.: `/api/jobs/{id}`, `/api/jobs?…`, `/api/lotes/{id}`) **fora do `/monitor`**, autenticado
   pelo Bearer + prova de dono (ver §9, decisão 3), com **espera longa** (segura a consulta até concluir ou `aguardar_s`).
5. **Map-reduce no gateway** (proposta "b"): o mcpollama lê o arquivo na estação, envia o **texto inteiro** e o gateway divide, roda as
   partes e junta; assim o job sobrevive ao fim da sessão. (Hoje quem divide é o mcpollama; D11-15 previa job composto local.)
6. **WebSocket** `/api/ws` como transporte opcional de aviso/entrega (D11-16/D11-23 continuam válidos), com ping < 100 s.
7. `/monitor`: mostrar lotes, jobs assíncronos, "pronto não buscado", conexões WebSocket.

### 7.4 Armazenamento no servidor

| Banco | Existe | Papel no desenho |
| --- | --- | --- |
| `gateway.db` | sim | metadados de todos os jobs (já tem `status`, estação, tokens, tempos) + colunas novas (ver §7.6) |
| `content.db` | sim | log integral (10.L) — passa a ser o **registro 100%** (ver §7.6) ou fica como está, com o 100% em outro lugar (§9, decisão 6) |
| `treino.db` | sim | sem mudança |
| ⛔ **`lotes.db`** (superado: fonte única de estado `jobs.db`, D11-34) | **novo** | lotes, jobs assíncronos pendentes (fila persistente), resultados ainda não buscados, situação `pronto_nao_buscado`/`buscado`, dono |

- Campos de **lote**: `lote_id`, `rotulo`, `dono` (hash do token + estação + usuário; **nunca o token**), `criado_em`, `prazo_guarda`,
  contadores (na_fila/rodando/pronto/erro/buscado).
- Campos de **job**: `job_id`, `lote_id`, `ordem`, `situacao`, `tool`, `modelo`, `think`, `entrada` (TEXT), `resultado` (TEXT),
  `raciocinio` (TEXT), `[uso]` (tokens), `erro`, `enfileirado/iniciado/terminado/buscado_em`.
- Guarda: até ser buscado + N dias (configurável); limpeza automática; teto por dono; arquivo 0600.
- Tamanho [estimativa]: 5–20 MB por lote de 200 documentos; disco com 173 GB livres.

### 7.5 Notificação à IA (Claude Code) e custo de tokens

| Forma | Tokens durante a espera | Funciona onde |
| --- | --- | --- |
| Modelo consultando a cada 60 s | sim (uma rodada por consulta) | qualquer cliente |
| `job_resultado(aguardar_s≤~100)` em ciclo | sim, poucas rodadas | qualquer cliente MCP |
| Subagente com **uma** espera bloqueante | só no início e no fim | Claude Code |
| **`--esperar` em `Bash run_in_background`** | **zero até o aviso** | Claude Code (**recomendado**) |
| Hook `SessionStart` com `--lotes-prontos` | zero (roda sem o modelo) | Claude Code (dia seguinte) |
| MCP *tasks* | — | Claude Code ≥ 2.1.304 (não instalado) |

Itens antigos que continuam úteis: D11-17 (aviso "jobs concluídos aguardando leitura" no fim de cada resposta de tool) e D11-18
(toast do Windows sem conteúdo).

### 7.6 Registro integral ("100% do que entra e do que sai, sem perder nada")

Lacunas atuais e o que fecha cada uma:

| # | Lacuna hoje | Proposta |
| --- | --- | --- |
| 1 | corte acima de 16 MiB por parte | sem corte; teto de segurança alto que **avisa**, não corta |
| 2 | apaga com 7 dias e acima de 4 GiB (mais antigos) | sem apagamento automático do registro integral; arquivo mensal compactado; alerta de disco |
| 3 | gateway só vê o prompt montado | mcpollama envia o **envelope da chamada**: tool, argumentos (sem segredos), caminho e SHA-256 do arquivo, instrução, formato |
| 4 | partes do map-reduce soltas | **ID de chamada** liga partes e junção |
| 5 | resultado final entregue à IA não chega ao gateway | mcpollama envia o **fechamento** com o texto final entregue |
| 6 | ⛔ texto do raciocínio não registrado (**errado**: já é registrado, `journal.go:214-250`; §15.3 item 1) | campo próprio `raciocinio` |
| 7 | recusados (cheia/expirado/removido) sem texto garantido | gravar o texto **antes** da decisão de fila |
| 8 | imagens só com marcador | **somente metadados** (decisão do owner 17:49; §7.7) |
| 9 | sem cópia fora do servidor | backup para fora (ex.: TrueNAS da CSL), cifrado ou não (decisão) |

Metadados por registro: **quem** (estação: máquina/usuário/domínio/MAC — já existe; **projeto** = pasta/repositório da sessão, que o
mcpollama conhece pelo diretório de trabalho e `--allow-root`; **cliente** = `clientInfo` do `initialize` MCP, ex.: Claude Code + versão;
**ID de sessão** e **modelo da IA chamadora** se o cliente expuser — **[não verificado]**); **o quê** (tool, argumentos, arquivo/hash,
modelo local, `think`, prioridade, lote); **entrada** (prompt montado + JSON exato enviado ao Ollama, em `TEXT`); **saída** (resposta do
Ollama, raciocínio, resultado final entregue, em `TEXT`); **como foi** (status, tokens, tempos, backend, se a IA recebeu).
Organização: **um registro por chamada da IA**, com os jobs do Ollama como filhos; números no `gateway.db`, texto em banco 0600,
ligados por ID (preserva a separação decidida em 30/10). Campos `TEXT` em vez de `BLOB` (ou view com `CAST(… AS TEXT)`; cuidado com
bytes inválidos, provável motivo do `BLOB`). Prova de não adulteração opcional (encadeamento de hashes).

### 7.7 Imagens / OCR — decisão do owner: só metadados no servidor

> ⛔ **Superado:** o texto extraído pelo OCR é **conteúdo**, não metadado; o owner decidiu **guardá-lo** com o prazo do registro (D11-30, §15.2). A imagem em si continua fora.

- O OCR roda **na estação** (mcpollama, `src/ocr/`, Tesseract 5.3.3 em processos filhos isolados, opt-in por `--ocr-lib` +
  `--ocr-lib-sha256`; limites `--ocr-max-pages` 30, `--ocr-dpi` 300, `--ocr-max-megapixels` 40). Logo, os metadados saem do mcpollama no
  envelope.
- Metadados: SHA-256 do arquivo e de cada página; nome, caminho na estação, tamanho; tipo (`.png/.jpg`/PDF digitalizado); páginas
  totais e lidas; dimensões, DPI, megapixels; motor/versão/idioma/tessdata; **confiança média e tempo por página**; **texto extraído
  por página**; se a imagem foi enviada **direto ao modelo** (o gemma4 aceita imagem; hoje `tem_imagem` + marcador no `prompt_json`).
- Consequência aceita: sem a imagem, não se refaz o OCR só com o servidor; o hash identifica o original na estação.

### 7.8 Raciocínio (`think`)

- Com o assíncrono, o custo de tempo do `think` deixa de bloquear a IA; continua segurando a fila de 1 slot (prioridade e lotes
  amenizam).
- Medir antes de adotar: acrescentar `-think` ao bake-off (`src/cmd/bakeoff`) para comparar acerto com/sem raciocínio.
- Proteger contra resposta vazia por esgotar `num_predict` (a tool já avisa; revisar limites para jobs assíncronos).

### 7.9 Segurança e dados de cliente

- Resultado e registro integral contêm dados de clientes: **só o dono** lê pelo canal da IA; admin no monitor (LAN); arquivos 0600;
  nunca o token em claro; limites por dono; prazo de guarda; LGPD e backup são decisão do owner.
- Superfícies novas para revisão adversarial: endpoint de consulta (IDOR por ID adivinhado/forjado, rótulo forjado), fila persistente
  (injeção/corrupção do arquivo), lotes (flood, lote gigante), WebSocket (reconexão em massa, mensagens grandes, `resume` com claim
  errado), envelope/fechamento (cliente mente sobre o resultado final), registro integral (crescimento sem teto, disco cheio).

### 7.10 Volume [estimativa]

Bake-off de hoje: ~200 mil tokens de entrada em 60 jobs ≈ < 1 MB de texto. Uso pesado: dezenas a centenas de MB/mês de texto;
compactável.

### 7.11 Compatibilidade

- Se as tools atuais mudarem de comportamento (D11-6, tudo assíncrono), sessões antigas (≤ 0.10.x) continuam no HTTP síncrono; o
  servidor aceita os dois.
- Opção compatível: manter as tools síncronas e acrescentar `assincrono:true` + tools novas (decisão 1).

### 7.12 Etapas propostas (a detalhar no plano depois da revisão)

| Etapa | Conteúdo |
| --- | --- |
| 11.0 | Decisões e contrato (dono, guarda, LGPD, acesso, backup) |
| 11.1 | Spike WebSocket/espera (feito em 30/10) + teste interativo pendente (notificação de tarefa MCP em segundo plano) |
| 11.2 | Registro integral sem perda (§7.6, §7.7) |
| 11.3 | Jobs desacoplados + fila persistente + re-enfileirar interrompidos |
| 11.4 | `lotes.db` + consulta por dono com espera longa |
| 11.5 | Tools MCP (§7.2) |
| 11.6 | Claude Code: `--esperar`, `--lotes-prontos`, hook `SessionStart` |
| 11.7 | WebSocket (transporte, ping < 100 s, entrega pelo menos uma vez) |
| 11.8 | Lotes em prioridade baixa, limites por dono |
| 11.9 | Map-reduce no gateway |
| 11.10 | `-think` no bake-off e medição |
| 11.11 | Revisão adversarial (opus), testes de ataque, gate (vet/test/race/build Linux, ataques 9/10.5/12/11), aceite ao vivo, implantação com rollback, documentação (`TOOLS.md`, `docs/`, `D:\MCP\Documentation`, CLAUDE.md global) |

## 8. Desenho novo × decisões antigas (onde há conflito)

> ⚠️ **SUPERADO EM PARTE (2026-11-05).** Esta seção é o **entendimento original de 2026-11-04 17:59, mantido como histórico**. Onde ela diverge das §15.2–§15.7 e do plano reescrito, **valem a §15 e o plano**. Trechos sabidamente superados estão marcados com ⛔.

| Decisão antiga | Situação no desenho novo |
| --- | --- |
| D11-1 tools enviar/resultado | **mantém** |
| D11-2 não cancelar na queda | **mantém**, reforçado (fila persistente) |
| D11-3 só o mcpollama no WebSocket | mantém |
| D11-4 LAN + internet | mantém (consulta por dono também precisa funcionar pelo túnel) |
| D11-5 resultado responde na hora com o estado | mantém (`job_resultado` com `aguardar_s` opcional) |
| **D11-6 todas as tools assíncronas** | **conflito**: proposta nova sugere alternativa compatível `assincrono:true` (decisão 1) |
| **D11-7 mesma máquina/usuário + ID** | **conflito**: na conversa eu disse "consulta de qualquer máquina, desde que mesmo dono" (decisão 3) |
| D11-8 `list_models` síncrona | mantém |
| D11-10 `coder/websocket` | mantém (dependência nova, registrar no NOTICE) |
| **D11-11 segredo por job em `%LOCALAPPDATA%\mcpollama\jobs.json`** | **tensão** com "mcpollama não grava em disco" (já era exceção aprovada) e com "outra máquina" (decisão 3) |
| **D11-12 guarda de 30 min** | **conflito**: lotes do "dia seguinte" exigem guarda até buscar + N dias (decisão 4) |
| D11-13 mesma fila | mantém, com faixa/limite próprio para lotes |
| D11-14/D11-20/D11-25 fallback HTTP local | mantém para o interativo; **não cobre lote do dia seguinte** (processo local morre) |
| **D11-15/D11-22/D11-24 composto local** | **conflito**: proposta "b" move o map-reduce para o gateway (decisão 5) |
| D11-16 push imediato | mantém (WebSocket) |
| D11-17 aviso na próxima resposta | mantém |
| D11-18 toast do Windows | mantém (opcional) |
| **D11-19 `aguardar_jobs` 5 min vs 100 s** | **reaberta**: com `--esperar` em segundo plano, o ciclo curto vira só fallback (decisão 7) |
| D11-21 limpeza do `jobs.json` | depende da decisão 3 |
| D11-23 pelo menos uma vez, sem conteúdo em disco na estação | mantém |
| D11-26/27/28/29 | mantêm |
| Versões 0.13.0/0.10.0 | **obsoletas** (usadas pela Fase 12); novas a definir |

## 9. Decisões em aberto (para o owner, após a revisão)

> ⚠️ **SUPERADO EM PARTE (2026-11-05).** Itens 2, 3, 4, 6 e o transporte foram decididos pelo owner (D11-30 a D11-33, D11-41, D11-42; §15.2 e §15.7); o item 1 continua com a D11-6 até decisão do owner (P11-7). A lista vigente é a §6 do plano.

1. **Forma do assíncrono**: tudo assíncrono (D11-6, incompatível) **ou** `assincrono:true` + tools novas (compatível).
2. **Dono do job**: segredo por job no perfil (D11-11) **ou** dono = hash do token + estação/usuário (forjável pelo cabeçalho
   declarado) **ou** ambos.
3. **Buscar de outra máquina**: permitido para o mesmo dono? (afeta 2 e D11-7).
4. **Guarda dos resultados assíncronos/lotes**: 30 min (antigo) / até buscar + N dias / outro.
5. **Map-reduce**: no gateway ("b") ou local (D11-15).
6. **Registro integral**: estender o `content.db` (sem corte, sem apagar, `TEXT`, envelope/fechamento/raciocínio) **ou** banco novo;
   prazo (infinito? N anos?), quem acessa, backup cifrado no TrueNAS.
7. **Espera da IA**: `--esperar` em segundo plano como padrão (zero tokens) + `job_resultado(aguardar_s≤100)` como fallback.
8. **Lotes**: limites (jobs por lote, lotes por dono, bytes), prioridade padrão, rótulo obrigatório?
9. **Prova de não adulteração** (encadeamento de hashes): sim/não.
10. **`think` padrão** para lotes (ligado?) depois da medição com `-think`.
11. **Webhook** para integrações com servidor (n8n/ERP): agora ou depois.
12. **Atualizar o Claude Code** para ≥ 2.1.304 (habilita MCP *tasks*) — pode simplificar o desenho.

## 10. Riscos

1. **Energia/fonte de 850 W**: lotes de madrugada com o modelo gerando por horas aumentam o tempo sob carga; reinícios já ocorreram;
   a fila persistente mitiga a perda, não a causa.
2. **Fila de 1 slot**: lote grande + `think` pode atrasar o interativo; prioridade baixa e limites são obrigatórios.
3. **Disco**: registro integral sem apagamento cresce sem teto; precisa de alerta e arquivo compactado.
4. **Dados de cliente acumulados** no servidor por longo prazo (LGPD).
5. **Identidade declarada**: `X-Mcpollama-Station` não é autenticado; qualquer um com o token pode declarar outra estação.
6. **Dependência nova** (`coder/websocket`) e complexidade do protocolo (reconexão, entrega pelo menos uma vez).
7. **Comportamento do cliente**: notificação de tarefa MCP em segundo plano no interativo não provada; mudanças de versão do Claude Code.
8. **Mudança de modelo na Fase 13** (mais lento) muda a escala de tempo de tudo.

## 11. O que NÃO foi verificado (ser honesto com a revisora)

1. Se o Claude Code expõe ao servidor MCP um **ID de sessão** e o **modelo da IA chamadora** (só o `clientInfo` do `initialize` é
   garantido pelo protocolo).
2. Se `notifications/progress` reinicia algum timeout no Claude Code 2.1.296.
3. Se a notificação de tarefa MCP em segundo plano **acorda o modelo** no interativo (spike 11.1 não provou).
4. Suporte a MCP *tasks* (≥ 2.1.304, não instalado).
5. O motivo exato do `BLOB` no `content.db` (minha hipótese: bytes não UTF-8).
6. O gabarito do S2 (itens 1.2.x "inventados" por todos os modelos).
7. Esperas de 5–10 min numa tool no cliente interativo (só 300 s em `-p`).
8. Limites reais do túnel Cloudflare para WebSocket nesta zona (padrão: liberado; ocioso ~100 s pela doc).

## 12. Perguntas específicas para a IA revisora

1. O desenho "job desacoplado + fila persistente + consulta por dono + `--esperar` em segundo plano" tem algum furo de consistência
   (job duplicado, perdido, entregue a outro dono) em reinício do gateway, do servidor ou da estação?
2. Map-reduce no gateway ("b") versus composto local (D11-15): qual é mais seguro/robusto dado que o arquivo só pode ser lido na
   estação e o texto vai inteiro ao servidor?
3. Como provar **dono** de forma robusta sem gravar segredo em disco na estação e permitindo (ou não) buscar de outra máquina?
4. O registro integral "100%" está completo? Falta algum dado de entrada/saída/metadado? A separação números (`gateway.db`) × texto
   (banco 0600) é adequada?
5. `TEXT` × `BLOB` no SQLite (modernc.org/sqlite, sem cgo) para texto possivelmente inválido em UTF-8: qual a melhor prática?
6. A prioridade baixa + limite próprio para lotes é suficiente para não degradar o interativo com 1 slot?
7. O WebSocket ainda se justifica, dado que `--esperar` + espera longa por HTTP já resolve a notificação no Claude Code? Ou basta
   long-poll HTTP (sem dependência nova)?
8. Riscos de segurança novos que não listei na §7.9.
9. As etapas da §7.12 estão na ordem certa (gates, revisão adversarial, aceite)?
10. Alguma decisão da §9 que você recomendaria fechar de um jeito específico, com o motivo?

## 13. Referências (arquivos e caminhos)

- Plano congelado: `.workspace/plans/congelados/MCPOllam2026-10-30-224500-fase11-websocket-jobs-assincronos.plan.md`
- Spike 11.1: `.workspace/reports/MCPOllam2026-10-30-234600-fase11-1-spike-espera-longa.report.md`
- Fase 12 (contrato de dados de treino): `docs/CONTRATO-DADOS-TREINO.md`, plano `.workspace/plans/MCPOllam2026-11-01-213500-fase12-erros-treino.plan.md`
- Fase 13 (rota automática de modelo, **não executar**): `.workspace/plans/MCPOllam2026-11-04-033446-fase13-rota-auto.plan.md`
- Bake-off de hoje: `.workspace/bakeoff/out-f13-e4b/`, `out-f13-12b/`, `out-f13-26b/`, `out-f13-31b/` (em curso)
- Código citado: `src/gateway/queue/queue.go`, `src/gateway/store/store.go`, `src/gateway/content/content.go`,
  `src/gateway/proxy/policy.go`, `src/gateway/monitor/monitor.go`, `src/gateway/models/models.go`, `src/cmd/mcpollama-gateway/config.go`,
  `src/mcpollama-mcp/config.go`, `src/cmd/bakeoff/`, `src/ocr/`
- Operação: `docs/MANUAL-GATEWAY.md`, `docs/INSTALACAO.md`, `deploy/gateway/`, `deploy/servidor-ia/`

## 14. Avaliação independente do Codex — 2026-11-04 18:11

> ℹ️ **Histórico:** as recomendações desta seção foram tratadas na §15 (decisões do owner e correções) e no plano reescrito. Onde esta seção diz "decidir", "definir" ou "escolher", a decisão está na §15.2 ou na §15.7.

### 14.1 Escopo e parecer

- Revisei este report, o plano congelado da Fase 11, o spike 11.1 e os trechos atuais do código Go citados abaixo. **Não** rodei
  testes, não consultei o servidor vivo nem reabri os diretórios de resultados da Fase 13; portanto os números das §§1.4, 3.2 e 3.3
  continuam sendo evidências transcritas do report, não uma nova medição minha.
- **Parecer:** o objetivo é viável, mas o desenho ainda não está pronto para virar plano de execução. Ele mistura a entrega do job,
  o aviso ao cliente MCP e o acionamento de uma nova rodada do modelo; precisa fechar a fronteira de dados OCR, a definição de
  “100% registrado”, a identidade do dono e a recuperação após falha antes de aprovar arquitetura/bancos.
- Distinção central: **job assíncrono** desacopla geração da conexão original; **WebSocket/MCP notification** entrega um evento a um
  cliente/processo; **Claude Code** só volta a raciocinar se uma execução ativa receber uma tarefa concluída ou se outra execução do
  Claude for iniciada. Esses três eventos não são equivalentes.

### 14.2 Conclusões para o uso real: Cursor + Claude Code

| Caminho | O que viabiliza | Limite/custo |
| --- | --- | --- |
| Processo local `mcpollama-mcp.exe --esperar <id>` iniciado pelo Claude Code como comando em segundo plano | Enquanto a sessão do Claude Code está ativa, o host acompanha o processo; quando ele termina, o Claude Code pode receber a conclusão e continuar a tarefa. É o melhor encaixe para o uso atual e preserva o trabalho fora do contexto enquanto espera. A §4 registra esse comportamento como medido nesta sessão. | O processo precisa conseguir consultar o job e sobreviver ao tempo de espera. A espera sem inferência não consome tokens do modelo; iniciar o fluxo e tratar o resultado ainda consome uso do Claude. Se a sessão fechar, não se deve depender de o host acordar depois. |
| Hook `SessionStart` que consulta jobs/lotes prontos | Na próxima abertura/início de sessão, mostra que há resultados aguardando; o Claude pode então buscá-los pelo MCP. | Não acorda uma sessão já aberta nem uma sessão encerrada. O hook deve retornar só IDs/estado/resumo mínimo; não inserir saída do modelo ou texto de documentos diretamente no contexto. |
| Servidor MCP enviando `notifications/*` ou MCP Tasks | Pode informar o **host MCP** sobre progresso/estado, se cliente e extensão suportarem a capacidade. | A notificação, por si só, não cria uma nova inferência nem garante que o Claude Code reabra um turno ocioso. O suporte de Tasks continua condicionado à versão/capability real da extensão; o registro deste report cita 2.1.296 e o requisito ≥2.1.304, mas não foi revalidado nesta avaliação. |
| Monitor local iniciando `claude --resume <session-id> "job X terminou; consulte o resultado"` | A CLI documenta retomar uma conversa por ID e fornecer uma nova mensagem; é uma forma de automatizar uma nova rodada. [CLI do Claude Code](https://docs.anthropic.com/en/docs/claude-code/cli-usage) · [SDK do Claude Code em português](https://docs.anthropic.com/pt/docs/claude-code/sdk) | É uma nova execução/rodada de Claude e consome seu uso. Não há evidência de que isso acorde ou se conecte ao painel já aberto da extensão no Cursor; o ID da sessão também não é garantido pelo `initialize` MCP. Não deve ser tratado como notificação gratuita nem como integração pronta. |

**Conclusão prática:** para reduzir chamadas pagas, o fluxo preferível é enviar o job, deixar um processo local acompanhado pelo Claude Code aguardar enquanto a sessão está ativa e consultar o resultado quando esse processo concluir. Para resultados do dia seguinte, persistência no servidor + `SessionStart` + busca pelo MCP cobre retomada humana. O caminho `claude --resume` é opcional quando se deseja uma nova rodada automática e se aceita gastar uso do Claude. O servidor MCP e o WebSocket não acordam o modelo sozinhos.

WebSocket não é requisito para jobs assíncronos, fila durável ou busca no dia seguinte: submissão, consulta/long-poll e cancelamento por HTTPS iniciado pela estação também atravessam NAT e o túnel. WebSocket acrescenta entrega imediata ao processo local enquanto a conexão está ativa, mas traz reconexão, heartbeat, autenticação da sessão e protocolo de confirmação. Como o owner atribui pouca importância ao tempo e prioriza economia, recomendo manter WebSocket **opcional** até um aceite provar que o push imediato agrega valor ao fluxo com o processo `--esperar`. Essa recomendação reabre a escolha antiga de transporte; não revoga D11-3/D11-16 sem decisão do owner.

### 14.3 Achados que precisam corrigir ou qualificar o entendimento

1. **“Só metadados de OCR” não pode incluir texto OCR.** A proposta da §7.7 chama o texto reconhecido por página de metadado. Texto OCR é conteúdo do documento e pode conter os mesmos dados sensíveis da imagem. O código lê `f.Text`, divide o texto extraído em partes e o inclui nos prompts enviados ao modelo (`src/tools/filetools.go:324,426-488`); a trilha do gateway também guarda o texto de prompt e omite/substitui os bytes das imagens por marcadores (`src/gateway/proxy/journal.go:51-99,119-132`; `src/gateway/content/content.go:262-265`). É preciso decidir separadamente: não guardar bytes/imagem; guardar ou não o texto OCR; e por quanto tempo. Nome/caminho do arquivo também pode identificar cliente.
2. **A lacuna “raciocínio não registrado” da §7.6, item 6, está incorreta para os formatos já reconhecidos pelo gateway.** `respTee` extrai os campos `thinking`/`reasoning` e escreve `[raciocínio]` no sink do journal (`src/gateway/proxy/journal.go:214-250`); os testes verificam esse registro (`src/gateway/joblog/joblog_test.go:232-261`). Substituir por uma lacuna mais precisa: falta provar cobertura de todos os formatos/erros de stream e distinguir o raciocínio bruto do Ollama do texto final transformado que o Claude Code efetivamente recebeu. Armazenar raciocínio integral continua sendo decisão de retenção e privacidade, não deve ser presumido.
3. **“100% sem perder nada” é incompatível com limites e exclusões sem definir o escopo.** O `content.db` atual tem teto total de 4 GiB, retenção padrão de 7 dias e teto de 16 MiB por parte, podendo apagar ou marcar conteúdo como truncado (`src/gateway/content/content.go:41-43,262-267`). Embeddings não guardam os vetores completos no journal; imagens têm conteúdo substituído por marcador (`src/gateway/proxy/journal.go:41-45,94-99,278-295`). A proposta de não expirar e não truncar apenas transfere o risco para crescimento ilimitado, disco cheio e exposição prolongada. Definir “100%” como cobertura de cada chamada aceita **dentro de limites explícitos**, com estados de truncamento/falha registrados, e decidir por separado payloads recusados, imagens binárias, vetores e retenção/backup.
4. **Mais de um banco cria risco de estado divergente.** `gateway.db`, `content.db` e o `lotes.db` proposto teriam metadados, fila/status e conteúdo duplicado. Uma gravação pode persistir num banco e falhar no outro; WAL em arquivos SQLite distintos não fornece automaticamente uma única transação de negócio. Antes de criar `lotes.db`, escolher uma fonte autoritativa para estado durável, resultado e dono; definir se `gateway.db`/`content.db` são projeções, e como reconciliar falhas. Preservar texto fora do banco de metadados continua válido, mas não duplicar a máquina de estados.
5. **Reenfileirar job “em execução” oferece entrega/execução pelo menos uma vez, não execução exatamente uma vez.** Se o gateway cair depois de o Ollama terminar e antes de persistir o resultado final, o retry pode consumir GPU/tokens novamente. Cada job precisa de chave de idempotência, contador de tentativas, lease/estado recuperável e gravação durável do resultado antes de anunciar conclusão; o contrato deve permitir duplicação de cálculo sem duplicar/embaralhar resultado. O protocolo D11-23 cobre redelivery do resultado, não esta janela de execução.
6. **O rótulo da estação não autentica o dono.** `X-Mcpollama-Station` é explicitamente declarado pelo cliente e não autenticado (`src/gateway/proxy/proxy.go:34-46`). Se as estações compartilham o mesmo Bearer, `hash(token)+máquina/usuário` não separa usuários de forma confiável. O segredo aleatório por job de D11-11 é uma base melhor para recuperar jobs da mesma estação; buscar de outra máquina exige uma credencial de usuário/dispositivo autorizada, não só MAC/nome/rótulo. Definir rotação, revogação e recuperação de segredo antes de decidir acesso cross-device.
7. **O registro de auditoria precisa dizer em qual fronteira é “integral”.** O gateway vê o corpo transformado que envia ao Ollama e guarda um extrato/JSON com imagens redigidas; não conhece por si só todos os argumentos originais da tool MCP nem necessariamente a resposta final depois de o mcpollama acrescentar avisos/contadores. Para provar o que o Claude recebeu, registrar um envelope/fechamento correlacionado no limite MCP, com versão de ferramenta, modelo/digest, parâmetros, estado de entrega e marcadores de truncamento. Não registrar tokens/segredos. A recusa de manter imagem binária deve constar como exclusão expressa, não como “registro integral”.
8. **Map-reduce remoto altera a fronteira de privacidade.** O gateway não pode ler o caminho local do usuário. Para sobreviver ao fim do processo da estação, ele teria de receber e persistir texto/partes já extraídos localmente. Manter leitura, extração e OCR na estação; só enviar partes de texto necessárias ao job se o owner aprovar que elas sejam retidas no servidor. Não enviar imagem binária sob a decisão atual. O composto remoto também precisa vincular hash do arquivo, versão das instruções, IDs/ordem das partes e resultado de redução.
9. **Fase 13 e Fase 11 devem ser desacopladas.** A Fase 11 resolve espera, persistência e retomada; não melhora a precisão do modelo nem reduz o tempo de GPU. O valor econômico só aparece se o resultado local tiver qualidade suficiente para evitar uma chamada paga. Ao aceitar um job, fixar modelo resolvido/digest, `think`, contexto/parâmetros e versão da instrução; caso contrário uma troca de rota/modelo enquanto o job espera torna o resultado difícil de reproduzir. Rodar o aceite com o modelo escolhido na Fase 13, não com o servidor temporário do bake-off.
10. **Prioridade baixa não isola o tempo da GPU.** Com um slot, um job de lote que já começou não é interrompido por uma chegada interativa. Limite de lote e prioridade no escalonador reduzem a fila, mas não garantem preempção; medir a espera interativa no aceite e definir o máximo de trabalho de lote admitido/ativo.

### 14.4 Ajustes recomendados antes de reescrever o plano

1. Corrigir a §7.7 para separar metadados OCR de texto OCR; corrigir a lacuna de raciocínio em §7.6; esclarecer que conteúdo de imagem binária e vetores de embedding não fazem parte do registro atual.
2. Especificar o contrato de completude, limites, retenção, exclusões, truncamento, backup e tratamento de disco cheio. Não adotar retenção infinita por omissão.
3. Escolher um único estado autoritativo para fila persistente/lotes/resultados; desenhar transições, chave idempotente, tentativas, falhas e recuperação após queda do gateway/Ollama/estação.
4. Fechar autenticação do dono e acesso entre máquinas antes dos endpoints `job_listar`/`lote_resultado`; cabeçalhos declarados não servem como credencial.
5. Manter extração/OCR local e avaliar map-reduce remoto só depois de aprovar a retenção do texto enviado. Registrar no job modelo/digest e configuração efetivamente usados após a decisão da Fase 13.
6. Provar no **Claude Code dentro do Cursor** um comando local de espera em segundo plano, com conclusão enquanto o turno está ativo, sessão fechada/reaberta e job pronto antes do próximo `SessionStart`. Medir o uso do Claude na conclusão. O spike `claude -p` de 11.1 não prova esses cenários de extensão; a nova prova não foi executada aqui.
7. Decidir WebSocket depois da prova do fluxo de espera. Se não houver requisito demonstrado de push imediato, começar com HTTPS/long-poll e manter WebSocket como otimização posterior. Se o owner mantiver a decisão antiga de WS, preservar a entrega por `resume`/ack, mas não atribuir a ele a função de acordar o modelo.
8. Conservar D11-6 (todas as tools assíncronas) como decisão previamente registrada até o owner mudar explicitamente; a alternativa `assincrono:true` da §7.11 é incompatível e não deve substituir D11-6 silenciosamente.

### 14.5 Conclusão

O desenho pode atender ao objetivo do owner: deslocar trabalho mecânico para o modelo local e não pagar uma rodada de Claude apenas porque a fila excedeu o timeout síncrono. O encaixe mais econômico no Claude Code é o próprio host acompanhar um processo local de espera em segundo plano; para o dia seguinte, recuperar IDs/estados por hook e buscar o resultado sob demanda. Um evento MCP/WebSocket sozinho não acorda o modelo, e `claude --resume` é uma nova rodada que consome uso.

Antes do plano novo, os bloqueadores de desenho são: (1) definir “100%” e retenção; (2) corrigir a classificação do texto OCR; (3) definir identidade do dono; (4) escolher a fonte única do estado durável e o comportamento de retry; (5) provar o fluxo de espera na extensão Claude Code do Cursor. WebSocket é uma decisão de transporte separada. Nenhuma dessas conclusões implementa ou descongela a Fase 11.

**Referências técnicas desta avaliação:** `src/gateway/proxy/journal.go`, `src/gateway/content/content.go`, `src/gateway/proxy/proxy.go`,
`src/gateway/joblog/joblog_test.go`, `src/tools/filetools.go`; plano congelado e spike citados na capa; [MCP Tasks (extensão)](https://tasks.extensions.modelcontextprotocol.io/specification/draft/tasks).

## 15. Resposta do coordenador (Claude) à avaliação, decisões do owner e plano reescrito — 2026-11-04 18:31

### 15.1 Conferência dos achados no código

| Achado do Codex (§14.3) | Conferência | Veredito |
| --- | --- | --- |
| 1. Texto OCR não é metadado | o texto extraído entra no prompt (`src/tools/filetools.go`) e já fica no `content.db` | **procede — erro meu** na §7.7 |
| 2. Raciocínio já é registrado | lido `src/gateway/proxy/journal.go:214-250`: `respTee.line` junta `Thinking`, `Message.Thinking`, `Delta.Reasoning`, `Message.Reasoning` e `emit("raciocinio", …)` escreve `[raciocínio] ` no sink | **procede — erro meu** na §7.6 item 6 (afirmei sem ler o código) |
| 3. "100%" sem escopo | limites conferidos em `content.go:41-43` (4 GiB, 16 MiB, 7 dias); embeddings: `maxEmbedBody` e "os vetores nunca são guardados" (`journal.go:41-43`) | procede |
| 4. Vários bancos divergem | arquitetura de arquivos SQLite separados (`gateway.db`, `content.db`, `treino.db`) | procede |
| 5. Reenfileirar = execução pelo menos uma vez | lógico; o D11-23 cobre só a reentrega | procede |
| 6. Rótulo não autentica | lido `src/gateway/proxy/proxy.go`: `HeaderStation = "X-Mcpollama-Station" // … (declared, not authenticated)`; **e todas as estações usam o mesmo token** (`--api-key`), então o hash do token não separa usuários | procede, e é mais grave do que eu tinha escrito |
| 7. Fronteira do registro | o gateway vê o corpo enviado ao Ollama, não os argumentos da tool nem o texto final do mcpollama | procede |
| 8. Map-reduce remoto muda a privacidade | o texto já vai ao servidor hoje; o novo é **reter** as partes para o job sobreviver | procede em parte (retenção aprovada na D11-31) |
| 9. Desacoplar Fase 13 / fixar modelo e digest | — | procede |
| 10. Prioridade baixa sem preempção | `--gen-slots 1`, job iniciado não é interrompido | procede |

Recomendações gerais aceitas: WebSocket opcional (HTTPS primeiro); `claude --resume` registrado como rodada nova que consome uso
pago; D11-6 mantida até decisão do owner (eu tinha proposto trocá-la sem marcar como decisão dele — erro de processo).

### 15.2 Decisões do owner (AskUserQuestion, 2026-11-04 18:24)

| # | Pergunta | Escolha |
| --- | --- | --- |
| D11-30 | Texto extraído pelo OCR | **Guardar, com o prazo do registro** (mesmo acesso: admin e dono); bytes da imagem continuam fora |
| D11-31 | Prazo e limites do "100%" | **90 dias + backup cifrado** (compactação mensal, teto com alerta sem apagar sozinho, cortes marcados, exclusões expressas: imagem binária e vetores de embedding) |
| D11-32 | Dono do job | **Token por usuário** (revogável, rotacionável; mesma pessoa busca de qualquer máquina) |
| D11-33 | Transporte | **HTTPS primeiro, WebSocket depois** (só se um aceite provar valor do aviso imediato) |

### 15.3 Correções ao entendimento (substituem os trechos indicados)

1. **§7.6, item 6** passa a ser: "o raciocínio **já é registrado** no `content.db` (`[raciocínio]`); falta provar cobertura de todos os
   formatos e erros de stream e distinguir o raciocínio bruto do texto final entregue ao Claude".
2. **§7.7**: o "texto extraído por página" **sai da lista de metadados** e passa a ser **conteúdo**, guardado pela D11-30.
3. **§7.4**: o `lotes.db` como banco adicional é substituído pela **fonte única de estado** (D11-34, nome provisório `jobs.db`,
   autoridade; `gateway.db` e `content.db` viram projeções).
4. **§7.2/§7.11**: a alternativa `assincrono:true` **não substitui** a D11-6; virou pendência P11-7 do plano.
5. **§6 item 13 e §7.4**: "consulta de qualquer máquina, desde que mesmo dono" só é válida com **token por usuário** (D11-32).
6. **§7.3 item 6 e §7.12 etapa 11.7**: WebSocket passa a ser etapa condicional (11.16 do plano).

### 15.4 Propostas técnicas que entraram no plano (a confirmar na 11.0)

D11-34 fonte única de estado (`jobs.db`) · D11-35 execução pelo menos uma vez, resultado exatamente uma vez (idempotência, *lease*,
tentativas, gravação durável antes de anunciar) · D11-36 espera por `--esperar` em segundo plano + reserva `job_resultado(aguardar_s ≤ 100)`
+ hook `SessionStart` só com IDs/estado · D11-37 extração/OCR na estação, map-reduce orquestrado no gateway com partes retidas pelo
prazo da D11-31 · D11-38 limites de lote admitido/ativo e medição da espera interativa · D11-39 fixar modelo/digest/`think`/parâmetros/
versão da instrução no job · D11-40 envelope e fechamento no mcpollama, sem tokens/segredos.

### 15.5 Plano reescrito

- Arquivo (reescrito no lugar, com data/hora atualizadas, ordem do owner):
  `.workspace/plans/congelados/MCPOllam2026-10-30-224500-fase11-websocket-jobs-assincronos.plan.md`
  — título novo "Fase 11: jobs assíncronos, lotes e registro integral (HTTPS primeiro; WebSocket depois)".
- Conteúdo: §1 objetivo e fronteiras; §2.1 as 29 decisões antigas com a situação de cada uma (mantida/ajustada/substituída/adiada/
  reaberta/obsoleta); §2.2 D11-30…33 do owner; §2.3 D11-34…40 propostas; §3 arquitetura e endpoints HTTPS; §4 etapas 11.0–11.17
  (incluindo **11.1b**, a prova no Cursor + Claude Code pedida pelo Codex); §5 riscos; §6 pendências P11-1…P11-10; §7 changelog
  (histórico original preservado + entradas de 2026-11-04).
- **Estado: continua CONGELADO.** Nenhum código, deploy ou mudança de servidor por esta fase. Próximo passo: 11.0 (aprovação), quando o
  owner mandar.

### 15.6 Pontos que ainda podem ir à outra IA (opcional)

1. D11-34: `jobs.db` como autoridade com `gateway.db`/`content.db` como projeções — há forma mais simples (estender o `content.db`)?
2. D11-35: desenho de *lease* e tentativas com 1 slot e reboot do host (boot_id).
3. D11-32: emissão e transição do token único para tokens por usuário sem derrubar estações.
4. D11-37: limites de tamanho para partes retidas e vínculo com o hash do arquivo.

### 15.7 Segunda avaliação externa (GPT-6, 2026-11-05 10:25) e tratamento — 2026-11-05 11:40

| # | Ponto do GPT-6 | Tratamento |
| --- | --- | --- |
| 1 | O report seguia contraditório: §7–§9 com afirmações antigas (raciocínio não registrado, OCR como metadado, `lotes.db`, decisões em aberto); a §14 com recomendações já decididas; a §0 sem explicar a relação | **procede.** §0 com a ordem de validade (§15 e plano > §14 > §1–§13); avisos "superado em parte" nas §7, §8 e §9; marcas ⛔ nos trechos errados (§7.4 `lotes.db`, §7.6 item 6, §7.7); aviso de histórico na §14. O texto original foi **mantido** (histórico), não apagado |
| 2 | "Token por usuário" × "(ou por estação)" deixava incerto o acesso de uma pessoa em máquinas diferentes | **procede.** Owner (11:35): **token por máquina, agrupado por pessoa** — o dono do job é a pessoa; ela busca de qualquer máquina dela; revogar um token corta só aquela máquina. D11-32 reescrita no plano |
| 3 | Faltavam o prazo do backup e o comportamento sem espaço | **procede.** Owner (11:35): **backup sem prazo** (D11-41; as cópias ficam até o owner apagar) e **recusar antes de aceitar** (D11-42; HTTP 507 abaixo de um limite de espaço livre, proposta 10 GB; nenhum job roda sem registro). P11-3 atualizada (falta caminho/chave do backup e confirmar o limite) |
| 4 | "Resultado exatamente uma vez" precisava de contrato preciso | **procede.** D11-35 reescrita: **um único resultado autoritativo por job** (o primeiro gravado de forma durável; tentativas posteriores registradas e descartadas); anúncio só após gravação; `idempotency_key`: mesmo dono + mesmo corpo → **mesmo `job_id`**, corpo diferente → **409**, validade = prazo do registro; fechamento idempotente que prova **entrega ao cliente MCP**, não leitura pelo modelo |

Conclusão do GPT-6 aceita: a Fase 11 **continua congelada**; pendentes a aprovação **11.0** e a prova **11.1b** (Claude Code dentro do Cursor).

### 15.8 Terceira leitura externa (GPT-6, 2026-11-05 11:52) e tratamento — 2026-11-05 12:00

| # | Ponto | Tratamento |
| --- | --- | --- |
| 1 | A linha "Estado" da capa ainda dizia "somente entendimento" e "o plano será reescrito depois"; a §0 item 1 dizia "o sistema como ele é hoje" | **procede.** Capa atualizada (plano já reescrito; vale a §0/§15/plano); §0 item 1 diz que §1–§4 são a fotografia de 2026-11-04 17:59 |
| 2 | O limite de 10 GB só protege os jobs já aceitos se houver reserva calculável por job/lote | **procede.** D11-42 ganhou a **reserva**: cada job aceito reserva `bytes_in` + o teto de resposta (`num_predict` × 4 bytes, limitado ao teto de 16 MiB por parte) + 64 KiB de metadados; um lote reserva a soma dos seus jobs; o gateway só aceita se `espaço livre − reservas ativas − novo pedido ≥ limite`; a reserva é liberada quando o registro final é gravado |
| 3 | O fechamento prova que o mcpollama escreveu a resposta no canal MCP, não que o cliente a consumiu | **procede.** D11-35 item 5: o estado `entregue` passa a significar "**resposta escrita com sucesso no stdout (canal MCP)**", com a limitação explícita: não prova que o cliente leu nem que o modelo usou |
| 4 | O aceite do Qwen ainda não estava consolidado (`run-B2.log` em curso) | **procede**; o aceite é consolidado no relatório da Fase 13 antes de qualquer implantação |
| 5 | Commit `33ac4f5` local e duas modificações fora dele (`deploy/gateway/mcpollama-gateway.service`, `src/cmd/bakeoff/suites.go`) | **correto e intencional:** são o `--max-wait 30m` (owner, 11:39) e a folga de tokens do S6 com `-think`; entram no commit da implantação, com o push |
