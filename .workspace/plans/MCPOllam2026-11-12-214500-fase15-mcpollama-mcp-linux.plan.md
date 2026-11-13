---
title: "Plano: Fase 15, mcpollama-mcp funcionando no Linux (/root/MCP, espelho do D:\\MCP)"
name: "Fase 15: mcpollama-mcp no Linux"
overview: "Validar, empacotar e instalar o mcpollama-mcp no Linux em /root/MCP, com a mesma organização do D:\\MCP (binário, backups por versão, cache, documentação), e registrá-lo num cliente MCP Linux. Sem mudar o contrato das tools."
created: 2026-11-12
updated: 2026-11-12
status: draft
locale: pt-BR
---

# Plano: Fase 15, mcpollama-mcp no Linux

> **Estado:** 📝 **PLANO, não aprovado. Nada foi executado.** Escrito a pedido do owner em 2026-11-12 21:44. Nenhum servidor foi acessado (nem por SSH) e nenhum arquivo do código foi alterado; só este arquivo foi criado.
> **Origem:** "necessito que o mcpollama-mcp funcione no Linux; na pasta `/root/MCP` (se não existir, crie) faça uma similaridade com o `D:\MCP`".
> **Relacionados:** Fase 14 (`0.10.3`, é a versão-base); `docs/INSTALACAO.md` e `deploy/servidor-ia/` (como o servidor Linux é montado); metodologia da §11 do plano da Fase 12 (gates, commits, armadilhas), que este plano **não repete**.

## 0. Para quem lê este documento

A §2 tem os fatos, lidos no código e em `D:\MCP` em 2026-11-12. A §3 diz o que NÃO se sabe e precisa ser medido. A §6 tem as etapas e a §8 as questões que o owner precisa fechar **antes da S1**, em especial a **questão 1 (em qual máquina fica `/root/MCP`)**, de que o plano inteiro depende.

## 1. Objetivo

1. O `mcpollama-mcp` roda e **passa nos testes** num Linux amd64 (e, se o owner quiser, arm64).
2. Existe em `/root/MCP` uma árvore **equivalente à do `D:\MCP`**: binário atual, backups por versão, cache do PDF, documentação de uso, e um exemplo de `.mcp.json` para Linux.
3. Um cliente MCP no Linux (Claude Code) consegue chamar as tools (`list_models`, `generate`, `chat`, `embed` e, com `--allow-root`, `summarize_file`, `extract`, `translate`, `reportar_erro`) pelo gateway da CSL.
4. O processo de build/implantação do binário Linux fica **repetível e documentado** (hoje ele é um efeito colateral do build do Windows).

**Fora do escopo** (cada item exigiria decisão própria, §8): OCR no Linux; instalar/atualizar o Claude Code no Linux; mudar o gateway, o servidor Ollama ou as tools; empacotar como `.deb`/`.rpm`/serviço systemd (o MCP é stdio, é iniciado pelo cliente).

## 2. Fatos de partida (lidos em 2026-11-12; HEAD do `master` após a Fase 14)

1. **O código já é majoritariamente portável.** Arquivos específicos de plataforma, todos com a contrapartida `!windows`: `mcpollama-mcp/station_wmi_{windows,other}.go` (WMI só existe no Windows; no Linux `wmiMACs()` devolve `nil`), `files/nlink_{windows,other}.go`, `ocr/*_windows.go` e `ocr/ocr_other.go`. O `files/guard.go` já ramifica por `runtime.GOOS` (`guard.go:120,200`) e trata `/` como raiz larga demais (`guard.go:207`).
2. **Já existe um binário Linux**, `D:\MCP\mcpollama-mcp` (22.770.050 bytes, 2026-11-07 20:07), gerado por `GOOS=linux go build` (CLAUDE.md, seção Comandos). **Não há prova de que ele foi alguma vez executado num Linux**: o que se sabe é que compila e que o `go vet`/testes passam **no Windows**. O build Linux feito na S1 da Fase 14 só provou compilação (`CGO_ENABLED=0`, 22.769.753 bytes).
3. **O OCR é só Windows por decisão registrada (D-10).** `ocr/ocr_other.go` (`//go:build !windows`): `platformSupported = false`, e `OpenTesseract` devolve `ErrUnsupportedPlatform`. Consequência: **PDF digitalizado e `.png`/`.jpg` seguem recusados no Linux**, mesmo com `--ocr-lib`. PDF com camada de texto funciona (go-pdfium em WASM/wazero, sem cgo).
4. **Cabeçalho da estação no Linux.** `station.go` lê `COMPUTERNAME`, `USERDOMAIN` e `PLACA` do ambiente (variáveis do logon do Windows). No Linux elas não existem: a máquina vem do hostname, o usuário de `user.Current()`, o domínio fica vazio e o MAC vem da interface ativa com IPv4 privado. É o comportamento previsto ("WMI → PLACA → interface"), mas o **rótulo padrão `<máquina>/<usuário>` no `/monitor` será `<hostname>/root`** se rodar como root.
5. **Estrutura do `D:\MCP`** (modelo do espelho): `mcpollama-mcp.exe` e `mcpollama-mcp` (Linux), backups `mcpollama-mcp.exe.bak-<versão>` e `mcpollama-mcp.bak-<versão>`, `mcpollama-gateway` (cópia do binário Linux do gateway), `cache/` (cache de compilação do pdfium, `cache\pdfium`), `tessdata/` e `libtesseract64.dll` (OCR, **sem uso no Linux**), `Documentation/Ollama/` (7 arquivos numerados `01` a `07` + `README.md` + `.bak-<data>`), além de binários de outros MCPs (`TruenasMcp.exe`, `mcpvmware-mcp.exe`) que **não** fazem parte desta fase.
6. **Documentação do uso** em `D:\MCP\Documentation\Ollama\` (`01-pre-requisitos-e-copia`, `02-mcp-json-do-zero`, `03-parametros`, `04-tools`, `05-relatar-erros`, `06-solucao-de-problemas`, `07-checklist-ia`) está escrita **para Windows** (caminhos `D:\`, PowerShell, `.exe`). Não há versão Linux.
7. **Acesso ao servidor Linux da CSL:** só pelo alias SSH `lx-ai01-root` (10.100.2.2, usuário root), conforme a memória do projeto. Muitas conexões SSH seguidas podem acionar o fail2ban e banir a estação admin (CLAUDE.md, "Segredos").
8. **Segredo:** o token do Ollama vai como argumento `--api-key` no `.mcp.json`, fora do git (decisão do owner de 2026-10-26). Num Linux multiusuário, os `args` aparecem em `ps`/`/proc/<pid>/cmdline`; a variável `OLLAMA_API_KEY` é a alternativa suportada (fallback das flags).
9. **O bakeoff** (`cmd/bakeoff`) tem caminhos Windows como padrão (`E:\MCPOllam\...`, `D:\Users\...`); **não** faz parte do binário do MCP e fica fora desta fase.

## 3. O que NÃO se sabe (a medir; nada abaixo foi verificado)

- **A:** se o binário Linux atual **inicia** e responde `initialize`/`tools/list` num Linux real (nunca foi executado lá).
- **B:** se `go test ./...` passa **num Linux** (hoje só foi rodado no Windows). Testes com caminhos, permissões, symlink e `nlink` têm comportamento diferente no Linux e o guard de arquivos (`src/files/`) tem testes de ataque que exigem repetição ao mexer nele.
- **C:** o que é "o problema" do owner: o binário Linux não roda? falta a pasta? o cliente não o encontra? o OCR? Não foi dito (questão 2).
- **D:** a versão do glibc/kernel, a arquitetura (amd64 ou arm64) e o sistema do alvo.
- **E:** se de dentro do servidor `lx-ai01` a URL `https://ia.cslsolucoes.com.br:8443` resolve e passa pelo nginx (split-DNS) ou se o certo é apontar para o gateway local; e onde está o PEM da CA Cloudflare Origin no Linux (`--tls-ca`).
- **F:** se `--pdf-cache-dir` em `/root/MCP/cache/pdfium` funciona com permissões de root e reduz a primeira carga do PDF como no Windows.

## 4. Desenho (proposta)

### 4.1 Árvore em `/root/MCP` (espelho do `D:\MCP`)

```
/root/MCP/
  mcpollama-mcp                      # binário atual (linux/amd64, ELF estático, CGO_ENABLED=0)
  mcpollama-mcp.bak-<versão>         # um backup por troca, mesma convenção do D:\MCP
  cache/pdfium/                      # --pdf-cache-dir
  Documentation/Ollama/              # 01..07 + README, versão Linux (caminhos /root/MCP, bash)
  mcp.json.exemplo                   # exemplo de .mcp.json para Linux, SEM token
  VERSION                            # versão-hash do binário e data (conferência)
```

Fora do espelho, de propósito: `libtesseract64.dll` e `tessdata/` (OCR é só Windows, D-10), `TruenasMcp.exe`, `mcpvmware-mcp.exe` e o `mcpollama-gateway` (o gateway já é instalado em `/usr/local/bin` por `deploy/gateway/`; copiar outro binário para `/root/MCP` criaria duas fontes de verdade, ver questão 6).

### 4.2 Build repetível

- Comando documentado e fixo: `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=<versão>-<hash>" -o <destino>/mcpollama-mcp ./mcpollama-mcp`, a partir de `src/`. `CGO_ENABLED=0` é **obrigatório** (o binário não pode depender do glibc do alvo).
- Conferir com `file`/`readelf` que é ELF estático e que `--version` imprime a versão esperada **executando no Linux**.
- `SHA256SUMS` do binário gravado ao lado (como no `Build-Release.ps1` do molde, MCPO-REQ-BUILD-004) e conferido na instalação.

### 4.3 Código

A princípio **nenhuma mudança de código** (S2 só atua se a S1 achar defeito). Candidatos a ajuste, **só se a medição mostrar necessidade**: (a) mensagem de erro do `--ocr-lib` no Linux, para dizer claramente "OCR só no Windows" em vez de falhar de forma obscura; (b) documentar `OLLAMA_API_KEY`/`OLLAMA_TLS_CA` como caminho recomendado no Linux. Qualquer mudança em `src/files/` exige rodar de novo os testes de ataque da Fase 4.

### 4.4 Documentação Linux

Versão Linux dos 7 documentos de `Documentation/Ollama/` (e do README), com caminhos e comandos bash. A fonte **única** é o repositório (`docs/`, ver questão 7); as cópias em `D:\MCP\Documentation` e `/root/MCP/Documentation` são geradas dela, não editadas à mão.

## 5. Decisões propostas (D15-n)

| # | Decisão | Estado |
| --- | --- | --- |
| D15-1 | Versão: **0.10.4** só se houver mudança de código; se a fase for só empacotamento/validação, **sem nova versão** e sem tag (o binário continua `0.10.3`) | proposta, questão 3 |
| D15-2 | Arquitetura alvo: **linux/amd64**; arm64 só se pedido | proposta, questão 4 |
| D15-3 | OCR no Linux **fora do escopo** (D-10 mantida); PDF digitalizado e imagens seguem recusados no Linux | proposta, questão 5 |
| D15-4 | Não mexer em `src/gateway/` nem no contrato das tools | proposta |
| D15-5 | Instalação em `/root/MCP` **só depois** de a S1 e a S2 passarem, e só com autorização explícita do owner por etapa (S5 mexe numa máquina em produção) | proposta |
| D15-6 | Token por `OLLAMA_API_KEY` no ambiente do cliente (arquivo `0600`) em vez de `--api-key` na linha de comando, para não aparecer em `ps` | proposta, questão 8 |
| D15-7 | Commit por etapa verde, por caminho; **push e tag só com autorização** (§11.1 da Fase 12) | regra existente |

## 6. Etapas

Cada etapa tem um gate; ao fim de cada uma este plano é atualizado com o que deu certo, o que não deu, a solução, os números medidos, os commits e as pendências numeradas.

| Etapa | O que fazer | Quem | Gate |
| --- | --- | --- | --- |
| **S0** | Owner responde as questões 1 a 3 (no mínimo) e aprova o plano | owner | respostas registradas na §8 |
| **S1** | **Diagnóstico somente leitura.** (a) Conferir o alvo: `uname -m`, `/etc/os-release`, `ldd --version`, espaço em disco, se `/root/MCP` existe (só `ls`); (b) copiar o binário atual para um diretório **temporário** do alvo (não para `/root/MCP`) e rodar `--version`, `initialize` e `tools/list` por stdio; (c) rodar `go test -count=1 ./...` e `go vet ./...` **num Linux** (no alvo, se houver Go, ou num contêiner/WSL/outra máquina Linux de teste, a definir na questão 1); (d) registrar cada saída. Uma conexão SSH por vez, com pausa, por causa do fail2ban | sonnet | tabela comando → código de saída → resultado; lista de falhas do `go test` no Linux (se houver) |
| **S2** | **Correções**, só se a S1 achou defeito (mensagem do OCR no Linux, testes dependentes de plataforma, caminho). Uma correção por commit; testes novos provados por mutação; repetir os testes de ataque de `src/files/` se ele for tocado | sonnet + revisão opus | `go test -count=1 ./...` verde no Windows **e** no Linux; `-race` nos pacotes tocados |
| **S3** | **Build Linux repetível:** comando da §4.2, `CGO_ENABLED=0`, `--version` conferido executando no Linux, `SHA256SUMS`; registrar tamanho e hash | sonnet | `file` diz ELF estático; hash conferido no destino; `--version` correto |
| **S4** | **Documentação Linux** (§4.4) e o exemplo `mcp.json.exemplo` sem token; varredura de segredos (meta 0) | sonnet | `git diff` só com o previsto; varredura 0 ocorrências |
| **S5** | **Instalação em `/root/MCP`** (só com autorização do owner): criar a árvore da §4.1, copiar o binário e conferir o hash, `chmod 0755` no binário e `0700` na pasta, criar `cache/pdfium`, gravar `VERSION`. Se já houver binário, guardar `mcpollama-mcp.bak-<versão>` **antes de sobrescrever** | owner + sonnet | árvore criada; hash igual ao da S3; `--version` correto no destino |
| **S6** | **Validação ponta a ponta no Linux:** registrar o MCP num cliente (Claude Code Linux) com o `.mcp.json` da §4.1, chamar `list_models`, um `generate` curto e `embed`; se houver `--allow-root`, um `summarize_file` de um `.txt` e de um PDF com texto; conferir no `/monitor` o rótulo da estação | owner + sonnet | cada tool responde; jobs aparecem no `/monitor` com o rótulo esperado |
| **S7** | **Fechamento:** CHANGELOG (seção "Não lançado" ou versão, conforme D15-1), `README.md` (nota "Linux"), `CLAUDE.md` (estado), `docs/` (fonte única); cópia da documentação para `D:\MCP\Documentation` e `/root/MCP/Documentation` (com backup `.bak-<data>` antes) | sonnet | `git diff` só com o previsto; varredura 0 |
| **S8** | **Push, tag e release** (se D15-1 gerou versão): só com autorização | owner | tag e release criadas |

## 7. Riscos

| Risco | Efeito | Mitigação |
| --- | --- | --- |
| Binário Linux nunca executado num Linux real | "funciona" afirmado sem prova | S1(b): executar e ler a saída; nada é dado como OK sem isso |
| Testes só rodaram no Windows | defeito de caminho/permissão/symlink aparece só no Linux | S1(c) e S2; gate "verde nos dois sistemas" |
| `src/files/` (guard) é a barreira de segurança das tools de arquivo | uma correção de portabilidade abre brecha | repetir os testes de ataque da Fase 4 ao tocar nele |
| Rodar como root | o MCP lê qualquer arquivo permitido pelo `--allow-root`; `/` como raiz já é recusado, mas `--allow-root /root` seria amplo | `--allow-root` só em pastas específicas; avaliar usuário sem privilégio (questão 9) |
| Token em `ps`/`cmdline` | outro usuário local lê o token | D15-6 (`OLLAMA_API_KEY` em arquivo `0600`) |
| Muitas conexões SSH seguidas ao 10.100.2.2 | fail2ban bane a estação admin | uma conexão por vez, comandos agrupados, pausa entre etapas |
| Split-DNS: de dentro do servidor o host público pode não passar pelo nginx | MCP sem acesso ou contornando limites/auth | medir na S1 (fato E); decidir a URL na questão 10 |
| Dois binários Linux (`D:\MCP\mcpollama-mcp` e o de `/root/MCP`) divergirem | confusão de versão | `VERSION` + hash em ambos; S3 gera um só e copia |
| OCR esperado no Linux | PDF digitalizado falha | D15-3 explícita; mensagem clara (§4.3a); plano próprio se o owner quiser OCR |
| Sobrescrever um binário em uso | sessão ativa quebra ou fica com versão antiga | backup `.bak-<versão>` antes; reiniciar a sessão do cliente; não trocar durante rodada longa |

## 8. Questões para o owner (fechar antes da S1; a 1 e a 2 são bloqueantes)

1. **Onde fica `/root/MCP`?** (a) no servidor de IA `lx-ai01` (10.100.2.2, alias `lx-ai01-root`), (b) em outra máquina Linux (qual? IP/alias SSH), ou (c) num Linux local (WSL/VM) desta estação. O Windows desta estação não tem `/root`. **Sem esta resposta não dá para começar.**
2. **Qual é o problema hoje?** O binário Linux existente falha ao iniciar? Faltam a pasta e a organização? O cliente MCP no Linux não o encontra? Falta OCR? Uma frase basta; muda a S1.
3. **Versão (D15-1):** só empacotar e validar, **sem nova versão** (recomendado se a S1 não achar defeito), ou já reservar a 0.10.4?
4. **Arquitetura (D15-2):** só linux/amd64 (recomendado) ou também arm64?
5. **OCR no Linux (D15-3):** fora desta fase (recomendado; é uma fase própria com `libtesseract.so`, tessdata e processos filhos isolados) ou incluir agora?
6. **Gateway em `/root/MCP`?** O `D:\MCP` guarda uma cópia do `mcpollama-gateway`. Espelhar também (duas fontes de verdade com o `/usr/local/bin`), ou manter só o MCP (recomendado)?
7. **Documentação:** manter a fonte única no repositório (`docs/`) e gerar as cópias de `D:\MCP\Documentation` e `/root/MCP/Documentation` a partir dela (recomendado), ou continuar editando à mão em cada pasta?
8. **Token (D15-6):** `OLLAMA_API_KEY` em arquivo `0600` carregado pelo cliente (recomendado) ou manter `--api-key` no `.mcp.json`, como no Windows (visível em `ps`)?
9. **Usuário:** rodar como `root` (como o pedido de `/root/MCP`) ou criar um usuário sem privilégio para o cliente MCP? Com root, o `--allow-root` deve listar pastas específicas.
10. **URL e CA:** de dentro do servidor, usar `https://ia.cslsolucoes.com.br:8443` (precisa do `--tls-ca` e do DNS interno) ou o gateway local `127.0.0.1:11500`? Esta última **dispensa o nginx**, ou seja, contorna o limite por IP e a autenticação do nginx, então só com sua decisão explícita. Onde está o PEM da CA no Linux?
11. **Quem executa:** eu coordeno e delego a medição/implementação a subagente `sonnet` com revisão `opus`, como na Fase 14 (recomendado), ou você implementa?
12. **Ramo e commits:** ramo `feat/mcp-linux`, commit por etapa verde, sem push (recomendado)?

## 9. Changelog

- 2026-11-12 21:55: **deploy feito por ordem do owner** ("você mesmo faz o deploy, sem empacotamento, tudo em `/root/MCP`"), antes da aprovação formal do plano e sem passar pelas S1 a S4; a máquina foi **inferida** como `lx-ai01` (alias `lx-ai01-root`, 10.100.2.2, CentOS Stream 10 x86_64, glibc 2.39, 88 GB livres; a questão 1 não foi respondida). **Feito:** criada `/root/MCP` (0700, não existia) com `mcpollama-mcp` (cópia do binário Linux de `D:\MCP`, `0.10.3-b1faec3`, ELF estático, `GOOS=linux`, `CGO_ENABLED=0`, sem `-trimpath`), `cache/pdfium` (0700), `Documentation/Ollama` (7 arquivos + README, **escritos para Windows**, sem os `.bak`), `VERSION` e `mcp.json.exemplo` (placeholders, sem token), tudo `root:root`. **Provas (lidas):** sha256 local = remoto (`d61f5e82…0333`); `--version` executado no Linux; `initialize` e `tools/list` por stdio com saída lida: `serverInfo` 0.10.3-b1faec3, 8 tools (`chat`, `embed`, `extract`, `generate`, `list_models`, `reportar_erro`, `summarize_file`, `translate`) e o `seed` no esquema. **Não feito / não verificado:** nenhuma chamada ao Ollama (URL, CA e token do Linux não definidos: questões 8 e 10); `go test` num Linux (fato 3.B); cliente MCP registrado no Linux (S6); PDF com `--pdf-cache-dir` (3.F); backups das versões antigas e `libtesseract`/`tessdata` **não** copiados (OCR é só Windows); documentação Linux (S4) e CHANGELOG/README (S7) **não** feitos; nada commitado além deste arquivo (não rastreado). Erro de processo: o primeiro `tar` trouxe o dono numérico do Windows; corrigido com `chown -R root:root`.

- 2026-11-12 21:50: plano criado (rascunho, não aprovado). Fatos lidos no código (`station*.go`, `ocr/ocr_other.go`, `files/guard.go`, `files/nlink_*.go`) e em `D:\MCP` (listagem). **Não** foi acessado nenhum servidor Linux, **não** foi executado o binário Linux e **não** foi rodado `go test` em Linux; esses pontos estão na §3 como não verificados. Único arquivo criado: este.
