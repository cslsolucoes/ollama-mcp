# Changelog

Histórico de versões do **mcpollama** (servidor MCP, na estação) e do **mcpollama-gateway** (fila + `/monitor`, no servidor
de IA). O formato segue o [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/); as versões não têm release formal no
GitHub, só **tags anotadas** nas versões marcadas abaixo (`mcpollama-v<versão>` e `gateway-v<versão>`).

Fontes: commits, tags, e os planos e reports de `.workspace/`. As versões vêm do `--version` do binário
(`<versão>-<hash curto do commit>`, definido na compilação por `-X main.Version`). O que não deu para confirmar está marcado
**(data/versão a conferir)**.

## Não lançado

A Fase 11 (jobs assíncronos, lotes e registro integral) foi **descongelada em 2026-11-05 20:22** por ordem do owner (estava congelada
desde 2026-10-30 23:50); o plano foi refeito e a aprovação 11.0 está pendente (só o spike 11.1 foi feito; nenhum código):
`.workspace/plans/MCPOllam2026-10-30-224500-fase11-websocket-jobs-assincronos.plan.md`.

**Fase 12** (registro dos erros do modelo local → base de treino): **implantada em 2026-11-04** (gateway 0.13.0 às 13:02, depois
0.13.1; mcpollama 0.10.0 em `D:\MCP`); aceite ao vivo pendente. Plano:
`.workspace/plans/MCPOllam2026-11-01-213500-fase12-erros-treino.plan.md`; contrato de dados: `docs/CONTRATO-DADOS-TREINO.md`;
instruções para o Claude em `docs/RELATAR-ERROS-IA.md` (em vigor desde 2026-11-04).

**Release de ajuste da Fase 12, IMPLANTADA em 2026-11-05 12:02–12:06:** **gateway 0.13.2** (falso alarme "fora da GPU") e
**mcpollama 0.10.1** (modelo de produção `qwen3.8:27b` com o nome real, sem alias). A troca do modelo depende do aceite do
`qwen3.8:27b` em curso (S1/S4/S5, `think` em tarefa grande, carga contínua) e da liberação do owner. Ordem prevista: no servidor,
baixar o `qwen3.8:27b`, ajustar o `ollama-warm` e o `--allowed-models` do gateway e só depois apagar o `qwen3-ia`; em seguida,
mcpollama 0.10.1 em `D:\MCP` e reinício das sessões.


## mcpollama

### [0.10.2] - 2026-11-05 12:58, em `D:\MCP` (`0.10.2-09e2936`; ajuste da Fase 12; ainda sem tag)

#### Alterado
- **`--max-prompt-chars` padrão de 128000 para 270000** (~90k tokens; owner, 2026-11-05 12:23: "ok, implemente"), junto com o
  contexto do servidor de 64k para **96k** (`OLLAMA_CONTEXT_LENGTH=98304` e `ollama-warm` com `num_ctx 98304`, aplicados às 12:25).
  96k é o maior contexto com o `qwen3.8:27b` 100% na GPU (66/66 camadas, ~21,3 GB) junto do nomic: 104k já deixa 1 camada na CPU,
  128k cai para 15 tok/s e 256k não cabe (41/66, 4,8 tok/s). Medido pelo caminho real: prompt de 87k tokens em ~227 s; PDF de 110k
  tokens em 2 partes (~6 min) e de 268k tokens em 5 partes (~15 min). Cada parte grande ocupa a fila (1 por vez) por até ~4 min.

### [0.10.1] - 2026-11-05 12:06, em `D:\MCP` (`0.10.1-bb51ed6`; ajuste da Fase 12; ainda sem tag)

#### Alterado
- **Modelo de produção com o nome real** (decisão do owner, 2026-11-05, opção "b"): o padrão de `--model`/`OLLAMA_MODEL` e a lista
  padrão de `--allowed-models`/`MCPOLLAMA_ALLOWED_MODELS` passam a ser `qwen3.8:27b` (antes `qwen3-ia`, alias do `qwen3.5:9b`).
  O nome virtual de raciocínio passa a ser `qwen3.8:27b-think`. Motivo: o owner quer ver o modelo que roda, e as amostras de treino da
  Fase 12 ficam separadas pelo campo `modelo` (o `treino.db` tinha 0 ocorrências na data da decisão). **Incompatível com o servidor
  antigo:** só funciona depois que o servidor tiver o `qwen3.8:27b` e o gateway aceitar o nome (`deploy/gateway/mcpollama-gateway.service`).
- **`--timeout` padrão de 120 s para 30 min** (owner, 2026-11-05): igual ao `--max-wait 30m` do gateway (aplicado às 11:40). No aceite, o
  `qwen3.8:27b` leu um prompt de 40k em ~90 s e, com 10 agentes e `think`, 27 de 60 chamadas estouraram 300 s esperando na fila.
- **Bake-off** (`src/cmd/bakeoff`, ferramenta de teste): padrão de `-atual` = `qwen3.8:27b`; nova opção **`-think`**, que envia
  `think:true` nas chamadas de `generate`, `chat`, `summarize_file`, `extract` e `translate` (teste `TestWithThink`); com `-think`, o S6
  reserva 4.096 tokens (com 50, o raciocínio esgotava o limite e a resposta vinha vazia).

#### Corrigido
- **Marcadores de arquivo repetidos na resposta** (aceite 2026-11-05): o `qwen3.8:27b` repetiu `=====ARQUIVO_INICIO <id>=====` e
  `=====ARQUIVO_FIM <id>=====` em 1 de 3 traduções. As tools de arquivo removem da resposta as linhas que só repetem os marcadores
  **daquele pedido** (o id é aleatório por pedido e ausente do documento); qualquer outro texto fica. Testes `TestStripEchoedMarkers` e
  `TestTranslateDropsEchoedMarkers` (este falha sem a correção).

### [0.10.0] - 2026-11-04, em `D:\MCP` (Fase 12; ainda sem tag)

Binário `0.10.0-5372f4c` instalado em `D:\MCP` em 2026-11-04 (backup da anterior em `mcpollama-mcp.exe.bak-0.9.2`). **Só instale depois do gateway 0.13.0** (ver abaixo): o gateway
0.12.6, em produção, repassa o cabeçalho novo `X-Mcpollama-Doc` ao Ollama (que o ignora), mas a remoção dele e a partição por
documento só passam a valer com o 0.13.0 (C-2.11). Commits da Fase 12 nesta parte: `1bb42fc` (12.2) e `9f67fee` (12.5), mais o contrato.

#### Adicionado
- **ID e papel de cada job** (12.2): o mcpollama lê o `X-Request-Id` (32 hex minúsculos) da resposta usada, inclusive a do
  fallback, e a linha `[uso]` ganha `job=<id>` (uma chamada) ou `jobs=<papel>:<id>,...` (várias; papéis `unico`, `mapN`,
  `reduceN`, `final`; `@fallback` quando o job rodou no segundo servidor). O mesmo vai em `jobs` no `_meta` `mcpollama/usage`.
- **Cabeçalho `X-Mcpollama-Doc`** (12.2, C-4.1): nas tools de arquivo, o sha256 do texto extraído do documento inteiro, enviado em
  **cada** chamada do pedido (map, reduce e partes). O gateway só grava o HMAC dele, nunca o sha256 puro.
- **Tool `reportar_erro`** (12.5): o Claude relata um erro do modelo local ao gateway (`POST /api/feedback`), com os jobs
  envolvidos (até 64), a resposta correta e a categoria. Aparece sem configuração nenhuma. Só retenta no 409 (uma vez) e nunca
  cai para o outro servidor. Instruções: `docs/RELATAR-ERROS-IA.md`.
- **Flag `--auto-report` / `MCPOLLAMA_AUTO_REPORT`** (12.5): liga os relatos **automáticos** de candidatos a erro (resposta vazia
  ou cortada pelo limite de tokens; a regra de JSON inválido está reservada e não dispara, porque nenhuma tool expõe o `format`
  do Ollama). **Desligada por padrão** (`false`): cada automático copia pedido e resposta, inclusive de documentos sensíveis, para
  a base de treino do servidor, sem expiração. Quando ligada: fila de 16, 1 goroutine, 30 s por envio, nunca atrasa nem falha uma tool.

#### Alterado
- `TOOLS.md` regenerado (tool nova); o orçamento de tamanho do `tools/list` subiu (4.620 para 6.650 bytes no modo compacto e
  8.330 para 10.360 no completo), porque o esquema da `reportar_erro` tem 3 enums.

### [0.9.2] - 2026-10-31 (tag `mcpollama-v0.9.2`)

Binário `0.9.2-f367b35` em `D:\MCP\mcpollama-mcp.exe` (22:21); vale após reiniciar as sessões do Claude Code.
Provado ponta a ponta em 2026-10-31 22:22: o job gravou `002-1985` / `claiton.linhares` / `CSLSOLUCOES` / `5CCD5BF40B2D` (WMI igual à `PLACA`).

Pedido do owner em 2026-10-31: a estação se identifica **pelo executável**, lendo o ambiente do Windows.
- **Cabeçalho novo `X-Mcpollama-Station`**, enviado em todo pedido junto com o `X-Mcpollama-Client` (que continua igual,
  `máquina/usuário`). Formato `computer=<v>;user=<v>;domain=<v>;mac=<v>`, campos vazios omitidos; cada valor só com
  `[A-Za-z0-9._-]` (o resto vira `_`), até 64 caracteres.
- **Origem dos valores:** `computer` = `COMPUTERNAME` (ou `os.Hostname()`); `user` = usuário do processo, sem o prefixo
  `DOMÍNIO\`; `domain` = `USERDOMAIN` (ou o prefixo `DOMÍNIO\` do usuário); `mac` por esta precedência:
  1. **WMI**, em Go puro (`syscall` sobre as vtables do COM, sem cgo e **sem dependências novas**; só `golang.org/x/sys`, que
     já estava no `go.mod`): `SELECT MACAddress FROM Win32_NetworkAdapter WHERE PhysicalAdapter=TRUE`, o primeiro com
     `MACAddress` não nulo, na ordem do WMI, com limite de 3 s e uma vez só na partida. É a mesma regra do script de logon do AD;
  2. a variável **`PLACA`** (12 hex, aceita `:` e `-`; inválida é ignorada);
  3. o MAC da primeira interface ativa, não loopback, com 6 bytes e IPv4 privado.
- **O script de logon** (`LoginScriptV4.00.vbs`, v4.10) é quem grava as variáveis nas máquinas do domínio: `PLACA` é o MAC do
  **primeiro** adaptador de `Win32_NetworkAdapter where PhysicalAdapter=True` com `MACAddress` não nulo, sem os `:`, no ambiente
  do USUÁRIO (`HKCU\Environment`); `USERDOMAIN`, `USERNAME` e `COMPUTERNAME` têm os mesmos valores de
  `WScript.Network.UserDomain/UserName/ComputerName`. **Diferença do fallback:** sem WMI e sem `PLACA` (máquina fora do domínio ou
  sem o script), o exe usa o MAC da **interface ativa com IPv4 privado**, que não é necessariamente o "primeiro físico do WMI".
- **Flag `--station-info` / `MCPOLLAMA_STATION_INFO`** (booleana, padrão `true`); `--station-info=false` não envia o cabeçalho.
  Tudo é coletado uma vez, na partida.
- **É informação declarada pela estação, não autenticada:** qualquer cliente pode mandar qualquer valor. O `onda.py`, que chama o
  servidor direto sem o exe, continua anônimo.

### [0.9.1] - 2026-10-30 (tag `mcpollama-v0.9.1`)

Binário `0.9.1-755f168` em `D:\MCP\mcpollama-mcp.exe` (22:20); vale após reiniciar as sessões do Claude Code (feito 23:31).

#### Alterado
- Rótulo padrão do cliente (commit `54ad2d6`): `<máquina>/<usuário>` em vez de `<hostname>/<pid>` (o domínio do Windows,
  `DOMINIO\usuario`, é removido; sem usuário, volta para `<máquina>/<pid>`). Todas as sessões do mesmo usuário passam a dividir
  **um só cliente** na justiça da fila (limite de 64 por cliente desde o gateway 0.12.3). `--client-label` continua sobrescrevendo.
  Provado ao vivo: pedido do 0.9.1 aparece no `/monitor` como `002-1985/claiton.linhares`.

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

### [0.13.3] - 2026-11-05 (ainda sem tag; monitor)

#### Corrigido
- **Monitor: "Em execução" e "Fila de espera" com altura fixa e paginador sempre visível** (owner, 2026-11-05: "não está aparecendo aqui ainda").
  A 0.13.1 entendeu o pedido "fixo 5 linhas" e "10 linhas fixas" como "mostrar **até** 5 e 10 linhas": o paginador só aparecia quando a lista
  passava do limite e, vazio, cada quadro encolhia para uma linha de texto. Agora cada quadro **sempre** ocupa 5 linhas (Execução) ou 10
  (Fila), com linhas em branco quando há menos itens; o paginador aparece sempre ("página 1 de 1 · 0 itens", botões desabilitados); cada
  quadro tem uma linha de status fixa ("Nada em execução (slots: …)" / "N em execução (slots: …)"; "Ninguém esperando." / "N esperando."),
  para a tela não pular. Só muda `src/gateway/monitor/index.html` (elementos `#runst` e `#waitst`, `padRows`, `boardStatus`); o teste
  `TestIndexIsResponsiveAndStable` passou a exigir as linhas em branco e o paginador sempre desenhado.
  **Verificado em navegador real** (Playwright, gateway local com um Ollama de mentira): estado vazio (5 e 10 linhas, "página 1 de 1 · 0 itens");
  carga de 13 pedidos (1 em execução + 4 linhas em branco; fila com 10 linhas, "página 1 de 2 · 12 itens"); página 2 pelo botão "próxima ›"
  ("página 2 de 2 · 11 itens", 1 item + 9 linhas em branco; a página escolhida se mantém entre as atualizações); sem erro de console além do
  `favicon.ico`. Testes de ataque das revisões 9, 10.5 e 12: `.workspace/reports/MCPOllam2026-11-05-ataques-gateway-0.13.3.log`.

### [0.13.2] - 2026-11-05 12:02, em produção (`0.13.2-bb51ed6`; ajuste da Fase 12; ainda sem tag)

#### Corrigido
- **Falso alarme "degradado (fora da GPU)"** (2026-11-04, com o `gemma4:26b`): o `/api/ps` conta em `size` um buffer em memória do
  host (`CUDA_Host`, ~0,45–0,6 GiB) que não entra em `size_vram`, com todas as camadas na GPU ("offloaded 31/31 layers"). Regra nova
  em `src/gateway/models/models.go` (`Processor`): diferença de até **1 GiB e até 25% do tamanho** conta como "100% GPU"; acima disso
  continua o alarme (ex.: o `qwen3-ia` com 60% fora da VRAM na etapa B). Testes: `TestProcessor` (casos do 26b, do offload real, de
  modelo pequeno meio fora e de 1 GiB + 1 byte) e `TestHostBufferIsNotOffTheGPU` (pool: backend continua "ok" e o trabalho fica nele);
  sem a correção, os testes novos falham com "7%/93% CPU/GPU". Testes de ataque das revisões 9, 10.5 e 12: 138 PASS, 0 FAIL
  (`.workspace/reports/MCPOllam2026-11-04-1645-ataques-gateway-0.13.2.log`).

#### Alterado
- `deploy/gateway/mcpollama-gateway.service`: `--allowed-models "qwen3.8:27b;nomic-embed-text"` (modelo de produção com o nome real);
  `deploy/gateway/stage2.sh` testa com `qwen3.8:27b`; `deploy/servidor-ia/ollama-warm.sh` carrega o `qwen3.8:27b` a 64k no boot.
- **`--max-wait 30m`** na unit (owner, 2026-11-05 11:39: "aumente o timeout na fila para 30 minutos"; antes o padrão de 10 min); já
  aplicado no servidor às 11:40 (backup `mcpollama-gateway.service.bak-20261105-114002`). O `--run-timeout` (execução) continua 10 min.

### [0.13.1] - 2026-11-04 (ainda sem tag)

- **Monitor:** o quadro "Em execução" mostra até 5 linhas e o "Fila de espera" até 10, com paginação do restante
  ("‹ anterior · página X de N · T itens · próxima ›"). A página é guardada por quadro e não reinicia com a atualização de 1 s.
  Os demais quadros não mudaram (pedido do owner).

### [0.13.0] - 2026-11-04 13:02, em produção (Fase 12; ainda sem tag)

Binário `0.13.0-<hash>` (Linux amd64) ainda **não** instalado no servidor. Commits da Fase 12: `8fc5558` (contrato 1.0.0), `1fd5b27`,
`3531beb` (12.3), `a1a4f9f`, `a820b70` (12.4), `37706bb`, `d27a4ca`, `0493f7f` (12.6), `3fc8fa9` (12.7), `9e3c111` (12.8) e `481aee0`
(correções da revisão adversarial 12.9 e contrato 1.1.6); `c11aff9` é só o pack do Codex no git, sem código.

**Ordem de implantação obrigatória: gateway 0.13.0 ANTES do mcpollama 0.10.0** (C-2.11). O 0.12.6 repassa o `X-Mcpollama-Doc` ao
Ollama; o 0.13.0 o remove e grava só o HMAC. Gateway novo com mcpollama antigo não quebra nada (o `/api/feedback` só não é
chamado); o inverso é o que se evita.

#### Adicionado
- **Pedido canônico** (12.3): o `content.db` guarda também o `prompt_json` de cada job (o pedido, com cada imagem trocada por
  `{"imagem_hmac":"<hex>"}`), com tetos próprios e marcas de truncamento; funções `prompt_hmac` e `conteudo_hmac` (C-2.12).
- **Chave HMAC** `--doc-key-file` / `MCPGW_DOC_KEY_FILE` (12.3): 64 hex, modo 0600. Com ela, `requests.doc_hash` guarda o HMAC do
  `X-Mcpollama-Doc` com o `key_id` (16 hex do sha256 da chave). Sem ela, o hash fica vazio e o `/api/feedback` responde 503.
- **`treino.db` e `POST /api/feedback`** (12.4): `--treino-db` / `MCPGW_TREINO_DB` (absoluto, 0600, teto de **2 GiB** de dados vivos;
  acima, **HTTP 507** sem apagar nada). Ocorrências, jobs, amostras, histórico (só hash e tamanho nos campos de conteúdo),
  retratos e embeddings. **Esquema v4** (`PRAGMA user_version`); o gateway recusa abrir um arquivo de versão maior.
  `--role primario|fallback` / `MCPGW_ROLE`: num gateway de fallback, todo relato vira estado `fallback`, sem copiar amostra.
- **Aba "Erros (treino)" no `/monitor`** (12.6): lista e filtros; detalhe só para o admin; ações do admin (confirmar, descartar,
  completar, apagar, marcar exemplo público, liberar quarentena e "Marcar erro" num job do log); **exportação JSONL** em streaming
  (`parte=treino|teste`, regra única do capítulo 3 do contrato, `exportou` por ocorrência no histórico); partição treino/teste por
  grupo com cota e quarentena. Rotas `GET /monitor/api/treino[/export|/{id}]` e `POST /monitor/api/treino/{id}/{ação}`.
- **Bakeoff `-suites erros`** (12.7): retrato congelado do teste, linha de base do `qwen3-ia`, comparação candidato x atual em 3
  rodadas pareadas, revisão às cegas e apuração (`-erros-modo retrato|base|comparar|apurar|indice|indices-limpar`). Roda **no
  servidor**, com o mesmo usuário do gateway (o conteúdo dos casos não sai da rede). Binário próprio (`cmd/bakeoff`, Linux).
- **Melhora sem treino, DESLIGADA por padrão** (12.8, D12-15): `--melhora-tools`, `--melhora-instrucoes`, `--melhora-indice`,
  `--melhora-embed-model`, `--melhora-embed-timeout` e `--melhora-embeddings` (env `MCPGW_MELHORA_*`). Só liga depois que o bakeoff
  mostrar ganho (C-5.10 a C-5.12). Com ela desligada, o caminho quente fica idêntico ao do 0.12.6.

#### Alterado (padrões novos)
- `--content-max-bytes`: padrão de **2 GiB para 4 GiB**; o `prompt_json` conta nesse teto.
- `--content-max-job-bytes` (16 MiB) vale para cada um: prompt plano, `prompt_json` e resposta.
- `--melhora-*` vem **desligado**; no mcpollama 0.10.0, `--auto-report` vem **desligado** (`false`).
- Remoção do `X-Mcpollama-Doc` antes do Ollama (C-2.11).
- Migração do `content.db` no start, por `ALTER TABLE ... ADD COLUMN` (`prompt_json`, `prompt_json_truncated`, `tem_imagem`,
  `prompt_json_original*`, `melhora_*`), e do `gateway.db` (`requests.doc_hash`). **O 0.12.6 lê esse `content.db`** (ignora as colunas novas).

#### Implantação
- A unit versionada (`deploy/gateway/mcpollama-gateway.service`) configura a Fase 12 por **variáveis de ambiente** (`MCPGW_TREINO_DB`,
  `MCPGW_DOC_KEY_FILE`, `MCPGW_ROLE`), **não** por flags no `ExecStart`: o 0.12.6 recusa flag desconhecida (laço de reinício), mas
  ignora variável desconhecida, então o rollback só do binário continua subindo com a mesma unit.
- A etapa 1 (`stage1.sh`) gera `/etc/mcpollama-gateway/doc.key` se não existir (32 bytes aleatórios em hex, `mcpgw:mcpgw` 0600),
  nunca a sobrescreve nem a imprime (só o `key_id`). Novo `stage-atualizar.sh`: o `-Modo Atualizar` agora também troca a unit se
  mudou, reinicia e confere, e volta sozinho (unit e binário) se o gateway não subir; não mexe no nginx nem no Ollama.
- `treino.db`, `content.db` e `doc.key` ficam **fora do backup comum** (C-6.8); a mesma chave vai para o gateway de fallback, por
  canal protegido (C-4.3a).
- Procedimento e rollback: `docs/MANUAL-GATEWAY.md` seção 17 e `docs/INSTALACAO.md` (C.1).

### [0.12.6] - 2026-11-01 (tag `gateway-v0.12.6`)

Binário `0.12.6-a155513` implantado às 21:45, com a fila vazia (5 leituras do `/monitor/api/state` logo depois: 45–80 ms).

Incidente de 2026-11-01 (gateway `0.12.5-4cd3290`): o `/monitor/api/state` parou de responder, enquanto `/api/*` e
`/monitor/api/series` seguiram normais. Os pedidos do painel se acumularam até o `limit_conn 80` do nginx e a estação do
owner passou a receber **429** em tudo (o painel mostrava "sem conexão").

Causa, provada por dump de goroutines (SIGQUIT, 211 goroutines):
- uma goroutine do `apiStream` estava em `refreshExtras` -> `store.Stats` (o segundo `rows.Err()`), presa em
  `database/sql.closingMutex.RLock` havia 112 min, **segurando o `extraMu`** do monitor;
- outra, `database/sql.Rows.awaitDone` -> `Rows.close`, presa no `closingMutex.Lock` pelo mesmo tempo. Ela existe porque
  o `Stats` recebia o contexto do pedido SSE: o cliente fechou e o cancelamento correu contra o `rows.Err()` final
  (impasse no `database/sql`);
- outras 87 esperavam o `extraMu`. Como o refresh segurava o mutex do monitor, o painel inteiro travou.

Correção:
- `store.Stats`: removido o segundo `rows.Err()` (o erro já é checado logo após o laço); retorna `st, nil`.
- `monitor`: o refresh do cache compartilhado (Stats hoje e 15 min, History, Events, e as listas de jobs ao vivo e
  concluídos) usa `context.Background()`, nunca o contexto do visitante, então o `database/sql` nem cria o `awaitDone`.
  O `Stats` do cache de `/monitor/api/stats` também.
- Novo `swrCache` (`monitor/swr.go`): o mutex só protege o valor e as flags, nunca a consulta. Quem chama recebe na hora o
  último valor em cache; vencido o TTL, **um** refresh roda numa goroutine (um refresh preso custa uma goroutine, não uma
  por visitante). A primeira chamada, sem cache, espera o refresh no máximo 2 s e depois devolve vazio. Refresh com erro
  mantém os valores anteriores.
- `readSource` (interface mínima: `Stats`, `History`, `Events`, `Jobs`) para permitir a injeção no teste; em produção é o
  `*store.Store`.

Prova: `TestStateSurvivesHungDatabaseRead` (`monitor/hang_regress_test.go`) injeta um `Stats` que bloqueia para sempre e
dispara 20 goroutines x 5 pedidos a `/monitor/api/state`. No código antigo **falha**
(`state() travado: o mutex do monitor segura a leitura do banco`, 10,03 s); no novo passa em ~0,5 s, todas as respostas
abaixo de 500 ms, 1 só chamada a `Stats` e sem goroutines acumuladas. Os testes de `jobs_test.go` que dependiam de
recálculo imediato do cache agora esperam o refresh (`settleState`).

### [0.12.5] - 2026-10-31 (tag `gateway-v0.12.5`)

Binário `0.12.5-4cd3290` implantado às 22:22, com a fila vazia. Decisão do owner (17:05): os quatro campos ficam **visíveis na LAN**, como o resto do `/monitor`.

- **`proxy`:** lê `X-Mcpollama-Station` com validação estrita (só as quatro chaves, cada uma uma vez; valores
  `^[A-Za-z0-9._-]{1,64}$`; `mac` com exatamente 12 hex maiúsculos; cabeçalho de até 512 bytes). Ausente ou inválido: campos vazios,
  **sem recusar o pedido**. O cabeçalho é **apagado antes de repassar ao backend**.
- **Banco:** quatro colunas novas em `requests` (`station_computer`, `station_user`, `station_domain`, `station_mac`), com
  **migração idempotente** (`ALTER TABLE ADD COLUMN` só se faltar; bancos antigos abrem e leem normalmente). Vão do pedido até o
  registro (fila, `joblog`, `store`); o fim do job sem estação não apaga a que foi gravada na entrada.
- **API e exportações:** os quatro campos entram no JSON de `/monitor/api/jobs/<id>` e da lista, no JSONL e no CSV (colunas
  `maquina`, `usuario`, `dominio`, `mac`, com `csvSafe`).
- **Tela:** a coluna Cliente continua sendo o `client_label`; o **Detalhes** do job ganha as linhas Máquina, Usuário, Domínio e MAC
  (sempre `textContent`).
- **Privacidade:** o **MAC e o usuário ficam no log** (7 dias, como o resto do log de jobs). Como o resto da leitura do `/monitor`,
  os campos aparecem a quem acessa o painel na LAN.

### [0.12.4] - 2026-10-31 (tag `gateway-v0.12.4`)

Binário `0.12.4-bfbc884` implantado às 16:02, com a fila vazia.

- **Botão Detalhes** (em "Concluídos nos últimos 5 min") não fazia nada: o painel do job fica dentro da aba "Log em tempo real" e
  abria escondido. Agora `openJob` abre essa aba antes. Provado no Firefox headless: na 0.12.3 a aba continuava oculta depois do
  clique; na 0.12.4 a aba abre e o detalhe aparece. Teste de regressão em `responsive_test.go`; testes de ataque 9.5/10.L/10.5 ok.

### [0.12.3] - 2026-10-30 (tag `gateway-v0.12.3`)

Binário `0.12.3-55fd624` implantado às 22:35 pelo `Instalar-Gateway.ps1 -Modo Atualizar`.

- Ordem do painel (pedido do owner): **Em execução → Fila de espera → Concluídos recentes** (os concluídos saíram da seção
  "Em execução" para uma seção própria); depois Backends, GPUs etc.
- Botões ▲▼✕ da fila numa linha só dentro da tabela (a 720 px quebravam em coluna e alongavam a linha).
- Limites (unit `deploy/gateway/mcpollama-gateway.service`, decisão do owner 22:28): `--queue-per-client 64`,
  `--queue-per-ip 64`, `--queue-max 128` (antes 16/16/64); nginx `limit_conn 80` e `burst=100` (antes 30/60). Prova: 30 pedidos
  simultâneos de uma estação → 30× HTTP 200 (antes 17× 200 + 13× 503 "limite de pedidos em espera deste cliente").

### [0.12.2] - 2026-10-30 (tag `gateway-v0.12.2`)

Binário `0.12.2-755f168` implantado às 22:20.

- **Responsivo:** a coluna do `<main>` passou a poder encolher (`minmax(0,1fr)`) e as grades de cards, GPUs, CPU, gráficos e do
  detalhe do job quebram em telas estreitas (`minmax(min(<valor>,100%),1fr)`); abaixo de 640 px os espaçamentos diminuem e o
  cabeçalho empilha. Antes, a ~570 e ~720 px as seções ficavam mais largas que a janela e cards, GPUs, Backends e threads eram
  cortados à direita. Só as tabelas, dentro do `.wrap`, rolam na horizontal.
- **Tabelas estáveis:** a cada mensagem (~2/s) o painel reconstruía todas as tabelas, o que zerava a rolagem horizontal e
  trocava os botões (Detalhes, Cancelar, ▲▼✕, Drenar) entre o mousedown e o mouseup (cliques perdidos). Agora uma tabela de
  mesmo formato é atualizada no lugar (só texto, classe e título das células que mudaram), os botões têm chave estável
  (`data-k`) e são mantidos, e uma reconstrução preserva o `scrollLeft`. Cards de GPU/CPU só são redesenhados se o dado mudar.
- **Erros de desenho visíveis:** o `catch` vazio do `onmessage` foi trocado por `console.error` e pela linha "Erro ao desenhar o
  painel: ..." em `#msg` (some quando um desenho seguinte dá certo); o erro não derruba o EventSource.
- Aba "Log em tempo real": um job que acabou de terminar, ainda "em execução" na lista vinda do SQLite, passa a mostrar o status
  final vindo de `recent_done`.

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

[0.12.6]: https://github.com/cslsolucoes/ollama-mcp/releases/tag/gateway-v0.12.6
[0.12.5]: https://github.com/cslsolucoes/ollama-mcp/releases/tag/gateway-v0.12.5
[0.12.4]: https://github.com/cslsolucoes/ollama-mcp/releases/tag/gateway-v0.12.4
[0.12.3]: https://github.com/cslsolucoes/ollama-mcp/releases/tag/gateway-v0.12.3
[0.12.2]: https://github.com/cslsolucoes/ollama-mcp/releases/tag/gateway-v0.12.2
[0.12.1]: https://github.com/cslsolucoes/ollama-mcp/releases/tag/gateway-v0.12.1
[0.12.0]: https://github.com/cslsolucoes/ollama-mcp/releases/tag/gateway-v0.12.0
[0.11.1]: https://github.com/cslsolucoes/ollama-mcp/releases/tag/gateway-v0.11.1
[0.8.1]: https://github.com/cslsolucoes/ollama-mcp/releases/tag/mcpollama-v0.8.1
