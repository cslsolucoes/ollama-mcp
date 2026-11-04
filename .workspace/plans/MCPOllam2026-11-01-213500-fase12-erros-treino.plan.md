# Plano — Fase 12: o MCP organiza os erros do modelo local e gera o histórico para treinar o servidor de IA

> **Estado:** ▶️ **APROVADO pelo owner em 2026-11-02 19:45** ("ok, execute o plano"); **em execução** (12.2 concluída e publicada; 12.9 concluída; próxima: 12.10, a implantação). Contrato de dados **1.1.6 aprovado** (1.0.0 em 2026-11-02 20:05; 1.1.0 às 20:33; 1.1.1 às 20:40; 1.1.2 às 23:40) (`docs/CONTRATO-DADOS-TREINO.md`), que **prevalece** sobre este plano onde divergirem. Revisado **três vezes** por uma IA externa (GPT-6 Codex, 2026-11-02 15:23, 15:39 e 15:59), com os
> achados incorporados (§15). As questões da §14 foram fechadas pelo owner (15:50). O código começa na 12.2.
> **Escopo (owner, 2026-11-02 15:30):** **base confiável + medição + melhora sem treino**. A fase precisa **reduzir erros já**, sem mexer
> nos pesos do modelo (D12-15). O treino LoRA segue congelado (§10).
> **Versões alvo:** mcpollama `0.10.0` (ID do job visível + tool `reportar_erro`) e gateway `0.13.0` (captura canônica, base de treino,
> aba Erros, melhora sem treino).
> **Última atualização deste documento:** 2026-11-02 20:28 (sincronizado com o contrato 1.0.0, §16). Fatos de código levantados em 2026-11-02, com `arquivo:linha` relativos a `src/`.
> **Origem (owner, 2026-11-01 21:31):** "preparar o MCP para se organizar e gerar as informações necessárias para que possamos evoluir
> e treinar a IA para que determinados erros não ocorram", mais um md que instrua o Claude dos projetos a relatar os erros e o que foi
> feito para corrigir, gerando histórico para treinar o servidor de IA.

---

## 0. Para quem lê este documento (outra IA ou pessoa)

Ele foi escrito para que alguém **sem nenhum histórico do projeto** entenda o sistema como está hoje, o problema, o desenho proposto e a
forma de trabalhar do owner. Leia nesta ordem:
1. §1–§5: contexto e estado atual;
2. §6–§9: a proposta da fase;
3. §11: metodologia, obrigatória para quem for executar;
4. §15: o que a revisão externa apontou e como foi tratado.

Tudo marcado **"fato"** foi conferido no código ou no servidor em 2026-11-02. Tudo marcado **"proposta"** ainda depende do owner.

**Ponto de atenção:** onde a revisão sugerir algo que contradiga um **fato** desta seção (ex.: "o gateway não tem X"), confira no código
antes de mudar o plano. Na revisão externa da Fase 11, uma das lacunas apontadas já estava resolvida no código.

---

## 1. O projeto em uma página

- **O que é:** o `mcpollama` é um servidor **MCP** (Model Context Protocol, stdio, JSON-RPC 2.0) escrito em **Go**, que roda **na estação**
  (Windows) dentro do Claude Code. Ele expõe como *tools* o modelo de linguagem **self-hosted** da CSL, servido por **Ollama** num servidor
  Linux com GPUs.
- **Objetivo:** **economizar tokens Anthropic.** O Claude orquestra e decide; o trabalho mecânico em volume (resumir, extrair, traduzir,
  rascunhar, embeddings) vai para o modelo local. Documentos sensíveis de clientes também vão para ele, porque **não saem da rede local**.
- **Princípio de design:** a economia vem de o **mcpollama ler o arquivo do lado dele** e devolver ao Claude só o resultado compacto. Uma
  tool que exige o Claude já ter o conteúdo no contexto economiza pouco.
  - Medido na Fase 6: resumir um arquivo de ~72 mil caracteres custou **−66%** em Anthropic e deixou ~22 mil tokens fora do contexto.
- **Componentes:**
  1. **mcpollama** (`mcpollama-mcp.exe`): nas estações, em `D:\MCP\`;
  2. **mcpollama-gateway**: no servidor de IA, entre o nginx e o Ollama; tem fila, painel `/monitor`, log de jobs e pool de backends;
  3. **nginx**, com TLS e Bearer;
  4. **Ollama**, com o modelo.
- **Repositório:** `https://github.com/cslsolucoes/ollama-mcp` (privado; renomeado de `mcpollama` em 2026-11-01). Pasta local
  `E:\MCPOllam`. Módulo Go `github.com/cslsoftwares/mcpollama` (o nome do módulo e dos binários não mudou).
- **Owner:** Claiton de Souza Linhares (CSL Soluções). Decide tudo; ver a metodologia na §11.

## 2. Estado atual (2026-11-02), fato

### 2.1 Versões em produção

| Componente | Versão | Onde | Tag |
| --- | --- | --- | --- |
| mcpollama | `0.9.2-f367b35` | `D:\MCP\mcpollama-mcp.exe` em cada estação | `mcpollama-v0.9.2` |
| gateway | `0.12.6-a155513` (implantado 2026-11-01 21:45) | `/usr/local/bin/mcpollama-gateway` no servidor | `gateway-v0.12.6` |
| Ollama | `0.40.13` (desde 2026-10-31; antes 0.40.8) | servidor, `/usr/local/bin/ollama` + `/usr/local/lib/ollama` (tarball) | — |
| Modelo de geração | `qwen3-ia` = alias de **`qwen3.5:9b`** (Q4_K_M, ~13 GB em 64k, 100% GPU) | servidor | — |
| Modelo de embeddings | `nomic-embed-text` (768 dimensões) | servidor | — |

O GitHub está em `d331316`, igual ao local. As tags vão de `mcpollama-v0.8.1` a `v0.9.2` e de `gateway-v0.11.1` a `v0.12.6`.

### 2.2 Servidor de IA (fato)

- `ia.cslsolucoes.com.br` = **10.100.2.2** (`lx-ai01`), CentOS Stream 10.
- **4× RTX 3070 (8 GB cada, 32 GB no total).**
  - Potência limitada a **100 W** por placa e clock travado em **210–1400 MHz**, pelo `nvidia-powercap.service`.
  - Motivo: a **fonte de 850 W não aguenta as 4 placas** e já derrubou o servidor várias vezes.
  - O owner comprou um acionador para uma 2ª fonte, ainda sem data de instalação.
  - Últimos reinícios: 30/10 16:40/16:42, 31/10 12:17, 01/11 ~14:10 e 02/11 09:26. Todos se recuperaram sozinhos.
- **Ollama:** `NUM_PARALLEL=1`. A arquitetura do `qwen3.5` **não roda pedidos em paralelo** nesse Ollama: tudo é servido **um por vez**,
  e a vazão vem da fila.
  - Contexto 65536 e `KEEP_ALIVE=-1` (modelos residentes).
  - `ollama-warm.service` carrega o `qwen3-ia` no boot.
- **Caminho de um pedido:**
  1. estação → `https://ia.cslsolucoes.com.br:8443` (LAN; pela internet, o mesmo host sem porta, via Cloudflare Tunnel);
  2. **nginx** (TLS com certificado Cloudflare Origin CA, Bearer, `limit_conn 80` por IP, 600 req/min com rajada de 100);
  3. **gateway** em `127.0.0.1:11500`;
  4. **Ollama** em `127.0.0.1:11434` (só loopback).
- **Gateway em produção** (unit `deploy/gateway/mcpollama-gateway.service`):
  - `--gen-slots 1 --embed-slots 2`;
  - `--queue-max 128 --queue-per-client 64 --queue-per-ip 64`;
  - `--priority-ips 10.100.2.0/24`;
  - `--allowed-models "qwen3-ia;nomic-embed-text"`;
  - `--admin-file /etc/mcpollama-gateway/admin`.
  - Os padrões do código são diferentes (ex.: `--gen-slots 2`, `--queue-max 64`); vale a unit.
- **Monitor:** `https://ia.cslsolucoes.com.br:8443/monitor`, **só LAN**. Leitura livre; ações só com o usuário `admin`.

### 2.3 Fases do projeto

| Fase | Assunto | Estado |
| --- | --- | --- |
| 1–7 | protocolo, tools base, tools de arquivo, guard de caminhos, integração Claude Code, medição, documentos (docx/pptx/pdf) e OCR | concluídas |
| 8 | escolha do modelo (`qwen3.5:9b` pelo foco multiagente) e `think` por chamada | concluída |
| 9 | gateway: fila com prioridade, `/monitor`, SQLite | concluída, em produção |
| 10 | log de jobs com conteúdo (`content.db`), pool de backends, fallback do mcpollama; faixa CPU descartada | **congelada** (falta o 2º servidor com 4× RTX 2060, sem data) |
| 11 | túnel WebSocket + jobs assíncronos | **congelada** em 2026-10-30 (só o spike 11.1 foi feito) |
| 0.12.2–0.12.6 / 0.9.1–0.9.2 | painel responsivo, Detalhes, identificação da estação (máquina/usuário/domínio/MAC via WMI), correção do travamento do painel | concluídas, em produção |
| **12** | **este plano** | **em planejamento** |

Planos: `.workspace/plans/` (congelados em `.workspace/plans/congelados/`). Relatórios: `.workspace/reports/`. Histórico de versões:
`CHANGELOG.md`.

### 2.4 Incidentes recentes que importam para esta fase

- **2026-11-01:** o `/monitor/api/state` travou por um impasse no `database/sql`. O `store.Stats` usava o ctx da aba do visitante, segurando
  o mutex do monitor; os pedidos do painel se acumularam até o `limit_conn 80` e a estação passou a receber 429.
  - Corrigido na 0.12.6: leitura do banco com `context.Background()`, cache em segundo plano (`monitor/swr.go`) e o teste de regressão
    `TestStateSurvivesHungDatabaseRead`.
  - **Lição para a Fase 12:** nenhuma leitura de banco atendida a um visitante pode usar o ctx dele sob mutex compartilhado. Toda
    consulta nova (aba Erros, exportação) segue o mesmo padrão.
- **2026-11-01:** o disco D: da estação lotou (10 MB livres) por saídas de comandos de agentes. Ao rodar testes e builds, conferir
  espaço livre.

## 3. Arquitetura (fato)

```
 Estação Windows (Claude Code)                          Servidor de IA 10.100.2.2 (CentOS)
 ┌───────────────────────────────┐   HTTPS :8443       ┌──────────────────────────────────────────────────────────┐
 │ Claude (orquestra)            │   Bearer            │ nginx ── TLS, limit_conn 80/IP, 600r/m                    │
 │   │ tools MCP                 │ ─────────────────▶  │   │ X-Real-IP                                             │
 │   ▼                           │  X-Mcpollama-Client │   ▼                                                       │
 │ mcpollama-mcp.exe (Go, stdio) │  X-Mcpollama-Tool   │ mcpollama-gateway 127.0.0.1:11500 (Go)                    │
 │  - lê arquivos (guard)        │  X-Mcpollama-Station│   ├ fila (prioridade, justiça IP+rótulo, limites)          │
 │  - docx/pptx/pdf, OCR         │                     │   ├ políticas (think off, remove keep_alive/num_ctx...)    │
 │  - map-reduce de arquivo      │ ◀─────────────────  │   ├ pool de backends (hoje 1: gpu-local)                   │
 │  - linha [uso] no fim         │   X-Request-Id      │   ├ gateway.db (requests, eventos, amostras GPU/CPU)       │
 └───────────────────────────────┘   (ignorado hoje)   │   ├ content.db (prompt+resposta completos, 7 dias)         │
                                                       │   └ /monitor (SSE 1 s, admin p/ ações e conteúdo)         │
                                                       │   ▼                                                       │
                                                       │ Ollama 127.0.0.1:11434 — qwen3-ia (1 por vez), nomic     │
                                                       └──────────────────────────────────────────────────────────┘
```

## 4. Estrutura do repositório (fato)

```
E:\MCPOllam
├ src/                    código Go (go.mod: go 1.26.0; toolchain local go1.27.0)
│ ├ mcpollama-mcp/  (24 .go)  entrypoint do MCP, config.go (flags→env), station.go + station_wmi_windows.go (WMI em Go puro)
│ ├ mcp/            (1)       tipos JSON-RPC/MCP escritos à mão (sem SDK)
│ ├ ollama/         (12)      cliente HTTP do Ollama/gateway: stream, TLS, mapa de erros, fallback, cabeçalhos
│ ├ tools/          (33)      tools MCP: generate, chat, embed, list_models, filetools (summarize_file/extract/translate), usage.go
│ ├ files/          (7)       guard de caminhos (--allow-root, lista de arquivos sensíveis)
│ ├ doctext/        (31)      extração de .docx/.pptx/.pdf (PDF por go-pdfium em WASM/wazero, sem cgo)
│ ├ ocr/            (21)      OCR por Tesseract em processo filho isolado (opt-in)
│ ├ redact/         (2)       redação de token em logs
│ ├ gateway/        queue (8), proxy (8), store (14), content (3), joblog (2), monitor (17), pool (8), gpu (3), cpu (2), models (2)
│ └ cmd/            mcpollama-gateway (10), dumptools (2: gera TOOLS.md), bakeoff (9: carga e avaliação)
├ deploy/gateway/         Instalar-Gateway.ps1 (instalador/atualizador), unit systemd, ollama.conf (nginx), scripts
├ deploy/servidor-ia/     cópias das configs do servidor (inventário, nginx etc.)
├ docs/                   INSTALACAO.md, MANUAL-GATEWAY.md, RELATAR-ERROS-IA.md (Fase 12), README.md
├ .workspace/             plans/, reports/ (inclui *.testes/ com testes de ataque das revisões), bakeoff/ (corpus, gabarito; fora do git)
├ referencias/            material de terceiros, SOMENTE LEITURA (só o Brief_Referencias/ é versionado)
├ CHANGELOG.md · NOTICE.md · TOOLS.md (gerado) · README.md · CLAUDE.md (instruções do projeto para o Claude)
└ .cursor/ (pack canônico) → .claude/, .vscode/ ... são espelhos por symlink; áreas protegidas exigem plano aprovado
```

## 5. Dados que já existem e que a Fase 12 vai reaproveitar (fato, com arquivo:linha)

### 5.1 `content.db` (log de conteúdo, `gateway/content/content.go`)

- Tabela `job_content` (`:46-52`): `job_id TEXT PK`, `created_at`/`updated_at` (ms Unix), `prompt BLOB`, `prompt_bytes`, `prompt_sha256`,
  `prompt_truncated`, `response BLOB`, `resp_bytes`, `resp_truncated`, `resp_final`. Índice `idx_content_created`.
- WAL, `synchronous(FULL)`, arquivo 0600 (`:98-109`).
- **Quando grava:**
  - **prompt:** na **entrada** da fila (`proxy.go:481-485` → `Journal.Enqueued` → `Content.SavePrompt`, `content.go:195`, `INSERT OR IGNORE`);
  - **resposta:** acumulada no streaming, com flush a cada 2 s, e finalizada com `resp_final=1` (`content.go:423-488`, `journal.go:245`).
- **O que grava é texto rotulado, não o JSON do pedido** (`proxy/journal.go:50-116`):
  - seções `[system]`, `[prompt]`, `[suffix]`, `[<role>]` (para `messages`) e `[input N]`;
  - imagens viram "(N imagens omitidas)";
  - na resposta, só o texto gerado. Com raciocínio, `[raciocínio] … [resposta] …`. Nos embeddings, só uma nota.
  - **Consequência para o treino:** o texto rotulado não é exatamente o JSON que o Ollama recebeu. Para reconstruir o par de treino
    (system/user/assistant), a 12.2 precisa ou desfazer os rótulos, ou gravar também o corpo JSON (decisão D12-13, §7).
- **Retenção:** `--retention-content`, padrão 168 h = **7 dias** (`cmd/mcpollama-gateway/config.go:90,327`). A limpeza roda de hora em
  hora (`main.go:288-290`).
- **Tetos:** 2 GiB no total (`--content-max-bytes`; acima disso apaga os mais antigos) e 16 MiB por prompt e por resposta
  (`--content-max-job-bytes`; corta e marca truncado).
- **Desligar:** `--log-content=false` (padrão `true`).
- **Quem lê:** só o admin.
  - `GET /monitor/api/jobs/{id}/content` exige sessão admin + same-site (`monitor/jobs.go:261-273`);
  - `GET /monitor/api/jobs/{id}` mostra tamanhos exatos e hash só para o admin (`:248`).

### 5.2 `gateway.db` (`gateway/store/store.go`)

- `requests` (`:39-47`):
  - identificação e origem: `id`, `client_ip`, `client_label`, `tool`, `endpoint`, `model`;
  - fila e prioridade: `lane`, `priority_in`, `priority_final`;
  - medidas: `bytes_in`, `enqueued_at`, `started_at`, `finished_at`, `wait_ms`, `run_ms`;
  - resultado: `status`, `http_status`, `error_class`;
  - tokens e carga: `prompt_eval_count`, `eval_count`, `load_ms`;
  - contexto: `boot_id`, `think`, `responded`, `resp_bytes`, `backend`, `failover_rule`;
  - estação: `station_computer`, `station_user`, `station_domain`, `station_mac`.
- Outras tabelas: `audit`, `gpu_samples`, `job_gpu_samples`, `cpu_samples`, `backend_samples`, `events`, `meta`.
- **Migração:** idempotente, por `PRAGMA table_info` + `ALTER TABLE ADD COLUMN` só se faltar (`migrate`, `:255-330`). É o molde para
  qualquer coluna ou tabela nova.
- **Retenção de `requests`:** 90 dias, teto de 3.000.000 linhas. Amostras de GPU/CPU: 7 dias.
- **Ciclo de vida:** `JobEnqueued` (`na_fila`) → `JobStarted` (`em_execucao`) → `Finish` (UPSERT). Na abertura, `recoverInterrupted`
  marca jobs presos como `interrompido_reboot` ou `interrompido_gateway` (`store/jobs.go`).

### 5.3 ID do job

- Gerado por `queue.NewID()` (`gateway/queue/queue.go:396-402`): 16 bytes aleatórios em hex, ou seja, **32 hex minúsculos**. O monitor
  valida `^[0-9a-f]{32}$` (`monitor.go:840`).
- O gateway devolve o ID no cabeçalho **`X-Request-Id`** em toda resposta (`proxy.go:416-417`), além de `X-Queue-Ms` e `X-Queue-Position`.
- **O cliente mcpollama NÃO lê `X-Request-Id` hoje.** Ele lê só `X-Queue-Ms` (`ollama/client.go:295`) e `Retry-After` (`ollama/api.go:430`).
  É o 1º ajuste desta fase (D12-1).

### 5.4 Linha `[uso]` e map-reduce

- **Formato** (`tools/usage.go:30-50`):
  `[uso] modelo=<m> prompt_eval_count=<n> [eval_count=<n>] total_ms=<n> load_ms=<n> queue_ms=<n> [partes=<n>] [fim=<done_reason>] [think=on] [backend=fallback]`.
  - Vai no fim do texto (`textResult`, `:90`) e também em `_meta` como `mcpollama/usage` (`:52-76`).
- **Tools de arquivo** (`tools/filetools.go`):
  - o texto é dividido em partes (`splitChunks`; teto `--max-prompt-chars` 128000), e **cada parte é uma chamada, logo um job com ID
    próprio, no gateway**;
  - `summarize_file`/`extract` com mais de uma parte fazem map + reduce, que pode ter várias rodadas (`:590-619`);
  - `translate` faz map + concatenação (`:567`);
  - teto de 40 chamadas (`--max-chunks`);
  - **sai uma única linha `[uso]` somada** por pedido, mais a linha `[arquivo] ... partes=... chamadas=...`.
  - **Consequência:** para o relato de erro, o mcpollama precisa guardar os IDs de **todas** as chamadas (map, reduce e as rodadas) e
    devolvê-los (D12-1).
- **Prompt de sistema único das tools de arquivo:** `fileSystemPrompt` (`filetools.go:47-52`), com marcadores `ARQUIVO_INICIO/FIM <nonce>` e
  `PARCIAL_INICIO/FIM <nonce>` (nonce aleatório de 8 bytes por chamada).
  - Instruções por tool: `summarize_file` `:172-182`, `extract` `:215-226`, `translate` `:251-262`.
  - Temperatura 0,2 (`:40`).
- `generate`/`chat` não têm prompt de sistema próprio: o `system` vem do argumento.

### 5.5 Tools MCP atuais (`tools/registry.go:89-111`; catálogo `TOOLS.md`, gerado por `cmd/dumptools`, conferido por `TestCatalogInSync`)

| Tool | Argumentos (**obrigatório**) |
| --- | --- |
| `list_models` | — |
| `generate` | **prompt**, system, model, temperature, num_predict, think |
| `chat` | **messages**, system, model, temperature, num_predict, think |
| `embed` | **input**, model, include_vectors |
| `summarize_file` | **path**, focus, language, max_words, model, think |
| `extract` | **path**, **instruction**, format, model, think |
| `translate` | **path**, **target_language**, model, think |

As tools de arquivo só existem com `--allow-root`. Todas são `readOnlyHint=true`. **A Fase 12 cria a primeira tool que escreve**
(`reportar_erro`), o que muda o `readOnlyHint` dela.

### 5.6 Cabeçalhos do cliente (fato)

| Cabeçalho | Quem envia | Validação no gateway | Removido antes do Ollama |
| --- | --- | --- | --- |
| `X-Mcpollama-Client` | mcpollama, rótulo `<máquina>/<usuário>` | `^[A-Za-z0-9._:/@+-]{1,64}$`, senão `invalido`; vazio vira `anonimo` | sim (`proxy.go:190-192`) |
| `X-Mcpollama-Tool` | mcpollama, nome da tool | `^[a-z0-9_.-]{1,32}$` | sim |
| `X-Mcpollama-Station` | mcpollama 0.9.2+, `computer=;user=;domain=;mac=` | `parseStation` (`proxy.go:329-364`): 4 chaves, até 512 bytes, MAC com 12 hex; inválido vira vazio, **sem recusar o pedido**; **declarado, não autenticado** | sim |
| `X-Mcpollama-Priority` / `-Think` | nenhum cliente envia hoje | `alta` só de `--priority-ips` | sim |

Clientes **sem** mcpollama (ex.: o script `onda.py` do GestorERP, que chama `/api/chat` direto) aparecem como `anonimo`. Por decisão do
owner, continuam assim.

### 5.7 Endpoints (fato)

- **Proxy:**
  - enfileirados (POST): `/api/generate`, `/api/chat`, `/api/embed`, `/api/embeddings`, `/v1/chat/completions`, `/v1/completions`,
    `/v1/embeddings`;
  - diretos: `/api/tags`, `/api/ps`, `/api/show`, `/api/version`, `/v1/models`;
  - **403:** `/api/pull|push|create|delete|copy|blobs`;
  - corpo máximo 32 MiB.
- **Monitor:**
  - leitura livre: `/monitor`, `/monitor/api/state|stream|series|history|stats|audit|jobs|jobs/{id}|jobs/export`;
  - só admin: `/monitor/api/jobs/{id}/content`, `jobs/export?conteudo=1`, e os POSTs de `queue/{id}/promote|demote|remove`,
    `running/{id}/cancel`, `backends/{id}/drain|undrain`, `logout`.
  - Os POSTs exigem sessão + `X-CSRF-Token` + same-site, com corpo JSON ≤ 4096 bytes (`monitor.go:728,809-828`), e gravam em `audit`.
  - **CSP** por hash do `<script>`/`<style>` do `index.html`, calculada em tempo de execução (`monitor.go:191-202`): nada de `style=` inline
    nem `setAttribute('style')`, e o arquivo tem de ficar em **LF**.
- **Abas do monitor:**
  - seções: Em execução, Fila de espera, Concluídos recentes, Backends, GPUs, CPU/RAM, Modelos, Última hora, Auditoria;
  - abas do histórico: "Concluídos recentes", "Eventos" e "Log em tempo real";
  - **Detalhes** de um job: máquina/usuário/domínio/MAC; o conteúdo só para o admin.

### 5.8 Ferramenta de avaliação existente (`cmd/bakeoff`)

- Sobe um mcpollama de teste e chama as tools por MCP.
- Suítes:
  - `s1` (resumo);
  - **`s2`** (extract das seções numeradas, comparado com o gabarito do Word em `.workspace/bakeoff/gab/s2-word.tsv`);
  - `s3` (extract JSON), `s4` (tradução), `s5` (prefill grande), `s6` (contagem);
  - `multi` (N agentes concorrentes), `score` (pontua offline), `blind` (revisão humana anonimizada).
- **Saídas:** `.workspace/bakeoff/out/<modelo>/calls.jsonl` e as respostas, fora do git. O corpus tem documentos de clientes e **não vai
  ao git**.
- **Linha de base conhecida do `qwen3.5:9b`:** S2 (numeração) **~80–85%**; a armadilha é copiar o sumário desatualizado. S3 (JSON) 100%.

## 6. O problema e o objetivo desta fase

O modelo **não aprende em uso**: os pesos são fixos e cada pedido é independente. Para que **determinados erros deixem de ocorrer**, é
preciso um **ciclo**:

```
 uso → o Claude/owner confere → ERRO → relato (categoria + resposta certa + o que foi feito)
     → base de treino permanente (pedido exato + resposta errada + resposta certa)
     → banco de provas (mede o modelo atual por categoria)
     → [futuro, §10] ajuste fino LoRA → qwen3-ia-v2 → mede de novo → troca só se ganhar
```

**Hoje já existe:** um registro do pedido (em **texto achatado** e rotulado, **não** o JSON exato que o Ollama recebeu, §5.1) e a
**resposta errada** de cada job, por 7 dias. O pedido exato passa a ser capturado nesta fase (D12-13).

**Hoje falta:**
1. ligar o erro ao job, porque o ID não chega ao Claude;
2. registrar a correção;
3. guardar de forma **permanente** os pares escolhidos;
4. exportar no formato de treino;
5. medir.

Cada um deles tem a sua regra numerada no contrato de dados (`docs/CONTRATO-DADOS-TREINO.md`, C-1 a C-8), que prevalece sobre este texto (C-0.3).

**Esta fase faz esses 5 itens e mais um 6º: uma melhora sem treino** (instruções melhores e exemplos recuperados, D12-15), medida pelo banco de provas. O treino em si fica congelado (§10).

## 7. Decisões (estado de cada uma)

> Revisadas em 2026-11-02 15:40 com os achados da revisão externa (§15). Onde a decisão mudou por causa dela, a linha diz **(rev.)**.

| # | Decisão | Estado |
| --- | --- | --- |
| D12-1 | **(rev.) ID do job visível e rastreável:** o mcpollama lê o `X-Request-Id` de **cada** chamada e acrescenta à linha `[uso]` (e a `_meta`) `job=<id>`, ou, nas tools de arquivo, `jobs=<papel>:<id>,…` com o papel de cada chamada (`map1..mapN`, `reduce1..`, `final`). **Cada ID leva o backend que o criou** (`primario` ou `fallback`), porque com o fallback ligado o relato precisa ir ao gateway certo; hoje só existe `backend=fallback` somado (`tools/filetools.go:408`). **(contrato C-1.2, C-1.17)** `papel` ∈ {`map1..`, `reduce1..`, `final`, `unico`}, `backend` ∈ {`primario`, `fallback`} e `ordem` por job; uma ocorrência aceita **no máximo 64 jobs**. Implementado na 12.2 | proposta (implementada na 12.2) |
| D12-2 | **(rev.) Tool `reportar_erro`:** relata **uma ocorrência** (o resultado errado que o Claude viu), não jobs soltos. Argumentos: `jobs` (a lista do `[uso]`, com os papéis), `job_do_erro` (qual chamada produziu o erro; padrão: a `final`, ou o `unico` quando há um só job; **(contrato C-1.3)** em `translate`, que não tem `final`, e em qualquer lista com mais de um job e sem `final`, o `job_do_erro` é **obrigatório**, e o relato sem ele é **recusado**; ele tem de pertencer à lista de jobs), `categoria` (D12-3), `descricao`, `resposta_correta` (**a resposta certa da chamada `job_do_erro`**, não do resultado montado), `correcao_aplicada`, `severidade`, `origem` (`claude`/`usuario`/`automatico`), `projeto`. Valor fora dos enums é recusado (C-1.16). **Limites (C-1.17):** corpo ≤ 256 KB, `descricao` ≤ 4 KB, `resposta_correta` ≤ 128 KB e no máximo 64 jobs. **Confirmar exige `resposta_correta` não vazia (C-1.12a).** Devolve o número da ocorrência | proposta |
| D12-3 | **Categorias fixas (contrato C-1.16, enum estrito; valor fora é recusado):** `numeracao_errada`, `contagem_errada`, `calculo_errado`, `inventou`, `omitiu`, `formato_invalido`, `resumo_infiel`, `traducao_errada`, `idioma_errado`, `truncado`, `ignorou_instrucao`, `outro` | proposta |
| D12-4 | **(rev.) Base de treino `treino.db`** no gateway, 0600. Modelo de dados em 3 níveis (§8.2): **ocorrência** (o relato lógico) → **jobs** da ocorrência (com papel) → **amostra** (a cópia do pedido canônico e da resposta de cada job). Ao receber o relato, o gateway copia do `content.db` as amostras de **todos** os jobs da ocorrência numa transação. **(contrato C-1.9, C-1.9a)** `sem_pedido` **só** quando o conteúdo do **`job_do_erro`** já expirou do `content.db` ou não tem `prompt_json` (job anterior à 0.13.0); conta na estatística e nunca entra no treino. Se expiraram só jobs que **não** são o `job_do_erro`, a ocorrência nasce `nova`, as amostras existentes são copiadas e as dos expirados ficam só como referência em `ocorrencia_jobs`. Toda ocorrência nasce `nova`, exceto `distribuida`, `fallback` e `sem_pedido`, com a **precedência** `distribuida` > `fallback` > `sem_pedido` (C-1.9). **Transações (C-1.5):** o relato grava a ocorrência, os jobs e as amostras de **todos** os jobs numa única transação (cada amostra é uma linha por `job_id`, compartilhada se o job está em mais de uma ocorrência, C-1.6); `distribuida` e `fallback` não copiam amostras. **(3ª rev.) Ocorrência distribuída** (os jobs da lista têm backends diferentes, D12-1): o relato vai ao gateway do `job_do_erro`, que guarda **só a ocorrência e as referências** de todos os jobs (com o backend de cada um) e **não copia nenhuma amostra**, nem as locais: o caso é só estatística (owner, 15:50). Estado `distribuida` (C-1.10: a resposta traz `estado=distribuida`, `amostras_copiadas=0` e `jobs_externos`). **(contrato C-1.21, C-1.8)** Só o gateway **primário** guarda ocorrências elegíveis, porque cada gateway tem o seu `treino.db` e a partição não é coordenada entre eles; o papel vem da flag **`--role`** (`primario`, o padrão, ou `fallback`). Num gateway `fallback`, toda ocorrência com jobs num só backend nasce no estado próprio **`fallback`**: só a ocorrência e as referências dos jobs, nenhuma amostra, só estatística (resposta com `estado=fallback`, `amostras_copiadas=0`). `distribuida` fica reservado para jobs em backends **diferentes**. **Estados (C-1.8):** `nova`, `confirmada`, `descartada`, `sem_pedido`, `distribuida`, `fallback`, `apagada`; a `particao` (`treino`/`teste`/`quarentena`) é coluna à parte, não é estado. **Transições (C-1.12), só estas:** `nova`→`confirmada`; `nova`/`sem_pedido`/`distribuida`/`fallback`→`descartada` (o admin julgou que não foi erro do modelo, e o caso sai também da estatística de erros; `descartada` não volta a `nova`); `confirmada`→`nova` (alterar a correção, C-6.5); qualquer estado→`apagada`. `sem_pedido`, `distribuida` e `fallback` nunca são confirmadas. Estatística (C-1.14): `sem_pedido`, `distribuida`, `fallback` e `nova` contam só na estatística de erros e nunca entram em exportação, exemplos ou avaliação; `descartada` e `apagada` não contam na estatística de erros (`descartada` conta só na taxa de descarte da D12-8). Quarentena: C-1.15 | proposta |
| D12-5 | **(rev.) `POST /api/feedback`** como **rota própria no mux do gateway, antes do `/`** (hoje tudo fora de `/monitor` vai ao proxy e dá 404, `main.go:263-267` e `proxy.go:426`). Bearer via nginx (a `location /` atual já cobre `/api/`), corpo ≤ 256 KB, validação estrita. O `job_do_erro` precisa existir em `requests` do gateway que recebe; um `job_do_erro` inexistente é recusado com **404, sem gravar nada** (C-1.4); os jobs de **outro** backend (ocorrência distribuída) são aceitos só como referência, sem validação local, e o relato vai ao gateway do `job_do_erro` (C-1.11). Aceita relato de qualquer estação; a confiança vem da **revisão do admin** (D12-16). **Respostas de recusa (contrato):** **503** com mensagem explícita se o gateway estiver sem `--doc-key-file` (C-4.3a); **507** com corpo de erro explícito quando o `treino.db` passa do teto (C-6.2); relato acima dos limites da C-1.17 ou fora dos enums da C-1.16 é recusado. Os dados de estação, projeto e origem são declarados e não autenticados (C-1.18) | proposta |
| D12-6 | **Monitor:** aba **"Erros (treino)"** com a lista de ocorrências, filtros por categoria/tool/período/estação/estado, contagens e as ações **confirmar**, **completar a correção**, **descartar**, **marcar exemplo público**, **apagar** e **liberar quarentena** (admin, com sessão, CSRF e same-site, cada uma gravando no histórico; contrato C-1.13). A liberação escolhe um lado para o componente inteiro, pela rota `POST /monitor/api/treino/{id}/liberar-quarentena`, e o histórico grava `alterou` com `campo=particao` (C-4.11). Descartar vale também para `sem_pedido`, `distribuida` e `fallback` (C-1.12). Confirmar exige `resposta_correta` não vazia (C-1.12a). Consultas da aba e da exportação usam `context.Background()` e cache em segundo plano (C-6.10). Botão **"Marcar erro"** no Detalhes do job (admin) | proposta |
| D12-7 | **(rev.) Regra ÚNICA de exportação** (predicado; substitui o texto anterior, que se contradizia com o risco 1). Uma amostra entra no JSONL se e somente se: (1) a ocorrência está `confirmada` pelo admin; (2) a amostra é o `job_do_erro` da ocorrência; (3) `resposta_correta` não está vazia; (4) o job é de **geração** (`/api/generate`, `/api/chat`, `/v1/chat/completions`, `/v1/completions`; nunca embeddings), terminou com `status=ok` e `http_status=200`; (5) pedido canônico e resposta **não truncados** (`prompt_truncated=0`, **`prompt_json_truncated=0`** (3ª rev.), `resp_final=1`, `resp_truncated=0`), conferidos nas colunas da **amostra**, que as copia do `content.db` no relato e as preserva depois que ele expira (§8.2); (6) a tool é elegível (`generate`, `chat`, `summarize_file`, `extract`, `translate`); **(7, 2ª rev.) o pedido não tem imagem** (`tem_imagem=0`); **(8) a ocorrência não é `distribuida`** (jobs em mais de um gateway) **nem `fallback`** (C-3.9) **nem está em `quarentena`** (D12-7a, item d); **(9) não é erro no resultado montado do `translate`**, que não tem chamada `final` (`tools/filetools.go:567`): no `translate`, só entra se o `job_do_erro` for uma parte (`mapN`) com a tradução certa daquela parte. Ficam de fora `nova`, `descartada`, `sem_pedido`, `distribuida`, `fallback`, `quarentena`, respostas parciais, embeddings e pedidos com imagem, que contam **só na estatística nesta fase** (owner, 15:50; contrato C-1.14, C-1.15, C-2.6). **Contrato (C-3.1 a C-3.16):** este predicado é o **único** caminho de exportação (C-3.1), com um teste por condição, cada um com um caso que falha só nela (C-3.16); a exportação aceita `parte=treino` ou `parte=teste`, exporta só a partição pedida e nenhuma amostra de `quarentena` (C-3.11); é restrita ao admin (sessão + same-site), gerada na hora em streaming, não fica guardada no servidor e grava `exportou` no histórico, **uma linha por ocorrência exportada** (C-3.12, C-7.7); cada linha é `{"messages":[system, user, assistant],"meta":{...}}`, com `system`/`user` do `prompt_json` e o `assistant` = `resposta_correta` (C-3.13); no `chat`, as `messages` completas, com a última do assistente = `resposta_correta` (C-3.14); o `meta` traz só `ocorrencia_id`, `job_id`, `papel`, `tool`, `categoria` e `particao` (C-3.15). A amostra guarda as marcas do item 5 e o predicado lê só a amostra (C-2.9) | proposta |
| D12-7a | **(2ª rev.) Partição treino/teste sem vazamento.** (a) **Identidade do documento:** o mcpollama, que lê o arquivo, envia em **cada** chamada das tools de arquivo o cabeçalho novo **`X-Mcpollama-Doc`** = sha256 do texto extraído do documento inteiro (vai por TLS). **(3ª rev., owner 16:00)** O gateway **não guarda o sha256 puro**, que permitiria correlacionar arquivos e confirmar palpites sobre um documento conhecido: grava em `requests.doc_hash` só **HMAC-SHA256(chave, sha256)**, com a chave num arquivo 0600 gerado pelo instalador (`--doc-key-file`, biblioteca padrão), e remove o cabeçalho antes do Ollama (C-2.11). Trocar a chave quebra os grupos antigos: só com o owner (C-4.4). **(contrato C-4.1 a C-4.3a)** O sha256 puro nunca é gravado (nem em `requests`, amostras, ocorrências, histórico ou logs); a chave fica **fora do backup comum**; **todos os gateways que calculam e guardam HMACs (primário e fallback) usam a mesma chave; os Ollamas do pool não a recebem** (contrato 1.1.0, C-4.3a), copiada por canal protegido; cada hash gravado leva `key_id` (os 16 primeiros hex do sha256 da chave) e o agrupamento só compara hashes com o mesmo `key_id`; **sem `--doc-key-file`, o gateway não aceita relatos** (`/api/feedback` responde 503 com mensagem explícita). Pedidos sem documento (`generate`, `chat`) têm `doc_hash` vazio (C-4.15). (b) **Grupos (C-4.5):** ocorrências ligadas pelo **mesmo `doc_hash`, pelo mesmo `conteudo_hmac` (C-2.12) ou por um job em comum** formam um grupo (componente conectado), e o grupo inteiro cai do mesmo lado; entram nos componentes **todas as ocorrências não apagadas**, inclusive `nova` e `descartada`, para não haver ligação oculta. (c) **Cota por categoria** (owner, 15:50): os primeiros **30** grupos **com ao menos uma amostra exportável** (que passe em todo o predicado da D12-7) de cada categoria vão para o **teste**; "primeiros" segue o momento em que o grupo ganha a primeira amostra exportável, a categoria do grupo é a dessa amostra, e a partição é atribuída nesse momento; antes disso, fica **pendente** (contrato 1.1.0, C-4.6, C-1.8). Depois da cota, 15% (`sha256(id do grupo) mod 100 < 15`, C-4.7). (d) A partição é **estável** depois de atribuída (C-4.8); quando grupos do **mesmo** lado se fundem, o resultado usa o menor `grupo_id` anterior, mantém o lado e não é reavaliado pela cota nem pelos 15%. **Desconfirmar ou apagar uma ocorrência de teste não libera a vaga dela na cota** e invalida (`valido=0`) todo retrato que a lista (C-4.11a). Nenhum `job_id`, `doc_hash` ou `grupo_id` aparece ao mesmo tempo em amostras exportáveis de treino e de teste (C-4.12). **(3ª rev., owner 16:00) Quarentena:** se uma ocorrência nova ligar grupos que já estão em lados diferentes, o **componente inteiro** (todos os grupos unidos mais a nova) vai para `quarentena`: fica fora da exportação de treino e de teste, do índice de exemplos e da avaliação. Os **retratos** que tinham casos desse componente são **invalidados** (a próxima comparação exige retrato e linha de base novos), assim como o índice de exemplos congelado, se usava algum deles. O admin vê o caso na aba e pode liberá-lo, escolhendo um lado para o componente inteiro, pela rota `POST /monitor/api/treino/{id}/liberar-quarentena`, com histórico (`alterou`, `campo=particao`) (C-4.9 a C-4.11). (e) **Retrato do teste (C-4.13, C-4.14):** há **um retrato por avaliação**, uma lista congelada de (`ocorrencia_id`, `job_id`, `prompt_hmac`, `categoria`) mais o hash do índice de exemplos congelado, o ID/hash da versão das instruções e `valido`; depois de criado não muda, exceto `valido`→0. Linha de base e candidato são sempre comparados no mesmo retrato; um retrato novo exige uma linha de base nova | proposta |
| D12-8 | **(rev.) Candidatos automáticos com contrato:** cada regra tem critério objetivo, e todas geram ocorrência `origem=automatico`, estado `nova`, **sem** resposta certa. Elas só entram no treino depois que o admin completa a correção e confirma. Regras: (a) **JSON inválido** quando o pedido **declarou** JSON no campo `format` (`"json"` ou um schema JSON) e o texto não passa por `json.Valid`. **(3ª rev.)** "A instrução pede JSON" saiu, porque não tem critério objetivo; esses casos só entram por relato humano; (b) **vazio**: a resposta só com espaços; (c) **truncado**: `fim=length` **e** a tool não pediu limite curto (`num_predict` ≥ 256 **ou ausente**, C-1.20); (d) **idioma errado**: **fora desta fase**, porque o critério é subjetivo. **(contrato C-1.19, C-1.20)** Essas são as **únicas** regras automáticas; o candidato nasce `nova`, `origem=automatico` e `resposta_correta` vazia. Deduplicação: um relato humano da mesma ocorrência **substitui** o automático, e o histórico grava `alterou` com `campo=origem`. Meta de falso-positivo: acompanhar a taxa de "descartado" por regra no monitor (`descartada` conta só nessa taxa, C-1.14); regra com > 50% de descarte em 50 casos é desligada. **Padrão: ligados** (owner, 15:50) | proposta (padrão **decidido**) |
| D12-9 | ~~Registro provisório em arquivo local~~: **descartado pelo owner** (2026-11-01 21:45). O registro começa só com a tool pronta | **decidida** |
| D12-10 | **(rev.) Retenção e descarte do `treino.db`:** sem expiração automática (arquivo 0600, C-6.1), mas com **teto de 2 GiB**. Acima disso, recusa novos relatos com **HTTP 507** e corpo de erro explícito, grava um evento e mostra aviso no monitor, sem apagar nada sozinho, porque são dados curados (C-6.2). **Apagar** é ação do admin, com histórico: remove todo o conteúdo da ocorrência, os jobs dela e as amostras que ninguém mais usa, e **deixa só uma linha mínima** (`id`, `estado=apagada`, datas), sem nenhum campo de conteúdo (C-6.3). A exportação é **gerada na hora** e não fica guardada no servidor; o arquivo baixado é responsabilidade de quem baixou, e isso fica registrado no histórico. **Backup:** o `treino.db` fica **fora** do backup comum, ou só entra cifrado, como o `content.db`. Treinar fora da rede exige anonimização (§10, C-6.9); `treino.db` e `content.db` ficam fora do backup comum, ou só entram cifrados (C-6.8). **(2ª rev.)** O `historico` **nunca guarda texto sensível**: nos campos de conteúdo (`resposta_correta`, `descricao`, `correcao_aplicada`, pedidos, respostas) registra só **hash e tamanho** de antes e depois (C-7.4, C-7.6, C-7.8). **Apagar** usa contagem de referências: a amostra e o embedding só saem quando nenhuma ocorrência os usa mais (C-6.4). **Alterar** `resposta_correta`, `descricao` ou `correcao_aplicada` de uma ocorrência já confirmada **desfaz a confirmação** (volta a `nova`), **apaga o embedding** anterior e grava `desconfirmou` no histórico (C-6.5). O **embedding** é calculado só quando o admin marca `exemplo_publico=1` numa ocorrência confirmada, e é apagado quando a marca sai ou a confirmação é desfeita (C-6.6). **O `historico` não expira**, é mantido depois de `apagou` (sem texto sensível) e conta no teto de 2 GiB (C-7.9). A exportação grava uma linha `exportou` **por ocorrência exportada**, com a parte em `campo` e quem baixou (C-7.7, C-6.7) | proposta |
| D12-11 | **Sem dependências de terceiros:** tudo em Go, biblioteca padrão + o que já está no `go.mod` (regra do owner). Exceção futura: o treino (§10), em Python, **fora** deste repositório | **decidida** |
| D12-12 | **Preferência pelo `mcpOllama`** no `~\.claude\CLAUDE.md` global (texto na §9.1), **aplicada só na etapa de documentação** | **decidida** (owner, 2026-11-01 21:40) |
| D12-13 | **(rev.) Captura canônica do pedido na ENTRADA** (a proposta anterior, copiar o JSON no relato, não funciona, porque hoje o JSON não fica guardado em lugar nenhum; o `content.db` só tem o texto achatado de `extractPrompt`, `proxy.go:483`, `journal.go:50`). Nova coluna `prompt_json` no `content.db`, gravada no `SavePrompt`, com o **corpo JSON já com a política aplicada** (o que o Ollama de fato recebe): `system`, `prompt`/`messages` completos (todo o histórico do `chat`), `options`, `format`, `think`, `model`. Imagens são substituídas por um marcador `{"imagem_hmac":"<hex>"}` (HMAC-SHA256 da imagem com a chave da C-4.3; contrato C-2.5). O `prompt_json` gravado é igual, byte a byte, ao corpo que chegou ao Ollama, exceto pelas imagens trocadas (C-2.10), e não contém o cabeçalho `X-Mcpollama-Doc` (C-2.11). A resposta tem teto de 16 MiB, marca `resp_truncated` e `resp_final=1` só quando o streaming terminou (C-2.4). As colunas novas entram por migração idempotente (C-2.7). **Hashes (C-2.12):** `prompt_hmac` = HMAC-SHA256(chave, sha256 do `prompt_json` gravado) e `conteudo_hmac` = HMAC-SHA256(chave, sha256 do JSON canônico de `system`, `prompt` e `messages`, sem `model`, `options`, `format` e `think`; chaves ordenadas, sem espaços); nenhum é guardado como sha256 puro (substitui o `prompt_sha256` do rascunho). **(2ª rev.)** Teto de **16 MiB separado** para cada campo (texto achatado e `prompt_json`), cada um com a sua marca de truncado (`prompt_truncated`, `prompt_json_truncated`). **Pedido com imagem:** o `prompt_json` guarda um marcador com o hash no lugar da imagem e a linha recebe `tem_imagem=1`, o que a deixa **não exportável e fora da avaliação**, só na estatística (D12-7, item 7; C-2.6). **Teto total do `content.db`: 4 GiB, com retenção de 7 dias** (owner, 15:50) | proposta |
| D12-14 | **Instruções onde o Claude lê:** `docs/RELATAR-ERROS-IA.md` (fonte única) + seção curta no `~\.claude\CLAUDE.md` global apontando para ele. Entra na etapa de documentação, junto com a tool | **decidida** (owner, 2026-11-01 21:45) |
| D12-15 | **(novo) Melhora sem treino**, dentro desta fase (escopo do owner, 2026-11-02 15:30). Duas frentes, medidas antes e depois pelo banco de provas: (a) **instruções por tool** reescritas a partir das categorias que mais erram (ex.: no `extract`, "use a numeração do corpo do documento, nunca a do sumário"). **(3ª rev.)** Essa análise usa **só a partição de treino**, nunca a de teste. Cada versão das instruções ganha um ID e um hash e é **congelada antes da avaliação**; o retrato registra a versão usada; (b) **exemplos recuperados**: para cada pedido de uma tool, o gateway busca por embedding (`nomic-embed-text`) as ocorrências **confirmadas** parecidas e inclui até k pares "erro → certo" como exemplo no pedido. **Restrição de privacidade (2ª rev., owner 15:50):** os exemplos vêm **só de ocorrências marcadas pelo admin como "exemplo público"** (sintéticas ou anonimizadas); **nenhum documento de cliente é injetado no pedido de outro**. "Mesmo projeto" fica para uma fase futura, porque hoje não existe identidade de projeto no pedido e um valor declarado não autoriza acesso. **Contra vazamento na avaliação:** a busca usa **só a partição de treino**; o índice de exemplos é **congelado** junto com o retrato do teste antes de cada avaliação; a ocorrência, o grupo e o documento em avaliação ficam fora da busca. Ligável por tool e por flag, com padrão desligado até a medição provar ganho. **(contrato C-8.1 a C-8.10)** Só entram como exemplo as ocorrências `confirmadas` com `exemplo_publico=1` (C-8.1); o embedding existe só para elas (C-6.6); a busca usa só a partição `treino` e exclui a ocorrência, o grupo e o documento em avaliação e todo caso em `quarentena` (C-8.3, C-8.4); uma ocorrência desconfirmada, descartada, apagada, `sem_pedido`, `distribuida` ou `fallback` não é usada como exemplo (C-8.9); o índice é congelado com o retrato e o hash dele é registrado no retrato (C-8.5), e a entrada em quarentena de um caso usado o invalida (C-4.10); cada versão das instruções tem ID e hash e é congelada antes da avaliação (C-8.7); a ligação só fica ativa quando a avaliação cumprir C-5.10 a C-5.12 (C-8.8); **o valor de k e o limiar de similaridade são fixados na 12.8, medidos pela 12.7, e registrados no retrato junto com o índice (C-8.10)**; nenhum documento de cliente é injetado no pedido de outro (C-8.2) | proposta |
| D12-16 | **(novo) Fronteira de confiança = revisão do admin** (owner, 2026-11-02 15:30). Qualquer estação relata (dados declarados, não autenticados); **nada entra no treino, nem nos exemplos da D12-15, sem confirmação do admin no monitor**. **Histórico completo** de cada ocorrência (tabela `historico`: `id`, `quando`, `ocorrencia_id`, `acao`, `campo`, `antes`, `depois`, `quem`, `ip`; C-7.1): criação, cada alteração (campo, antes, depois), revisão, apagamento, exportação; `acao` ∈ {`criou`, `alterou`, `confirmou`, `desconfirmou`, `descartou`, `apagou`, `exportou`} (C-7.2), gravada na mesma transação da mudança (C-7.3); com quem fez (estação declarada ou `admin`), IP de origem e quando, pelo horário do gateway (C-7.5). As ações do admin são confirmar, descartar, completar a correção, marcar exemplo público, apagar e liberar quarentena, com sessão, CSRF e same-site (C-1.13). A cobertura é testada varrendo a tabela depois de um ciclo completo (C-7.6) | **decidida** |
| D12-17 | **(novo) Protocolo de avaliação** (§8.5): o que "melhorou" significa, com números. O gate **falha** se a suíte não rodar, se rodar zero casos, se nenhuma categoria atingir o mínimo de **30** ou se o retrato estiver com `valido=0` (C-5.13); o relatório diz "dados insuficientes", nunca "sem regressão". Cada caso automático roda em **3 rodadas pareadas** (`options.seed` = 1, 2, 3), acerto com 2 de 3, e o modelo atual é **medido de novo** a cada comparação no mesmo retrato e sessão (C-5.15, C-5.16) | proposta |

## 8. Desenho técnico detalhado (proposta, revisada em 2026-11-02)

### 8.1 Contrato de dados (etapa 12.1, antes de qualquer código)

É o documento `docs/CONTRATO-DADOS-TREINO.md`, **versão 1.0.0, aprovada pelo owner em 2026-11-02 20:05**, com 119 regras numeradas `C-<cap>.<n>`. Ele fixa (cap. 1 a 8; precedência e escopo no cap. 0):
1. a **unidade do erro**: a ocorrência;
2. o formato do **pedido canônico**: D12-13;
3. a **regra única de exportação**: D12-7;
4. a **partição**: D12-7a;
5. o **protocolo de avaliação**: D12-17, §8.5;
6. a **retenção e o descarte**: D12-10;
7. o **histórico**: D12-16;
8. a **regra de privacidade dos exemplos**: D12-15b.

Todas as etapas seguintes testam contra esse contrato: cada regra `C-<cap>.<n>` tem de ter ao menos um teste que falha se ela for violada (C-0.4). Se o código divergir dele, o contrato vence, ou é alterado com aprovação explícita do owner, e o código nunca o altera por conta própria (C-0.3); mudança de regra incrementa a versão do contrato e é registrada no changelog deste plano (C-0.6). Onde este plano divergir do contrato, o contrato prevalece.

### 8.2 Gateway 0.13.0: dados

- **`gateway.db`** (2ª rev.): coluna nova `requests.doc_hash` = **HMAC-SHA256** do valor do `X-Mcpollama-Doc`, com a chave do `--doc-key-file` (D12-7a, 3ª rev.), junto com o `key_id` (16 primeiros hex do sha256 da chave; C-4.2, C-4.3a). O sha256 puro nunca é gravado. Sem `--doc-key-file`, `/api/feedback` responde 503.
- **Flags novas do gateway (contrato):** `--doc-key-file` (C-4.3, C-4.3a) e `--role` (`primario`, o padrão, ou `fallback`; C-1.21).
- **`content.db`** (D12-13): colunas novas `prompt_json BLOB`, `prompt_json_truncated` e `tem_imagem`, por migração idempotente. Ela é gravada em `SavePrompt` (`content.go:195`),
  chamado por `joblog.go:42`, a partir do **corpo depois da política** (`proxy.go:454`, `applyPolicy`), e não do texto de `extractPrompt`.
  Teto de 16 MiB **por campo**, cada um com a sua marca de truncado (C-2.3); teto total do `content.db` de **4 GiB**, com retenção de 7 dias; acima do teto apaga os mais antigos (owner, 15:50; C-2.8). `prompt_json` vem do corpo depois da política, nunca do texto de `extractPrompt` (C-2.1), e o marcador de imagem é `{"imagem_hmac":"<hex>"}` (C-2.5). As marcas `prompt_truncated`, `prompt_json_truncated`, `tem_imagem`, `resp_final`, `resp_truncated`, `status` e `http_status` são copiadas para a amostra no relato (C-2.9).
- **`treino.db`**: pacote novo `gateway/treino`, migração no molde de `store.migrate`, consultas da aba com `context.Background()` e cache em
  segundo plano (padrão 0.12.6). Tabelas:
  - `ocorrencias`:
    - identificação: `id` (hex 32), `criada_em`, `atualizada_em`, `estado` (`nova`/`confirmada`/`descartada`/`sem_pedido`/`distribuida`/`fallback`/`apagada`; transições na C-1.12 do contrato);
    - o erro: `categoria`, `severidade`, `descricao`, `resposta_correta`, `correcao_aplicada`;
    - origem: `origem`, `projeto`, `tool`, `estacao_computer/user/domain/mac`, `client_ip`, `client_label`;
    - ligação e treino: `job_do_erro`, `doc_hash` (HMAC, com `key_id`), `grupo_id`, `particao` (`treino`/`teste`/`quarentena`; atribuída na primeira confirmação do grupo, C-4.6), `exemplo_publico` (0/1, D12-15b). A antiga coluna `distribuida` (0/1) saiu: o `estado` (`distribuida`/`fallback`) já a cobre (C-1.8, C-1.10, C-1.21);
    - apagada: só uma linha mínima (`id`, `estado=apagada`, datas), sem campo de conteúdo (C-6.3);
    - revisão: `revisada_por`, `revisada_em`.
  - `ocorrencia_jobs`: `ocorrencia_id`, `job_id`, `papel` (`map1..`, `reduce1..`, `final`, `unico`), `backend`, `ordem`. PK
    (`ocorrencia_id`, `job_id`).
  - `amostras` (C-1.6, C-1.7; uma linha por `job_id`): `job_id PK`, `copiada_em`, `endpoint`, `modelo`, `think`, `prompt_json`, `prompt_texto`, `resposta`, `prompt_hmac`, `conteudo_hmac`, `key_id`,
    `prompt_truncado`, **`prompt_json_truncado`**, **`tem_imagem`**, **`doc_hash`** (o HMAC), `resp_final`, `resp_truncada`, `status`, `http_status`.
    Um job pode pertencer a mais de uma ocorrência. **(3ª rev.)** Todas as marcas que o predicado da D12-7 confere são **copiadas para a
    amostra no relato**, para que a exportação não dependa do `content.db`, que expira em 7 dias.
  - `historico`: `id`, `quando`, `ocorrencia_id`, `acao` (`criou`/`alterou`/`confirmou`/`desconfirmou`/`descartou`/`apagou`/`exportou`), `campo`,
    `antes`, `depois`, `quem`, `ip`. Nos campos de conteúdo, `antes`/`depois` guardam **só hash e tamanho** (D12-10).
  - `retratos` (D12-7a; C-4.13): `id`, `criado_em`, **um por avaliação** (sem coluna de categoria no retrato: a categoria vai em cada item da lista); lista congelada de (`ocorrencia_id`, `job_id`, `prompt_hmac`, `categoria`), o hash do índice de exemplos congelado, o ID/hash da versão das instruções (D12-15a) e `valido`. Depois de criado, nada muda, exceto `valido`→0, que acontece quando uma quarentena o invalida (C-4.10) ou quando se desconfirma/apaga uma ocorrência de teste que ele lista (C-4.11a).
  - `embeddings` (D12-15b): `ocorrencia_id`, `vetor` (768 floats), `modelo` (`nomic-embed-text`). Calculado **só quando o admin marca `exemplo_publico=1`** numa ocorrência confirmada, e apagado quando a marca sai ou a confirmação é desfeita (C-6.6; **não** é calculado ao confirmar).
- **Teto:** 2 GiB; acima disso, novos relatos são recusados com HTTP 507, com evento e aviso no monitor, sem apagar nada sozinho (D12-10, C-6.2). O `historico` não expira e conta no teto (C-7.9).

### 8.3 Gateway 0.13.0: rotas

- `POST /api/feedback`:
  - **rota própria** registrada no mux **antes** do `/` (`main.go:263-267`);
  - autenticação: o Bearer do nginx;
  - corpo ≤ 256 KB, `descricao` ≤ 4 KB, `resposta_correta` ≤ 128 KB e no máximo 64 jobs (C-1.17), validado como `readSmallJSON`/`parseStation`; enums da C-1.16;
  - grava a ocorrência, os jobs e as amostras **numa transação** (C-1.5);
  - devolve `{ocorrencia_id, estado, amostras_copiadas, sem_pedido, jobs_externos}`. Numa ocorrência distribuída: `estado=distribuida`,
    `amostras_copiadas=0` e `jobs_externos` = quantos jobs são de outro backend (D12-4, 3ª rev.; C-1.10). Num gateway `--role fallback`: `estado=fallback`, `amostras_copiadas=0` (C-1.21).
  - Pedido vindo do fallback: o relato vai ao gateway que criou o job (D12-1); o gateway recusa ID que não existe nele com **404 claro, sem gravar nada** (C-1.4).
  - Recusas: **503** sem `--doc-key-file` (C-4.3a); **507** com o `treino.db` acima de 2 GiB (C-6.2).
- **Monitor, só admin, com sessão, CSRF e same-site e gravando no `historico`:** `GET /monitor/api/treino` (lista),
  `GET /monitor/api/treino/{id}`, e `POST /monitor/api/treino/{id}/confirmar|descartar|completar|apagar|exemplo-publico|liberar-quarentena`
  (C-1.13, C-4.11; `liberar-quarentena` escolhe um lado para o componente inteiro e grava `alterou`, `campo=particao`).
- **Exportação:** `GET /monitor/api/treino/export?parte=treino|teste` (admin + same-site), com o predicado da D12-7 e a partição da D12-7a,
  gerada na hora em streaming, sem ficar guardada no servidor (C-3.12, C-6.7), exportando só a partição pedida e nenhuma amostra de `quarentena` (C-3.11), e registrada no `historico` com uma linha `exportou` **por ocorrência exportada** (C-7.7). Formato da linha: C-3.13 a C-3.15.

### 8.4 mcpollama 0.10.0

- **IDs:** o `ollama/client.go` guarda o `X-Request-Id` e o backend de cada resposta (`Usage.Jobs []JobRef{ID, Papel, Backend}`). O
  `tools/filetools.go` atribui os papéis (`map<i>`, `reduce<r>`, `final`). Em `usage.go`, `job=` ou `jobs=` vai na linha `[uso]` e em `_meta`.
- **(2ª rev.) `X-Mcpollama-Doc`:** nas tools de arquivo, o mcpollama calcula uma vez o sha256 do texto extraído do documento inteiro e o envia em todas as chamadas daquele pedido (map, reduce e partes) (D12-7a; C-4.1). Tools sem documento (`generate`, `chat`) não enviam o cabeçalho (C-4.15).
- **`reportar_erro`** (`tools/reportar_erro.go`):
  - schema com `enum` para categoria, severidade e origem;
  - limites: `descricao` ≤ 4 KB, `resposta_correta` ≤ 128 KB e no máximo 64 jobs (C-1.17);
  - `job_do_erro` **obrigatório** em `translate` e em qualquer lista com mais de um job e sem `final`; sem ele o relato é recusado (C-1.3);
  - `readOnlyHint=false`;
  - registrada sem depender de `--allow-root`;
  - envia ao backend do `job_do_erro` (C-1.11); se esse backend for um gateway `--role fallback`, a ocorrência nasce `fallback`, sem amostras (C-1.21).
- **Automáticos (D12-8):** o mcpollama checa as regras depois da resposta e dispara o relato **em segundo plano**, sem atrasar a resposta nem
  falhar a tool. Flag `--auto-report` (padrão: **ligado**, owner 15:50). Só as três regras da C-1.20 (JSON inválido quando o pedido tem `format` = `"json"` ou schema e o texto reprova em `json.Valid`; resposta só de espaços; `fim=length` com `num_predict` ≥ 256 ou ausente); o candidato nasce `nova`, `origem=automatico`, sem `resposta_correta` (C-1.19).

### 8.5 Protocolo de avaliação (D12-17; `bakeoff -suites erros`, a criar)

- **Entrada:** as amostras da partição **teste** que passam no predicado da D12-7, exportadas como JSONL.
  - **Replay (C-5.1):** o pedido canônico é reenviado ao modelo candidato, pelo gateway, com o `prompt_json` **igual, exceto `options.seed`**, e com `X-Mcpollama-Tool: bakeoff-erros`.
  - **Variação (C-5.15):** cada caso automático roda em **3 rodadas pareadas**, com `options.seed` = 1, 2 e 3 e as demais opções iguais ao `prompt_json`. O modelo atual e o candidato rodam com as mesmas sementes, intercalados na mesma sessão. O caso conta como acerto com **2 de 3** rodadas certas, e o relatório traz quantos casos mudaram de resultado entre rodadas. A avaliação humana às cegas usa 1 rodada (`seed`=1).
- **Pontuação por categoria:**
  - **automática** onde há critério determinístico:
    - `numeracao_errada` e `omitiu`: lista de itens comparada com a resposta certa; acerto só com precisão **e** cobertura de 100% (C-5.2);
    - `formato_invalido`: `json.Valid` **e** igualdade dos **valores** com a resposta certa, depois de normalizar espaços e ordem de chaves (2ª rev.: comparar só as chaves deixaria passar valores errados);
    - `contagem_errada` e `calculo_errado`: número final igual (C-5.4);
    - `truncado`: `fim=stop`, `resp_final=1` e `resp_truncated=0` (C-5.4);
  - **humana às cegas** em `resumo_infiel`, `inventou`, `traducao_errada` e `ignorou_instrucao`: o modo `blind` do `bakeoff` embaralha a
    saída do modelo atual e a do candidato, e **só o owner** escolhe sem saber qual é qual (decisão do owner na 2ª revisão; outra pessoa só em casos anonimizados). **(2ª rev.) O Claude
    não avalia casos com conteúdo de cliente**, porque isso mandaria o documento para fora da rede; só casos anonimizados;
  - **sem critério nesta fase:** `outro` e `idioma_errado` ficam **fora da avaliação** e contam só na estatística.
- **Mínimos (C-5.8):** uma categoria só é **reportada** com ≥ 20 casos de teste, e só **decide** com ≥ 30; os limiares abaixo valem só para categorias com ≥ 30, as demais só são reportadas. Com a **cota** da D12-7a (os 30 primeiros
  grupos de cada categoria vão para o teste), o mínimo para decidir chega com 30 **grupos** confirmados por categoria, sem depender dos 15% (C-4.6; a cota conta grupos confirmados, não casos que passam o predicado da D12-7, então o N de casos avaliáveis pode ficar abaixo de 30 mesmo com a cota cheia; contradição 1 da entrada de 2026-11-02 20:28 da §16, para o owner decidir se o contrato muda).
- **Retrato:** toda comparação diz o ID do retrato do teste usado; linha de base e candidato **sempre no mesmo retrato**.
  - Há **um retrato por avaliação** (C-4.13); um retrato novo exige uma linha de base nova (C-4.14).
  - O relatório mostra o ID do retrato, N, acerto por categoria e o intervalo de confiança (Wilson 95%) (C-5.9).
- **Limiares de aceite de uma mudança** (instrução nova, exemplos da D12-15 ou um modelo v2):
  - **melhora (C-5.10):** ganho ≥ 10 pontos percentuais na categoria-alvo, com o limite inferior do intervalo acima do **ponto** (acerto) do modelo atual, **medido de novo no mesmo retrato e na mesma sessão** (C-5.15, C-5.16);
  - **sem regressão (C-5.11):** nenhuma outra categoria cai mais de 3 pontos;
  - e o **S2/S3 da Fase 8** não cai mais de 2 pontos (C-5.12).
- **Gate (C-5.13):** falha se a suíte não rodar, se avaliar 0 casos, se nenhuma categoria atingir o mínimo de **30** ou se o retrato estiver
  com `valido=0`. Nesse caso, o relatório diz "dados insuficientes", e nunca "sem regressão".
- **Linha de base (C-5.14):** a primeira rodada, com o `qwen3-ia` atual e sem a D12-15, fica registrada no relatório da etapa como
  registro histórico. **A cada comparação, o modelo atual é medido de novo** (C-5.16); se a linha de base e essa nova medição diferirem
  em mais de 3 pontos numa categoria, o relatório avisa (mudança no servidor).
- **Quem avalia (C-5.5 a C-5.7):** `resumo_infiel`, `inventou`, `traducao_errada` e `ignorou_instrucao` por revisão humana às cegas (modo `blind`); casos com conteúdo de cliente só pelo owner (o Claude avalia só casos anonimizados); `outro` e `idioma_errado` fora da avaliação.

### 8.6 Exemplo de linha exportada (formato de chat)

```json
{"messages":[
 {"role":"system","content":"<system do prompt_json: fileSystemPrompt da tool extract>"},
 {"role":"user","content":"<prompt do prompt_json: instrução + ARQUIVO_INICIO <nonce> ... ARQUIVO_FIM <nonce>>"},
 {"role":"assistant","content":"<resposta_correta da ocorrência>"}],
 "meta":{"ocorrencia_id":"<hex32>","job_id":"<hex32>","papel":"final","tool":"extract","categoria":"numeracao_errada","particao":"treino"}}
```

Para `chat`, as `messages` vêm completas do `prompt_json`, com todo o histórico, e a última mensagem do assistente é a resposta certa (C-3.14). A linha é `{"messages":[system, user, assistant],"meta":{...}}` (C-3.13), e o `meta` traz só os seis campos acima, sem texto livre (C-3.15).

## 9. Instruções para o Claude dos projetos

- **Arquivo:** `docs/RELATAR-ERROS-IA.md` (já escrito; sem commit até o owner aprovar o plano). Diz:
  - **quando** relatar (o Claude conferiu um resultado do Qwen e está errado; ou o usuário apontou) e quando **não** relatar (erro do
    pedido, erro de infraestrutura, repetição);
  - **o que** registrar (§2 do md);
  - **como** registrar: só pela tool, sem registro alternativo;
  - **o que é uma boa correção:** resposta inteira, no formato pedido;
  - **`job_do_erro`:** obrigatório em `translate` e em qualquer lista com mais de um job e sem `final` (C-1.3), com a `resposta_correta` daquela chamada; no `translate`, só entra no treino o erro de uma parte `mapN` (C-3.10); no máximo 64 jobs por relato (C-1.17). O `docs/RELATAR-ERROS-IA.md` precisa refletir isso na 12.11.
- **Onde o Claude lê:** seção curta no `~\.claude\CLAUDE.md` global (D12-14), com o texto da §9.1. Entra na **12.11**, nunca antes.

### 9.1 Texto proposto para o `~\.claude\CLAUDE.md` global (D12-12, aplicar na 12.11)

```markdown
### MCP `mcpOllama` ativo no projeto → PREFERÊNCIA (owner, 01/11/2026)

**Se as tools `mcp__mcpOllama__*` existirem na sessão, elas são a PRIMEIRA escolha** para o trabalho que cabe ao nível
`local`, antes de ler o arquivo no meu contexto e antes de delegar a um subagente haiku:
- **Ler, resumir ou extrair de arquivo** (sobretudo grande): `summarize_file` / `extract`, em vez de `Read` + resumir eu mesmo.
  O texto não entra no meu contexto, e é aí que está a economia.
- **Traduzir, converter formato, rascunhar, reescrever:** `translate` / `generate` / `chat`.
- **Dados sensíveis** (documentos de clientes, cofre): **sempre** pelo `mcpOllama`, porque não saem da rede local.
- **Busca semântica:** `embed`.
- **Subagentes:** ao delegar uma coleta ou leitura em volume, instruir o subagente a usar as tools `mcp__mcpOllama__*` quando
  estiverem disponíveis para ele.
- **Erros do modelo:** ao conferir um resultado e achar erro, relatar com `reportar_erro` (ver `E:\MCPOllam\docs\RELATAR-ERROS-IA.md`).

**Não usar o `mcpOllama`** (ir direto ao nível Anthropic adequado) quando a tarefa for decisão, código final, verificação crítica ou
revisão adversarial; quando o arquivo estiver fora do `--allow-root`; ou quando a tool falhar ou o servidor estiver fora (aí sobe para
haiku, ver "Regras de uso"). Ao usar, indicar "nível local (Qwen)" no texto.
```

## 10. Treino (congelado até haver dados e hardware)

- **Condição para começar** (owner, 15:50): ≥ 300 ocorrências **confirmadas e exportáveis na partição de treino**, com ≥ 50 por categoria
  que se quer corrigir, **além** da cota de teste de 30 por categoria já preenchida (D12-7a), **e** um lugar para treinar.
- **Hardware:** QLoRA de um 9B pede **uma GPU de 16–24 GB** por algumas horas. As RTX 3070 de 8 GB e a fonte de 850 W não suportam
  treino longo a 100%, e foi exatamente esse cenário que derrubou o servidor. Opções:
  - GPU nova;
  - 2ª fonte + divisão entre placas, complexo;
  - nuvem **com dados anonimizados**, porque são documentos de clientes (C-6.9: treinar fora da rede exige dados anonimizados).
- **Ferramentas:** Unsloth ou LLaMA-Factory (Python), **fora deste repositório**. Antes, confirmar o suporte ao `qwen3.5` (arquitetura nova)
  e à conversão do adaptador LoRA para GGUF.
- **Volta ao Ollama:** `Modelfile` com `FROM qwen3.5:9b` + `ADAPTER <lora.gguf>` → `ollama create qwen3-ia-v2` → banco de provas (12.7, protocolo da §8.5)
  e S2/S3 da Fase 8.
  - **Só substitui o `qwen3-ia` se ganhar sem piorar o resto.**
  - A/B pelo gateway: o `--allowed-models` aceita os dois durante o teste.
- **Alternativas sem treino**, para avaliar antes (baratas, em Go):
  - instruções melhores por tool onde o modelo mais erra;
  - schema JSON forçado (`format` do Ollama);
  - conferência e nova tentativa automática;
  - **exemplos de correções passadas** buscados por embedding (`nomic`) e incluídos no pedido (few-shot), um "aprender" sem mexer nos pesos (só ocorrências confirmadas com `exemplo_publico=1`, só da partição de treino; C-8.1, C-8.3).

## 11. Metodologia de trabalho (obrigatória para quem executar)

### 11.1 Regras do owner (do `~\.claude\CLAUDE.md` global e do `CLAUDE.md` do projeto)

- **Idioma:** respostas, planos e relatórios em **pt-BR**. Código, identificadores e comentários técnicos em inglês.
- **Cabeçalho de toda resposta ao owner:** `Modelo: <modelo> · <data/hora real>`. A hora vem do sistema; nunca inventar.
- **Decisões do owner:** sempre por pergunta com opções concretas (a recomendada em 1º). Nunca decidir sozinho o que é do owner: âmbito,
  o que entra num commit, abordagem, área sensível. Passos mecânicos não viram pergunta.
- **Rigor empírico:** um falso-OK é pior que um FAIL.
  - Nunca afirmar "funciona" sem **rodar e ler a saída**.
  - Provar por evidência e por uma **2ª via independente**: para estado no servidor, conferir pelo servidor; para versão, pelo binário
    e pela API.
  - Validar com cache limpo (`go test -count=1`).
  - "Esperado" ou "ambiente" só com prova.
- **Teste de regressão que falha no código antigo e passa no novo**, sempre que se corrige um defeito.
- **Honestidade:** relatar os próprios erros de processo ("errei X, corrigi assim"), registrados no changelog do plano.
- **Planos:** atualizar **o próprio plano a cada etapa**, com detalhe e sem resumir: o que deu certo, o que não deu, a solução, os números
  medidos, os commits e as pendências numeradas, além do estado de cada etapa.
- **Commits:** stage **por caminho** (nunca `git add -A`); 1 commit por etapa verde; nunca commitar com FAIL.
  - Mensagem por arquivo (`git commit -F`), terminando com `Co-Authored-By: ...`.
  - **Push e tags só com autorização do owner.**
  - Antes de todo push, **varredura de segredos** nas linhas adicionadas (Bearer, `sk-`, `api_key=`, senha literal; meta 0).
- **Delegação:** o loop principal coordena, revisa, verifica e commita. A implementação vai para subagentes, sempre com o modelo mais
  barato que dá conta:
  - **sonnet:** implementação e documentação;
  - **opus:** revisão adversarial;
  - **haiku:** coleta simples;
  - nível **local** (o próprio `mcpOllama`): resumo e extração.
- **Sem dependências de terceiros:** Go puro + o `go.mod` atual (`go-pdfium`, `wazero`, `x/image`, `x/sys`, `modernc sqlite`). Ex.: o WMI
  da 0.9.2 foi feito por vtables COM com `syscall`, sem `go-ole`.
- **SSOT:** antes de criar algo, procurar e reaproveitar o que existe. Camada nova agrega, não reimplementa.

### 11.2 Testes e gates (o que rodar em cada mudança)

- **Sempre:** `cd src; go build ./...; go vet ./...; gofmt -l .` (vazio); `go test -count=1 ./...`; build Linux
  (`$env:GOOS='linux'; go build -o NUL ./...`).
- **`-race`:** precisa de cgo, com o gcc em `E:\MCPOllam\deps\gcc-mingw-w64\bin`:
  `$env:PATH=...; $env:CC='gcc'; $env:CGO_ENABLED='1'`. Remover as variáveis no fim.
- **Testes de ataque das revisões adversariais**, obrigatórios em mudanças nas áreas abaixo. Copiar os `*_test.go` das pastas
  `.workspace/reports/*.testes/` para os caminhos espelhados em `src/`, rodar e **remover as cópias**:
  - `src/gateway/`: Fase 9 (`MCPOllam2026-10-28-235633-revisao-adversarial-fase9.testes`), 10.L (`…2026-10-30-173000-…-10L.testes`)
    e 10.5 pool (`…2026-10-30-190200-…-10-5-pool.testes`);
  - `src/files/`: Fase 4;
  - `src/doctext/`: Fase 7;
  - `src/ocr/`: testes de tag `ocr`, com a DLL real.
- **Interface do monitor:** prova no **Firefox headless** (instalado em `C:\Program Files (x86)\Mozilla Firefox`) com o estado real
  embutido. Larguras de 570, 720, 900 e 1400 px; ler o PNG gerado.
- **Ao vivo:** **uma chamada por vez** no servidor, salvo em teste de carga autorizado pelo owner. Atenção à energia: muitas GPUs a 100%
  derrubam a fonte.
- **Revisão adversarial (opus)** em toda superfície nova exposta à rede. Aqui: `/api/feedback`, exportação e aba Erros. Os testes de ataque
  ficam guardados em `.workspace/reports/<...>.testes/`.

### 11.3 Implantação

- **Gateway:** `deploy\gateway\Instalar-Gateway.ps1 -Modo Atualizar -Versao "<versão>-<hash curto>"`. Ele compila, envia, confere o SHA-256,
  troca o binário com backup e reinicia o serviço. **Só com a fila vazia**: o padrão é um vigia em segundo plano que espera 60 s de fila
  vazia pelo `/monitor/api/state`, para não derrubar jobs de outras estações. Depois conferir a versão pela API.
- **mcpollama:** `go build -ldflags "-X main.Version=<versão>-<hash>" -o D:\MCP\mcpollama-mcp.exe ./mcpollama-mcp`.
  - Se o exe estiver em uso, vira `.exe~`; apagar depois.
  - **Só vale após reiniciar as sessões do Claude Code.** Validar depois com uma chamada real e conferir no gateway.
- **Documentação a atualizar em cada versão:** `CHANGELOG.md` (seção "Não lançado" → versão com tag), `README.md` (tabela de versões),
  `CLAUDE.md` (estado), `docs/MANUAL-GATEWAY.md`, `TOOLS.md` (se as tools mudarem) e, fora do repositório, o
  `CONEXAO-IA.md` (sincronizado com `/root` do servidor, conferindo o MD5) e a memória do projeto.

### 11.4 Ambiente e armadilhas conhecidas

- **Estação Windows 11, PowerShell 5.1:**
  - ele **remove aspas duplas** de argumentos de executáveis nativos (`ssh.exe`, `git`);
  - `Get-Content`/`Set-Content`/`Out-File` **leem como ANSI e gravam BOM**, o que já corrompeu acentos em arquivos Go;
  - here-strings enviadas a `ssh 'bash -s'` ganham um `\r` na última linha.
  - Para editar arquivos, usar as ferramentas de edição do agente ou .NET `UTF8Encoding($false)`.
- **SSH ao servidor:** sempre pelo alias **`lx-ai01-root`** (`~/.ssh/config`, com chave). Com `root@10.100.2.2` direto, o ssh pede senha e
  trava. Poucas conexões seguidas, por causa do fail2ban.
- **Segredos:**
  - token do Ollama/nginx só pelo caminho (`%USERPROFILE%\.openclaude\csl-ollama.env`), nunca impresso, em código, log, chat ou commit;
  - CA em `%USERPROFILE%\.openclaude\certs\cloudflare-origin-ca.pem`;
  - não abrir `.mcp.json`;
  - **`referencias/` é somente leitura.**
- **Áreas protegidas** (plano aprovado antes de criar, mover ou apagar): `Documentation/`, `.cursor/skills|Templates|agents|rules`.
- **Documentos de clientes:** nunca no git, em relatório, em chat ou em log, exceto o `content.db` e, nesta fase, o `treino.db` no servidor,
  com acesso de admin.

## 12. Etapas (com critério de aceite; renumeradas em 2026-11-02 por causa do contrato de dados)

| # | Etapa | Modelo | Aceite | Estado |
| --- | --- | --- | --- | --- |
| 12.0 | Aprovação do plano e das decisões abertas (§14) | owner | owner aprova | ✅ **concluída** (2026-11-02 19:45) |
| 12.1 | **Contrato de dados** (`docs/CONTRATO-DADOS-TREINO.md`, §8.1): unidade do erro, pedido canônico, regra de exportação, partição, avaliação, retenção, histórico, privacidade dos exemplos | sonnet + owner | owner aprova o contrato | ✅ **concluída** (2026-11-02 20:05): `docs/CONTRATO-DADOS-TREINO.md` 1.0.0, 119 regras; rascunho em Sonnet, conferência e fechamento em Opus; 2 ajustes por revisão externa (estado `fallback`; descartar estados de estatística) |
| 12.2 | mcpollama: `job=`/`jobs=<papel>:<id>` com o backend de cada ID, na `[uso]` e em `_meta` (D12-1); cabeçalho `X-Mcpollama-Doc` (D12-7a) | sonnet | teste com gateway falso: IDs e papéis certos no map-reduce e no fallback; o mesmo `X-Mcpollama-Doc` em todas as chamadas de um documento (contrato C-4.1; papel e backend de cada ID: C-1.2) | ✅ **concluída** (2026-11-02 20:16, sem commit ainda): Sonnet implementou, Opus revisou e repetiu o gate |
| 12.3 | gateway: `prompt_json` no `content.db` na **entrada** (D12-13; C-2.1 a C-2.8, C-2.10 a C-2.12), a partir do corpo depois de `applyPolicy`; `requests.doc_hash` como HMAC com `key_id` e flag `--doc-key-file` (C-4.2, C-4.3, C-4.3a) | sonnet | teste, um por regra: o `prompt_json` é igual, byte a byte, ao corpo que o Ollama falso recebeu, exceto pelas imagens, inclusive todo o histórico de `chat` (C-2.10); migração idempotente de banco antigo sem perder dados (C-2.7); teto de 16 MiB **por campo** com marca própria (`prompt_truncated`, `prompt_json_truncated`), `resp_final`/`resp_truncated` (C-2.3, C-2.4), e o predicado recusa `prompt_json_truncated=1` mesmo depois de o `content.db` expirar; imagem vira `{"imagem_hmac":"<hex>"}` + `tem_imagem=1` e fica não exportável (C-2.5, C-2.6); teto total de 4 GiB apagando os mais antigos (C-2.8); `prompt_hmac` e `conteudo_hmac` calculados como na C-2.12 e nunca como sha256 puro; `requests.doc_hash` gravado só como HMAC com `key_id`, o sha256 puro ausente de todo banco e log (C-4.2), e o cabeçalho `X-Mcpollama-Doc` removido antes do Ollama e ausente do `prompt_json` (C-2.11); sem `--doc-key-file` o gateway não aceita relatos (C-4.3a, verificado na 12.4) | ✅ **concluída** (2026-11-02 22:35, sem commit ainda): Sonnet implementou; revisão Opus (1 bloqueador, 2 importantes, 6 menores) corrigida; gate refeito pelo coordenador |
| 12.4 | gateway: `treino.db` (ocorrências, jobs, amostras, histórico) + `POST /api/feedback` em rota própria (D12-4, 5, 10, 16); flag `--role` (C-1.21) | sonnet | testes, um por regra do contrato nesta etapa (C-0.4): validação dos enums e limites (C-1.16, C-1.17: corpo 256 KB, `descricao` 4 KB, `resposta_correta` 128 KB, 64 jobs); `job_do_erro` inexistente → 404 sem gravar nada (C-1.4) e obrigatório em `translate`/listas sem `final` (C-1.3); transação única com as amostras de todos os jobs (C-1.5); `sem_pedido` só pelo `job_do_erro` expirado ou sem `prompt_json`, e `nova` quando expiram só os outros (C-1.9, C-1.9a); `distribuida` sem amostras com `jobs_externos` (C-1.10) e `fallback` com `--role fallback` sem amostras (C-1.21), com a precedência da C-1.9; transições da C-1.12 e confirmar sem `resposta_correta` recusado (C-1.12a); candidato automático substituído por relato humano com `alterou`/`campo=origem` (C-1.19); 503 sem `--doc-key-file` (C-4.3a); 507 no teto de 2 GiB sem apagar nada (C-6.2); apagar deixa só a linha mínima e respeita a contagem de referências (C-6.3, C-6.4); alterar a correção desfaz a confirmação e apaga o embedding (C-6.5); histórico sem texto, só hash e tamanho, e sem expirar (C-7.4, C-7.6, C-7.9); consultas com `context.Background()` (C-6.10); ataques 9.5/10.L/10.5 ok | ✅ **concluída** (2026-11-03 00:43, sem commit ainda): Sonnet implementou; revisão Opus (6 importantes, 6 menores) corrigida com o contrato 1.1.2; gate refeito pelo coordenador |
| 12.5 | mcpollama: tool `reportar_erro` (ocorrência, `job_do_erro`) + automáticos (a)–(c) (D12-2, 3, 8; C-1.3, C-1.19, C-1.20); `TOOLS.md` | sonnet | o relato chega com todos os jobs e papéis (até 64, C-1.17) e o backend de cada um; `job_do_erro` padrão `final`/`unico` e obrigatório em `translate` e em listas sem `final` (C-1.3); enums e limites (C-1.16, C-1.17); relato ao gateway do `job_do_erro` (C-1.11); os automáticos são só as três regras da C-1.20 (inclui `num_predict` ausente), nascem `nova` sem `resposta_correta` (C-1.19); o automático não atrasa nem quebra a tool; deduplicação (relato humano substitui o automático) | ✅ **concluída** (2026-11-03 10:40, sem commit ainda): Sonnet implementou; revisão Opus (pode publicar) com ajustes; decisões do owner no contrato 1.1.4; gate refeito pelo coordenador |
| 12.6 | monitor: aba "Erros (treino)", "Marcar erro", confirmar/completar/descartar/apagar/exemplo público/**liberar quarentena** (rota `POST /monitor/api/treino/{id}/liberar-quarentena`), exportação com predicado e partição (D12-6, 7, 7a; C-1.13, C-3.x, C-4.x) | sonnet | Firefox headless 570–1400 px; ações só do admin com sessão, CSRF e same-site, cada uma no histórico (C-1.13); o predicado exclui cada caso proibido, inclusive imagem, `distribuida`, `fallback`, `quarentena` e `translate` montado, **um teste por condição, com um caso que falha só nela** (C-3.16); exportação por `parte=treino\|teste` só da partição pedida, em streaming, sem guardar no servidor, com `meta` sem texto livre e uma linha `exportou` **por ocorrência** (C-3.11 a C-3.15, C-7.7); grupos por `doc_hash`, por `conteudo_hmac` ou por job em comum, com todas as não apagadas, inclusive `nova` e `descartada` (C-4.5, C-4.15); cota de 30 grupos **com amostra exportável** por categoria, na ordem em que cada grupo ganha a primeira amostra exportável, com a categoria dessa amostra e partição **pendente** antes disso (contrato 1.1.0: C-4.6, C-4.7, C-1.8); partição estável e fusão de grupos do mesmo lado pelo menor `grupo_id` sem reavaliar (C-4.8); **união de grupos já atribuídos a lados opostos vai para quarentena** e invalida os retratos e o índice de exemplos; um lado só é herdado; nenhum lado fica pendente (C-4.9 1.1.0, C-4.10, teste dedicado para cada caso); `liberar-quarentena` grava `alterou`/`campo=particao` (C-4.11); desconfirmar/descartar (C-1.12b)/apagar ocorrência de teste não libera a vaga e invalida os retratos que a listam (C-4.11a); nenhum `job_id`, `doc_hash` ou `grupo_id` em treino e teste ao mesmo tempo (C-4.12); `doc_hash` guardado como HMAC com `key_id`, nunca o sha256 puro (C-4.2); ocorrência distribuída e `fallback` sem amostras; descartar vale também para `sem_pedido`/`distribuida`/`fallback` (C-1.12); histórico sem texto sensível (C-7.6); alterar `resposta_correta`, `descricao` ou `correcao_aplicada` confirmada desfaz a confirmação (C-6.5); embedding só ao marcar `exemplo_publico` (C-6.6); consultas com `context.Background()` e cache em segundo plano (C-6.10) | ✅ **concluída** (2026-11-03 13:30, sem commit ainda): Sonnet implementou; revisão Opus corrigida; desvio: telas com login conferidas no Chromium (Playwright), o visitante no Firefox headless |
| 12.7 | banco de provas: `bakeoff -suites erros` com o protocolo da §8.5 + **retrato** do teste + **linha de base** do `qwen3-ia` atual | sonnet | relatório com o ID do retrato, N, acerto e intervalo de Wilson 95% por categoria (C-5.9); um retrato por avaliação, com os campos da C-4.13; replay com o `prompt_json` igual exceto `options.seed` e `X-Mcpollama-Tool: bakeoff-erros` (C-5.1); **3 rodadas pareadas** (seed 1/2/3), acerto com 2 de 3, modelo atual e candidato intercalados, e o relatório traz os casos que mudaram entre rodadas (C-5.15); a **linha de base** registrada e o modelo atual **medido de novo** a cada comparação, com aviso se divergirem mais de 3 pontos numa categoria (C-5.14, C-5.16); pontuação da C-5.2 (100% de precisão e cobertura), `formato_invalido` compara valores (C-5.3), C-5.4 (`contagem`/`calculo`/`truncado`); humana às cegas em `blind` com 1 rodada, conteúdo de cliente só pelo owner (C-5.5, C-5.6); `outro`/`idioma_errado` fora (C-5.7); reporta com ≥ 20 e decide com ≥ 30 (C-5.8); o gate falha com 0 casos, sem categoria com 30 casos ou com retrato `valido=0`, dizendo "dados insuficientes" (C-5.13) | ✅ **concluída no código** (2026-11-03 23:10, sem commit ainda); linha de base real só com dados (depois da 12.10) |
| 12.8 | **melhora sem treino** (D12-15): (a) instruções por tool; (b) exemplos recuperados por embedding, com a regra de privacidade | sonnet | **medido pela 12.7** com os limiares da §8.5 (C-5.10 a C-5.12); só fica ligado o que passar, por tool e por flag, com padrão desligado (C-8.8); exemplos só de ocorrências `confirmadas` com `exemplo_publico=1`, com embedding só para elas (C-8.1, C-6.6); busca só na partição `treino`, excluindo a ocorrência, o grupo, o documento em avaliação e todo caso em `quarentena` (C-8.3, C-8.4); nenhuma ocorrência desconfirmada, descartada, apagada, `sem_pedido`, `distribuida` ou `fallback` como exemplo (C-8.9); índice congelado com o retrato e hash registrado (C-8.5); análise das instruções só no treino, versão com ID e hash congelada antes da avaliação (C-8.6, C-8.7); k e limiar de similaridade fixados aqui, medidos pela 12.7 e registrados no retrato (C-8.10) | ✅ **concluída** (2026-11-04 02:22, `9e3c111`): mecanismo completo com conteúdo vazio, desligado por padrão; Sonnet implementou; revisão Opus corrigida (contrato 1.1.5) |
| 12.9 | revisão adversarial: relato forjado, flood, corpo gigante, injeção no JSONL exportado, **vazamento entre clientes pelos exemplos (D12-15b)**, **resposta do teste vazando pelos exemplos (B1)**, apagamento incompleto (histórico, amostras compartilhadas), vazamento na aba, CSRF, rota `/api/feedback`; ainda: chave HMAC (`--doc-key-file`) ausente ou diferente entre gateways (503, `key_id`, C-4.3a), gateway `--role fallback` guardando amostra (C-1.21), `liberar-quarentena` e cota manipuláveis (C-4.9 a C-4.11a), teto 507 contornável (C-6.2), apagar que deixe conteúdo além da linha mínima (C-6.3), linha de `historico` com texto (C-7.4, C-7.6) | **opus** | achados corrigidos; testes de ataque guardados em `.workspace/reports/...testes/` | ✅ **concluída** (2026-11-04 04:15, sem commit ainda): revisão Opus (0 bloqueadores, 9 importantes, 19 menores), contrato 1.1.6, correções em Sonnet; 364/364 vetores OK |
| 12.10 | gate completo + implantação (gateway 0.13.0, mcpollama 0.10.0) + aceite ao vivo com o owner | coordenador + owner | `--doc-key-file` gerado pelo instalador, 0600, fora do backup comum, e a **mesma chave** em todos os gateways que atendem as mesmas estações, com o `key_id` conferido (C-4.3, C-4.3a); gateway de fallback (se existir) com `--role fallback` (C-1.21); relato real pela tool e pelo monitor; confirmação (com `resposta_correta`, C-1.12a); exportação com o par certo, só da partição pedida (C-3.11); relatório da 12.7 antes e depois da 12.8 | pendente |
| 12.11 | documentação + **`~\.claude\CLAUDE.md` global** (§9.1 e o apontador para `RELATAR-ERROS-IA.md`) | sonnet + coordenador | instruções em vigor; o Claude de outro projeto consegue relatar; `docs/RELATAR-ERROS-IA.md` reflete o `job_do_erro` obrigatório em `translate` e em listas sem `final`, o limite de 64 jobs e os limites de tamanho (C-1.3, C-1.17), e que só o confirmado pelo admin e elegível pela C-3 entra no treino | pendente |

## 13. Riscos

1. **Correção errada ensina errado:** nada entra no treino nem nos exemplos sem **confirmação do admin** (D12-16), e o histórico registra
   quem confirmou.
2. **Volume baixo:** os automáticos geram candidatos, e a instrução global faz todo projeto relatar. A avaliação só decide com os mínimos da §8.5.
3. **Conteúdo sensível sem prazo:** D12-10 (teto de 2 GiB com 507 e sem apagar sozinho, apagar com histórico e linha mínima, exportação gerada na hora e registrada por ocorrência, backup fora ou cifrado; C-6.2, C-6.3, C-6.8, C-7.7). O `historico` não expira e conta no teto (C-7.9), por isso só guarda hash e tamanho.
4. **Vazamento entre clientes pelos exemplos (D12-15b):** exemplos **só públicos/anonimizados** (owner, 15:50), com ataque dedicado na 12.9.
5. **Teste "fácil" por vazamento:** partição por grupo (mesmo documento, mesmo `conteudo_hmac` ou job em comum, com todas as ocorrências não
   apagadas), estável, com retrato congelado, um por avaliação; exemplos só da partição de treino, com índice congelado (D12-7a, D12-15; C-4.5, C-4.13, C-8.3, C-8.5).
6. **Medir errado:** o gate falha sem dados (mínimo de 30, retrato inválido) e nunca relata "sem regressão" por falta de casos (§8.5; C-5.13); a variação do modelo é controlada por 3 rodadas pareadas e pela nova medição do modelo atual (C-5.15, C-5.16).
7. **`content.db` maior com o `prompt_json`:** teto sobe para 4 GiB com 7 dias (owner, 15:50). Medir na 12.3; se ainda apagar antes dos 7 dias, voltar ao owner (acima do teto, o contrato manda apagar os mais antigos, C-2.8).
10. **Poucos casos exportáveis:** imagem, `distribuida`, `fallback` (C-1.21) e `translate` montado ficam só na estatística. Acompanhar a proporção na aba; se for alta, reabrir com o owner.
11. **Correlação de documentos pelo hash:** o gateway guarda só o HMAC (D12-7a), com a chave num arquivo 0600 (`--doc-key-file`), **a mesma em todos os gateways que atendem as mesmas estações**, com `key_id` em cada hash; a chave fica fora do backup comum, como o `content.db`. Sem a chave o gateway responde 503 aos relatos, e trocá-la quebra os grupos antigos, só com o owner (C-4.2 a C-4.4).
8. **Impasse ou travamento em consultas novas:** padrão 0.12.6 (ctx próprio, cache em segundo plano) e teste de regressão.
9. **Relato forjado:** os dados são declarados, mas a revisão do admin é a barreira. O histórico guarda IP e estação de quem relatou.

## 14. Questões do owner (fechadas em 2026-11-02 15:50)

1. D12-8: automáticos **ligados** por padrão. → **Revisto em 2026-11-03 10:21: desligados por padrão** (contrato 1.1.4, C-1.20a).
2. D12-13: `content.db` com **teto de 4 GiB e retenção de 7 dias**.
3. D12-15b: exemplos **só públicos/anonimizados**; "mesmo projeto" fica para o futuro.
4. §8.5: limiares de +10 pontos na categoria-alvo, no máximo −3 nas outras e no máximo −2 no S2/S3; mínimos de 20 para reportar e 30 para decidir.
5. §10: treino com ≥ 300 confirmados na partição de treino e ≥ 50 por categoria.
6. Casos compostos (jobs em 2 gateways, pedidos com imagem, resultado montado do `translate`): **só estatística nesta fase** (C-1.10, C-2.6, C-3.10; e o gateway `fallback`, C-1.21).
7. Conjunto de teste: **cota de 30 grupos por categoria, na ordem de confirmação (C-4.6), + um retrato congelado por avaliação (C-4.13)**.
8. (16:00) Grupo que une lados diferentes: **quarentena** do componente inteiro e invalidação dos retratos afetados.
9. (16:00) Hash do documento: **HMAC-SHA256 com chave do gateway**, nunca o sha256 puro (C-4.2); a chave é **compartilhada** entre os gateways, com `key_id` (C-4.3a, revisão do contrato, item 10).
10. (contrato, 2026-11-02 19:5x) As 21 lacunas do rascunho do contrato foram fechadas pela sugestão conservadora de cada uma, e o owner acrescentou: a chave HMAC compartilhada entre gateways (C-4.3a, C-1.21); o agrupamento de pedidos sem documento por `conteudo_hmac` (C-2.12, C-4.5, C-4.15); e o controle de variação do bakeoff, com 3 rodadas pareadas e nova medição do modelo atual (C-5.15, C-5.16). Por coerência com a C-4.2, os hashes do pedido e da imagem viraram HMAC (`prompt_hmac`, `conteudo_hmac`, `imagem_hmac`).
11. (contrato, 2026-11-02 20:00 e 20:02, revisão externa) Estado próprio `fallback` e a flag `--role` no lugar de reaproveitar `distribuida` (C-1.21); descartar vale também para `sem_pedido`, `distribuida` e `fallback` (C-1.12).

Fora do plano, ainda sem resposta: os arquivos `.agents/`, `.codex/` e `AGENTS.md`, criados no repositório pela ferramenta da IA revisora,
entram no `.gitignore` ou ficam só fora do commit?

## 15. Revisões externas e como cada ponto foi tratado

### 15.1 Primeira revisão (GPT-6 Codex, 2026-11-02 15:23)

A revisão foi somente leitura. Os fatos de código que ela citou foram **conferidos** pelo coordenador:
- o pedido só é gravado como texto achatado (`proxy.go:483` → `joblog.go:42` → `SavePrompt`), e o JSON original não fica guardado em lugar
  nenhum;
- tudo fora de `/monitor` vai ao proxy, e caminho fora da tabela `routes` dá 404 (`main.go:267`, `proxy.go:426`).

| # | Achado | Procede? | Tratamento |
| --- | --- | --- | --- |
| P0-1 | O pedido exato não pode ser recuperado no momento do relato | sim | D12-13 refeita: captura canônica **na entrada**; etapa 12.3 |
| P0-2 | Um relato com vários jobs não define o que a correção representa | sim | ocorrência → jobs com papel → amostras; `job_do_erro` (D12-2, D12-4, §8.2) |
| P0-3 | Regra de exportação contraditória (D12-7 × risco 1) e sem exigir resposta completa | sim (erro do coordenador) | predicado único (D12-7) |
| P0-4 | Partição por `job_id` vaza exemplos relacionados | sim | partição por ocorrência e documento, estável (D12-7a) |
| P0-5 | A avaliação não define como provar melhora; a suíte não existe | em parte (criar a suíte já era a proposta) | protocolo completo, mínimos, limiares e gate que falha sem dados (D12-17, §8.5) |
| — | A fase não reduz erros sozinha (escopo) | sim | owner: escopo inclui **melhora sem treino** (D12-15, etapa 12.8) |
| — | Autoria do relato não é provada | sim | owner: **revisão do admin** + histórico completo (D12-16) |
| — | Dados permanentes sem limite operacional | sim | teto, apagamento com histórico, exportação registrada, backup (D12-10) |
| — | Candidatos automáticos sem contrato mensurável | sim | critérios objetivos, "idioma" fora, deduplicação, meta de descarte (D12-8) |
| — | Rota `/api/feedback` e IDs no fallback | sim | rota própria no mux; backend por ID (D12-5, D12-1) |
| — | O documento de instruções promete mais do que o fluxo garante | sim | `docs/RELATAR-ERROS-IA.md` ajustado: "em vigor só quando a tool existir" e "só o confirmado entra no treino" |

Achado **acrescentado pelo coordenador** na 1ª revisão: a melhora sem treino por exemplos (D12-15b) cria um risco novo de **vazamento
entre clientes**. Tratado com a regra de privacidade dos exemplos e com um ataque dedicado na 12.9.

### 15.2 Segunda revisão (GPT-6 Codex, 2026-11-02 15:39)

Fatos conferidos pelo coordenador antes de aceitar: o `translate` concatena sem chamada `final` (`tools/filetools.go:567`); não existe
identidade de projeto nos cabeçalhos; o fallback só marca `backend=fallback` somado. Os achados atingiram sobretudo o que o coordenador
tinha acrescentado na 1ª rodada (D12-15).

| # | Achado | Procede? | Tratamento |
| --- | --- | --- | --- |
| B1 | A busca de exemplos pode trazer a resposta do próprio caso de teste | sim, grave | busca só na partição de treino; índice congelado junto com o retrato; exclui a ocorrência, o grupo e o documento em avaliação (D12-15) |
| B2 | Filtro "mesmo projeto" sem identidade confiável | sim (furo do coordenador) | só exemplos públicos/anonimizados; "mesmo projeto" no futuro (D12-15, owner) |
| B3 | Ocorrência com jobs em 2 gateways não cabe numa transação | sim | `distribuida`, só estatística nesta fase (D12-7 item 8, owner) |
| B4 | Origem do hash do documento indefinida; job em várias ocorrências vaza entre treino e teste | sim | `X-Mcpollama-Doc` + grupos por documento ou job em comum (D12-7a) |
| B5 | A avaliação pode aprovar melhora falsa (JSON só por chaves; categorias sem critério; mínimos × 15% incompatíveis; teste não congelado) | sim | comparar valores; `outro`/`idioma_errado` fora; cota de 30 por categoria; retrato congelado (D12-7a, §8.5) |
| — | Imagens não dão replay fiel | sim | `tem_imagem`: não exportável e fora da avaliação (D12-13, D12-7 item 7) |
| — | `translate` sem `final` | sim | só estatística, salvo relato apontando a parte `mapN` (D12-7 item 9, owner) |
| — | Apagar não apaga tudo (histórico com texto; amostras compartilhadas); alterar correção confirmada | sim | histórico só com hash e tamanho; contagem de referências; alterar desfaz a confirmação e o embedding (D12-10) |
| — | Avaliação às cegas pelo Claude manda conteúdo de cliente para fora | sim (furo do coordenador) | só o owner ou casos anonimizados (§8.5) |
| — | Teto de 16 MiB compartilhado ou separado | sim | separado, com marca de truncado própria (D12-13) |
| — | §6 chamava de "pedido exato" o texto achatado | sim | alinhado com a §5.1 |

### 15.3 Terceira revisão (GPT-6 Codex, 2026-11-02 15:59)

A revisão conferiu o plano de 715 linhas e apontou que a 2ª rodada não tinha sido incorporada por inteiro. Todos os pontos procedem; os dois
bloqueadores foram **erros do coordenador** na edição de 15:54.

| # | Achado | Procede? | Tratamento |
| --- | --- | --- | --- |
| C1 | União de grupos em lados diferentes: só a ocorrência nova mudava de lado; os grupos antigos seguiam separados (vazamento) | sim, bloqueador (erro do coordenador) | quarentena do componente inteiro + retratos e índice invalidados; o admin libera escolhendo um lado (D12-7a d, owner 16:00) |
| C2 | O predicado não conferia `prompt_json_truncated`; a amostra não guardava essa marca nem `tem_imagem` | sim, bloqueador (erro do coordenador) | predicado item 5; as marcas são copiadas para a amostra no relato (D12-7, §8.2) |
| C3 | Cópia de ocorrência distribuída indefinida (bancos diferentes) | sim | só ocorrência + referências, nenhuma amostra; `estado=distribuida`, `jobs_externos` na resposta (D12-4, D12-5, §8.3) |
| C4 | Instruções reescritas podiam aprender com o teste | sim | análise só no treino; versão das instruções congelada e registrada no retrato (D12-15a) |
| C5 | `RELATAR-ERROS-IA.md` prometia "entra na base de treino"; §4 sem a exceção do `translate` | sim | "fica como candidato; só é elegível depois de corrigido e confirmado"; exceção `mapN` na §4 |
| — | Critério automático "a instrução pede JSON" sem definição | sim | só o campo `format` declarado conta (D12-8a) |
| — | sha256 puro do documento permite correlação e teste de palpites | sim | HMAC-SHA256 com chave do gateway em arquivo 0600 (D12-7a, owner 16:00) |

## 16. Changelog do plano

- 2026-11-01 21:35: criado a pedido do owner. Decisões D12-1…D12-11 propostas, aguardando aprovação.
- 2026-11-01 21:40: ajuste do owner: **preferência pelo `mcpOllama`** no `CLAUDE.md` global (D12-12), **só no plano**, aplicada na 12.9.
  - Erro de processo do coordenador: aplicou o texto no global antes da hora. O owner interrompeu, e o texto foi **revertido na hora**
    (o global voltou ao estado anterior, conferido).
- 2026-11-01 21:45: decisões do owner:
  - instruções no `~\.claude\CLAUDE.md` global, entrando na 12.9 junto com a tool (D12-14);
  - **sem registro provisório**: D12-9 e 12.4 saem.
  - O owner pediu **ajustes** antes de aprovar.
- 2026-11-02 15:10: a pedido do owner, o documento foi **reescrito para ser autossuficiente**, para revisão por outra IA. Entraram:
  - §0 (como ler) e §1–§5: projeto, estado atual, arquitetura, estrutura e os dados existentes com `arquivo:linha`, levantados no código
    por um agente de busca nessa data;
  - §8: desenho técnico detalhado;
  - §11: metodologia completa (regras do owner, gates, implantação, armadilhas);
  - §12: critérios de aceite por etapa;
  - §14: questões abertas;
  - **D12-13**, nova, sobre o formato do pedido: o `content.db` guarda texto rotulado, não o JSON;
  - fatos novos que mudam o desenho:
    - cada parte do map-reduce é um job com ID próprio, então `jobs=` lista todos;
    - a `reportar_erro` será a 1ª tool que escreve (`readOnlyHint=false`);
    - os padrões do código e da unit de produção diferem; vale a unit.
  - Nenhuma decisão do owner foi alterada.
- 2026-11-02 15:40: **revisão externa incorporada** (GPT-6 Codex, 15:23; detalhe na §15). Decisões do owner (15:30):
  - **escopo** = base + medição + **melhora sem treino** (D12-15);
  - **confiança** = revisão do admin + histórico (D12-16);
  - incorporar **tudo**, com uma **etapa de contrato de dados** (12.1) antes de qualquer código.

  Mudanças:
  - D12-1, 2, 4, 5, 7, 8, 10 e 13 revistas;
  - novas: D12-7a (partição), D12-15, D12-16 e D12-17 (protocolo de avaliação);
  - §8 reescrita: contrato, dados em 3 níveis, rotas, mcpollama, protocolo de avaliação;
  - etapas renumeradas de 12.0 a 12.11;
  - riscos e questões em aberto atualizados.

  Fatos conferidos no código antes de aceitar os achados: `proxy.go:483`, `joblog.go:42`, `main.go:267`, `proxy.go:426`.

  **Erro do coordenador** apontado pela revisão: a D12-13 anterior ("copiar o JSON no relato") era inviável, e a D12-7 contradizia o risco 1.
- 2026-11-02 15:54: **2ª revisão externa incorporada** (GPT-6 Codex, 15:39; §15.2). Decisões do owner (15:50): exemplos só
  públicos/anonimizados; casos compostos só como estatística; teste por cota de 30 por categoria + retrato congelado; padrões da §14 aceitos.
  Mudanças: D12-7 (itens 7–9); D12-7a reescrita (`X-Mcpollama-Doc`, grupos, cota, retrato); D12-8 ligado por padrão; D12-10 (histórico
  sem texto, contagem de referências, alteração desfaz a confirmação); D12-13 (tetos separados, imagem, 4 GiB); D12-15 (só públicos, busca
  só no treino, índice congelado); §6; §8.2 (`doc_hash`, `grupo_id`, `distribuida`, `retratos`); §8.4; §8.5 (valores, categorias fora,
  só o owner avalia conteúdo de cliente, retrato); §10; aceites da §12; riscos 4, 5, 7 e 10; §14 fechada. **Erros do coordenador** apontados
  e corrigidos: o filtro "mesmo projeto" e a avaliação às cegas pelo Claude, ambos introduzidos na 1ª rodada.
- 2026-11-02 16:05: **3ª revisão externa incorporada** (GPT-6 Codex, 15:59; §15.3). Decisões do owner (16:00): grupo que une lados
  diferentes vai para **quarentena** (componente inteiro, retratos invalidados); hash do documento guardado como **HMAC-SHA256** com chave do
  gateway. Mudanças: D12-4 e D12-5 (ocorrência distribuída sem amostras); D12-7 itens 5 e 8 (`prompt_json_truncated`, quarentena); D12-7a
  (HMAC, quarentena); D12-8a (só `format`); D12-15a (instruções só do treino, versão congelada); §8.2 (marcas na amostra, estado `distribuida`,
  partição `quarentena`, retrato com a versão das instruções e `valido`); §8.3 (`jobs_externos`); aceites 12.3 e 12.6; risco 11; §14 itens 8 e
  9; `RELATAR-ERROS-IA.md` (§3 candidato, §4 exceção do `translate`). **Erros do coordenador:** os dois bloqueadores (C1, C2) nasceram na
  edição de 15:54, que tinha sido relatada como completa.
- 2026-11-02 19:47: **plano aprovado** pelo owner (19:45, "ok, execute o plano"); 12.0 concluída. Início da 12.1: rascunho do
  `docs/CONTRATO-DADOS-TREINO.md` delegado ao Sonnet, só a partir do plano, sem decisões novas (lacunas vão para "Questões para o owner").
- 2026-11-02 20:07: **12.1 concluída.** Contrato de dados 1.0.0 aprovado pelo owner (20:05), com 119 regras `C-x.y`.
  - O que deu certo: o rascunho do Sonnet (109 regras + 22 lacunas) ficou fiel ao plano. A conferência em Opus corrigiu 4 pontos: C-1.5 × distribuída, C-2.10 × imagem, a sugestão da questão 7 (`--max-chunks` já inclui os reduces) e a questão "quem avalia às cegas", resolvida pela decisão anterior do owner.
  - Lacunas: o owner aceitou as 21 sugestões conservadoras e acrescentou três pontos, a chave HMAC compartilhada (C-4.3a, C-1.21), o agrupamento por `conteudo_hmac` (C-2.12, C-4.5, C-4.15) e o controle de variação do bakeoff (C-5.15, C-5.16). Por coerência, `prompt_sha256` virou `prompt_hmac` e a imagem virou `imagem_hmac`.
  - O que não deu certo: a C-1.21 reaproveitava o estado `distribuida` para o fallback (erro do coordenador, apontado pela revisão externa), e a C-1.12 não deixava descartar estados de estatística. Solução: estado próprio `fallback`, flag `--role`, precedência na C-1.9 e descarte permitido, com a C-1.14 alinhada.
  - Plano: §8.2 alinhada (estado `fallback`, `prompt_hmac`/`conteudo_hmac`/`key_id`, retrato por avaliação). Onde houver divergência, **o contrato prevalece**. `docs/README.md` passou a indexar o contrato e o `RELATAR-ERROS-IA.md`.
  - Início da 12.2.
- 2026-11-02 20:16: **12.2 concluída** (mcpollama, sem tocar em `src/gateway/`, `src/files/`, `src/doctext/` e `src/ocr/`). A implementação foi do Sonnet; a revisão do diff e o gate foram refeitos pelo Opus.
  - **Arquivos:**
    - `ollama/api.go`: `recordJob` lê o `X-Request-Id` (só 32 hex minúsculos) na resposta **usada**, inclusive na do fallback, e envia `X-Mcpollama-Doc`;
    - `ollama/client.go`: `WithDoc`, que só aceita 64 hex minúsculos;
    - `ollama/types.go`: `JobID` em `Usage` e em `EmbedResponse`;
    - `tools/usage.go`: `jobRef{ID,Role,Backend}`, o sufixo `job=`/`jobs=papel:id,…` com `@fallback`, e `jobs` no `_meta` `mcpollama/usage`, que já existia;
    - `tools/filetools.go`: sha256 de `f.Text` uma vez por pedido; papéis `unico`/`mapN`/`reduceN`/`final`; no `translate`, só `mapN`;
    - `generate.go`/`chat.go`/`embed.go`: papel `unico`;
    - teste novo: `tools/jobs_test.go`.
  - **Decisões de implementação, coerentes com o contrato:**
    - o formato `job=` ou `jobs=` depende do número de **chamadas**, não do número de IDs;
    - `reduceK` conta cada chamada intermediária de redução, e a última chamada é a `final`;
    - uma chamada que falha não entra na lista;
    - `Usage.Backend` continua `""` no primário, então `backend=` na linha não mudou;
    - `TOOLS.md` ficou inalterado (`TestCatalogInSync` ok).
  - **Exemplo real:** `... fim=stop backend=fallback job=<32hex>@fallback`; map-reduce: `... partes=6 fim=stop jobs=map1:…,…,reduce1:…,…,final:…`.
  - **Testes novos (9, PASS):** `TestUsageLineSingleJob`, `TestUsageLineWithoutRequestIDIsUnchanged`, `TestUsageLineIgnoresInvalidRequestID`, `TestEmbedHasJobAndNoDocHeader`, `TestMapReduceRolesAndDocHeader`, `TestSingleCallFileToolIsUnico`, `TestTranslateHasOnlyMapRoles`, `TestFallbackJobCarriesSuffixAndBackend`, `TestFallbackInMapReduceKeepsRoleAndDocHeader`.
  - **Gate (Opus, lido):**
    - `go build ./...` 0; `go vet ./...` 0; `gofmt -l` vazio;
    - `go test -count=1 ./...` 20 pacotes ok, 0 FAIL;
    - `go test -race -count=1 ./...` 20 ok, 0 FAIL, sem data race.
  - **O que não deu certo:** nada no código. Um diagnóstico do IDE apontava o helper `usageMeta` como não usado, o que sugeria `_meta` sem teste. Conferido: era um diagnóstico **velho**; o helper se chama `jobsMeta` e é usado em 5 testes.
  - **Pendências:**
    1. CHANGELOG `mcpollama` 0.10.0 fica para a 12.10 (versão única com a 12.5).
    2. O binário em `D:\MCP` só muda na implantação (12.10).
    3. Até a 12.3, o gateway em produção repassa o `X-Mcpollama-Doc` ao Ollama, que ignora o cabeçalho. A C-2.11 vale a partir da 0.13.0, e os dois vão juntos na 12.10.
- 2026-11-02 20:28: **Plano sincronizado com o contrato 1.0.0** (`docs/CONTRATO-DADOS-TREINO.md`, aprovado 20:05). Só este plano foi editado; o contrato, o código e as seções §0–§5, §11, §15 e as entradas antigas da §16 ficaram intactos; as linhas 12.0 a 12.2 da §12 só ganharam a citação C-4.1/C-1.2 no "Aceite" da 12.2. Onde havia divergência, o contrato prevaleceu (C-0.3). O que mudou, por seção:
  - **Cabeçalho e §6:** data da última atualização; parágrafo novo dizendo que cada um dos 5 itens tem regra numerada no contrato (C-1 a C-8), que prevalece (C-0.3).
  - **D12-1:** `papel`, `backend` e `ordem` por job (C-1.2); no máximo 64 jobs (C-1.17); marcada como implementada na 12.2.
  - **D12-2:** `job_do_erro` padrão `final`/`unico` e **obrigatório** em `translate` e em listas sem `final`, com recusa sem ele (C-1.3); enums estritos (C-1.16); limites 256 KB/4 KB/128 KB/64 jobs (C-1.17); confirmar exige `resposta_correta` (C-1.12a).
  - **D12-3:** citada a C-1.16 (enum estrito).
  - **D12-4:** `sem_pedido` **só** pelo `job_do_erro` expirado ou sem `prompt_json`; `nova` quando expiram só os outros jobs, com as referências mantidas (C-1.9, C-1.9a). O texto antigo "job já expirado → `sem_pedido`" saiu. Acrescentados: precedência `distribuida` > `fallback` > `sem_pedido`; transação única e amostra por `job_id` compartilhada (C-1.5, C-1.6); resposta da distribuída com `jobs_externos` (C-1.10); estado `fallback`, flag `--role` e papel do gateway (C-1.21); lista de estados (C-1.8); transições exatas e o descarte de `sem_pedido`/`distribuida`/`fallback` (C-1.12); regras de estatística (C-1.14, C-1.15).
  - **D12-5:** 404 sem gravar para `job_do_erro` inexistente (C-1.4); jobs externos só como referência e relato ao gateway do `job_do_erro` (C-1.11); 503 sem `--doc-key-file` (C-4.3a); 507 no teto (C-6.2); limites e enums; dados declarados e não autenticados (C-1.18).
  - **D12-6:** ações do admin completas (incluindo marcar exemplo público e **liberar quarentena**) com sessão, CSRF e same-site (C-1.13); rota `POST /monitor/api/treino/{id}/liberar-quarentena` com `alterou`/`campo=particao` (C-4.11); `contexto próprio` nas consultas (C-6.10).
  - **D12-7:** item 8 passou a excluir também `fallback` (C-3.9); lista de exclusões com `fallback` e `quarentena`; acrescentadas as regras de exportação C-3.1 (caminho único), C-3.11 (`parte`), C-3.12 (admin, streaming, `exportou`), C-3.13 a C-3.15 (formato e `meta`), C-3.16 (um teste por condição) e C-2.9 (a amostra guarda as marcas).
  - **D12-7a:** (a) citadas C-4.1 a C-4.4, a chave fora do backup comum, a **chave compartilhada** entre primário, fallback e pool, o `key_id`, o 503 sem `--doc-key-file` e `doc_hash` vazio nas tools sem documento (C-4.15); (b) o agrupamento também por `conteudo_hmac` e com **todas as não apagadas**, inclusive `nova` e `descartada` (C-4.5); (c) "primeiros" = **ordem de confirmação**, categoria do grupo = a da primeira confirmada, partição atribuída na primeira confirmação (C-4.6), e 15% pela C-4.7; (d) fusão de grupos do mesmo lado pelo menor `grupo_id`, sem reavaliar (C-4.8), a **C-4.11a** (desconfirmar/apagar não libera a vaga e invalida retratos) e a C-4.12; quarentena com a rota de liberação (C-4.9 a C-4.11); (e) **um retrato por avaliação** com os campos da C-4.13 e a C-4.14.
  - **D12-8:** `num_predict` ≥ 256 **ou ausente** (C-1.20); só essas regras automáticas; candidato `nova`/`automatico`/sem resposta; substituição por relato humano com `alterou`/`campo=origem` (C-1.19); `descartada` conta só na taxa de descarte (C-1.14).
  - **D12-10:** 507 no teto, com evento e aviso (C-6.2); arquivo 0600 sem expiração (C-6.1); **apagar deixa uma linha mínima** (C-6.3), e o texto antigo "apaga a ocorrência, os jobs e as amostras" foi corrigido; backup (C-6.8); `desconfirmou` e os três campos que desfazem a confirmação (C-6.5); **embedding só com `exemplo_publico=1`** (C-6.6); `historico` sem expiração, mantido após `apagou` e contado no teto (C-7.9); `exportou` por ocorrência (C-7.7); hash e tamanho (C-7.4, C-7.6, C-7.8).
  - **D12-13:** marcador `{"imagem_hmac":"<hex>"}` (C-2.5), no lugar de "marcador com o hash"; `prompt_json` byte a byte igual ao corpo do Ollama, sem `X-Mcpollama-Doc` (C-2.10, C-2.11); resposta com 16 MiB e `resp_final` (C-2.4); migração idempotente (C-2.7); `prompt_hmac` e `conteudo_hmac` (C-2.12) no lugar de `prompt_sha256`; imagem só na estatística (C-2.6).
  - **D12-15:** regras C-8.1 a C-8.10 (exemplos só públicos e do treino, exclusões, índice congelado, versão das instruções, flag desligada, k e limiar fixados na 12.8 e registrados no retrato), embedding só de `exemplo_publico` (C-6.6) e invalidação do índice por quarentena (C-4.10).
  - **D12-16:** campos do `historico` (C-7.1), lista de `acao` (C-7.2), mesma transação (C-7.3), `quem`/`ip`/`quando` (C-7.5), ações do admin (C-1.13) e o teste de varredura (C-7.6).
  - **D12-17:** gate também com **retrato `valido=0`** e mínimo de **30** (C-5.13); 3 rodadas pareadas (seed 1/2/3, 2 de 3) e nova medição do modelo atual (C-5.15, C-5.16).
  - **§8.1:** contrato 1.0.0 aprovado, 119 regras; regra de precedência, teste por regra, semver (C-0.3, C-0.4, C-0.6).
  - **§8.2:** `doc_hash` com `key_id`; flags `--doc-key-file` e `--role`; `content.db` com C-2.1, C-2.3, C-2.5, C-2.8, C-2.9; `doc_hash` da ocorrência com `key_id`; **a coluna `distribuida` (0/1) saiu** da tabela `ocorrencias` (o `estado` a cobre: C-1.8, C-1.10, C-1.21); linha mínima da apagada (C-6.3); `amostras` com C-1.6/C-1.7; **`retratos` sem a coluna `categoria` de nível de retrato** e com a regra de `valido` (C-4.13, C-4.10, C-4.11a); **`embeddings` não é mais "calculado ao confirmar"**, e sim só com `exemplo_publico=1` (C-6.6); teto com 507 e o `historico` contado nele.
  - **§8.3:** limites da C-1.17, transação (C-1.5), resposta do `fallback`, 404 (C-1.4), 503 e 507; rota `liberar-quarentena`; exportação em streaming, só a partição pedida, `exportou` por ocorrência.
  - **§8.4:** `X-Mcpollama-Doc` sem documento (C-4.15); `reportar_erro` com 64 jobs e `job_do_erro` obrigatório (C-1.3); envio ao gateway do `job_do_erro` e estado `fallback`; as três regras automáticas da C-1.20.
  - **§8.5:** replay com `options.seed` como única diferença (C-5.1); 3 rodadas pareadas (C-5.15); acerto só com precisão **e** cobertura de 100% (C-5.2); `truncado` com `resp_final=1` e `resp_truncated=0` (C-5.4); mínimos de C-5.8 (limiares só com ≥ 30); um retrato por avaliação (C-4.13, C-4.14); relatório com ID do retrato (C-5.9); ponto do modelo atual **medido de novo** (C-5.10, C-5.16); C-5.11 e C-5.12; gate da C-5.13; linha de base como registro histórico (C-5.14) com aviso de divergência > 3 pontos; quem avalia (C-5.5 a C-5.7). A cota passa a ser descrita em **grupos** (C-4.6).
  - **§8.6 e §9:** citadas C-3.13 a C-3.15; o `RELATAR-ERROS-IA.md` precisa refletir `job_do_erro` obrigatório, 64 jobs e a exceção do `translate` (C-1.3, C-1.17, C-3.10).
  - **§10:** C-6.9 (anonimização) e C-8.1/C-8.3 nos exemplos sem treino.
  - **§12 (Etapa e Aceite):** **12.3** (C-2.1 a C-2.12, C-4.2, C-4.3, C-4.3a, `--doc-key-file`); **12.4** (C-1.3 a C-1.21 nesta etapa, 404/503/507, apagar com linha mínima, histórico, `--role`); **12.5** (C-1.3, C-1.16, C-1.17, C-1.19, C-1.20); **12.6** (liberar quarentena, C-3.x com um teste por condição, C-4.5 a C-4.15, C-6.5, C-6.6, C-6.10, C-7.6, C-7.7); **12.7** (C-5.1 a C-5.16 e C-4.13); **12.8** (C-8.1 a C-8.10 e C-6.6); **12.9** (ataques novos: chave HMAC, `--role fallback`, quarentena e cota, 507, linha mínima, histórico); **12.10** (`--doc-key-file` e a mesma chave em todos os gateways, `--role`); **12.11** (conteúdo do `RELATAR-ERROS-IA.md`).
  - **§13:** riscos 3, 5, 6, 7, 10 e 11 com as regras (C-6.2, C-6.3, C-7.9, C-4.5, C-4.13, C-5.13, C-5.15, C-5.16, C-2.8, C-1.21, C-4.2 a C-4.4).
  - **§14:** itens 6, 7 e 9 ajustados; itens 10 e 11 novos, registrando as decisões do owner na revisão do contrato (lacunas, chave compartilhada, `conteudo_hmac`, variação do bakeoff, estado `fallback`, descartar estados de estatística).
  - **Contradições e lacunas achadas no PRÓPRIO contrato (não corrigidas aqui; aguardam o owner):**
    1. A cota da C-4.6 conta **grupos confirmados**, mas a C-5.8 exige ≥ 30 **casos** que passem o predicado do cap. 3 (sem `translate` montado, sem imagem etc.); com a cota cheia, o N avaliável pode ficar abaixo de 30, e a categoria só seria reportada.
    2. A C-1.7 (campos da amostra) não lista `key_id` nem a marca de imagem `imagem_hmac`, mas a C-4.3a diz que cada hash gravado leva `key_id` e a C-2.5 usa `imagem_hmac`; o plano manteve `key_id` na amostra.
    3. A C-4.5 inclui `nova` nos grupos, mas a partição só é atribuída na primeira confirmação (C-4.6); a C-4.9 fala em "lados" de grupos que podem ter ocorrências só `nova`, sem lado.
    4. A C-4.3a manda usar a mesma chave nos "backends do pool", mas a C-1.2 só admite `backend` ∈ {`primario`, `fallback`}, sem identificar um backend do pool.
    5. A C-1.12 não prevê `confirmada`→`descartada` nem `descartada`→`apagada` de forma explícita (só "qualquer estado→`apagada`", o que cobre o segundo); `descartar` uma já confirmada exige antes desconfirmar (C-6.5), o que não está escrito.
    6. A tabela de Rastreabilidade do contrato lista "C-7.1 a C-7.8" e não inclui a C-7.9 na sua linha do cap. 7 (ela só aparece na última linha, de revisão); questão menor de rastreabilidade, sem efeito nas regras.
- 2026-11-02 20:34: **contrato 1.1.0** aprovado pelo owner (20:33), com as precisões da revisão externa, para fechar 5 das 6 lacunas que a sincronização achou (a 6ª era a rastreabilidade).
  - (1) **Cota:** conta só grupos com amostra exportável; a categoria e a ordem vêm dessa primeira amostra, e a partição fica pendente antes dela (C-4.6, C-4.7, C-1.8).
  - (2) `key_id` na amostra (C-1.7).
  - (3) **Quarentena:** só quando grupos já atribuídos a lados opostos se ligam; se só um lado aparece, ele é herdado; se nenhum, fica pendente (C-4.9).
  - (4) **Chave:** só nos gateways, não nos Ollamas do pool (C-4.3a).
  - (5) **`confirmada`→`descartada` direto** (C-1.12, C-1.12b, C-4.11a): apaga o embedding, zera `exemplo_publico`, invalida retratos e não libera a vaga; uma exportação já baixada não é desfeita.
  - (6) Rastreabilidade C-7.1 a C-7.9.
  - Plano: D12-7a (chave e cota) alinhada.
  - Ordem dos trabalhos: a **12.2 foi implementada antes** do pedido do owner de sincronizar primeiro (20:17); o pedido chegou com ela em andamento. Revisão depois da sincronização: o código da 12.2 cumpre as regras do 1.1.0 (C-1.2, C-1.3, C-4.1), e nenhuma mudança 1.1.0 atinge o mcpollama.
- 2026-11-02 20:41: **revisão independente do código da 12.2** (Opus, só leitura; o revisor não tinha shell, e o gate que vale é o do coordenador): **0 bloqueadores**.
  - **I-1:** o gateway 0.12.6 em produção repassa `X-Mcpollama-Doc` ao Ollama (`gateway/proxy/proxy.go:190-192`, a lista de remoção não tem esse cabeçalho). **Condição de implantação (12.10): o mcpollama 0.10.0 só vai para `D:\MCP` depois do gateway 0.13.0**; a remoção entra no aceite da 12.3, com teste no molde de `policy_test.go:164`.
  - **M-1:** `translate` de uma parte recebia `unico`, e a C-3.10 só aceitava `mapN`. Owner (20:40): o contrato aceita `unico` → **contrato 1.1.1**.
  - **M-2:** o `--max-chunks` não tem teto. Owner: só documentar (C-1.17 e §5 do `RELATAR-ERROS-IA.md`); a `reportar_erro` (12.5) recusa com mensagem clara acima de 64.
  - **M-3:** falha no meio do stream de `generate`/`chat` sem `job=` → corrigir.
  - **6 lacunas de teste:** 503 com ID e depois fallback sem ID; retentativa no primário; 2 ou mais rodadas de reduce; `final` = texto devolvido; fallback em map-reduce com todos os papéis; `chat` do mock sem `IsError`.
  - M-3 e as lacunas foram delegados ao Sonnet que implementou a 12.2.
- 2026-11-02 20:53: **correções da revisão da 12.2 concluídas** (Sonnet). Gate refeito pelo coordenador.
  - **M-3:** `partialFailure(partial, err, jobRef)` acrescenta `job=<id>` (com `@fallback` quando for o caso) ao texto parcial; sem ID, o texto fica idêntico ao de antes. Vale para `generate` e `chat`.
  - **Testes novos em `tools/jobs_more_test.go`:**
    - `TestNon2xxRequestIDIsNeverRecorded`;
    - `TestRetryOnPrimaryUsesTheSecondRequestID`;
    - `TestReduceRoundsHaveContinuousRolesAndFinalIsTheResult`: 6 maps, 11 reduces em 3 rodadas, e o resultado vem do `final`;
    - `TestPartialFailureNamesTheJob`.
  - **Testes reforçados em `jobs_test.go`:**
    - `TestFallbackInMapReduceKeepsRoleAndDocHeader` confere todos os papéis, o backend no `_meta` e o número de chamadas;
    - `TestEmbedHasJobAndNoDocHeader` confere que não há `IsError` em `generate`/`chat`, e o mock de `/api/chat` passou a responder no formato do chat.
  - **Prova por mutação:** cada regra foi quebrada localmente (o teste deu FAIL) e o código foi restaurado. O coordenador conferiu o código depois (`recordJob` só após 2xx; `Store("")`; contador `rounds` contínuo; `final`).
  - **Gate do coordenador:**
    - `go build ./...` 0; `go vet ./...` 0; `gofmt -l` vazio;
    - `go test -count=1 ./...` 20 ok, 0 FAIL;
    - `-race -count=1` 20 ok, 0 FAIL, sem data race.
  - **Incidente (Sonnet):** um comando de mutação com heredoc aninhado travou e foi para segundo plano; os arquivos foram restaurados a partir das cópias e conferidos com `diff`/`cmp`. Nenhum processo `go` ficou rodando (conferido). Rascunhos do scratchpad foram sobrescritos, sem efeito no repositório.
- 2026-11-02 20:56: **12.2 publicada** (`1fd5b27` docs, `1bb42fc` código; varredura de segredos 0; push autorizado pelo owner). **Início da 12.3** (gateway) no Sonnet, com o escopo do aceite da §12: `prompt_json` na entrada, tetos separados, imagem → `imagem_hmac`, remoção e HMAC do `X-Mcpollama-Doc`, `--doc-key-file`, migrações, `content.db` com 4 GiB e funções `prompt_hmac`/`conteudo_hmac`; mais os ataques 9.5, 10.L e 10.5 de novo e prova por mutação.
- 2026-11-02 21:57: **12.3 implementada** pelo Sonnet (54 min, 42 mutações, todas com FAIL e restauradas com `cmp`). Gate refeito pelo coordenador. Revisão independente em Opus em andamento.
  - **Arquivos novos:**
    - `gateway/hmackey/`: `Load` (hex ≥ 64, recusa permissão de grupo/outros e lê pelo handle, sem TOCTOU), `ID`, `MAC`, `DocHash`, `PromptHMAC` e `ContentHMAC`, com vetores fixos calculados por `openssl`;
    - `proxy/promptjson.go`: o marcador de imagem por splice de bytes, com chave insensível a maiúsculas e a escapes `\u`;
    - testes `fase12_*` em `content`, `store`, `joblog` e `cmd`, mais `proxy/promptjson_test.go`.
  - **Arquivos alterados:**
    - `content.go`: `prompt_json`, `prompt_json_truncated` e `tem_imagem` por migração idempotente; `SavePromptJSON` com tetos separados; teto total de 4 GiB, contando o JSON;
    - `proxy.go`: remove o `X-Mcpollama-Doc` e grava só o HMAC e o `key_id`;
    - `queue.go`: `DocHash`/`DocKeyID`, fora do snapshot e do monitor;
    - `store`: colunas `doc_hash`/`doc_key_id`;
    - `joblog`: `EnqueuedJSON`;
    - `config.go`/`main.go`: `--doc-key-file` (`MCPGW_DOC_KEY_FILE`, absoluto); com chave inválida, não sobe (exit 2); o log mostra só o `key_id`;
    - o `deploy/gateway/mcpollama-gateway.service` não foi tocado, porque não fixa `--content-max-bytes`.
  - **Gate (coordenador):**
    - `go build ./...` 0; `go vet ./...` 0; `gofmt -l` vazio;
    - `go test -count=1 ./...` **21 ok**, 0 FAIL;
    - `-race -count=1` 21 ok, 0 FAIL.
  - **Ataques (Sonnet; o resultado é dele, não repetido pelo coordenador):**
    - Fase 9: 463 PASS, 0 FAIL. O `fase9_adv_test.go` do monitor **já não compilava no HEAD**, desde a 0.12.6 (`h.extraAt` sumiu). Foi adaptado só na cópia temporária, zerando `extras.have/at` sob o mutex.
    - 10.5: 244 PASS, 0 FAIL.
    - 10.L: 150 PASS, **1 FAIL** (D3 `TestAdvLTotalCapUnderConcurrency`, contador diferente da soma real por 8 a 16 KiB). Medido como **preexistente**, num binário do HEAD limpo em worktree. Isolado, 40 execuções: HEAD 5 falhas, 12.3 2 falhas. Pacote completo, 6 execuções: 1 e 1. É uma corrida entre `enforceCap` e o sink. Fora do escopo, mas vira **pendência**.
  - **Flake:** `-race` no `gateway/models` (`TestLatestNeverQueriesAndLoopRespectsInterval`, "cache empty"), pacote não tocado. Deu 6/6 ok de novo no Sonnet e ok no gate do coordenador. **Pendência:** acompanhar.
  - **Decisões de implementação:**
    - o embed também grava `prompt_json`, porque o texto achatado do embed já era gravado;
    - imagem: HMAC da string decodificada (`images`) ou dos bytes brutos (`image_url`); `images` vazio não conta;
    - `key_id` é o sha256 dos bytes da chave;
    - `prompt_hmac`/`conteudo_hmac` usam o digest sha256 bruto (32 bytes) como mensagem; a 12.4 tem de usar `hmackey`;
    - `doc_key_id` só é gravado com um `doc_hash` válido.
  - **Achado preexistente:** no `/v1`, o texto achatado (`extractPrompt`) guarda o data-URI da imagem em base64, até 16 MiB. **Pendência**, com correção pequena em `journal.go`.
  - **Pendências para a 12.4:** `GetPromptJSON`/`JobDoc` nas amostras; copiar as marcas; 503 sem chave; usar `Key.ID`/`PromptHMAC`/`ContentHMAC`.
  - **Pendências para a 12.10:** o instalador gera a chave (32 bytes hex, 0600, fora do backup); a mesma chave vai para o fallback; `--doc-key-file` na unit; CHANGELOG e MANUAL-GATEWAY.
- 2026-11-02 22:07: **revisão independente da 12.3** (Opus, só leitura): **corrigir antes de publicar**.
  - **B1, bloqueador:** o `promptjson.go` falha **aberto**. O decoder roda sem `UseNumber()`, então um número como `1e400` num campo desconhecido passa pela política, derruba a varredura e faz o corpo ser gravado com a imagem em claro e `tem_imagem=0`. O teste `TestBuildPromptJSONInvalidBody` exigia justamente esse comportamento.
  - **I1:** as chaves são comparadas com `ToLower`, enquanto o Ollama casa nomes por *case folding* Unicode: `imageſ` e `meſſages` escapam. O `CanonicalContent` tem o mesmo problema.
  - **I2:** DoS por custo O(tokens × profundidade) antes da fila, pago mesmo com `--log-content=false`.
  - **Menores:**
    - M1: cópias extras de imagem grande;
    - M2: `Load` sem `LimitReader`;
    - M3: `Key` imprimível por `fmt`;
    - M4: `JobDoc` lia pelo `jdb`;
    - M5: data-URI em claro no texto achatado do `/v1`, preexistente desde a 10.L;
    - M6: rollback para 0.12.6 é seguro, mas o teto antigo não vê o `prompt_json`; documentar no MANUAL na 12.10.
  - **Pontos conferidos sem achado:**
    - o `X-Mcpollama-Doc` é removido em todas as rotas, inclusive no pool;
    - nenhum sha256 puro é gravado;
    - os tetos estão certos e as migrações são idempotentes;
    - nenhum lock novo e nenhuma consulta sob mutex.
  - **Correções:** B1, I1, I2, M1 a M5 e os testes que faltavam foram delegados ao Sonnet da 12.3.
- 2026-11-02 22:35: **correções da revisão da 12.3 concluídas** (Sonnet, 19 mutações FAIL-CONFIRMED e restauradas com `cmp`). Gate refeito pelo coordenador.
  - **B1:**
    - `UseNumber()`; a varredura que não termina **falha fechada**: `SavePromptJSONFailed` grava `prompt_json` NULL, `tem_imagem=1` e `prompt_json_truncated=1`;
    - o teste foi invertido (`TestBuildPromptJSONFailsClosed`), e o novo `TestHugeNumberDoesNotLeakTheImage` cobre o caso: o base64 não aparece no `content.db` nem no `-wal`;
    - JSON válido que não é objeto passa como veio.
  - **I1:** `strings.EqualFold`, também no `CanonicalContent`; testes com `imageſ` e `meſſages`.
  - **I2:**
    - o rastreio passou a ser incremental, com máscara de regras por frame;
    - 9.000 níveis e 300 mil tokens: < 2 s e < 400 MiB, testado;
    - sem varredura com `--log-content=false` (`WantsPromptJSON`);
    - pedido rejeitado não paga a varredura, porque ela foi para o `OnAccepted`.
  - **Menores:**
    - **M1:** HMAC sobre a fatia; 16 MiB com < 1 MiB alocado;
    - **M2:** `LimitReader(4097)`;
    - **M3:** `Key` com `String`/`GoString`/`Format` só com o `key_id`;
    - **M4:** `JobDoc` por `s.db`;
    - **M5:** o texto achatado do `/v1` também troca o `image_url` pelo marcador (`redactParts`); `extractPrompt` ganhou `key`; nenhum teste 10.L precisou de ajuste.
  - **Gate (coordenador, 22:33):**
    - `go build` 0; `go vet` 0; `gofmt -l` vazio;
    - `go test -count=1 ./...` 21 ok, 0 FAIL;
    - `-race -count=1 ./...` 21 ok, 0 FAIL, sem data race.
  - **Flake do `gateway/models`** (`TestLatestNeverQueriesAndLoopRespectsInterval`, "cache empty", `models_test.go:186`), medido pelo coordenador com `-race -count=10`: **1/10 no código atual e 1/10 no HEAD limpo** (worktree temporário, já removido). É **preexistente** e não tem relação com a 12.3. **Pendência:** corrigir à parte.
  - **Ataques (Sonnet, depois das correções):**
    - Fase 9: 470 PASS, 0 FAIL;
    - 10.L: 155 PASS, 0 FAIL (o D3 passou nesta rodada, mas a corrida preexistente continua como pendência);
    - 10.5: 244 PASS, 0 FAIL;
    - 0 vetores com FALHA.
  - **Pasta estranha:** `.workspace/ollama-native-smoke-9a43a4940cd642fab73f0dd56e329413/` (imagens `native_jpg_*.jpg`) apareceu, criada por outra sessão ou ferramenta, nem pelo coordenador nem pelo Sonnet. Não foi tocada e fica fora de commit, porque pode conter imagem de cliente. Perguntar ao owner.
  - **Revisão adversarial completa do gateway:** fica para a 12.9, conforme o plano.
- 2026-11-02 22:40: **12.3 publicada** (`3531beb`; segredos 0; os hex do diff são chaves fictícias de teste). `.gitignore` passou a ignorar `.workspace/ollama-native-smoke-*/` (owner). O aceite da 12.6 foi realinhado ao contrato 1.1.0 (cota com grupos exportáveis, partição pendente, quarentena só entre lados atribuídos, descarte de confirmada). **Início da 12.4.**
- 2026-11-02 22:42: 12.4 delegada ao Sonnet.
  - **Escopo:** pacote `gateway/treino` (`treino.db` 0600, `--treino-db`), `POST /api/feedback` antes do `/`, `--role`, validação, ingestão transacional com precedência de estados, máquina de estados e histórico só com hash e tamanho. A 12.4 reusa `hmackey`, `GetPromptJSON` e `JobDoc`.
  - **Fora do escopo:** rotas admin, exportação e partição (12.6); embeddings (12.6/12.8).
- 2026-11-02 23:33: **12.4 implementada** pelo Sonnet (50 min; 99 mutações, todas FAIL e restauradas com `cmp`).
  - **Pacote `gateway/treino`** (`treino.go`, `report.go`, `ingest.go`, `state.go`, `http.go` e testes):
    - tabelas `ocorrencias`, `ocorrencia_jobs`, `amostras`, `historico`, `retratos`, `retrato_itens` e `embeddings`, com CHECKs de estado, partição e ação;
    - teto medido pelo arquivo mais o `-wal`;
    - mutex de escrita próprio.
  - **Flags:** `--treino-db` (`MCPGW_TREINO_DB`, absoluto e distinto dos outros bancos) e `--role` (`MCPGW_ROLE`).
  - **Rota:** `/api/feedback` no mux antes do `/`.
  - **`proxy.go`:** wrappers `ParseStation` e `ClientLabel` (SSOT).
  - **Teste de ponta a ponta:** `fase12_feedback_main_test.go`.
  - **Gate (coordenador):**
    - `build` 0; `vet` 0; `gofmt` vazio;
    - `go test -count=1 ./...` **22 ok**, 0 FAIL;
    - `-race` 21 ok + o flake conhecido do `gateway/models` (preexistente, 1/10 no HEAD);
    - `treino` + `cmd` com `-race -count=3`: ok.
  - **Ataques (Sonnet):** 787 PASS, 0 FAIL, 10 SKIP com os ataques presentes; 79 vetores sem falha; cópias removidas.
  - **Flake novo observado pelo Sonnet:** `gateway/pool` `TestFailureOnASecondaryIsReportedNotMigrated` (502), 1 vez sob carga, depois 15/15 isolado; pacote não tocado. **Pendência:** acompanhar.
  - **Decisões de implementação** (13, listadas no relatório e em revisão pelo Opus), entre elas o formato do histórico `sha256=<hex> bytes=<n>` do texto da correção.
  - **Revisão independente em Opus:** em andamento.
- 2026-11-02 23:41: **revisão independente da 12.4** (Opus, só leitura): 0 bloqueadores, 6 importantes, 6 menores; **corrigir antes de publicar**.
  - **I-1:** a substituição do automático forçava `nova` por cima de `distribuida`, somava as listas e não registrava a estação no histórico.
  - **I-2:** o teto contava o `-wal` em dobro, e o banco não tinha `auto_vacuum`, então o 507 ficava permanente.
  - **I-3:** relato de job na fila ou em execução congelava a amostra incompleta.
  - **I-4:** o histórico guardava sha256 puro.
  - **I-5:** apagar liberava a vaga da cota.
  - **I-6:** um relato podia copiar job de outra estação.
  - **Menores:**
    - M-1: memória dentro da transação;
    - M-2: espera no mutex terminava em 500;
    - M-3: rollback para 0.12.6 com flags novas no `ExecStart` entra em laço de reinício (documentar na 12.10; avaliar env `MCPGW_*`);
    - M-4: limites extras não documentados;
    - M-5: edição nos estados de estatística;
    - M-6: o teste do 0600 só roda fora do Windows.
  - **Decisões do owner (23:40) → contrato 1.1.2:**
    - I-4: HMAC + tamanho (C-7.4);
    - I-5: tabela `cota_grupos` (C-4.11b);
    - I-6: só jobs da mesma origem (C-1.11a);
    - M-5: só `nova`/`confirmada` (C-1.12c).
  - **Regras novas sem decisão nova:**
    - C-1.4a: 409 para job na fila ou em execução;
    - C-1.12d: a substituição respeita a precedência e troca a lista;
    - C-6.2 e C-6.2a: dados vivos e `auto_vacuum`;
    - C-1.17: os limites documentados.
  - O `RELATAR-ERROS-IA.md` §5 foi atualizado. As correções foram delegadas ao Sonnet da 12.4.
- 2026-11-03 00:43: **correções da revisão da 12.4 concluídas** (Sonnet, 2 h; 141 mutações, todas FAIL e restauradas com `cmp`). Gate refeito pelo coordenador.
  - **I-1 (C-1.12d):** a substituição recalcula o estado pela precedência e troca a lista de jobs; nada é copiado em `distribuida`/`fallback`; as órfãs saem pela contagem de referências; `alterou` para estado, jobs, origem, estação, IP e label.
  - **I-2 (C-6.2/6.2a):** o teto passou a ser `(page_count − freelist_count) × page_size`, com `auto_vacuum(2)`, `journal_size_limit` 64 MiB e `incremental_vacuum` depois de Delete/Discard. O `Size()` passou a ser medido **antes** da transação, porque dentro dela abria outra conexão que esperava o lock de escrita.
  - **I-3 (C-1.4a):** job em `na_fila`/`em_execucao` dá 409 com `Retry-After: 5`, sem gravar nada.
  - **I-4 (C-7.4):** o histórico guarda `hmac=… key_id=… bytes=…` (`hmackey.MACString`); sem chave, só `bytes`.
  - **I-5 (C-4.11b):** tabela `cota_grupos`; nenhuma operação mexe nela.
  - **I-6 (C-1.11a):** `sameOrigin` compara a estação (máquina+usuário) ou o IP; os outros jobs viram referência (`jobs_outra_origem`); o dedupe e a substituição só enxergam a mesma origem, sem revelar o id alheio; `proxy.FairIP` foi exportado.
  - **Menores:**
    - **M-1:** jobs que já têm amostra não são lidos de novo; o tamanho vem de `content.Probe` antes de ler;
    - **M-2:** a vez de escrever usa um semáforo com `select` no ctx; prazo estourado dá 503 com `Retry-After`;
    - **M-4:** comentário alinhado;
    - **M-5:** teste explícito;
    - o `sleep` do teste virou poll com prazo.
  - **Gate (coordenador, 00:42):**
    - `build` 0; `vet` 0; `gofmt` vazio;
    - `go test -count=1 ./...` 22 ok, 0 FAIL;
    - `-race -count=1 ./...` 22 ok, 0 FAIL;
    - `treino`+`cmd` com `-race -count=3`: ok.
  - **Ataques (Sonnet):** 804 PASS, 0 FAIL, 10 SKIP (só Linux); 0 vetores com falha; cópias removidas (conferido).
  - **Ressalvas, como pendências:**
    1. A fiação do `Options.Key` no `main.go` só é exercida pelas operações do admin: testar na 12.6.
    2. `incremental_vacuum` depois de Discard sem teste próprio.
    3. Um `treino.db` criado antes do `auto_vacuum` não é migrado (não existe em produção).
    4. O 0600 não é provado no Windows.
  - Revisão adversarial completa da Fase 12 no gateway: **12.9**.
- 2026-11-03 00:47: **12.4 publicada** (`a1a4f9f` docs/contrato 1.1.2, `a820b70` código; segredos 0; push autorizado). **Início da 12.5** no Sonnet da 12.2.
  - **Escopo:** cliente `POST /api/feedback`, que reusa TLS, Bearer, estação e mapa de erros e escolhe o servidor pelo backend do `job_do_erro`; tool `reportar_erro`, que entende as 3 formas de `jobs`, valida antes de enviar e trata 409 com uma retentativa; automáticos com `--auto-report` (padrão true), as 3 regras da C-1.20, envio em segundo plano com fila limitada; `TOOLS.md`.
- 2026-11-03 10:01: **revisão automática de segurança do commit `a820b70`** (12.4): 2 achados médios, ambos **procedem**.
  - **(1) `sameOrigin`:** a estação é declarada e pode ser falsificada (o `/monitor` mostra máquina e usuário na LAN). Owner (09:59): **estação E IP**, com o job sem estação comparado só pelo IP → **contrato 1.1.3 (C-1.11a)**.
  - **(2) TOCTOU do teto:** o tamanho era lido antes da vez de escrever → C-6.2 1.1.3: medir depois da vez e antes da transação.
  - As correções foram delegadas ao Sonnet da 12.4. O `RELATAR-ERROS-IA.md` §5 e o `docs/README.md` foram alinhados.
- 2026-11-03 10:01: **12.5 implementada** pelo Sonnet (30 mutações, todas FAIL e restauradas com `cmp`).
  - **Arquivos novos:**
    - `ollama/feedback.go`: `Client.Feedback`, que escolhe o cliente pelo backend, nunca retenta e nunca cai para o outro servidor;
    - `tools/reportar_erro.go`: parse das 3 formas de `jobs`, validação antes de enviar, C-1.3, 409 com uma retentativa e teto de 10 s;
    - `tools/autoreport.go`: as 3 regras e o `AutoReporter`, com fila de 16, 1 goroutine, 30 s por envio, até 3 retentativas no 409 e `Close`.
  - **Arquivos alterados:**
    - `api.go`: `setHeaders` extraído do `sendOnce` (SSOT);
    - `--auto-report` (`MCPOLLAMA_AUTO_REPORT`, padrão true) e `buildToolsWithClose`;
    - `TOOLS.md` regenerado;
    - orçamentos de `tools/list` em `registry_test`: 4.620→6.650 e 8.330→10.360, porque o schema com 3 enums soma 2.000 B.
  - **Gate (coordenador, 09:59):**
    - `build` 0; `vet` 0; `gofmt` vazio;
    - `go test -count=1 ./...` 22 ok;
    - `-race` 22 ok, 0 FAIL.
  - **Decisões:**
    - (1) **A regra (a) da C-1.20, "JSON inválido", não tem chamador**: as tools não expõem o `format` do Ollama, e o `format` de `extract`/`summarize_file` é texto livre. **Pendência para o owner.**
    - (2) Ordem das regras: vazio > JSON > truncado.
    - (3) Nas tools de arquivo, só a última chamada é checada.
    - (4) Automáticos com `projeto=mcpollama-auto` e `severidade=baixa`.
    - (5) Nulo no texto e corpo acima de 256 KB são recusados na estação.
  - **Sem divergência** com o `RELATAR-ERROS-IA.md`.
  - Revisão independente em Opus em andamento.
- 2026-11-03 10:22: **revisão independente da 12.5** (Opus, só leitura): **pode publicar**, 0 bloqueadores.
  - **Sem vazamento:** o token não troca de servidor, nenhum redirect é seguido e a redação cobre os dois tokens.
  - **Automáticos:** não atrasam nem falham a tool, e não geram tempestade (no máximo 1 candidato por chamada e 4 POSTs).
  - **I-1:** a regra (a) não tem chamador, e a ajuda da flag prometia o contrário.
  - **Menores:**
    - M-1: falta teste do cancelamento na espera do 409;
    - M-2: falta teste de papel repetido;
    - M-3: id solto vira `unico`;
    - M-4: duas linhas `[uso]`;
    - M-5: o id e o estado vindos do servidor não são validados;
    - M-6: 404 com gateway antigo, o que é ordem de implantação: o gateway vai antes;
    - M-7: no modo compact, a tool perde as instruções;
    - M-8: o `RELATAR` dizia "caracteres" onde é bytes; o `buildTools` não fecha o reporter nos testes.
  - **Custo:** o `tools/list` cresce +43%, cerca de 500 tokens por sessão.
  - **Decisões do owner (10:21) → contrato 1.1.4 (C-1.20a):**
    - (a) reservada e documentada;
    - descrição da tool **enxugada**, com alvo de metade do acréscimo;
    - automáticos **desligados por padrão**, revendo a D12-8 e a §14 item 1.
  - **Correção do gateway (revisão de segurança) concluída** pelo Sonnet da 12.4:
    - `sameOrigin` exige estação **e** IP (`ClientIP`+`FairIP`, `X-Real-Ip` só de loopback); job sem estação decide pelo IP; 10 mutações FAIL, 1 equivalente;
    - `withTxSized` mede depois da vez de escrever; o pool foi aquecido (`SetMaxIdleConns(4)`) para evitar `SQLITE_BUSY` numa conexão nova;
    - `TestConcurrentReportsNearTheCeilingDoNotOvershoot`: 8 simultâneos → 1 aceito e 7 com 507, passou 15× com `-race`;
    - gate do Sonnet: 22 ok, com e sem `-race`;
    - ataques (só gateway/cmd): 576 PASS, 0 FAIL.
- 2026-11-03 10:40: **ajustes da 12.5 concluídos** (Sonnet; 10 mutações, todas FAIL).
  - **A:** `--auto-report` com padrão **false** e help sem prometer a regra (a).
  - **B:** tool enxuta. Medição do `tools/list`, em bytes, com o acréscimo da tool:
    - full base: 6.620 → **5.604** (+2.032 → +1.016, metade do acréscimo);
    - full com tools de arquivo: 10.327 → 9.311;
    - compact: +1.054 → +885 (só 16%; o que sobra são enums, nomes de campo e annotations obrigatórias);
    - orçamentos em `registry_test`: 5.640 / 9.345.
  - **Menores:**
    - M-1: cancelamento na espera do 409;
    - M-2: papel repetido;
    - M-3: id solto só sozinho, lista de ids soltos recusada;
    - M-4: mais de um campo de job recusado;
    - M-5: `FeedbackResult.valid()`, sem ecoar valor inválido;
    - M-8: `buildToolsFor` com `t.Cleanup`;
    - `jobs_outra_origem` explicado.
  - **O que não deu certo:** o teste novo `TestBuildToolsCloseStopsTheSender` contava `runtime.NumGoroutine()` e **falhou no `-race` do pacote completo**. Isolado, passava 10/10 com e sem `-race`: as goroutines de outros testes do pacote alteravam a contagem.
  - **Solução (coordenador):**
    - `AutoReporter.Done()`, o canal do `run()`;
    - a variável `newAutoReporter` no `backend.go`, para o teste capturar a instância;
    - o teste passou a esperar o `Done()`;
    - pacote com `-race -count=5`: 5/5 ok;
    - mutação do closer vazio: FAIL, depois restaurado com `cmp` e ok.
  - **Gate final (coordenador, 10:39):**
    - `build` 0; `vet` 0; `gofmt` vazio;
    - `go test -count=1 ./...` 22 ok;
    - `-race -count=1 ./...` 22 ok, 0 FAIL;
    - inclui a correção de segurança do gateway.
  - **Pendências:**
    1. M-6: implantar o gateway 0.13.0 **antes** do mcpollama 0.10.0 (12.10).
    2. M-7: no modo compact, a tool fica sem instruções; aceito, porque o `RELATAR-ERROS-IA.md` é a fonte.
- 2026-11-03 10:44: **publicados** `37706bb` (fix gateway: origem estação+IP, teto na vez), `9f67fee` (12.5) e `d27a4ca` (contrato 1.1.3/1.1.4 e plano); segredos 0. **Início da 12.6** (Sonnet da 12.4).
  - **Escopo:** rotas admin; "Marcar erro" sem a restrição de origem; partição, grupos, cota, quarentena e retratos (cap. 4 1.1.x); exportação JSONL em streaming com o predicado único e `exportou` por ocorrência; marca de exemplo público; aba "Erros (treino)".
  - **Decisão de divisão:** o **cálculo do embedding** (C-6.6) fica para a **12.8**. A 12.6 só registra a marca, que fica pendente de embedding.
- 2026-11-03 12:10: **incidente.** Um comando antigo, em segundo plano, do Sonnet da 12.2 (um script de mutação que tinha travado esperando stdin e nunca foi encerrado) terminou por volta das 12:00 e **restaurou backups velhos** por cima de `ollama/api.go`, `tools/generate.go`, `tools/chat.go` e `tools/filetools.go`. Isso desfez trabalho da 12.5 só na árvore de trabalho: o commit `9f67fee` não foi afetado. O Sonnet refez os arquivos a partir dos backups da 12.5 e à mão.
  - **Verificação do coordenador:**
    - `git diff --stat HEAD -- src/ollama src/tools src/mcpollama-mcp TOOLS.md` → **vazio**: os 4 arquivos estão idênticos ao commit;
    - sem dano no que foi publicado;
    - nenhum processo velho rodando (só o `mut4.py` da 12.6, iniciado às 12:06).
  - **O coordenador repetiu o erro** ao registrar o incidente: um `cat >> arquivo` sem entrada travou esperando stdin. O comando foi **encerrado** (TaskStop) e o arquivo vazio que ele criou foi apagado.
  - **Lição (para todos os agentes):** nunca usar `cat`/`python -` sem entrada definida. Comando em segundo plano que trava tem de ser encerrado e conferido antes de seguir, e nenhum `restore` pode ficar pendente.
- 2026-11-03 12:48: **12.6 implementada** pelo Sonnet (105 mutações: 94 FAIL de primeira, 8 depois de ajustes, 3 equivalentes, que são camadas redundantes do predicado).
  - **Arquivos novos:**
    - `treino/partition.go`: grupos, cota, quarentena, `Eligible`, `ReleaseQuarantine`, `SetExemploPublico`;
    - `treino/export.go`: JSONL em streaming;
    - `treino/query.go`: `Summary`/`List`/`Detail`;
    - `monitor/treino.go`: rotas.
  - **Arquivos alterados:**
    - `ingest.go`/`state.go`: `IngestWith{Admin}` e `recompute` na mesma transação;
    - `monitor.go`: `Config` e o resumo no `/state` via `swr`;
    - `index.html`: aba "Erros (treino)", detalhe, formulários e "Marcar erro"; continua em LF (0 CR, conferido), com a CSP recalculada pelo código;
    - `main.go`: fiação.
  - **Gate (coordenador, 12:47):**
    - `build` 0; `vet` 0; `gofmt` vazio;
    - `go test -count=1 ./...` 22 ok;
    - `-race -count=1 ./...` 22 ok, 0 FAIL;
    - `treino`/`monitor`/`cmd` com `-race -count=3`: ok;
    - `proxy`, `ollama`, `tools` e `mcpollama-mcp` idênticos ao HEAD (conferido);
    - nenhum processo python/go velho.
  - **Ataques (Sonnet, só `cmd`/`gateway`):** 612 PASS, 0 FAIL, 10 SKIP; 0 vetores com falha.
    - O vetor **A3 da 10.L** compara a lista exata de rotas POST do `monitor.go` e teve de ser atualizado na cópia temporária, com as 7 rotas novas.
    - **Pendência:** atualizar o arquivo guardado em `.workspace/reports/...10L.testes/gateway/monitor/fase10l_adv_test.go`.
  - **UI:**
    - Firefox headless em 570/900/1400 px só para o visitante, porque o `--screenshot` não faz login;
    - **as telas com login foram feitas no Chromium (Playwright)**, porque não houve login no Firefox. O aceite pedia Firefox; isso fica registrado como **desvio**;
    - sem rolagem horizontal;
    - console: só o `favicon.ico` 404, que já existia.
  - **Incidentes do Sonnet:**
    - um comando antigo da 12.4 (heredoc de Python preso) terminou e acrescentou um bloco duplicado ao `proxy.go`. Foi removido, e o arquivo está igual ao HEAD;
    - um script de mutação morto no meio deixou o `partition.go` mutado. Restaurado do snapshot, com a rodada refeita limpa.
  - **Decisões de implementação:**
    1. `grupo_id` = menor id entre os membros; numa divisão, cada parte fica com um id próprio.
    2. O sha256 entra no cálculo como número de 256 bits.
    3. A quarentena é persistente para o componente; liberar não consome vaga.
    4. "Pendente de embedding" = linha em `embeddings` com `vetor`/`modelo` NULL.
    5. O índice de exemplos congelado ainda não é uma tabela: é invalidado pelos retratos (`indice_hash`). A 12.8 decide.
    6. Mudanças de partição feitas pelo sistema gravam `alterou`/`particao` com `quem=sistema`.
    7. O estado é checado só no `Eligible`.
    8. A ação do admin vai só ao `historico`, não à auditoria do monitor.
    9. `exportou` em lotes de 20, depois de escrever na resposta.
  - **Revisão independente em Opus:** em andamento.
- 2026-11-03 12:56: **revisão independente da 12.6** (Opus, só leitura): **corrigir antes de publicar**.
  - **Sem falha encontrada em:** vazamento ao visitante, XSS, CSRF/same-site, predicado de exportação (completo), formato e `meta` do JSONL, C-4.12 (por construção; a divisão de grupo não muda o lado).
  - **I-1:** `exportou` pode faltar para até 19 linhas já entregues (retornos de erro sem `record()`) → gravar o lote antes de escrever.
  - **I-2:** o `recompute` refaz tudo sob a vez de escrever (O(N²) no total) → índices de cobertura e teste de escala com meta (< 200 ms com 10 mil ocorrências), ou recompute incremental.
  - **I-3:** as leituras usavam `WithTimeout` → `context.Background()`, como pede a lição da 0.12.6.
  - **Menores:**
    - M-1: avisar ao liberar uma quarentena com membro já exportado na outra parte;
    - M-2: detalhe sem corte (pior caso: 2 GB);
    - M-3/M-4: documentar.
  - **Lacunas de teste:** C-4.12 intercalado, divisão de grupo, `exportou` em erro, `innerHTML` estático, `/v1`.
  - Correções delegadas ao Sonnet da 12.6, incluindo a atualização do vetor A3 guardado em `.workspace/reports`.
- 2026-11-03 13:30: **correções da revisão da 12.6 concluídas** (Sonnet; 40 mutações, todas provadas ou explicadas). Gate refeito pelo coordenador.
  - **I-1:** `exportou` passou a ser **write-ahead**. Cada lote (20 linhas ou 8 MiB) é gravado antes de ser enviado, com contexto e transação próprios; se gravar falha, o lote não é enviado. Testes: writer falhando, erro de banco no meio (trigger), contexto expirado no meio.
  - **I-2:** índices de cobertura em `amostras`/`ocorrencias` com `INDEXED BY`; o índice do predicado usa `COALESCE(length(prompt_json),0)>0`, para não ler os blobs.
    - Escala com 10 mil ocorrências, ~30 mil jobs e 242 MiB: **recompute 342 → 94 ms** e **Summary 98 → 59 ms**, medidos também pelo coordenador às 13:28. O incremental não foi preciso.
    - O teste é pulado sob `-race` (`race_on/off_test.go`).
    - Só a mutação do índice do join prova o tempo; as dos índices do resumo e do predicado sobreviveram, porque o planner escolhe o índice sozinho.
  - **I-3:** leituras do monitor com `context.Background()`; o prazo do export virou `ExportUntil`, verificado entre linhas; LIMIT na lista. `TestReadPathsUseNoContextWithDone` (go/ast).
  - **M-1:** `ReleaseQuarantineInfo` avisa quando há membros já exportados para a outra parte.
  - **M-2:** o detalhe corta cada campo em 256 KiB no SQL (`substr`), em fronteira de caractere, com a marca `[cortado (N bytes)]`.
  - **M-3/M-4:** documentados no código.
  - **Testes novos:**
    - C-4.12 com 900 operações intercaladas (60 quarentenas e 105 heranças exercidas);
    - divisão de grupo;
    - teste estático do `index.html` (sem `innerHTML`/`on*=`/`style=`…);
    - exportação das rotas `/v1`.
  - **Bug real encontrado:** ao apagar a ponte de um grupo, as duas partes podiam receber o **mesmo `grupo_id`**. Corrigido com um id derivado de forma determinística quando todos os ids possíveis já estão tomados. O teste falha sem a correção.
  - **Vetor A3 guardado** (`.workspace/reports/...10L.testes/gateway/monitor/fase10l_adv_test.go`): a lista agora tem as 7 rotas POST novas e continua exigindo a lista exata; passa copiado.
  - **Gate (coordenador, 13:28):**
    - `build` 0; `vet` 0; `gofmt` vazio;
    - `go test -count=1 ./...` 22 ok;
    - `-race -count=1 ./...` 22 ok, 0 FAIL;
    - `treino`/`monitor` com `-race -count=2`: ok;
    - `index.html` 0 CR;
    - `proxy`/`ollama`/`tools`/`mcpollama-mcp` iguais ao HEAD;
    - nenhum processo python/go velho.
  - **Ataques (Sonnet):** 2ª rodada com 627 PASS, 0 FAIL, 10 SKIP.
    - Na 1ª rodada, 1 FAIL esporádico em `TestAdvCSRF` (Fase 9): o vetor troca o último caractere do token por `A` e, em 1 de cada 64 casos, o token "errado" é igual ao certo e a resposta sai 409.
    - **Pendência:** endurecer o vetor guardado (trocar o caractere por outro sempre diferente).
- 2026-11-03 13:35: **12.6 publicada** (`0493f7f`; segredos 0). Pergunta ao owner sobre a ordem: a 12.7 e a 12.8 só medem com dados reais (≥ 30 confirmados por categoria). **Decisão do owner: seguir a ordem do plano**: código da 12.7 e da 12.8 agora, com dados sintéticos; linha de base e medição quando houver casos, depois da 12.10. **Início da 12.7** num Sonnet novo (os anteriores acumularam muito contexto).
- 2026-11-03 18:45: **12.7 implementada** pelo Sonnet (74 mutações: 72 mortas, 1 equivalente, 1 só provável no Linux, o modo 0600).
  - **Arquivos:**
    - `treino/retrato.go`: `CreateRetrato`, `GetRetrato`, `InvalidateRetrato`, `RetratoCases`, com reconferência e invalidação na leitura (só aditivo; reusa `Eligible`);
    - `cmd/bakeoff/erros*.go`: modos `retrato`, `base` e `comparar`; replay com seeds 1/2/3 intercalados; pontuação das C-5.2 a C-5.7; Wilson; limiares; gate; arquivo blind 0600; `linha-de-base.json`;
    - `main.go` com 12 flags.
  - **Onde roda:** no servidor, ao lado do `treino.db`, contra `127.0.0.1:11500`. O cliente recusa outro host, ignora proxy e não segue redirect, e fora de loopback só roda com `-gateway-remoto`. O relatório e os logs só têm ids e números.
  - **Exit codes:** 0 aceito; 1 gate falhou ("dados insuficientes"/"medição incompleta"); 2 uso; 3 reprovado.
  - **O que não deu certo:**
    - o **teste de escala da 12.6** (`TestScaleRecomputeAndSummary`) falhou na suíte completa do coordenador, com o `Summary` acima de 100 ms. Isolado, deu 112–138 ms no recompute e 70–90 ms no `Summary`; com TEMP no E:, 117/71 ms. Em paralelo com os outros pacotes e com a revisão rodando, a máquina fica carregada.
    - **Solução (coordenador):** o teste de tempo deixou de ser gate e passou a ser opcional por `MCPOLLAMA_TEST_SCALE=1` (mesmo padrão dos testes `live`/`ocr`). Rodado à parte às 18:44: **112 ms / 73 ms, PASS**.
  - **Gate (coordenador, 18:44):**
    - `build` 0; `vet` 0; `gofmt` vazio;
    - `go test -count=1 ./...` 22 ok;
    - `-race -count=1 ./...` 22 ok, 0 FAIL;
    - `gateway/proxy|monitor|store|queue`, `ollama`, `tools` e `mcpollama-mcp` iguais ao HEAD;
    - nenhum processo velho.
  - **Ataques (Sonnet, numa cópia):** 803 PASS, 10 SKIP.
    - `TestAdvRequestsGrowthWithoutCap` deu "disk is full" no D: e passou com TEMP no E:.
    - **Os `fase9_adv_test.go` e `fase9_adv_m8_test.go` do monitor guardados não compilam no HEAD** (`extraAt` e `vec` sumiram na 0.12.6). Nas etapas anteriores eram adaptados na cópia; desta vez foram excluídos, sem efeito, porque a 12.7 não toca no monitor. **Pendência: corrigir os dois arquivos guardados de vez.**
  - **Disco D: com 2,6 GB livres (99%).** Cache do Go (`go-build`) com 4,88 GB e scratchpad com 1,1 GB. Perguntar ao owner sobre a limpeza.
  - **Decisões de implementação:**
    1. O candidato troca só `model` e `seed`; uma amostra `qwen3-ia-think` precisa de um nome de candidato próprio.
    2. Retrato vazio é recusado.
    3. Caso não medido falha o gate ("medição incompleta").
    4. Só as categorias automáticas contam para o ≥ 30; as do blind ficam pendentes da avaliação humana.
    5. A linha de base só é gravada com o gate passando, e nunca é sobrescrita.
    6. N < 20 não mostra taxas.
  - **Pendências para a 12.8:** `-indice-hash`/`-instrucoes-*` no retrato; k e o limiar de similaridade sem coluna no retrato.
  - **Pendências para a 12.10:** compilar o `bakeoff` para Linux e rodá-lo com o usuário do gateway; segundo processo abrindo o `treino.db` vivo ainda não exercitado ao vivo.
  - Revisão independente em Opus em andamento.
- 2026-11-03 22:10: **owner: não mexer no disco D:** (2,6 GB livres; nem o cache do Go nem o scratchpad). Agentes passam a usar TEMP no E: quando precisarem de espaço.
- 2026-11-03 22:10: **revisão independente da 12.7** (Opus, só leitura): **corrigir antes de publicar**.
  - **Bloqueadores:**
    - (1) o retrato não era conferido de novo no fim da rodada, que leva horas;
    - (2) stream cortado era pontuado como medição;
    - (3) o veredito ignorava as categorias às cegas não avaliadas, dando "aceita" antes de o owner ler o blind.
  - **Importantes:**
    - (4) o `-gateway-remoto` aceitava qualquer host, inclusive passando pelo Cloudflare;
    - (5) comparar sem linha de base;
    - (6) número final extraído da resposta certa em prosa;
    - (7) a 12.8 não seria medível, porque o replay não aplica instruções nem exemplos.
  - **Decisão do owner (22:09) sobre (7):** a 12.8 vai ser medida **transformando o `prompt_json` no bakeoff** com as instruções e os exemplos congelados, num código compartilhado com o mcpollama. Até lá, o comparar é recusado quando o retrato tem índice ou instruções.
  - **Menores:**
    - JSON com chave duplicada;
    - `_txlock=immediate`;
    - modelo gravado deve ser igual ao `-atual`;
    - `score.json` com modelo e data;
    - `-out` obrigatório.
  - Todas as correções foram delegadas ao Sonnet da 12.7.
- 2026-11-03 23:10: **correções da revisão da 12.7 concluídas** (Sonnet; todas as mutações mortas, exceto o P17, que é equivalente). Gate refeito pelo coordenador.
  - **Bloqueadores:**
    - (1) `recheckRetrato` refaz a conferência antes do `finish`; um retrato mudado durante a rodada faz o gate falhar, sem linha de base;
    - (2) resposta sem `done`/`[DONE]` não é medida, em qualquer categoria ("medição incompleta");
    - (3) com blind pendente, o resultado é **"PENDENTE da revisão às cegas", exit 4**; se a parte automática já reprovar, vale REPROVADA. Modo `apurar` com o arquivo de respostas do owner: `rótulo<TAB>certa|errada<TAB>certa|errada`; todos os rótulos são obrigatórios; o modo não chama o gateway e reconfere o retrato.
  - **Importantes:**
    - (4) `-gateway-remoto` só aceita IP literal privado, em `https`, reconferido no `Dialer.Control`;
    - (5) comparar exige a `linha-de-base.json` do mesmo retrato e do mesmo modelo;
    - (6) o `want` precisa de `RESULTADO: n` ou de um número único; casos ambíguos são inaplicáveis e não entram no replay;
    - (7) comparar é recusado com índice ou instruções; `requestTransform` (identidade por padrão, só para o candidato) é o ponto de extensão da 12.8.
  - **Menores:**
    - chave duplicada → `chave_duplicada`;
    - `treino.Options.TxLockImmediate` (padrão false; o **DSN do gateway não mudou**, conferido no diff), usado só pelo bakeoff;
    - o modelo gravado tem de ser o `-atual`;
    - o `score.json` passa a ter `modelo`/`gerado_em`, com janela de 48 h;
    - `-out` obrigatório.
  - **Gate (coordenador, 23:07):**
    - `build` 0; `vet` 0; `gofmt` vazio;
    - `go test -count=1 ./...` 22 ok;
    - `-race -count=1 ./...` 22 ok, 0 FAIL;
    - fora do escopo, tudo igual ao HEAD;
    - nenhum processo velho;
    - `.tmp-gate` removido.
  - **Ataques (Sonnet, porque tocou o `treino.go`):** 888 PASS, 0 FAIL, 11 SKIP. Os `fase9_adv*` do monitor guardados continuam sem compilar no HEAD: **pendência**.
  - **Decisão a conferir:** num retrato só com categorias às cegas, a linha de base é registrada vazia.
- 2026-11-03 23:10: o **owner perguntou** se o `D:\MCP` continua operacional. Conferido: `mcpollama-mcp.exe` **0.9.2-f367b35** (31/10 22:21, intocado); `list_models` pelo MCP respondeu normalmente. Nada da Fase 12 foi implantado.
- 2026-11-03 23:15: **12.7 publicada** (`3fc8fa9`; segredos 0).
  - **Decisões do owner (23:13) para a 12.8:** (a) a melhora é aplicada **no gateway**: pacote compartilhado com o `requestTransform` do bakeoff (SSOT), dados no servidor, sem atualizar as estações; (b) **mecanismo completo com conteúdo vazio**: instruções v0 vazias e k/limiar configuráveis, a definir com dados medidos pela 12.7.
  - **Início da 12.8** num Sonnet novo.
- 2026-11-04 00:45: **operação no servidor (owner, 00:42: "configure para 2 simultâneo"; opção "Aplicar direto e deixar").**
  - **Mudanças:** `OLLAMA_NUM_PARALLEL` 1→2 em `/etc/systemd/system/ollama.service.d/10-performance.conf` e `--gen-slots` 1→2 em `/etc/systemd/system/mcpollama-gateway.service`. Backups com o sufixo `.bak-20261104-004517`.
  - **Antes:** verificação de jobs pelo `/monitor/api/state` (`queue.running`/`waiting`/`in_use` = 0). A conexão aberta com o Ollama era só um keep-alive parado.
  - **O que não deu certo:**
    - (1) a 1ª checagem por `ss` abortou por causa do keep-alive;
    - (2) erro no script: tratei `running` como int;
    - (3) o `sed` do gateway falhou no escape de `\\`.
    - Nenhum serviço foi reiniciado com arquivo pela metade (`set -e`). O estado foi conferido e a mudança terminada com um `sed` sem barra invertida.
  - **Depois:**
    - `daemon-reload`, restart do Ollama (0.40.13) e do gateway;
    - `active`/`active`, `healthz` 200, `slots: geracao 2, embed 2`.
  - **Conferido no log:** `msg="model architecture does not currently support parallel requests" architecture=qwen35` e `n_seq_max = 1`. **O `qwen3-ia` continua um por vez**: o gateway manda 2 pedidos e o 2º espera dentro do Ollama.
    - VRAM igual, 12,2 GiB a 64k; GPUs ociosas a 30–35 W de 100 W.
    - Teste `generate`: ok, 335 ms.
  - **Repositório:** `deploy/servidor-ia/ollama-10-performance.conf`, `deploy/servidor-ia/README.md`, `deploy/gateway/mcpollama-gateway.service`, `CLAUDE.md` e `README.md` alinhados, e a memória `criterio-modelo-multiagente` atualizada. **Pendência:** o `CONEXAO-IA.md` (fora do repositório) e o `MANUAL-GATEWAY.md` na 12.10.
- 2026-11-04 00:50: **12.8 implementada** pelo Sonnet (85 mutações: 79 mortas, 6 explicadas).
  - **Arquivos:**
    - `gateway/melhora/` (+ `feed/`), separado para não criar ciclo de import `treino→proxy→melhora→treino`;
    - `treino/indice.go`: índice congelado e invalidação;
    - `proxy/embedder.go`: busca antes do `Acquire`, pela raia de embed;
    - bakeoff `erros_melhora.go` e o modo `indice`;
    - flags `--melhora-*`.
  - **Gate (coordenador, 00:50):**
    - `build` 0; `vet` 0; `gofmt` vazio;
    - `go test -count=1 ./...` **24 ok**;
    - `-race` 24 ok, 0 FAIL;
    - fora do escopo, tudo igual ao HEAD.
  - **Ataques (Sonnet):** 1246 PASS, 0 FAIL. Os `fase9_adv*` do monitor foram adaptados só na cópia.
  - **Revisão independente da 12.8** (Opus): **pode publicar com a melhora desligada** (o caminho quente fica idêntico); corrigir antes de ligar.
    - **I-1:** os exemplos desmontam a defesa por nonce (todos viram `<id>`), e o corte deixa um `ARQUIVO_INICIO` aberto no `system`.
    - **I-2:** o `prompt_json` gravado era o pedido antes da melhora, citando uma "nota 1.1.5" inexistente.
    - **I-3:** desmarcar o exemplo deixava o vetor em `indice_itens`.
    - **Menores:**
      - M-1: cota única de embed;
      - M-2: pendente sem amostra trava a fila;
      - M-3: o hash não cobre o formato;
      - M-4: índices nunca apagados;
      - M-5: `--melhora-embeddings` vinha ligado.
  - **Decisões do owner (00:56) → contrato 1.1.5:**
    - I-2: **gravar os dois pedidos e o rastro** (C-2.10a), com os jobs de melhora fora da exportação e do retrato (C-3.10a);
    - M-5: embeddings **desligados por padrão** (C-6.6).
  - **Regras novas, sem decisão do owner:**
    - C-8.10a: k e o limiar ficam congelados no índice (decisão do Sonnet, aceita);
    - C-8.11: delimitação dos exemplos.
  - Correções delegadas ao Sonnet da 12.8.
- 2026-11-04 00:52: owner perguntou se há algo a configurar no `.mcp.json` (sem abrir o arquivo). **Nada agora.** Na 12.10, com o mcpollama 0.10.0, a `reportar_erro` aparece sem configuração; só `--auto-report` seria acrescentado, e só se o owner quiser os automáticos.
- 2026-11-04 02:20: **operação no servidor: 5 simultâneos** (owner, 02:03: "pode aumentar para 5"; depois, "5 + run-timeout para 10min").
  - **Aviso prévio do coordenador:** com 5 slots, o 5º pedido espera dentro do Ollama (o `qwen3-ia` gera 1 por vez) e esse tempo conta no run-timeout. O `--run-timeout` padrão já era 10 min (herdado do `--max-wait`); o owner manteve 10 min, agora explícito na unit.
  - **Espera pela fila vazia:** 9 min sem brecha. A fila **não esvaziava** porque uma tool `cursor-file-explanation-single` mandava jobs sem parar (75 desde 00:45).
    - **Erro de contagem do coordenador:** o "5 ocupados" somava `running` duas vezes (lista + `in_use`); eram 3 jobs.
    - Owner (02:14): **aplicar agora**, aceitando a queda dos jobs ativos.
  - **Incidente: gateway fora do ar por ~2 min (02:15–02:17), erro do coordenador.**
    - O `sed` que inseria `--run-timeout` gravou um `\n` literal no `ExecStart`, porque nesta ferramenta o `\\` chega como `\` no `ssh 'bash -s'`. O gateway ficou em `activating`, reiniciando em loop. O Ollama já estava em 5.
    - A tentativa de corrigir com `awk` não casou a linha (mesmo problema de escape).
    - **Recuperação:** a unit original foi restaurada do backup, seguida de `daemon-reload` e restart (`active`, `healthz` 200, com 2 slots). A tentativa com Python também falhou na compilação pelo mesmo escape, sem efeito no serviço.
    - **Solução final:** edição com Python **sem nenhuma barra invertida** (`chr(92)`/`chr(10)`) e rollback automático se o `healthz` não voltasse.
  - **Estado final (02:16):**
    - `OLLAMA_NUM_PARALLEL=5`;
    - gateway com `--gen-slots 5`, `--run-timeout 10m` e `--embed-slots 2`;
    - `active`/`active`, `healthz` 200, monitor com `geracao: 5`.
    - Backups: `10-performance.conf.bak-20261104-021514` e `mcpollama-gateway.service.bak-20261104-021514` e `-021624`.
  - **Repositório:** `deploy/servidor-ia/ollama-10-performance.conf`, `deploy/servidor-ia/README.md`, `deploy/gateway/mcpollama-gateway.service` (com comentário), `CLAUDE.md`, `README.md` e a memória `criterio-modelo-multiagente`, que guarda a lição sobre a barra invertida.
  - **Lição para todos:** editar arquivo remoto sem barra invertida e sempre com rollback automático pelo `healthz`.
- 2026-11-04 02:20: **correções da revisão da 12.8 concluídas** pelo Sonnet (33 mutações, todas mortas).
  - **C-2.10a:** `prompt_json_original` e as colunas `melhora_*` no `content.db`/`treino.db`, copiadas para a amostra.
  - **C-3.10a:** critério `melhora` no `Eligible`.
  - **`--melhora-embeddings`:** padrão false.
  - **C-8.11:** bloco `EXEMPLOS_INICIO/FIM <nonce>` sem as linhas `=====…=====` e sem `[sistema]`. O teste com o documento de ataque passa.
  - **Desmarcar o exemplo** apaga as cópias em `indice_itens`.
  - **Fila de pendentes:** o embedder pagina por rowid e marca `falha:` as linhas sem pedido (205 ruins não bloqueiam).
  - **Índice:** `FormatVersion "ex1"` no hash; `DeleteInvalidIndexes` e o modo `indices-limpar`; o índice invalidado sai da memória.
  - **Schema:** o `treino.db` passa para a versão 3; um gateway antigo recusa o arquivo, como deve.
  - **Gate do Sonnet:** 24 ok, com e sem `-race`.
  - **Ataques do Sonnet:** 1277 PASS, 0 FAIL.
- 2026-11-04 02:25: **publicados** `9e3c111` (12.8 + contrato 1.1.5) e `fe8be32` (deploy e docs com 5 simultâneos); segredos 0. **Início da 12.9**: revisão adversarial completa da Fase 12 em **Opus**, só com testes de ataque, sem tocar no código de produção, guardados em `.workspace/reports/...-revisao-adversarial-fase12.testes/`. Inclui corrigir de vez os `fase9_adv*` guardados.
- 2026-11-04 02:51: **operação no servidor:** owner, "coloque 3 jobs em paralelo". Aplicado direto: `OLLAMA_NUM_PARALLEL=3` e `--gen-slots 3`; serviços `active`. Pendência: alinhar as cópias em `deploy/`, o `CLAUDE.md` e o `README.md` (hoje dizem 5).
- 2026-11-04 02:43: os 9 jobs cancelados da `Treinamento-Codex` (10.100.2.30) foram fechados pelo **cliente**: nginx 499 nos 9 no mesmo segundo, e o Ollama registrou "aborting completion request due to client closing the connection". Não foi falha do servidor.
- 2026-11-04 02:55–03:20: **conversa com o owner, sem nenhuma ação**:
  - por que não há paralelo (arquitetura `qwen35`);
  - quais modelos aceitam paralelo;
  - os dois Ollamas no pool;
  - os três modelos residentes (`qwen3.5` + `gemma4` a 32k + `nomic` ≈ 28,8 GB);
  - acerto por tipo de tarefa (Fase 8), qwen3.5 contra gemma4;
  - rota por velocidade, qualidade e tipo (proposta de Fase 13);
  - na produção atual, a escolha manual por `model` funciona só com configuração; a rota automática exige código.
- 2026-11-04 03:22: **12.9 concluída** (revisão adversarial em Opus, 5 revisores em worktrees isolados).
  - **Resultado:** 0 bloqueadores, 9 importantes e 19 menores; 364 vetores (325 OK e 39 FALHA, iguais em 2 rodadas); sem DATA RACE.
  - **Artefatos:**
    - report `.workspace/reports/MCPOllam2026-11-04-022324-revisao-adversarial-fase12.report.md`;
    - testes em `...-fase12.testes/`;
    - **`fase9_adv*` do monitor guardados corrigidos de vez** (`extraAt` → `extras` sob o mutex; token errado sempre diferente, o que acaba com o 409 esporádico de 1/64).
  - **`src/`** igual ao HEAD `fe8be32`, conferido pelo coordenador.
  - **Decisões do owner (03:23) → contrato 1.1.6:**
    - I-1 → C-4.5a: referência de outra origem não liga grupos;
    - I-2 → C-6.3a: `secure_delete` e checkpoint TRUNCATE;
    - I-5 → C-4.11c: recusar liberação para o lado oposto ao exportado;
    - I-6 → C-4.8a: herdar o lado anterior.
  - **Correções** (I-1 a I-9 e os 19 menores) delegadas a um Sonnet novo. O aceite é os 364 vetores OK sem enfraquecer nenhum teste.
- 2026-11-04 04:15: **correções da revisão 12.9 concluídas** (Sonnet): **364 vetores OK** (antes eram 39 FALHA: F 5, E 6, M 4, P 16, B 7), sem nenhum teste enfraquecido.
  - **Importantes:**
    - **I-1:** `ocorrencia_jobs.valida`; só job validado e de mesma origem liga grupos.
    - **I-2:** `secure_delete(ON)` e checkpoint TRUNCATE no `vacuum()`.
    - **I-3:** o export resolve as chaves como o decoder do Ollama (decisão: imitar, não recusar).
    - **I-4:** UTF-8 inválido é recodificado.
    - **I-5:** `ErrReleaseExported` (409).
    - **I-6:** tabela `lados` (só ids e HMACs) com herança do lado.
    - **I-7:** trailers zerados no `Rewrite`.
    - **I-8:** marcador removido em qualquer posição, até estabilizar; `FormatVersion` ex2.
    - **I-9:** `-candidato-score`; o mesmo `score.json` para atual e candidato reprova.
  - **Menores (19):**
    - m-6: `amostras.forma_ok` no `Eligible`;
    - m-10: `queue.Request.Internal` (a busca da melhora fora da cota por IP);
    - m-17: `correcao_aplicada` passa a ser obrigatório no schema e o `TOOLS.md` foi regenerado;
    - o `treino.db` passa para o `user_version` 4 (migração aditiva).
  - **Testes ajustados de propósito:** a liberação passou de "avisa" para "recusa" (C-4.11c), o export vencido passou a ser erro, e o golden ficou em ex2.
  - **Gate do coordenador:**
    - `build` 0; `vet` 0; `gofmt` vazio;
    - `-race -count=1 ./...` **24 ok**, 0 FAIL;
    - `go test -count=1 ./...`: na 1ª rodada, 23 ok e 1 FAIL em `TestRegress10LCapNotOnTheWritePath` (`gateway/content`, não tocado). Medido: 10/10 ok isolado no código atual e 10/10 no HEAD `fe8be32` (worktree temporário removido). É flake de tempo sob carga, como a D3 da 10.L. Na 2ª rodada completa, **24 ok**.
  - **Conjuntos antigos (Sonnet):** 9.5 30/30; 10.5 26/26; 10.L 22/23 (a D3 intermitente já conhecida).
  - **`src/`** sem cópias de teste de ataque (conferido); `.tmp-fix` removido.
- 2026-11-04 04:15: cópias de `deploy/`, `CLAUDE.md`, `README.md` e a memória alinhadas com os **3 slots** em produção.
  - **Erro do coordenador:** um `python -` vazio, que sobrou num comando, travou de novo esperando stdin. Encerrado (TaskStop); não tinha alterado nada. Refeito com o script gravado em arquivo.
- 2026-11-04 03:35: **plano da Fase 13** (rota automática) criado em `.workspace/plans/MCPOllam2026-11-04-033446-fase13-rota-auto.plan.md`; aguarda o owner.
