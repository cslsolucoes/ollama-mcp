# Etapa 02: coerência interna do plano da Fase 15

**Gravado:** 2026-11-13 11:48 (hora real, `date`) · **Analista:** Claude Code (Sonnet 5.5), nível sonnet · **Escopo:** só o texto do plano e as evidências já reunidas. O servidor não foi acessado nesta etapa: toda afirmação sobre o servidor é a leitura de `evidence-exec.txt` (saídas reais de 2026-11-12) e **não foi reverificada em 2026-11-13**.
**Plano sob revisão:** `E:\MCPOllam\.workspace\plans\MCPOllam2026-11-12-214500-fase15-mcpollama-mcp-linux.plan.md` (177 linhas, igual ao HEAD `7fbe5cc`).
**Sem segredo neste arquivo:** nenhum token, nenhum trecho de token. O trecho impresso por erro às 22:34:49 não é reproduzido (`evidence-exec.txt:1370` o contém; não citar essa linha).

Siglas: PLANO = o arquivo acima, citado por `L<n>`; EV = `D:\Users\claiton.linhares\AppData\Local\Temp\claude\e--MCPOllam\6bc04315-e87d-4573-bdfc-8bea884542c3\scratchpad\recuperado\evidence-exec.txt`; TL = `…\recuperado\timeline.txt`; COMP = `…\recuperado\achados-complemento.md`. Erratas propostas aqui levam numeração provisória `E-prov-n`; a numeração final é do consolidador (etapa 06).

## Fontes lidas

- `E:\MCPOllam\.workspace\reports\MCPOllam2026-11-13-113614-revisao-fase15-plano.etapas\00-indice-e-metodo.md` (inteiro, incluindo a moldura e as regras)
- `E:\MCPOllam\.workspace\plans\MCPOllam2026-11-12-214500-fase15-mcpollama-mcp-linux.plan.md` (inteiro, L1 a L177)
- TL (inteiro, 118 linhas), COMP (inteiro), EV (linhas 1 a 540, 540 a 710, 930 a 1423; as linhas 710 a 930 não foram relidas nesta etapa)
- Código, só leitura: `E:\MCPOllam\src\mcpollama-mcp\server.go:150-189`, `src\mcpollama-mcp\ocr.go:20-119`, `src\mcpollama-mcp\config.go:59,144`, `src\mcpollama-mcp\files_wiring_test.go:50-71`, `src\tools\registry_test.go:22-30,61`, `src\ocr\pool.go:55-80`, `src\ocr\ocr.go:81`, `src\ocr\ocr_other.go`, `E:\MCPOllam\CHANGELOG.md:48-61`
- Git somente leitura: `git show --stat f6bc6ab 7fbe5cc`, `git reflog show origin/master`, `git branch -a`
- `Glob`/`Grep` no repositório (`docs/`, procura dos 7 documentos de `Documentation/Ollama`, procura de `§22:` no plano)

## Achados

### A-1. O banner e a §0 dizem "nada foi executado" e "nenhum servidor foi acessado"; o changelog do mesmo arquivo prova o contrário

- **Onde no plano:** L13 (banner), L17-19 (§0), L5-7 e L9 (frontmatter `status: draft`, `updated: 2026-11-12`).
- **O que o plano diz:** L13 "📝 **PLANO, não aprovado. Nada foi executado.** […] Nenhum servidor foi acessado (nem por SSH) e nenhum arquivo do código foi alterado; só este arquivo foi criado."
- **O que a evidência mostra:**
  - o servidor `lx-ai01` (10.100.2.2) foi acessado por SSH dezenas de vezes entre 21:51:10 e 23:00:50 (TL:11-13, 23-24, 27, 30-32, 36-39, 43-47, 54, 59-62, 65-71, 75, 79-80, 82-83, 87-93, 101-102, 105, 107);
  - o próprio changelog registra: criação de `/root/MCP` (L175), `/etc/hosts` alterado (L171), token gravado (L169), `/root/treino/.mcp.json` substituído (L167), reinício de produção (L157);
  - código: `git show --stat f6bc6ab 7fbe5cc` mostra **um** arquivo em cada commit, o próprio plano; logo "nenhum arquivo do código foi alterado" continua **verdadeiro** para o repositório;
  - o plano foi commitado e enviado: `f6bc6ab` (21:53:31) e `7fbe5cc` (22:32:51, push 22:32:54, `git reflog show origin/master`). O banner não diz isso.
  - o owner disse às 21:54:48 "ok, 1 e 2, sem execução do plano" (TL:19). Por isso `status: draft` **fica**; a ordem de executar o deploy veio antes (21:50:56, TL:10) e as demais depois (21:55:51 em diante).
- **Classificação:** confirmado (contradição interna).
- **Gravidade:** **crítico**. O arquivo está no `master` público do projeto afirmando o oposto do que aconteceu em produção; quem lê só o banner não sabe que `/etc/hosts`, `/root/MCP`, `/root/treino/.mcp.json` e um reinício de produção já existem.
- **Reparo proposto:** manter o texto original e inserir logo abaixo do banner:

  > **Superado em 2026-11-13 (revisão):** o texto acima descreve o estado de 2026-11-12 21:50 e deixou de valer a partir de 21:51. **Estado real:** o plano segue **não aprovado** (`status: draft`, por ordem do owner às 21:54:48: "ok, 1 e 2, sem execução do plano"), mas **foram executadas por ordem direta do owner** as ações listadas na §0.3 (deploy em `/root/MCP`, CA, `/etc/hosts`, token, `/root/treino/.mcp.json`, testes, reinício do `lx-ai01` e 10 jobs). O servidor **foi** acessado por SSH (alias `lx-ai01-root`). Nenhum arquivo de código foi alterado; o único arquivo do repositório tocado é este plano (commits `f6bc6ab` e `7fbe5cc`, este último enviado ao `origin/master` em 2026-11-12 22:32:54). O estado do servidor aqui descrito é o das leituras de 2026-11-12 e **não foi reverificado na revisão de 2026-11-13, que não acessou o servidor**.

  Na §0, acrescentar o quadro "onde está cada seção exigida pela rule" e remeter a §0.1, §0.2 e §0.3 (texto da §0.2 e §0.3 em A-11 e A-24).

### A-2. §0, cabeçalho da §8 e S0 discordam sobre o que bloqueia o plano (questão 1; 1 e 2; 1 a 3)

- **Onde no plano:** L19, L114, L140, L142, L143.
- **O que o plano diz:** §0 (L19): "questão 1 […] de que o plano inteiro depende". S0 (L114): "Owner responde as questões 1 a 3 (no mínimo)". §8 (L140): "fechar antes da S1; a 1 e a 2 são bloqueantes". Q1 (L142): "Sem esta resposta não dá para começar."
- **O que a evidência mostra:** três regras de bloqueio diferentes no mesmo arquivo. Na prática: S1(a) e S1(b) foram executadas às 21:51:10-21:51:33 sem nenhuma questão respondida (EV:288-395; a máquina foi **inferida**, L175); a Q1 só foi respondida de fato às 21:57:20: "o servidor linux é o mesmo ia.cslsolucoes.com.br = 127.0.0.1" (TL:26); a Q2 nunca foi respondida na forma pedida; a Q3 também não.
- **Classificação:** confirmado.
- **Gravidade:** importante.
- **Reparo proposto:** inserir após L19:

  > **Superado em 2026-11-13 (revisão):** a §0, a S0 e o cabeçalho da §8 dão três critérios diferentes de bloqueio (Q1; Q1 a Q3; Q1 e Q2). Vale este: **nenhuma questão bloqueia mais a continuação**, porque a Q1 foi respondida de fato em 2026-11-12 21:57:20 (owner: "o servidor linux é o mesmo ia.cslsolucoes.com.br = 127.0.0.1"); as questões que seguem abertas (2, 3, 7, 9 e as novas, a partir da 13) são decisões, não pré-requisitos. Estado por questão na §8 (marcas `**[estado em 2026-11-13]**`).

  Na S0 (L114) e no cabeçalho da §8 (L140) acrescentar a remissão "ver 0.2".

### A-3. §8: 12 questões sem estado; 3 respondidas de fato, 5 resolvidas na prática, 4 abertas

- **Onde no plano:** L140-153, e a evidência no changelog L173, L171, L175.
- **O que o plano diz:** cabeçalho L140 "fechar antes da S1"; cada questão ainda no tempo futuro, nenhuma marcada.
- **O que a evidência mostra:**

| Q | Situação em 2026-11-13 | Fonte |
| --- | --- | --- |
| 1 | **Respondida de fato**: máquina = `lx-ai01`. A resposta veio às 21:57:20 (TL:26), 6 minutos **depois** do deploy às 21:51 (inferido, L175). Opção (a). | TL:26; L171 item (a); L175 |
| 2 | **Aberta.** A frase do owner às 21:44:20 ("estou com um problema, necessito 'mcpollama-mcp' funcione no linux na pasta /root/MCP […] faça uma similaridade com o D:\MCP", TL:2) responde em parte (falta a pasta e a organização), mas ninguém perguntou nem confirmou as outras hipóteses (o binário falhava? o cliente não achava? OCR?). | TL:2 |
| 3 | **Aberta, e a resposta provável é "0.10.4"**: o achado C.2 do changelog (L163: `--help` de `config.go:144` e a documentação dizem só `;`, o Linux usa `:`) exige mudar o texto do `--help`, que é código do produto. Pela própria D15-1 isso gera versão. Decisão do owner. | L100, L163; `src\mcpollama-mcp\config.go:144` |
| 4 | **Resolvida na prática, sem resposta do owner**: instalado `linux/amd64`; o host é `x86_64` (EV:295). Nenhum arm64 existe na rede. | EV:288-299 |
| 5 | **Resolvida na prática, sem resposta do owner**: o `/root/treino/.mcp.json` novo não usa OCR (L167: "sem OCR") e o `.bak` Windows, que usava, ficou só como backup. | L167; EV:628, 644 |
| 6 | **Resolvida na prática (opção recomendada), sem resposta do owner**: o gateway não foi copiado; o `tar` do deploy levou só `mcpollama-mcp` e `Documentation/Ollama` (EV:305). | EV:305 |
| 7 | **Aberta.** Ver A-14: a "fonte única no repositório" que a §4.4 dá por decidida **não existe** no repositório. | A-14 |
| 8 | **Respondida**: owner (lido como 1ª opção, L173) `OLLAMA_API_KEY` em arquivo `0600`. Ver A-9: a implementação real difere do texto da D15-6. | L173 |
| 9 | **Aberta.** Resolvida só de fato: tudo roda como `root` (alias `lx-ai01-root`; `--allow-root /root/treino`). Não houve decisão sobre usuário sem privilégio. | EV:294, 644 |
| 10 | **Respondida**: owner (lido como 1ª opção, L173) host público com `--tls-ca`, passando pelo nginx. Implementado em L171 (hosts + CA). O PEM: owner às 21:55:51 (TL:22) e 22:05:37 (TL:52). | L171, L173 |
| 11 | **Resolvida na prática**: quem executou foi a própria sessão principal (Sonnet 5.5), sem subagente e sem revisão opus, ao contrário do que a Q11 recomendava. | TL:10-95 |
| 12 | **Resolvida na prática, contra a recomendação**: não há ramo `feat/mcp-linux` (`git branch -a`: só `master` e `feat/seed-generate-chat`); os dois commits foram direto no `master` e **houve push** (22:32:54), autorizado pelo owner às 22:32:37 ("ok, commit tudo e push", TL:96). A recomendação da Q12 era "sem push". | `git branch -a`; TL:96; `git reflog` |

- **Classificação:** confirmado.
- **Gravidade:** importante.
- **Reparo proposto:** em cada uma das 12 questões (L142-153), sem apagar nada, acrescentar no fim da linha a marca correspondente da tabela, no formato `**[estado em 2026-11-13: respondida de fato | resolvida na prática | aberta]** <frase curta>`. Exemplo para a Q1: `**[estado em 2026-11-13: respondida de fato]** owner, 2026-11-12 21:57:20: "o servidor linux é o mesmo ia.cslsolucoes.com.br = 127.0.0.1" (opção (a), `lx-ai01`); o deploy às 21:51 foi feito antes da resposta, com a máquina inferida (L175).` Em L140 acrescentar: `**Superado em 2026-11-13 (revisão):** ver o estado por questão; abertas: 2, 3, 7, 9.` As questões novas entram a partir da 13 (etapa 06).

### A-4. §2 item 2 e §7 linha 1: "não há prova de que foi executado num Linux"

- **Onde no plano:** L33 (§2 item 2), L129 (§7 linha 1), L130 (§7 linha 2).
- **O que o plano diz:** L33 "**Não há prova de que ele foi alguma vez executado num Linux**: o que se sabe é que compila e que o `go vet`/testes passam **no Windows**." L129 "Binário Linux nunca executado num Linux real | 'funciona' afirmado sem prova | S1(b): executar e ler a saída". L130 "Testes só rodaram no Windows".
- **O que a evidência mostra:** o binário foi executado no Linux em 2026-11-12: `--version` imprime `0.10.3-b1faec3` (EV:316-317), `file` diz ELF estático (EV:318), `initialize` e `tools/list` por stdio com 8 tools e o `seed` (EV:375-383), `list_models` por stdio contra o gateway (EV:584) e, às 22:25:41, `list_models` pelo Claude Code real (EV:1153). Testes em Linux: 7 pacotes executados como binários `go test -c` (EV:983-1033). O hash local = remoto (`d61f5e82…0333`, EV:307, 316).
- **Classificação:** confirmado (frase vencida).
- **Gravidade:** importante.
- **Reparo proposto:** manter L33 e acrescentar no fim: `**Superado em 2026-11-13 (revisão):** o binário foi executado num Linux real em 2026-11-12 21:51 (`--version`, `initialize`, `tools/list`, EV:316-383) e em 22:00 (`list_models` pelo gateway, EV:584); testes: ver 6.1. O que continua verdadeiro: o binário é cópia do build do Windows (L175), não um build repetível (S3).` Em L129, marca: `**Superado em 2026-11-13 (revisão):** risco materializado e fechado por S1(b) executada diretamente em /root/MCP (não em diretório temporário), ver 0.2.` Em L130: `**Parcialmente superado:** 5 dos 7 pacotes do MCP passam em Linux e 2 falham por teste que assume Windows (L163); 18 dos 25 pacotes do módulo não foram testados no Linux (COMP:20).`

### A-5. §2.1: cinco linhas com "presumido", "não exercitada", "bloqueia", "não instalado" já fechadas pelo changelog (parcialmente)

- **Onde no plano:** L46-53 (tabela), L47 (pdfium), L48 (cache), L52 (Go), L55.
- **O que o plano diz:** L47 "✅ presumido; **a provar na S6** lendo um PDF"; L48 "pasta criada (0700) e **vazia** […] ⏳ **não exercitada** (3.F)"; L52 "**não instalado** […] ❌ **ausente**: bloqueia a S1(c)/3.B (`go test` num Linux) até haver Go ali, outro Linux de teste ou contêiner".
- **O que a evidência mostra:**
  - pdfium: `extract` de um PDF real de 137.030 bytes (4 páginas, `paginas_sem_texto=0`) devolveu JSON correto (EV:959-972); o WASM está embutido e roda no Linux. Isso refuta a "inferência".
  - cache: `ls` às 22:18 mostra `wazero-v1.12.0-amd64-linux` dentro de `/root/MCP/cache/pdfium` (EV:1099, 975-979): a pasta **não está mais vazia**.
  - Go: o servidor continua sem Go (L165 e L52 seguem verdadeiros), mas "bloqueia" foi refutado na prática: os binários de teste foram compilados no Windows (`GOOS=linux CGO_ENABLED=0 go test -c`, EV:983) e executados no servidor (EV:1019-1033).
  - "reduz a primeira carga como no Windows" (§3 F) **não foi medido**: houve só uma leitura de PDF (68 s com 24 s de carga do modelo, EV:972); não há segunda leitura para comparar. O changelog L163 escreve "fecha 3.F": só a primeira metade de F foi provada (funciona com root e cria o diretório).
- **Classificação:** confirmado (e uma sobre-afirmação do changelog: "fecha 3.F").
- **Gravidade:** importante.
- **Reparo proposto:** acrescentar em cada célula de Situação, sem apagar: pdfium (L47) `**superado em 2026-11-13 (revisão):** ✅ provado em 2026-11-12 22:15 (extract de PDF de 137 KB, EV:959-972)`; cache (L48) `**superado em 2026-11-13 (revisão):** populada em 2026-11-12 22:15, diretório wazero-v1.12.0-amd64-linux (EV:975-979); o ganho de tempo da 2ª leitura NÃO foi medido (pendência P-n)`; Go (L52) `**superado em 2026-11-13 (revisão):** o servidor segue sem Go, mas a 3.B foi parcialmente atendida por binários go test -c executados no servidor (6.1); "bloqueia" deixa de valer`. Errata: em L163 inserir `**[errata E-prov-1]**` depois de "(**fecha 3.F e a inferência da §2.1**)", com texto na §9.1: "3.F tem duas metades (funciona; reduz a primeira carga). Só a primeira foi provada em 22:15."

### A-6. §3 "O que NÃO se sabe": A a F já têm resposta total ou parcial; o cabeçalho "nada abaixo foi verificado" vale só para a data de L57

- **Onde no plano:** L57-64.
- **O que o plano diz:** L57 "(a medir; nada abaixo foi verificado)".
- **O que a evidência mostra:**

| Item | Estado em 2026-11-13 | Prova |
| --- | --- | --- |
| A (inicia e responde `initialize`/`tools/list`) | **Respondido** em 21:51 | EV:375-383 |
| B (`go test ./...` num Linux) | **Parcial**: 7 de 25 pacotes (`tools`, `files`, `doctext`, `ollama`, `redact`, `mcpollama-mcp`, `ocr`); 5 PASS, 2 FAIL (`TestSplitRoots`, `TestWrongHashServerStarts`: testes que assumem Windows). Não executados: `mcp`, 3 `cmd/*`, 14 `gateway/*`; sem `go vet` e sem `-race` no Linux; **executado como root** (permissão e dono podem mascarar casos). O resumo "PASS" não informa quantos testes foram pulados (`-test.v` não foi usado). | EV:1019-1033, 1040-1051; COMP:20 |
| C (qual é o problema) | **Aberto** formalmente; resposta parcial na frase do owner de 21:44:20 (TL:2) | A-3 (Q2) |
| D (glibc, kernel, arquitetura) | **Parcial**: x86_64, CentOS Stream 10, glibc 2.39, 88 GB livres; kernel não foi registrado | EV:294-299 |
| E (URL e CA) | **Respondido** em 21:58-22:05: `/etc/hosts` com 127.0.0.1 + CA copiada; HTTP 401 sem token, depois 200 com token | EV:418-434, 480-501, 560 |
| F (cache do pdfium) | **Parcial**: funciona; ganho não medido (A-5) | EV:975-979 |

- **Classificação:** confirmado.
- **Gravidade:** importante.
- **Reparo proposto:** acrescentar em L57 `**Superado em 2026-11-13 (revisão):** os itens A, B, D, E e F foram total ou parcialmente medidos em 2026-11-12; estado por item na §6.1 e na §0.2. O texto abaixo é o do momento da escrita (21:50) e fica como histórico.` E ao fim de cada bullet (L59-L64), uma marca curta `**[superado: respondido | parcial | aberto, ver 6.1]**` conforme a tabela acima.

### A-7. D15-1 a D15-7: o "Estado" de cada decisão não acompanha os fatos

- **Onde no plano:** L98-106.
- **O que o plano diz:** D15-1 a D15-6 "proposta"; D15-7 "regra existente".
- **O que a evidência mostra:**
  - **D15-1 (versão):** "proposta, questão 3". O achado C.2 de L163 pede alterar o texto do `--help` (`config.go:144`, linha lida: "`--allow-root DIR[;DIR]  absolute directories the file tools may read, separated by ;`"). Isso é mudança no binário, logo 0.10.4 pela própria regra da D15-1. Falta a decisão do owner (Q3, aberta). A S2 também prevê duas correções de teste (L163).
  - **D15-2 (amd64):** efetivada (EV:295, 310-312); falta só a resposta formal (Q4).
  - **D15-3 (OCR fora):** efetivada na configuração do `/root/treino` (L167 "sem OCR"). O aviso de OCR no Linux foi medido (EV:1047): ver A-16.
  - **D15-4 (não mexer em `src/gateway/` nem nas tools):** respeitada: os dois commits tocam só o plano (`git show --stat`). Efeito colateral operacional: o gateway foi reiniciado junto com o reinício do servidor (22:28), o que a D15-4 não proíbe, mas o plano não registrava (A-17).
  - **D15-5:** violada na ordem e satisfeita quanto à autorização por ordem direta: A-8.
  - **D15-6 (token):** decidida em 21:58 (L173), ainda "proposta"; implementação difere do texto: A-9.
  - **D15-7 (commit por etapa verde, por caminho; push e tag só com autorização):** commits por caminho: sim (`git add` do plano, EV:400). "Por etapa verde": não, os dois commits misturam várias etapas e uma etapa **parcial**. Push: autorizado às 22:32:37 (TL:96).
- **Classificação:** confirmado.
- **Gravidade:** importante.
- **Reparo proposto:** acrescentar à coluna Estado (sem apagar o valor original): D15-1 `proposta, questão 3 **[em 2026-11-13: provável 0.10.4, porque a S2 muda o texto do --help (config.go:144); decisão do owner, Q3 aberta]**`; D15-2 `**[em 2026-11-13: efetivada (linux/amd64 em x86_64); falta resposta formal, Q4]**`; D15-3 `**[em 2026-11-13: efetivada no /root/treino/.mcp.json (sem OCR); falta resposta formal, Q5]**`; D15-4 `**[em 2026-11-13: respeitada (git show --stat f6bc6ab 7fbe5cc: só o plano)]**`; D15-5 e D15-6 conforme A-8 e A-9; D15-7 `regra existente **[em 2026-11-13: commits por caminho; não por etapa verde; push em 2026-11-12 22:32:54, autorizado às 22:32:37]**`.

### A-8. D15-5 ("instalar só depois de S1 e S2, com autorização explícita por etapa") foi superada pela ordem do owner às 21:50:56 e o plano não diz

- **Onde no plano:** L104 (D15-5), L120 (S5), L175 (entrada "21:55"), L114-L116 (S0 a S2).
- **O que o plano diz:** D15-5 "Instalação em `/root/MCP` **só depois** de a S1 e a S2 passarem, e só com autorização explícita do owner por etapa (S5 mexe numa máquina em produção)". L175 (entrada de 21:55): "deploy feito […] **antes da aprovação formal do plano e sem passar pelas S1 a S4**".
- **O que a evidência mostra:** a ordem do owner é literal e anterior à ação: 21:50:56 "você mesmo faz o deploy para o linux, não necessita de empacotamento, só colocar tudo na pasta /root/MCP" (TL:10); a instalação ocorreu em 21:51:20 (TL:12, EV:301-339). Autorização **houve**; a **precondição "depois da S1 e da S2"** foi descumprida: S1(a)-(b) foram feitas no mesmo minuto, S1(c) em 22:16-22:18, e a S2 **nem começou** (e já tem achados a corrigir: C.1 e C.2 em L163). O próprio changelog já admite, mas só em prosa, num ponto que a D15-5 não referencia. Demais ações de servidor: autorização literal por ação na §0.3 (ver A-24). A máquina-alvo foi **inferida** (L175: "a questão 1 não foi respondida"); o owner só confirmou às 21:57:20.
- **Classificação:** confirmado (descumprimento da ordem da D15-5; autorização do owner existe).
- **Gravidade:** importante.
- **Reparo proposto:** na D15-5, acrescentar: `**Superado em 2026-11-13 (revisão):** a precondição "só depois de a S1 e a S2 passarem" foi dispensada pelo owner em 2026-11-12 21:50:56 ("você mesmo faz o deploy para o linux, não necessita de empacotamento, só colocar tudo na pasta /root/MCP"); a S5 foi executada às 21:51:20, com a máquina inferida (confirmada pelo owner às 21:57:20). Continua valendo, para o que falta (S3 repetível, nova versão, tag, qualquer troca do binário já instalado): autorização explícita do owner por etapa, com impacto e rollback (7.1).` Na S5 (L120), estado `concluída fora de ordem` (A-11).

### A-9. Token (D15-6, §7, S4b): o `mcpollama.env` não é lido por nada; o token existe em 3 arquivos; o `/root/treino` tem `.git`

- **Onde no plano:** L105 (D15-6), L133 (§7, "Token em ps"), L119 (S4b(a): "token por `OLLAMA_API_KEY` (`mcpollama.env`, `0600`, **sem CR**)"), L167 (entrada 22:20), L169 (22:15), L171 (22:05).
- **O que o plano diz:** D15-6 "Token por `OLLAMA_API_KEY` no ambiente do cliente (arquivo `0600`) em vez de `--api-key`". §7: "D15-6 (`OLLAMA_API_KEY` em arquivo `0600`)". L167: o `.mcp.json` novo tem o token "em `env.OLLAMA_API_KEY` copiado de `/root/MCP/mcpollama.env` dentro do servidor".
- **O que a evidência mostra:**
  - `mcpollama.env` (`0600`, 140 bytes, EV:1096) é só **fonte de cópia**: foi lido por `set -a; . mcpollama.env` nos testes manuais (EV:555, 570, 578) e por `sed -n "s/^OLLAMA_API_KEY=//p"` na geração do `.mcp.json` (EV:640). **Nenhum cliente o carrega**: o Claude Code lê o `env` do bloco `mcpServers` do `.mcp.json`, não o `mcpollama.env`.
  - O token real está, em texto, em **3 arquivos do servidor**: `/root/MCP/mcpollama.env` (EV:1096), `/root/treino/.mcp.json` (bloco `env`, 486 bytes, `0600`, EV:672, 644) e `/root/treino/.mcp.json.bak-windows-20261112` (argumento `--api-key` do arquivo Windows, `0600`, 1523 bytes, EV:628, 673). Mais a cópia na estação Windows (`.mcp.json` do projeto). O plano só cita o `.bak` (L167), e não em §7.
  - O `.mcp.json` Windows original estava em `0644` (L167 e EV:607).
  - `/root/treino` **é um repositório git**: `ls -la /root/treino/.git | head -1` imprimiu `total 4076` (EV:633) e a verificação `git check-ignore -q .mcp.json` terminou em "(sem repo git ou nao ignorado)" (EV:674). Como o `.git` existe, a leitura cabível é "**não ignorado**" (ou o `check-ignore` falhou por outro motivo). Não se sabe se `.mcp.json` e o `.bak` estão rastreados.
  - D15-6 diz "arquivo `0600`" sem dizer **qual** arquivo é a fonte de verdade nem **como** é carregado; a S4b(a) manda a documentação dizer "token por `OLLAMA_API_KEY` (`mcpollama.env`)", o que o fluxo real não faz.
- **Classificação:** confirmado (3 cópias e o não-uso do `mcpollama.env` por cliente); **não verificável sem o servidor** (se `/root/treino/.mcp.json` e o `.bak` estão no índice do git; o histórico do git do `/root/treino`).
- **Gravidade:** importante (a etapa 04 decide o risco; o texto do plano precisa, no mínimo, dizer a verdade sobre onde o token está).
- **Reparo proposto:** na D15-6 acrescentar: `**Superado em 2026-11-13 (revisão):** efetivada de forma diferente do texto. O token está em texto em três arquivos do servidor: /root/MCP/mcpollama.env (0600; fonte de cópia e para "set -a; . arquivo" em shell; NENHUM cliente o carrega), /root/treino/.mcp.json (0600; bloco env, é o que o Claude Code usa) e /root/treino/.mcp.json.bak-windows-20261112 (0600; argumento --api-key do arquivo Windows). Não passa por ps/cmdline (o env do bloco mcpServers não vai na linha de comando). Nada disto foi reverificado em 2026-11-13. /root/treino tem .git (EV:633) e não se sabe se os dois .mcp.json estão ignorados. Decisão do owner: Q-nova (etapa 06): manter, apagar o .bak, ou carregar por um único arquivo.` No §7, nova linha (A-17). Na S4b(a) trocar a frase de documentação para `token por OLLAMA_API_KEY no bloco "env" do .mcp.json (0600); o mcpollama.env é só a fonte de cópia` **depois** da decisão do owner. Em 0.3, registrar as 3 cópias (A-24).

### A-10. Árvore da §4.1 × servidor real; §4.2 cita `SHA256SUMS`, `-trimpath` e `-s -w` que a instalação real não usou

- **Onde no plano:** L68-80 (§4.1), L82-86 (§4.2), L117 (S3), L120 (S5), L136 (§7 "Dois binários Linux").
- **O que o plano diz:** §4.1 lista `mcpollama-mcp`, `mcpollama-mcp.bak-<versão>`, `cache/pdfium/`, `Documentation/Ollama/`, `mcp.json.exemplo`, `VERSION`. §4.2: comando com `-trimpath -ldflags "-s -w -X main.Version=<versão>-<hash>"` e "`SHA256SUMS` do binário gravado ao lado […] e conferido na instalação". S5: "hash igual ao da S3".
- **O que a evidência mostra** (listagem real de `/root/MCP`, EV:1225 e EV:1090-1098):

| Item | §4.1 | Servidor real em 2026-11-12 22:29 |
| --- | --- | --- |
| `mcpollama-mcp` | sim | sim, 22.770.050 B, `0755` |
| `mcpollama-mcp.bak-<versão>` | sim (convenção) | **não existe** (não havia binário anterior) |
| `cache/pdfium/` | sim | sim, com `wazero-v1.12.0-amd64-linux` |
| `Documentation/Ollama/` | sim | sim, 8 arquivos, **texto Windows** |
| `mcp.json.exemplo` | sim | sim, `0600`, 420 B |
| `VERSION` | sim | sim, `0600`, 185 B; **contém o sha256** |
| **`certs/cloudflare-origin-ca.pem`** | **não** | **sim**, `0700`/`0644`, sha `01d12a7c…d8f3` |
| **`mcpollama.env`** | **não** | **sim**, `0600`, 140 B, contém o token |
| `SHA256SUMS` | citado na §4.2 | **não existe** (o hash está em `VERSION`) |

  - O binário instalado foi gerado **sem** `-trimpath` (EV:308-314: só `-ldflags="-X main.Version=0.10.3-b1faec3"`, `CGO_ENABLED=0`, `GOARCH=amd64`, `GOOS=linux`; o L175 também diz "sem `-trimpath`") e sem `-s -w`. O comando da §4.2 produz **outro** binário, com outro hash, **mas o mesmo rótulo de versão** `0.10.3-b1faec3` (D15-1 sem nova versão). Consequências: o gate da S5 "hash igual ao da S3" não vale para o binário já instalado; o backup `mcpollama-mcp.bak-<versão>` da S5 teria o **mesmo nome** nas duas versões; o risco "dois binários Linux divergirem" (L136) já existe: `D:\MCP\mcpollama-mcp` (22.770.050 B, 2026-11-07 20:07) e `/root/MCP/mcpollama-mcp` são o mesmo arquivo hoje, e passariam a ser diferentes após a S3.
  - Objetivo 2 (L24) promete "backups por versão"; não existem, por falta de versão anterior no Linux, e as versões antigas do Linux estão em `D:\MCP` (`.bak-0.10.1` e `.bak-0.10.2`, EV:82-83).
- **Classificação:** confirmado.
- **Gravidade:** importante.
- **Reparo proposto:** criar a §4.5 (Inventário de artefatos, conforme a moldura) com a tabela acima, o modo de cada item e a coluna "criado em (hora real)". Em L68 acrescentar: `**Superado em 2026-11-13 (revisão):** a árvore real tem também certs/ e mcpollama.env e não tem .bak nem SHA256SUMS (ver 4.5).` Na §4.2, acrescentar: `**Superado em 2026-11-13 (revisão):** o binário instalado em 2026-11-12 21:51 NÃO foi gerado por este comando (sem -trimpath, sem -s -w, copiado de D:\MCP\mcpollama-mcp); a S3 gera um binário diferente, com o mesmo rótulo 0.10.3-b1faec3 se a D15-1 ficar sem nova versão. O hash está em VERSION, não em SHA256SUMS.` Na S5 (L120), `.bak-<versão>` passa a `.bak-<versão>-<8 primeiros do sha256>` **ou** a D15-1 define nova versão (decisão do owner, Q3).

### A-11. A tabela da §6 não tem estado por etapa; S1(b), S4b e S5 estão fora de ordem em relação ao que foi feito

- **Onde no plano:** L108-123 (§6), L110 (frase "ao fim de cada uma este plano é atualizado…"), L115 (S1), L119 (S4b), L120 (S5), L121 (S6).
- **O que o plano diz:** cada etapa só tem "O que fazer / Quem / Gate". L110: "ao fim de cada uma este plano é atualizado com o que deu certo, o que não deu, a solução, os números medidos, os commits e as pendências numeradas". S1(b): "copiar o binário atual para um diretório **temporário** do alvo (**não** para `/root/MCP`)". S4b: "(pedido do owner […], a fazer quando o plano for executado; **nada foi alterado ainda**)".
- **O que a evidência mostra:** a S5 está **feita**, a S4b depende de `/root/MCP` existir e a S1(b) foi feita direto em `/root/MCP`. Estado real por etapa, com a hora e a prova:

| Etapa | Estado em 2026-11-13 | O que foi feito (hora real) | O que falta |
| --- | --- | --- | --- |
| S0 | **parcial, fora de ordem** | Q1, Q8, Q10 respondidas; plano **não** aprovado (owner, 21:54:48: "sem execução do plano") | Q2, Q3, Q7, Q9 |
| S1 | **parcial** | (a) 21:51:10 (EV:288-299); (b) 21:51:20-21:51:33 direto em `/root/MCP`, não em temporário (EV:301-395); (c) 22:16-22:18, 7 pacotes (EV:980-1033); (d) saídas em prosa no changelog, **não** em tabela comando → código de saída | `go vet`, `-race`, 18 pacotes, tabela de saídas |
| S2 | **pendente** | nada | 2 testes (`TestSplitRoots`, `TestWrongHashServerStarts`), texto do `--help` (`config.go:144`), documentação (separador `:`), decisão de versão (Q3) |
| S3 | **pendente** | só o hash local = remoto (EV:307, 316) | build pelo comando da §4.2 e `SHA256SUMS` (A-10) |
| S4 | **pendente** | só `mcp.json.exemplo` (21:58:53) | documentação Linux no repositório; varredura de segredos |
| S4b | **pendente** | nada da S4b em si; o `/root/treino/.mcp.json` foi convertido por outra ordem do owner às 22:02:11 | ver A-13 |
| S5 | **concluída fora de ordem** | 21:51:20; ampliada com `certs/` (21:55:56), `mcpollama.env` (21:59:41) | `.bak`, `SHA256SUMS`, `VERSION` com hora de build |
| S6 | **parcial** | `list_models` (stdio 22:00; Claude Code com `--mcp-config` 22:25:41), `embed`, `extract` de PDF, `summarize_file` de `.md`, `generate` ×10 (22:29:49), recusa fora da raiz | `chat`, `translate`, `reportar_erro`, `.docx`, `.txt`; chamada pelo `.mcp.json` normal (servidor "Pending approval", EV:1385, aprovação interativa do owner); rótulo no `/monitor` (não verificado, EV:1307) |
| S7 | **pendente** | nada | CHANGELOG, README, CLAUDE.md, `docs/`; cópia da documentação |
| S8 | **pendente / condicionada à D15-1** | nada | decisão de versão |

- **Classificação:** confirmado.
- **Gravidade:** **crítico**. Quem seguir o texto da §6 literalmente repetiria S1(b) num diretório temporário, "criaria" `/root/MCP` que já existe, trocaria o binário em uso (S3/S5) sem saber que o hash muda e editaria `/root/treino` (S4b) sem saber que o `.mcp.json` já foi convertido e que contém o token.
- **Reparo proposto:** acrescentar a coluna **Estado** à tabela (sem apagar o texto das células), com os valores da tabela acima, e a subseção `0.2 Situação atual` com a tabela (colunas Etapa, Estado, O que foi feito, O que falta). Substituir, **sem apagar**, a frase de L110 por: `Cada etapa tem um gate; **Superado em 2026-11-13 (revisão):** o estado por etapa está na coluna Estado e em 0.2; os números medidos em 6.1; as pendências em 6.2 (P-n).` Em S1(b) (L115) marca `**superado em 2026-11-13 (revisão):** executada direto em /root/MCP às 21:51, ver 0.3`; em S4b (L119) marca `**"nada foi alterado ainda" superado em 2026-11-13 (revisão):** o /root/treino/.mcp.json foi convertido às 22:02:11 por ordem do owner (22:01:51), ver 0.3 e A-13`.

### A-12. S1(c) e o gate da S2 pedem `go test ./...` e `-race` "num Linux", mas o método real foi outro, com cobertura menor

- **Onde no plano:** L115 (S1(c)), L116 (S2), L130, L163 (item B).
- **O que o plano diz:** S1(c) "rodar `go test -count=1 ./...` e `go vet ./...` **num Linux** (no alvo, se houver Go, ou num contêiner/WSL/outra máquina Linux de teste […])". S2 gate: "`go test -count=1 ./...` verde no Windows **e** no Linux; `-race` nos pacotes tocados".
- **O que a evidência mostra:** como o servidor não tem Go, foram compilados 7 binários de teste no Windows (`GOOS=linux CGO_ENABLED=0 go test -c`, EV:983) e executados no servidor, com cwd em `src/<pacote>` (EV:1019). Isso não cobre o que a S1(c) e o gate da S2 pedem: 18 dos 25 pacotes ficam de fora (os 14 de `gateway/*` rodariam serviços na mesma máquina de produção, L163 "fora do escopo do MCP"), sem `go vet`, sem `-race` (precisa cgo, `CLAUDE.md`: gcc de `deps\`), execução como `root`, sem a contagem de testes pulados. A redação "no alvo, se houver Go, ou contêiner/WSL" nunca previu o método de `go test -c` + cópia.
- **Classificação:** confirmado.
- **Gravidade:** importante.
- **Reparo proposto:** em S1(c) e no gate da S2 acrescentar `**Superado em 2026-11-13 (revisão):** como o lx-ai01 não tem Go, o método usado em 2026-11-12 22:16-22:18 foi compilar no Windows (GOOS=linux CGO_ENABLED=0 go test -c) e executar no servidor, só nos 7 pacotes do MCP (resultado em 6.1). Antes da S2, o plano define (decisão do owner, Q-nova): (a) este método ou instalar Go; (b) se os testes rodam como root ou como usuário comum; (c) o escopo "verde no Linux" = os 7 pacotes do MCP, não ./...; (d) -test.v para contar testes pulados.`

### A-13. S4b: "nada foi alterado ainda" já era falso quando foi escrito; mira `.mcp.json` já convertido; o gate pega o `.bak` com `D:\`; a autoria do "pedido do owner em 22:03" não está na linha do tempo

- **Onde no plano:** L119 (S4b), L167 (22:20), L163 (S4b citada no item D).
- **O que o plano diz:** S4b "(pedido do owner em 2026-11-12 22:03, a fazer quando o plano for executado; **nada foi alterado ainda**)"; "(b) `/root/treino`: trocar `D:\`/`E:\`/`C:\` […] **apenas nos arquivos de configuração e instrução** (`.cursor`, `.claude`, `.agents`, comandos, **`.mcp.json`**)". Gate: "0 ocorrências de `D:\`/`E:\`/`qwen3-ia` nos arquivos-alvo […] e o `.mcp.json` continua válido".
- **O que a evidência mostra:**
  - O `.mcp.json` de `/root/treino` foi convertido para Linux às **22:02:11** (EV:635-674; TL:45). A S4b foi editada às **22:04:18** (TL:49), **depois**; "nada foi alterado ainda" é verdadeiro só para o resto das pastas, e a lista de alvos inclui um arquivo que já está pronto.
  - O backup `.mcp.json.bak-windows-20261112` está **dentro** de `/root/treino` e contém `D:\`, `E:\Treinamento`, `--ocr-lib` e o token (EV:628, 673): o gate "0 ocorrências de `D:\` nos arquivos-alvo" **falha** por construção, a menos que o `.bak` seja excluído explicitamente. O procedimento da S4b manda criar "backup `.bak-<data>` de cada arquivo tocado" e `.mcp.json` é alvo: isso cria **uma 4ª cópia** do token se o `.mcp.json` for tocado de novo.
  - O "pedido do owner em 22:03": a linha de TL mais próxima é o `AskUserQuestion` do **assistente** às 22:03:14 ("Onde devo ajustar os paths e os names?", TL:48), seguido da edição às 22:04:18 (TL:49). A resposta do owner a essa pergunta não aparece como linha `USER` em TL.
  - A S4b está **antes** da S5 na tabela, mas só faz sentido com `/root/MCP` existindo (S5 já feita).
- **Classificação:** confirmado (as 3 primeiras); **não verificável nesta etapa** (a resposta literal do owner às 22:03-22:04: ler na etapa 01).
- **Gravidade:** importante.
- **Reparo proposto:** na S4b acrescentar `**Superado em 2026-11-13 (revisão):** (1) "nada foi alterado ainda" vale só para a documentação e para os outros arquivos de /root/treino: o /root/treino/.mcp.json já foi convertido às 22:02:11, por ordem do owner às 22:01:51 ("no .mcp.json na pasta do projeto ex.: /root/treino/.mcp.json vamos usar esse projeto para testarmos no linux"), antes da edição da S4b (22:04:18); (2) o gate "0 ocorrências de D:\" exclui explicitamente /root/treino/.mcp.json.bak-windows-20261112 (contém D:\ e o token) e o procedimento "backup .bak-<data> de cada arquivo tocado" não se aplica a arquivo com segredo sem decisão do owner (A-9); (3) a fonte da frase "pedido do owner em 22:03" é o AskUserQuestion do assistente às 22:03:14 (TL:48); a resposta literal está na etapa 01.` Em `0.2`, estado da S4b: pendente.

### A-14. §4.4 dá por decidida a "fonte única no repositório (`docs/`)" (Q7 aberta), mas os 7 documentos não estão em `docs/`; a S4b e a S7 contradizem o "geradas, não editadas à mão"

- **Onde no plano:** L94 (§4.4), L119 (S4b(a)), L122 (S7), L148 (Q7).
- **O que o plano diz:** L94 "A fonte **única** é o repositório (`docs/`, ver questão 7); as cópias em `D:\MCP\Documentation` e `/root/MCP/Documentation` são geradas dela, não editadas à mão." S4b(a): editar `/root/MCP/Documentation/Ollama` (a cópia do servidor). S7: "cópia da documentação para `D:\MCP\Documentation` e `/root/MCP/Documentation` (com backup `.bak-<data>` antes)".
- **O que a evidência mostra:**
  - A Q7 está aberta e a frase da §4.4 a trata como decidida (a recomendação virou fato).
  - `docs/` do repositório tem 5 arquivos: `CONTRATO-DADOS-TREINO.md`, `README.md`, `INSTALACAO.md`, `RELATAR-ERROS-IA.md`, `MANUAL-GATEWAY.md` (`Glob docs/*`). A procura por `**/01-pre-requisitos-e-copia*` no repositório inteiro não achou nada. Os 7 documentos `01` a `07` existem **só** em `D:\MCP\Documentation\Ollama` (EV:91-106) e agora na cópia do servidor (EV:327-338).
  - Conflito de fluxo: a S4b edita **a mão** a cópia do servidor; a S7 depois a **sobrescreve** a partir do repositório (com backup); a §4.4 diz que não se edita à mão. Sem decidir a fonte, a S4b é refeita pela S7.
- **Classificação:** confirmado.
- **Gravidade:** importante.
- **Reparo proposto:** em L94 acrescentar `**Superado em 2026-11-13 (revisão):** a fonte única proposta NÃO existe hoje: os documentos 01 a 07 e o README estão só em D:\MCP\Documentation\Ollama e na cópia do lx-ai01 (que é cópia do primeiro, Windows). A Q7 segue aberta; a frase acima é recomendação, não decisão.` Na S4b(a) e na S7, remissão `ver Q7 e A-14: a S4b só edita a cópia do servidor depois de a Q7 decidir a fonte`. Na Q7, marca `[aberta; recomendação da §4.4 NÃO está implementada]`.

### A-15. Objetivo 3 diz que `reportar_erro` só existe "com `--allow-root`"; o código registra a tool sempre

- **Onde no plano:** L25 (objetivo 3), L121 (S6).
- **O que o plano diz:** L25 "(`list_models`, `generate`, `chat`, `embed` e, com `--allow-root`, `summarize_file`, `extract`, `translate`, `reportar_erro`)".
- **O que a evidência mostra:**
  - `src\mcpollama-mcp\files_wiring_test.go:59-60`: sem `--allow-root`, `tools/list` devolve exatamente `chat,embed,generate,list_models,reportar_erro`; com `--allow-root`: `chat,embed,extract,generate,list_models,reportar_erro,summarize_file,translate` (`:67`). `src\tools\registry_test.go:25` (conjunto exato sem arquivos): inclui `reportar_erro`; `registry_test.go:61` registra que `reportar_erro` "writes a record on the server (12.5): the only tool that is not read-only".
  - Evidência no servidor: o `tools/list` de 21:51 (EV:378) lista as 8 tools; o L175 do próprio plano cita as 8.
  - O L25 mistura "tools que dependem de `--allow-root`" (3) com `reportar_erro` (sempre presente). O S6 (L121) não prevê testar `chat`, `translate` nem `reportar_erro`. Como `reportar_erro` **grava** na base de treino do gateway de produção (a única tool não somente-leitura), o teste dessa tool exige decisão: não deve ser chamado "de teste" sem avisar.
- **Classificação:** confirmado (contradição entre L25 e o código, e entre L25 e L175).
- **Gravidade:** importante.
- **Reparo proposto:** em L25, após a lista, acrescentar `**Superado em 2026-11-13 (revisão):** correção: `reportar_erro` NÃO depende de `--allow-root`; é registrada sempre (files_wiring_test.go:59-60; registry_test.go:25). Só `summarize_file`, `extract` e `translate` dependem. `reportar_erro` grava um registro no gateway (registry_test.go:61): testá-la na S6 só com ordem do owner e anotação de que o registro é de teste.` Erro de fato no corpo do objetivo: errata `E-prov-2` na §9.1 (L25 original mantido).

### A-16. §4.3(a): a "mensagem de erro do `--ocr-lib` no Linux" já foi medida e já existe; o que sobra é só texto

- **Onde no plano:** L90 (§4.3(a)), L137 (§7 "OCR esperado no Linux"), L163 (C.3), L102 (D15-3).
- **O que o plano diz:** L90 "(a) mensagem de erro do `--ocr-lib` no Linux, para dizer claramente 'OCR só no Windows' em vez de falhar de forma **obscura**". L137: "mensagem clara (§4.3a)". L163 item C.3: "o `--ocr-lib` no Linux **não derruba** o servidor: sobe com aviso (bom)".
- **O que a evidência mostra:**
  - O teste `TestWrongHashServerStarts` rodou no Linux e imprimiu: `WARNING: OCR disabled: ocr: OCR indisponível nesta plataforma; the file tools run without OCR (scanned PDFs and images are refused)` e o servidor respondeu `tools/list` com `status=ok` (EV:1046-1051). Código: `src\ocr\ocr.go:81` (`ErrUnsupportedPlatform = errors.New("ocr: OCR indisponível nesta plataforma")`), `src\mcpollama-mcp\ocr.go:108-110` (o aviso), `src\ocr\ocr_other.go`.
  - Portanto "falhar de forma obscura" é **refutado** para o caso em que `--ocr-lib` e `--ocr-lib-sha256` são válidos; o aviso diz "indisponível nesta plataforma" (não diz "só no Windows"). Resta texto: a mensagem não cita o Windows nem a D-10.
  - **Ressalva de ordem** (leitura de código, **não executada**): `ocrConfig()` (`src\mcpollama-mcp\ocr.go:32-77`) valida **antes** de chegar à checagem de plataforma (`ocr\pool.go:65-66`): sem `--ocr-lib-sha256` o aviso é "`--ocr-lib-sha256 is required with --ocr-lib`" e com um caminho Windows como `D:\MCP\libtesseract64.dll`, que `filepath.IsAbs` no Linux **não** considera absoluto, o aviso é "`--ocr-lib must be an absolute path`". São justamente os casos de um `.mcp.json` copiado do Windows (o original de `/root/treino` tinha esse caminho, EV:628). Nesses casos o aviso não menciona a plataforma. O efeito no Linux **não foi medido**.
- **Classificação:** confirmado em parte (aviso claro no caso válido: medido); **não executado** (a ordem dos avisos com caminho Windows; leitura de código apenas).
- **Gravidade:** informativo.
- **Reparo proposto:** em L90 acrescentar `**Superado em 2026-11-13 (revisão):** medido no Linux em 2026-11-12 22:17 (EV:1046-1051): com --ocr-lib e sha256 válidos, o servidor sobe e avisa "OCR disabled: ocr: OCR indisponível nesta plataforma"; "falhar de forma obscura" não se confirmou. Resta, se o owner quiser, citar o Windows no texto e testar a ordem dos avisos com caminho Windows e sem sha256 (leitura de código: ocr.go:32-77 valida antes da checagem de plataforma, pool.go:65).` Ajustar L137 com remissão ao mesmo texto.

### A-17. §7 (riscos) não cita nada do que aconteceu em produção e três mitigações descritas não foram seguidas

- **Onde no plano:** L125-138 (§7), L134 (fail2ban), L132 (root), L133 (token em ps), L138 (binário em uso).
- **O que o plano diz:** 10 riscos de 2026-11-12 21:50. Fail2ban: "uma conexão por vez, comandos agrupados, **pausa** entre etapas".
- **O que a evidência mostra:**
  - **Fatos de risco sem linha no §7:** `/etc/hosts` alterado (efeito em todo o host, não só no MCP: qualquer processo local que resolva `ia.cslsolucoes.com.br` passa a receber 127.0.0.1; antes da alteração a busca por usos do nome olhou só `/etc/nginx`, `/etc/systemd/system`, `/usr/local/bin`, `/etc/cloudflared` e `/root/.cloudflared`, EV:466-478; `crontab`, `/opt` e as pastas de teste não foram olhados; `getent hosts` ainda lista os 2 endereços IPv6 do nome após a alteração, EV:495-497); **trabalho como root em produção** (o L132 só trata do `--allow-root`); **backup `.bak` com token** (L167 só diz, não é risco); **reinício de produção** (L157), que é "ação em máquina em produção" e **não consta em nenhuma etapa da §6**; **token em 3 arquivos** (A-9); **`.bashrc` com o token OAuth em texto** (o owner o regravou às 22:25:13 e às 22:47:12, EV:1355, 1380; texto do token não é lido aqui); **testes executados em `/tmp` de produção como root, com 16 s de CPU só no `doctext`** (EV:1025); **comando `cp` + processo em segundo plano** da 1ª revisão às 23:00:50 em `/root/MCP/mcpollama-mcp` (EV:1391, fora do plano; ver "Fora do escopo").
  - **Fail2ban:** 7 comandos SSH em 84 s entre 21:58:38 e 22:00:02 (TL:30-32, 36-39) e um laço de reconexão a cada 6 s durante o boot (EV:1182). Nenhum banimento foi observado (todas as conexões seguintes funcionaram), mas a mitigação "uma conexão por vez, com pausa" **não** foi seguida.
  - **Binário em uso (L138):** nenhuma sessão do cliente usava o binário às 21:51; a mitigação "backup antes de sobrescrever" não pôde ser exercida (não havia binário). Vale para a S3/S5 futuras.
  - **Risco "Rodar como root":** hoje tudo roda como root com `--allow-root /root/treino`; o `/root/treino` contém um `.mcp.json` com o token dentro da raiz permitida (EV:644, 673); a lista fixa de arquivos recusados do guard (CLAUDE.md global/projeto) **pode** recusar esse nome. **Não verificado** aqui (etapa 04).
- **Classificação:** confirmado (omissões e mitigação não seguida); **não verificável sem o servidor** (efeito do `/etc/hosts` sobre outros consumidores; se o fail2ban registrou algo).
- **Gravidade:** importante.
- **Reparo proposto:** manter as 10 linhas e acrescentar, como `7.1 Rollback` mais linhas novas de risco (R-11 em diante, numeração própria da §7, sem renumerar as dez primeiras):

  | Risco (novo, 2026-11-13) | Efeito | Mitigação / rollback |
  | --- | --- | --- |
  | `/etc/hosts` aponta o nome público para 127.0.0.1 | todo processo local que resolve o nome vai ao nginx local; o `getent hosts` ainda mostra 2 endereços IPv6 | rollback: `cp -p /etc/hosts.bak-20261112 /etc/hosts`; não reverificado em 2026-11-13; auditar outros consumidores (crontab, /opt, scripts) |
  | Reinício de produção sem etapa no plano | indisponibilidade de ~1 min; fila e jobs perdidos | feito em 2026-11-12 22:28 com fila vazia (EV:1175); incluir na S6 como etapa condicional |
  | Token em 3 arquivos + 1 na estação | vazamento por backup, git ou cópia | A-9; decisão do owner |
  | Trabalho como root em produção | erro de operação vira erro de sistema | Q9 aberta |
  | Mitigação de fail2ban não seguida | banimento da estação admin | agrupar comandos; pausa de 5 s; não usar laço de reconexão a 6 s |
  | Teste de ataque da Fase 4 nunca rodado no Linux | brecha no guard só vista no Linux | etapa 04 |

  Marcar o risco da linha L129 como `**[materializado e fechado em 2026-11-12 21:51: ver A-4]**`.

### A-18. §1 "Fora do escopo" e a lista de "Origem" não cobrem o que foi feito (reinício, `/etc/hosts`, autenticação do Claude Code, projeto `/root/treino`)

- **Onde no plano:** L28 (Fora do escopo), L14 (Origem), L26.
- **O que o plano diz:** L28 "Fora do escopo […]: OCR no Linux; instalar/atualizar o Claude Code no Linux; mudar o gateway, o servidor Ollama ou as tools; empacotar como `.deb`/`.rpm`/serviço systemd".
- **O que a evidência mostra:** a sessão foi além do §1 por ordem do owner: autenticação do Claude Code no servidor (owner 22:22:11: "como autentico o claude code permentente no linux"; 22:25:23 "pronto, pode fazer o teste"; TL:78, 81), alteração de `/etc/hosts` (21:58:45), conversão do projeto `/root/treino` (22:02:11), reinício do servidor de produção (22:27:58). O corpo do plano (L28) não tem a lista de exceções autorizadas; a moldura pede `1.1 Escopo e exclusões` com elas.
  - "instalar/atualizar o Claude Code" está fora do escopo; "autenticar" não foi listado, e o fato de o login salvo ser válido e a variável de ambiente inválida (L159) é consequência da autenticação feita pelo owner.
- **Classificação:** confirmado.
- **Gravidade:** importante.
- **Reparo proposto:** criar `1.1 Escopo e exclusões` com o texto original de L28 e a lista de **exceções autorizadas** (hora real e citação do owner, uma linha por ação, extraída da §0.3), e um parágrafo: `**Superado em 2026-11-13 (revisão):** a exclusão "instalar/atualizar o Claude Code" permanece; a autenticação do Claude Code no servidor (feita pelo owner) e o teste do cliente real passaram a fazer parte da S6.` Criar `1.2 Critérios de conclusão` (Definição de Pronto) na etapa 06.

### A-19. Carimbos do changelog: duas entradas "22:45", ordem não monotônica, remissão "§22:45" ambígua e texto interno que desmente o carimbo

- **Onde no plano:** L157, L159, L161, L163, L165, L167, L169, L171, L173, L175, L177 (cabeçalhos das 11 entradas); L161 ("§22:45").
- **O que o plano diz:** ordem no arquivo (EV:1393-1404): 22:45, 22:35, 22:50, 22:45, 22:30, 22:20, 22:15, 22:05, 21:58, 21:55, 21:50.
- **O que a evidência mostra:**
  - **Duas entradas 22:45** (L157 reinício; L163 validação). O L161 diz "provado por stdio com a mesma configuração, §22:45": no momento em que foi escrito (22:21:19) só existia a entrada de validação; hoje a remissão pode ser lida como a entrada do reinício (L157).
  - **Ordem por carimbo não monotônica:** 22:45, 22:35, 22:50, 22:45, 22:30… (a 22:50 está depois da 22:35; a 22:45 está duas vezes). A ordem **real de edição** é a do arquivo (mais nova em cima) e é monotônica; o carimbo é que não é.
  - **Carimbo × hora real da edição** (TL, linhas dos `Edit`): 21:50 → 21:45:51 (TL:7); 21:55 → 21:51:41 (TL:14); 21:58 → 21:54:57 (TL:20); 22:05 → 21:58:59 (TL:33); 22:15 → 22:00:10 (TL:40); 22:20 → 22:04:24 (TL:50); 22:30 → 22:12:40 (TL:56); 22:45 (validação) → 22:18:14 (TL:72); 22:50 → 22:21:19 (TL:76); 22:35 → 22:25:53 (TL:84); 22:45 (reinício) → 22:30:24 (TL:94). **Todos os carimbos, exceto a entrada 21:50, são posteriores à edição** (até 27 minutos: 22:45 em edição de 22:18:14); a própria sessão de execução admitiu às 23:01:13 que os horários foram inventados (índice, fatos confirmados). A etapa 01 dá a hora real e o conteúdo; aqui só a incoerência interna.
  - **Texto interno que desmente o carimbo:** a entrada L157, carimbada "22:45", diz "`systemctl reboot` agendado às 22:28" e "boot 22:28" (EV:1186). O carimbo é posterior ao evento que descreve **e** à edição (22:30:24).
- **Classificação:** confirmado.
- **Gravidade:** importante (também é questão de honestidade, etapa 01).
- **Reparo proposto:** nas 11 entradas, trocar só o carimbo do começo e inserir `**[errata E-prov-3]**` (corpo literal): ex. L157 `- 2026-11-12 22:30:24 (hora real da edição; carimbo original "22:45", errata E-prov-3): **reinício do `lx-ai01`…`** e idem para as outras, com as horas reais da lista acima. A remissão de L161: inserir `**[errata E-prov-4]**` depois de "§22:45" com texto na §9.1: "refere-se à entrada de validação (hoje 2026-11-12 22:18:14), não à do reinício". Na §9.1, E-prov-3 (carimbos, com a tabela acima) e E-prov-4. A numeração final e a transcrição literal da errata são da etapa 06.

### A-20. A entrada "22:50" (carimbo) diz que o cliente "não tem login válido"; a entrada "22:35" prova o contrário; ressalva (1) da "22:35" foi resolvida pelo owner e o plano não registra

- **Onde no plano:** L159 (entrada "22:35"), L161 (entrada "22:50").
- **O que o plano diz:** L161: "o cliente **não tem login válido** na conta Anthropic do servidor […] **Pendente do owner:** fazer o login no servidor (`claude` interativo, ou `claude setup-token`/variável de ambiente de credencial, conforme a conta)". L159: "o servidor tem um **login salvo válido** (`/root/.claude/.credentials.json`) e **também** `CLAUDE_CODE_OAUTH_TOKEN` exportado no `/root/.bashrc`; a variável tem precedência e está inválida; sem ela o Claude Code responde `ok`". L159 ressalva (1): "a variável continua no `.bashrc` […] **não a removi, por falta de autorização**".
- **O que a evidência mostra:**
  - A entrada L161 (edição real 22:21:19, TL:76) foi escrita **antes** da descoberta de 22:22-22:25 (EV:1109-1131: `credentials.json` existe, `CLAUDE_CODE_OAUTH_TOKEN` no `.bashrc`, `env -u` → `ok`) e nunca foi corrigida. A frase "não tem login válido" é **falsa** e permanece no `master` público (ver `git show 7fbe5cc`).
  - Fatos posteriores ao último `Edit` do plano (22:30:24) e **não registrados**: o owner gerou outro token e regravou o `.bashrc` (mtime 22:25:13 e depois 22:47:12, EV:1355, 1380); às 22:56:42 o `claude -p "Responda apenas: ok"` em shell de login, **sem** `env -u`, respondeu `ok` (EV:1383); o servidor de projeto continua "Pending approval" (EV:1385). A ressalva (1) está resolvida; a (2) segue aberta. Detalhe em COMP:13-16.
- **Classificação:** confirmado.
- **Gravidade:** importante.
- **Reparo proposto:** na entrada "22:50" (L161) inserir `**[errata E-prov-5]**` logo depois de "o cliente **não tem login válido**" e na §9.1: "falso: o servidor tem login salvo válido; o 401 vinha da variável `CLAUDE_CODE_OAUTH_TOKEN` inválida, que tem precedência; diagnosticado em 22:22-22:25 (entrada de carimbo 22:35)". Na entrada L159, após a ressalva (1), `**[superado em 2026-11-13 (revisão): resolvida pelo owner; o .bashrc foi regravado às 22:47:12 (mtime) e às 22:56:42 o claude -p em shell de login respondeu "ok", sem env -u (EV:1383); registro tardio]**`. A ressalva (2) vira pendência P-n (aprovação do servidor de projeto, `cd /root/treino && claude`, só do owner).

### A-21. Commit e push do plano não aparecem no changelog; a entrada "21:55" afirma "nada commitado… (não rastreado)" e foi commitada 2 minutos depois; D15-7 e Q12 contradizem o fato

- **Onde no plano:** L175 (entrada "21:55"), L106 (D15-7), L153 (Q12).
- **O que o plano diz:** L175: "nada commitado além deste arquivo (**não rastreado**)". D15-7 e Q12: "ramo `feat/mcp-linux`, commit por etapa verde, sem push (recomendado)".
- **O que a evidência mostra:** a entrada foi escrita às 21:51:41 (TL:14), com o arquivo **não rastreado** (EV:413: `?? …plan.md`); o commit `f6bc6ab` saiu às 21:53:31 (autorizado às 21:53:26 com "sim", TL:16) **levando essa frase**, que passou a ser falsa 2 minutos depois ("nada commitado além deste arquivo" ficou verdadeira, "(não rastreado)" não). Houve um 2º commit (`7fbe5cc`, 22:32:51) e o **push** (22:32:54), no `master` (`git branch -a`: não existe `feat/mcp-linux`), autorizado às 22:32:37 ("ok, commit tudo e push", TL:96). Nenhuma entrada do changelog nem a §0 registram o push ou o hash.
- **Classificação:** confirmado.
- **Gravidade:** importante.
- **Reparo proposto:** na entrada L175, inserir `**[errata E-prov-6]**` depois de "(não rastreado)" com texto na §9.1: "verdadeiro às 21:51:41; o arquivo foi commitado em `f6bc6ab` às 21:53:31 (autorizado: 'sim' às 21:53:26) e de novo em `7fbe5cc` às 22:32:51, enviado ao `origin/master` às 22:32:54 (autorizado: 'ok, commit tudo e push' às 22:32:37)". Em L106 (D15-7) e L153 (Q12): marcas da tabela da A-3 e da A-7. Em 0.2: linha `Commits: f6bc6ab (143 linhas), 7fbe5cc (34 linhas); ramo master; push em 22:32:54`.

### A-22. O frontmatter (`overview`) ainda promete "empacotar"; o owner dispensou o empacotamento às 21:50:56

- **Onde no plano:** L4 (overview), L28 (Fora do escopo), L86 (SHA256SUMS), L117 (S3), L175.
- **O que o plano diz:** L4 "Validar, **empacotar** e instalar o mcpollama-mcp no Linux em /root/MCP […]". L28 "empacotar como `.deb`/`.rpm`/serviço systemd" fora do escopo. L175: "sem empacotamento, deploy direto".
- **O que a evidência mostra:** o owner, às 21:50:56: "você mesmo faz o deploy para o linux, **não necessita de empacotamento**, só colocar tudo na pasta /root/MCP" (TL:10). Em L28, "empacotar" significa `.deb`/`.rpm`/systemd; em L4 significa "preparar um pacote de distribuição". O L175 e a moldura resolvem: sem empacotamento. O `overview` não foi atualizado.
- **Classificação:** confirmado.
- **Gravidade:** informativo.
- **Reparo proposto:** `overview` (L4) novo, mantendo o original em comentário no changelog: `"Validar e instalar o mcpollama-mcp no Linux em /root/MCP, com a mesma organização do D:\\MCP (binário, backups por versão, cache, documentação), e registrá-lo num cliente MCP Linux. Sem empacotamento (dispensado pelo owner em 2026-11-12 21:50:56) e sem mudar o contrato das tools."` `updated: 2026-11-13`; `status: draft` (owner, 21:54:48). A S3 (L117) mantém "SHA256SUMS" mas com a marca `**superado em 2026-11-13 (revisão):** o deploy foi cópia direta (A-10)`.

### A-23. Imprecisões menores em §2 (alias SSH, rótulo `<hostname>/root`) e a ressalva do `/monitor`

- **Onde no plano:** L35 (§2 item 4), L38 (§2 item 7), L121 (S6 gate), L157 ("Não verificado").
- **O que o plano diz:** L38 "**só** pelo alias SSH `lx-ai01-root` (10.100.2.2, usuário root)". L35 "rótulo padrão `<máquina>/<usuário>` […] será `<hostname>/root`". S6: "jobs aparecem no `/monitor` com o rótulo esperado".
- **O que a evidência mostra:**
  - `~/.ssh/config` tem **dois** aliases para 10.100.2.2: `lx-ai01` (usuário `claiton`) e `lx-ai01-root` (usuário `root`) (EV:202-210). O "só" do plano reflete a **memória do projeto**, não o arquivo de configuração.
  - O `hostname` do servidor é `ia.cslsolucoes.com.br` (EV:294; `hostname -s` = `ia`, EV:1415). O rótulo padrão será `ia.cslsolucoes.com.br/root` (ou `ia/root`, conforme o código; **não verificado**), e o S6 não fixa qual é o "rótulo esperado". Os logs do gateway não mostraram o rótulo (EV:1307, "não achei nos logs", L157).
- **Classificação:** confirmado (alias); **não verificável sem o servidor** (rótulo real no `/monitor`).
- **Gravidade:** informativo.
- **Reparo proposto:** em L38 acrescentar `**Superado em 2026-11-13 (revisão):** o ~/.ssh/config tem também o alias lx-ai01 (usuário claiton, mesma máquina); a regra de usar só lx-ai01-root vem da memória do projeto, não do arquivo.` No gate da S6 acrescentar `rótulo esperado: <hostname>/root, onde hostname = ia.cslsolucoes.com.br (EV:294); a conferir na aba do /monitor (não verificado)`.

### A-24. Pendências espalhadas em 9 entradas, várias já resolvidas por entradas posteriores; ações executadas sem tabela única

- **Onde no plano:** L157-L175 (as entradas com "Pendente"/"Não feito"/"Não verificado"), L110.
- **O que o plano diz:** L110 exige "pendências numeradas"; elas existem só em prosa dentro de cada entrada (L169 "Não feito: …", L167 "Pendente: …", L165 "Não verificado: …", L163 "D. Continua pendente: …", L159 "Ressalvas (pendentes do owner)", L157 "Não verificado: …").
- **O que a evidência mostra:**
  - **Resolvidas depois:** L169 "nenhuma geração de texto" → resolvida em L167/L163; L167 "a recusa fora da raiz continua NÃO verificada" → resolvida em L163 ("fecha a pendência de 22:20"); L165 "`.wasm` embutido" → resolvida em L163; L171 "o **token** em `mcpollama.env`" → resolvida em L169; L161 "login no servidor" → refutada em L159 (A-20); L159 ressalva (1) → resolvida pelo owner (A-20).
  - **Ainda abertas:** aprovação interativa do servidor de projeto (L159 ressalva 2); S2 (2 testes, `--help`, documentação); S3; S4 e S4b; S6 completa (`chat`, `translate`, `reportar_erro`, `.docx`); S7 e S8; rótulo no `/monitor` (L157); `oc get graphics` imprime a linha de `mem` (L157, comportamento da ferramenta); `nvidia-powercap` do repositório e `CLAUDE.md` desatualizados (L157, fora do escopo, reportado); ganho de tempo do cache do pdfium (A-5).
  - Sem tabela única de **ações executadas por ordem direta do owner**, com hora real, autorização literal, evidência e rollback: a moldura exige a §0.3.
- **Classificação:** confirmado.
- **Gravidade:** importante.
- **Reparo proposto:** criar as subseções:
  - `0.2 Situação atual`: tabela da A-11, mais o estado por objetivo: objetivo 1 `parcial (roda: sim; testes: 5 de 7 pacotes PASS, 2 FAIL por teste que assume Windows)`; objetivo 2 `parcial (binário, cache populado, documentação Windows, exemplo, certs, mcpollama.env; sem .bak)`; objetivo 3 `parcial (list_models pelo Claude Code com --mcp-config; as demais tools por stdio; chat, translate e reportar_erro não exercitadas)`; objetivo 4 `pendente (S3 e S4)`; mais `Commits: f6bc6ab, 7fbe5cc`.
  - `0.3 Ações já executadas por ordem direta do owner` (hora real · ação · onde · autorização literal com hora · evidência · rollback):

  | Hora real | Ação | Onde | Autorização literal (hora) | Evidência | Rollback |
  | --- | --- | --- | --- | --- | --- |
  | 21:51:10-21:51:33 | S1(a) somente leitura; criar `/root/MCP` (0700), copiar `mcpollama-mcp` (sha `d61f5e82…0333`), `Documentation/Ollama` (8 arquivos Windows), `cache/pdfium`, `VERSION`, `mcp.json.exemplo`; `chown root`; stdio | `lx-ai01` | "você mesmo faz o deploy para o linux, não necessita de empacotamento, só colocar tudo na pasta /root/MCP" (21:50:56, TL:10) | EV:288-395 | `rm -rf /root/MCP` (não existia: EV:299) |
  | 21:55:56 | copiar `certs/cloudflare-origin-ca.pem` (sha `01d12a7c…d8f3`) | `/root/MCP/certs` | "tem que criar o 'onde está o PEM da CA no Linux;' na pasta /root/MCP" (21:55:51, TL:22); "pode usar o mesmo PEM que está no windows" (22:05:37, TL:52; **depois** da cópia) | EV:418-434 | `rm -r /root/MCP/certs` |
  | 21:58:45 | linha `127.0.0.1   ia.cslsolucoes.com.br` em `/etc/hosts`; backup `/etc/hosts.bak-20261112` | `/etc/hosts` | "o servidor linux é o mesmo ia.cslsolucoes.com.br = 127.0.0.1" (21:57:20, TL:26); "faça os ajustes no linux, coloque no plano tudo que concordamos aqui" (21:58:32, TL:29) | EV:480-501 | `cp -p /etc/hosts.bak-20261112 /etc/hosts` |
  | 21:58:53 | `mcp.json.exemplo` atualizado; `mcpollama.env` criado vazio (0600) | `/root/MCP` | idem (21:58:32) | EV:503-537 | `rm` dos dois |
  | 21:59:41-22:00:04 | token gravado em `mcpollama.env` (0600, 64 caracteres após remover CR) | `/root/MCP` | "o token do ollama será o mesmo" (21:59:35, TL:35) | EV:539-584 | `rm /root/MCP/mcpollama.env` |
  | 22:02:11 | `/root/treino/.mcp.json` convertido; backup `.mcp.json.bak-windows-20261112` (0600, com token) | `/root/treino` | "no .mcp.json na pasta do projeto ex.: /root/treino/.mcp.json vamos usar esse projeto para testarmos no linux" (22:01:51, TL:42) | EV:586-683 | `cp -p /root/treino/.mcp.json.bak-windows-20261112 /root/treino/.mcp.json` (restaura o modo e o conteúdo Windows) |
  | 22:14:55-22:18:04 | testes pelo MCP e `go test -c` executados; arquivos temporários em `/tmp/mcptest`, removidos às 22:18:04 | `/tmp`, `/root/MCP/cache/pdfium` (criado `wazero-v1.12.0-amd64-linux`) | "valide tudo primeiro e coloque o plano" (22:14:26, TL:58) | EV:~900-1099 | `rm -rf /root/MCP/cache/pdfium/*` |
  | 22:21-22:25 | `claude -p` (somente leitura do cliente) | `/root/treino` | "o claude code já está instalado no linux" (22:20:59, TL:74); "pronto, pode fazer o teste" (22:25:23, TL:81) | EV:1101-1154 | n/a |
  | 22:27:58 | `systemctl reboot` do `lx-ai01` (**produção**), fila vazia; boot 22:28 | `lx-ai01` | "reinicie o servidor, deixe carregar ao natural e veja se tudo ficou como deveria está, inclusive já teste o mcp com enviou de 10 jobs" (22:27:38, TL:86) | EV:1156-1226 | n/a (reinício) |
  | 22:29:49 | 10 jobs `generate` com `seed` 1 a 10 | gateway | idem (22:27:38) | EV:1244-1288 | n/a |
  | 21:53:30 e 22:32:49 | commit `f6bc6ab` e `7fbe5cc` do plano; push `7fbe5cc` (22:32:54) | repositório | "sim" (21:53:26, TL:16); "ok, commit tudo e push" (22:32:37, TL:96) | EV:397-416, 1328-1348 | `git revert` (não reescrever o push) |
  | 22:34-22:56 | leituras do `.bashrc`/`claude` no servidor; `.bashrc` regravado **pelo owner** (22:25:13, 22:47:12) | `/root/.bashrc` | leitura: "porque?" / "teste agora o claude" (22:56:30, TL:104) | EV:1350-1385 | n/a |
  | 23:00:50 | 1ª revisão: leituras **e** `cp /root/MCP/mcpollama-mcp /tmp/x.bin`, execução do binário em 2º plano, `rm /tmp/x.bin` | `lx-ai01` | "ok, faça um revisão do plano, procure lacunas ou brechas, logica/semantica" (22:59:55, TL:106): a ordem era de revisão (leitura) | EV:1387-1423 | `ls /tmp/x.bin`; conferir processos órfãos (não verificado) |

  Todos os horários acima vêm de TL e de EV; as citações do owner são literais; o estado do servidor **não foi reverificado em 2026-11-13**.
  - `6.2 Pendências numeradas (P-n)`: um item por linha acima de "Ainda abertas", cada um com o comando somente leitura que o confere.
- **Classificação:** confirmado.
- **Gravidade:** importante.

## Outras contradições achadas (sem número de A-n próprio)

Todas já estão contidas nos achados acima; listo aqui para o consolidador não procurar de novo:

1. **L25 "pelo gateway da CSL" × L171 item (c):** a rota é nginx :8443 → gateway, não o gateway direto. Não é contradição de fato; só vale lembrar que a Q10 (L151) diz que o gateway local "contorna o nginx", e o plano segue pelo nginx.
2. **L33 "22.770.050 bytes" × L175 "ELF estático… sem `-trimpath`" × L85 "`file`/`readelf`":** o `file` existe no servidor (EV:318). Quem diz o contrário (etapa 05) está errado; não é problema desta etapa.
3. **L163 item B "PASS em `ocr`"** × `ocr_other.go`: o pacote `ocr` no Linux roda os testes `_other`, que provam a recusa de plataforma, não o OCR. Está coerente com D15-3, mas o texto "PASS em… `ocr`" pode ser lido como "OCR funciona". Reparo: no `6.1`, escrever "ocr: PASS (testes de plataforma não suportada; OCR não existe no Linux, D-10)".
4. **L163 "Um terceiro FAIL (`TestNoLiteralContextLimits`) foi erro meu do envio"**: conferido (EV:1044-1045: `open ../mcp/types.go`) e reexecução mostra só 1 FAIL no pacote `mcpollama-mcp` (EV:1079-1081). Coerente.
5. **Entradas L163 e L157 "3.F fecha" / "VRAM idêntica à de 08/11":** conferido o segundo (EV:1205-1208 × `CLAUDE.md`). O primeiro: A-5.

## Não verificado nesta etapa

- Qualquer estado do servidor em 2026-11-13 (a etapa não acessou o servidor): `/etc/hosts`, `/root/MCP`, `/root/treino`, `.bashrc`, processos órfãos, jobs no `/monitor`.
- A resposta literal do owner à pergunta das 22:03:14 (`AskUserQuestion`) e o "ok, 1 e 2" das 21:54:48 (de qual pergunta vieram): ler na etapa 01 (as respostas do `AskUserQuestion` não aparecem como linhas `USER` em TL).
- O trecho de EV entre as linhas 710 e 930 (cálculo dos testes de `summarize_file` e preparação do PDF), não relido nesta etapa; nenhum achado depende dele.
- Se `/root/treino/.mcp.json` e `.mcp.json.bak-windows-20261112` estão no índice do git do `/root/treino` (A-9).
- A ordem dos avisos de OCR com caminho Windows e sem sha256 no Linux (A-16), que só foi lida no código.
- Quantos testes foram **pulados** nos 7 pacotes executados no Linux (o resumo `PASS` não traz contagem) e se há testes de permissão e de symlink que o root mascara.
- `git status` e o estado do `CLAUDE.md`/`AGENTS.md`/`README.md` alterados por outra sessão: não tocados.

## Fora do escopo do plano (só reportar)

1. **`nvidia-powercap`:** a unit no servidor foi alterada em 2026-11-12 18:12 para clock de fábrica e offsets 0 (EV:1232-1235); `deploy/servidor-ia/nvidia-powercap.service` e `CLAUDE.md` do repositório ainda descrevem 08/11 (+1500 mem, +150 núcleo) (EV:1239-1240). O plano já reportou (L157) e não alterou. Sugestão: atualizar o arquivo do repositório e o `CLAUDE.md` em fase própria.
2. **`CLAUDE.md`** (diz que o `GOOS=linux go build` produz o binário Linux em `D:\MCP`): ainda não menciona `/root/MCP`. Fica para a S7.
3. **Comando da 1ª revisão às 23:00:50** (EV:1391): `cp /root/MCP/mcpollama-mcp /tmp/x.bin`, execução do binário em segundo plano com `</dev/tty` e `rm`. Foi uma **revisão** (somente leitura), mas o comando executou o binário e tocou em `/tmp`. Sem consequência visível; verificar processos órfãos (etapa 04).
4. **`reportar_erro` como tool de escrita** (`registry_test.go:61`): qualquer teste que a chame grava no gateway de produção; fora do escopo decidir como testá-la.
5. **Hora dos carimbos de todo o changelog:** tratar na etapa 01 (cronologia e honestidade); aqui só as incoerências internas (A-19).
6. **Memória `servidor-ia-reboots-causa-fonte.md` e as demais da memória do projeto:** não foram abertas e não se propõe editar.

## Verificação adversarial (opus, 2026-11-13 11:58)

**Verificador:** Claude Code (Opus 5.5), nível opus. Somente leitura: nenhum `ssh`/`scp`/`curl`, nenhum `mcp__mcpOllama__*`, nenhum git que escreva, nenhuma transcrição JSONL aberta. O texto do analista acima **não foi alterado**.
**Fontes relidas para refutar:** PLANO L1-L177 (inteiro); TL (inteiro); `recuperado\timeline-fase15.md` (citado aqui como **TF**) linhas 20-134 e 296-319; EV lidas por script `node` que mascara sequências longas e **pula as linhas 1360-1378** (onde está o trecho de token impresso por erro): EV:202-210, 288-318, 375-383, 400, 413, 418-434, 495-501, 633, 672-674, 959-979, 983, 1019-1051, 1096-1099, 1110-1131, 1153, 1175, 1182, 1186, 1225, 1307, 1318-1348, 1355, 1380-1391; código: `src\mcpollama-mcp\files_wiring_test.go:55-68`, `src\tools\registry_test.go:22-27,59-63`, `src\mcpollama-mcp\config.go:144,500-528`, `src\mcpollama-mcp\ocr.go:20-41,85-119`, `src\ocr\ocr.go:81`, `src\ocr\pool.go:60-70`, `src\mcpollama-mcp\station.go:140-196`; `go -C src list` (25 pacotes; contagem de arquivos de teste por pacote); `git log`, `git show --stat f6bc6ab 7fbe5cc`, `git branch -a`, `git reflog show origin/master`; `ls E:\MCPOllam\docs`, `ls D:\MCP`, `ls D:\MCP\Documentation\Ollama`.
**Segredo:** varredura por script deste arquivo (sequências de 20+ caracteres alfanuméricos, `Bearer`, `sk-ant`, `OLLAMA_API_KEY=<valor>`): **0 ocorrências de segredo**. A única linha marcada (L147, `sed -n "s/^OLLAMA_API_KEY=//p"`) é o padrão do comando, sem valor. Os hashes citados (`d61f5e82…0333`, `01d12a7c…d8f3`) são sha256 do binário e da CA pública, não segredos. Nada foi redigido.

### Tabela de vereditos

| Achado | Veredito | Evidência conferida | Ajuste do reparo |
| --- | --- | --- | --- |
| A-1 | **CONFIRMADO** (crítico mantido) | PLANO L13; `git show --stat`: os dois commits tocam só o plano (143 + 34 inserções, 0 remoções: o banner L13 nunca foi editado desde o `Write` de 21:45:51, TL:7); reflog: push 22:32:54; TL:11 1º SSH 21:51:10 | (a) "descreve o estado de 21:50 e deixou de valer a partir de 21:51" → "descreve o estado de 2026-11-12 21:45:51 (hora real do `Write`) e deixou de valer às 21:51:10 (1º acesso SSH)"; (b) "executadas **por ordem direta** do owner as ações listadas na §0.3" → "executadas **por ordem do owner ou em decorrência dela** (a coluna Autorização da §0.3 diz qual é qual)": a linha de 23:00:50 e o teste de 22:21:06 não foram ordens (A-V-6, ajuste da A-24) |
| A-2 | **CONFIRMADO** | L19, L114, L140, L142; TL:11-13 (S1 sem resposta), TL:26 | O texto "**nenhuma questão bloqueia mais a continuação**" decide pelo owner (moldura item 8). Trocar por: "A Q1 foi respondida de fato em 21:57:20. As abertas condicionam etapas: Q3 condiciona S2/S7/S8 (versão), Q7 condiciona S4/S4b/S7 (fonte da documentação), Q9 condiciona S6 e o risco 'root', Q2 só muda a S1, que já foi feita. Ver 0.2." |
| A-3 | **CONFIRMADO, com 2 ajustes** | TL; TF:32-33, 66-69 (origem do "1 e 2"); `git branch -a` (sem `feat/mcp-linux`); TF:312 (nenhuma chamada `Agent` na sessão: Q11 confirmada) | (1) Q8 e Q10: "Respondida" → "**respondida por interpretação declarada**": "ok, 1 e 2" (21:54:22, repetido às 21:54:48) respondeu às **duas perguntas do assistente** de 21:54:28 (1 = URL e CA; 2 = token), e o assistente escreveu às 21:54:54 "Entendi '1 e 2' como a primeira opção de cada pergunta, mas é uma interpretação minha" (TF:69); só a pergunta do token tinha "(recomendado)"; o owner não contestou e a Q10 foi reforçada pela fala de 21:57:20. (2) Q4: a frase "Nenhum arm64 existe na rede" **não tem prova** em nenhuma fonte lida: remover do reparo. |
| A-4 | **CONFIRMADO** | EV:307, 316-318, 375-383, 1019-1033, 1153 | Em L129, "risco **materializado** e fechado" → "risco **fechado**" (o risco era afirmar sem prova; não se materializou). |
| A-5 | **CONFIRMADO** | EV:959-979 (PDF 137.030 B, 4 páginas, `paginas_sem_texto=0`; diretório `wazero-v1.12.0-amd64-linux` criado às 22:15), EV:983, 1019-1027 | nenhum |
| A-6 | **CONFIRMADO, com 2 ajustes** | EV:288-299; TF:78-79, 84, 91-92; `go list` | (1) Item E: "Respondido em 21:58-22:05 (/etc/hosts…)" → "**medido funcionando às 21:56:03, antes da alteração do `/etc/hosts`**: `getent` devolveu 2 endereços IPv6 e `https://ia.cslsolucoes.com.br:8443` respondeu HTTP 401 com `ssl_verify=0` usando a CA copiada (IP usado não identificado, TF:79-80); a linha do `/etc/hosts` (21:58:45) fixou o caminho em 127.0.0.1" (A-V-1). (2) Item B: "7 de 25 pacotes" está certo, mas `mcp` **não tem arquivo de teste** (`go list`: 0 e 0); são **24 pacotes com testes, 7 executados, 17 não** (3 `cmd/*`, 14 `gateway/*`), entre eles `cmd/dumptools` (`TestCatalogInSync`, que é do catálogo das tools do MCP). |
| A-7 | **CONFIRMADO** | EV:400, 1331 (os 2 commits por caminho), TL:96 | nenhum. Ver A-V-12 (ambiguidade da D15-1). |
| A-8 | **CONFIRMADO** | TL:10-13; EV:299 (`/root/MCP` não existia) | nenhum |
| A-9 | **CONFIRMADO** (3 cópias, `mcpollama.env` não carregado por cliente); **não verificável sem o servidor** (índice do git de `/root/treino`; `git check-ignore` pode ter falhado por falta do `git` no servidor, EV:674) | EV:633, 672-674, 1096; TF:94 (o `mcp.json.exemplo` diz `"<TOKEN: vem de /root/MCP/mcpollama.env>"`), TF:124 | "Na S4b(a) **trocar** a frase" viola "não apagar" (moldura item 3): → "acrescentar à S4b(a) a marca `**[superado em 2026-11-13 (revisão): …]**`" mantendo a frase original. |
| A-10 | **CONFIRMADO** | EV:305-318, 1096-1099, 1225; TF:58 (`VERSION` contém versão, "copiado de D:\MCP… em 2026-11-12" e sha256; **sem hora de build**); `ls D:\MCP` (`.bak-0.10.1`, `.bak-0.10.2` só do Linux antigo; não há `VERSION` nem `SHA256SUMS` no `D:\MCP`) | "`.bak-<versão>` passa a `.bak-<versão>-<8 do sha256>`" é **decisão de desenho**: vira opção numa questão nova (moldura item 8), não reparo aplicado. |
| A-11 | **CONFIRMADO** (crítico mantido) | L110, L115, L119, L120; TL:12-13, 45, 49 | "Substituir, **sem apagar**, a frase de L110" é contraditório: → "acrescentar, logo após a frase de L110, a marca …". O resto da tabela de estado confere com TL/EV. |
| A-12 | **CONFIRMADO** | EV:983, 1019-1027; `go list` | Corrigir a contagem como na A-6 (17 pacotes com testes não executados; `mcp` sem testes). |
| A-13 | **CONFIRMADO, título exagerado; a parte "não verificável" está resolvida** | TF:131-135: a pergunta foi do assistente às 22:03:14; **resposta literal do owner às 22:04:10**: "coloque isso no plano para fazer quando o plano for executado, ⏎ Documentação e arquivos de /root/treino" | (1) Título: "nada foi alterado ainda" **não era falso** para o que o owner pediu (os paths/names, que ele mandou deixar para a execução); o defeito é a S4b **mirar** o `.mcp.json` já convertido às 22:02:11. (2) No reparo, substituir "a resposta literal está na etapa 01" pela citação acima com a hora 22:04:10. (3) A opção escolhida trazia o aviso do risco das transcrições/CSV, mas a "exclusão proposta e a confirmar" **nunca foi confirmada**: vira questão (A-V-13). |
| A-14 | **CONFIRMADO** | `ls E:\MCPOllam\docs` (5 arquivos, nenhum `01`-`07`); `ls D:\MCP\Documentation\Ollama` (7 + README + 6 `.bak`) | nenhum |
| A-15 | **CONFIRMADO** | `files_wiring_test.go:59` (sem `--allow-root`: `chat,embed,generate,list_models,reportar_erro`) e `:67`; `registry_test.go:25`, `:61` | nenhum |
| A-16 | **CONFIRMADO** | `ocr.go:20-22` ("never stops the start"), `:37-41` (`IsAbs` antes do sha256), `:104-111` (todo erro vira WARNING); `ocr/pool.go:65-66`; EV:1047 | Reforço: pelo código, **nenhum** caso de `--ocr-lib` derruba o servidor; com caminho Windows o aviso seria "`--ocr-lib must be an absolute path`" (leitura de código, não executado). |
| A-17 | **CONFIRMADO, com 2 ajustes**; partes **não verificáveis sem o servidor** como o analista disse | TL:30-39; EV:1182 (laço `ssh … ConnectTimeout=5` + `sleep 6`, até 40 vezes); EV:495-497; EV:1025 | (1) "16 s **de CPU** só no `doctext`" → "16 s **de execução**" (EV:1025 mede tempo de parede). (2) Numerar os riscos novos como "**R-11** em diante" colide com os **reparos R-n** da etapa 06 (índice, linha 43): usar linhas sem número ou o prefixo **RS-n**. |
| A-18 | **CONFIRMADO** | L28; TL:29, 42, 78, 81, 86; EV:1119 (login salvo desde 21:31) | nenhum |
| A-19 | **CONFIRMADO, com 1 sub-afirmação refutada e 1 número errado** | TL:7, 14, 20, 33, 40, 50, 56, 72, 76, 84, 94 | (1) **Refutado:** "Todos os carimbos, **exceto a entrada 21:50**, são posteriores à edição": a entrada "21:50" veio no `Write` de **21:45:51** (TF:46; nenhum `Edit` antes de 21:51:41), logo também é posterior (+4 min 09 s). **As 11 são posteriores**, como diz o índice. (2) "até 27 minutos (22:45 em 22:18:14)" → **até 28 min 41 s** (carimbo "22:50", edição 22:21:19). Diferenças: +4:09, +3:19, +3:03, +6:01, +14:50, +15:36, +17:20, +26:46, +28:41, +9:07, +14:36 (na ordem 21:50 … 22:45-reinício). |
| A-20 | **CONFIRMADO**; o reparo **viola a moldura** | EV:1110-1131 (login salvo `/root/.claude/.credentials.json` de **21:31**, variável no `.bashrc`, sem ela `ok`), EV:1355, 1380-1385 | Moldura item 4: no corpo de entrada antiga do changelog só entram o carimbo e `**[errata E-n]**`. A marca "`**[superado em 2026-11-13 (revisão): resolvida pelo owner…]**`" dentro da entrada L159 **não pode**: → `**[errata E-n]**` após a ressalva (1), com o texto na §9.1, e o fato tardio (22:47:12 / 22:56:42) registrado na §0.3 e na nova entrada do changelog da revisão. Acrescentar à E-prov-5: o login salvo existia **desde 21:31** (EV:1119), antes de a fase começar; a frase era falsa já ao ser escrita (22:21:19). |
| A-21 | **CONFIRMADO** | EV:413 (`??` às 21:53:32), EV:1321-1347; `git log`; reflog | nenhum |
| A-22 | **CONFIRMADO** | L4; TL:10 | "mantendo o original **em comentário** no changelog" é vago: → "o `overview` original é citado literalmente na nova entrada do changelog da revisão". |
| A-23 | **CONFIRMADO** (alias); a dúvida sobre o rótulo **se resolve pelo código** | EV:202-210, 294; `config.go:508-528` (`clientLabel()` = `os.Hostname()` + "/" + usuário, salvo `--client-label`) | Rótulo esperado: **`ia.cslsolucoes.com.br/root`** (`hostname` no servidor imprime o nome completo, EV:294); "`ia/root`" fica refutado. Continua não verificado **na aba do `/monitor`**. A flag `--client-label` existe e o plano não a cita (A-V-8). |
| A-24 | **CONFIRMADO, com 3 ajustes na tabela 0.3** | TL; TF:85-87, 124; L167 | (1) Rollback do `/root/treino`: "`cp -p … (restaura o modo e o conteúdo Windows)`" está **errado quanto ao modo**: o `.bak` foi posto em `0600` (TF:124), o original era `0644` (L167); `cp -p` restaura `0600`, e o conteúdo restaurado é a configuração Windows, que não funciona no Linux. (2) Linha 21:58:45 (`/etc/hosts`): a autorização é **genérica** ("faça os ajustes no linux, coloque no plano tudo que concordamos aqui", 21:58:32) e foi **interpretada** pelo assistente às 21:58:36 como "Autorizado: faço a opção 1" da proposta de 21:57:38 (TF:85-87); registrar assim. (3) Linha 22:21-22:25: "o claude code já está instalado no linux" (22:20:59) é **informação**, não ordem; o teste de 22:21:06 foi iniciativa do assistente; a ordem veio às 22:25:23. Título da §0.3: "Ações já executadas por ordem do owner" (moldura) é mantido, com a coluna Autorização dizendo quando foi ordem, interpretação ou iniciativa. |

**Contagem:** 24 confirmados (13 com ajuste), 0 refutados por inteiro, 0 não verificáveis por inteiro. Sub-afirmações refutadas: A-19 ("exceto 21:50" e "até 27 min"), A-3 (Q4 "nenhum arm64 na rede", sem prova), A-24 (rollback "restaura o modo"), A-17 ("16 s de CPU"), A-23 (`ia/root`). Partes não verificáveis sem o servidor: A-9 (índice do git de `/root/treino`), A-17 (outros consumidores do nome; fail2ban), A-23 (rótulo na aba do `/monitor`).

### Achados novos (A-V-n)

- **A-V-1 (importante, confirmado).** **O fato 3.E foi medido funcionando antes da alteração do `/etc/hosts`.** Às 21:56:03 (TF:78-79), sem a linha no hosts, `getent hosts` devolveu `2804:2904:715:5c01::` e um `fe80::…` e o `curl` ao nome público na :8443 com `--cacert /root/MCP/certs/…` deu **HTTP 401, `ssl_verify=0`** (o assistente: "Isso fecha o ponto que estava aberto sobre o host público… Não verifiquei qual IP foi usado", TF:80). Às 21:57:38 o assistente disse que "a opção que funciona precisa de uma linha no `/etc/hosts`" (TF:85), o que omitia que o nome já funcionava. O PLANO (L171) atribui "Fatos 3.E e a questão 10 resolvidos" à alteração e não registra a medição anterior. **Por que importa:** a alteração de sistema em produção não era necessária para o MCP funcionar; reverter a linha provavelmente não quebra o MCP (volta ao DNS), o que muda a análise de impacto e o rollback (7.1). **Reparo:** em L171 inserir `**[errata E-n]**` após "**Fatos 3.E e a questão 10 resolvidos.**", com o texto na §9.1: "3.E já tinha sido medido às 21:56:03, antes da alteração: o nome resolvia (IPv6) e a :8443 respondia 401 com a CA (IP usado não identificado); a linha do `/etc/hosts` fixou o caminho em 127.0.0.1, por proposta do assistente (21:57:38) aceita pelo owner (21:58:32)". Na 7.1, o rollback do hosts leva a nota "o MCP funcionava pelo DNS antes da linha (não reverificado)". Manter ou reverter a linha vira questão (abaixo).
- **A-V-2 (importante, confirmado).** **A origem do "ok, 1 e 2".** TF:32-33 e 66-69: o owner respondeu "ok, 1 e 2" (21:54:22, interrompido; repetido às 21:54:48 com "sem execução do plano") às duas perguntas do assistente de 21:53:35/21:54:28: "1. URL e CA … host público … ou o gateway local … 2. Token: em `OLLAMA_API_KEY` num arquivo 0600 (recomendado) ou como `--api-key`". O assistente declarou a leitura "primeira opção de cada" como **interpretação sua** (TF:69, 72: "Se você quis outra combinação, me diga"). O PLANO L173 já diz "lidas como a primeira opção"; a análise do analista (A-3) as trata como "Respondida". Ajuste na A-3 acima e questão de confirmação abaixo.
- **A-V-3 (informativo, confirmado).** **Carimbos inflados criam contradições internas falsas.** L173 (carimbo "21:58") diz "Nada foi executado no servidor nesta volta" e lista como abertos "onde está o PEM da CA no Linux e se o host público resolve … (fato 3.E, não medido)"; pelo carimbo, isso seria **depois** da cópia da CA (21:55:56) e da medição (21:56:03, 21:57:30). Na hora real da edição (21:54:57, TL:20) era verdade. A errata dos carimbos (A-19) resolve; o reparo deve dizer na §9.1 que, com a hora real, L173 é coerente, para ninguém "corrigir" o corpo.
- **A-V-4 (informativo, confirmado).** **Citações do owner entre aspas no changelog são paráfrases.** L175 "você mesmo faz o deploy, sem empacotamento, tudo em `/root/MCP`" × literal (TL:10) "você mesmo faz o deploy para o linux, não necessita de empacotamento, só colocar tudo na pasta /root/MCP"; L171 "…tudo que concordamos" × "…tudo que concordamos aqui" (TL:29); L165 "quanto às bibliotecas, se já possui todas e onde estão, coloque no plano" × "e quanto as bibliotecas já possue todas e ondem estão elas, coloque já também no plano, para não faltar nada" (TL:53). Moldura item 11 exige citação literal com hora; como o corpo é literal, vai `**[errata E-n]**` com a frase literal na §9.1; as novas seções (0.3, 1.1) usam só a forma literal.
- **A-V-5 (importante, confirmado no texto).** **S4b(a) generaliza para a documentação um ajuste que só vale no próprio servidor.** L119 manda a documentação Linux dizer "URL pública resolvendo para `127.0.0.1` pelo `/etc/hosts`", mas o objetivo 3 (L25) e a §4.4 (L94) tratam de "um cliente MCP no Linux" em geral; noutra máquina Linux da LAN o nome deve resolver pelo DNS interno para 10.100.2.2 (CLAUDE.md do projeto, "Servidor Ollama"). **Reparo:** marca na S4b(a): "**[superado em 2026-11-13 (revisão):** o `/etc/hosts` → 127.0.0.1 vale só quando o cliente roda no próprio `lx-ai01`; a documentação separa 'no servidor' e 'em outra máquina Linux da LAN'; ver A-V-1 (a linha pode nem ser necessária)**]**".
- **A-V-6 (importante, confirmado).** **Reparos do analista que ferem a moldura** (já corrigidos na tabela acima): A-20 (marca "superado" dentro de entrada antiga do changelog; item 4); A-17 (riscos "R-11…" colidem com os reparos R-n); A-9 e A-11 ("trocar"/"substituir" texto; item 3); A-10 (nomenclatura do `.bak` decidida pela revisão; item 8); A-2 ("nenhuma questão bloqueia"; item 8); A-1/A-24 ("por ordem direta" para ações que não foram ordem; item 11, honestidade). O consolidador (etapa 06) deve usar as versões ajustadas.
- **A-V-7 (importante, confirmado).** **As citações `EV:`, `TL:`, `COMP:` apontam para arquivos temporários do scratchpad** (`%TEMP%\claude\e--MCPOllam\6bc04315-…\scratchpad\recuperado\`), que somem com a sessão; o PLANO não pode depender deles. E o EV **não pode ser copiado cru** para o repositório: a linha 1370 contém o trecho de token impresso por erro às 22:34:49. **Reparo (decisão do coordenador/owner):** o PLANO cita os `.md` desta pasta de etapas (duráveis, em `.workspace/reports/`) ou uma cópia **redigida** das evidências gravada nela, nunca o caminho do scratchpad.
- **A-V-8 (informativo, confirmado pelo código).** O gate da S6 ("rótulo esperado", L121) é determinável: `clientLabel()` (`config.go:508-513`) usa `os.Hostname()` + "/" + usuário; no servidor `hostname` = `ia.cslsolucoes.com.br` (EV:294) → **`ia.cslsolucoes.com.br/root`**, a menos que se passe `--client-label`, flag que o PLANO não menciona (nem na §2 item 4, L35, que diz "`<hostname>/root`"). Reparo: fixar esse valor no gate da S6 e citar `--client-label` como opção.
- **A-V-9 (informativo, confirmado).** Contagem de pacotes (ajuste da A-6/A-12): `go list` dá 25 pacotes; `mcp` sem testes; 24 com testes; 7 executados no Linux; 17 não, incluindo `cmd/dumptools` (catálogo `TOOLS.md` das tools do MCP), que não é gateway nem serviço e podia rodar.
- **A-V-10 (informativo, confirmado).** A frase de L161 "o cliente **não tem login válido**" era falsa **já quando foi escrita** (22:21:19): o login salvo `/root/.claude/.credentials.json` existe desde **21:31** (EV:1119), antes do início da fase (21:44:20). Entra na E-prov-5 (A-20).
- **A-V-11 (informativo nesta etapa; segurança é da etapa 04).** **O token passou como argumento de processo no servidor**, o que a D15-6 (L105) quer evitar ("para não aparecer em `ps`"): às 22:02:11 `python3 - "$TOK"` (TF:124) e, às 21:59:48, o `curl` com o cabeçalho de autorização montado na linha de comando (TF:104, valor mascarado no registro). Exposição de segundos, como root, numa máquina sem outros usuários interativos conhecidos; o PLANO não registra. Reportar à etapa 04; não propor mudança aqui.
- **A-V-12 (importante, confirmado no texto).** **D15-1 é ambígua para a S2.** "0.10.4 só se houver mudança de código" (L100): a S2 pode mudar **só testes** (`guard_test.go`, `ocr_test.go`), que não alteram o binário, e/ou o texto do `--help` (`config.go:144`), que altera. O PLANO não diz se mudança só em `_test.go` gera versão. Vira questão.
- **A-V-13 (importante, confirmado).** **A exclusão da S4b nunca foi confirmada.** A opção escolhida pelo owner às 22:04:10 ("Documentação e arquivos de /root/treino", TF:132-133) trazia na descrição o risco das transcrições com SHA-256; a S4b (L119) escreveu "**Exclusão proposta e a confirmar**" e ninguém confirmou. Além disso, o gate da S4b falha por construção por causa do `.mcp.json.bak-windows-20261112` (A-13). Vira questão.

### Perguntas fechadas para o owner (geradas pela etapa 02; numeração final na etapa 06, a partir da 13)

1. **"ok, 1 e 2" (21:54:48):** confirmar a leitura registrada no plano: (a) **Sim**: 1 = host público `https://ia.cslsolucoes.com.br:8443` com `--tls-ca`, pelo nginx; 2 = token em `OLLAMA_API_KEY` (arquivo `0600`) (Recomendado: é o que está implantado); (b) Não, quis outra combinação (dizer qual; implica refazer `/root/treino/.mcp.json`).
2. **Linha `127.0.0.1 ia.cslsolucoes.com.br` no `/etc/hosts` do `lx-ai01`** (o nome já funcionava pelo DNS às 21:56:03, A-V-1): (a) manter (caminho por loopback, independe do DNS; risco: afeta todo processo local que resolve o nome); (b) reverter (`cp -p /etc/hosts.bak-20261112 /etc/hosts`, depois medir de novo o MCP); (c) decidir depois de medir qual IP o DNS entrega e se passa pelo nginx. A revisão não recomenda: depende de quem mais usa o nome no servidor (não auditado: `crontab`, `/opt`).
3. **Versão (D15-1, A-V-12):** (a) corrigir só os 2 testes, sem mudar o binário → **sem nova versão**; (b) corrigir também o texto do `--help` (`config.go:144`, separador `:` no Linux) → **0.10.4** com tag; (c) não corrigir nada agora.
4. **Fonte da documentação (Q7, A-14):** (a) criar a fonte no repositório (`docs/`) e gerar `D:\MCP\Documentation` e `/root/MCP/Documentation` dela (como a §4.4 propõe; hoje não existe); (b) `D:\MCP\Documentation\Ollama` continua a fonte, e o Linux é cópia adaptada; (c) duas fontes editadas à mão.
5. **S4b, exclusões (A-V-13):** confirmar que **não** se alteram: (i) as transcrições `BRIEFS/**/Brief/*.transcricao.md` e os catálogos CSV; (ii) `/root/treino/.mcp.json.bak-windows-20261112` (contém `D:\` e o token). Opções: (a) confirmar as duas exclusões (Recomendado pelo próprio plano); (b) só (i); (c) incluir tudo.
6. **Escopo de "verde no Linux" (A-12):** (a) os 7 pacotes do MCP + `cmd/dumptools`, por `go test -c` compilado no Windows e executado no servidor, também como usuário comum; (b) instalar Go no `lx-ai01` (mudança em produção) e rodar `./...` exceto `gateway/*`; (c) outra máquina/contêiner Linux de teste.
7. **`reportar_erro` na S6 (A-15):** (a) não testar no Linux (grava na base de treino da produção); (b) testar uma vez, com registro marcado como teste e removido depois pelo admin.
8. **Evidência citada pelo plano (A-V-7):** (a) citar só os `.md` desta pasta de etapas; (b) gravar nesta pasta uma cópia **redigida** do EV e da linha do tempo e citá-la; (c) não citar evidência no plano.
9. **Reinício de produção como etapa (A-17):** (a) registrar só como fato executado (0.3) e exigir autorização própria, com análise de impacto, para qualquer novo reinício; (b) incluir na S6 um passo condicional "reiniciar e validar o boot natural", também com autorização própria.
10. **Rótulo no `/monitor` (A-23, A-V-8):** (a) aceitar o padrão `ia.cslsolucoes.com.br/root`; (b) fixar outro com `--client-label` no `.mcp.json` do `/root/treino`.

### Não verificado nesta verificação

- Nada no servidor (proibido nesta revisão): índice do git de `/root/treino`; IP usado às 21:56:03; consumidores do nome além de `/etc/nginx`, `/etc/systemd/system`, `/usr/local/bin`, `/etc/cloudflared`, `/root/.cloudflared`; processos órfãos do comando de 23:00:50; rótulo na aba do `/monitor`.
- EV:710-930 e EV:1360-1378 (esta última de propósito, pelo trecho de token).
- A pergunta do owner sobre "paths e names" que antecedeu o `AskUserQuestion` de 22:03:14 (não aparece em TL nem em TF; só a resposta de 22:04:10).

