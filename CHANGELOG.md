# Changelog

Histórico de versões do **mcpollama** (servidor MCP, na estação) e do **mcpollama-gateway** (fila + `/monitor`, no servidor
de IA). O formato segue o [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/); as versões não têm release formal no
GitHub, só **tags anotadas** nas três marcadas abaixo (`mcpollama-v<versão>` e `gateway-v<versão>`).

Fontes: commits, tags, e os planos e reports de `.workspace/`. As versões vêm do `--version` do binário
(`<versão>-<hash curto do commit>`, definido na compilação por `-X main.Version`). O que não deu para confirmar está marcado
**(data/versão a conferir)**.

## mcpollama

### [0.9.0] - 2026-10-30 (tag `mcpollama-v0.9.0`)

Binário `0.9.0-c4031f7` gerado em `D:\MCP\mcpollama-mcp.exe` em 2026-10-30; vale após reiniciar as sessões do Claude Code.

#### Corrigido
- Mensagem do 403 da lista de modelos do gateway: um 403 com JSON `... não permitido neste servidor` agora diz **"modelo não
  permitido pelo servidor"** em vez de "endpoint bloqueado pelo nginx" (o 403 HTML do nginx não mudou). Nova classe de erro
  `ErrModelNotAllowed` (também é `ErrForbidden`); sem mudança nas descrições das tools nem no `tools/list`.
- Teste `TestConnectionDroppedMidStream` (Fase 2) determinístico (e novo `TestConnectionClosedMidStreamKeepsPartial`): só testes mudaram, não o código do cliente.

#### Adicionado
- Fallback opcional para um segundo servidor (`5a77853`, 2026-10-30): flags `--ollama-url-fallback`, `--fallback-api-key` e
  `--fallback-tls-ca` (env `OLLAMA_HOST_FALLBACK`, `OLLAMA_FALLBACK_API_KEY`, `OLLAMA_FALLBACK_TLS_CA`). **Desligado por
  padrão.** Tenta o fallback **uma vez** em erro de conexão ou 503 de fila cheia; a linha `[uso]` ganha `backend=fallback`.
  O token do primário nunca vai para o fallback, e o `--insecure` não vale para ele.

### [0.8.1] - 2026-10-29 (tag `mcpollama-v0.8.1`)

#### Adicionado
- Argumento `think` por chamada em `generate`, `chat`, `extract`, `summarize_file` e `translate` (Fase 8.8b, `26c8bc1`): o
  padrão continua `false` (rápido); `true` faz o modelo raciocinar. Em produção desde 2026-10-29 15:11.

### [0.8.0] - 2026-10-29

#### Adicionado
- Cliente do gateway (Fase 9.4): envia `X-Mcpollama-Client` (rótulo, `--client-label`) e `X-Mcpollama-Tool`, lê `X-Queue-Ms`
  e repete **uma vez** em 503 com `Retry-After` (409 não é repetido). `queue_ms` na linha `[uso]`.
- `think:false` em toda geração (Fase 8.8, `e519221`): o `qwen3-ia` passou a ser o `qwen3.5:9b`, que raciocina por padrão.

#### Alterado
- Binário `0.8.0-fa8c95e` instalado em `D:\MCP` em 2026-10-29 (junto do merge da Fase 9.1 a 9.5).

### [0.7.0] - 2026-10-28

#### Adicionado
- OCR (Fase 7.9), opt-in por `--ocr-lib` + `--ocr-lib-sha256`: Tesseract 5.3.3 em processos filhos isolados; PDF digitalizado,
  `.png` e `.jpg`/`.jpeg`. Sem as flags, o PDF digitalizado segue recusado. Binário `0.7.0-9f09aaa`.

### [0.6.0] - 2026-10-28

#### Adicionado
- Leitura de `.docx`, `.pptx` e `.pdf` com camada de texto (Fase 7; `src/doctext/`), pelas tools de arquivo; PDF por go-pdfium
  em WASM/wazero, sem cgo. Flags `--max-doc-bytes` e `--pdf-cache-dir` (cache de compilação do motor de PDF).
  Binário `0.6.0-4538523`.

### [0.5.2] - 2026-10-26

#### Alterado
- `generate` avisa que contas e contagens feitas pelo modelo não são confiáveis (`10e879a`). Binário `0.5.2-10e879a`.

### [0.5.1] - 2026-10-26

#### Alterado
- `embed` avisa sobre a descarga do modelo (`0b2aab6`). Binário `0.5.1-0b2aab6`.

### [0.5.0] - 2026-10-26

#### Adicionado
- `--allowed-models` (padrão `qwen3-ia;nomic-embed-text`): só modelos otimizados para o hardware; o modelo fora da lista é
  recusado na partida e por chamada (`6acd0d3`). Binário `0.5.0-19c3b8c`.

### [0.1 a 0.4] - 2026-10-23 a 2026-10-24 **(data/versão a conferir)**

#### Adicionado
- Fases 1 a 6: esqueleto do protocolo MCP (stdio, JSON-RPC 2.0, sem SDK externo); cliente Ollama (stream interno, TLS com
  `--tls-ca`, mapa de erros, redação do token); tools `list_models`, `generate`, `chat` e `embed`; tools de arquivo
  `summarize_file`, `extract` e `translate` (`--allow-root`, guard em `src/files/`); integração com o Claude Code (servidor
  `mcpOllama`); medição da economia (−66% de custo Anthropic num arquivo de ~72 mil caracteres).
- Os números de versão desse período não foram encontrados nos commits nem nos planos.

## mcpollama-gateway

Código em `src/gateway/` e `src/cmd/mcpollama-gateway/`; instalação em [`deploy/gateway/`](deploy/gateway/README.md) e operação
em [`docs/MANUAL-GATEWAY.md`](docs/MANUAL-GATEWAY.md). Todas as datas abaixo são de 2026-10-30, hora local.

### [0.12.1] - 2026-10-30 (tag `gateway-v0.12.1`)

Em produção desde 20:46 (`0.12.1-c4031f7`, instalado por `Instalar-Gateway.ps1 -Modo Atualizar`; correções pós-aceite da
Fase 10.7).

#### Corrigido
- **Sobrecarga S1 não conta mais o próprio pedido** (10.7-1): a espera estimada passa a ser EWMA × (esperando à frente +
  executando) ÷ slots; antes, um pedido isolado com o primário livre dava EWMA × 2 ÷ slots e ligava `overload_on` sem motivo
  quando a geração média passava de metade de `--overload-wait`. A S2 e a histerese de calma passam a contar só a fila que
  não cabe nos slots livres. Só age com secundário configurado.
- **"Último erro" do backend** some do monitor quando ele volta a ficar saudável (`backend_up`).
- **Eventos do disjuntor**: no máximo 1 `breaker_open` e 1 `breaker_close` por minuto por backend; o evento seguinte informa
  quantos iguais foram suprimidos. O estado do disjuntor no quadro não muda.

### [0.12.0] - 2026-10-30 (tag `gateway-v0.12.0`)

Em produção desde 19:43 (`0.12.0-736c205`, merge das Fases 10.4 e 10.5).

#### Adicionado
- **Pool de backends** (Fase 10.4): registro `backends.json` (modo 0600, flag `--backends`, lido só na partida), saúde ativa e
  passiva, disjuntor (só falha de conexão e checagem ativa contam), repasse por sobrecarga a um secundário da LAN
  (`--overload-wait`, `--overload-queue`), "drenar" e "retomar" pelo monitor, quadro **Backends** e coluna Backend.
  **Sem `--backends`, o pool é um backend só (`gpu-local`) e o comportamento é o de antes**; é o que roda em produção.
- Revisão adversarial do pool (Fase 10.5; `b1d8418`): correções no disjuntor do secundário, no quadro Modelos, no tratamento de
  redirect do secundário, no corte depois da conexão, no fallback e na validação de porta.
- Seção 16 do manual (pool e secundário).

#### Descartado
- **Faixa CPU/RAM** (D9-C1): a medição das Fases 10.1 e 10.2 deu saldo de vazão ~0. Não foi implementada.

### [0.11.1] - 2026-10-30 (tag `gateway-v0.11.1`)

Em produção desde 18:09 (`0.11.1-f7975fb`).

#### Corrigido
- Correções da revisão adversarial do log de jobs (`cd71ed8`, `998a70d`): hash e tamanhos só para o admin, teto do conteúdo em
  segundo plano (e que não gira quando o `enforceCap` falha), `content.db` corrompido não derruba o gateway.

### [0.11.0] - 2026-10-30

Em produção às 17:29 (`0.11.0-6cbaf22`).

#### Adicionado
- **Log de jobs durável (Fase 10.L):** entrada, início e fim de cada job; prompt e resposta completos em `content.db` (modo 0600,
  7 dias, texto só para o admin); extrato das GPUs a 1 s por job; aba "Log em tempo real" com Detalhes e exportação JSONL/CSV;
  jobs que sobrevivem a reinício ficam como `interrompido_reboot` ou `interrompido_gateway`.

### [0.10.1] - 2026-10-30

Em produção às 14:25 (`0.10.1-e0d273c`).

#### Corrigido
- Modelo fixo na memória (`keep_alive -1`) aparece como "permanente" no quadro de Modelos, e não como uma data no ano 2319.

### [0.10.0] - 2026-10-30 **(data/versão a conferir)**

Não há registro da instalação desta versão; pelos commits, ficou entre as 13:54 e as 14:24.

#### Adicionado
- Monitor: Concluídos recentes e Eventos em abas (`9d90ba6`); quadro CPU/RAM por thread, limite de W das GPUs, quadro de Modelos
  e atualização a 1 s (`ea4b4dc`).

### [0.9.2] - 2026-10-30 **(data/versão a conferir)**

Não há registro da instalação desta versão; pelos commits, provavelmente ficou por volta das 12:35.

#### Corrigido
- `/monitor` ficava exposto pela internet via túnel da Cloudflare (chegava como `::1` e o loopback era liberado): o nginx passou a
  recusar pedidos com `CF-Connecting-IP`, e o loopback deixou de contar como LAN (`2a16888`).

### [0.9.1] - 2026-10-30

Instalado pelo `-Modo Atualizar` por volta das 12:20 (`0.9.1-7a86350`).

#### Corrigido
- `/monitor` bloqueado pelo CSP com o HTML em CRLF (hash dos bytes brutos x hash do navegador): o HTML é normalizado para LF antes do
  CSP e de servir (`7a86350`).
- Instalador: no `-Modo Atualizar`, o PowerShell 5.1 removia as aspas duplas do comando remoto e o `curl -w` se desfazia (`1e5e70c`).

### [0.9.0] - 2026-10-30

Primeira versão em produção, às 12:12:40 (`0.9.0-2cb599d`; etapa 2 do instalador).

#### Adicionado
- Fila com 2 faixas (geração, 1 slot; embedding, 2 slots), prioridade (`alta`, `normal`, `baixa`), justiça por IP e por rótulo,
  limites (fila 64, 16 por IP/rótulo, espera e execução de 10 min) e escalonamento (Fase 9.1).
- Proxy com streaming e rotas diretas e bloqueadas (9.2); SQLite com WAL, gravador em lote e retenção; coleta de GPU por
  `nvidia-smi` e AER (9.3).
- `/monitor` ao vivo por SSE, com login do `admin`, CSRF e ações (promover, despromover, remover, cancelar) auditadas (9.4);
  revisão adversarial com 30 vetores e 12 achados corrigidos (9.5).
- Políticas do gateway (9.6a): raciocínio desligado por padrão e remoção silenciosa de `keep_alive`, `num_ctx`, `num_gpu` etc.
- Instalador `Instalar-Gateway.ps1`, scripts de etapa, unit do systemd e `ollama.conf` do nginx (9.6); manual de operação.

### Mudanças de implantação (nginx, sem versão do gateway)

- 2026-10-30 14:45: `limit_conn` do nginx de 12 para **30** por IP (`339517e`).

[0.12.1]: https://github.com/cslsolucoes/mcpollama/releases/tag/gateway-v0.12.1
[0.12.0]: https://github.com/cslsolucoes/mcpollama/releases/tag/gateway-v0.12.0
[0.11.1]: https://github.com/cslsolucoes/mcpollama/releases/tag/gateway-v0.11.1
[0.8.1]: https://github.com/cslsolucoes/mcpollama/releases/tag/mcpollama-v0.8.1
