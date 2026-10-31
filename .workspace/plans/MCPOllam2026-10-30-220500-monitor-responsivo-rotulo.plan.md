# Plano — Monitor responsivo, tabelas estáveis, erros visíveis (gateway 0.12.2) e rótulo do cliente (mcpollama 0.9.1)

> **Estado:** código e testes **concluídos e commitados** (sem implantação, sem push, sem tag — pedido do coordenador).
> Implantação do gateway 0.12.2 e do binário 0.9.1: **pendente** (decisão do owner).
> **Arquivo:** `src/gateway/monitor/index.html` (HTML+CSS+JS embutidos; a CSP hasheia o `<style>` e o `<script>` em tempo de
> execução, em `buildCSP`, `monitor.go`; o arquivo segue em LF).

## 1. Evidência (medida antes de mexer)

1. Owner, Firefox 152, janela ~570 px: as `section` ficavam mais largas que a janela; cards de GPU, CPU/RAM e tabelas cortados à
   direita. Reproduzido em Firefox headless a 570 px.
2. Owner, Chrome, `https://10.100.2.2:8443/monitor`, ~720 px, versão 0.12.1: 5º card da linha de cards cortado, GPU 3 cortada,
   coluna "Último erro" do Backends cortada, card Swap cortado, núcleos (threads) saindo pela direita.
3. O SSE chega ~2,4 mensagens/s (cada estado ~160 KB) e `render()` reconstruía TODAS as tabelas (render ~6–9 ms no Firefox): a
   rolagem horizontal dos `.wrap` voltava ao início a cada ~0,4 s e os botões eram trocados entre mousedown e mouseup (cliques
   perdidos: "não responde").
4. `es.onmessage` tinha `try { render(...) } catch (x) {}`: erro de desenho engolido.

Causa do corte: `main{display:grid}` com coluna implícita `auto` (item de grid com `min-width:auto`) + tabelas `white-space:nowrap`
forçavam a coluna a crescer; o `.wrap{overflow-x:auto}` nunca chegava a atuar.

## 2. Etapas

| # | Etapa | Estado |
| --- | --- | --- |
| 1 | CSS responsivo | concluído |
| 2 | `table()` estável (atualização no lugar, chaves `data-k`, `scrollLeft` preservado) e cards GPU/CPU/cards só se o dado muda | concluído |
| 3 | `safeRender` + `console.error` + `#msg` | concluído |
| 4 | Log em tempo real: status final vindo de `recent_done` | concluído |
| 5 | Teste Go de regressão (`responsive_test.go`) | concluído |
| 6 | Prova no Firefox headless (570, 720, 900, 1400; leitor e admin) | concluído |
| 7 | Testes de ataque das revisões 9.5, 10.5 e 10.L | concluído (todos ok) |
| 8 | CHANGELOG, README, MANUAL, este plano | concluído |
| 9 | Commit por caminho | concluído |

## 3. O que foi feito (detalhe)

1. **CSS:** `main{grid-template-columns:minmax(0,1fr)}` e `main>*{min-width:0}`; `section`/`.card` com `min-width:0` e
   `overflow-wrap:anywhere`; grades `.cards`, `.gpus`, `.threads`, `.charts`, `.det .kv` com `minmax(min(<valor>,100%),1fr)`;
   `.det` com coluna `minmax(0,1fr)`; `.tabs` com `flex-wrap`; `@media (max-width:640px)` (paddings 8 px, header empilhado,
   `.card .v` 20 px, campos de login flexíveis). Nenhum `style=` nem `setAttribute('style')` (CSP); `buildCSP` calcula o hash a
   partir do HTML embutido, então continua válido (`TestCSPHashesMatchWhatTheBrowserHashes` ok).
2. **Tabelas:** `table()` calcula a forma (cabeçalhos + número de linhas + chave de cada célula-nó). Se igual à última desenhada,
   só atualiza `textContent`/`className`/`title` das células que mudaram, mantendo os nós de botão. Se mudou, reconstrói e
   restaura o `scrollLeft` do `.wrap`. Chaves: `btn()` → `kind:id`; Detalhes → `det:<id>`; caixa de botões da fila →
   `wbox:<id>:<logado>`. Nó sem chave nunca é reaproveitado. Mudança de login (botão ↔ vazio) muda a forma e reconstrói.
   Estado vazio: o aviso só é recriado se o texto mudou. Cards de métricas, GPUs e CPU usam `sameSig` (JSON do dado).
3. **Erros:** `safeRender` (try/catch → `console.error` + "Erro ao desenhar o painel: <mensagem>" em `#msg`; limpa quando um
   desenho seguinte funciona); usado no `onmessage` (JSON inválido também é reportado), no polling e nos redesenhos do login/ação.
4. **Log em tempo real:** `renderJobs` troca o status de um job `na_fila`/`em_execucao` pelo de `recent_done` (mesma id), com
   `finished_at`, `run_ms` e contadores de tokens.

## 4. O que deu certo / o que não deu

- Certo: a causa do corte confirmada no Firefox; com `minmax(0,1fr)` nenhuma seção passa da borda em 570/720/900/1400 px.
- Errado (processo): a 1ª versão da sonda procurava rolagem em `#joblog`, que está numa aba oculta (`scrollWidth` 0) e dava "sem
  overflow"; trocado por `#running` (visível). A 1ª sonda também não tinha a faixa fixa visível; ajustada.
- Errado (processo): um `python` com `"""..."""` e `\u` em "DOMINIO\usuario" falhou por escape; as edições dos .md foram refeitas
  com a ferramenta Edit.

## 5. Números medidos (Firefox headless, estado real de `state-queue.json`)

| Largura | Conteúdo fora dos `.wrap` passando da borda | `scrollWidth` do documento | `.wrap` que rolam |
| --- | --- | --- | --- |
| 570 (leitor e admin) | 0 | 570 | running, donelist, waiting, backends, minstalled, recent |
| 720 | 0 | 720 | running, donelist, waiting, backends, recent |
| 900 | 0 | 900 | backends, recent |
| 1400 | 0 | 1400 | nenhum |

Estabilidade (sonda, `ff2-test*.html`): 2º `render` com o mesmo estado → mesmo nó de botão (SIM); `scrollLeft` setado em 50 no
`.wrap` de `#running` a 570 px → 50 depois do 2º render, de um render só com tempos mudados e de uma reconstrução por linha
removida (OK); só `running_ms`/`waiting_ms` mudando → mesma `<td>` com texto novo (3,2 s → 4,7 s), mesmo botão Cancelar e mesmo
botão da fila (SIM); `render` com `queue = null` → `#msg` = "Erro ao desenhar o painel: can't access property "slots", q is null",
limpo no render seguinte; job `em_execucao` na lista com a mesma id em `recent_done` → "concluído". Render médio estável: 3,3–4,8 ms.

## 6. Testes

- `go build`, `go vet`, `gofmt -l` limpos; `go test -count=1 ./...` todos `ok` (21 pacotes com teste); pacote `monitor` -v: 51 PASS, 0 FAIL.
- Ataque (cópias temporárias em `src/`, removidas depois): fase 9.5, 10.5 (pool) e 10.L → `cmd`, `gateway/{content,joblog,monitor,pool,proxy,queue,store}`, `ollama`, `tools`: todos `ok`.
- Novo: `src/gateway/monitor/responsive_test.go` (`minmax(0,1fr)`, grades com `min()`, `@media`, `safeRender`, `data-k`, sem `catch` vazio no `onmessage`, sem `style=` inline).

## 7. Pendências

1. Implantar gateway 0.12.2 (`Instalar-Gateway.ps1 -Modo Atualizar`) e compilar/reiniciar o mcpollama 0.9.1; conferir no Chrome do owner a 720 px.
2. `renderDetail` (detalhe do job) ainda reconstrói a caixa a cada ~1 s enquanto o job roda: a rolagem do `pre`/extrato (`.sc`) e o botão Fechar são trocados.
3. `loadSeries` (gráficos) recria os gráficos a cada 5 s (sem interação, só pisca).
4. Provas de arrastar/clicar com o mouse real no Chrome/Firefox não foram feitas (só DOM/rolagem em Firefox headless).
5. O status final na aba de log vem de `recent_done`, que só guarda `--monitor-keep-done`; depois disso vale o do SQLite (que já está final).

## 8. Artefatos

Commit único (ver `git log`): `src/gateway/monitor/index.html`, `src/gateway/monitor/responsive_test.go`, `CHANGELOG.md`, `README.md`,
`docs/MANUAL-GATEWAY.md`, este plano. Provas (fora do git): `...\scratchpad\domshim\m-ff2-test*-{570,720,900,1400}.png`, `sonda.js`, `build.py`.
