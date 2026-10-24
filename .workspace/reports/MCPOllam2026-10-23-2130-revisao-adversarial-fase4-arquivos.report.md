# Revisão adversarial de segurança da Fase 4 (tools de leitura de arquivo)

- **Projeto:** mcpollama (`E:\MCPOllam`), servidor MCP em Go
- **Alvo:** `src/files/{policy,guard}.go`, `src/tools/{filetools,chunk,args}.go` e `src/mcpollama-mcp/{config,backend,server}.go`, no estado do working tree em 2026-10-23 (não commitado sobre `dedc305`)
- **Revisor:** subagente adversarial, modelo Opus 5.5
- **Data:** 2026-10-23 21:30–21:50 (hora local)
- **Ambiente:** Windows 11 IoT Enterprise 10.0.28000, Go 1.26.5 windows/amd64. Symlink, hardlink (`mklink /H`) e junction (`mklink /J`) funcionam com o usuário atual. O 8dot3 está habilitado em C: e desligado em D:. TEMP fica em D:.
- **Restrições respeitadas:** `src/` não foi editado (só leitura e `go vet`/`go test -c -o NUL` com `GOOS=linux`, que não gravam em `src/`). Nenhuma chamada ao Ollama real, nenhum git em `referencias/`, e planos, CLAUDE.md e README.md intocados.

---

## 1. Veredito

**NÃO SEGURO PARA COMMIT no estado atual.**

Um vetor escapou das duas proteções principais. Um **hardlink** dentro do `--allow-root` lê arquivos da denylist (`.env`, `.git\config`) e arquivos **fora da raiz**, no mesmo volume. O conteúdo chega ao Ollama remoto; isso foi provado ponta-a-ponta pelo binário (`realMain`). Ver SEC-C-01.

A correção proposta é recusar arquivos com mais de um link. São 5 linhas em `guard.go` mais 2 arquivos pequenos por plataforma. Ela foi **aplicada numa cópia separada e provada**: os 4 vetores de hardlink passam a BLOQUEADO e toda a suíte existente continua verde (detalhe em §6). Com essa correção, o veredito passa a **SEGURO COM RESSALVAS**. As ressalvas são SEC-M-01 a SEC-M-03; o owner decide se entram antes do commit ou viram pendência.

Tudo o mais que foi tentado ficou bloqueado. Isso inclui `..` em todas as grafias, UNC, `\\?\`, `\\.\`, `\??\`, ADS, nomes de dispositivo, 8.3 em C:, symlink e junction para fora e para dentro, TOCTOU com troca de arquivo e de diretório em corrida, teto de prompt em 1.350 combinações de map-reduce, `--max-chunks`, terminação da redução e vazamento em stdout/stderr com `--debug`.

## 2. Números

| Execução | Vetores (linhas `VEC`) | BLOQUEADO | ESCAPOU | ACHADO (política/config/injeção) | INFO | Suíte (`go test -count=1 -v ./...`) |
| --- | --- | --- | --- | --- | --- | --- |
| Código atual + testes de ataque | 232 | 219 | **4** (hardlink ×3 + e2e) | 7 | 2 | 263 PASS, 3 FAIL (só testes de ataque: `TestAttackHardlinks`, `TestAttackE2ELeaksAndHardlink`, `TestAttackRootIsJunction`), 1 SKIP (`TestDenylistWindowsShortName`, que pula em D: com 8dot3 desligado, como esperado) |
| Cópia com a correção do SEC-C-01 | 232 | 223 | **0** | 7 | 2 | 264 PASS, 1 FAIL (`TestAttackRootIsJunction`, o achado funcional SEC-m-06), 1 SKIP |
| `-race` nos testes de ataque (código atual) | 230 | 218 | 4 | 6 | 2 | **0 DATA RACE** |

Além das linhas `VEC`, há controles positivos (`CTRL`): caminho relativo legítimo, symlink que fica dentro da raiz, caminho com mais de 260 caracteres, BOM com CRLF e sem LF final. Todos passaram, exceto a raiz via junction (SEC-m-06).

**NÃO TESTÁVEL nesta estação (3):**

1. TOCTOU com FIFO no Linux (SEC-m-08): exige Linux.
2. Hardlink entre volumes: o NTFS não permite, então é impossível por construção.
3. Ollama real: proibido pela tarefa. Usei `httptest` e um backend falso.

Todo o resto foi executado e lido.

## 3. Método

1. Copiei `E:\MCPOllam\src` para `scratchpad\advrev\src` (robocopy sem `.exe`). Conferi por hash: **0 diferenças** em `*.go` e `go.mod` contra `E:\MCPOllam\src`. O `TOOLS.md` foi copiado para `scratchpad\advrev\` porque `TestCatalogInSync` e `TestToolsMdMatchesTheRealToolsList` o procuram em `..\..\TOOLS.md`.
2. Escrevi os testes de ataque como testes internos dos pacotes (`package files`, `package tools`, `package main`), reaproveitando os helpers existentes (`newWorld`, `newFileEnv`, `newMock`, `runMain`...):
   - Cada vetor registra `VEC|<id>|BLOQUEADO|ESCAPOU|ACHADO|INFO|NAO-TESTAVEL|<evidência>`.
   - Um ESCAPOU é falha de teste.
   - Sentinelas: `OUTSIDE-CONTENT-MUST-NOT-LEAK` fora da raiz, `SENSITIVE-DENYLIST-CONTENT-4b2a` nos nomes da denylist, `fileSentinel` e `wiringToken` no teste ponta-a-ponta.
   - Toda mensagem de recusa foi conferida contra o caminho-base, o nome do diretório temporário, os sentinelas e o token.
3. No map-reduce, usei um `Backend` falso cru, que **não** passa pelo cliente Ollama. O cliente já recusa um prompt acima do teto, e isso esconderia um erro de orçamento das tools. O falso registra todo `prompt` e `system` e confere `runes(prompt)+runes(system) <= teto` e `chamadas <= --max-chunks` em cada execução.
4. Os nomes curtos 8.3 foram testados em `C:\Users\Public\advrev*`, porque D: tem 8dot3 desligado (contexto do coordenador).
5. A correção proposta do SEC-C-01 foi aplicada numa segunda cópia (`scratchpad\advrev-fix`), com a suíte inteira e os ataques re-rodados.
6. **Erro de processo meu, já corrigido:** editei testes com `Get-Content -Raw` do PowerShell 5.1 sem `-Encoding utf8`. Isso converteu os literais não-ASCII (`COM¹`, `😀ção中`, `ñ`...) em mojibake, e também o `guard.go` da cópia de correção (por isso `TestNewGuardValidatesRoots` falhou na 1ª rodada da cópia corrigida, comparando `"não é um diretório"`). Reverti a mojibake (UTF-8 → 1252 → bytes originais) e recopiei o `guard.go` original antes de reaplicar a correção com I/O .NET UTF-8. Também limpei o test cache. **Todos os números deste relatório são da rodada final, depois do reparo.**

## 4. Achados

### SEC-C-01 — Hardlink dentro da raiz lê a denylist e arquivos fora da raiz (CRÍTICO)

**O que acontece.** `Guard.Read` decide pela denylist e pela contenção usando **o nome** do arquivo: o pedido e o destino de `EvalSymlinks`. Um hardlink é um segundo nome para o mesmo arquivo. `EvalSymlinks` não o resolve, e o `os.Root` o abre normalmente. Assim:

- `root\notes.txt`, hardlink para `root\.env`, devolve o conteúdo da `.env`;
- `root\cfg.txt`, hardlink para `root\.git\config`, devolve o `.git\config`;
- `root\copia.txt`, hardlink para `<base>\outside\x.txt`, devolve **conteúdo de fora da raiz**.

**Medido:** os 3 vetores ESCAPARAM (`TestAttackHardlinks`). Ponta-a-ponta, com `realMain`, `--allow-root` e `--debug`, `summarize_file {"path":"hl-env.txt"}` **enviou o conteúdo da `.env` ao Ollama** (`TestAttackE2ELeaksAndHardlink`).

**Pré-condição.** Alguém com escrita na raiz cria o hardlink no mesmo volume. No Windows isso **não exige** administrador nem modo desenvolvedor: `os.Link` e `mklink /H` funcionaram com o usuário comum. Caminhos realistas:

- um agente com tool de escrita, sob prompt injection, cria o link e depois chama `summarize_file`;
- um arquivo `.tar` extraído para dentro da raiz com entrada de hardlink;
- uma ferramenta que deduplica com hardlinks.

**Reprodução mínima** (`package files`):

```go
func TestHardlinkEscape(t *testing.T) {
	w := newWorld(t)
	write(t, filepath.Join(w.root, ".env"), "SENSITIVE")
	if err := os.Link(filepath.Join(w.root, ".env"), filepath.Join(w.root, "notes.txt")); err != nil {
		t.Skip(err)
	}
	os.Link(filepath.Join(w.outside, "x.txt"), filepath.Join(w.root, "copia.txt"))
	g := w.guard(t, 1<<20)
	if f, err := g.Read("notes.txt"); err == nil {
		t.Fatalf("leu a .env por hardlink: %q", f.Text)
	}
	if f, err := g.Read("copia.txt"); err == nil {
		t.Fatalf("leu fora da raiz por hardlink: %q", f.Text)
	}
}
```

**Correção proposta (provada).** Depois do `f.Stat()` do descritor aberto, recusar quando o número de links for diferente de 1, com a mesma mensagem opaca (FILE-014). Diff em `...testes\correcao-proposta\guard.go.diff`:

```go
	// A hard link is a second name for the same file: the denylist and the
	// containment saw only this name (SEC-C-01 of the adversarial review).
	if n, err := linkCount(f); err != nil || n != 1 {
		return nil, errNotAllowed()
	}
```

`linkCount` fica em `nlink_windows.go` (`syscall.GetFileInformationByHandle` → `NumberOfLinks`) e em `nlink_other.go` (`Stat_t.Nlink`). O `GOOS=linux go vet ./files` passa com eles.

- **Resultado na cópia corrigida:** 4/4 vetores de hardlink BLOQUEADOS (inclusive o e2e: `isError=true sent=false`) e a suíte existente inteira verde.
- **Efeito colateral:** arquivos legítimos com vários links passam a ser recusados (fail closed). São raros em projetos; no Windows aparecem em `C:\Windows\System32`/WinSxS e em stores como o pnpm.
- **Requisito:** cabe em FILE-004 ("reparse point que saia da raiz") e FILE-008. Sugiro registrar no plano a pendência "hardlink" e acrescentar o caso ao TEST-009.

### SEC-M-01 — Erro do Ollama pode devolver conteúdo do arquivo ao cliente (MÉDIO)

**O que acontece.** `fileJob.call` devolve o erro do cliente embrulhado (`falha na parte %d de %d: %w`). O `ollama.Error.Msg` inclui o *detalhe* do corpo devolvido pelo servidor (`statusError` → `detail`, até `maxDetailRunes`). Se o upstream ecoar parte do prompt no corpo do erro, o **conteúdo do arquivo vai para o resultado `isError` que o Claude lê**. O upstream pode ser o Ollama, o nginx, um proxy ou um servidor comprometido. Isso viola literalmente o FILE-012 ("nunca vai para ... erro").

**Medido** (`TestAttackErrorEchoLeak`): o mock responde 500 com `{"error":"bad input near: <trecho do prompt>"}`, e o erro devolvido foi `erro do servidor Ollama [HTTP 500 POST /api/generate]: bad input near: CONTEUDO-PRIVADO-DO-ARQUIVO-91`. O token e o caminho **não** vazaram.

**Pré-condição:** upstream que ecoa o prompt. O Ollama atual normalmente não faz isso, mas a garantia do FILE-012 não pode depender do servidor.

**Reprodução mínima** (`package tools`): ver `TestAttackErrorEchoLeak` em `tools/tools_attack_test.go`.

**Correção proposta.** Nas tools de arquivo, não repassar `Msg` do upstream. Em `fileJob.call`, fazer `var oe *ollama.Error; if errors.As(err, &oe) { return "", fmt.Errorf("o servidor Ollama recusou a chamada (%v, HTTP %d)", oe.Kind, oe.Status) }`, mantendo a mensagem acionável fixa de cada `Kind` sem o detalhe. A alternativa é remover do erro qualquer trecho de `f.Text`, o que é frágil. Vale também para erro no meio do stream (campo `error`).

### SEC-M-02 — `--allow-root` aceita a raiz de uma unidade (MÉDIO, decisão do owner)

**Medido** (`TestAttackAllowRootConfig`): `NewGuard([]string{"C:\\"})` é aceito, e `Read("C:\\Windows\\win.ini")` lê (`err=<nil>`). Com a raiz em `C:\` ou em `%USERPROFILE%`, todo o perfil fica legível e é enviado ao Ollama remoto, exceto a denylist. Somado às lacunas do SEC-M-03, entram `.npmrc`, `.kube\config`, `.docker\config.json`, `.ssh\config` e `.claude.json`. Não é escape de código: é escopo de configuração amplo demais sem nenhum aviso além do banner.

**Correção proposta** (escolher):

- (a) recusar na partida a raiz de volume (`filepath.VolumeName(abs)+"\"`) e o próprio `%USERPROFILE%`;
- (b) aceitar, mas com aviso explícito em stderr;
- (c) documentar como responsabilidade do operador.

### SEC-M-03 — Lacunas da denylist fixa (MÉDIO, decisão do owner — P-10)

**Medido** (`TestAttackDenylistGaps`, sobre `Denied`). Estes nomes com segredo **não** são negados:

- configs com token: `.npmrc`, `.pypirc`, `.netrc`, `_netrc`, `.pgpass`, `.htpasswd`;
- cofres e chaves: `*.ppk` (chave PuTTY), `*.jks`, `*.keystore`, `*.kdbx`, `*.ovpn`;
- infraestrutura: `terraform.tfstate`, `kubeconfig`, `.kube\config`, `.docker\config.json`;
- SSH e AWS: `.ssh\config`, `.ssh\known_hosts`, `.ssh\<chave-sem-extensão>` (ex.: `github`), `.aws\config`;
- genéricos: `token.txt`, `apikey.txt`, `passwords.txt`, `web.config`, `appsettings.json`, `.claude.json`.

Já cobertos e conferidos: `.git-credentials` (contém "credential"), `csl-ollama.env` (sufixo `.env`) e `id_rsa.pub` (prefixo `id_rsa`, negado por excesso, o que é aceitável).

**Correção proposta:** ampliar a lista com, no mínimo:

- `.npmrc`, `.pypirc`, `.netrc`, `_netrc`, `.pgpass`, `.htpasswd`;
- `*.ppk`, `*.jks`, `*.keystore`, `*.kdbx`, `*.ovpn`, `*.tfstate`;
- `.kube`, `.docker\config.json`, `kubeconfig`;
- o diretório `.ssh` inteiro (como já é feito com `.git`);
- `.aws`, `.azure`, `.gnupg`;
- nomes com `token`, `password` ou `apikey`.

A P-10 decidiu "denylist fixa sem override", então a ampliação é decisão do owner.

### SEC-m-01 — O nonce dos marcadores é reutilizado na redução (menor, prompt injection)

**O que acontece.** `newNonce` só é checado contra `f.Text`, e **o mesmo** nonce marca `ARQUIVO_*` (fase map) e `PARCIAL_*` (fase reduce). O modelo vê o nonce na fase map. Um arquivo com injection pode fazê-lo repetir na saída parcial a linha `=====PARCIAL_FIM <nonce>=====`, seguida de "ordens". Na redução, essas ordens ficam **fora** do bloco de dados.

**Medido** (`TestAttackNonceForgeryInReduce`): o prompt de redução teve **4** marcadores de fim válidos, e `ORDEM-FORJADA` ficou fora do bloco. Um arquivo **não** consegue forjar o marcador da fase map (`TestAttackFileForgesMarker`: BLOQUEADO).

**Impacto:** limitado. A saída do map já é influenciada pelo arquivo; o ganho do atacante é sair da "zona de dados" na redução.

**Correção:** gerar um nonce novo por etapa de redução com `newNonce(joined)` (checado contra os parciais) e citar esse nonce no `reduceTask`.

### SEC-m-02 — Vírgula no nome do diretório parte o `--allow-root` (menor)

**Medido:** `SplitRoots("<base>\Cliente, Inc")` → `["<base>\Cliente", "Inc"]`.

- Se `Inc` não existir relativo ao cwd, a partida falha. Foi o que aconteceu no teste: `--allow-root "Inc": o diretório não existe`. É fail-closed, mas a mensagem confunde.
- Se `<base>\Cliente` e `.\Inc` existirem, a raiz é **trocada em silêncio**: um diretório-prefixo vira raiz, mais um diretório relativo ao cwd.

**Correção:** usar `os.PathListSeparator` (`;` no Windows) ou exigir raízes absolutas (SEC-m-03).

### SEC-m-03 — Raiz relativa resolvida contra o cwd do processo (menor)

**Medido:** `NewGuard([]string{"."})` é aceito e vira o cwd do processo. O cwd de um servidor MCP lançado por um cliente é imprevisível.

**Correção:** recusar raiz não absoluta na partida.

### SEC-m-04 — Raízes aninhadas: o resultado da denylist depende da ordem (menor)

**Medido** (`TestAttackNestedRootsOrder`), lendo `root\.git\hooks\h.txt`:

| Ordem das raízes | Resultado |
| --- | --- |
| `[root, root\.git\hooks]` | **Denylisted** |
| `[root\.git\hooks, root]` | **lido** |

`requestSegments` e o laço de contenção usam a **primeira** raiz que contém o caminho.

**Correção:** usar a raiz **mais externa** que contém o caminho (o rel mais longo) para a denylist, ou recusar na partida uma raiz aninhada em outra ou sob um nome da denylist.

### SEC-m-05 — A redução em várias rodadas gasta GPU antes de falhar no `--max-chunks` (menor)

**O que acontece.** A pré-checagem conta `n+1` chamadas (n partes mais uma combinação). Quando os parciais não cabem numa redução, cada rodada consome chamadas e o erro só sai ao bater no teto.

**Medido:**

- `TestAttackReduceRoundsAfterPrecheck`: 20 chamadas de 20 e depois `o processamento passaria de 20 chamadas`;
- `reduce-term/huge` (saída de 2 M caracteres por chamada): 400 de 400 chamadas e depois erro.

O teto **nunca** é ultrapassado (FILE-011 cumprido no limite), mas o trabalho é jogado fora.

**Correção:** antes de cada rodada, falhar se `len(groups)` passar das chamadas restantes, e/ou limitar o tamanho de cada parcial (`num_predict` nas chamadas de map).

### SEC-m-06 — Raiz via junction: aceita na partida, mas nenhuma leitura funciona (menor, funcional, fail-closed)

**Medido** (`TestAttackRootIsJunction` e `debug_junction_attack_test.go`):

- `NewGuard([jroot])`, com `jroot` como junction para a raiz, é **aceito** com `roots=[...\jroot]` (o `EvalSymlinks` do Go não atravessa a junction).
- `Read("a.txt")` falha, porque `EvalSymlinks(jroot\a.txt)` dá "O sistema não pode encontrar o caminho especificado".
- Uma junction **acima** da raiz é recusada na partida com a mensagem enganosa "o diretório não existe".

Não há vazamento: ataques via `..` a partir da raiz-junction ficaram BLOQUEADOS.

**Correção:** na partida, sondar a raiz (ex.: `EvalSymlinks(filepath.Join(res, "."))` mais um `os.ReadDir`, ou detectar `ModeIrregular` no caminho) e recusar com uma mensagem clara ("junction não suportada como raiz; use o caminho real"). A alternativa é resolver a junction explicitamente.

### SEC-m-07 — `guard_test.go` não compila para Linux (menor, teste/build)

**Medido:** com `GOOS=linux`, tanto `go vet ./files` quanto `go test -c ./files` em `E:\MCPOllam\src` falham com `undefined: syscall.UTF16PtrFromString`, `syscall.GetShortPathName` e `syscall.UTF16ToString` (`guard_test.go:341-350`). O binário Linux previsto no CLAUDE.md compila, mas a suíte do pacote `files` não roda em Linux. Os meus testes de ataque foram separados em `files_attack_windows_test.go` para não repetir o problema.

**Correção:** mover `TestDenylistWindowsShortName` para `guard_windows_test.go` (`//go:build windows`).

### SEC-m-08 — TOCTOU com FIFO no Linux pode travar a goroutine (menor, NÃO TESTÁVEL aqui)

**Hipótese (não executada, por falta de Linux):** entre `rt.Stat(rel)` (arquivo regular) e `rt.Open(rel)`, trocar o arquivo por uma FIFO faz o `open(O_RDONLY)` bloquear até aparecer um escritor. A goroutine da chamada fica presa, e o cancelamento não a solta. A trava não pega a fila de GPU (o `acquire` vem depois), então o impacto é uma goroutine e um descritor por chamada.

**Correção:** no Unix, abrir com `O_NONBLOCK` e validar o tipo pelo `f.Stat()`, ou usar `O_PATH`. Para confirmar, rodar o `TestAttackTOCTOUFileSwap` adaptado (`syscall.Mkfifo`) em Linux.

## 5. O que foi tentado e ficou bloqueado (síntese; a tabela completa está no Apêndice A)

- **Travessia (42 vetores):**
  - `..` com `\`, `/` e misto; `....//`, `...`;
  - `.. `, `..  `, `.. .` e `..` seguido de NBSP, relativos e absolutos sob a raiz;
  - URL-encoded simples e duplo, ponto e barra fullwidth, `~`, `%USERPROFILE%`, `$env:USERPROFILE`, tab, espaço inicial, separador final;
  - caminho "rooted" sem unidade, relativo à unidade (`D:..\`), outra unidade, raiz com prefixo igual (inclusive em maiúsculas);
  - 30× `..`. Todos os relativos caem no `NotAllowed` opaco.
- **Sintaxe especial do Windows (33, incluindo 2 sobre um fluxo ADS real):**
  - UNC: `\\localhost\d$`, `\\127.0.0.1`, `//`, `\/`, `/\`;
  - prefixos: `\\?\`, `//?/`, `\\?\UNC`, `\\.\`, `//./`, `\??\`, `/??/`, `\\?\GLOBALROOT`, `\\;LanmanRedirector`;
  - ADS: `:x`, `::$DATA`, `:x:$DATA`, `::$INDEX_ALLOCATION` e um fluxo real com segredo gravado (`a.txt:hidden`);
  - curingas, NUL, caracteres de controle, vazio e só espaços.
- **Dispositivos (24):** `CON`, `CON.txt`, `con .txt`, `CON.`, `CON `, `nul.tar.gz`, `COM1`, `COM¹`, `com²`, `LPT³.txt`, `conin$`, `CONOUT$`, `CLOCK$`, `sub\aux\x.txt` e mais. `COM0`, `LPT0` e `COM10` não estão na lista, mas caem em `NotAllowed` (o sistema não os abre como dispositivo aqui).
- **Aliases da denylist (40):**
  - maiúsculas e sufixos: `.ENV`, `.env.`, `.env..`, `.env `, `.env. . .`;
  - Unicode: `ſecret.txt` (s longo), `credentıals.json` (i sem ponto), `ıd_rsa`, `.gıt`;
  - variantes de `.git`: `.git.\config`, `.git \config`, `.git\.\config`;
  - ADS sobre `.env`.

  A segunda checagem da denylist (sobre o nome **resolvido**) fecha o que a primeira deixaria passar.
- **8.3 em C: (18):** `ENVPRO~1.LOC`, `CREDEN~1.JSO`, `MYSECR~1.TXT`, `ID_ED2~1`, `GIT~1\config`, `SECRET~1\plain.txt`, `CREDEN~1\plain.txt`, `SETTIN~1.JSO` e `CERTIF~1.PEM`, cada um em forma absoluta e relativa: todos **Denylisted**.
- **Symlinks (14):** arquivo e diretório para fora, alvo relativo, alvo `\\?\` e UNC, cadeia, laço, `..`, symlink para `.env`, `.git` e `secrets`. Junctions (9): para fora, para dentro, para `.git`, aninhada e variantes de caixa, ponto e espaço finais.
- **Diretório case-sensitive** (`fsutil setCaseSensitiveInfo`), com `case\root` como raiz e `case\Root` como irmã com segredo: sem vazamento. O `filepath.Rel` (case-insensitive) confundiu as duas, mas o `os.Root` abriu a raiz real e devolveu o arquivo **de dentro**. É uma confusão de nome, não uma fuga (INFO).
- **TOCTOU (3 corridas de 4 s, 2 goroutines trocando):**

  | Troca | Leituras | Benignas | Vazamentos |
  | --- | --- | --- | --- |
  | arquivo → symlink para `.env` | 2.345 | 130 | **0** |
  | arquivo → symlink para fora | 2.461 | 138 | **0** |
  | diretório → symlink para fora | 3.433 | 0 | **0** |

  Defesas que seguram: `os.Root` mais `SameFile` e o `EvalSymlinks` pós-leitura.
- **Tamanho e encoding:**
  - tamanho: 1 byte acima do teto; arquivo esparso de 1 TiB (`TooLarge` sem ler); arquivo que cresce durante a leitura (nenhuma leitura acima do teto);
  - encoding: UTF-16 LE/BE com BOM, UTF-16 sem BOM, UTF-32, surrogate CESU, overlong e Latin-1, todos recusados.
- **Caminhos longos:** caminho legítimo com mais de 260 caracteres lido corretamente; nome de 40 k, 40 k de `a\..\`, 100 k de `\` e 1 M de caracteres: recusa limpa, sem pânico.
- **Map-reduce:**
  - 1.350 execuções combinando tetos {1.200, 2.500, 7.000, 20.000, 128.000}, `--max-chunks` {3, 40, 400}, 6 arquivos (ASCII, emoji de 4 bytes, linha única de 30 k, linha única de emoji, 15 k linhas curtas, CRLF) e 5 combinações de argumentos no máximo (instruction de 3.999 runes multibyte, format de 1.000, focus de 2.000, language de 100), com respostas curtas, longas ou de meio prompt: **0 prompts acima do teto, 0 casos acima do `--max-chunks` e 0 acionamentos da defesa interna** ("erro interno").
  - Pré-checagem: 0 chamadas quando `need > max-chunks` nas 3 tools.
  - Terminação: com saída do mesmo tamanho, crescente, encolhendo, vazia ou enorme, a redução **sempre termina** (≤ 450 ms) e o teto é respeitado.
- **Vazamento ponta-a-ponta** (`realMain`, `--debug`, `OLLAMA_API_KEY` sentinela): em 12 recusas, nenhum conteúdo, caminho-base ou token no stdout nem no stderr, e 0 requisições ao Ollama. Nas tools, nenhuma recusa chama o backend. Também não houve escrita: a árvore ficou idêntica antes e depois de leituras aceitas e recusadas.

## 6. Recomendação objetiva antes do commit

1. **Obrigatório:** corrigir o SEC-C-01 (hardlink) com a correção provada (`...testes\correcao-proposta\`) e trazer `TestAttackHardlinks` (ou a reprodução mínima de §4) para `guard_test.go` como teste de regressão. Depois rodar a suíte limpa e o `-race`.
2. **Recomendado no mesmo commit** (baixo custo):
   - SEC-M-01: não repassar o detalhe do upstream nos erros das tools de arquivo;
   - SEC-m-07: mover o teste 8.3 para `_windows_test.go`;
   - SEC-m-01: nonce novo na redução.
3. **Decisões do owner (AskUserQuestion):** SEC-M-02 (raiz de volume e perfil), SEC-M-03 (ampliar a denylist), SEC-m-02/03 (separador `;` e raiz só absoluta), SEC-m-04 (raízes aninhadas) e SEC-m-06 (raiz via junction).
4. **Pendências para o plano:** SEC-m-05 (pré-checagem das rodadas de redução) e SEC-m-08 (validar a FIFO em Linux).

## 7. Artefatos

- Relatório: `E:\MCPOllam\.workspace\reports\MCPOllam2026-10-23-2130-revisao-adversarial-fase4-arquivos.report.md` (este arquivo).
- Testes de ataque (para virar regressão): `E:\MCPOllam\.workspace\reports\MCPOllam2026-10-23-2130-revisao-adversarial-fase4-arquivos.testes\`
  - `files\files_attack_test.go`: travessia, sintaxe Windows, dispositivos, aliases da denylist, symlink, junction, hardlink, TOCTOU, tamanho, encoding, caminhos longos, configuração do `--allow-root`, raízes aninhadas, sem escrita.
  - `files\files_attack_windows_test.go`: nomes 8.3 em C: (`//go:build windows`).
  - `files\files_attack_extra_test.go`: lacunas da denylist e diretório case-sensitive.
  - `files\debug_junction_attack_test.go`: diagnóstico do SEC-m-06.
  - `tools\tools_attack_test.go`: matriz de teto, pré-checagem, rodadas e terminação da redução, nonce, eco de erro, recusa sem chamada.
  - `mcpollama-mcp\e2e_attack_test.go`: vazamento ponta-a-ponta e hardlink até o Ollama.
  - `correcao-proposta\guard.go.diff`, `nlink_windows.go`, `nlink_other.go`: correção do SEC-C-01.
- Os testes dependem dos helpers existentes (`newWorld`, `write`, `newFileEnv`, `lines`, `newMock`, `runMain`, `rootWithFiles`...). Para rodá-los, copie cada um para o pacote correspondente em `src/`.
- Logs (scratchpad da sessão, não versionados): `advrev-final.log`, `advrev-fix-final.log`, `attack-race.log`, `files-attack.log`, `tools-attack.log`.

## Apêndice A — Tabela de vetores (rodada final, código atual)

Motivos: `NotAllowed (opaco)` = "arquivo não encontrado ou fora das raízes permitidas"; `Denylisted`; `SpecialPath` = recusa de sintaxe (UNC/ADS/dispositivo/curinga/relativo à unidade); `BadRequest`; `TooLarge`; `Binary`; `Encoding`.

| # | Vetor | Resultado | Motivo / evidência |
| --- | --- | --- | --- |
| 1 | `deny/gaps` | ACHADO | nao cobertos pela denylist fixa: .npmrc .pypirc .netrc _netrc .pgpass .htpasswd putty.ppk release.jks app.keystore vault.kdbx client.ovpn terraform.tfstate k... |
| 2 | `case-sensitive-dir/sibling-Root` | BLOQUEADO | sem vazamento; pedido de case\Root\f.txt devolveu "INSIDE" (arquivo da raiz real: confusao de nome, nao fuga) |
| 3 | `trav/four-dots-back` | BLOQUEADO | NotAllowed (opaco) |
| 4 | `trav/tab` | BLOQUEADO | NotAllowed (opaco) |
| 5 | `trav/dotdot-many` | BLOQUEADO | NotAllowed (opaco) |
| 6 | `trav/dotdot-backslash` | BLOQUEADO | NotAllowed (opaco) |
| 7 | `trav/three-dots` | BLOQUEADO | NotAllowed (opaco) |
| 8 | `trav/dotdot-2space` | BLOQUEADO | NotAllowed (opaco) |
| 9 | `trav/dotdot-dot-seg` | BLOQUEADO | NotAllowed (opaco) |
| 10 | `trav/leading-space` | BLOQUEADO | NotAllowed (opaco) |
| 11 | `trav/rooted-no-drive-slash` | BLOQUEADO | NotAllowed (opaco) |
| 12 | `trav/unicode-fullstop` | BLOQUEADO | NotAllowed (opaco) |
| 13 | `trav/tilde` | BLOQUEADO | NotAllowed (opaco) |
| 14 | `trav/dotdot-nbsp` | BLOQUEADO | NotAllowed (opaco) |
| 15 | `trav/dot-dotdot` | BLOQUEADO | NotAllowed (opaco) |
| 16 | `trav/drive-relative` | BLOQUEADO | SpecialPath |
| 17 | `trav/dotdot-mixed` | BLOQUEADO | NotAllowed (opaco) |
| 18 | `trav/urlenc-slash` | BLOQUEADO | NotAllowed (opaco) |
| 19 | `trav/double-urlenc` | BLOQUEADO | NotAllowed (opaco) |
| 20 | `trav/dotdot-slash` | BLOQUEADO | NotAllowed (opaco) |
| 21 | `trav/dotdot-deep` | BLOQUEADO | NotAllowed (opaco) |
| 22 | `trav/four-dots-slash` | BLOQUEADO | NotAllowed (opaco) |
| 23 | `trav/unicode-fwslash` | BLOQUEADO | NotAllowed (opaco) |
| 24 | `trav/envvar-percent` | BLOQUEADO | NotAllowed (opaco) |
| 25 | `trav/drive-relative-dotdot` | BLOQUEADO | SpecialPath |
| 26 | `trav/dotdot-space` | BLOQUEADO | NotAllowed (opaco) |
| 27 | `trav/dotdot-space-dot` | BLOQUEADO | NotAllowed (opaco) |
| 28 | `trav/urlenc-dotdot` | BLOQUEADO | NotAllowed (opaco) |
| 29 | `trav/dotdot-dot` | BLOQUEADO | NotAllowed (opaco) |
| 30 | `trav/urlenc-backslash` | BLOQUEADO | NotAllowed (opaco) |
| 31 | `trav/envvar-ps` | BLOQUEADO | SpecialPath |
| 32 | `trav/trailing-sep` | BLOQUEADO | NotAllowed (opaco) |
| 33 | `trav/rooted-no-drive` | BLOQUEADO | NotAllowed (opaco) |
| 34 | `trav/abs-outside` | BLOQUEADO | NotAllowed (opaco) |
| 35 | `trav/abs-sibling-upper` | BLOQUEADO | NotAllowed (opaco) |
| 36 | `trav/abs-root-space-trailing` | BLOQUEADO | NotAllowed (opaco) |
| 37 | `trav/abs-winini-dotdot` | BLOQUEADO | NotAllowed (opaco) |
| 38 | `trav/abs-other-drive` | BLOQUEADO | NotAllowed (opaco) |
| 39 | `trav/abs-other-drive-fwd` | BLOQUEADO | NotAllowed (opaco) |
| 40 | `trav/abs-dotdot` | BLOQUEADO | NotAllowed (opaco) |
| 41 | `trav/abs-sibling` | BLOQUEADO | NotAllowed (opaco) |
| 42 | `trav/abs-root-dotdot-space` | BLOQUEADO | NotAllowed (opaco) |
| 43 | `trav/abs-root-dot-trailing` | BLOQUEADO | NotAllowed (opaco) |
| 44 | `trav/abs-dotdot-slash` | BLOQUEADO | NotAllowed (opaco) |
| 45 | `win/wildcard-star` | BLOQUEADO | SpecialPath |
| 46 | `win/empty` | BLOQUEADO | BadRequest |
| 47 | `win/spaces` | BLOQUEADO | BadRequest |
| 48 | `win/device-prefix-fwd` | BLOQUEADO | SpecialPath |
| 49 | `win/globalroot` | BLOQUEADO | SpecialPath |
| 50 | `win/ads-on-dir-dotdot` | BLOQUEADO | SpecialPath |
| 51 | `win/wildcard-gt` | BLOQUEADO | SpecialPath |
| 52 | `win/nul-only` | BLOQUEADO | SpecialPath |
| 53 | `win/ctrl-chars` | BLOQUEADO | NotAllowed (opaco) |
| 54 | `win/ads-name` | BLOQUEADO | SpecialPath |
| 55 | `win/ads-data` | BLOQUEADO | SpecialPath |
| 56 | `win/ads-dir-index` | BLOQUEADO | SpecialPath |
| 57 | `win/unc-127` | BLOQUEADO | SpecialPath |
| 58 | `win/long-prefix-fwd` | BLOQUEADO | SpecialPath |
| 59 | `win/nul-byte` | BLOQUEADO | SpecialPath |
| 60 | `win/device-prefix` | BLOQUEADO | SpecialPath |
| 61 | `win/ads-abs` | BLOQUEADO | SpecialPath |
| 62 | `win/ads-named-data` | BLOQUEADO | SpecialPath |
| 63 | `win/pipe` | BLOQUEADO | SpecialPath |
| 64 | `win/unc-localhost-admin` | BLOQUEADO | SpecialPath |
| 65 | `win/unc-mixed2` | BLOQUEADO | SpecialPath |
| 66 | `win/long-prefix-unc` | BLOQUEADO | SpecialPath |
| 67 | `win/wildcard-lt` | BLOQUEADO | SpecialPath |
| 68 | `win/unc-fwd` | BLOQUEADO | SpecialPath |
| 69 | `win/long-prefix` | BLOQUEADO | SpecialPath |
| 70 | `win/nt-prefix-fwd` | BLOQUEADO | SpecialPath |
| 71 | `win/wildcard-q` | BLOQUEADO | SpecialPath |
| 72 | `win/nt-prefix` | BLOQUEADO | SpecialPath |
| 73 | `win/lanman` | BLOQUEADO | SpecialPath |
| 74 | `win/wildcard-quote` | BLOQUEADO | SpecialPath |
| 75 | `win/unc-mixed` | BLOQUEADO | SpecialPath |
| 76 | `win/ads-real-stream` | BLOQUEADO | SpecialPath |
| 77 | `win/ads-real-stream-data` | BLOQUEADO | SpecialPath |
| 78 | `dev/con` | BLOQUEADO | SpecialPath |
| 79 | `dev/CON` | BLOQUEADO | SpecialPath |
| 80 | `dev/CON.txt` | BLOQUEADO | SpecialPath |
| 81 | `dev/con .txt` | BLOQUEADO | SpecialPath |
| 82 | `dev/CON.` | BLOQUEADO | SpecialPath |
| 83 | `dev/CON ` | BLOQUEADO | SpecialPath |
| 84 | `dev/nul.tar.gz` | BLOQUEADO | SpecialPath |
| 85 | `dev/NUL` | BLOQUEADO | SpecialPath |
| 86 | `dev/prn` | BLOQUEADO | SpecialPath |
| 87 | `dev/aux.c` | BLOQUEADO | SpecialPath |
| 88 | `dev/COM1` | BLOQUEADO | SpecialPath |
| 89 | `dev/com9.log` | BLOQUEADO | SpecialPath |
| 90 | `dev/COM¹` | BLOQUEADO | SpecialPath |
| 91 | `dev/com²` | BLOQUEADO | SpecialPath |
| 92 | `dev/LPT³.txt` | BLOQUEADO | SpecialPath |
| 93 | `dev/lpt1` | BLOQUEADO | SpecialPath |
| 94 | `dev/conin$` | BLOQUEADO | SpecialPath |
| 95 | `dev/CONOUT$` | BLOQUEADO | SpecialPath |
| 96 | `dev/CLOCK$` | BLOQUEADO | SpecialPath |
| 97 | `dev/sub\aux\x.txt` | BLOQUEADO | SpecialPath |
| 98 | `dev/sub\con` | BLOQUEADO | SpecialPath |
| 99 | `dev/COM0` | BLOQUEADO | NotAllowed (opaco) |
| 100 | `dev/LPT0` | BLOQUEADO | NotAllowed (opaco) |
| 101 | `dev/COM10` | BLOQUEADO | NotAllowed (opaco) |
| 102 | `deny/.ENV` | BLOQUEADO | Denylisted |
| 103 | `deny/.Env` | BLOQUEADO | Denylisted |
| 104 | `deny/.\.env` | BLOQUEADO | Denylisted |
| 105 | `deny/sub\..\.env` | BLOQUEADO | Denylisted |
| 106 | `deny/.env.` | BLOQUEADO | Denylisted |
| 107 | `deny/.env..` | BLOQUEADO | Denylisted |
| 108 | `deny/.env ` | BLOQUEADO | Denylisted |
| 109 | `deny/.env. . .` | BLOQUEADO | Denylisted |
| 110 | `deny/.env  .` | BLOQUEADO | Denylisted |
| 111 | `deny/SECRET.TXT` | BLOQUEADO | Denylisted |
| 112 | `deny/secret.txt.` | BLOQUEADO | Denylisted |
| 113 | `deny/secret.txt ` | BLOQUEADO | Denylisted |
| 114 | `deny/Secret.Txt...` | BLOQUEADO | Denylisted |
| 115 | `deny/ſecret.txt` | BLOQUEADO | NotAllowed (opaco) |
| 116 | `deny/credentıals.json` | BLOQUEADO | NotAllowed (opaco) |
| 117 | `deny/CREDENTIALS.JSON` | BLOQUEADO | Denylisted |
| 118 | `deny/ıd_rsa` | BLOQUEADO | NotAllowed (opaco) |
| 119 | `deny/id_rſa` | BLOQUEADO | NotAllowed (opaco) |
| 120 | `deny/ID_RSA` | BLOQUEADO | Denylisted |
| 121 | `deny/id_rsa.` | BLOQUEADO | Denylisted |
| 122 | `deny/server.KEY` | BLOQUEADO | Denylisted |
| 123 | `deny/server.key.` | BLOQUEADO | Denylisted |
| 124 | `deny/my.PEM ` | BLOQUEADO | Denylisted |
| 125 | `deny/.MCP.JSON` | BLOQUEADO | Denylisted |
| 126 | `deny/.mcp.json.` | BLOQUEADO | Denylisted |
| 127 | `deny/SETTINGS.LOCAL.JSON` | BLOQUEADO | Denylisted |
| 128 | `deny/.git\config` | BLOQUEADO | Denylisted |
| 129 | `deny/.GIT\config` | BLOQUEADO | Denylisted |
| 130 | `deny/.git.\config` | BLOQUEADO | Denylisted |
| 131 | `deny/.git \config` | BLOQUEADO | Denylisted |
| 132 | `deny/.git\.\config` | BLOQUEADO | Denylisted |
| 133 | `deny/.g\u0131t\config` | BLOQUEADO | NotAllowed (opaco) |
| 134 | `deny/secrets\a.txt` | BLOQUEADO | Denylisted |
| 135 | `deny/SECRETS\a.txt` | BLOQUEADO | Denylisted |
| 136 | `deny/secrets.\a.txt` | BLOQUEADO | Denylisted |
| 137 | `deny/\u017fecrets\a.txt` | BLOQUEADO | NotAllowed (opaco) |
| 138 | `deny/.env::$DATA` | BLOQUEADO | SpecialPath |
| 139 | `deny/.env:$DATA` | BLOQUEADO | SpecialPath |
| 140 | `deny/abs-upper` | BLOQUEADO | Denylisted |
| 141 | `deny/abs-dotdot` | BLOQUEADO | Denylisted |
| 142 | `sym/f-out.txt` | BLOQUEADO | NotAllowed (opaco) |
| 143 | `sym/f-out-rel.txt` | BLOQUEADO | NotAllowed (opaco) |
| 144 | `sym/f-out-long.txt` | BLOQUEADO | NotAllowed (opaco) |
| 145 | `sym/f-env.txt` | BLOQUEADO | Denylisted |
| 146 | `sym/f-env-rel.txt` | BLOQUEADO | Denylisted |
| 147 | `sym/f-gitcfg.txt` | BLOQUEADO | Denylisted |
| 148 | `sym/d-out\x.txt` | BLOQUEADO | NotAllowed (opaco) |
| 149 | `sym/d-git\config` | BLOQUEADO | Denylisted |
| 150 | `sym/d-secrets\a.txt` | BLOQUEADO | Denylisted |
| 151 | `sym/chain1.txt` | BLOQUEADO | NotAllowed (opaco) |
| 152 | `sym/loop1.txt` | BLOQUEADO | NotAllowed (opaco) |
| 153 | `sym/d-up\outside\x.txt` | BLOQUEADO | NotAllowed (opaco) |
| 154 | `sym/d-up\root\.env` | BLOQUEADO | Denylisted |
| 155 | `sym/f-unc.txt` | BLOQUEADO | NotAllowed (opaco) |
| 156 | `junc/j-out\x.txt` | BLOQUEADO | NotAllowed (opaco) |
| 157 | `junc/j-in\in.txt` | BLOQUEADO | NotAllowed (opaco) |
| 158 | `junc/j-git\config` | BLOQUEADO | NotAllowed (opaco) |
| 159 | `junc/real\j-deep\outside\x.txt` | BLOQUEADO | NotAllowed (opaco) |
| 160 | `junc/real\j-deep\root\.env` | BLOQUEADO | Denylisted |
| 161 | `junc/D:\Users\claiton.linhares\AppData\Local\Temp\gd456480006\root\j-out\x.txt` | BLOQUEADO | NotAllowed (opaco) |
| 162 | `junc/J-OUT\x.txt` | BLOQUEADO | NotAllowed (opaco) |
| 163 | `junc/j-out.\x.txt` | BLOQUEADO | NotAllowed (opaco) |
| 164 | `junc/j-out \x.txt` | BLOQUEADO | NotAllowed (opaco) |
| 165 | `rootj/dotdot` | BLOQUEADO | NotAllowed (opaco) |
| 166 | `rootj/abs-dotdot` | BLOQUEADO | NotAllowed (opaco) |
| 167 | `hard/hl-outside.txt` | ESCAPOU | conteudo-de-fora |
| 168 | `hard/hl-env.txt` | ESCAPOU | conteudo-da-denylist |
| 169 | `hard/hl-gitcfg.txt` | ESCAPOU | conteudo-da-denylist |
| 170 | `toctou/file->symlink-.env-dentro` | BLOQUEADO | 2345 leituras, 130 ok (benignas), 0 vazamentos |
| 171 | `toctou/file->symlink-fora` | BLOQUEADO | 2461 leituras, 138 ok (benignas), 0 vazamentos |
| 172 | `toctou/dir->symlink-fora` | BLOQUEADO | 3433 leituras, 0 ok (benignas), 0 vazamentos |
| 173 | `size/growing` | BLOQUEADO | nenhuma leitura acima do teto |
| 174 | `size/one-over` | BLOQUEADO | TooLarge |
| 175 | `size/sparse-1TiB` | BLOQUEADO | TooLarge |
| 176 | `enc/overlong-slash` | BLOQUEADO | Encoding |
| 177 | `enc/latin1` | BLOQUEADO | Encoding |
| 178 | `enc/utf16le-bom` | BLOQUEADO | Encoding |
| 179 | `enc/utf16be-bom` | BLOQUEADO | Encoding |
| 180 | `enc/utf16le-nobom` | BLOQUEADO | Binary |
| 181 | `enc/utf32le-bom` | BLOQUEADO | Encoding |
| 182 | `enc/cesu-surrogate` | BLOQUEADO | Encoding |
| 183 | `long/40k-name` | BLOQUEADO | NotAllowed (opaco) |
| 184 | `long/40k-dotdot` | BLOQUEADO | NotAllowed (opaco) |
| 185 | `long/100k-seps` | BLOQUEADO | SpecialPath |
| 186 | `long/1M` | BLOQUEADO | NotAllowed (opaco) |
| 187 | `cfg/comma-in-root` | ACHADO | SplitRoots corta o nome; NewGuard err=--allow-root "Inc": o diretório não existe ou não pode ser resolvido roots=0 |
| 188 | `cfg/relative-root` | ACHADO | raiz '.' aceita e resolvida para o cwd do processo (true) |
| 189 | `cfg/drive-root` | ACHADO | --allow-root C:\ aceito; leitura de C:\Windows\win.ini err=<nil> |
| 190 | `cfg/root-"\\\\?\\C:\\Windows"` | BLOQUEADO | --allow-root "\\\\?\\C:\\Windows": caminho não permitido: caminhos UNC (\\servidor\compartilhamento) e os prefixos \\?\ e \\.\ são recusados |
| 191 | `cfg/root-"\\\\localhost\\c$"` | BLOQUEADO | --allow-root "\\\\localhost\\c$": caminho não permitido: caminhos UNC (\\servidor\compartilhamento) e os prefixos \\?\ e \\.\ são recusados |
| 192 | `cfg/root-"C:"` | BLOQUEADO | --allow-root "C:": caminho não permitido: caminhos relativos à unidade (C:arquivo) são recusados; use um caminho completo |
| 193 | `cfg/root-"C:Windows"` | BLOQUEADO | --allow-root "C:Windows": caminho não permitido: caminhos relativos à unidade (C:arquivo) são recusados; use um caminho completo |
| 194 | `cfg/root-"C:\\Windows\\win.ini"` | BLOQUEADO | --allow-root "C:\\Windows\\win.ini": não é um diretório |
| 195 | `cfg/root-"C:\\Windows::$INDEX_ALLOCATION"` | BLOQUEADO | --allow-root "C:\\Windows::$INDEX_ALLOCATION": caminho não permitido: fluxos NTFS alternativos (arquivo:fluxo) são recusados |
| 196 | `cfg/nested-roots` | INFO | ordem [raiz, raiz/.git/hooks] err=acesso negado: o arquivo está na lista fixa de nomes sensíveis (.env, chaves, credenciais, .git...) e não pode ser lido ; o... |
| 197 | `cfg/nested-roots` | ACHADO | resultado depende da ordem das raizes |
| 198 | `nowrite` | BLOQUEADO | nenhuma mudanca na arvore |
| 199 | `short/abs/.env.production.local` | BLOQUEADO | Denylisted |
| 200 | `short/rel/.env.production.local` | BLOQUEADO | Denylisted |
| 201 | `short/abs/credentials.json` | BLOQUEADO | Denylisted |
| 202 | `short/rel/credentials.json` | BLOQUEADO | Denylisted |
| 203 | `short/abs/mysecretfile.txt` | BLOQUEADO | Denylisted |
| 204 | `short/rel/mysecretfile.txt` | BLOQUEADO | Denylisted |
| 205 | `short/abs/id_ed25519_github` | BLOQUEADO | Denylisted |
| 206 | `short/rel/id_ed25519_github` | BLOQUEADO | Denylisted |
| 207 | `short/abs/.git\config` | BLOQUEADO | Denylisted |
| 208 | `short/rel/.git\config` | BLOQUEADO | Denylisted |
| 209 | `short/abs/secretsdir\plain.txt` | BLOQUEADO | Denylisted |
| 210 | `short/rel/secretsdir\plain.txt` | BLOQUEADO | Denylisted |
| 211 | `short/abs/credentialstore\plain.txt` | BLOQUEADO | Denylisted |
| 212 | `short/rel/credentialstore\plain.txt` | BLOQUEADO | Denylisted |
| 213 | `short/abs/settings.local.json` | BLOQUEADO | Denylisted |
| 214 | `short/rel/settings.local.json` | BLOQUEADO | Denylisted |
| 215 | `short/abs/certificate.pem` | BLOQUEADO | Denylisted |
| 216 | `short/rel/certificate.pem` | BLOQUEADO | Denylisted |
| 217 | `e2e/refusals` | BLOQUEADO | 12 recusas, stdout/stderr sem conteudo, caminho ou token; 0 chamadas ao Ollama |
| 218 | `e2e/hardlink-.env` | ESCAPOU | o conteudo do .env (via hardlink hl-env.txt) foi enviado ao Ollama |
| 219 | `ceiling-matrix` | BLOQUEADO | 1350 execucoes, 0 violacoes, 0 defesas-internas acionadas |
| 220 | `precheck/summarize_file` | BLOQUEADO | 0 chamadas: o arquivo (132000 caracteres) exigiria 135 chamadas ao modelo, acima do teto --max-chunks (5); reduza o arquivo ou aumente o teto |
| 221 | `precheck/extract` | BLOQUEADO | 0 chamadas: o arquivo (132000 caracteres) exigiria 126 chamadas ao modelo, acima do teto --max-chunks (5); reduza o arquivo ou aumente o teto |
| 222 | `precheck/translate` | BLOQUEADO | 0 chamadas: o arquivo (132000 caracteres) exigiria 134 chamadas ao modelo, acima do teto --max-chunks (5); reduza o arquivo ou aumente o teto |
| 223 | `reduce-rounds` | INFO | err=falha ao combinar as 14 partes: o processamento passaria de 20 chamadas ao modelo (--max-chunks); reduza o arquivo ou aumente o teto chamadas=20 (teto 20) |
| 224 | `reduce-term/grow` | BLOQUEADO | 3ms chamadas=52 max=2436 err=falha ao combinar as 9 partes: a combinação dos parciais não reduziu o texto; processamento interrompido |
| 225 | `reduce-term/shrink-by-1` | BLOQUEADO | 2ms chamadas=18 max=2403 err=<nil> |
| 226 | `reduce-term/empty` | BLOQUEADO | 2ms chamadas=10 max=2403 err=<nil> |
| 227 | `reduce-term/huge` | BLOQUEADO | 448ms chamadas=400 max=2436 err=falha ao combinar as 9 partes: o processamento passaria de 400 chamadas ao modelo (--max-chunks); reduza o arquivo ou aumente... |
| 228 | `reduce-term/same-size` | BLOQUEADO | 3ms chamadas=33 max=2436 err=falha ao combinar as 9 partes: a combinação dos parciais não reduziu o texto; processamento interrompido |
| 229 | `inject/nonce-reuse-reduce` | ACHADO | o prompt de reducao tem 4 marcadores de fim validos; 'ORDEM-FORJADA' fica FORA do bloco de dados |
| 230 | `inject/file-forged-marker` | BLOQUEADO | nonce aleatorio, 1 marcador de fim valido |
| 231 | `leak/upstream-echo` | ACHADO | o erro devolvido ao cliente repete o corpo do Ollama com conteudo do arquivo: "erro do servidor Ollama [HTTP 500 POST /api/generate]: bad input near: CONTEUD... |
| 232 | `refusal-no-call` | BLOQUEADO | 0 chamadas ao backend |
