# Plano — Fase 12: o MCP organiza os erros do modelo local e gera o histórico para treinar o servidor de IA

> **Estado:** 📝 PLANO, **não aprovado**. Revisado **três vezes** por uma IA externa (GPT-6 Codex, 2026-11-02 15:23, 15:39 e 15:59), com os
> achados incorporados (§15). As questões da §14 foram fechadas pelo owner (15:50). Aguarda a aprovação final. Nada da Fase 12 foi implementado.
> **Escopo (owner, 2026-11-02 15:30):** **base confiável + medição + melhora sem treino**. A fase precisa **reduzir erros já**, sem mexer
> nos pesos do modelo (D12-15). O treino LoRA segue congelado (§10).
> **Versões alvo:** mcpollama `0.10.0` (ID do job visível + tool `reportar_erro`) e gateway `0.13.0` (captura canônica, base de treino,
> aba Erros, melhora sem treino).
> **Última atualização deste documento:** 2026-11-02 16:05. Fatos de código levantados em 2026-11-02, com `arquivo:linha` relativos a `src/`.
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

**Esta fase faz esses 5 itens e mais um 6º: uma melhora sem treino** (instruções melhores e exemplos recuperados, D12-15), medida pelo banco de provas. O treino em si fica congelado (§10).

## 7. Decisões (estado de cada uma)

> Revisadas em 2026-11-02 15:40 com os achados da revisão externa (§15). Onde a decisão mudou por causa dela, a linha diz **(rev.)**.

| # | Decisão | Estado |
| --- | --- | --- |
| D12-1 | **(rev.) ID do job visível e rastreável:** o mcpollama lê o `X-Request-Id` de **cada** chamada e acrescenta à linha `[uso]` (e a `_meta`) `job=<id>`, ou, nas tools de arquivo, `jobs=<papel>:<id>,…` com o papel de cada chamada (`map1..mapN`, `reduce1..`, `final`). **Cada ID leva o backend que o criou** (`primario` ou `fallback`), porque com o fallback ligado o relato precisa ir ao gateway certo; hoje só existe `backend=fallback` somado (`tools/filetools.go:408`) | proposta |
| D12-2 | **(rev.) Tool `reportar_erro`:** relata **uma ocorrência** (o resultado errado que o Claude viu), não jobs soltos. Argumentos: `jobs` (a lista do `[uso]`, com os papéis), `job_do_erro` (qual chamada produziu o erro; padrão: a `final`), `categoria` (D12-3), `descricao`, `resposta_correta` (**a resposta certa da chamada `job_do_erro`**, não do resultado montado), `correcao_aplicada`, `severidade`, `origem` (`claude`/`usuario`/`automatico`), `projeto`. Devolve o número da ocorrência | proposta |
| D12-3 | **Categorias fixas:** `numeracao_errada`, `contagem_errada`, `calculo_errado`, `inventou`, `omitiu`, `formato_invalido`, `resumo_infiel`, `traducao_errada`, `idioma_errado`, `truncado`, `ignorou_instrucao`, `outro` | proposta |
| D12-4 | **(rev.) Base de treino `treino.db`** no gateway, 0600. Modelo de dados em 3 níveis (§8.2): **ocorrência** (o relato lógico) → **jobs** da ocorrência (com papel) → **amostra** (a cópia do pedido canônico e da resposta de cada job). Ao receber o relato, o gateway copia do `content.db` as amostras de **todos** os jobs da ocorrência numa transação. Job já expirado → ocorrência `sem_pedido` (conta na estatística, nunca entra no treino). **(3ª rev.) Ocorrência distribuída** (os jobs da lista têm backends diferentes, D12-1): o relato vai ao gateway do `job_do_erro`, que guarda **só a ocorrência e as referências** de todos os jobs (com o backend de cada um) e **não copia nenhuma amostra**, nem as locais: o caso é só estatística (owner, 15:50). Estado `distribuida` | proposta |
| D12-5 | **(rev.) `POST /api/feedback`** como **rota própria no mux do gateway, antes do `/`** (hoje tudo fora de `/monitor` vai ao proxy e dá 404, `main.go:263-267` e `proxy.go:426`). Bearer via nginx (a `location /` atual já cobre `/api/`), corpo ≤ 256 KB, validação estrita. O `job_do_erro` precisa existir em `requests` do gateway que recebe; os jobs de **outro** backend (ocorrência distribuída) são aceitos só como referência, sem validação local. Aceita relato de qualquer estação; a confiança vem da **revisão do admin** (D12-16) | proposta |
| D12-6 | **Monitor:** aba **"Erros (treino)"** com a lista de ocorrências, filtros por categoria/tool/período/estação/estado, contagens e as ações **confirmar**, **completar a correção**, **descartar** e **apagar** (admin, com histórico). Botão **"Marcar erro"** no Detalhes do job (admin) | proposta |
| D12-7 | **(rev.) Regra ÚNICA de exportação** (predicado; substitui o texto anterior, que se contradizia com o risco 1). Uma amostra entra no JSONL se e somente se: (1) a ocorrência está `confirmada` pelo admin; (2) a amostra é o `job_do_erro` da ocorrência; (3) `resposta_correta` não está vazia; (4) o job é de **geração** (`/api/generate`, `/api/chat`, `/v1/chat/completions`, `/v1/completions`; nunca embeddings), terminou com `status=ok` e `http_status=200`; (5) pedido canônico e resposta **não truncados** (`prompt_truncated=0`, **`prompt_json_truncated=0`** (3ª rev.), `resp_final=1`, `resp_truncated=0`), conferidos nas colunas da **amostra**, que as copia do `content.db` no relato e as preserva depois que ele expira (§8.2); (6) a tool é elegível (`generate`, `chat`, `summarize_file`, `extract`, `translate`); **(7, 2ª rev.) o pedido não tem imagem** (`tem_imagem=0`); **(8) a ocorrência não é `distribuida`** (jobs em mais de um gateway) **nem está em `quarentena`** (D12-7a, item d); **(9) não é erro no resultado montado do `translate`**, que não tem chamada `final` (`tools/filetools.go:567`): no `translate`, só entra se o `job_do_erro` for uma parte (`mapN`) com a tradução certa daquela parte. Ficam de fora `nova`, `descartada`, `sem_pedido`, `distribuida`, respostas parciais, embeddings e pedidos com imagem, que contam **só na estatística nesta fase** (owner, 15:50) | proposta |
| D12-7a | **(2ª rev.) Partição treino/teste sem vazamento.** (a) **Identidade do documento:** o mcpollama, que lê o arquivo, envia em **cada** chamada das tools de arquivo o cabeçalho novo **`X-Mcpollama-Doc`** = sha256 do texto extraído do documento inteiro (vai por TLS). **(3ª rev., owner 16:00)** O gateway **não guarda o sha256 puro**, que permitiria correlacionar arquivos e confirmar palpites sobre um documento conhecido: grava em `requests.doc_hash` só **HMAC-SHA256(chave, sha256)**, com a chave num arquivo 0600 gerado pelo instalador (`--doc-key-file`, biblioteca padrão), e remove o cabeçalho antes do Ollama. Trocar a chave quebra os grupos antigos: só com o owner. (b) **Grupos:** ocorrências ligadas pelo **mesmo documento ou por um job em comum** formam um grupo (componente conectado), e o grupo inteiro cai do mesmo lado. (c) **Cota por categoria** (owner, 15:50): os primeiros **30** grupos confirmados de cada categoria vão para o **teste**; depois disso, 15% (`sha256(id do grupo) mod 100 < 15`). (d) A partição é **estável** depois de atribuída. **(3ª rev., owner 16:00) Quarentena:** se uma ocorrência nova ligar grupos que já estão em lados diferentes, o **componente inteiro** (todos os grupos unidos mais a nova) vai para `quarentena`: fica fora da exportação de treino e de teste, do índice de exemplos e da avaliação. Os **retratos** que tinham casos desse componente são **invalidados** (a próxima comparação exige retrato e linha de base novos), assim como o índice de exemplos congelado, se usava algum deles. O admin vê o caso na aba e pode liberá-lo, escolhendo um lado para o componente inteiro, com histórico. (e) **Retrato do teste:** cada avaliação usa um **retrato congelado** do conjunto de teste (lista de ocorrências e jobs, com ID e hash), o mesmo para a linha de base e para a comparação; um retrato novo exige uma linha de base nova | proposta |
| D12-8 | **(rev.) Candidatos automáticos com contrato:** cada regra tem critério objetivo, e todas geram ocorrência `origem=automatico`, estado `nova`, **sem** resposta certa. Elas só entram no treino depois que o admin completa a correção e confirma. Regras: (a) **JSON inválido** quando o pedido **declarou** JSON no campo `format` (`"json"` ou um schema JSON) e o texto não passa por `json.Valid`. **(3ª rev.)** "A instrução pede JSON" saiu, porque não tem critério objetivo; esses casos só entram por relato humano; (b) **vazio**: a resposta só com espaços; (c) **truncado**: `fim=length` **e** a tool não pediu limite curto (`num_predict` ≥ 256); (d) **idioma errado**: **fora desta fase**, porque o critério é subjetivo. Deduplicação: um relato humano da mesma ocorrência **substitui** o automático. Meta de falso-positivo: acompanhar a taxa de "descartado" por regra no monitor; regra com > 50% de descarte em 50 casos é desligada. **Padrão: ligados** (owner, 15:50) | proposta (padrão **decidido**) |
| D12-9 | ~~Registro provisório em arquivo local~~: **descartado pelo owner** (2026-11-01 21:45). O registro começa só com a tool pronta | **decidida** |
| D12-10 | **(rev.) Retenção e descarte do `treino.db`:** sem expiração automática, mas com **teto de 2 GiB**. Acima disso, recusa novos relatos com aviso no monitor, sem apagar nada sozinho, porque são dados curados. **Apagar** é ação do admin, com histórico: apaga a ocorrência, os jobs e as amostras. A exportação é **gerada na hora** e não fica guardada no servidor; o arquivo baixado é responsabilidade de quem baixou, e isso fica registrado no histórico. **Backup:** o `treino.db` fica **fora** do backup comum, ou só entra cifrado, como o `content.db`. Treinar fora da rede exige anonimização (§10). **(2ª rev.)** O `historico` **nunca guarda texto sensível**: nos campos de conteúdo (`resposta_correta`, `descricao`, `correcao_aplicada`, pedidos, respostas) registra só **hash e tamanho** de antes e depois. **Apagar** usa contagem de referências: a amostra e o embedding só saem quando nenhuma ocorrência os usa mais. **Alterar** uma correção já confirmada **desfaz a confirmação** (volta a `nova`) e **apaga o embedding** anterior | proposta |
| D12-11 | **Sem dependências de terceiros:** tudo em Go, biblioteca padrão + o que já está no `go.mod` (regra do owner). Exceção futura: o treino (§10), em Python, **fora** deste repositório | **decidida** |
| D12-12 | **Preferência pelo `mcpOllama`** no `~\.claude\CLAUDE.md` global (texto na §9.1), **aplicada só na etapa de documentação** | **decidida** (owner, 2026-11-01 21:40) |
| D12-13 | **(rev.) Captura canônica do pedido na ENTRADA** (a proposta anterior, copiar o JSON no relato, não funciona, porque hoje o JSON não fica guardado em lugar nenhum; o `content.db` só tem o texto achatado de `extractPrompt`, `proxy.go:483`, `journal.go:50`). Nova coluna `prompt_json` no `content.db`, gravada no `SavePrompt`, com o **corpo JSON já com a política aplicada** (o que o Ollama de fato recebe): `system`, `prompt`/`messages` completos (todo o histórico do `chat`), `options`, `format`, `think`, `model`. Imagens são substituídas por um marcador com o hash. **(2ª rev.)** Teto de **16 MiB separado** para cada campo (texto achatado e `prompt_json`), cada um com a sua marca de truncado (`prompt_truncated`, `prompt_json_truncated`). **Pedido com imagem:** o `prompt_json` guarda um marcador com o hash no lugar da imagem e a linha recebe `tem_imagem=1`, o que a deixa **não exportável e fora da avaliação** (D12-7, item 7). **Teto total do `content.db`: 4 GiB, com retenção de 7 dias** (owner, 15:50) | proposta |
| D12-14 | **Instruções onde o Claude lê:** `docs/RELATAR-ERROS-IA.md` (fonte única) + seção curta no `~\.claude\CLAUDE.md` global apontando para ele. Entra na etapa de documentação, junto com a tool | **decidida** (owner, 2026-11-01 21:45) |
| D12-15 | **(novo) Melhora sem treino**, dentro desta fase (escopo do owner, 2026-11-02 15:30). Duas frentes, medidas antes e depois pelo banco de provas: (a) **instruções por tool** reescritas a partir das categorias que mais erram (ex.: no `extract`, "use a numeração do corpo do documento, nunca a do sumário"). **(3ª rev.)** Essa análise usa **só a partição de treino**, nunca a de teste. Cada versão das instruções ganha um ID e um hash e é **congelada antes da avaliação**; o retrato registra a versão usada; (b) **exemplos recuperados**: para cada pedido de uma tool, o gateway busca por embedding (`nomic-embed-text`) as ocorrências **confirmadas** parecidas e inclui até k pares "erro → certo" como exemplo no pedido. **Restrição de privacidade (2ª rev., owner 15:50):** os exemplos vêm **só de ocorrências marcadas pelo admin como "exemplo público"** (sintéticas ou anonimizadas); **nenhum documento de cliente é injetado no pedido de outro**. "Mesmo projeto" fica para uma fase futura, porque hoje não existe identidade de projeto no pedido e um valor declarado não autoriza acesso. **Contra vazamento na avaliação:** a busca usa **só a partição de treino**; o índice de exemplos é **congelado** junto com o retrato do teste antes de cada avaliação; a ocorrência, o grupo e o documento em avaliação ficam fora da busca. Ligável por tool e por flag, com padrão desligado até a medição provar ganho | proposta |
| D12-16 | **(novo) Fronteira de confiança = revisão do admin** (owner, 2026-11-02 15:30). Qualquer estação relata (dados declarados, não autenticados); **nada entra no treino, nem nos exemplos da D12-15, sem confirmação do admin no monitor**. **Histórico completo** de cada ocorrência (tabela `historico`): criação, cada alteração (campo, antes, depois), revisão, apagamento, exportação; com quem fez (estação declarada ou `admin`), IP e quando | **decidida** |
| D12-17 | **(novo) Protocolo de avaliação** (§8.5): o que "melhorou" significa, com números. O gate **falha** se a suíte não rodar, se rodar zero casos ou se faltar o mínimo por categoria | proposta |

## 8. Desenho técnico detalhado (proposta, revisada em 2026-11-02)

### 8.1 Contrato de dados (etapa 12.1, antes de qualquer código)

É um documento curto, `docs/CONTRATO-DADOS-TREINO.md`, aprovado pelo owner. Ele fixa:
1. a **unidade do erro**: a ocorrência;
2. o formato do **pedido canônico**: D12-13;
3. a **regra única de exportação**: D12-7;
4. a **partição**: D12-7a;
5. o **protocolo de avaliação**: D12-17, §8.5;
6. a **retenção e o descarte**: D12-10;
7. o **histórico**: D12-16;
8. a **regra de privacidade dos exemplos**: D12-15b.

Todas as etapas seguintes testam contra esse contrato. Se o código divergir dele, o contrato vence, ou é alterado com aprovação.

### 8.2 Gateway 0.13.0: dados

- **`gateway.db`** (2ª rev.): coluna nova `requests.doc_hash` = **HMAC-SHA256** do valor do `X-Mcpollama-Doc`, com a chave do `--doc-key-file` (D12-7a, 3ª rev.).
- **`content.db`** (D12-13): colunas novas `prompt_json BLOB`, `prompt_json_truncated` e `tem_imagem`, por migração idempotente. Ela é gravada em `SavePrompt` (`content.go:195`),
  chamado por `joblog.go:42`, a partir do **corpo depois da política** (`proxy.go:454`, `applyPolicy`), e não do texto de `extractPrompt`.
  Teto de 16 MiB **por campo**, cada um com a sua marca de truncado; teto total do `content.db` de **4 GiB** (owner, 15:50).
- **`treino.db`**: pacote novo `gateway/treino`, migração no molde de `store.migrate`, consultas da aba com `context.Background()` e cache em
  segundo plano (padrão 0.12.6). Tabelas:
  - `ocorrencias`:
    - identificação: `id` (hex 32), `criada_em`, `atualizada_em`, `estado` (`nova`/`confirmada`/`descartada`/`sem_pedido`/`distribuida`/`apagada`);
    - o erro: `categoria`, `severidade`, `descricao`, `resposta_correta`, `correcao_aplicada`;
    - origem: `origem`, `projeto`, `tool`, `estacao_computer/user/domain/mac`, `client_ip`, `client_label`;
    - ligação e treino: `job_do_erro`, `doc_hash`, `grupo_id`, `particao` (`treino`/`teste`/`quarentena`), `distribuida` (0/1), `exemplo_publico` (0/1, D12-15b);
    - revisão: `revisada_por`, `revisada_em`.
  - `ocorrencia_jobs`: `ocorrencia_id`, `job_id`, `papel` (`map1..`, `reduce1..`, `final`, `unico`), `backend`, `ordem`. PK
    (`ocorrencia_id`, `job_id`).
  - `amostras`: `job_id PK`, `copiada_em`, `endpoint`, `modelo`, `think`, `prompt_json`, `prompt_texto`, `resposta`, `prompt_sha256`,
    `prompt_truncado`, **`prompt_json_truncado`**, **`tem_imagem`**, **`doc_hash`** (o HMAC), `resp_final`, `resp_truncada`, `status`, `http_status`.
    Um job pode pertencer a mais de uma ocorrência. **(3ª rev.)** Todas as marcas que o predicado da D12-7 confere são **copiadas para a
    amostra no relato**, para que a exportação não dependa do `content.db`, que expira em 7 dias.
  - `historico`: `id`, `quando`, `ocorrencia_id`, `acao` (`criou`/`alterou`/`confirmou`/`desconfirmou`/`descartou`/`apagou`/`exportou`), `campo`,
    `antes`, `depois`, `quem`, `ip`. Nos campos de conteúdo, `antes`/`depois` guardam **só hash e tamanho** (D12-10).
  - `retratos` (D12-7a): `id`, `criado_em`, `categoria`, lista de (`ocorrencia_id`, `job_id`, `prompt_sha256`), o hash do índice de exemplos congelado, o ID/hash da versão das instruções (D12-15a) e `valido` (0 quando uma quarentena o invalida).
  - `embeddings` (D12-15b): `ocorrencia_id`, `vetor` (768 floats), `modelo` (`nomic-embed-text`). Calculado ao **confirmar**.
- **Teto:** 2 GiB; acima disso, novos relatos são recusados, com evento e aviso no monitor (D12-10).

### 8.3 Gateway 0.13.0: rotas

- `POST /api/feedback`:
  - **rota própria** registrada no mux **antes** do `/` (`main.go:263-267`);
  - autenticação: o Bearer do nginx;
  - corpo ≤ 256 KB, validado como `readSmallJSON`/`parseStation`;
  - grava a ocorrência, os jobs e as amostras **numa transação**;
  - devolve `{ocorrencia_id, estado, amostras_copiadas, sem_pedido, jobs_externos}`. Numa ocorrência distribuída: `estado=distribuida`,
    `amostras_copiadas=0` e `jobs_externos` = quantos jobs são de outro backend (D12-4, 3ª rev.).
  - Pedido vindo do fallback: o relato vai ao gateway que criou o job (D12-1); o primário recusa ID que não existe nele com 404 claro.
- **Monitor, só admin, com sessão, CSRF e same-site e gravando no `historico`:** `GET /monitor/api/treino` (lista),
  `GET /monitor/api/treino/{id}`, e `POST /monitor/api/treino/{id}/confirmar|descartar|completar|apagar|exemplo-publico`.
- **Exportação:** `GET /monitor/api/treino/export?parte=treino|teste` (admin + same-site), com o predicado da D12-7 e a partição da D12-7a,
  gerada na hora em streaming e registrada no `historico`.

### 8.4 mcpollama 0.10.0

- **IDs:** o `ollama/client.go` guarda o `X-Request-Id` e o backend de cada resposta (`Usage.Jobs []JobRef{ID, Papel, Backend}`). O
  `tools/filetools.go` atribui os papéis (`map<i>`, `reduce<r>`, `final`). Em `usage.go`, `job=` ou `jobs=` vai na linha `[uso]` e em `_meta`.
- **(2ª rev.) `X-Mcpollama-Doc`:** nas tools de arquivo, o mcpollama calcula uma vez o sha256 do texto extraído do documento inteiro e o envia em todas as chamadas daquele pedido (map, reduce e partes) (D12-7a).
- **`reportar_erro`** (`tools/reportar_erro.go`):
  - schema com `enum` para categoria, severidade e origem;
  - limites: `descricao` ≤ 4 KB e `resposta_correta` ≤ 128 KB;
  - `readOnlyHint=false`;
  - registrada sem depender de `--allow-root`;
  - envia ao backend do `job_do_erro`.
- **Automáticos (D12-8):** o mcpollama checa as regras depois da resposta e dispara o relato **em segundo plano**, sem atrasar a resposta nem
  falhar a tool. Flag `--auto-report` (padrão: **ligado**, owner 15:50).

### 8.5 Protocolo de avaliação (D12-17; `bakeoff -suites erros`, a criar)

- **Entrada:** as amostras da partição **teste** que passam no predicado da D12-7, exportadas como JSONL.
  - **Replay:** o pedido canônico é reenviado **igual** (`prompt_json`) ao modelo candidato, pelo gateway, com `X-Mcpollama-Tool: bakeoff-erros`.
- **Pontuação por categoria:**
  - **automática** onde há critério determinístico:
    - `numeracao_errada` e `omitiu`: lista de itens comparada com a resposta certa, com precisão e cobertura;
    - `formato_invalido`: `json.Valid` **e** igualdade dos **valores** com a resposta certa, depois de normalizar espaços e ordem de chaves (2ª rev.: comparar só as chaves deixaria passar valores errados);
    - `contagem_errada` e `calculo_errado`: número final igual;
    - `truncado`: `fim=stop` e conteúdo completo;
  - **humana às cegas** em `resumo_infiel`, `inventou`, `traducao_errada` e `ignorou_instrucao`: o modo `blind` do `bakeoff` embaralha a
    saída do modelo atual e a do candidato, e **só o owner** (ou outra pessoa da rede) escolhe sem saber qual é qual. **(2ª rev.) O Claude
    não avalia casos com conteúdo de cliente**, porque isso mandaria o documento para fora da rede; só casos anonimizados;
  - **sem critério nesta fase:** `outro` e `idioma_errado` ficam **fora da avaliação** e contam só na estatística.
- **Mínimos:** uma categoria só é **reportada** com ≥ 20 casos de teste, e só **decide** com ≥ 30. Com a **cota** da D12-7a (os 30 primeiros
  grupos de cada categoria vão para o teste), o mínimo para decidir chega com 30 confirmados por categoria, sem depender dos 15%.
- **Retrato:** toda comparação diz o ID do retrato do teste usado; linha de base e candidato **sempre no mesmo retrato**.
  - O relatório mostra N, acerto por categoria e o intervalo de confiança (Wilson 95%).
- **Limiares de aceite de uma mudança** (instrução nova, exemplos da D12-15 ou um modelo v2):
  - **melhora:** ganho ≥ 10 pontos percentuais na categoria-alvo, com o intervalo inferior acima do acerto atual;
  - **sem regressão:** nenhuma outra categoria cai mais de 3 pontos;
  - e o **S2/S3 da Fase 8** não cai mais de 2 pontos.
- **Gate:** falha se a suíte não rodar, se avaliar 0 casos ou se nenhuma categoria atingir o mínimo. Nesse caso, o relatório diz "dados
  insuficientes", e nunca "sem regressão".
- **Linha de base:** a primeira rodada, com o `qwen3-ia` atual e sem a D12-15, fica registrada no relatório da etapa e é a referência
  para tudo o que vier depois.

### 8.6 Exemplo de linha exportada (formato de chat)

```json
{"messages":[
 {"role":"system","content":"<system do prompt_json: fileSystemPrompt da tool extract>"},
 {"role":"user","content":"<prompt do prompt_json: instrução + ARQUIVO_INICIO <nonce> ... ARQUIVO_FIM <nonce>>"},
 {"role":"assistant","content":"<resposta_correta da ocorrência>"}],
 "meta":{"ocorrencia_id":"<hex32>","job_id":"<hex32>","papel":"final","tool":"extract","categoria":"numeracao_errada","particao":"treino"}}
```

Para `chat`, as `messages` vêm completas do `prompt_json`, com todo o histórico, e a última mensagem do assistente é a resposta certa.

## 9. Instruções para o Claude dos projetos

- **Arquivo:** `docs/RELATAR-ERROS-IA.md` (já escrito; sem commit até o owner aprovar o plano). Diz:
  - **quando** relatar (o Claude conferiu um resultado do Qwen e está errado; ou o usuário apontou) e quando **não** relatar (erro do
    pedido, erro de infraestrutura, repetição);
  - **o que** registrar (§2 do md);
  - **como** registrar: só pela tool, sem registro alternativo;
  - **o que é uma boa correção:** resposta inteira, no formato pedido.
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
  - nuvem **com dados anonimizados**, porque são documentos de clientes.
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
  - **exemplos de correções passadas** buscados por embedding (`nomic`) e incluídos no pedido (few-shot), um "aprender" sem mexer nos pesos.

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
| 12.0 | Aprovação do plano e das decisões abertas (§14) | owner | owner aprova | **pendente** |
| 12.1 | **Contrato de dados** (`docs/CONTRATO-DADOS-TREINO.md`, §8.1): unidade do erro, pedido canônico, regra de exportação, partição, avaliação, retenção, histórico, privacidade dos exemplos | sonnet + owner | owner aprova o contrato | pendente |
| 12.2 | mcpollama: `job=`/`jobs=<papel>:<id>` com o backend de cada ID, na `[uso]` e em `_meta` (D12-1); cabeçalho `X-Mcpollama-Doc` (D12-7a) | sonnet | teste com gateway falso: IDs e papéis certos no map-reduce e no fallback; o mesmo `X-Mcpollama-Doc` em todas as chamadas de um documento | pendente |
| 12.3 | gateway: `prompt_json` no `content.db` na **entrada** (D12-13) | sonnet | teste: o `prompt_json` é igual ao corpo que o Ollama falso recebeu, inclusive todo o histórico de `chat`; migração de banco antigo; teto de 16 MiB **por campo** com marca própria, e o predicado recusa `prompt_json_truncated=1` mesmo depois de o `content.db` expirar; imagem vira marcador + `tem_imagem=1`; `requests.doc_hash` gravado e o cabeçalho removido antes do Ollama | pendente |
| 12.4 | gateway: `treino.db` (ocorrências, jobs, amostras, histórico) + `POST /api/feedback` em rota própria (D12-4, 5, 10, 16) | sonnet | testes: validação, transação, `sem_pedido`, teto, histórico; ataques 9.5/10.L/10.5 ok | pendente |
| 12.5 | mcpollama: tool `reportar_erro` (ocorrência, `job_do_erro`) + automáticos (a)–(c) (D12-2, 3, 8); `TOOLS.md` | sonnet | o relato chega com todos os jobs e papéis; o automático não atrasa nem quebra a tool; deduplicação | pendente |
| 12.6 | monitor: aba "Erros (treino)", "Marcar erro", confirmar/completar/descartar/apagar/exemplo público, exportação com predicado e partição (D12-6, 7, 7a) | sonnet | Firefox headless 570–1400 px; o predicado exclui cada caso proibido, inclusive imagem, `distribuida` e `translate` montado (teste por caso); grupos por documento ou job em comum; **união de lados diferentes vai para quarentena e invalida os retratos** (teste dedicado); `doc_hash` guardado como HMAC, nunca o sha256 puro; ocorrência distribuída sem amostras; cota de 30 por categoria; partição estável e sem vazamento entre treino e teste; histórico sem texto sensível; alterar correção confirmada desfaz a confirmação | pendente |
| 12.7 | banco de provas: `bakeoff -suites erros` com o protocolo da §8.5 + **retrato** do teste + **linha de base** do `qwen3-ia` atual | sonnet | relatório com N, acerto e intervalo por categoria e o ID do retrato; `formato_invalido` compara valores; `outro`/`idioma_errado` fora; o gate falha com 0 casos | pendente |
| 12.8 | **melhora sem treino** (D12-15): (a) instruções por tool; (b) exemplos recuperados por embedding, com a regra de privacidade | sonnet | **medido pela 12.7** com os limiares da §8.5; só fica ligado o que passar | pendente |
| 12.9 | revisão adversarial: relato forjado, flood, corpo gigante, injeção no JSONL exportado, **vazamento entre clientes pelos exemplos (D12-15b)**, **resposta do teste vazando pelos exemplos (B1)**, apagamento incompleto (histórico, amostras compartilhadas), vazamento na aba, CSRF, rota `/api/feedback` | **opus** | achados corrigidos; testes de ataque guardados em `.workspace/reports/...testes/` | pendente |
| 12.10 | gate completo + implantação (gateway 0.13.0, mcpollama 0.10.0) + aceite ao vivo com o owner | coordenador + owner | relato real pela tool e pelo monitor; confirmação; exportação com o par certo; relatório da 12.7 antes e depois da 12.8 | pendente |
| 12.11 | documentação + **`~\.claude\CLAUDE.md` global** (§9.1 e o apontador para `RELATAR-ERROS-IA.md`) | sonnet + coordenador | instruções em vigor; o Claude de outro projeto consegue relatar | pendente |

## 13. Riscos

1. **Correção errada ensina errado:** nada entra no treino nem nos exemplos sem **confirmação do admin** (D12-16), e o histórico registra
   quem confirmou.
2. **Volume baixo:** os automáticos geram candidatos, e a instrução global faz todo projeto relatar. A avaliação só decide com os mínimos da §8.5.
3. **Conteúdo sensível sem prazo:** D12-10 (teto, apagar com histórico, exportação gerada na hora e registrada, backup fora ou cifrado).
4. **Vazamento entre clientes pelos exemplos (D12-15b):** exemplos **só públicos/anonimizados** (owner, 15:50), com ataque dedicado na 12.9.
5. **Teste "fácil" por vazamento:** partição por grupo (documento ou job em comum), estável, com retrato congelado; exemplos só da partição
   de treino, com índice congelado (D12-7a, D12-15).
6. **Medir errado:** o gate falha sem dados e nunca relata "sem regressão" por falta de casos (§8.5).
7. **`content.db` maior com o `prompt_json`:** teto sobe para 4 GiB com 7 dias (owner, 15:50). Medir na 12.3; se ainda apagar antes dos 7 dias, voltar ao owner.
10. **Poucos casos exportáveis:** imagem, `distribuida` e `translate` montado ficam só na estatística. Acompanhar a proporção na aba; se for alta, reabrir com o owner.
11. **Correlação de documentos pelo hash:** o gateway guarda só o HMAC com chave própria (D12-7a). A chave fica fora do backup comum, como o `content.db`.
8. **Impasse ou travamento em consultas novas:** padrão 0.12.6 (ctx próprio, cache em segundo plano) e teste de regressão.
9. **Relato forjado:** os dados são declarados, mas a revisão do admin é a barreira. O histórico guarda IP e estação de quem relatou.

## 14. Questões do owner (fechadas em 2026-11-02 15:50)

1. D12-8: automáticos **ligados** por padrão.
2. D12-13: `content.db` com **teto de 4 GiB e retenção de 7 dias**.
3. D12-15b: exemplos **só públicos/anonimizados**; "mesmo projeto" fica para o futuro.
4. §8.5: limiares de +10 pontos na categoria-alvo, no máximo −3 nas outras e no máximo −2 no S2/S3; mínimos de 20 para reportar e 30 para decidir.
5. §10: treino com ≥ 300 confirmados na partição de treino e ≥ 50 por categoria.
6. Casos compostos (jobs em 2 gateways, pedidos com imagem, resultado montado do `translate`): **só estatística nesta fase**.
7. Conjunto de teste: **cota de 30 por categoria + retrato congelado**.
8. (16:00) Grupo que une lados diferentes: **quarentena** do componente inteiro e invalidação dos retratos afetados.
9. (16:00) Hash do documento: **HMAC-SHA256 com chave do gateway**, nunca o sha256 puro.

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
