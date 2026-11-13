# Revisão profunda do plano da Fase 15 — índice e método

**Criado:** 2026-11-13 11:38 (hora real, `date`) · **Coordenador:** Claude Code (Opus 5.5) · **Pedido do owner (2026-11-13, literal):**
"ok, faça um revisão do plano @.workspace\plans\MCPOllam2026-11-12-214500-fase15-mcpollama-mcp-linux.plan.md, procure lacunas ou brechas, logica/semantica novamente com profundidade, separando por etapas criando .md até a conclusão completa, sempre olhando o contexto para não extourar, faça os reparos e ajustes necessários no plano"

## Alvo e o que está autorizado

| Item | Valor |
| --- | --- |
| PLANO (único arquivo que será editado) | `E:\MCPOllam\.workspace\plans\MCPOllam2026-11-12-214500-fase15-mcpollama-mcp-linux.plan.md` (177 linhas, 38.601 bytes, igual ao HEAD `7fbe5cc`; `mtime` 2026-11-12 22:30:26) |
| Autorizado | ler o repositório e as transcrições; **editar o PLANO**; gravar os `.md` desta pasta |
| **Não** autorizado | tocar código, servidor (`ssh`/`scp`/`curl`), `CLAUDE.md`, memória, `D:\MCP`; `git add/commit/push/tag`; usar `mcp__mcpOllama__*` (é verificação crítica, não cabe ao nível local); imprimir token ou trecho de token |
| Achados fora do plano | **reportar, não corrigir** (regra global de escopo estrito, 04/11/2026) |
| Estado do git no início | `master` = `origin/master` = `7fbe5cc`; árvore com alterações de **outra sessão** (planos movidos para `plans/concluidos/`, `CLAUDE.md`, `AGENTS.md`, `README.md`): **não tocar, não commitar** |

## Tentativas anteriores (recuperadas; usar como insumo, não como verdade)

| Sessão | Quando | O que ficou |
| --- | --- | --- |
| `5f11d6b4` (a sessão que executou a Fase 15 em 2026-11-12) | 1ª revisão 22:59–23:14; estourou o contexto 2× sem editar o plano | `recuperado/timeline.txt` (linha do tempo real, 118 linhas: hora real de cada prompt do owner, de cada comando e de cada `Edit` do plano), `recuperado/evidence-exec.txt` (saídas reais de 56 comandos, segredos redigidos), `recuperado/evidence.js` |
| `e4503236` | 2026-11-12 23:44 → 2026-11-13 01:22; 6 compactações, só levantamento, plano não editado | `recuperado/achados-complemento.md` (fatos novos de 22:34–22:56, **moldura do reparo**, regras para agentes), `recuperado/timeline-fase15.md` (318 linhas, detalhada), `recuperado/timeline.js`, `tokencheck.js` |

Pasta dos recuperados: `D:\Users\claiton.linhares\AppData\Local\Temp\claude\e--MCPOllam\6bc04315-e87d-4573-bdfc-8bea884542c3\scratchpad\recuperado\`.
Transcrições JSONL (fonte primária, **só por script node que mascare segredos**; nunca `Read`/`cat`/`grep` cru, as linhas têm centenas de KB): `D:\Users\claiton.linhares\.claude\projects\e--MCPOllam\{0855f643-…,5f11d6b4-…}.jsonl`. Os `timestamp` são UTC; hora local = UTC − 3 h (conferido: o commit `f6bc6ab` às 21:53:31 local casa com o comando `git commit` da transcrição às 21:53:30).

## Fatos já confirmados pelo coordenador (2ª via: git)

- Commits que tocaram o plano: `f6bc6ab` (2026-11-12 21:53:31) e `7fbe5cc` (22:32:51). Push de `7fbe5cc` às **22:32:54** (`git reflog show origin/master`), **autorizado** pelo owner às 22:32:37 ("ok, commit tudo e push", `timeline.txt:96`). O commit `f6bc6ab` foi autorizado às 21:53:26 ("sim", `timeline.txt:16`).
- O plano tem 11 entradas de changelog; a linha do tempo mostra os 13 `Write`/`Edit` do plano entre 21:45:51 e 22:30:24 (`timeline.txt:7,14,20,33,40,49-50,55-56,72,76,84,94`). **Nenhuma entrada do changelog tem a hora real**: os carimbos são posteriores à edição (ex.: entrada "22:45" do reinício escrita às 22:30:24; "22:50" escrita às 22:21:19). A própria sessão de execução admitiu às 23:01:13: "Os horários do changelog foram inventados. […] Várias eram horários futuros quando as escrevi."
- Depois do último `Edit` (22:30:24) houve fatos **não registrados no plano**: 22:34–22:35 (token OAuth novo inválido, 401) e 22:56:42 (token OAuth válido, `claude -p` → `ok`, servidor MCP ainda "Pending approval"). Detalhe em `recuperado/achados-complemento.md` §1.
- Toolchain local: go1.27.0 (`go -C E:\MCPOllam\src …` é permitido para `vet`/`list`/`test` sem gravar nada no repositório nem em `D:\MCP`). `D:\MCP\mcpollama-mcp` tem 22.770.050 bytes, 2026-11-07 20:07 (confere com §2 item 2 do plano).
- `.mcp.json` deste projeto: `--allow-root E:\MCPOllam` (só o valor, lido por script).

## Etapas (um `.md` por etapa, nesta pasta; cada um gravado antes da seguinte depender dele)

| # | Arquivo | Pergunta que responde | Analista → verificador adversarial |
| --- | --- | --- | --- |
| 01 | `01-cronologia-e-honestidade.md` | Que hora real tem cada entrada do changelog e cada ação no servidor? Que autorizações literais do owner existem, com hora? O que aconteceu depois do último `Edit` e não está no plano? Quais afirmações do plano a cronologia refuta? | sonnet → opus |
| 02 | `02-coerencia-interna.md` | Onde o plano se contradiz (banner × changelog; `status`; questões "abertas" já respondidas; D15-n "proposta" já decididas ou violadas; §2/§2.1/§3 superados; árvore §4.1 × servidor real; §4.4 × questão 7; etapas sem estado)? | sonnet → opus |
| 03 | `03-fatos-codigo-repositorio.md` | Cada afirmação do plano sobre código, repositório, `D:\MCP` e documentação é verdadeira? (`config.go:144`, `guard.go:120/200/207/223`, `guard_test.go:125`, `ocr_test.go:173`, `policy.go`, registro do `reportar_erro`, `go.mod`, `CHANGELOG.md` 0.10.4?, linha de build Linux do `CLAUDE.md`, separador do `--allowed-models`, referências a outros planos) | sonnet → opus |
| 04 | `04-seguranca.md` | Brechas: `.mcp.json.bak-windows-20261112` com token **dentro** do `--allow-root /root/treino` (a lista fixa do guard recusa esse nome?); token em 3 lugares; testes de ataque da Fase 4 nunca rodados no Linux (symlink, hardlink de root, FIFO, `/proc`); root em produção; `/etc/hosts` fora dos riscos; `CLAUDE_CODE_OAUTH_TOKEN` no `.bashrc`; CRLF | opus → opus |
| 05 | `05-etapas-gates-pronto.md` | As etapas S0–S8/S4b têm estado, ordem, gates mensuráveis, rollback, Definição de Pronto? O que falta na S6 (rótulo, `chat`, `translate`, `reportar_erro`, `.docx/.pptx`), na S3 (`-trimpath` muda o hash com o mesmo rótulo; `file`/`readelf` podem não existir; ETXTBSY), na S4/S4b (tabela de mapeamento, exclusões, 3 cenários de rede), D15-1 × "0.10.4", referências sem caminho? | sonnet → opus |
| 06 | `06-reparos-consolidados.md` | Lista única e numerada de reparos (R-n) com texto pronto para aplicar, tabela de gaps (G-n: crítico/importante/informativo, pergunta fechada, resposta, ação), edge cases, questões novas para o owner (a partir da 13), status sugerido | opus |
| 07 | `07-aplicacao-no-plano.md` | Diário da aplicação dos reparos no PLANO (o que foi aplicado, o que ficou de fora e por quê) | opus (2 editores em sequência; previsto sonnet às 11:38, elevado a opus às 12:31 pelo risco: 46 reparos, corpo literal do changelog) |
| 08 | `08-verificacao-final.md` | `git diff` do PLANO conferido contra o 06 e contra a moldura (sem renumerar, corpo das entradas antigas literal, sem segredo, erratas marcadas) | opus |
| — | `MCPOllam2026-11-13-<HHMMSS>-revisao-fase15-plano.report.md` (pasta `reports/`) | Relatório final no formato da skill `governance-spec-reviewer` (status Aprovada / com ressalvas / Reprovada) | coordenador |

## Moldura do reparo (vale para todos; herdada de `achados-complemento.md` §2 e revista)

1. Alvo único: o PLANO, editado no lugar. **Sem commit** (o owner decide no fim).
2. **Não renumerar** §0–§9 nem S0–S8/S4b, D15-n, questões 1–12, itens A–F da §3: o changelog os cita.
3. **Não condensar nem apagar.** Texto vencido fica, com a marca `**superado em 2026-11-13 (revisão):** <o que vale agora>` ou remissão. O que é falso é corrigido e o erro vai para a errata.
4. **Corpo das entradas antigas do changelog é literal.** Nelas só muda: (a) o carimbo do começo, para `- 2026-11-12 HH:MM:SS (hora real da edição; carimbo original "HH:MM", errata E-n)`; (b) inserção de marcadores `**[errata E-n]**` logo após a frase errada. Nenhuma outra palavra.
5. Seções novas entram como **subseções** das existentes (nomes da rule `workspace-plans-persist_V1.4.0`): `0.1 Resumo`, `0.2 Situação atual` (estado por objetivo e por etapa, artefatos, commits), `0.3 Ações já executadas por ordem do owner` (hora real · ação · onde · autorização literal com hora · evidência · rollback), `1.1 Escopo e exclusões`, `1.2 Critérios de conclusão (Definição de Pronto)`, `4.5 Inventário de artefatos`, `6.1 Resultado dos testes no Linux`, `6.2 Pendências numeradas (P-n)`, `7.1 Rollback`, `9.1 Errata (E-n)`. A §0 ganha um quadro "onde está cada seção exigida pela rule".
6. Tabela da §6 ganha a coluna **Estado** (concluída / parcial / pendente / bloqueada / fora de ordem) sem perder o texto das células.
7. Frontmatter: `status: draft` fica (owner, 21:54:48: "ok, 1 e 2, sem execução do plano"); `updated: 2026-11-13`; `overview` sem "empacotar" (o owner dispensou o empacotamento às 21:50:56).
8. **Decisão do owner não é tomada pela revisão**: vira questão nova na §8 (a partir da 13), com opções (recomendada primeiro), impacto, rollback e a alternativa de não fazer.
9. Toda afirmação sobre o estado do servidor leva data/hora da leitura e a nota "não reverificado na revisão de 2026-11-13, que não acessou o servidor".
10. Achado de produto fora do escopo vai para "fora do escopo: reportar, não corrigir". Não se propõe mudança de código como reparo do plano (pode virar item da S2 ou questão).
11. Citações do owner **literais**, com hora real. **Nenhum segredo** (nem trecho de token, nem `Authorization`). A transcrição contém trechos de um token OAuth **inválido** impressos por erro às 22:34:49 (**20 de 92 caracteres**: 16 do início e os 4 do fim; corrigido em 12:30 pela etapa 01, que refutou o "13" escrito aqui às 11:38): **não reproduzir em lugar nenhum**.
12. Nova entrada no topo do changelog para esta revisão, com hora real de `date` no momento da edição, listando reparos (R-n), erratas (E-n), questões novas e o que NÃO foi feito.

## Regras para os agentes (herdadas e ampliadas)

- **Proibido:** `ssh`, `scp`, `curl`, qualquer acesso a servidor; `mcp__mcpOllama__*`; `git add/commit/push/tag/checkout/reset/stash/mv`; criar/editar arquivo fora desta pasta e do scratchpad (o editor da etapa 07 é a única exceção, e só para o PLANO); ler o conteúdo de `.mcp.json`, `.env`, `.credentials.json` ou qualquer credencial (um fato deles só por script que imprima nomes de chaves, nunca valores); rodar `bootstrap-mirror-symlinks.ps1`; abrir transcrições JSONL com `Read`/`cat`/`grep`.
- **Permitido:** `Read`/`Grep`/`Glob` no repositório e nesta pasta; `git log/show/blame/diff/status/reflog`; `go -C E:\MCPOllam\src list|vet|test` sem `-o` e sem gravar; `node` em scripts gravados no scratchpad; `ls` em `D:\MCP`.
- **Rigor:** só afirmar o que foi lido; citar `arquivo:linha` ou `timeline.txt:linha`; classificar cada achado como **confirmado**, **refutado** ou **não verificável sem o servidor**. Um falso-OK é pior que um FAIL. pt-BR. Gravar o `.md` da etapa antes de devolver o resultado.

## Critérios de conclusão desta revisão

- [ ] 01–05 gravados, cada um com a seção "Verificação adversarial" do opus (confirmado/refutado/não verificável por achado).
- [ ] 06 com reparos R-n prontos para aplicar, gaps G-n classificados, questões novas para o owner.
- [ ] PLANO editado conforme 06 e a moldura; 07 e 08 gravados; `git diff` do PLANO lido pelo coordenador.
- [ ] Relatório final no formato `governance-spec-reviewer`, com status, modelos usados por etapa e achados fora do escopo.
- [ ] Decisões do owner levantadas por `AskUserQuestion` no fim (versão, fonte da documentação, mecanismo do token, `.bak` com token dentro do `--allow-root`, S4b, `.bashrc`, commit desta revisão).

## Estado desta revisão (atualizado pelo coordenador)

- 11:24 — hora real; reconhecimento somente leitura (git, rule `workspace-plans-persist`, scratchpads antigos recuperados para `recuperado/`).
- 11:36 — pasta criada; 11:38 — este índice gravado.
- 11:40 → 12:26 — **Workflow 1 concluído** (`wf_7107cabe-caa`; 11 agentes, 0 erros, 472 chamadas de ferramenta, 45 min): analistas 01/02/03/05 em sonnet e 04 em opus; 5 verificadores adversariais em opus; consolidador em opus. Resultado: 01–05 gravados com a seção "Verificação adversarial" (confirmados 15+24+20+13+…, refutados 1 total e 20 sub-afirmações, novos 8+13+5+9+…); **06** com 49 gaps (5 críticos, 28 importantes, 16 informativos), 46 reparos R-1..R-46, erratas E-1..E-36, riscos RS-1..RS-15, pendências P-1..P-20, questões novas 13..32; **status sugerido: Reprovada** (passa a "Aprovada com ressalvas" com os reparos aplicados e a questão 13 respondida). O PLANO seguiu intacto (igual ao HEAD). Correção à moldura (item 11): 20 caracteres, não 13.
- 12:30 — a sessão principal passou a rodar em **Fable 5.1** (comando `/model fable` do owner). Próximo: workflow 2 (aplicação dos reparos no PLANO por 2 editores opus em sequência → 2 verificadores opus em paralelo → correções → verificação final).
- 12:31 → 13:03 — **Workflow 2 concluído** (`wf_17bebb16-a9c`; 8 agentes opus, 0 erros, 270 chamadas, 34 min): editor 1 aplicou R-1..R-29 (plano 177 → 326 linhas), editor 2 aplicou R-30..R-46 (→ 483 linhas); verificação rodada 1 achou 1 discrepância (R-42 dizia "aplicação em sonnet"; corrigido para "opus", desvio registrado no 07); rodada 2: completude ok, moldura ok, 0 discrepâncias; **08** gravado. Desvios aceitos pelo coordenador: 3 `\|` escapados nas células P-7/P-8 (o texto do 06 partia a tabela), posições dos R-3/R-24/R-25/R-31/R-39 (parágrafo próprio abaixo do título em vez de "fim da linha" do título).
- 13:05 — **prova própria do coordenador** (script node, 2ª via independente dos agentes): 11/11 entradas antigas do changelog com o corpo idêntico ao HEAD depois de remover só o carimbo e os marcadores `**[errata E-n]**`; `git diff --numstat` 385/79; 0 sequências suspeitas de segredo (`sk-ant` só como prefixo de formato); frontmatter `status: draft`, `updated: 2026-11-13`, `overview` sem "empacotar". Lidas as L1–45 e L370–415 do plano novo.
- 13:05 — relatório final gravado: `.workspace/reports/MCPOllam2026-11-13-130538-revisao-fase15-plano.report.md` (status inicial Reprovada, pendente da questão 13).
- 13:10–13:13 — **AskUserQuestion, 2 rodadas (8 perguntas):** 13 = manter (RS-1 aceito); git = commit e push; 19 = só testes, sem versão; 22 = importar para `docs/`; 15 = token no `env` do `.mcp.json`; 17 = manter no `.bashrc` (RS-6 aceito); 18 = não revogar, apagar temporários; 23 = incluir tudo. Respostas gravadas na §8 e no changelog do plano (entrada 13:13:18) e no relatório (status → Aprovada com ressalvas).
- 13:15 — apagados os 4 arquivos temporários com o trecho do token inválido (2 originais nos scratchpads `5f11d6b4` e `e4503236`, 2 cópias em `recuperado/`); os JSONL das transcrições não foram tocados. Próximo: commit por caminho + push, autorizados às 13:10.
