# Etapa 04 — Segurança (revisão adversarial do plano da Fase 15)

**Gravado:** 2026-11-13 11:49 (hora real, `date`) · **Autor:** subagente da etapa 04 (Opus 5.5) · **Escopo:** somente leitura. Nenhum acesso ao servidor, nenhum `mcp__mcpOllama__*`, nenhum `git` que escreva, nenhum token impresso. O PLANO não foi editado.
**Alvo:** `E:\MCPOllam\.workspace\plans\MCPOllam2026-11-12-214500-fase15-mcpollama-mcp-linux.plan.md` (citado como "PLANO:linha").

## Fontes lidas

- `E:\MCPOllam\.workspace\reports\MCPOllam2026-11-13-113614-revisao-fase15-plano.etapas\00-indice-e-metodo.md` (inteiro).
- PLANO (177 linhas, inteiro).
- Código: `E:\MCPOllam\src\files\policy.go`, `guard.go`, `nlink_other.go`, `nlink_windows.go`, `guard_test.go:255-330`; `E:\MCPOllam\src\redact\redact.go`; `E:\MCPOllam\src\mcpollama-mcp\server.go:59-90,440-458`, `config.go:67,166-170,211,288,458-470`; `E:\MCPOllam\src\tools\autoreport.go:1-56`, `filetools.go:468-538` (grep); `E:\MCPOllam\src\gateway\content\content.go:1-130`; `E:\MCPOllam\src\gateway\treino\ingest.go` (comentários, grep); `E:\MCPOllam\src\cmd\mcpollama-gateway\config.go:98-117,247-251,378-381`.
- Implantação (cópias do repositório, não o servidor): `E:\MCPOllam\deploy\gateway\mcpollama-gateway.service`, `E:\MCPOllam\deploy\gateway\ollama.conf:1-90`.
- Revisão adversarial da Fase 4: `E:\MCPOllam\.workspace\reports\MCPOllam2026-10-23-2130-revisao-adversarial-fase4-arquivos.report.md` (§1-§6 e início do Apêndice A) e a pasta `...fase4-arquivos.testes\` (listagem e `grep` de `go:build`/`Skip`/`mklink`).
- Recuperados: `...\scratchpad\recuperado\timeline.txt` (inteiro), `achados-complemento.md` (inteiro), `evidence-exec.txt` (linhas 300-395, 460-700, 1100-1180, 1340-1430).
- `D:\MCP\Documentation\Ollama\*.md` (só contagens por `grep`: `--api-key` e CR).
- `E:\MCPOllam\.cursor\mcp.json`, `.vscode\mcp.json`, `.mcp.json`: **só nomes de chaves e booleanos** por script node (`scratchpad\s04-keys.js`), nunca valores.

### Provas executadas nesta etapa (2ª via, locais, sem tocar o repositório)

| # | O quê | Onde | Resultado lido |
| --- | --- | --- | --- |
| P1 | `files.Denied` do próprio pacote do repositório (módulo no scratchpad com `replace` para `E:/MCPOllam/src`) sobre 21 nomes | `scratchpad\s04denycheck\main.go` | `.mcp.json`/`.MCP.JSON`/`mcpollama.env`/`.env.bak`/`.credentials.json`/`cloudflare-origin-ca.pem` → `denied=true`; **`.mcp.json.bak-windows-20261112`, `.mcp.json.bak`, `.mcp.json~`, `mcp.json`, `.cursor/mcp.json`, `.vscode/mcp.json`, `mcp.json.exemplo`, `mcpollama.env.bak-20261112`, `settings.local.json.bak-20261113`, `.claude.json`, `.bashrc`, `.bash_history`, `shadow`, `token`, `settings.json` → `denied=false`** |
| P2 | `files.NewGuard` + `Guard.Read` numa raiz temporária com 3 arquivos contendo um sentinela falso (`FAKE-SENTINEL-NOT-A-TOKEN`) | `scratchpad\s04denycheck\guardread\main.go` | `.mcp.json` → RECUSADO (lista fixa); **`.mcp.json.bak-windows-20261112` → LIDO, 72 bytes, sentinela presente**; **`mcpollama.env.bak-20261112` → LIDO, sentinela presente** |
| P3 | Pacote `files` + os 4 arquivos de ataque da Fase 4, copiados para o scratchpad; `GOOS=linux go vet ./files` e `GOOS=linux CGO_ENABLED=0 go test -c` | `scratchpad\s04src\` | `vet` limpo; binário de teste Linux gerado (5.279.179 bytes). **Não executado**: esta estação não tem WSL (`wsl.exe -l`: "não está instalado") e o servidor é proibido nesta revisão |
| P4 | Os mesmos testes no Windows, código atual (`go test -count=1 -v ./files`) | `scratchpad\s04src\files-attack-win.log` | `ok` (15,6 s); vetores: **208 BLOQUEADO, 0 ESCAPOU**, 2 ACHADO (`deny/gaps`, `cfg/drive-root`), 1 INFO |

---

## Achados

### S-1 — O backup `/root/treino/.mcp.json.bak-windows-20261112` (com o token) está dentro do `--allow-root` e **não** é recusado pela lista fixa

- **Onde no plano:** §9, entrada "22:20" (PLANO:167); §7 Riscos (PLANO:125-138, não há linha para isto); §5 D15-6 (PLANO:105).
- **O que o plano diz:** "backup em `/root/treino/.mcp.json.bak-windows-20261112` (agora `0600`; **contém o token em texto**)" e o novo `.mcp.json` com "`--allow-root /root/treino`" (PLANO:167). Nenhum risco ou pendência associada.
- **O que a evidência mostra:**
  1. Criação: `cp -p .mcp.json .mcp.json.bak-windows-20261112; chmod 0600 …` às 22:02:11 (`evidence-exec.txt:638-639`; `timeline.txt:45`); listagem `-rw-------. 1 root root 1523 nov 12 20:16 .mcp.json.bak-windows-20261112` (`evidence-exec.txt:673`). O conteúdo é o `.mcp.json` do Windows com o token como argumento `--api-key` (`evidence-exec.txt:628`, mascarado).
  2. Raiz configurada: `--allow-root /root/treino` (`evidence-exec.txt:644,662`).
  3. Lista fixa: `deniedName` compara o segmento **inteiro** depois de `ToLower` e `TrimRight(". ")` (`policy.go:77-79,101-118`); `.mcp.json` só é negado por igualdade exata (`policy.go:105`). O nome `.mcp.json.bak-windows-20261112` não casa com nenhuma regra (não é igual a `.mcp.json`, não começa nem termina com `.env`, não termina em `.pem/.key/.pfx/.p12`, não contém `credential`/`secret`). **Prova P1 e P2: lido.**
  4. Demais barreiras não o bloqueiam: arquivo regular, 1.523 bytes (abaixo de 4 MiB, `evidence-exec.txt:382`), `nlink` 1 (o `cp` cria inode novo; o guard só recusa `n != 1`, `guard.go:407`), UTF-8/JSON (passa no `decode`, `guard.go:533-545`), o MCP roda como root (lê `0600 root`).
  5. Caminho do token se for lido: `summarize_file`/`extract`/`translate` mandam `f.Text` ao modelo (`filetools.go:468-538`); o binário avisa isso na partida ("the text of files under the roots is sent to the Ollama server", `evidence-exec.txt:382`). **O `redact` só se aplica à resposta da tool e ao log**, nunca ao prompt (`server.go:453-456`; `redact.go:12-24`; nenhum uso de `redact` em `src/tools`).
  6. Retenção no gateway: `--log-content` padrão `true` e `--retention-content` padrão 168 h = **7 dias** (`cmd/mcpollama-gateway/config.go:104-107,117,247,251,378,381`; `content.go:15,43`); a unit do repositório não muda esses padrões (`deploy/gateway/mcpollama-gateway.service:21-49`). O `content.db` é `0600` do usuário `mcpgw` (`content.go:10`) e o texto é servido pelo `/monitor` **ao administrador logado** (`content.go:16`), na LAN e no loopback (`ollama.conf:74-83`); root do servidor também lê o arquivo.
  7. Agravante: um `reportar_erro` sobre esse job copia a amostra do pedido para o `treino.db` (`ingest.go:439-466`, comentários), que **não tem retenção por tempo** (só apagamento pelo admin; `treino/state.go:194-195`, `ingest.go:605`). O `--auto-report` está **desligado** por padrão (`config.go:67,211`) e o `.mcp.json` de `/root/treino` não o liga (`evidence-exec.txt:644`).
  8. Volta ao Claude: a resposta da tool passa pelo `redact` com o token **atual** (`config.go:458-459`); o token do `.bak` é o mesmo (owner, 21:59:35: "o token do ollama será o mesmo", `timeline.txt:35`), então só uma cópia **exata** é mascarada; o modelo reformatando (espaços, quebra, parcial) escapa do `redact` e entra na transcrição do Claude Code.
  9. `/root/treino/.mcp.json` (0600, token em `env`) **é recusado** pela lista fixa (igualdade exata, `policy.go:105`; P1 e P2). `/root/MCP/mcpollama.env` está **fora** da raiz (`within` por segmentos, `policy.go:204-213`; raiz única `/root/treino`) **e** seria negado pelo sufixo `.env` (`policy.go:107`; P1).
  10. Não houve leitura do `.bak` pelas tools nas provas da sessão (nenhum comando o cita em `evidence-exec.txt`). Estado atual do arquivo: não reverificado (a revisão não acessou o servidor).
- **Classificação:** **confirmado** pelo código e por 2ª via (P1/P2, mesmo código puro que roda no Linux); existência do arquivo confirmada em 2026-11-12 22:02 (não reverificada em 2026-11-13).
- **Gravidade:** **crítico**.
- **Reparo proposto (texto pronto):**
  - Na §9, entrada "22:20": só o marcador `**[errata E-n]**` logo após "(agora `0600`; **contém o token em texto**)", com a errata na §9.1: "E-n: o backup com o token ficou **dentro** do `--allow-root /root/treino`; a lista fixa do guard nega só o nome exato `.mcp.json` (`src/files/policy.go:105`), então `.mcp.json.bak-windows-20261112` é lido pelas tools de arquivo (prova local em 2026-11-13, revisão etapa 04, P1/P2). O risco não foi registrado na §7."
  - Na §7, nova linha: "| Arquivo com segredo dentro do `--allow-root` sob nome fora da lista fixa (ex.: `/root/treino/.mcp.json.bak-windows-20261112`, criado 2026-11-12 22:02) | `summarize_file`/`extract`/`translate` mandam o token ao modelo; o prompt fica 7 dias no `content.db` do gateway (visível ao admin no `/monitor`) e, se houver `reportar_erro`, no `treino.db` sem prazo; a resposta só é mascarada se o token vier idêntico | questão 13 (S-1); regra: nenhum backup de arquivo com segredo dentro de raiz permitida; varredura de nomes `*.bak*`/`*mcp.json*` nas raízes antes da S6 (gate) |".
  - Na §6.2 (pendências): "P-n: decidir o destino de `/root/treino/.mcp.json.bak-windows-20261112` (questão 13); depois, conferir por leitura `ls -la /root/treino/.mcp.json*` (não reverificado em 2026-11-13)."
  - Na §8, **questão nova (numerar na 06 a partir de 13)**: "**Backup com token dentro da raiz permitida.** `/root/treino/.mcp.json.bak-windows-20261112` (0600, root) guarda o token e é legível pelas tools de arquivo. (a) **Mover para `/root/MCP/backups/` (0700) com `0600` (Recomendado)**: tira da raiz e preserva o original do Windows; impacto: nenhum serviço usa o `.bak`; rollback: `mv` de volta. (b) Apagar (`shred -u`): elimina a cópia; impacto: perde o `.mcp.json` original do Windows (que tem caminhos `D:\`/`E:\` e o mesmo token, sem valor de uso no Linux); sem rollback. (c) Manter como está: o token pode ir ao modelo e ao `content.db` por um pedido de resumo de rotina (ex.: 'resuma os arquivos da raiz'); risco aceito pelo owner. Em qualquer opção: confirmar por `ls` e registrar hora."

### S-2 — A S4b manda criar `.bak-<data>` de cada arquivo tocado em `/root/treino`, inclusive o `.mcp.json`: o plano reproduz a brecha do S-1

- **Onde no plano:** §6, S4b (PLANO:119).
- **O que o plano diz:** "(b) `/root/treino`: trocar … **apenas nos arquivos de configuração e instrução** (`.cursor`, `.claude`, `.agents`, comandos, `.mcp.json`) … **backup `.bak-<data>` de cada arquivo tocado**".
- **O que a evidência mostra:** o `.bak` ao lado fica dentro de `/root/treino`; `.mcp.json.bak-<data>`, `settings.local.json.bak-<data>`, `mcpollama.env.bak-<data>` e qualquer `*.json.bak` **não** são negados (P1). Os alvos citados incluem `.cursor` (onde o padrão é `.cursor/mcp.json`) e `.mcp.json`, os dois com token no padrão da CSL (S-3).
- **Classificação:** **confirmado** (texto do plano + código + P1/P2).
- **Gravidade:** **crítico** (instrução do próprio plano que recria um arquivo com segredo legível pelas tools).
- **Reparo proposto (texto pronto):** acrescentar ao fim da célula da S4b, sem apagar o texto: "**superado em 2026-11-13 (revisão):** os backups **não** ficam dentro de `/root/treino` nem de outra raiz do `--allow-root`; vão para `/root/MCP/backups/<data>/<caminho relativo>` (pasta `0700`, arquivos `0600`), com a lista de origem→destino registrada. Gate extra: `find /root/treino \( -name '*.bak*' -o -name '*mcp.json*' -o -name '*.env*' \) -newer <marca>` lista só os previstos, e nenhum arquivo com segredo fica na raiz além do `.mcp.json` (negado pela lista fixa)."

### S-3 — A lista fixa não nega `mcp.json` sem ponto (`.cursor/mcp.json`, `.vscode/mcp.json`), padrão da CSL com o token em `--api-key`

- **Onde no plano:** S4b (PLANO:119, cita `.cursor`); §4.3 (PLANO:90, "Qualquer mudança em `src/files/` exige rodar de novo os testes de ataque"); §8 sem questão sobre a lista fixa.
- **O que o plano diz:** nada sobre a cobertura da lista fixa para os arquivos de configuração MCP.
- **O que a evidência mostra:** `deniedName` só tem `.mcp.json` e `settings.local.json` exatos (`policy.go:105`); P1: `mcp.json`, `.cursor/mcp.json`, `.vscode/mcp.json` → `denied=false`. Na estação Windows, `E:\MCPOllam\.cursor\mcp.json` e `E:\MCPOllam\.vscode\mcp.json` existem e têm `--api-key` nos `args` (script de chaves, `hasApiKeyArg=true`), e a raiz do `.mcp.json` deste projeto é `E:\MCPOllam` (`00-indice-e-metodo.md:32`). Em `/root/treino` existem `.claude` e `.agents` (`evidence-exec.txt:606,630-632`); a presença de `.cursor/mcp.json` lá não foi verificada. A revisão da Fase 4 já listou lacunas (SEC-M-03, `...fase4-arquivos.report.md:129-150`) e a P-10 do owner fixou "lista fixa sem override".
- **Classificação:** **confirmado** (código + P1); presença em `/root/treino`: **não verificável sem o servidor**.
- **Gravidade:** **importante** (no Linux depende de existir `.cursor/mcp.json` em `/root/treino`; na estação Windows o arquivo existe, ver "Fora do escopo").
- **Reparo proposto:** na S4b, acrescentar ao inventário: "conferir se há `mcp.json`/`*.mcp.json*`/`*.bak*` com segredo sob `/root/treino` (só nomes e contagem, sem imprimir conteúdo); se houver, mesma decisão da questão 13". Questão nova para o owner (é decisão P-10, muda código, então vira item candidato da S2 e não reparo do plano): "**Ampliar a lista fixa do guard** para `mcp.json`, `*.mcp.json*`, `*.bak*` de nomes negados (ex.: `.mcp.json.bak*`, `.env*.bak`) e as lacunas do SEC-M-03? (a) **Sim, numa mudança própria com os testes de ataque da Fase 4 no Windows e no Linux (Recomendado)**; impacto: arquivos com esses nomes passam a ser recusados (ex.: um `notas.bak` legítimo não, só os derivados de nomes negados); rollback: reverter o commit. (b) Não, manter a P-10 como está e tratar só por procedimento (S-1/S-2). (c) Só documentar."

### S-4 — Token em três lugares no Linux; D15-6 não foi implementada como escrita; o `mcpollama.env` não é lido por nada

- **Onde no plano:** §5 D15-6 (PLANO:105); §7 "Token em `ps`" (PLANO:133); questão 8 (PLANO:149); S4b(a) (PLANO:119); entradas "21:58" (PLANO:173), "22:05" (PLANO:171), "22:15" (PLANO:169), "22:20" (PLANO:167).
- **O que o plano diz:** D15-6 "Token por `OLLAMA_API_KEY` no ambiente do cliente (arquivo `0600`) … proposta, questão 8"; questão 8 "`OLLAMA_API_KEY` em arquivo `0600` **carregado pelo cliente**"; S4b(a) "token por `OLLAMA_API_KEY` (`mcpollama.env`, `0600`, **sem CR**)"; "22:20": "token em `env.OLLAMA_API_KEY` copiado de `/root/MCP/mcpollama.env` dentro do servidor".
- **O que a evidência mostra:**
  1. Cópias do mesmo token no servidor: `/root/MCP/mcpollama.env` (0600; `evidence-exec.txt:550,578-583`), `/root/treino/.mcp.json` em `env` **literal** (0600; `evidence-exec.txt:640-647,666-667`) e o `.bak` do S-1 em `--api-key` (`evidence-exec.txt:628`). Fora do servidor: `E:\MCPOllam\.mcp.json` (fonte da cópia, `evidence-exec.txt:542-548`) e, com o mesmo token, `.cursor/mcp.json` e `.vscode/mcp.json` desta estação (script de chaves).
  2. **Nada consome o `mcpollama.env` em tempo de execução**: o Claude Code passa ao processo filho o bloco `env` do `.mcp.json` (valor literal); o `mcpollama-mcp` lê `OLLAMA_API_KEY` do ambiente (fallback da flag, PLANO:39). O `.env` só serviu de origem da cópia. O `mcp.json.exemplo` diz `"<TOKEN: vem de /root/MCP/mcpollama.env>"` (`evidence-exec.txt:518`), o que induz a pensar que o cliente o carrega.
  3. O Claude Code aceita expansão `${VAR}` nos campos do `.mcp.json` (`command`, `args`, `env`, `url`, `headers`); isso é documentação pública do Claude Code, **não verificada nesta etapa para a versão 2.1.321 do servidor**. Mesmo com expansão, a variável teria de estar no ambiente do `claude` (ex.: exportada no `.bashrc`, um 4º lugar).
  4. Rotação: o nginx guarda o Bearer num arquivo próprio "para rotação" (`ollama.conf:22-23`). Trocar o token exige atualizar `.mcp.json` (efetivo), `mcpollama.env` (decorativo) e os arquivos da estação; o `.bak` guarda o antigo para sempre. Divergência silenciosa: editar só o `.env` não muda nada.
  5. O objetivo "não aparecer em `ps`" **foi cumprido**: o token não está nos `args` (`evidence-exec.txt:683`, `claude mcp list` sem token).
  6. Decisão do owner existente: questão 8 respondida às 21:54:48 ("ok, 1 e 2", lida como 1ª opção, `timeline.txt:19`; PLANO:173). O owner decidiu "arquivo 0600"; **não** decidiu o mecanismo de carga, e o `--api-key-file` foi recusado em 2026-10-26 (`CLAUDE.md`, seção Configuração).
- **Classificação:** **confirmado** (1, 2, 4, 5 pelos registros de execução e pelo código); 3 **não verificável sem o servidor**.
- **Gravidade:** **importante**.
- **Reparo proposto (texto pronto):**
  - D15-6, coluna Estado: acrescentar "**superado em 2026-11-13 (revisão):** decidida pelo owner em 2026-11-12 21:54:48 ('ok, 1 e 2'); **implementada parcialmente**: o token está em `env.OLLAMA_API_KEY` **literal** no `/root/treino/.mcp.json` (0600), fora dos `args` (não aparece em `ps`); o `/root/MCP/mcpollama.env` (0600) é só a origem da cópia, nenhum processo o lê. Mecanismo de carga em aberto: questão 14."
  - §7, linha "Token em `ps`/`cmdline`", acrescentar na mitigação: "**superado em 2026-11-13 (revisão):** cumprido para `ps`; risco novo: token em 3 arquivos no servidor (`.mcp.json`, `mcpollama.env`, `.bak`) e 3 na estação; divergência na rotação. Ver questão 14 e S-1."
  - Questão nova (14): "**Como o cliente recebe o token no Linux?** (a) **Script de partida `/root/MCP/mcpollama-mcp.sh` (0700) que carrega `/root/MCP/mcpollama.env` e faz `exec /root/MCP/mcpollama-mcp "$@"`; o `.mcp.json` passa a apontar `command` para o script e perde o bloco `env` (Recomendado)**: token em **um** arquivo; sem mudança de código; impacto: o `.mcp.json` muda (backup fora da raiz, S-2), o servidor MCP precisa ser reaprovado no Claude Code se o comando mudar; rollback: voltar o `.mcp.json` anterior. (b) `env: {"OLLAMA_API_KEY": "${OLLAMA_API_KEY}"}` com a variável exportada no shell do root: token sai do `.mcp.json`, mas vai para o `.bashrc` e para o ambiente de todo processo do shell; depende da expansão na versão do Claude Code (conferir antes). (c) Manter o valor literal no `.mcp.json` (0600) e **apagar** o `mcpollama.env` para ter um lugar só (o exemplo deixa de citar o `.env`). (d) Flag `--api-key-file` no código: contraria a decisão do owner de 2026-10-26; só com nova decisão."
  - `mcp.json.exemplo` (entra na S4): o texto do valor deixa de dizer "vem de `/root/MCP/mcpollama.env`" e passa a seguir a opção escolhida na questão 14.

### S-5 — Testes de ataque da Fase 4 nunca rodaram no Linux: o que o código garante, o que só o Linux prova, e o gate

- **Onde no plano:** §3.B (PLANO:60); §4.3 (PLANO:90); S2 (PLANO:116); §7 "`src/files/` (guard)" (PLANO:131); entrada "22:45" validação (PLANO:163).
- **O que o plano diz:** "repetir os testes de ataque de `src/files/` se ele for tocado" (S2) e "Testes com caminhos, permissões, symlink e `nlink` têm comportamento diferente no Linux" (3.B). A entrada "22:45" registra o `go test` dos pacotes no Linux (`files` com 1 FAIL de teste, `TestSplitRoots`), mas **não** os testes de ataque.
- **O que a evidência mostra:**
  - Ataques da Fase 4 (`...fase4-arquivos.report.md:20,227-268`; arquivos em `...fase4-arquivos.testes\`): travessia (42 vetores, `..` em todas as grafias, URL-encoded, fullwidth, `~`, 30× `..`); sintaxe especial do Windows (33: UNC, `\\?\`, `\\.\`, ADS); dispositivos DOS (24); aliases da lista fixa (40: caixa, ponto/espaço final, Unicode, variantes de `.git`); nomes 8.3 (18); symlinks (14) e junctions (9); hardlink (SEC-C-01, corrigido com `linkCount`); diretório case-sensitive; TOCTOU (3 corridas); tamanho e encoding; caminhos longos; map-reduce (1.350 combinações); vazamento ponta a ponta; configuração do `--allow-root` (SEC-M-02, SEC-m-02/03/04/06). NÃO TESTÁVEIS em 2026-10-23: **TOCTOU com FIFO no Linux (SEC-m-08)**, hardlink entre volumes, Ollama real (`...report.md:32-36`).
  - Hoje no Windows (P4): 208 BLOQUEADO, **0 ESCAPOU**. Para Linux (P3): compila e o `vet` passa; **não executado**. Vários vetores são só Windows e pulam no Linux (`files_attack_test.go:68-69`, `files_attack_extra_test.go:35-36`, `files_attack_windows_test.go:1`, `mklink`), então a suíte não traz vetores próprios do Linux.
  - Análise por vetor no Linux, pelo código:

| Vetor (Linux) | O que o código faz | Confirmado pelo código? | Só o Linux prova |
| --- | --- | --- | --- |
| Symlink dentro da raiz → fora (ex.: `/etc/shadow`, `/proc/self/environ`) | `EvalSymlinks` resolve o alvo real (`guard.go:353`), contenção por segmentos na raiz resolvida (`guard.go:359-367`, `policy.go:204-213`), abertura por `os.OpenRoot`/`rt.Open` (`guard.go:379-394`), `SameFile` e 2º `EvalSymlinks` (`guard.go:401,418-420`) | **sim** (recusa opaca `NotAllowed`) | execução real (ordem dos passos e semântica do `os.Root` no kernel Linux) |
| Symlink dentro da raiz → dentro | lido se o destino não estiver na lista (2ª checagem no nome resolvido, `guard.go:368-370`) | sim | — |
| `..` (relativo e absoluto) | `filepath.Clean`/`Join` e contenção (`guard.go:482-493`) | sim; **medido no Linux** em 2026-11-12 22:14 (`/root/treino/../../etc/hosts` recusado, PLANO:163) | — |
| Hardlink criado por root para arquivo fora da raiz (mesmo FS; `fs.protected_hardlinks` não limita root) | `linkCount` lê `Stat_t.Nlink` (`nlink_other.go:12-22`); qualquer `n != 1` → `NotAllowed` (`guard.go:407-409`). O alvo passa a ter `nlink=2` | **sim** | execução real; arquivos legítimos com hardlink (ex.: corpus extraído de `.tar` com links) passam a ser recusados (fail-closed, mensagem opaca) |
| **Bind mount** dentro da raiz (`mount --bind /etc /root/treino/x`, exige root) | invisível ao `Lstat`/`EvalSymlinks`; `nlink` do alvo segue 1; `shadow` não está na lista → **lido** | **sim, como brecha residual** (não é tratada) | o teste como root; o ponto é de modelo de ameaça (S-7): quem monta já é root |
| FIFO no lugar do arquivo | `rt.Stat` → não regular → recusa (`guard.go:388-390`) | sim | — |
| Troca para FIFO entre `rt.Stat` e `rt.Open` (SEC-m-08) | `rt.Open(rel)` sem `O_NONBLOCK` (`guard.go:394`) bloqueia até haver escritor | **brecha aberta pelo código** (não corrigida desde 2026-10-23) | só o teste com `syscall.Mkfifo` no Linux confirma o travamento |
| Dispositivo (`mknod` dentro da raiz; symlink para `/dev/*`) | não regular → recusa; symlink resolve fora → `NotAllowed` | sim | — |
| `/proc` | fora de `/root/treino`; symlink para `/proc/...` resolve fora | sim | — |
| Caixa (Linux diferencia) | lista fixa compara em minúsculas (`policy.go:77-79,94`): `.MCP.JSON` também é negado (mais restritivo); contenção usa `filepath.Rel`, sensível a caixa no Linux (correto) | sim | — |
| Ponto/espaço final, `\` no nome (nomes válidos no Linux) | `normComponent` apara e `components` parte em `/` **e** `\` (`policy.go:69-79`): só gera recusas a mais, nenhuma fuga | sim | — |
| Raiz larga no Linux | `BroadRoots` marca só `/` e o home (`/root`); `DefaultBroadDirs` só lê variáveis do Windows (`guard.go:170-179,207`): `/etc`, `/proc`, `/sys`, `/var`, `/usr` **não** geram aviso. Com `--allow-root /proc`, `/proc/self/environ` é arquivo regular de tamanho 0 e seria lido (ambiente do MCP: `OLLAMA_API_KEY` e, se herdado, o `CLAUDE_CODE_OAUTH_TOKEN`, S-9) | sim (erro de configuração, não fuga) | leitura real de `/proc` (tamanho 0 + `LimitReader`) |

- **Classificação:** comportamento por vetor **confirmado pelo código** como na tabela; execução **não verificável sem um Linux** (nem WSL nesta estação).
- **Gravidade:** **importante** (barreira de segurança das tools de arquivo, exigida pelo `CLAUDE.md` do projeto e pela §7 do plano).
- **Reparo proposto (texto pronto):**
  - S2, acrescentar ao fim da célula (sem apagar): "**superado em 2026-11-13 (revisão):** os testes de ataque da Fase 4 (`.workspace/reports/MCPOllam2026-10-23-2130-revisao-adversarial-fase4-arquivos.testes/`) rodam **no Linux** mesmo sem mudança em `src/files/`, porque nunca rodaram lá: compilar com `GOOS=linux CGO_ENABLED=0 go test -c` (compilação conferida em 2026-11-13, revisão etapa 04) e executar fora da produção (contêiner/VM) ou, com autorização, num diretório temporário do `lx-ai01`. Somar vetores Linux: hardlink de root para fora da raiz, symlink para `/proc/self/environ`, FIFO na troca `Stat`→`Open` (SEC-m-08, `syscall.Mkfifo`), `mknod`, bind mount (resultado esperado hoje: lido, risco residual documentado), nomes `.mcp.json.bak*`/`mcp.json` (esperado hoje: lidos, ver questão 13/S-3)."
  - Gate da S2: "linhas `VEC`: 0 ESCAPOU nos vetores existentes, no Windows **e** no Linux; cada vetor Linux novo com resultado lido e classificado; log guardado em `.workspace/reports/`."
  - Gate da S6 (acrescentar): "antes do 1º uso real das tools de arquivo no Linux: S2-ataques verde no Linux **ou** risco aceito pelo owner por escrito na §8".
  - §6.2: "P-n: SEC-m-08 (FIFO) segue aberto desde 2026-10-23; decidir se vira correção na S2."
  - Documentação Linux (S4): "não usar como raiz `/`, `/root`, `/etc`, `/proc`, `/sys`, `/var`, `/usr`, `/boot`; o aviso de raiz larga do binário só cobre `/` e o home."

### S-6 — Root em produção: o guard não é a barreira quando o cliente (Claude Code) roda como root com shell; questão 9 conflita com `/root/MCP`

- **Onde no plano:** §7 "Rodar como root" (PLANO:132); questão 9 (PLANO:150); §2 item 4 (PLANO:35); S6 (PLANO:121).
- **O que o plano diz:** "`--allow-root` só em pastas específicas; avaliar usuário sem privilégio (questão 9)"; questão 9 "rodar como `root` … ou criar um usuário sem privilégio".
- **O que a evidência mostra:**
  1. `/root` é `dr-xr-x---` (0550) e `/root/MCP` é `0700` (`evidence-exec.txt:321-322,386-387`): **outro usuário não alcança** o binário, o cache, a CA nem o `.env`. A opção "usuário sem privilégio" da questão 9 é incompatível com a árvore em `/root/MCP` sem mudar caminho ou permissões (o pedido literal do owner foi `/root/MCP`, `timeline.txt:2`).
  2. O cliente do teste é o Claude Code **como root** em `/root/treino`, no próprio servidor de produção (`evidence-exec.txt:1104,1126,1151`). Com as tools de shell/leitura do próprio Claude Code, o agente lê qualquer arquivo do sistema; o guard do MCP só controla o que vai ao modelo **local**. O `/root/treino` guarda um corpus de documentos de terceiros (`evidence-exec.txt:595-605`), ou seja, entrada não confiável para prompt injection num agente root em produção.
  3. Bind mount e hardlink (S-5) só são criáveis por quem já tem root, o que confirma que o risco está no cliente, não no guard.
  4. O rótulo no `/monitor` será `ia/root` ou similar (PLANO:35; hostname = `ia.cslsolucoes.com.br`, `evidence-exec.txt:1415`).
- **Classificação:** 1 e 2 **confirmados** (registros da sessão de 2026-11-12, não reverificados); modo de permissão do Claude Code no servidor: **não verificável sem o servidor**.
- **Gravidade:** **importante**.
- **Reparo proposto (texto pronto):**
  - §7, linha "Rodar como root", acrescentar: "**superado em 2026-11-13 (revisão):** o risco maior não é o MCP, é o **cliente**: Claude Code como root no servidor de produção, com shell, sobre um corpus de terceiros (`/root/treino`); o guard não protege contra as tools do próprio cliente. A questão 9 conflita com `/root/MCP` (0700 sob `/root` 0550): um usuário sem privilégio exige mover a árvore (ex.: `/opt/mcpollama`) ou mudar permissões, o que contraria o pedido literal do owner."
  - Questão nova (15): "**Com que usuário e permissões o Claude Code roda no `lx-ai01`?** (a) **Manter root só para os testes da Fase 15, com o Claude Code em modo que pede aprovação para comandos de shell (sem `--dangerously-skip-permissions`) e `/root/treino` sem documentos não confiáveis novos (Recomendado agora)**; impacto: nenhum serviço muda; reversão: n/a. (b) Usuário dedicado (`mcpuser`) com o MCP em `/opt/mcpollama` (0750, grupo próprio) e a CA/`.env` legíveis só por ele; `/root/MCP` fica como espelho do `D:\MCP` sem uso: impacto: nova árvore e novo `.mcp.json`, rótulo no `/monitor` muda; reversão: apagar o usuário e a árvore. (c) Root sem restrição (como está): aceitar que um agente root em produção possa ser induzido por conteúdo de arquivo."

### S-7 — `/etc/hosts` alterado em produção sem item de risco nem rollback na §7; dependências não conferidas

- **Onde no plano:** entrada "22:05" (PLANO:171); §7 (PLANO:135, só o split-DNS); S4b(a) (PLANO:119, "URL pública resolvendo para `127.0.0.1` pelo `/etc/hosts`").
- **O que o plano diz:** "`/etc/hosts` com a linha `127.0.0.1 ia.cslsolucoes.com.br` (backup `/etc/hosts.bak-20261112`; … reverter = apagar as 2 linhas ou restaurar o backup)"; "conferi que só o `server_name` do nginx e o script `ollama-install-cert` citam o nome, nenhum como cliente".
- **O que a evidência mostra:**
  1. Autorização: owner 21:57:20 "o servidor linux é o mesmo ia.cslsolucoes.com.br = 127.0.0.1" e 21:58:32 "faça os ajustes no linux, coloque no plano tudo que concordamos aqui" (`timeline.txt:26,29`); aplicado às 21:58:45 (`timeline.txt:31`).
  2. Conferência feita: `grep -rIl` em `/etc/nginx`, `/etc/systemd/system`, `/usr/local/bin`, `/etc/cloudflared`, `/root/.cloudflared` (`evidence-exec.txt:466-478`). **A origem do túnel não foi conferida**: o `cloudflared` roda com `--token-file` (túnel gerenciado no painel da Cloudflare; `/etc/cloudflared` só tem `token`, `evidence-exec.txt:1416-1420`), então o `ingress`/serviço de origem está no painel e o `grep` de YAML voltou vazio (`evidence-exec.txt:473`). Se a origem for `https://ia.cslsolucoes.com.br:8443`, ela agora resolve para `127.0.0.1`; o nginx escuta `8443` em todo IPv4 (`ollama.conf:27`), então **provavelmente** segue funcionando, mas nenhum teste pelo caminho público foi feito depois da mudança nem depois do reboot (só `systemctl is-active`, `evidence-exec.txt:1166,1202`).
  3. **O hostname da máquina é o próprio nome** (`hostname=ia.cslsolucoes.com.br`, `evidence-exec.txt:1415`): tudo que resolve o FQDN local passa a obter `127.0.0.1` em IPv4 (`hostname -i`, scripts, `fail2ban` `ignoreself`, journald/rsyslog remoto, postfix, Kerberos/SSSD se o servidor estiver no AD, o próprio `ollama-install-cert`). O `getent hosts` também devolveu IPv6 global e link-local para o nome (`evidence-exec.txt:495-497`): clientes que preferem IPv6 vão para o endereço global, onde o nginx não escuta 8443 (só `[::1]:8443`, `ollama.conf:28`) e dependem de fallback para IPv4.
  4. Para o nginx, o MCP local chega de `127.0.0.1` **sem** `CF-Connecting-IP`: a chave de limite é `127.0.0.1` (`ollama.conf:12-21`), fora de `10.100.2.0/24` (sem prioridade no gateway, `--priority-ips 10.100.2.0/24`, `mcpollama-gateway.service:29`), e o `/monitor` aceita loopback (`ollama.conf:80-82`). Um processo local que envie `CF-Connecting-IP` escolhe o IP de limite (`ollama.conf:19`): só root/serviços locais, risco baixo.
  5. Rollback descrito ("restaurar o backup") perde qualquer mudança posterior em `/etc/hosts` e deixa o MCP local sem caminho válido (o certificado só vale pelo nome).
- **Classificação:** 1, 2 (lacuna), 3 (hostname, IPv6) e 4 **confirmados** pelos registros e pelas cópias do repositório; efeito real em cloudflared/AD/serviços: **não verificável sem o servidor**.
- **Gravidade:** **importante** (mudança de sistema em produção sem linha de risco).
- **Reparo proposto (texto pronto):**
  - §7, nova linha: "| `/etc/hosts` do `lx-ai01` com `127.0.0.1 ia.cslsolucoes.com.br` (2026-11-12 21:58:45, por ordem do owner 21:57:20/21:58:32), sendo esse nome também o **hostname** da máquina | tudo que resolve o FQDN local passa a obter 127.0.0.1 (scripts, `hostname -i`, fail2ban, logs remotos, AD/Kerberos se houver); a origem do túnel Cloudflare é configurada no painel e **não** foi conferida; o MCP local entra no nginx/gateway como `127.0.0.1` (sem prioridade da LAN) | P-n: testar o caminho público de fora da LAN (esperado 401 sem token) e confirmar no painel o serviço de origem do túnel; `getent ahosts` e `hostname -i` registrados; rollback cirúrgico abaixo |".
  - §7.1 Rollback: "`/etc/hosts`: `diff /etc/hosts.bak-20261112 /etc/hosts` primeiro; se só houver as 3 linhas da Fase 15 (linha vazia, comentário `# MCP local (Fase 15, 2026-11-12)…` e a entrada), apagar só essas 3 linhas; restaurar o backup inteiro só se o `diff` mostrar apenas elas. Efeito: o MCP no servidor deixa de alcançar o nginx pelo nome (precisa de outra URL decidida pelo owner, questão 10)."
  - §6.2: "P-n: conferir dependências do nome que o `grep` de 2026-11-12 não cobre (`/etc/sssd`, `/etc/krb5.conf`, `/etc/postfix`, `/etc/rsyslog*`, `/etc/fail2ban`, crontabs) e a origem do túnel no painel Cloudflare; somente leitura; não reverificado em 2026-11-13."

### S-8 — `CLAUDE_CODE_OAUTH_TOKEN` exportado no `/root/.bashrc` (válido desde 22:47:12): o plano não registra o estado atual nem o risco

- **Onde no plano:** entrada "22:35" (PLANO:159) e "22:50" (PLANO:161); §7 sem linha.
- **O que o plano diz:** "o servidor tem um **login salvo válido** (`/root/.claude/.credentials.json`) e **também** `CLAUDE_CODE_OAUTH_TOKEN` exportado no `/root/.bashrc`; a variável tem precedência e está inválida … **não a removi, por falta de autorização**".
- **O que a evidência mostra:**
  1. O owner gravou a variável ele mesmo: `.bashrc` alterado às 22:25:13 (token inválido, 401) e às 22:47:12 (token de 108 caracteres, formato válido, `claude -p` → `ok` às 22:56:42) (`evidence-exec.txt:1355-1358,1380-1383`; `timeline.txt:99-100,104-105`). Motivo literal do owner, 22:34:29: "como, acabei de gerar, para não ter que fazer toda hora como no windows" (`timeline.txt:100`). **Nada disso está no plano** (último `Edit` 22:30:24).
  2. Coexistem duas credenciais: `.credentials.json` (0600, `evidence-exec.txt:1119`) e a variável (precedência da variável, PLANO:159).
  3. `/root` é 0550 (`evidence-exec.txt:322`): a leitura do `.bashrc` por outro usuário está bloqueada pelo diretório, **qualquer que seja o modo do arquivo** (o modo `0644` do `.bashrc` **não** foi lido em nenhum registro).
  4. Riscos de um token OAuth de longa duração exportado no shell: vai para o **ambiente de todo processo** iniciado pelo shell do root, inclusive o `claude`, os servidores MCP que ele inicia (o `mcpollama-mcp` herda o ambiente; o caso `--allow-root /proc` do S-5 o exporia ao modelo) e qualquer comando que um agente rode; um `env`/`printenv` num agente manda o valor para a transcrição (a sessão de execução imprimiu 13 caracteres de um token inválido às 22:34:49, `achados-complemento.md:14`); pode estar no `.bash_history` se foi digitado; vai junto em backups de `/root`.
- **Classificação:** 1-3 **confirmados** (registros de 2026-11-12, não reverificados); modo do arquivo e histórico: **não verificáveis sem o servidor**.
- **Gravidade:** **importante** (decisão do owner já tomada; a revisão só registra risco e opções).
- **Reparo proposto (texto pronto):**
  - Na §0.3/§6.2 (subseções da moldura): "2026-11-12 22:25:13 e 22:47:12 — o owner gravou `CLAUDE_CODE_OAUTH_TOKEN` no `/root/.bashrc` (motivo literal, 22:34:29: 'como, acabei de gerar, para não ter que fazer toda hora como no windows'); 22:56:42 — `claude -p` em shell de login → `ok`; o servidor MCP de `/root/treino` segue 'Pending approval'. A ressalva (1) da entrada '22:35' está **resolvida pelo owner** (superada em 2026-11-13 (revisão)). Não reverificado em 2026-11-13."
  - §7, nova linha: "| Token OAuth do Claude Code (longa duração) exportado no `/root/.bashrc` | herdado por todo processo do shell do root (cliente, servidores MCP, comandos de agentes); um `env` num agente o leva à transcrição; convive com `.credentials.json` | questão 16 |".
  - Questão nova (16): "**Onde fica a credencial do Claude Code no servidor?** (a) **Manter a variável, mas num arquivo próprio `0600` (ex.: `/root/.config/claude-oauth.env`) carregado só por um alias/função `claude`, e não exportado para todo o shell (Recomendado)**: menos processos herdam o valor; impacto: abrir `claude` pelo alias; reversão: voltar a linha ao `.bashrc`. (b) Remover a variável e usar só o login salvo (`.credentials.json`, que já respondia `ok` às 22:22): uma credencial só; impacto: o login pode expirar e pedir `claude` interativo de novo (o motivo do owner). (c) Manter como está, aceitando a herança do valor por todos os processos do shell. Em todas: nunca rodar `env`/`printenv` sem filtro numa sessão de agente."

### S-9 — CRLF: a documentação copiada para `/root/MCP/Documentation/Ollama` é CRLF; o plano não tem gate para isso

- **Onde no plano:** S4 (PLANO:118) e S4b (PLANO:119); entrada "22:15" (PLANO:169, lição do CR).
- **O que o plano diz:** "**Lição para a documentação Linux (S4):** nunca gerar o arquivo de ambiente a partir do Windows sem tirar o CR"; S4b(a) "`mcpollama.env`, `0600`, **sem CR**". Nenhum gate mensurável.
- **O que a evidência mostra:** o token entrou com `\r` (65 caracteres) e quebrou o cabeçalho `Authorization` (`evidence-exec.txt:550,573`); corrigido por `sed -i 's/\r$//'` (`evidence-exec.txt:578-583`). Os 8 `.md` de `D:\MCP\Documentation\Ollama` têm CR em todas as linhas contadas (de 52 a 264 por arquivo, contagem local de 2026-11-13) e foram copiados por `tar` sem conversão (`evidence-exec.txt:305`); logo os de `/root/MCP/Documentation/Ollama` são, por inferência, CRLF (não conferido no servidor). Comando copiado de um `.md` CRLF para o bash leva `\r` para valores (o mesmo defeito do token).
- **Classificação:** CR no Windows **confirmado**; CR no servidor **não verificável sem o servidor** (inferência forte: cópia byte a byte).
- **Gravidade:** **importante**.
- **Reparo proposto (texto pronto):** gate novo na S4 e na S4b: "`grep -c $'\r'` = 0 em cada arquivo gerado para o Linux (`Documentation/Ollama/*.md`, `mcp.json.exemplo`, `mcpollama.env`, `.mcp.json`, scripts) **e** `file <arquivo>` sem 'with CRLF line terminators'; para o `.env`, também `awk -F= '/^OLLAMA_API_KEY=/{print length($2)}'` igual ao tamanho esperado do token (sem imprimir o valor). Conversão com `sed -i 's/\r$//'` ou `dos2unix`, com backup fora da raiz (S-2)."

### S-10 — A documentação Windows em `/root/MCP/Documentation` ensina `--api-key` nos `args`, o oposto da D15-6

- **Onde no plano:** entrada "21:55" (PLANO:175, "escritos para Windows"); §2 item 6 (PLANO:37); S4 (PLANO:118).
- **O que o plano diz:** a documentação foi copiada "**escritos para Windows**" e a versão Linux é tarefa da S4.
- **O que a evidência mostra:** contagem local de `--api-key` em `D:\MCP\Documentation\Ollama`: `02-mcp-json-do-zero.md` 10, `01`, `03`, `06`, `07` e `README.md` 1 cada. Essa cópia está no servidor desde 21:51 (`evidence-exec.txt:327-338`). Um agente no servidor que siga esses documentos recoloca o token em `args` (visível em `ps`) e usa `;` e flags de OCR inválidos no Linux.
- **Classificação:** **confirmado** (contagem local; cópia confirmada pelos registros).
- **Gravidade:** **informativo** (até a S4 ser feita).
- **Reparo proposto:** §7, linha nova curta: "| Documentação Windows em `/root/MCP/Documentation/Ollama` desde 2026-11-12 21:51 | quem a seguir no Linux põe o token nos `args` (`ps`) e usa `;`/OCR | até a S4: arquivo `LEIA-ANTES.md` (ou nota no `README.md` do servidor) dizendo 'documentação do Windows; não usar no Linux', **ou** retirar a cópia (decisão do owner na questão 7) |".

### S-11 — Token passado como argumento de processo durante a instalação (22:02:11)

- **Onde no plano:** entrada "22:20" (PLANO:167: "copiado de `/root/MCP/mcpollama.env` **dentro do servidor, sem imprimir**").
- **O que a evidência mostra:** o comando fez `python3 - "$TOK"` e `sed "s/$TOK/<TOKEN>/"` (`evidence-exec.txt:640-648`): durante a execução, o valor esteve em `/proc/<pid>/cmdline`, legível por qualquer usuário local (ex.: `mcpgw`, `ollama`) se `/proc` não usa `hidepid`. Não foi impresso (correto), mas não foi "sem exposição".
- **Classificação:** **confirmado** (comando registrado); configuração de `hidepid`: não verificável.
- **Gravidade:** **informativo** (janela de milissegundos, servidor com poucos usuários).
- **Reparo proposto:** no procedimento da S5/S4b: "segredo nunca vai como argumento de comando (nem `python3 - "$TOK"`, nem `sed s/$TOK/`); passar por stdin ou por arquivo `0600`, e mascarar com regex de formato, não com o próprio valor."

### S-12 — `/root/treino` tem `.git` e o `.mcp.json` não está no `.gitignore`

- **Onde no plano:** entrada "22:20" (PLANO:167); §8 sem questão.
- **O que a evidência mostra:** `ls -la /root/treino/.git | head -1` → "total 4076" (diretório existe, `evidence-exec.txt:626,633`); `git -C /root/treino check-ignore -q .mcp.json` → "(sem repo git ou nao ignorado)" (`evidence-exec.txt:649,674`). Se for repositório, `.mcp.json` e o `.bak` (ambos com token) podem ser commitados por um `git add -A`. O `.mcp.json` original estava `0644` desde 20:16 (`evidence-exec.txt:607`); se veio de um repositório no Windows (`E:\Treinamento`, `evidence-exec.txt:628`), o token pode já estar no histórico.
- **Classificação:** **não verificável sem o servidor** (se é repositório válido, se tem remoto, se o histórico tem o token).
- **Gravidade:** **importante** (vazamento por commit é permanente).
- **Reparo proposto:** §6.2: "P-n (somente leitura): `git -C /root/treino rev-parse --is-inside-work-tree`, `git -C /root/treino remote -v`, `git -C /root/treino check-ignore -v .mcp.json`, `git -C /root/treino log --all --oneline -- .mcp.json | head`; se for repositório, questão ao owner sobre `.gitignore` com `.mcp.json` e `*.bak*`, e sobre o histórico."

### S-13 — Metadados do token no plano ("64 caracteres") e a máscara por formato

- **Onde no plano:** entradas "22:15" (PLANO:169: "65 caracteres", "64 caracteres") e "22:20" (PLANO:167: "(64 caracteres)").
- **O que a evidência mostra:** os comandos mascaram com `[A-Fa-f0-9]\{64\}` (`evidence-exec.txt:679`), o que revela também o alfabeto (hex). Tamanho e alfabeto de um segredo de 256 bits não reduzem o esforço de força bruta de forma útil. O hash parcial da CA (`01d12a7c…d8f3`) e o do binário são públicos.
- **Classificação:** **confirmado**.
- **Gravidade:** **informativo** (aceitável).
- **Reparo proposto:** nenhum no texto existente. Regra para as próximas entradas: "nunca registrar prefixo, sufixo nem trecho do valor; tamanho é aceitável".

### S-14 — `VERSION` e `mcp.json.exemplo`

- **Onde no plano:** §4.1 (PLANO:76-77); entrada "21:55" (PLANO:175).
- **O que a evidência mostra:** `VERSION` tem versão-hash **e** `sha256` do binário (`evidence-exec.txt:345-349,393-395`): a hipótese "VERSION sem hash" é **refutada**. O texto diz "copiado de `D:\MCP\mcpollama-mcp`" (caminho Windows, só informativo). `mcp.json.exemplo` (0600, placeholders, sem token, `evidence-exec.txt:507-524`) está fora de qualquer raiz; o único problema é o texto "vem de `/root/MCP/mcpollama.env`" (S-4).
- **Classificação:** VERSION **refutado** como brecha; exemplo **confirmado** (texto enganoso).
- **Gravidade:** **informativo**.
- **Reparo proposto:** coberto pelo S-4 (texto do exemplo).

---

## Não verificado nesta etapa

1. Estado atual no servidor (a revisão não acessou o `lx-ai01`): existência e modo do `.bak` (S-1), conteúdo de `/root/treino/.cursor`, CR na documentação copiada (S-9), modo do `/root/.bashrc` e `.bash_history` (S-8), `git` em `/root/treino` (S-12), `hidepid` de `/proc` (S-11).
2. Origem do túnel Cloudflare (painel) e dependências do FQDN fora do `grep` de 2026-11-12 (S-7).
3. Expansão `${VAR}` no `.mcp.json` do Claude Code 2.1.321 (S-4 item 3).
4. Execução dos testes de ataque no Linux (S-5): só compilação e `vet` (P3). Esta estação não tem WSL.
5. Configuração real do gateway em produção (`--log-content`, retenção): a evidência é a unit e os padrões do código no repositório, não o servidor (S-1 item 6).
6. Se o Ollama do servidor registra prompts em log (ex.: `OLLAMA_DEBUG`).

## Fora do escopo do plano (só reportar)

1. **Estação Windows, mesma brecha da lista fixa (S-3):** `E:\MCPOllam\.cursor\mcp.json` e `E:\MCPOllam\.vscode\mcp.json` têm o token em `--api-key` (nomes de chaves lidos por script) e ficam dentro do `--allow-root E:\MCPOllam` deste projeto; o nome `mcp.json` não é negado (P1). Um `summarize_file` desses caminhos mandaria o token ao modelo e ao `content.db`. Reportar ao owner; não corrigir aqui.
2. **`E:\MCPOllam\.playwright-mcp\`** não rastreado e **não** ignorado (`git check-ignore` sem saída): `console-2026-11-08T21-50-04-052Z.log` (905.444 bytes), `mon-0134-1400.png` (captura do `/monitor`) e `page-…yml`. Conteúdo não lido; risco de commit acidental de dados do `/monitor`. Sugestão: `.gitignore` ou apagar, por decisão do owner.
3. **`…\scratchpad\recuperado\evidence-exec.txt:1370`** contém, sem máscara, o trecho de 13 caracteres do token OAuth **inválido** impresso às 22:34:49 (o mesmo que o índice manda não reproduzir; **não reproduzido aqui**). O arquivo é insumo desta revisão; sugerir ao coordenador redigir a linha ou apagar o arquivo ao fim.
4. Para a etapa 03: a afirmação do plano "o `--help` (`config.go:144`) e a documentação dizem só `;`" (PLANO:163) é contrariada em parte por `D:\MCP\Documentation\Ollama\03-parametros.md:47`, que já diz "separadas por `;` (`:` no Linux)".
5. Código (candidatos de S2, decisão do owner, não reparo do plano): SEC-m-08 (abrir com `O_NONBLOCK` no Unix); `DefaultBroadDirs` sem diretórios de sistema do Linux; ampliação da lista fixa (S-3); `redact` só por igualdade exata.

---

## Verificação adversarial (opus, 2026-11-13 11:58)

**Verificador:** Opus 5.5, somente leitura. Nada de `ssh`/`scp`/`curl`, nada de `mcp__mcpOllama__*`, nenhum `git` que escreva, PLANO não editado. O texto do analista acima **não foi alterado**.

### Fontes relidas pelo verificador (fonte primária, não a citação)

- PLANO inteiro (177 linhas); `00-indice-e-metodo.md` inteiro; `timeline.txt` inteiro (118 linhas).
- Código: `src/files/policy.go` (inteiro), `src/files/guard.go:160-546`, `src/files/nlink_other.go`, `src/mcpollama-mcp/server.go:440-459`, `src/mcpollama-mcp/config.go` (grep `auto-report`, `secrets()`), `src/gateway/content/content.go:1-60`, `src/cmd/mcpollama-gateway/config.go` (grep `content`/`retention`), `src/gateway/treino/state.go:190-198`, `ingest.go:436-468,600-608`, `treino.go:65,112`, `src/tools/filetools.go` (grep `f.Text`, nenhum `redact` em `src/tools`).
- Implantação (cópias do repositório): `deploy/gateway/mcpollama-gateway.service:15-55` (há `MCPGW_TREINO_DB`; não há `--content-db`, que cai no padrão `content.db` ao lado do `--db`, `config.go:431-432`), `deploy/gateway/ollama.conf:10-30,70-90`.
- `evidence-exec.txt` linhas 300-330, 380-395, 460-525, 540-690, 1100-1240, 1340-1363, 1377-1430, lidas por script node com máscara (`scratchpad\v04\lines2.js`); **linhas 1364-1376 deliberadamente não lidas** (contêm o trecho do token OAuth inválido).
- `D:\MCP\Documentation\Ollama\*.md`: contagem de CR, linhas e `--api-key` (bash `grep -c`).
- `.cursor/mcp.json` e `.vscode/mcp.json`: só booleanos por script (`scratchpad\v04\keys.js`); `git check-ignore -v`.

### Provas próprias (2ª via, independentes das P1-P4 do analista)

| # | O quê | Resultado lido |
| --- | --- | --- |
| V1 | `files.NewGuard` + `Guard.Read` (código do repositório via `replace`), raiz temporária, sentinela falso `FAKE-SENTINEL-V04`, programa novo em `scratchpad\v04\proof\main.go` | `.mcp.json` → RECUSADO (lista fixa); **`.mcp.json.bak-windows-20261112`, `.cursor/mcp.json`, `mcpollama.env.bak-20261112`, `settings.local.json.bak-20261113` → LIDOS**; arquivo com bytes NUL no formato de `/proc/<pid>/environ` (`OLLAMA_API_KEY=…\0PATH=…\0`) → **RECUSADO** ("arquivo binário recusado (contém bytes nulos)", `guard.go:537-538`) |
| V2 | Booleanos dos `mcp.json` da estação | `.cursor/mcp.json` e `.vscode/mcp.json`: `hasApiKeyArg=true`, valor não vazio, sem `env`; ambos ignorados pelo git (`.gitignore:37,39`) |
| V3 | Log P4 do analista (`scratchpad\s04src\files-attack-win.log`) | `ESCAPOU` = 0 ocorrências; `PASS`/`ok … 15.564s`; **há um `SKIP: TestDenylistWindowsShortName`** que o analista não citou (S-V-8) |
| V4 | CR na documentação Windows | 8 arquivos `.md`, CR em 100% das linhas (95/264/145/245/83/87/88/52); `--api-key`: 02=10, 01/03/06/07/README=1, 04/05=0 (bate com S-9 e S-10) |

### Tabela de vereditos

| Achado | Veredito | Evidência conferida pelo verificador | Ajuste do reparo |
| --- | --- | --- | --- |
| S-1 | **CONFIRMADO** (crítico mantido) | `policy.go:105` (igualdade exata); V1; `cp -p` cria inode novo e `guard.go:407` só recusa `n != 1`; `evidence-exec.txt:639,673` (`.bak` 1523 bytes, 0600); `:644,662` (`--allow-root /root/treino`); `server.go:453-456` (redact só na saída); `config.go:247,378,381` + `content.go:15-16,43` (7 dias, texto só ao admin logado); unit com `MCPGW_TREINO_DB` (o `treino.db` existe em produção pela unit do repositório) | (1) **Não pôr marcador `[errata E-n]` na entrada "22:20"**: a frase "contém o token em texto" é verdadeira; o defeito é **omissão de risco**, não erro. A moldura (itens 3-4) reserva errata/marcador para frase falsa. Registrar a omissão na nova entrada do changelog da revisão, na §7 e na §6.2; se o coordenador quiser uma E-n, que seja "E-n (omissão)" na §9.1 **sem** marcador no corpo antigo. (2) Somar ao efeito: o `treino.db` tem a ação `exportou` (`treino.go:65,112`), ou seja, uma amostra com o token pode sair para a base de treino exportada (S-V-3). (3) Precisar a exposição real: o modelo é local (mesma máquina); as cópias duráveis são o `content.db` (7 dias) e o `treino.db` (sem prazo, exportável); a saída para fora (transcrição Anthropic) ocorre quando o modelo devolve o token alterado, o que o `redact` não pega. |
| S-2 | **CONFIRMADO** (crítico mantido) | PLANO:119 ("backup `.bak-<data>` de cada arquivo tocado", alvos `.cursor`, `.claude`, `.mcp.json`); V1 (`*.bak*` lidos) | O destino `/root/MCP/backups/…` é decisão do owner (moldura 8): escrever "fora de qualquer raiz do `--allow-root`, **no destino escolhido na questão do S-1**" em vez de fixar o caminho. O gate por nome deve vir junto da varredura por conteúdo do S-V-2. |
| S-3 | **CONFIRMADO** (código + V1 + V2); presença em `/root/treino`: **NÃO VERIFICÁVEL SEM O SERVIDOR** | `policy.go:105`; V1 (`.cursor/mcp.json` LIDO); V2 | Sem ajuste de fundo. A estação Windows (Fora do escopo 1) é exposição **viva hoje**, ver S-V-9. |
| S-4 | **CONFIRMADO**, com nuance no item 2; item 3 **NÃO VERIFICÁVEL** | `evidence-exec.txt:518` (exemplo "vem de `/root/MCP/mcpollama.env`"); `:640-648` (valor literal em `env`); `:683` (sem token nos `args`) | Item 2 é verdadeiro para o **Claude Code**, mas o `mcpollama.env` **é** lido pelos testes manuais por stdio (`set -a; . /root/MCP/mcpollama.env`, `evidence-exec.txt:555,570,578`). Trocar "nenhum processo o lê" por "o Claude Code não o lê; só os testes manuais por stdio o carregam". |
| S-5 | **CONFIRMADO** no geral; **REFUTADO em parte** (linha "Raiz larga" e o exemplo `/proc/self/environ`) | `guard.go:353-420` (ordem dos passos), `nlink_other.go:12-22`, `guard.go:170-179,207` (raiz larga só `/` e home no Linux), `guard.go:388-394` (FIFO), V1 (NUL recusado) | (1) `/proc/<pid>/environ` e `/proc/<pid>/cmdline` são separados por NUL e o `decode` os recusa (`guard.go:537-538`); **o token não sai por eles**. O risco de `--allow-root /proc` continua para arquivos de texto do `/proc` (`status`, `mountinfo`, `net/*`), não para o ambiente. Corrigir a linha da tabela e a nota da documentação Linux. (2) Os vetores Linux novos **não** devem usar arquivos reais do sistema no `lx-ai01` (hardlink para `/etc/shadow`, `mknod`, `mount --bind`): usar alvos-isca criados pelo teste e, para bind mount e `mknod`, só contêiner/VM (S-V-5). |
| S-6 | **CONFIRMADO** | `evidence-exec.txt:321-322,386-387` (`/root/MCP` 0700, `/root` 0550); `:1104,1126,1151` (Claude Code como root em `/root/treino`) | Sem ajuste. Modo de permissão do Claude Code no servidor: não verificável. |
| S-7 | **CONFIRMADO** (efeito em cloudflared/AD/serviços: **NÃO VERIFICÁVEL SEM O SERVIDOR**) | `evidence-exec.txt:466-478` (grep; YAML do cloudflared vazio); `:490-493` (**3 linhas** acrescentadas: vazia, comentário e entrada; o PLANO:171 diz "as 2 linhas"); `:495-497` (IPv6 global e link-local para o nome); `:1415` (hostname = FQDN); `:1417-1420` (`--token-file`, túnel gerenciado no painel); `ollama.conf:12-21,27-28` | Acrescentar à errata: "reverter = apagar as 2 linhas" (PLANO:171) é impreciso: são 3 linhas (`evidence-exec.txt:490-493`). Aqui **sim** cabe marcador `[errata E-n]` após "apagar as 2 linhas". |
| S-8 | **CONFIRMADO**, com duas correções | `evidence-exec.txt:1112-1117` (**às 22:22:18 a variável já estava no `/root/.bashrc` e no ambiente**); `:1119` (`.credentials.json` 0600); `:1130-1131` (sem a variável → `ok`); `:1138-1139`; `:1355-1358` (22:25:13, 92 caracteres, 401); `:1380-1383` (22:47:12, 108 caracteres, `ok`) | (1) A cronologia tem **três** estados, não dois: valor anterior a 22:22:18 (origem e hora desconhecidas, inválido), gravação de 22:25:13 (92 caracteres, inválido) e de 22:47:12 (108 caracteres, válido). "O owner gravou a variável ele mesmo" só está provado para as duas últimas (inferência da fala de 22:25:23 "pronto, pode fazer o teste" e de 22:34:29 "acabei de gerar"). (2) O parêntese do item 4 sobre "`--allow-root /proc` … o exporia ao modelo" está **refutado** (V1, `guard.go:537-538`); e "o `mcpollama-mcp` herda o ambiente" do Claude Code **não foi verificado** (depende de como o Claude Code monta o ambiente do processo filho). |
| S-9 | **CONFIRMADO** no Windows (V4); no servidor, **NÃO VERIFICÁVEL** (inferência forte: `tar` byte a byte, `evidence-exec.txt:305`) | V4 | Sem ajuste. |
| S-10 | **CONFIRMADO** | V4; `evidence-exec.txt:305,327-330` | Sem ajuste. |
| S-11 | **CONFIRMADO** | `evidence-exec.txt:640-648` (`python3 - "$TOK"` e `sed "s/$TOK/<TOKEN>/"`: o valor esteve em `cmdline`) | Sem ajuste. |
| S-12 | **NÃO VERIFICÁVEL SEM O SERVIDOR** (existência do diretório `.git`: confirmada) | `evidence-exec.txt:626,633` ("total 4076"); `:649,674` (saída ambígua: "sem repo git ou nao ignorado") | Sem ajuste. Nota: o `.mcp.json` com token estava em `/root/treino` desde 20:16, **antes** da sessão (21:44), copiado por terceiro/owner; se veio de um clone, o token pode estar num remoto. |
| S-13 | **CONFIRMADO** (informativo) | PLANO:167,169; `evidence-exec.txt:679` (máscara hex de 64) | Sem ajuste. |
| S-14 | **CONFIRMADO** (VERSION com hash: brecha refutada; exemplo enganoso: confirmado) | `evidence-exec.txt:392-395,507-524` | Sem ajuste. |

**Varredura de segredo no `.md` do analista:** script `scratchpad\v04\scan.js` (padrões `sk-ant`, `Bearer <valor>`, hex ≥ 24, alfanumérico ≥ 30): nenhuma ocorrência de valor; só nomes de arquivos, a flag `--dangerously-skip-permissions` e a palavra "Bearer" sem valor. Metadados presentes (aceitáveis, já no PLANO ou no pedido): 64 e 65 caracteres do token do Ollama, alfabeto hex (S-13), 108 caracteres do token OAuth, hashes parciais públicos da CA e do binário. **Nada redigido.**

**Moldura dos reparos:** ver ajustes na tabela. Problemas de forma: (a) marcador de errata na entrada "22:20" para uma omissão (S-1, ajuste acima); (b) números de questão fixados (14, 15, 16 em S-4, S-6, S-8) quando o índice manda numerar na etapa 06 (S-V-6); (c) destino de backup fixado sem questão (S-2). O resto respeita a moldura (acréscimos com "superado em 2026-11-13 (revisão)", subseções §0.3/§6.2/§7.1/§9.1, decisões viram questão).

### Achados novos do verificador

**S-V-1 — `/proc/<pid>/environ` e `cmdline` não vazam pelas tools (corrige S-5 e S-8).**
- Onde: §4.3 (PLANO:90) e a nota de documentação proposta em S-5. Evidência: `guard.go:537-538` (byte NUL → recusa); V1. Classificação: **confirmado** pelo código e por teste local (a função `decode` é pura, igual no Linux). Gravidade: informativo. Reparo: na nota de raiz larga, escrever "`--allow-root /proc` expõe arquivos de texto do `/proc` (ex.: `status`, `mountinfo`, `net/*`); `environ` e `cmdline` são recusados por conterem bytes nulos".

**S-V-2 — Inventário só por nome não acha segredo em arquivo de nome comum; falta varredura por conteúdo em `/root/treino`.**
- Onde: S4b (PLANO:119) e os gates propostos em S-1/S-2/S-3. Evidência: a lista fixa é por nome (`policy.go:101-118`); `settings.json` (que no Claude Code pode ter bloco `env`), `*.bak*`, `mcp.json` e qualquer `.md` com token colado são lidos (V1 e P1). `/root/treino/.claude` existe com `agents`, `commands`, `plans` (`evidence-exec.txt:630-632`); conteúdo não verificado. Classificação: **confirmado** (código); conteúdo real: **não verificável sem o servidor**. Gravidade: **importante**. Reparo (§6.2, P-n, somente leitura, autorização do owner): "varrer `/root/treino` por **conteúdo**, imprimindo só nomes e contagem, nunca o valor: `grep -rlE '[A-Fa-f0-9]{64}|sk-ant-|OLLAMA_API_KEY=|CLAUDE_CODE_OAUTH_TOKEN' /root/treino --exclude-dir=.git | wc -l` e a lista de nomes; cada arquivo achado vai para a decisão da questão do S-1".

**S-V-3 — O `treino.db` tem exportação: o token numa amostra pode sair para a base de treino.**
- Onde: §7 (sem linha). Evidência: `treino.go:65` (`AcaoExportou`), `:112` (ação `exportou`); unit com `MCPGW_TREINO_DB` (`mcpollama-gateway.service`, bloco Fase 12). Classificação: **confirmado** pelo código e pela unit do repositório (produção não reverificada). Gravidade: importante (reforça o S-1). Reparo: incluir na linha de risco do S-1: "…e, se houver `reportar_erro`, no `treino.db` (sem prazo) e no que for **exportado** dele".

**S-V-4 — A variável `CLAUDE_CODE_OAUTH_TOKEN` já existia antes de 22:25:13.**
- Onde: entradas "22:35" (PLANO:159) e "22:50" (PLANO:161); §0.3 proposta em S-8. Evidência: `evidence-exec.txt:1112-1117` (22:22:18: presente no ambiente e no `/root/.bashrc`), `:1355-1356` (22:25:13, 92 caracteres), `:1380-1381` (22:47:12, 108 caracteres). Classificação: **confirmado**. Gravidade: informativo. Reparo: a linha da §0.3 registra os três estados (valor anterior de origem não registrada; 22:25:13 inválido; 22:47:12 válido), sem atribuir ao owner o primeiro.

**S-V-5 — Os vetores Linux propostos no S-5 não podem mexer em arquivos reais do servidor de produção.**
- Onde: reparo da S2 proposto em S-5 ("ou, com autorização, num diretório temporário do `lx-ai01`"). Evidência: o precedente de 22:17 rodou binários de teste em `/tmp` da produção (`timeline.txt:66-71`); hardlink para `/etc/shadow` cria um 2º nome do arquivo de senhas que fica se o teste cair; `mount --bind` e `mknod` alteram o sistema. Classificação: **confirmado** (texto do reparo proposto). Gravidade: **importante**. Reparo: "no `lx-ai01`, só vetores com alvos-isca criados pelo próprio teste, no mesmo sistema de arquivos e apagados ao fim (conferido por `ls`); hardlink para arquivo real do sistema, `mknod` e bind mount **só em contêiner/VM**; nunca rodar como root a suíte que cria links para fora".

**S-V-6 — Numeração de questões fixada pelo analista pode colidir com outras etapas.**
- Onde: S-4 ("Questão nova (14)"), S-6 ("(15)"), S-8 ("(16)"). Evidência: `00-indice-e-metodo.md:43` (a 06 numera "a partir da 13"). Classificação: confirmado. Gravidade: informativo (moldura). Reparo: tratar como IDs provisórios `Q-04-a…e`; a 06 numera.

**S-V-7 — Errata vs. omissão (moldura).**
- Onde: reparo do S-1. Evidência: `00-indice-e-metodo.md:52-53`. Classificação: confirmado. Gravidade: informativo. Reparo: ver o ajuste do S-1 na tabela; usar marcador de errata só onde há frase falsa (ex.: "apagar as 2 linhas", S-7).

**S-V-8 — A rodada de ataque no Windows tem um `SKIP` (nomes curtos 8.3).**
- Onde: P4 do analista ("208 BLOQUEADO, 0 ESCAPOU"). Evidência: `scratchpad\s04src\files-attack-win.log` (última parte: `--- SKIP: TestDenylistWindowsShortName`). Classificação: confirmado. Gravidade: informativo (não afeta o Linux; o "0 ESCAPOU" não cobre 8.3 neste volume). Reparo: no gate da S2, "registrar os `SKIP` de cada sistema e o motivo".

**S-V-9 — A mesma brecha está viva hoje na estação Windows (fora do escopo, só reportar, gravidade maior que a do Linux).**
- Evidência: V1 (`.cursor/mcp.json` LIDO) e V2 (`hasApiKeyArg=true` nos dois arquivos); `--allow-root E:\MCPOllam` (`00-indice-e-metodo.md:32`). Os arquivos estão fora do git (`.gitignore:37,39`), mas são legíveis pelas tools de arquivo do `mcpOllama` desta estação, que é usado todo dia. Classificação: confirmado. Gravidade: **importante** (fora do escopo do plano: reportar ao owner, não corrigir).

### Perguntas fechadas para o owner geradas por esta etapa (IDs provisórios; a 06 numera a partir da 13)

1. **Q-04-a (S-1, S-2, S-V-3) — Backup com o token dentro de `/root/treino`.** (a) **Mover `/root/treino/.mcp.json.bak-windows-20261112` para fora de qualquer raiz permitida (ex.: `/root/MCP/backups/`, pasta 0700, arquivo 0600) e adotar a regra "nenhum backup de arquivo com segredo dentro de raiz do `--allow-root`" na S4b (Recomendado)**; rollback: `mv` de volta. (b) Apagar o `.bak` (`shred -u`): sem rollback; perde o `.mcp.json` original do Windows (mesmo token, caminhos `D:\`/`E:\`). (c) Manter: aceitar que um pedido de rotina leve o token ao `content.db` (7 dias) e, com `reportar_erro`, ao `treino.db` e às exportações.
2. **Q-04-b (S-V-2) — Varredura por conteúdo em `/root/treino`, somente leitura, imprimindo só nomes e contagem?** (a) **Sim, antes da S4b e antes de qualquer novo uso das tools de arquivo no Linux (Recomendado)**. (b) Só por nome (`*.bak*`, `*mcp.json*`, `*.env*`). (c) Não varrer.
3. **Q-04-c (S-3) — Ampliar a lista fixa do guard** (`mcp.json`, derivados `.bak*` de nomes negados) numa mudança própria com os testes de ataque no Windows e no Linux? (a) **Sim, como item da S2 (Recomendado)**. (b) Não; manter a P-10 e tratar por procedimento. (c) Só documentar.
4. **Q-04-d (S-4) — Como o cliente recebe o token no Linux?** (a) **Script de partida 0700 que carrega `/root/MCP/mcpollama.env` e faz `exec` do binário; `.mcp.json` sem bloco `env` (Recomendado)**. (b) `${OLLAMA_API_KEY}` no `.mcp.json` com a variável exportada no shell (conferir antes se a versão 2.1.321 expande). (c) Manter o valor literal no `.mcp.json` e apagar o `mcpollama.env` (um lugar só). (d) `--api-key-file` no código (contraria a decisão de 2026-10-26).
5. **Q-04-e (S-5, S-V-5) — Onde rodar os testes de ataque no Linux?** (a) **Contêiner ou VM Linux fora da produção, com todos os vetores (Recomendado)**. (b) Diretório temporário do `lx-ai01`, só com alvos-isca, sem `mknod`/bind mount/links para arquivos do sistema, com autorização por rodada. (c) Não rodar e aceitar o risco por escrito na §8 antes da S6.
6. **Q-04-f (S-6) — Usuário do Claude Code no `lx-ai01`.** (a) **Root só para a Fase 15, com aprovação de comandos de shell ligada e sem novos documentos não confiáveis em `/root/treino` (Recomendado agora)**. (b) Usuário dedicado com o MCP em `/opt/mcpollama` (muda caminhos e rótulo). (c) Root sem restrição.
7. **Q-04-g (S-7) — `/etc/hosts` em produção.** (a) **Manter a linha e fazer, somente leitura, a conferência das dependências do FQDN (sssd/krb5/postfix/rsyslog/fail2ban/cron) e da origem do túnel no painel Cloudflare, mais um teste do caminho público de fora da LAN (esperado 401 sem token) (Recomendado)**. (b) Trocar por URL que não dependa do `/etc/hosts` (questão 10 reaberta). (c) Manter sem conferir.
8. **Q-04-h (S-8, S-V-4) — Credencial do Claude Code no servidor.** (a) **Arquivo próprio 0600 carregado só por alias/função `claude`, sem `export` global no `.bashrc` (Recomendado)**. (b) Remover a variável e usar só `.credentials.json`. (c) Manter como está.
9. **Q-04-i (S-12) — `/root/treino` como repositório git:** autorizar a leitura (`rev-parse`, `remote -v`, `check-ignore -v`, `log --all -- .mcp.json`) para saber se o token está em histórico/remoto? (a) **Sim, somente leitura (Recomendado)**. (b) Não.
10. **Q-04-j (S-V-9, fora do escopo) — Estação Windows:** `.cursor/mcp.json` e `.vscode/mcp.json` com o token dentro do `--allow-root E:\MCPOllam`. (a) **Registrar como pendência de outra frente e decidir junto com a Q-04-c (Recomendado)**. (b) Tratar já, em tarefa própria. (c) Aceitar o risco.

### Não verificado pelo verificador

1. Tudo o que depende do servidor (estado atual do `.bak`, `.bashrc`, CR na cópia, `git` em `/root/treino`, `hidepid`, origem do túnel, configuração real do gateway).
2. Se o Claude Code 2.1.321 passa ao processo filho do MCP o ambiente completo do shell (afeta S-8 item 4) e se expande `${VAR}` no `.mcp.json` (S-4 item 3).
3. Execução dos testes de ataque no Linux (sem WSL nesta estação; servidor proibido).
4. Linhas `evidence-exec.txt:1364-1376` (não lidas de propósito: trecho de token OAuth inválido).
