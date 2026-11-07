---
title: "Plano: Fase 14, parâmetro `seed` no `generate` e no `chat` do mcpollama"
name: "Fase 14: seed no generate e no chat"
overview: "Acrescentar `seed` (inteiro opcional) às tools `generate` e `chat` do mcpollama-mcp, para rodadas pareadas e reprodutíveis (contrato de dados da Fase 12, C-5.15), sem tocar no gateway."
created: 2026-11-07
updated: 2026-11-07
status: draft
locale: pt-BR
---

# Plano: Fase 14, parâmetro `seed` no `generate` e no `chat` do mcpollama

> **Estado (2026-11-07 00:58):** ✅ **S1 concluída** (ramo `feat/seed-generate-chat` e linha de base verde); **S2 a S8 pendentes, nada implementado**, por ordem do owner. Texto original: 📝 **PLANO, não aprovado. Nada foi implementado.** Escrito a pedido do owner em 2026-11-07 00:14, como **plano novo**; nenhum plano existente foi alterado.
> **Origem:** projeto `Treinamento` (treino do Qwen em Object Pascal), que precisa medir a variação entre rodadas do modelo e hoje não consegue pelo MCP.
> **Relacionados:** contrato de dados da Fase 12 (`docs/CONTRATO-DADOS-TREINO.md`, regras C-5.1 e C-5.15); plano da Fase 12 (§11, metodologia obrigatória); plano da Fase 13 (`rota automática`, não aprovado). Este plano **não depende** deles.

## 0. Para quem lê este documento

Plano de uma mudança pequena e localizada em `src/tools/`. Ele segue a metodologia da §11 do plano da Fase 12 (regras do owner, gates, implantação e armadilhas do ambiente); **não a repete**, só aponta onde se aplica. A §2 tem os fatos, com `arquivo:linha`. A §5 tem as decisões propostas. A §8 tem as questões que o owner precisa fechar antes da etapa S2.

## 1. Objetivo

- O `generate` e o `chat` passam a aceitar o argumento opcional **`seed`** (inteiro), repassado ao Ollama em `options.seed`.
- Com isso, quem usa o MCP **pode fazer chamadas pareadas à mão** (mesma entrada, sementes 1, 2 e 3, intercaladas), como o contrato pede (C-5.15), **sem** chamar o gateway por fora e **sem perder** o `reportar_erro` (o job continua saindo da mesma estação e do mesmo IP).
- **O que a fase NÃO faz:** ela só **habilita** o argumento. Não implementa as rodadas pareadas nem a comparação intercalada do modelo atual com o candidato (isso é o `bakeoff`, que já monta o corpo direto). E o MCP continua sem expor todo o `prompt_json` arbitrário: depois da fase, as opções de amostragem acessíveis são só `temperature`, `num_predict` e `seed`.
- Sem `seed`, o comportamento atual não muda.

Fora do escopo (ver §3): `top_p`, as tools de arquivo, o gateway, a Fase 13.

## 2. Fatos de partida (2026-11-07, lidos no código; HEAD `0eb9391`)

1. **Argumentos aceitos hoje.** `tools/generate.go:54` aceita só `prompt`, `system`, `model`, `temperature`, `num_predict` e `think`; `tools/chat.go:87` aceita `messages`, `system`, `model`, `temperature`, `num_predict` e `think`. Argumento fora da lista é recusado por `parseArgs`.
2. **Esquema da tool.** `tools/generate.go:23-30`, com os auxiliares de `tools/schema.go` (`numberProp`, `intProp`, `boolProp`; não há auxiliar de `seed`).
3. **Opções de amostragem.** `samplingOptions` (`tools/generate.go:38-51`; o `chat.go:113` **já a reaproveita**) lê só `temperature` e `num_predict` e devolve `nil` quando os dois faltam.
4. **O cliente já suporta.** `ollama/types.go:6-11`: `ModelOptions` tem `Seed *int` (`json:"seed,omitempty"`) e `TopP *float64`.
5. **O gateway já deixa passar.** `DefaultStripOptions` remove só `num_gpu`, `main_gpu`, `low_vram`, `use_mmap`, `use_mlock` e `numa` (`gateway/proxy/policy.go:28`); `seed` não está na lista.
6. **Único uso de `seed` hoje:** `cmd/bakeoff/erros_replay.go:33-34` (`replaySeeds = []int{1, 2, 3}`) e `withSeedAndModel` (linha 130), que montam o corpo do pedido direto, sem o MCP.
7. **Tools de arquivo.** `tools/filetools.go:42` fixa `fileTemperature = 0.2` e as chamadas (`filetools.go:396-399`) não passam `seed`; `extract`, `summarize_file` e `translate` não aceitam argumentos de amostragem.
8. **Catálogo travado por teste.** `TOOLS.md` é gerado por `src/cmd/dumptools`; três testes falham se divergir: `cmd/dumptools/main_test.go`, `tools/registry_test.go` e `mcpollama-mcp/tools_wiring_test.go`.
9. **Testes que tocam as opções:** `tools/generate_chat_test.go`, `tools/think_test.go`, `tools/autoreport_test.go` (e `gateway/joblog/fase12_prompt_json_test.go`, para a regra C-2.10: o `prompt_json` gravado é o corpo que chegou ao Ollama).
10. **Binário em uso:** `D:\MCP\mcpollama-mcp.exe` = `0.10.2-09e2936` (2026-11-05 12:57). Todas as sessões que usam o MCP carregam esse arquivo.
11. **Orçamento de bytes do `tools/list` (achado crítico da revisão Fable 5.1).** `tools/registry_test.go:19-20` fixa `toolsListBudget = 5640` (cinco tools base) e `toolsListBudgetFull = 9345` (com as tools de arquivo). Medido em 2026-11-07: **5624 e 9331 bytes, folga de 16 e 14** (os comentários "36/34 bytes" estão desatualizados). Um `seed` com `minimum`, `maximum`, `type` e descrição em duas tools custa, no mínimo, ~58 bytes por tool: **`TestToolsListBudget` e `TestToolsListBudgetWithFileTools` vão falhar**. Cada subida de teto no histórico foi decisão do owner (comentários "owner 2026-10-29", "owner D-13"). Ver D14-11.
12. **Mais 3 testes do catálogo falham na S2** se o esquema mudar e o `TOOLS.md` não for regenerado: `TestCatalogInSync`, `TestToolsMdMatchesTheRealToolsList` e `TestWriteThenCheck` (além dos 2 do orçamento).
13. **Prova do corpo ao vivo.** O endpoint de conteúdo do `/monitor` devolve o prompt achatado, resposta e tamanhos (`gateway/monitor/jobs.go:286-296`, `gateway/content/content.go:634-649`); `options` **não** aparece. `GetPromptJSON` (`content.go:761`) só serve ao `joblog` e ao `treino`. Ver D14-13.
14. **Semântica de `seed=0` ambígua na documentação do Ollama v0.40.13:** o `modelfile.mdx` diz "default 0" e o `cmd/bench/README.md` diz "-seed 0 (random)". Ver D14-12.
15. **Teste C-2.10 já tem `seed`:** `gateway/joblog/fase12_prompt_json_test.go:117,119` usam `seed:3` e `seed:1` nos corpos. A parte "gateway grava o `prompt_json` com `options.seed`" já está provada; falta só provar que a **tool** envia.
**Não verificado (a medir na S5):** a semântica exata do `seed` no Ollama usado (valor 0, valores negativos, valor omitido) e se a mesma `seed` com a mesma entrada devolve **exatamente** o mesmo texto na GPU. A documentação do MCP (`D:\MCP\Documentation\Ollama\04-tools.md:181`, fora do repositório; **não** o `README.md`) já diz que `temperature: 0` não prova determinismo entre rodadas; o `seed` pode ter o mesmo limite.

## 3. Escopo e exclusões

**Dentro:** `seed` no `generate` e no `chat`; validação; testes; `TOOLS.md`; documentação; build; implantação em `D:\MCP`; prova ao vivo.

**Fora (cada item exigiria decisão própria):**
- `top_p` (já existe no tipo, mas ninguém pediu);
- `seed` nas tools de arquivo (`extract`, `summarize_file`, `translate`), que hoje rodam com temperatura 0,2 fixa. **Consequência registrada:** a Fase 14 **não resolve** o controle de variação da revisão do `BRIEFS` do `Treinamento`, que usa essas tools e portanto não poderá repetir chamadas pareadas por `seed`. Se isso for necessário, vira plano próprio (questão 2 da §8);
- qualquer mudança em `src/gateway/` (não é necessária);
- a Fase 13 (rota automática) e a Fase 11;
- renomear tool (quebra de contrato).

## 4. Desenho (proposta)

1. **Esquema:** `seed` = inteiro opcional, com descrição curta em inglês no padrão do arquivo; auxiliar `seedProp()` em `tools/schema.go`, reaproveitado pelas duas tools.
2. **Validação:** inteiro entre **`1`** e `2147483647` (decisões **D14-3** e **D14-12**; o `0` é **recusado**, por ser ambíguo no Ollama), via `optInt(key, 1, 2147483647)` existente; recusar texto, decimal, objeto/array e valor fora do intervalo (inclui `0`, `-0` e negativos), com a mesma mensagem de erro dos outros argumentos numéricos. **`null` = omitido** (decisão **D14-9**): `args.get` (`tools/args.go:48`) já trata `null` como ausente, e `temperature` e `num_predict` fazem o mesmo; recusar `null` só no `seed` exigiria validação à parte e deixaria o argumento inconsistente com os irmãos.
3. **Código:** `generate.go` e `chat.go` ganham `"seed"` na lista do `parseArgs`; `samplingOptions` passa a ler `seed` e só devolve `nil` quando os **três** faltam. O `chat` **já reaproveita** `samplingOptions` (`chat.go:113`); nada a fazer além de a função passar a ler `seed`.
4. **Efeito colateral a conferir:** `autoCheck` e `numPredictOf` usam `NumPredict`; não devem mudar de comportamento.
5. **Gateway e joblog:** sem mudança de código. O `prompt_json` gravado pela C-2.10 passa a conter `options.seed` quando o cliente a enviou; é o comportamento correto e entra num teste.
6. **Compatibilidade:** argumento **novo e opcional** não quebra clientes existentes. Versão sugerida: **0.11.0** (nova capacidade), a confirmar na D14-1.

## 5. Decisões propostas (D14-n)

| # | Decisão | Estado |
| --- | --- | --- |
| D14-1 | Número da versão: **0.11.0** (nova capacidade; precedentes: 0.9.0 trouxe o fallback e 0.10.0 a `reportar_erro`) ou 0.10.3 | **decidida: 0.11.0** (questão 1) |
| D14-2 | Escopo: só `seed` no `generate` e no `chat`; `top_p` e as tools de arquivo ficam para outro plano | **decidida** (questão 2) |
| D14-3 | Intervalo aceito: inteiro de **`1`** a `2147483647`; omitido = padrão do servidor (nada é enviado); `0` recusado | **decidida** (2026-11-07 00:50; mínimo 1 pela D14-12). O limite superior segue sem confirmação contra o runner; a S5 só observa |
| D14-4 | Não mexer em `src/gateway/` (o gateway já repassa `seed`) | proposta |
| D14-5 | O `chat` reaproveita `samplingOptions` (SSOT) | **já atendido** no código (`chat.go:113`); sem trabalho |
| D14-6 | Implantar em `D:\MCP` **fora** de qualquer rodada longa que use o MCP e só com o owner; antes, compilar para um caminho temporário e fazer o teste de fumaça por stdio | **decidida: só depois do `BRIEFS`** (questão 3) |
| D14-7 | Documentar que o `seed` dá **amostragem pareada**, não garantia de texto idêntico, se a S5 mostrar variação; a S5 é **observação exploratória** e não afirma que o servidor aplicou o `seed` | proposta |
| D14-8 | Commit por etapa verde, por caminho; **push e tag só com autorização** (§11.1 da Fase 12) | regra existente |
| D14-9 | `seed: null` é tratado como **omitido** (igual a `temperature` e `num_predict`), não recusado. Alternativa: recusar, com validação própria que distinga campo ausente de `null` | **decidida: omitido** (questão 6) |
| D14-11 | **Orçamento do `tools/list` (decisão do owner, bloqueia a S2).** Os tetos 5640/9345 não comportam o `seed`. Opções: (a) subir os dois tetos para o valor medido após a mudança + ~15 bytes de folga (recomendada: o `seed` é capacidade nova); (b) encurtar o esquema (sem `minimum`/`maximum`, descrição mínima) e subir só o necessário. Em ambos, atualizar os comentários desatualizados de folga | **decidida: (a)** (questão 7, 2026-11-07 00:50): subir os dois tetos pelo crescimento **medido** após a mudança, preservando a folga atual (~16 e ~14 bytes), **sem** encurtar o esquema a ponto de prejudicar a descrição; registrar o número medido e "owner 2026-11-07" no comentário de cada teto |
| D14-12 | **Mínimo do `seed`: 0 ou 1.** Se no runner `0` for aleatório, `seed:0` seria igual a omitido. Recomendação: **mínimo 1** (o contrato C-5.15 usa 1, 2, 3; nada se perde e some a ambiguidade). Se mantiver 0, a S5(e) precisa provar a semântica | **decidida: mínimo 1** (questão 8, 2026-11-07 00:50). Alinhados §4.2, D14-3, S3, S5 e os riscos |
| D14-13 | **Prova de que o `seed` chegou ao Ollama:** (a) prova **transitiva** na S3 (tool → corpo do Ollama simulado; a parte gateway → `prompt_json` já está provada em `fase12_prompt_json_test.go:117,119`) e, ao vivo, só o efeito estatístico da S5; ou (b) além disso, um **harness tool → proxy → Ollama falso** (trabalho novo, não dimensionado; o rig da C-2.10 é interno ao pacote `joblog`, não exportado); ou (c) leitura somente leitura do `content.db` no servidor (`sqlite3`, alias `lx-ai01-root`), que contém pedidos de clientes. Recomendação: **(a)** | **decidida: (a)** (questão 9, 2026-11-07 00:50): sem harness novo e sem ler o `content.db`; a prova do envio fica nos testes da S3 |
| D14-10 | A fase **habilita** o `seed` por chamada; **não** implementa rodadas pareadas nem o bake-off, e não resolve o `seed` das tools de arquivo | proposta (ver §1 e §3) |

## 6. Etapas

Cada etapa tem um gate. A cada etapa, este plano é atualizado com o que deu certo, o que não deu, a solução, os números medidos, os commits e as pendências numeradas (§11.1 da Fase 12).

| Etapa | O que fazer | Quem | Gate |
| --- | --- | --- | --- |
| **S0** | Owner aprova o plano e fecha as questões da §8 | owner | questões 1 a 9 fechadas e registradas na §8 (1 a 6 em 2026-11-07 00:28; 7 a 9 em 00:50); **falta só a aprovação final do plano para iniciar a S1** |
| **S1** ✅ *concluída 2026-11-07 00:58* | Criar o ramo `feat/seed-generate-chat` a partir do HEAD e **medir a linha de base**: `go build ./...`, `go vet ./...`, `gofmt -l .` (vazio), `go test -count=1 ./...`, build Linux (`$env:GOOS='linux'; $env:CGO_ENABLED='0'; go build -o <caminho temporário fora de D:\MCP> ./mcpollama-mcp`, depois `Remove-Item Env:GOOS,Env:CGO_ENABLED`; registrar GOOS, CGO_ENABLED e o destino), `-race` (com o gcc de `deps\gcc-mingw-w64\bin`) | sonnet | tudo verde, com os números registrados; **registrar também a linha de base do orçamento do `tools/list`: 5624/5640 e 9331/9345** |
| **S2** | Implementar (§4): `seedProp`, `generate.go`, `chat.go`, `samplingOptions`; **na própria etapa**, regenerar o `TOOLS.md` (`src/cmd/dumptools`) e ajustar os tetos `toolsListBudget`/`toolsListBudgetFull` conforme a D14-11 (e corrigir os comentários de folga) | sonnet | compila; testes antigos verdes, **incluindo os 5 que o esquema novo mexe** (`TestToolsListBudget`, `TestToolsListBudgetWithFileTools`, `TestCatalogInSync`, `TestToolsMdMatchesTheRealToolsList`, `TestWriteThenCheck`) |
| **S3** | Testes novos, **um por regra**, cada um **provado por mutação** (falha sem a mudança): `seed` chega em `options.seed` no `generate` e no `chat` (Ollama simulado por `httptest`); sem `seed`, o campo **não** aparece no corpo; **fronteiras válidas:** `seed=1` e `seed=2147483647` são aceitos e chegam **intactos** em `options.seed` (nas duas tools); valores inválidos (texto, **`1.5`** como decimal, `true`, objeto, array, **`0`**, `-0`, `-1`, `2147483648`) são recusados (o `0` e o `-0` caem fora do intervalo por causa do mínimo 1, D14-12); **valores aceitos que não devem espantar:** `1.0` (=1) e `1e3` (=1000) são inteiros em float64 e passam (`args.go:124-138`); literal fora de float64 (`1e400`) é recusado, mas com a mensagem genérica "os argumentos devem ser um objeto JSON" (`args.go:22-25`); `seed: null` é tratado como omitido (D14-9) e o campo **não** aparece no corpo; argumento desconhecido continua recusado; o `think` e o `autoCheck` não mudam; o `prompt_json` registrado contém a `seed` (C-2.10), **prova transitiva (D14-13a):** o teste da tool prova que `options.seed` está no corpo que sai para o Ollama simulado, e a C-2.10 já está provada com `seed` em `fase12_prompt_json_test.go:117,119`. Só se o owner escolher a D14-13(b) entra o harness tool → proxy → Ollama falso, como item próprio e dimensionado (não há harness assim hoje; verificar ciclo de imports); regenerar o `TOOLS.md` e ver os 3 testes do catálogo verdes | sonnet | testes verdes com `-count=1` e `-race`; mutações provadas |
| **S4** | Revisão **adversarial** leve (opus) da entrada nova: estouro de inteiro, número gigante, `null`, tipo errado, objeto no lugar de inteiro; interação com `think` e com a política do gateway. Guardar os testes de ataque em `.workspace/reports/<...>.testes/` | opus | achados tratados ou justificados |
| **S5** | **Observação ao vivo, exploratória, sem gate de aprovação** (ela **não prova** que o servidor aplicou o `seed`: só a S3 prova o envio), **uma chamada por vez**: (a) `generate` com `seed=1` duas vezes, mesma entrada, `temperature` acima de 0, e comparar os textos; (b) idem `seed=2`; (c) sem `seed`; (d) `chat` com `seed`, **duas chamadas com a mesma entrada e a mesma seed**, registrando os dois textos (sem exigir identidade); (e) valor omitido contra `seed=1`, para registrar a diferença observada (o `seed=0` não é testável: a tool o recusa, D14-12). O `[uso]` **não prova** o corpo enviado: a prova de que `options.seed` saiu está nos testes da S3 (Ollama simulado) e, ao vivo, só pelo efeito estatístico acima. O `/monitor` **não mostra `options`** (fato 13): leitura do `content.db` só se o owner escolher a D14-13(c). Registrar que, sem isso, a S3 é a única prova do corpo. **Desenho estatístico mínimo** (duas chamadas por semente não bastam; o cache de prompt também pode mudar o resultado da 2ª chamada): ≥ 5 prompts curtos, `temperature` 1.0, `num_predict` ~64, `think:false`; por prompt, `seed=1` ×2, `seed=2` ×1 e sem `seed` ×2; comparar a taxa de textos idênticos entre **mesma seed**, **seed diferente** e **sem seed**. Mesma seed ≫ as outras = aplicado; tudo baixo e igual = ignorado **ou** GPU não determinística (só a prova do corpo, D14-13, separa os dois casos). Registrar **honestamente** o que se repete e o que não | sonnet + owner | resultado **registrado como observação** (sem aprovar nem reprovar a fase); D14-7 confirmada ou ajustada conforme o que se observou |
| **S6** | Documentação: `CHANGELOG.md` (seção "Não lançado" → versão), `README.md` (tabela de versões e tools), `CLAUDE.md` (estado), `TOOLS.md` (gerado), e, fora do repositório, `D:\MCP\Documentation\Ollama\04-tools.md` e o README dessa pasta (**fora do git: copiar antes para `.bak` com data**) | sonnet | `git diff` só com o previsto; varredura de segredos nas linhas adicionadas (meta 0) |
| **S7** | **Commit** por etapa verde, por caminho, com `git commit -F` e `Co-Authored-By` (§11.1). **Build e implantação** só com o owner e fora de rodada longa: compilar para caminho temporário, teste de fumaça por stdio (`initialize`, `tools/list`, `list_models`), guardar o binário atual como `mcpollama-mcp.exe.bak-0.10.2` (**antes, verificar se já existe e não sobrescrever**; se existir, usar sufixo com data), compilar para `D:\MCP\mcpollama-mcp.exe` com `-ldflags "-X main.Version=<versão>-<hash>"`, **reiniciar as sessões**, **apagar o `.exe~` que o `go build` deixa** (CLAUDE.md, seção Comandos) e validar com uma chamada real (nota: quem renomeia o `.exe` em uso para `.exe~` é o `go build`, não o Windows; existe também o binário Linux `D:\MCP\mcpollama-mcp`; backups anteriores já existem: `.bak-0.9.2`, `.bak-0.10.0`, `.bak-0.10.1`) | owner + sonnet | versão conferida pelo binário (`--version`) e por uma chamada real |
| **S8** | **Push e tag** `mcpollama-v<versão>`: só com autorização do owner, depois da varredura de segredos | owner | tag criada |

## 7. Riscos

| Risco | Efeito | Mitigação |
| --- | --- | --- |
| O `seed` não garante texto idêntico na GPU | rodada "pareada" menos útil do que o contrato supõe | S5 mede; D14-7 documenta o limite; o contrato já trata a variação (2 de 3 rodadas) |
| Semântica do `seed` no Ollama (negativos, omitido, limite superior) | comportamento surpreendente | o `0` e os negativos são recusados pela tool (D14-12); a S5 só observa e registra |
| Binário compartilhado por todas as sessões | build ruim derruba o MCP de todos | compilar fora de `D:\MCP`, teste de fumaça por stdio, backup do `.exe` atual |
| Rodada longa em andamento no projeto `Treinamento` (revisão do `BRIEFS`, estimativa de mais de 30 h) usa o mesmo `D:\MCP\mcpollama-mcp.exe` | trocar o binário no meio: o Windows renomeia o `.exe` em uso para `.exe~`; a rodada segue com o processo antigo, mas um reinício dela pegaria o novo | D14-6: implementar e testar em caminho temporário agora; implantar só depois, com o owner |
| `chat` duplicar a leitura das opções | divergência futura | **já atendido** (`chat.go:113`); risco encerrado |
| Orçamento do `tools/list` estoura (5624/5640 e 9331/9345; folga de 16 e 14 bytes) | 2 testes falham na S2 e o gate fica inalcançável | D14-11 decidida antes da S2; teto subido na própria S2, com o número medido |
| `seed=0` pode significar "aleatório" no runner | `seed:0` igual a omitido, valor inútil exposto | **resolvido pela D14-12:** mínimo 1, o `0` é recusado e tem teste na S3 |
| A S5 não distingue "seed ignorado" de "GPU não determinística" | conclusão enganosa | desenho estatístico da S5 + D14-13 |
| `go build` deixa `.exe~` em `D:\MCP` | lixo ao lado do binário | S7 manda apagar após reiniciar |
| Catálogo (`TOOLS.md`) esquecido | 3 testes falham | S3 regenera e roda os testes do catálogo |
| Mudar o contrato por engano | o contrato vence o código (C-0.3) | a mudança não altera regra nenhuma do contrato; só habilita C-5.15 pelo MCP |
| PowerShell 5.1 corromper acentos em arquivos Go | texto quebrado | usar as ferramentas de edição do agente ou .NET `UTF8Encoding($false)` (§11.4) |

## 8. Questões para o owner (fechar antes da S2)

**Fechadas pelo owner em 2026-11-07 00:28** ("já incluir no plano"): as respostas abaixo são as recomendações do plano, aceitas pelo owner.

1. **Versão:** ✅ **0.11.0** (D14-1 decidida).
2. **Escopo:** ✅ **só `generate` e `chat`**; `top_p` e `seed` nas tools de arquivo ficam fora (D14-2 decidida). A revisão do `BRIEFS` segue sem `seed`.
3. **Implantação:** ✅ **só depois que a revisão do `BRIEFS` do `Treinamento` terminar** (D14-6 decidida). Até lá: implementar e testar em caminho temporário.
4. **Execução:** ✅ eu **coordeno e delego a implementação a um subagente `sonnet` com revisão `opus`**, como na §11 da Fase 12 (a entrada do owner foi incluir a recomendação do plano; reverter aqui se não for isso).
5. **Ramo e commits:** ✅ ramo `feat/seed-generate-chat`, commit por etapa verde, por caminho, **sem push** (mesma ressalva da questão 4).
6. **`seed: null` (D14-9):** ✅ **tratado como omitido** (D14-9 decidida).

**Levantadas pela revisão Fable 5.1 (2026-11-07 00:37) e fechadas pelo owner em 2026-11-07 00:50** ("atualizar o plano", aceitando as recomendações, que o GPT-6 também revisou e confirmou):

7. **Orçamento do `tools/list` (D14-11):** ✅ **subir os dois tetos pelo crescimento medido**, preservando a folga atual (~16 e ~14 bytes), sem encurtar o esquema a ponto de prejudicar a descrição.
8. **Mínimo do `seed` (D14-12):** ✅ **1**. O `0` é recusado; §4.2, D14-3, S3, S5 e riscos alinhados.
9. **Prova do corpo (D14-13):** ✅ **(a)** prova transitiva na S3; sem harness novo e sem ler o `content.db`. A S5 é observação exploratória, sem gate.

## 9. Changelog

- 2026-11-07 00:58: **S1 CONCLUÍDA (ramo e linha de base), sem implementar.** Autorizada pelo owner às 00:52 ("vamos executar, sem implementar ainda"; às 00:57: "dê continuidade até o final, sem implementar"). Executada por subagente Sonnet; estado conferido por mim depois (ramo, HEAD, status, `D:\MCP`). **Ramo:** `feat/seed-generate-chat` a partir de `0eb9391`. **Linha de base (go1.27.0 windows/amd64, `src/`), tudo verde:** `go build ./...` código 0 (2 s); `go vet ./...` 0 (1 s); `gofmt -l .` vazio; `go test -count=1 ./...` 24 pacotes ok, 0 falha, 1 sem arquivos de teste (`mcp`), 53 s; build Linux (`GOOS=linux`, `CGO_ENABLED=0`, destino temporário no scratchpad, fora de `D:\MCP`) 0, binário de 22.769.753 bytes, apagado depois; `go test -race -count=1 ./...` (gcc de `deps\gcc-mingw-w64\bin`, `CGO_ENABLED=1`) 24 pacotes ok, 0 falha, 193 s. **Orçamento do `tools/list`:** `TestToolsListBudget` 5624 de 5640 (folga 16) e `TestToolsListBudgetWithFileTools` 9331 de 9345 (folga 14), os dois verdes. **O que deu certo:** os números bateram com o esperado no plano. **O que não deu certo:** nada falhou. **Observação:** os tempos de `go test` variam muito entre rodadas (`doctext`: 24,8 s sem `-race`, 180 s com), sem relação com a mudança. **Pendência para a S2:** a folga de 16/14 bytes é mínima, qualquer campo novo estoura; os tetos sobem pela D14-11. **Artefatos:** só este plano (não rastreado até o commit abaixo); nenhum `.go`, `TOOLS.md` ou arquivo de `D:\MCP` alterado. **Parada deliberada:** S2 a S8 são implementação, teste, revisão, documentação, implantação e tag; **não foram iniciadas**, por ordem do owner. A S0 (aprovação final do plano) foi implícita no pedido de executar a S1, a confirmar.
- 2026-11-07 00:50: **questões 7, 8 e 9 fechadas** (D14-11, D14-12, D14-13) e plano alinhado após a leitura da resposta do GPT-6, que concordou com as três e apontou a inconsistência do `seed=0`. **Alterado:** §4.2 e D14-3 (intervalo `1` a `2147483647`; `0` recusado); S3 (`seed=1` e `2147483647` aceitos; `0`, `-0`, `-1` e `2147483648` recusados; `-0` saiu da lista de aceitos, porque com mínimo 1 passa a ser recusado); S5 (observação exploratória, sem gate; o caso (e) deixou de testar `seed=0`); riscos do `seed=0` e da semântica; S0; §8. **Ressalva do GPT-6 adotada:** a S5 não afirma que o servidor aplicou o `seed`; a prova do envio é a S3. **Estado:** continua rascunho; falta a aprovação final do owner para a S1. Nada foi implementado nem commitado.
- 2026-11-07 00:40: **revisão completa por subagente Fable 5.1** (somente leitura, 62 consultas, 455 s; escolhido por pedido do owner, análise adversarial de um plano que vai para o binário compartilhado). **Veredito: aprovável após ajustes; a arquitetura está certa e o gateway não precisa mudar.** Os fatos 2.1 a 2.10 foram **confirmados**. **Incorporado:** (C1, crítico) estouro do orçamento de bytes do `tools/list`, fatos 11 e 12, D14-11 e S1/S2; (I1) o `/monitor` não mostra `options`, fato 13 e S5; (I2) não existe harness tool → proxy, D14-13 e S3; (I3) desenho estatístico da S5; (I4) `seed=0` ambíguo, fato 14 e D14-12; (I5) `TOOLS.md` regenerado na S2; (I6) referência corrigida para `04-tools.md:181`; menores: `samplingOptions` 38-51, D14-5 e risco do `chat` encerrados (`chat.go:113` já reaproveita), caso decimal = `1.5`, `1.0`/`1e3`/`-0` aceitos, `.exe~` na S7, backup do `04-tools.md` na S6, fato 15 (C-2.10 já tem `seed`). **Conferido por mim antes de gravar:** os tetos 5640/9345 em `registry_test.go:19-20` e o `chat.go:113`. **Não verificado pela revisão nem por mim:** semântica real do `seed` no runner do `qwen3.8:27b`, determinismo da GPU, crescimento exato em bytes (estimado ≥ 58 por tool), ciclo de imports de um harness, e se a produção usa `--tools-verbosity compact`. **Novas questões abertas: 7, 8 e 9 (§8).** Nada foi implementado.
- 2026-11-07 00:28: **questões 1 a 6 da §8 fechadas** a pedido do owner, com as recomendações do plano (0.11.0; só `generate` e `chat`; implantar depois do `BRIEFS`; execução por subagente `sonnet` com revisão `opus`; ramo `feat/seed-generate-chat` sem push; `null` = omitido). As decisões D14-1, D14-2, D14-3 (intervalo, ainda a validar na S5), D14-6 e D14-9 passam de "proposta" a decididas. As questões 4 e 5 foram incluídas pela recomendação do plano, não por resposta explícita do owner. **Estado:** continua rascunho até a aprovação final; nada foi implementado.
- 2026-11-07 00:25: plano ajustado após a revisão do GPT-6, conferida contra o código (nenhum código alterado). **Procedentes e incorporados:** (1) `null` já é tratado como ausente por `args.get`; o §4.2 mandava recusar, o que contradizia o parser; nova D14-9 e questão 6; (2) §1 e §3 dizem que a fase só habilita o `seed`, não implementa rodadas pareadas e **não resolve** o `BRIEFS` (tools de arquivo); nova D14-10; (3) S3 ganhou os limites válidos `0` e `2147483647`, o caso `null` e a prova do `prompt_json` pelo caminho MCP → gateway; (4) S5(d) passou a duas chamadas de `chat` e registra que `[uso]` não prova o corpo; (5) S1 ganhou o comando e as variáveis do build Linux; (6) S7 verifica o backup antes de gravar. **Mantido:** `seedProp()` (há precedente em `thinkProp()` sobre `boolProp`). **Não verificado:** a faixa de `seed` aceita pelo Ollama em uso e o que a documentação oficial promete (fica para a S5).
- 2026-11-07 00:16: plano criado (rascunho, não aprovado). Fatos conferidos no código em `E:\MCPOllam\src` (HEAD `0eb9391`, árvore limpa). Nenhum arquivo do projeto foi alterado além deste.
