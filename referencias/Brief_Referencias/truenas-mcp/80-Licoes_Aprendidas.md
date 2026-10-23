# truenas-mcp — Lições aprendidas

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B2 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

Fontes lidas: `MCPNas/truenas-mcp/.wolf/cerebrum.md` (85 linhas), `.wolf/buglog.json` (21 bugs), `.wolf/STATUS.md`, trechos de `.wolf/memory.md`, `.vscode/MCP-VSCODE.md`, os 6 reports permitidos (só os de infra/TLS foram lidos em detalhe) e as regras/planos de governança (sem lições técnicas). Os dois reports de referência de tools/comandos de infra e a regra de pacotes Delphi não geram lição. Cada lição: armadilha → causa raiz → solução → vale para o mcpollama?

Formato: **LIC-TNS-nnn** · título · fonte.

---

## Protocolo MCP, stdio e handshake

**LIC-TNS-001 · Linha de debug sem limite trava o servidor (bug-016)** · `.wolf/buglog.json` bug-016, `.wolf/cerebrum.md:32`
- Armadilha: no Cursor, com `-debug`, `tools/list` ficou preso e depois o cliente mostrou erro e 52 de 622 tools.
- Causa raiz: cada mensagem era logada como **uma linha** `log.Printf` sem limite (~716 KB no `tools/list`); o leitor do stderr (buffer de 64 KiB, reproduzido com `bufio.Scanner`) para de drenar, o pipe enche e o `Printf` bloqueia **o loop stdio inteiro**.
- Solução: `TruncateForLog` com teto de 4.000 bytes nos 4 pontos de log, depois da redação (`truenas/client.go:581-592`). Reproduzido ao vivo com o binário real e um leitor limitado.
- mcpollama: **sim**. Prompts e respostas do Ollama passam fácil de 4 KB; vale para `--debug` e para qualquer log de trace.

**LIC-TNS-002 · Handshake bloqueado por autenticação no backend (bug-021)** · `.wolf/buglog.json` bug-021, `.wolf/cerebrum.md:52`
- Armadilha: a extensão Claude Code do VS Code abriu duas conexões com 640 ms de intervalo e as duas falharam (`connected=false`), com o backend saudável.
- Causa raiz: `main()` chamava `Authenticate()` (WebSocket + TLS + login, ~950 ms) antes de começar a responder `initialize`; handshake medido em 1,374 s.
- Solução: remover a chamada; o `Call` autentica na primeira tool real (`main.go:89-97`). Custo aceito: backend inacessível só aparece na primeira chamada.
- mcpollama: **sim**. Não consultar `/api/tags` no arranque; o erro de rede aparece por chamada. Um `list_models` lento não pode atrasar `initialize`.

**LIC-TNS-003 · O loop stdio sequencial amplifica qualquer bloqueio** · `.wolf/cerebrum.md:32` ("single-threaded stdio loop"), `main.go:135-173`
- Armadilha: um único `Printf` bloqueado (LIC-001) ou uma chamada de 120 s (`client.go:815`) congela o servidor inteiro, inclusive `ping`.
- Causa raiz: `Run` processa uma requisição por vez; a concorrência do cliente WebSocket (`client.go:48-53`) nunca é usada.
- Solução no projeto: nenhuma além do teto de linha. Não houve correção estrutural.
- mcpollama: **sim**. Despachar cada `tools/call` em goroutine (resposta com `stdoutMutex`, padrão que já existe em `main.go:286-301`), tratar `ping` e `notifications/cancelled`, e serializar a GPU numa fila própria.

**LIC-TNS-004 · Parser stdio com limite padrão de 64 KiB** · achado da análise do código (`main.go:122-131`; `grep` não achou `.Buffer(`), reforçado por `client.go:570`
- Armadilha: `bufio.NewScanner(os.Stdin)` sem `Buffer()` aborta com `token too long` se uma requisição passar de 64 KiB; o servidor encerra (`main.go:168-170`, `log.Fatalf` em `:115`). Não está no `buglog.json` (**inferência**: não foi reproduzida com requisição grande; a leitura do código e a constante do `bufio` bastam).
- Solução: `scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)` ou `json.Decoder`; teste com requisição de vários MB.
- mcpollama: **sim**. Chamadas `chat`/`generate` com texto grande no argumento são o caso de uso.

**LIC-TNS-005 · Nome de tool + identificador do servidor ≤ 60 caracteres (bug-017)** · `.wolf/buglog.json` bug-017, `.wolf/cerebrum.md:36`
- Armadilha: o Cursor mostrou "Some tools have naming issues and may be filtered out" para 3 tools; mais 2 falhariam com outro identificador.
- Causa raiz: limite do cliente (`len(servidor)+len(tool) ≤ 60`) que a convenção "método com `_`" nunca considerou.
- Solução: renomear 5 tools (maior nome atual: 44 caracteres) e manter ≤ ~45; sugestão registrada de teste de CI que não foi implementada.
- mcpollama: **sim (parcial)**. Nomes curtos (`list_models`, `generate`, `summarize_file`) cabem, mas o identificador `mcpollama` (9) entra na conta; adicionar um teste que falhe se `len(nome) > 45`.

**LIC-TNS-006 · Cache de tools do cliente e processo antigo em memória** · `.wolf/memory.md:88`, `.wolf/buglog.json` bug-021 (campo `error_message`), `.vscode/MCP-VSCODE.md:33-34`
- Armadilha: o Cursor mostrava 52 tools após rebuild porque mantinha o processo antigo; o painel do VS Code mostrava "671 tools" com o binário já em 730 (era checkpoint histórico, não bug).
- Causa raiz: cliente reaproveita processo/cache; o `.exe` em uso também impede o rebuild (`STATUS.md:153`).
- Solução: reiniciar o servidor/cliente após rebuild; "MCP: Reset Cached Tools" no VS Code; investigar a divergência contando o registry real antes de supor bug.
- mcpollama: **sim**. Documentar no README do mcpollama, e contar tools com o próprio binário (`tools/list`).

**LIC-TNS-007 · Formatos de configuração por cliente e doc enganoso** · `.vscode/MCP-VSCODE.md:7,36-45`
- Armadilha: Cursor `mcpServers` sem `type`; VS Code `servers` com `"type":"stdio"` e MCP só a partir do VS Code 1.99. O mesmo doc afirma "stdio + Content-Length" (`:45`), mas o código usa uma mensagem JSON por linha (`main.go:140-141`, `:299`).
- Causa raiz: documento herdado do template Delphi; nunca revisado contra o código.
- Solução: nenhuma; validar sempre contra o binário (medido nesta onda).
- mcpollama: **sim (parcial)**. Gerar os dois formatos de exemplo; não herdar a afirmação sobre `Content-Length`.

**LIC-TNS-008 · Sem negociação de versão, `ping` nem cancelamento** · medido nesta onda (`protocolVersion` fixo `2024-11-05`; `ping` → `-32601`; `main.go:175-195`)
- Armadilha: cliente que envie `ping` recebe erro; cliente que peça versão mais nova recebe `2024-11-05` sem negociação.
- Causa raiz: 4 métodos implementados à mão, sem SDK.
- Solução: nenhuma no projeto (não gerou bug relatado).
- mcpollama: **sim**. Implementar `ping`, eco/negociação de `protocolVersion` (o `ollama-mcp-go` tem negociação, ver o brief dele) e `notifications/cancelled` ligado ao `context` da geração.

## Tarefas assíncronas e timeouts

**LIC-TNS-009 · `Task.Result` nunca chega ao cliente (bug-015)** · `.wolf/buglog.json` bug-015, `.wolf/STATUS.md` ("Dívida técnica")
- Armadilha: `tasks_get` não devolve o resultado de nenhum job.
- Causa raiz: `Result` com `json:"-"` (`tasks/types.go:42`) e `handleTasksGet` serializa o struct direto; nunca importou porque os jobs devolviam `null`.
- Solução: contornada localmente em `audit_export` com espera limitada; **raiz não corrigida**.
- mcpollama: **sim**. Numa geração assíncrona o resultado é o produto; expor `result` (texto, `prompt_eval_count`, `eval_count`) em `tasks_get`.

**LIC-TNS-010 · Utilitário de espera é, ele mesmo, um job (bug-020)** · `.wolf/buglog.json` bug-020, `.wolf/cerebrum.md:48`
- Armadilha: `core.job_wait` devolve o id de um **novo** job; dois handlers trataram esse inteiro como resultado e `audit_export` devolvia o número como "caminho de arquivo" com `export_completed:true`. Ficou ~20 fases sem ser notado.
- Causa raiz: suposição de que "wait" é bloqueante; um `int` desserializa sem erro em `interface{}`.
- Solução: consultar `core.get_jobs` pelo id do wrapper até estado terminal (`tools/core.go:552-596`).
- mcpollama: **parcial**. O Ollama não tem jobs, mas a regra vale: verificar o formato real da resposta em vez de supor, e não deixar `interface{}` engolir um tipo errado (usar structs tipados para `/api/*`).

**LIC-TNS-011 · Espera limitada abaixo do timeout do cliente + fallback** · `tools/core.go:510-516`, `tools/audit.go:810-830`
- Armadilha: chamadas de ~minutos estouram o timeout do cliente MCP.
- Solução: aguardar até 100 s (abaixo do limite de 120 s do cliente de backend), e se não terminou devolver `task_id` e instrução para `tasks_get`.
- mcpollama: **sim**. É exatamente o desenho para `generate`/`summarize_file`; escolher o orçamento conforme o timeout do cliente MCP real (não medido nesta onda).

**LIC-TNS-012 · Task store: Cancel sem chamador e erros engolidos** · achado da análise (`tasks/manager.go:123`, `tasks/poller.go:67-70`)
- Armadilha: sem como cancelar; erro de rede no poll deixa a task `working` até o TTL; ponteiro `*Task` compartilhado entre poller e leitor (corrida **inferida**, não testada com `-race`).
- Solução: nenhuma no projeto.
- mcpollama: **sim**. Ao copiar o store: `context.CancelFunc` por task, copiar o `Task` em `Get`, rodar `go test -race`.

## TLS, segredos e logs

**LIC-TNS-013 · CA própria em vez de `--insecure`, com teste de que não afrouxa a verificação** · `truenas/tls.go:18-34`, `truenas/tls_test.go:43-117`, `main.go:71-80`
- Armadilha: servidor com certificado que o SO não confia leva muita gente a `InsecureSkipVerify`.
- Solução: `--tls-ca` (PEM) e `--insecure` explícito com aviso; erro de `x509` acrescido de dica (`client.go:145-147`); 4 testes cobrindo rejeitar/aceitar/pin/pin-não-desliga.
- mcpollama: **sim**. É o caso do Origin CA do Cloudflare. **Ressalva** (achado da análise, não do projeto): `RootCAs: pool` substitui o pool do SO; usar `SystemCertPool()` + CA para funcionar também pela internet (certificado público na borda).

**LIC-TNS-014 · Máscara de segredo por nome de campo vive atrasada (bug-004/008/009/013/014/019)** · `.wolf/buglog.json`, `client.go:346-511`
- Armadilha: seis vezes um campo secreto não casava com nenhum fragmento (`key`, `salt`, `hash`, `community`, `pass`, `monpwd`) e iria para o log de debug.
- Causa raiz: lista negra por substring; cada namespace novo traz nomes novos; `pass` (4 letras) nunca poderia casar com fragmentos maiores; `url` de webhook com token foi deixada de fora de propósito (bug-013).
- Solução: acrescentar fragmentos medindo o raio de impacto por `grep` no repo; teste de regressão por caso.
- mcpollama: **parcial**. Só há um segredo (Bearer): preferir remover o **valor exato** (`scrubSecrets`) e não logar corpos por padrão, em vez de manter lista de campos.

**LIC-TNS-015 · Segredo escalar na resposta escapa da máscara por chave (bug-007, bug-018)** · `.wolf/buglog.json`, `client.go:594-661`
- Armadilha: `auth.generate_token` e `tn_connect.generate_claim_token` devolvem o segredo como `result` escalar; sem chave aninhada, ia para o log.
- Solução: mapa de métodos que mascaram o `result` inteiro (`pendingMethods` + `redactResponseForLog`); não alargar o prefixo.
- mcpollama: **parcial**. O equivalente é o conteúdo lido de arquivos e o texto gerado: não logar corpos; se logar, só tamanho.

**LIC-TNS-016 · O servidor pode ecoar a chave no erro** · `client.go:665-670,857-872`, `client_test.go:772-802`
- Armadilha: mensagem e *trace* de erro do backend podem conter a chave enviada.
- Solução: `strings.ReplaceAll(s, apiKey, "[REDACTED]")` em tudo que vem do servidor; teste com servidor que ecoa.
- mcpollama: **sim**. O nginx/Ollama podem devolver corpo de erro; aplicar o mesmo filtro ao texto de erro devolvido ao cliente MCP.

**LIC-TNS-017 · Não passar a chave como argumento literal; usar ambiente** · `.wolf/cerebrum.md:30,34`
- Armadilha: o ambiente bloqueia subagentes que passam a chave em linha de comando; a linha também fica visível a outros processos.
- Solução: variável de ambiente dentro de um harness descartável; o dono autorizou explicitamente quando necessário (`memory.md:90`).
- mcpollama: **sim**. Passar o token pela env `OLLAMA_API_KEY` (não em `args`) e nunca imprimir a chave (alinha com o CLAUDE.md do projeto). `--api-key-file` foi descartado pelo owner (P-1 = não, 2026-10-23 15:50).

**LIC-TNS-018 · Validação ao vivo de consultas com credenciais vaza para o transcript (bug-010)** · `.wolf/buglog.json` bug-010, `.wolf/cerebrum.md:67`
- Armadilha: `user.query` sem filtro trouxe hashes reais para a conversa e para um arquivo de resultado em disco.
- Solução: filtrar campos (`options.select`), apagar o arquivo, nunca citar o valor.
- mcpollama: **parcial**. As tools que leem arquivos (`summarize_file`, `extract`) podem expor segredos do disco; testar com arquivos fictícios e limitar a allowlist.

**LIC-TNS-019 · Prévia (`dry_run`) também vaza (bug-001/002)** · `.wolf/buglog.json`
- Armadilha: a prévia de `delete`/`create` imprimia senha de um dispositivo em `CurrentState`.
- Solução: passar toda superfície auxiliar da prévia pela mesma função de redação.
- mcpollama: **não** (sem escrita); a ideia "toda saída secundária passa pelo mesmo filtro" vale para `--debug`.

## Testes e build

**LIC-TNS-020 · O protocolo não tem teste; os bugs de cliente vieram de uso real** · `.wolf/STATUS.md` ("Testes: só funções puras"), medido: `cmd/`, `mcp/`, `tasks/` sem testes
- Armadilha: bug-016 e bug-021 só apareceram com Cursor/VS Code reais.
- Solução do projeto: harness descartável em Go que **sobe o binário real** e faz `initialize` → `tools/list`, variando uma variável por vez (`.wolf/cerebrum.md:34`).
- mcpollama: **sim**. Transformar esse harness em teste permanente (processo filho, `initialize`, `tools/list`, leitor de stderr com buffer pequeno, medir tempo do handshake).

**LIC-TNS-021 · Fim de linha CRLF e a ferramenta de escrita (bug-003/006/011/012)** · `.wolf/cerebrum.md:59-65`
- Armadilha: arquivos novos saíram em LF 4-5 vezes; `gofmt -w` reescreve tudo; `git diff` com `autocrlf` mascara; relatório do agente afirmava CRLF correto e estava errado (3 vezes); `grep $'\r$'` do Git Bash dá falso negativo.
- Solução: checar bytes (`od`, Python), `gofmt -l` em cópia sem `\r`.
- mcpollama: **parcial**. Decidir `.gitattributes` (`eol=lf`) no scaffold elimina a classe inteira.

**LIC-TNS-022 · BOM em arquivo `.go` quebra `go test -cover`** · medido nesta onda (`tools/kmip.go:1`, `EF BB BF`)
- Armadilha: `go build`/`go test` passam, mas `go test -cover ./tools` falha com `invalid BOM in the middle of the file`. O `STATUS` não registra isso (**o projeto nunca mediu a cobertura de `tools/`**, inferência).
- Solução: remover o BOM; gate com `-cover`.
- mcpollama: **sim (parcial)**. Verificar BOM nos arquivos gerados por editor/ferramenta.

**LIC-TNS-023 · Teste de log: capturar a saída e afirmar a ausência do segredo** · `truenas/client_test.go:700-770`
- Armadilha: redação "funciona" no unitário mas o caminho de log real vaza.
- Solução: servidor falso TLS+WebSocket; rodar uma sessão; ler o buffer do `log`.
- mcpollama: **sim**. Mesmo teste com `httptest.NewTLSServer` e Bearer sentinela.

**LIC-TNS-024 · Não confiar na auto-verificação de agentes** · `.wolf/cerebrum.md:61-63,81`, bug-006/011
- Armadilha: relatório detalhado de "CRLF 831/831" estava errado; comentário de arquivo dizia "validado ao vivo" sem ter rodado (sessão cortada por limite).
- Solução: o orquestrador repete build/vet/test/bytes por conta própria; verificar a evidência primária do que o agente afirma.
- mcpollama: **sim**. Já é regra global do owner; vale para as ondas do brief.

**LIC-TNS-025 · Retomar fase após queda de agente: checar o estado antes de repetir** · `.wolf/cerebrum.md:77`, `STATUS.md` ("Padrão dos 2 resets")
- Armadilha: um 2º agente lançado enquanto o 1º ainda rodava escreveu nos mesmos arquivos; `git status` primeiro.
- Solução: confirmar término real do agente anterior; worktree só para retry sobreposto.
- mcpollama: **parcial**. Relevante para a orquestração das ondas, não para o código.

**LIC-TNS-026 · Servidor de teste fora do ar: sondar vários protocolos antes de declarar queda** · `.wolf/cerebrum.md:42`
- Armadilha: validação ao vivo interrompida no meio; risco de afirmar "validado" sem ter validado.
- Solução: TCP, ICMP e SSH antes de concluir; registrar a pendência honestamente.
- mcpollama: **parcial**. O Ollama tem GPU em fila: separar "servidor fora" de "fila ocupada" (status/tempo de espera no erro).

## Itens do projeto de origem sem aplicação

**LIC-TNS-027 · Parâmetros posicionais e formatos por método (`*.query` com 1 ou 2 argumentos), doc local vs `core.get_methods` divergentes, métodos "job" que devolvem `null`** · `.wolf/cerebrum.md:40,46`, `tools/boot.go:81-127`
- Causa: particularidades da API JSON-RPC do TrueNAS.
- mcpollama: **não**. O genérico "ler o contrato real de cada endpoint (`/api/chat` x `/api/generate` x `/api/embed`), não supor pelo nome" já está em LIC-010.

**LIC-TNS-028 · Certificado autoassinado com `Key Usage` incompatível (`ERR_SSL_KEY_USAGE_INCOMPATIBLE`)** · `.wolf/STATUS.md` (seção da sessão de infra)
- Causa: certificado de teste de um host de virtualização sem `digitalSignature`; corrigido regenerando com `keyUsage = critical, digitalSignature, keyEncipherment`.
- mcpollama: **não**. O certificado é o Origin CA do Cloudflare, e o cliente Go não exige esse uso. Fica como lembrança ao gerar certificados de teste.

**LIC-TNS-029 · Colisão de nome de arquivo ao criar família nova (`nfs.go`, `smb.go`)** · `.wolf/cerebrum.md:24`
- Causa: convenção "nome do arquivo = nome curto do serviço" já ocupada por `sharing.*`; quase sobrescreveu código commitado.
- mcpollama: **não** (poucos arquivos). Ler antes de criar.

**LIC-TNS-030 · Plano com arquivo listado no commit mas nunca versionado** · `.wolf/cerebrum.md:71`
- Causa: mensagem de commit não prova o `git add`; conferir com `git ls-files`/`git log -- caminho`.
- mcpollama: **parcial**. O projeto ainda não tem commits (`git status` do CLAUDE.md); vale para os planos de `.workspace/`.

---

**Total: 30 lições `LIC-TNS-001` a `LIC-TNS-030`.** Marcadas como **achado da análise** (não registradas no projeto de origem): 004, 008, 012, 013 (ressalva do pool de CAs) e 022.
