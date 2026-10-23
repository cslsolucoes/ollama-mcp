# MCPVMWare — lições aprendidas (`LIC-VMW-nnn`)

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B2 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

Formato de cada lição: **armadilha → causa raiz → solução → vale para o mcpollama?** (sim / não / parcial + porquê). Citações relativas a `referencias/`. Fontes: `MCPVMWare/.wolf/cerebrum.md` (`cer:`), `MCPVMWare/.wolf/buglog.json` (`bug-NNN`), planos e relatórios em `MCPVMWare/.workspace/`, `MCPVMWare/MCP-GAPS-VMWARE.md` e medições **desta onda** (marcadas "medido B2", feitas em cópia fora de `referencias/`). Nenhum valor de credencial ou host de laboratório é reproduzido.

**Ordem:** protocolo MCP/stdio, tokens/`tools/list`, segurança, Windows/build/testes, processo. Não numerei por importância; as 5 principais estão no fim.

## A. Protocolo MCP e stdio

### LIC-VMW-001 — Linha de entrada acima de 64 KB derruba o servidor
- **Armadilha:** `bufio.NewScanner(os.Stdin)` sem `Buffer()` (`MCPVMWare/src/mcpvmware-mcp/main.go:310-316`).
- **Causa raiz:** o `Scanner` tem limite padrão de 64 KB por token; ao estourar, `Scan()` para e `Run` retorna erro → `log.Fatalf` (`:347-349`, `:297-299`).
- **Medido B2:** linha de 30 KB passou; de 70 KB gerou `stdin error: bufio.Scanner: token too long` em stderr e **exit 1**, sem resposta ao cliente.
- **Solução (no mcpollama):** `scanner.Buffer(make([]byte, 0, 64*1024), N)` com N configurável (flag, p.ex. `--max-request-bytes`) ou `bufio.Reader.ReadBytes('\n')`; ao exceder, responder erro JSON-RPC em vez de morrer.
- **Vale?** **Sim, crítico.** Prompts para `generate`/`chat` com contexto de 32k tokens passam de 100 KB. [desatualizado: o servidor medido na B4 tem `num_ctx` 131072, ver `Servidor-Ollama-CSL/dados/validacao-empirica.md` §3; o limite utilizável de prompt está em aberto, §7]

### LIC-VMW-002 — `ping` sem resposta, versão fixa e parse error sem `id`
- **Armadilha:** `ping` devolve `-32601`; `protocolVersion` é sempre `2024-11-05`; erro de parse sai sem `"id":null` (`main.go:353-380`, `mcp/types.go:15-20`).
- **Causa raiz:** `handleRequest` só conhece 4 métodos; a versão é literal; `ID` com `omitempty` some quando nulo.
- **Medido B2:** os três comportamentos reproduzidos por stdio (doc 10 §3).
- **Solução:** responder `ping` com `{}`; negociar a versão (devolver a do cliente se suportada, senão a mais recente conhecida); emitir `"id":null` em erro de parse.
- **Vale?** **Sim.** O Claude Code e o Cursor ficam mais robustos; o `ollama-mcp-go` já negocia versão (brief dele).

### LIC-VMW-003 — Backend inalcançável no boot = "Connection closed"
- **Armadilha:** o cliente MCP mostra `CONNECTION_CLOSED` quando o servidor não consegue logar no backend.
- **Causa raiz:** o login é feito antes do loop stdio e a falha chama `log.Fatalf` (`main.go:258-268`); o processo some antes do `initialize`. O dono registrou 2 servidores do mesmo conjunto caindo assim enquanto o 3º conectava (`MCP-GAPS-VMWARE.md:61-63`; "workaround: reiniciar a sessão", que não resolve a causa).
- **Solução:** subir sempre; conectar de forma lazy ou na primeira chamada; erro por tool com texto acionável.
- **Vale?** **Sim, forte.** O Ollama está atrás de LAN/Cloudflare e a rede varia (porta 8443 vs sem porta).

### LIC-VMW-004 — "Confia no timeout do chamador" era mentira de comentário
- **Armadilha:** tools de espera (`wait_for_ip` etc.) sem prazo; teste preso 10 minutos (bug-006).
- **Causa raiz:** um comentário afirmava "relies on the caller's MCP request timeout"; `grep` mostrou que nada no loop stdio define prazo (`main.go:184`, `context.Background()`).
- **Solução:** `timeout_seconds` com default (5 min) em toda tool de espera + `context.WithTimeout`; teste de regressão com prazo curto (`generated_vm_lifecycle.go`, `defaultWaitTimeout`).
- **Vale?** **Sim.** `generate` pode levar 40–90 s e entrar em fila de GPU; precisa de prazo por chamada (flag e argumento) e de cancelamento.

### LIC-VMW-005 — Erros de tool não vão no campo `error` do JSON-RPC
- **Armadilha:** script de smoke que só checa `resp["error"]` conclui "sucesso" para uma tool que falhou (`cer:376-383`).
- **Causa raiz:** o servidor embrulha falhas de tool em `result.isError:true` (convenção MCP).
- **Solução:** smoke/teste inspecionam `result.isError`; só erro de protocolo vai em `error`.
- **Vale?** **Sim.** Mesma convenção no `ollama-mcp-go`; registrar no requisito `TOOL`/`TEST`.

### LIC-VMW-006 — Resposta síncrona e sequencial trava tudo
- **Armadilha:** uma tool lenta bloqueia as demais, inclusive `tools/list` (`main.go:323-345`, sem goroutine por requisição).
- **Causa raiz:** loop único; o mutex de stdout (`:306`) existe mas não é usado por concorrência.
- **Solução/ressalva:** no mcpollama, a GPU é uma só e chamadas paralelas entram em fila (CLAUDE.md do projeto), então serializar **o Ollama** é correto; mas `ping`/`tools/list` não devem esperar por um `generate`.
- **Vale?** **Parcial.** Despachar cada requisição numa goroutine, com um semáforo de 1 só para as chamadas ao Ollama.

## B. Tamanho do `tools/list` e tokens

### LIC-VMW-007 — O peso está nas descrições das propriedades, não na da tool
- **Armadilha:** otimizar só a descrição da tool poupa 10,6% do payload.
- **Causa raiz (medida no plano B1):** 71,1% do payload é `inputSchema`; dentro dele, 66,4% é prosa das propriedades (47,2% do total).
- **Solução:** `--tools-verbosity compact` poda a descrição das propriedades e trunca a da tool na 1ª frase, sem tocar `type/required/enum/default` (`discovery.go:116-201`). **Medido B2:** −64,1% (vCenter), −66,4% (ESXi), −66,9% (Workstation), −63,3% (VMC); com `--profile=host`, −93,9% sobre o `full` completo.
- **Vale?** **Parcial.** Com ~8 tools o ganho absoluto é pequeno (~2 mil → ~0,7 mil tokens); o aprendizado útil é **medir antes** e **repartir a descrição em "o que faz" (1ª frase) e "detalhe" (resto)**. Implementar `compact` só se o `tools/list` do mcpollama crescer.

### LIC-VMW-008 — Truncar na 1ª frase tem armadilhas
- **Armadilha:** cortar no primeiro ponto quebra versões ("7.0.3"), abreviaturas ("e.g.") e frases curtas legítimas ("Is the VM a template?").
- **Causa raiz:** regex ingênua; guarda de comprimento 25 era alto demais (`discovery.go:78-114`, plano B1 §10.1).
- **Solução:** rejeitar corte após dígito, após abreviatura conhecida e abaixo de 15 caracteres; testar contra o **corpus real** (`TestFirstSentence_CorpusReal`, 1.034 descrições).
- **Vale?** **Parcial** (só se o mcpollama adotar o modo compacto). O método (testar contra o corpus real, não contra amostra escolhida) vale sempre.

### LIC-VMW-009 — Poda de schema precisa de cópia profunda e descer nas propriedades aninhadas
- **Armadilha:** mutar o schema no lugar fazia `describe_tool` devolver a versão amputada; a 1ª versão não podava propriedades **aninhadas**.
- **Causa raiz:** tratar o `properties` interno como mapa genérico (`discovery.go:116-121, 152-189`).
- **Solução:** `stripNodeDescription` dedicada; o Registry guarda sempre a definição completa e a poda acontece só na saída (`registry.go:412-418`).
- **Vale?** **Parcial.** Princípio: **guardar o completo e derivar a saída** (serve a `describe_tool`, `TOOLS.md` e testes).

### LIC-VMW-010 — Proxy de ambiguidade não é avaliação com modelo
- **Armadilha:** declarar sucesso olhando só o número de bytes.
- **Causa raiz:** o risco real é o modelo escolher pior a tool com descrição curta (RB-1); o proxy ("0 colisões em 911 primeiras frases") mede ambiguidade, não desempenho; avaliação com modelo ficou pendente (P-B1, `b1-desenho...plan.md:267-269, 288`).
- **Vale?** **Sim.** O mcpollama tem vantagem: poucas tools, então dá para **testar com o modelo** em tarefas-tipo (resumir/extrair/traduzir) e comparar a tool escolhida.

### LIC-VMW-011 — Descrição repetida em centenas de tools
- **Armadilha:** o aviso do `confirm` ("The server also requires --allow-destructive... this argument alone is not enough.") repete-se em 607 tools (`vm.go:26-29`).
- **Causa raiz:** fragmento de schema copiado; cada cópia custa tokens no `tools/list`.
- **Vale?** **Sim, em escala pequena:** texto de aviso comum **uma vez** (na descrição do servidor ou numa tool de ajuda), não por tool.

## C. Segurança e segredos

### LIC-VMW-012 — Audit log grava `args` em texto puro
- **Armadilha:** a trilha de auditoria vira repositório de segredos.
- **Causa raiz:** `auditEntry.Args` recebe `args` sem filtro (`destructive.go:38-47, 182-189`); não há `redact` no código. Existem tools destrutivas reais com argumento de senha (`generated_crypto_kmip.go:198-225`).
- **Medido B2:** chamada com campo `password` fictício → as 3 linhas do audit trazem o valor.
- **Solução (no mcpollama):** allowlist de campos auditáveis (ferramenta, caminho normalizado, tamanho, hash, duração, contadores), nunca o texto.
- **Vale?** **Sim, crítico** para `generate`/`chat` e para as tools de arquivo.

### LIC-VMW-013 — Segredo na URL vaza na mensagem de erro
- **Armadilha:** `vm_password` vai como query e a URL entra no erro (`workstation/client.go:139, 151, 153`; `workstation_vm.go:78-87`).
- **Medido B2:** erro continha a senha fictícia. **Defeito colateral medido:** o `?` ficou **dentro do path** (`%3F`), porque `u.Path = u.Path + path` (`client.go:120`); que o recurso falha contra um `vmrest` real é **(inferência)**.
- **Mesma classe (inferência):** o `refresh_token` do VMC vai na query do POST de troca (`cloudaws/client.go:141`) e erros de transporte carregam a URL.
- **Solução:** segredo só em cabeçalho; função única de formatação de erro que remove `userinfo` e query. O projeto só redige `userinfo` (`u.Redacted()`, `vmware/client.go:86, 92`).
- **Vale?** **Sim.** O token do Ollama deve ir em `Authorization: Bearer` e nunca em URL, log ou erro (CLAUDE.md do projeto, "Segredos").

### LIC-VMW-014 — Tool que lê arquivo do disco sem allowlist
- **Armadilha:** `vmware_datastore_upload_file` lê `local_path` livre (`tools/datastore.go:71-109`); não há `Clean`, `Abs`, `EvalSymlinks`, raiz permitida nem teto de tamanho.
- **Causa raiz:** o chamador era o dono da máquina; a tool foi acrescentada a pedido, com a ressalva de que o caminho é do servidor, não do cliente (`datastore.go:14-19`).
- **Vale?** **Sim, e é o principal buraco a não copiar.** As tools `summarize_file`/`extract`/`translate` leem por pedido do modelo: `--allow-root`, resolução de `..`/*symlink*/UNC/ADS/8.3 do Windows, limite de bytes e de linhas (REQ `FILE`).

### LIC-VMW-015 — Gate e `confirm` estritos; tier "fail-safe" erra nos dois sentidos
- **Armadilha:** classificar por nome do método gera falso positivo (`Unmount*` virou tier1) e falso negativo (`Issue` ficou sem tier).
- **Causa raiz:** regex sobre nome (`gen/main.go:194-205, 523-542`); `fail-safe` = tier2 para o que não casa.
- **Solução:** gate de servidor fechado por padrão + `confirm` booleano estrito (`"true"` e `1` não valem, `destructive_test.go:51-62`) + revisão humana da lista de tier antes de gerar; correções à mão (`cer:384-392`). Tier mais alto para operação que custa dinheiro (VMC, `cer:454-463`).
- **Vale?** **Parcial.** O mcpollama não tem tool destrutiva (nginx devolve 403 em `/api/pull|push|create|delete|copy|blobs`); o que vale é o **teste do gate** (provar que o handler não rodou) aplicado ao equivalente: tool de arquivo fora da raiz permitida.

## D. Windows, build, release e testes

### LIC-VMW-016 — Deploy no Windows sem matar sessões MCP
- **Armadilha:** substituir o `.exe` em uso falha; matar processos derrubava 9 sessões (`cer:34-38`).
- **Solução:** o Windows deixa **renomear** um `.exe` em execução: `Rename-Item` para `.exe.old-<stamp>` e copiar o novo; sessões vivas seguem no antigo até reconectarem.
- **Vale?** **Sim.** O binário do mcpollama fica na raiz do projeto e é o caminho registrado nos clientes MCP.

### LIC-VMW-017 — Higiene de build: `-dirty`, binários rastreados, `.ps1` ASCII, BOM
- **`-dirty`:** o `git describe --dirty` marcou a árvore como suja por arquivos de estado do OpenWolf versionados que regeneram a cada sessão (P-11 aberto); medido: `v0.1.0-11-g26490a2-dirty`. **Solução do projeto:** `git stash` desses 3 arquivos no build. **Para o mcpollama:** não versionar estado de tooling, ou passar `-Version` explícito.
- **Binários rastreados:** `src/gen.exe` e `src/mcpvmware-mcp.exe` (29 MB) já estavam commitados antes do `*.exe` no `.gitignore`; o ignore não remove o que já é rastreado (plano de ambiente D4). **Vale:** sim; o `CLAUDE.md` do mcpollama já manda não versionar binário.
- **`.ps1` com travessão/acento sem BOM:** PowerShell 5.1 lê como ANSI e dá erros de sintaxe que não apontam a causa (`tools/README.md:126-129`). **Vale:** sim; scripts de build do mcpollama em ASCII.
- **`-Encoding utf8` grava BOM e o `go.mod` rejeita BOM** (`:130-131`). **Vale:** sim para qualquer arquivo gerado que o Go leia.

### LIC-VMW-018 — Suíte intermitente no Windows: era o Defender, não o código
- **Armadilha:** "6 testes falham" e depois "outros 2"; hipótese fácil (race de arranque do listener, contenção de portas) **refutada** por evidência.
- **Causa raiz:** handshake TLS em loopback descartado sob rajada (netstat mostra `LISTENING`; `connect()` falha em ~1,000 s exatos; retentativa funciona; 60 iterações isoladas = 0 falhas). Com TLS 1/2 corridas falhou; sem TLS 2/2 passaram (bug-013).
- **Solução:** simulador em HTTP puro por padrão + retentativa limitada só para `*net.OpError` + um teste dedicado de TLS; **sem** `t.Skip` nem `-p 1`.
- **Vale?** **Parcial/sim.** Os testes do mcpollama com `httptest` do Ollama devem usar HTTP por padrão e ter **um** teste de TLS com `--tls-ca`.

### LIC-VMW-019 — Gate de testes confiável e erro de processo ao commitar
- **Armadilha:** `go test ./... | tail; git add; git commit` num único comando: o `exit 0` lido era do `echo` final; commitou com FAIL na tela (`cer:500-525`).
- **Solução:** rodar o gate **num comando**, **ler** o resultado e só então commitar em comando separado; retry de até 3 vezes para flaky de contenção em pacote grande, mas "verde limpo" por retry, não commit com FAIL.
- **Vale?** **Sim** (processo; reforça a regra global de commit disciplinado).

### LIC-VMW-020 — Simulador não é hardware: `null` vs `[]`
- **Armadilha:** 5 defeitos de contrato (`null` em vez de `[]`) só apareceram contra ESXi real (~15% das tools de leitura sondadas, bug-009/014/015).
- **Causa raiz:** o `vcsim` devolve slice vazio não-nil; o hardware real devolve `nil`; `encoding/json` serializa `nil` como `null`. A 1ª versão do teste de regressão **passava com o defeito presente**.
- **Solução:** rede central em `marshalJSON` (`normalizeNilSlices`) + `nonNilSlice` explícito + teste que exercita o helper com `nil` direto.
- **Vale?** **Sim, por analogia:** o mock `httptest` do Ollama devolve respostas "bonitas"; o servidor real tem comportamentos que o mock não tem (campos ausentes, NDJSON, `done_reason`, erro com HTTP 200 no meio do stream). Validar as tools contra o Ollama real (uma chamada por vez), e escrever os mocks a partir de respostas **reais** capturadas.

### LIC-VMW-021 — Provar o CI com um teste negativo
- **Armadilha:** CI "verde" sem prova de que pega regressão.
- **Solução:** ramo descartável com bug **deliberado** (retirar `normalizeNilSlices`): falhou exatamente no passo `go test`, com `build`/`vet`/`gofmt` passando antes. Revertido, 3/3 verde (STATUS 2026-08-23).
- **Vale?** **Sim.** Barato e dá confiança; aplicar quando o CI do mcpollama existir.

### LIC-VMW-022 — Checkout de referência pode divergir da versão fixada em `go.mod`
- **Armadilha:** `go build` falhou por `DeleteSolutionOnly`, método que existe em `referencia/govmomi` mas não no módulo v0.55.1 (bug-012).
- **Solução:** o gate de `go build` limpo por fase pegou; a tool foi removida.
- **Vale?** **Parcial.** Quem lê `referencias/` (como esta onda) não deve assumir que a API lida é a que o `go.mod` do mcpollama vai compilar.

### LIC-VMW-023 — Deixar um teste/smoke chamar serviço de terceiros em produção
- **Armadilha:** não há flag para redirecionar os hosts do CSP/VMC para um fixture; smoke sem gate iria à rede com token falso.
- **Solução:** o smoke da Fase 10 chamou só tool tier1 **sem** `--allow-destructive` (o gate fecha antes da rede); nenhuma tool sem tier foi chamada (`cer:472-480`).
- **Vale?** **Parcial.** Para o Ollama real da CSL o smoke ao vivo é legítimo, mas **uma chamada por vez** (4 GPUs, 1 modelo residente) e sem logar o token.

## E. Desenho do registro e do catálogo

### LIC-VMW-024 — Adicionar um 2º/3º tipo de cliente sem tocar nas tools existentes
- **Solução:** campo novo em `Tool` (`WSHandler`, `CloudHandler`) em vez de `interface{}`; campo novo opcional em `RegistryOptions` em vez de parâmetro posicional de `NewRegistry`; zero testes tiveram de mudar, provado 2 vezes (`cer:415-426, 446-453`).
- **Vale?** **Parcial.** Só se o mcpollama precisar de um 2º backend; o princípio "campo opcional novo, nunca parâmetro posicional" vale já.

### LIC-VMW-025 — Registry com cliente nulo permite dump offline do catálogo
- **Solução:** `NewRegistry(ctx, nil, opts)` não desreferencia o cliente (handlers são valores) → `cmd/dumptools` imprime o catálogo sem backend (`cer:688-693`; `cmd/dumptools/main.go:3-6`). Tools destrutivas listam sempre; o gate é na chamada.
- **Vale?** **Sim**; é a base do `TOOLS.md` gerado que o owner decidiu adotar.

### LIC-VMW-026 — Teste de conjunto exato por modo
- **Armadilha:** tool nova registrada fora da lista passa despercebida, ou some de um modo.
- **Solução:** igualdade de conjunto, não `contains` (`mode_test.go:1165-1193`); as 6 verificações de modo falharam todas ao entrar as 2 tools meta, e a correção foi num só lugar (`plano B1 §10.2`).
- **Vale?** **Sim.** Um teste "lista exata de tools" no mcpollama impede que uma tool administrativa entre por engano.

### LIC-VMW-027 — Nomes de tool são contrato (compatibilidade)
- **Decisão (owner, 2026-08-22, "Estratégia A"):** nenhum nome público muda; evolução é aditiva e opt-in; quebrar exige deprecação com versão de remoção ou frase explícita do owner (`plano-evolucao-produto...plan.md:95-110`).
- **Vale?** **Sim**: fixar os nomes (`list_models`, `generate`, `chat`, `embed`, `summarize_file`, `extract`, `translate`) antes do primeiro cliente os registrar.

### LIC-VMW-028 — A cobertura "100%" media o wrapper, não a superfície
- **Armadilha:** "100% das 734/611 tools" era 100% dos métodos com wrapper `object.*`; a família iSCSI inteira estava fora (`cer:74-88`); o README também prometia "100% da superfície" e foi corrigido em 2026-08-23 (`README.md:23-28`).
- **Causa raiz:** o gerador só via um subconjunto; "registrada" e "provada contra hardware" eram métricas misturadas (42 + 61 provadas de 1.047).
- **Vale?** **Sim:** separar "tool registrada" de "tool validada contra o Ollama real" no estado do projeto.

## F. Processo (delegação, honestidade, evidência)

### LIC-VMW-029 — Nunca escrever "o mecanismo já existe" sem `grep`
- **Casos:** nota "o gerador pula nomes já registrados" (falsa, 8 colisões, `cer:571-581`); comentário "relies on the caller's timeout" (falso, `LIC-VMW-004`); comentário do `handleVMCustomize` alegando que o decode genérico cobre um campo polimórfico (falso, bug-011).
- **Vale?** **Sim.** Afirmação sobre código no plano/brief tem `caminho:linha` ou é "(inferência)".

### LIC-VMW-030 — Relatório vago de subagente = sinal de problema; não fixar a hipótese fácil
- **Casos:** o subagente que terminou com "vou esperar o teste" escondia 3 bugs reais atrás de um teste preso (`cer:558-570`); o `vcsim` travava por deadlock do simulador (panic sem `defer` no `WithLock`), refutando a hipótese de "socket exhaustion" (`cer:484-499`); 2 de 3 subagentes Haiku **inventaram** sumários de PDF que não conseguiram ler (`cer:594-608`).
- **Vale?** **Sim**, para a delegação do próprio brief: ler PDF só após `pdftotext`; correr e **ler** o resultado, não aceitar o relato.

### LIC-VMW-031 — Copiar o vizinho não é ler a regra-fonte
- **Caso:** a nomenclatura de planos foi copiada de um arquivo, mas a regra que define também `reports/` nunca foi aberta; resultados ficaram espalhados em `.wolf/` sem relatório formal (`cer:540-557`). E a atualização do plano precisou ser conferida contra o checklist item a item (`cer:526-539`).
- **Vale?** **Sim**; o mcpollama tem o mesmo pack. Checklist literal de atualização de plano.

### LIC-VMW-032 — Scripts auxiliares no Windows: encoding e caminhos
- **Casos:** Python `open()` sem `encoding="utf-8"` usa cp1252 e fez "ver" mojibake inexistente; caminhos do Git Bash (`/tmp`, `/d/...`) não são entendidos pelo Python nativo (`cer:677-686`); JSON de terceiros com BOM UTF-8 quebra `JSON.parse` (bug-003).
- **Vale?** **Sim**, em especial para `summarize_file`/`extract`: tirar BOM, detectar UTF-16 e decodificar com `utf-8` explícito; e provar o mojibake por `hexdump` antes de "corrigir".

### LIC-VMW-033 — Erro do backend com chaves em caixa diferente da spec
- **Caso:** o `vmrest` serializa `{"Code","Message"}` com maiúsculas, diferente da spec; foi achado ao vivo (`cer:248-255`; `workstation/client.go:55-58`).
- **Vale?** **Sim:** a API nativa do Ollama devolve `{"error":"..."}`; o parse de erro deve ser tolerante e cair para o corpo cru truncado.

### LIC-VMW-034 — Log de lacunas alimentado por skill
- **Padrão:** `MCP-GAPS-VMWARE.md` registra, com evidência e contorno, cada vez que uma operação não pôde ser feita pelo MCP (BUG ou LACUNA), com formato fixo e contagem de ocorrências (`MCP-GAPS-VMWARE.md:1-33`). Já gerou pedido de feature real (config SNMP do host, `:54-59`).
- **Vale?** **Parcial.** Um `MCP-GAPS-OLLAMA.md` serve para registrar quando o nível `local` não deu conta e subiu para haiku.

### LIC-VMW-035 — O `CLAUDE.md` do projeto era o template Delphi
- **Caso:** `MCPVMWare/CLAUDE.md` descreve fases de bootstrap Delphi/FPC (`*.dpr`, `bootstrap-build-config.ps1`) e nada de Go/MCP; o conhecimento real vive em `.wolf/` e `.workspace/`.
- **Vale?** **Sim/negativo:** o `CLAUDE.md` do mcpollama já diz para não usar o wizard Delphi; manter o comando de build, o smoke stdio e as armadilhas **no próprio `CLAUDE.md`**, não só em `.wolf/`.

### LIC-VMW-036 — Lições específicas do govmomi (não valem para o mcpollama)
- `InventoryPath` vazio em objeto criado por construtor (bug-008), campos polimórficos `Base*` que `json.Unmarshal` não preenche (bug-011, `cer:131-141`), `Finder` sem datacenter (bug-002), fault `InvalidName` em `Query*` (bug-005), `WrapSoapFault` vs `WrapVimFault`, vcsim com `ESX()` vs `VPX()` simétricos ao contrário (`cer:95-101, 151-167`), `object.Task.Wait*` unificado (`cer:286-294`).
- **Vale?** **Não** (domínio SOAP/vSphere). Fica o método: provar o comportamento no simulador/fonte, não por nome.

## As 5 mais importantes para o mcpollama

1. **LIC-VMW-001** — o `Scanner` de 64 KB (medido) derruba o servidor com prompt grande.
2. **LIC-VMW-012** — audit log com `args` crus (medido): no mcpollama gravaria prompts e conteúdo de arquivo.
3. **LIC-VMW-014** — tool que lê arquivo sem allowlist: o desenho de `--allow-root` precisa nascer pronto.
4. **LIC-VMW-003** — backend fora no boot vira "Connection closed": o mcpollama sobe sempre e falha por chamada.
5. **LIC-VMW-004** — nenhum prazo por chamada: `generate` precisa de timeout e cancelamento (e `ping`, LIC-VMW-002, não pode esperar).

**Total: 36 lições `LIC-VMW`** (028 a 036 incluem processo e as de domínio que não se aplicam).
