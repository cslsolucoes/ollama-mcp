# Etapa 07 — Diário da aplicação dos reparos no PLANO

**Plano:** `E:\MCPOllam\.workspace\plans\MCPOllam2026-11-12-214500-fase15-mcpollama-mcp-linux.plan.md` · **Fonte dos reparos:** `06-reparos-consolidados.md`, seção (B).

## Editor 1 (opus) — R-1 a R-29

**Hora real:** início 2026-11-13 12:29:37 (`date`, 1ª leitura); fim das edições 12:34:52; diário gravado a partir de 12:35:02. Ferramenta: só `Edit`, ancorada em texto (nunca em número de linha). Sem acesso a servidor, sem `mcp__mcpOllama__*`, sem git que escreva (só `git diff`/`git show`).

### Aplicados (R-n → onde no plano)

| Reparo | Onde ficou no plano (após a edição) | Forma |
| --- | --- | --- |
| R-1 | frontmatter, linhas 4 (`overview`) e 6 (`updated: 2026-11-13`); `status: draft` intacto | troca (moldura item 7); o `overview` original deve ir literal na entrada R-42 (editor 2) |
| R-2 | fim da linha 13 (banner "Estado") | acréscimo na mesma linha: `**[errata E-12]** **superado em 2026-11-13 (revisão):** ...` |
| R-3 | linha 14, nova linha de citação "Estado atual (revisão de 2026-11-13)" | inserida logo abaixo da L13, dentro do mesmo bloco `>` (ver dúvida 1) |
| R-4 | fim da linha 15 ("Origem") | acréscimo na mesma linha (errata E-18) |
| R-5 | fim da linha 16 ("Relacionados") | acréscimo na mesma linha ("Caminhos ...") |
| R-6 | §0, depois do parágrafo original: "Superado em 2026-11-13", "Como ler as horas" e o quadro "Onde está cada seção exigida pela rule" (tabela de 2 colunas) | inserção |
| R-7 | `### 0.1 Resumo`, logo após o quadro do R-6 | inserção |
| R-8 | `### 0.2 Situação atual ...` (tabela por objetivo, 3 colunas; tabela por etapa, 4 colunas; artefatos/commits; registro de aprovação formal) | inserção |
| R-9 | `### 0.3 Ações já executadas por ordem do owner ...` (tabela de 6 colunas, 24 linhas de ação; parágrafo "Conexões SSH") | inserção |
| R-10 | fim do objetivo 2 da §1 | acréscimo na mesma linha |
| R-11 | fim do objetivo 3 da §1 | acréscimo na mesma linha (errata E-22) |
| R-12 | fim do parágrafo "Fora do escopo" da §1 | acréscimo na mesma linha |
| R-13 | `### 1.1 Escopo e exclusões (revisão de 2026-11-13)`, logo após "Fora do escopo" | inserção |
| R-14 | `### 1.2 Critérios de conclusão (Definição de Pronto da Fase 15)`, após a §1.1 (13 itens) | inserção |
| R-15 | fim do §2 item 1 | acréscimo (errata E-32) |
| R-16 | fim do §2 item 2 | acréscimo (errata E-28) |
| R-17 | fim do §2 item 3 | acréscimo (errata E-34) |
| R-18 | fim do §2 item 4 | acréscimo |
| R-19 | fim do §2 item 5 | acréscimo |
| R-20 | fim do §2 item 6 | acréscimo |
| R-21 | fim do §2 item 7 | acréscimo |
| R-22 | fim do §2 item 8 | acréscimo |
| R-23 | §2.1, célula "Situação" das linhas do motor do PDF, do cache, do `modernc`/`x/image` (errata E-33) e do Go | acréscimo antes do `|` que fecha a célula; nº de colunas conferido (igual ao original) |
| R-24 | §3: parágrafo "Superado em 2026-11-13" logo abaixo do título; marcas ao fim dos itens A a F | inserção + acréscimos (ver dúvida 2) |
| R-25 | §4.1, fim do parágrafo "Fora do espelho, de propósito" | acréscimo na mesma linha (ver dúvida 3) |
| R-26 | §4.2, parágrafo novo após o item `SHA256SUMS` | inserção |
| R-27 | §4.3, parágrafo novo após o parágrafo original | inserção |
| R-28 | §4.4, parágrafo novo após o parágrafo original | inserção |
| R-29 | `### 4.5 Inventário de artefatos ...` (tabela de 4 colunas, 20 linhas; "Faltam") antes da §5 | inserção |

### Não aplicados

- Nenhum dos R-1 a R-29 ficou de fora. R-30 em diante são do editor 2 (fora do meu lote, não tocados). Nenhuma marca `[estado em 2026-11-13]` foi posta na §8 (o R-6 as cita, mas elas pertencem a reparos posteriores).

### Dúvidas e leituras conservadoras

1. **R-3:** a convenção geral do 06 manda "linha em branco antes e depois" em "Inserir após a linha N", mas o próprio R-3 diz "nova linha de citação logo abaixo da L13 (antes da L14 'Origem')" e o texto começa com `> `. Uma linha em branco quebraria o bloco de citação em dois. Apliquei sem linha em branco (a linha fica no mesmo bloco `>`), que é a leitura do próprio R-3.
2. **R-24:** o reparo manda "acrescentar ao fim da L57", mas a L57 é o título `## 3. O que NÃO se sabe ...`. Acrescentar na mesma linha transformaria o parágrafo em parte do título (e mudaria o texto do cabeçalho que o changelog cita). Apliquei como parágrafo próprio logo abaixo do título, com linha em branco antes e depois. O título ficou intacto.
3. **R-25:** o cabeçalho diz "(ao fim da L80)" e o texto começa por "**Superado em ...:**" (forma de parágrafo). Segui a convenção do 06 ("Acrescentar ao fim de" = mesma linha, com espaço) e acrescentei na mesma linha do parágrafo "Fora do espelho". Se o verificador preferir parágrafo próprio, é só quebrar a linha; o texto é o mesmo.
4. **Fora do lote, para o coordenador:** a tarefa citou "corpo das 11 entradas antigas intocado salvo o carimbo (R-44)" e "R-45"; nenhum dos dois é do meu lote (R-1 a R-29), por isso a §9 não foi tocada. O R-6 já afirma que "cada entrada antiga começa pela hora real da edição": isso só fica verdadeiro depois que o editor 2 aplicar o R-44.
5. **Referências futuras:** o texto aplicado cita seções e itens que ainda não existem no plano até o editor 2 terminar: §6.1, §6.2 (P-n), §7.1, §9.1 (E-n), RS-n e as questões 13 a 32. É o esperado pela ordem dos reparos; o verificador da etapa 08 deve conferir que todos passaram a existir.
6. Nenhum `<HORA REAL>` aparece nos textos R-1 a R-29; nenhuma hora foi inventada.

### Conferências feitas (saída lida)

- Script `node` no scratchpad (`check07.js`): os **38 blocos de código** de R-1 a R-29 (linhas 121 a 534 do 06) estão **todos** presentes no plano, linha a linha (0 faltando).
- Comparação com o HEAD (`git show HEAD:...` → arquivo no scratchpad): das 177 linhas originais (o script conta 178 por causa da quebra de linha final), só 6 não aparecem idênticas ou como prefixo de uma linha nova: L4 e L6 (frontmatter, troca prevista no R-1) e as 4 linhas da tabela §2.1 (L47, L48, L49, L52), que foram conferidas como prefixo (o texto foi inserido antes do `|` final) com o **mesmo número de colunas** (7 separadores) do original. Nenhuma linha apagada.
- `git diff --numstat` do plano: 176 inserções, 27 remoções (as 27 são as linhas originais modificadas por acréscimo ou pela troca do frontmatter).
- Markdown: cercas de código balanceadas (2, a árvore da §4.1); nenhum cabeçalho duplicado; nenhuma tabela com número de colunas divergente entre linhas (conferência por script, ignorando o conteúdo entre crases).
- Segredos: busca por sequências de 40+ caracteres alfanuméricos, `sk-ant` e `Bearer ` no plano: só nomes de arquivo de planos e relatórios. Nenhum token nem trecho de token foi escrito.

### Tamanho do plano ao fim do editor 1

`wc -l` = **326** linhas (era 177 no HEAD `7fbe5cc`; o script conta 327 por causa da quebra de linha final).

## Editor 2 (opus) — R-30 a R-46

**Hora real:** 1ª leitura 2026-11-13 12:36:02 (`date`); início das edições 12:36:46 (cópias de referência gravadas no scratchpad: `plan-head.md` = `git show HEAD:<plano>`, `plan-after-ed1.md` = plano ao fim do editor 1); hora usada no `<HORA REAL>` do R-42 e do R-43: **12:44:37** (`date` imediatamente antes da inserção); fim das edições e conferências 12:50:46; diário a partir de 12:50:57. Ferramenta: só `Edit`, ancorada em texto (nunca em número de linha). Sem acesso a servidor, sem `mcp__mcpOllama__*`, sem git que escreva (só `git show`/`git diff --numstat`). Ordem seguida: R-30 → R-46.

### Aplicados (R-n → onde no plano)

| Reparo | Onde ficou no plano (após a edição) | Forma |
| --- | --- | --- |
| R-30 | §5, fim da célula "Estado" de D15-1 a D15-7 (antes do `\|` final) | acréscimo; 3 colunas mantidas |
| R-31 | §6, fim do parágrafo "Cada etapa tem um gate; …" | acréscimo na mesma linha (convenção "Acrescentar ao fim de") |
| R-32 | §6, cabeçalho e separador trocados pela versão de 5 colunas; célula Estado acrescentada depois do `\|` final de S0 a S8 (10 linhas) | troca do cabeçalho prevista no próprio R-32 + acréscimos |
| R-33 | §6, fim da célula "O que fazer" de S1, S2, S3, S4b, S5, S6 e S8; marcador `**[errata E-13]**` logo depois de "22:03" na S4b | acréscimos |
| R-34 | `### 6.1 Resultado dos testes no Linux`, depois da tabela da §6 | inserção |
| R-35 | `### 6.2 Pendências numeradas (P-n)`, depois da §6.1 (P-1 a P-20) | inserção (ver dúvida 1: 3 `\|` escapados) |
| R-36 | §7, fim da célula "Mitigação" das 10 linhas originais (errata E-27 na linha "Rodar como root") | acréscimos |
| R-37 | §7, linhas RS-1 a RS-15 ao fim da tabela | inserção (3 colunas) |
| R-38 | `### 7.1 Rollback`, depois da tabela da §7 | inserção |
| R-39 | §8, parágrafo próprio logo abaixo do título `## 8.` | inserção (ver dúvida 2) |
| R-40 | §8, fim das questões 1 a 12 | acréscimos na mesma linha |
| R-41 | §8, parágrafo "Questões novas (revisão de 2026-11-13)" e questões 13 a 32, depois da questão 12 | inserção |
| R-42 | topo da §9, nova entrada `- 2026-11-13 12:44:37 (hora real da edição): …`; `<N>` = 46 | inserção |
| R-43 | logo abaixo da entrada do R-42, `- 2026-11-13 12:44:37 (hora real do registro; …)` | inserção |
| R-44 | §9, começo das 11 entradas antigas (E-1 a E-11), exatamente no formato da tabela do R-44 | troca só do carimbo |
| R-45 | §9, 18 marcadores `**[errata E-n]**` nas âncoras da tabela do R-45 (E-15, E-14, E-31, E-19 ×2, E-16 ×2, E-21, E-25, E-35, E-26, E-18 ×6, E-24, E-20) | inserção de marcador, com um espaço antes |
| R-46 | `### 9.1 Errata (E-n)` ao fim do arquivo (E-1 a E-36 e ressalvas) | inserção |

### Não aplicados

- Nenhum. Os 17 reparos do lote (R-30 a R-46) foram aplicados. O editor 1 relatou R-1 a R-29 todos aplicados; não havia reparo pendente para retomar.

### Dúvidas e leituras conservadoras

1. **R-35, três `|` sem escape dentro de código em células da tabela P-n:** P-7 (`a|b|c`, 2 ocorrências) e P-8 (`ls /tmp/fase15-lt/src/<p>/*.go | wc -l`, 1 ocorrência). No markdown de tabela (GFM) um `|` sem escape parte a célula mesmo dentro de crases, e a linha ficaria com colunas a mais (conteúdo cortado na renderização). O próprio R-35 declara a convenção ("Nas células, `\|` é o caractere `|` escapado") e a usa nas outras células; apliquei o mesmo escape nesses 3 pontos e não mudei mais nada. Leitura do comando inalterada (`\|` = `|`). Se o verificador preferir o texto byte a byte do 06, basta tirar as 3 barras invertidas.
2. **R-39:** o reparo manda "acrescentar ao fim da L140", que é o título `## 8. …`. Para não mudar o cabeçalho, segui o precedente do editor 1 no R-24: parágrafo próprio logo abaixo do título, com linha em branco antes e depois. O título ficou intacto.
3. **R-31:** "acrescentar ao fim da L110"; a L110 é um parágrafo comum, então acrescentei na mesma linha (convenção do 06), como o editor 1 fez no R-25.
4. **R-42, `<N>`:** preenchido com 46 (R-1 a R-29 do editor 1, todos aplicados, + R-30 a R-46 deste editor). A frase "não aplicados e por quê: ver etapa 07" ficou literal.
5. **R-42 e R-43 com a mesma hora (12:44:37):** os dois foram inseridos no mesmo `Edit`, logo depois do `date`; é a hora real das duas edições.
6. **R-46, formatação:** "**Origem.**", "**Método.**" e "**Estado publicado.**" estão em linhas seguidas sem linha em branco no texto do 06; no markdown viram um só parágrafo. Mantive o texto literal (não é erro de conteúdo).
7. **Fora do meu lote, mas conferido:** com o R-44 aplicado, a frase do R-6 ("Agora cada entrada antiga começa pela hora real da edição") passou a ser verdadeira. As referências futuras citadas pelo editor 1 (§6.1, §6.2/P-n, §7.1, §9.1/E-n, RS-n, questões 13 a 32) agora existem todas no plano.

### Conferências feitas (saída lida)

- Script `node` no scratchpad (`check07b.js`): dos **48 blocos de código** do 06 entre as linhas 535 e 993 (175 linhas não vazias), 171 estão no plano linha a linha; as 4 restantes são as esperadas: P-7 e P-8 (escape da dúvida 1, conferidas por `check07c.js`: a linha do 06 com só o escape aplicado está no plano) e as 2 entradas novas da §9 (conferidas: a linha do 06 com `<HORA REAL>` → `12:44:37` e `<N>` → `46` está no plano). As 10 células do R-32 conferidas no fim de cada linha S0…S8.
- **Changelog antigo (R-44 e R-45):** para cada uma das 11 entradas, o script reconstruiu a linha esperada a partir do `git show HEAD:<plano>` (troca do carimbo pelo texto exato do R-44 + inserção de ` **[errata E-n]**` logo depois de cada âncora do R-45, cada âncora única na entrada) e comparou com a linha atual: **11/11 IGUAIS**. Nenhuma outra palavra mudou no corpo das entradas antigas.
- **Linhas do HEAD:** das 177, as que não aparecem idênticas nem como prefixo são: L4 e L6 (frontmatter, editor 1), L47-L52 da §2.1 (editor 1, já conferidas por ele), as 11 do changelog (conferidas acima) e as 24 linhas das tabelas §5, §6 e §7 (L100-L106, L115-L117, L119-L121, L123, L129-L138); para estas 24, `check07c.js` confirmou que a linha do HEAD é preservada inteira e a atual só tem blocos inseridos (1 bloco na §5 e na §7; 2 nas linhas da §6 com R-32 + R-33; 3 na S4b, com o marcador E-13). Nenhuma linha apagada.
- `git diff --numstat` do plano (contra o HEAD, somando os dois editores): 385 inserções, 79 remoções (as remoções são as linhas originais modificadas por acréscimo ou troca, todas preservadas como mostrado acima).
- **Markdown:** cercas de código = 2 (par, só a árvore da §4.1); 26 cabeçalhos, nenhum duplicado; **nenhuma tabela com número de colunas divergente** (contagem de `|` sem escape por linha, em todas as tabelas fora de cerca), depois da dúvida 1.
- **Segredos:** busca por `sk-ant-` seguido de 6+ caracteres, `Bearer ` + 10+ e sequências de 40+ caracteres: só nomes de arquivo de planos/relatórios, o sha256 **público** do binário (questão 20, texto do 06) e o prefixo de formato `sk-ant-oat01-` (texto do R-43, sem nenhum caractere do token). Nenhum token nem trecho de token escrito. Nenhum `<HORA REAL>` nem `<N>` restante.

### Tamanho do plano ao fim do editor 2

`wc -l` = **483** linhas (326 ao fim do editor 1; 177 no HEAD `7fbe5cc`).

## Corretor (opus), rodada 1 — 2026-11-13 12:57:16 (hora real, `date`)

### Aplicados (discrepância → onde no plano)

1. **Discrepância 1 (R-42, erro de fato herdado do 06):** na §9, entrada `- 2026-11-13 12:44:37 (hora real da edição): **revisão profunda do plano…`, frase final "Modelos: …", troquei `aplicação em sonnet` por `aplicação em opus (2 editores em sequência)`. Um único `Edit`, ancorado no texto `verificação adversarial e consolidação em opus; aplicação em sonnet; verificação final em opus.`. Nenhuma outra palavra mudou.

**Desvio intencional do texto literal do 06:** o TEXTO PRONTO do R-42 no 06 diz "aplicação em sonnet" (o 06 foi escrito antes da aplicação, quando o 00, tabela de etapas, previa a etapa 07 em sonnet). O fato real é outro: a aplicação foi feita por 2 editores opus em sequência (este diário, seções "Editor 1 (opus)" e "Editor 2 (opus)"; `00-indice-e-metodo.md:82`). Preferi corrigir o plano em vez de manter a frase errada e abrir errata nova, porque a entrada é **nova** desta revisão (não é uma das 11 entradas antigas protegidas pela moldura, item 4) e uma afirmação falsa no plano é pior que um desvio registrado do 06. A partir de agora, a linha do R-42 no plano **não** é mais igual byte a byte à do 06: quem conferir (etapa 08 ou `check07b.js`) deve esperar essa única diferença.

### Não aplicados

- Nenhum. A discrepância foi conferida antes de editar (07, cabeçalhos das linhas 5 e 68; 00, linha 82) e está correta.

### Dúvidas

- O 00, tabela de etapas (linha 44), ainda diz que a etapa 07 é feita em sonnet; isso é o planejado na hora, não o executado, e o 00 não é alvo desta rodada. Fica para o coordenador decidir se anota.

### Conferências feitas (saída lida)

- `grep -c 'aplicação em sonnet'` no plano = **0**; a frase nova aparece 1 vez.
- `git diff --numstat` do plano: 385 inserções, 79 remoções (igual ao fim do editor 2; a troca ficou dentro de uma linha já inserida).
- `wc -l` do plano = **483** linhas (sem mudança; troca dentro da mesma linha).
