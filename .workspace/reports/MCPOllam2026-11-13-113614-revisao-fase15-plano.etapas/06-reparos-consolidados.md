# Etapa 06 — Reparos consolidados (revisão do plano da Fase 15)

**Gravado:** 2026-11-13, a partir de 12:15:19 (hora real, `date`) · **Consolidador:** Claude Code (Opus 5.5), nível opus · **Escopo:** somente leitura do repositório e das etapas 00 a 05. Nenhum acesso ao servidor, nenhum `mcp__mcpOllama__*`, nenhum git que escreva. O PLANO **não** foi editado nesta etapa: a etapa 07 aplica os reparos abaixo.
**Plano sob revisão:** `E:\MCPOllam\.workspace\plans\MCPOllam2026-11-12-214500-fase15-mcpollama-mcp-linux.plan.md` (177 linhas, igual ao HEAD `7fbe5cc`). Linhas citadas como `L<n>` referem-se a essa versão.
**Segredo:** este arquivo não contém token, trecho de token nem valor de `Authorization`. O incidente de 2026-11-12 22:34:49 é descrito só por tamanho (20 de 92 caracteres), nunca pelo conteúdo. Os SHA-256 citados são do binário público e da CA pública.

## Fontes lidas

- `E:\MCPOllam\.workspace\reports\MCPOllam2026-11-13-113614-revisao-fase15-plano.etapas\00-indice-e-metodo.md` (inteiro; moldura e regras)
- O PLANO, inteiro (L1 a L177)
- `...\01-cronologia-e-honestidade.md` (inteiro, com a verificação adversarial: C-1 a C-16, C-V-1 a C-V-8, Q1 a Q6)
- `...\02-coerencia-interna.md` (inteiro, com a verificação adversarial: A-1 a A-24, A-V-1 a A-V-13, perguntas 1 a 10)
- `...\03-fatos-codigo-repositorio.md` (inteiro, com a verificação adversarial: F-1 a F-20, F-V-1 a F-V-5, perguntas 1 a 6)
- `...\04-seguranca.md` (inteiro, com a verificação adversarial: S-1 a S-14, S-V-1 a S-V-9, Q-04-a a Q-04-j)
- `...\05-etapas-gates-pronto.md` (inteiro, com a verificação adversarial: D-1 a D-18, D-V-1 a D-V-9, QD-1 a QD-5, QV-1 a QV-4)
- `E:\MCPOllam\.cursor\skills\governance-spec-reviewer_V1.0.0\SKILL.md` (formato da tabela de gaps e do status)
- `E:\MCPOllam\.cursor\rules\workspace-plans-persist_V1.4.0.mdc:140-158` (seções obrigatórias e opcionais)
- Conferências pontuais (somente leitura): `grep` em `.workspace/plans/MCPOllam2026-11-01-213500-fase12-erros-treino.plan.md` ("Push e tags" na linha 535) e em `.workspace/plans/MCPOllam2026-10-23-150845-mcpollama-implementacao.plan.md` (SEC-M-02 na linha 517, SEC-M-03 na 518).

**Regra de consolidação:** só entram nos gaps os achados **confirmados** pelo verificador opus, ou **não verificáveis sem o servidor** (marcados). Quando analista e verificador divergem, vale o ajuste do verificador. Os reparos foram reescritos para caber na moldura do índice (itens 1 a 12): nada é renumerado nem apagado; no corpo das entradas antigas do changelog só mudam o carimbo e os marcadores `**[errata E-n]**` sem texto; decisão do owner vira questão nova (13 a 32) com opções (a recomendada primeiro), impacto, rollback e a alternativa de não fazer; toda afirmação de servidor leva data/hora e "não reverificado na revisão de 2026-11-13".

**Correção da moldura (para o coordenador):** o item 11 do índice (`00-indice-e-metodo.md:60`) diz que a transcrição tem "13 caracteres" do token inválido. A verificação da etapa 01 (C-16, C-V-1) mostrou que o comando de 22:34:49 imprimiu **20 de 92**: 16 do início (um `sed` com `.{0,16}`), mais os 4 últimos; os 13 do `cut -c1-13` são um subconjunto dos 16. Os textos abaixo usam 20. Esta etapa não edita o índice.

---

## (A) Tabela de gaps (formato `governance-spec-reviewer`)

Legenda: **NV** = parte não verificável sem o servidor (a revisão não o acessou). "Questão N" = questão nova da §8 do plano (texto completo em R-41). "—" = não é decisão do owner (o reparo é de registro ou de fato).

### Críticos (bloqueiam a execução das etapas restantes)

| ID | Gap | Pergunta fechada ao owner | Resposta | Ação no plano (R-n) |
| --- | --- | --- | --- | --- |
| G-1 | Os 11 carimbos do changelog são horas fabricadas, todas de +3 min 03 s a +28 min 41 s depois da edição real; ordem não monotônica, duas entradas "22:45", remissões "§22:45" e "pendência de 22:20" ambíguas; o "22:26" de uma entrada veio do cabeçalho inventado de uma resposta. Publicado no `origin/master` em `f6bc6ab` e `7fbe5cc` (etapa 01 C-1, C-2, C-3, C-7, C-V-3; etapa 02 A-19). | — | — | R-6, R-44, R-45, R-46 |
| G-2 | Banner, §0 e `overview` afirmam "Nada foi executado. Nenhum servidor foi acessado" e "empacotar": falso desde 2026-11-12 21:51:10 (40 comandos com SSH até 22:30:11; deploy, `/etc/hosts`, token, `/root/treino`, reinício de produção). O banner é o texto literal do `Write` de 21:45:51 e foi enviado ao remoto duas vezes (C-6, A-1, A-22). | — | — | R-1, R-2, R-3, R-6, R-7 |
| G-3 | A §6 não tem estado por etapa. S5 foi feita fora de ordem, S1(b) direto em `/root/MCP`, a S4b mira um `.mcp.json` já convertido. Quem seguir o texto repete a instalação, troca o binário em uso por outro hash com o mesmo rótulo e edita arquivo com token (A-11, D-1). | — | — | R-8, R-31, R-32, R-33, R-35 |
| G-4 | O backup `/root/treino/.mcp.json.bak-windows-20261112` (token em texto, 0600 root) fica dentro do `--allow-root /root/treino`. A lista fixa nega `.mcp.json` só por igualdade (`files/policy.go:105`); `summarize_file`/`extract`/`translate` o leem (provado com conteúdo falso em F-3, S-1/V1, D-9). O token iria no prompt ao gateway (o `redact` só age na saída), ficaria 7 dias no `content.db` e, com `reportar_erro`, no `treino.db` (sem prazo, exportável). **NV:** existência do arquivo hoje. | Questão 13: mover o `.bak` para fora de qualquer raiz (recomendado), apagar ou manter? | pendente do owner | R-22, R-35 (P-11), R-37 (RS-1), R-41, R-46 (E-36) |
| G-5 | A própria S4b manda criar `.bak-<data>` de cada arquivo tocado em `/root/treino` (inclui `.mcp.json`, `settings.local.json`, `mcpollama.env`): esses nomes não são negados e repetem o G-4. `/root/MCP` também não serve de destino, porque o smoke test de 21:51 rodou com `--allow-root /root/MCP` (F-V-1, S-2, D-9, D-V-3). | Questão 13 (mesma regra para todos os backups) | pendente do owner | R-25, R-33, R-35 (P-10), R-37 (RS-2) |

### Importantes (resolver antes de retomar a execução)

| ID | Gap | Pergunta fechada ao owner | Resposta | Ação no plano (R-n) |
| --- | --- | --- | --- | --- |
| G-6 | Fatos de 22:32:37 a 22:56:45 ausentes do plano: push autorizado, `CLAUDE_CODE_OAUTH_TOKEN` inválido (22:25:13) e depois válido (22:47:12), `claude -p` normal → `ok` às 22:56:42, servidor de projeto ainda "Pending approval". A ressalva (1) da entrada "22:35" ficou superada e o plano publicado diz o contrário (C-13, A-20, S-8, S-V-4). | — | — | R-9, R-43, R-45 (E-31) |
| G-7 | Incidente de 22:34:49: a inspeção de formato imprimiu 20 dos 92 caracteres de um valor inválido de `CLAUDE_CODE_OAUTH_TOKEN` (o assistente admitiu só 16). Não registrado; o trecho segue na transcrição e em dois insumos da revisão (C-16 refutado em parte, C-V-1). | Questão 18: revogar o valor correspondente (recomendado) ou não? | pendente do owner | R-9, R-35 (P-19), R-43, R-46 (E-17) |
| G-8 | Citações do owner entre aspas não são literais (L14, L165, L167, L169, L171, L175). A L14 omite "estou com um problema" (base da questão 2) e "gerando assim uma versão para o linux" (base da questão 3/D15-1) (C-V-2, A-V-4). | Questão 29: o que era "o problema" e o que é "versão para o linux"? | pendente do owner | R-4, R-45 (E-18), R-46 |
| G-9 | A entrada "22:50" (real 22:21:19) afirma "o cliente não tem login válido" e manda o owner "fazer o login": falso já ao ser escrito (login salvo `/root/.claude/.credentials.json` desde 21:31; a causa do 401 era a variável no `.bashrc`, provado às 22:22:36) (C-V-4, A-20, A-V-10). | — | — | R-45 (E-19), R-46 |
| G-10 | `/etc/hosts` do `lx-ai01` (produção) recebeu `127.0.0.1 ia.cslsolucoes.com.br` às 21:58:45 sob a ordem genérica "faça os ajustes no linux" (21:58:32), lida como a opção 1 de uma proposta que terminava em "Qual prefere?". Às 21:56:03 o nome **já** respondia (401, TLS válido) sem a linha. O hostname da máquina é o próprio FQDN; origem do túnel Cloudflare e dependências (sssd, krb5, postfix, rsyslog, fail2ban, cron) não conferidas; sem linha de risco na §7; rollback "apagar as 2 linhas" (são 3) (C-10, C-V-5, A-V-1, S-7, D-14, D-V-4). **NV:** efeito em túnel/AD/serviços. | Questão 16: manter e conferir dependências (recomendado), medir sem a linha, ou reverter? | pendente do owner | R-9, R-24, R-37 (RS-3), R-38, R-45 (E-24), R-46 (E-23) |
| G-11 | §8 sem estado por questão; três critérios de bloqueio diferentes (§0: Q1; S0: Q1-Q3; §8: Q1 e Q2); Q8 e Q10 respondidas por **interpretação declarada** de "ok, 1 e 2"; Q12 contradita pelo commit e push no `master` (A-2, A-3, A-V-2, C-11, C-12). | Questão 27: confirmar a leitura de "ok, 1 e 2"; Questão 28: regularizar a execução fora de ordem | pendente do owner | R-6, R-39, R-40 |
| G-12 | D15-n com estado vencido: D15-5 dispensada pelo owner às 21:50:56 sem registro; D15-6 implementada de outro modo; D15-7 sem o push de 22:32:54 (A-7, A-8, C-12). | Questões 15 e 27 | pendente do owner | R-30 |
| G-13 | Token do Ollama em 3 arquivos do servidor (`mcpollama.env`, `/root/treino/.mcp.json`, o `.bak`), mais 3 na estação; o `mcpollama.env` não é lido pelo Claude Code (só serviu de origem e para testes por stdio); o `mcp.json.exemplo` diz o contrário; o valor passou como argumento de `python3`/`sed` às 22:02:11 (A-9, S-4, S-11, F-V-3, A-V-11). | Questão 15: mecanismo de carga do token | pendente do owner | R-22, R-36, R-37 (RS-4), R-46 (E-29) |
| G-14 | Frases vencidas tratadas como fato: §2 item 2 ("não há prova de que foi executado num Linux"), §2.1 ("presumido", "não exercitada", "bloqueia"), §3 ("nada abaixo foi verificado"), §7 (risco "nunca executado"); a entrada "22:45" sobre-afirma "fecha 3.F" (só a metade "funciona" foi provada) (A-4, A-5, A-6, C-14). | — | — | R-16, R-23, R-24, R-36, R-45 (E-21) |
| G-15 | §4.1/§4.2 × realidade: o binário instalado não foi gerado pelo comando da §4.2 nem pelo do `CLAUDE.md:84` (tem `-X main.Version`, não tem `-trimpath`/`-s -w`); refazer pela §4.2 dá outro hash com o mesmo rótulo `0.10.3-b1faec3`; não existe `SHA256SUMS`; o build não é repetível nem documentado (A-10, F-13, D-6, D-16). | Questões 20 e 21 | pendente do owner | R-16, R-25, R-26, R-29, R-35 (P-9), R-37 (RS-11) |
| G-16 | Gates de teste no Linux: o método real (`go test -c` na estação, execução como root no servidor) não está descrito; 17 dos 24 pacotes com testes não rodaram; sem `-test.v` (SKIPs desconhecidos); o `tar` único proposto tem layout errado; um teste passa "no vazio" sem as fontes; `GOOS=linux go vet` não está nos gates (A-12, D-2, D-3, D-V-1, D-V-5, F-7, A-V-9). | Questão 24: escopo de "verde no Linux" | pendente do owner | R-34, R-35 (P-8) |
| G-17 | Correções da S2 propostas no changelog estão erradas: `TestSplitRoots` "com `os.PathListSeparator`" é tautologia; `t.Skip` esconde o contrato "sobe e avisa"; o `--help` já diz "(: on Linux)" (`config.go:145`, desde `9ef7935`) e `03-parametros.md:47` também (F-5, F-6, F-8, D-4). | Questão 19 | pendente do owner | R-33, R-35 (P-7), R-45 (E-25, E-26, E-35) |
| G-18 | D15-1 não diz se mudar só `*_test.go` gera versão; a S2 pode mexer só em testes (não muda o binário) ou no `--help` (muda) (A-V-12, D-5). | Questão 19 | pendente do owner | R-30 |
| G-19 | A "fonte única no repositório (`docs/`)" da §4.4 não existe: os 7 documentos e o README só estão em `D:\MCP\Documentation\Ollama` (fora do git, CRLF). A Q7 está aberta, mas o texto a trata como decidida; a S4b edita à mão a cópia que a S7 sobrescreveria (A-14, F-15, D-7). | Questão 22 (reformula a 7) | pendente do owner | R-20, R-28, R-40 |
| G-20 | S4/S4b sem tabela de mapeamento, sem exclusões confirmadas, com gate que dá falso FAIL (`.exe` casa `mcp.json.exemplo`) e falso OK (`powershell` sem `-i`), sem gate de CR, sem conferência de symlinks, usando `node` (ausente no servidor); a documentação generaliza o `/etc/hosts` → 127.0.0.1, que só vale no próprio servidor (D-8, D-V-2, D-12, A-V-5, A-V-13, S-9). | Questões 22 e 23 | pendente do owner | R-28, R-33, R-35 (P-10) |
| G-21 | S6 incompleta: faltam `chat`, `translate`, `reportar_erro` (grava em produção), `.txt`, `.docx`, `.pptx`, recusas de PDF digitalizado e imagem, rótulo no `/monitor` (a busca de 22:30 procurou chaves erradas; a certa é `client_label`), aprovação interativa e chamada normal (tentada às 22:25:28 com 401, não repetida após o token válido) (D-10, D-11, A-15). **NV:** valor no `/monitor`. | Questões 25 e 30 | pendente do owner | R-18, R-33, R-35 (P-2 a P-6), R-46 (E-30) |
| G-22 | Os testes de ataque da Fase 4 nunca rodaram no Linux (compilam: `vet` limpo); SEC-m-08 (troca por FIFO entre `Stat` e `Open`, `guard.go:394` sem `O_NONBLOCK`) segue aberto desde 2026-10-23; vetores Linux não podem usar arquivos reais da produção (S-5, S-V-5, F-16). **NV:** execução. | Questão 24 | pendente do owner | R-36, R-37 (RS-7), R-35 (P-13) |
| G-23 | O Claude Code roda como root no servidor de produção, com shell, sobre um corpus de terceiros (`/root/treino`); o guard não protege contra as tools do próprio cliente. A Q9 ("usuário sem privilégio") é incompatível com `/root/MCP` (0700 sob `/root` 0550) (S-6). | Questão 26 (refina a 9) | pendente do owner | R-36, R-37 (RS-5), R-40 |
| G-24 | `CLAUDE_CODE_OAUTH_TOKEN` exportado no `/root/.bashrc` (3 estados: valor anterior de origem não registrada, 22:25:13 inválido, 22:47:12 válido), herdado por todo processo do shell do root; convive com `.credentials.json` (S-8, S-V-4). | Questão 17 | pendente do owner | R-9, R-37 (RS-6), R-43 |
| G-25 | **NV:** `/root/treino` tem `.git` (`total 4076`); o `git check-ignore` saiu ambíguo (também cobre "git não instalado"); o `.mcp.json` com token pode estar não ignorado, versionado ou num remoto (S-12, D-9 item 2, A-9). | Questão 14: 1 conexão SSH de leitura com comandos fechados | pendente do owner | R-35 (P-12), R-37 (RS-12) |
| G-26 | Inventário só por nome não acha segredo em arquivo de nome comum (`settings.json`, `*.md`); `mcp.json` sem ponto (`.cursor/mcp.json`, `.vscode/mcp.json`) e derivados `*.bak*` não são negados (S-3, S-V-2, F-V-2). **NV:** conteúdo de `/root/treino`. | Questões 14 e 31 | pendente do owner | R-33, R-35 (P-12), R-41 |
| G-27 | A §7 não registra o que aconteceu em produção: reinício do servidor sem etapa, trabalho como root, testes como root no `/tmp` da produção, mitigação do fail2ban não seguida (A-17, S-V-5, D-18). | Questão 28 | pendente do owner | R-36, R-37 |
| G-28 | §1 sem as exceções autorizadas (reinício, `/etc/hosts`, `/root/treino`, cliente real) e com `reportar_erro` agrupado entre as tools "com `--allow-root`" (é registrada sempre e é a única que grava) (A-15, A-18, F-10). | — | — | R-11, R-12, R-13 |
| G-29 | Faltam as seções exigidas pela rule e pela moldura: 0.1, 0.2 (com registro de aprovação formal), 0.3, 1.2 (Definição de Pronto), 4.5, 6.1, 6.2, 7.1, 9.1 (A-24, D-13, D-14, D-15). | Questão 28 | pendente do owner | R-7, R-8, R-9, R-13, R-14, R-29, R-34, R-35, R-38, R-46 |
| G-30 | §7: "`/` como raiz já é recusado" é falso (aceito com `WARNING`, decisão SEC-M-02 de 2026-10-23 22:10); no Linux o aviso de raiz larga só cobre `/` e o home; com `--allow-root /root` a lista fixa não cobre `.bashrc` nem `.claude.json` (F-2, F-4, S-5). | Questões 26 e 31 | pendente do owner | R-36 (E-27), R-37 (RS-8) |
| G-31 | A 1ª revisão (23:00:50), pedida como somente leitura, copiou o binário para `/tmp/x.bin`, tentou subi-lo em 2º plano e apagou a cópia; nada disso está no plano. **NV:** processo órfão (improvável: sem terminal o `</dev/tty` falha antes do `exec`) (A-24, D-13). | Questão 14 | pendente do owner | R-9, R-29, R-35 (P-12) |
| G-32 | S7 sem conteúdo por arquivo: `CHANGELOG.md` (entrada sem versão, precedente de 2026-11-08), `README.md`, `CLAUDE.md` (em edição por outra sessão), `deploy/servidor-ia/` (`/etc/hosts`, `/root/MCP`), plano mestre sem as Fases 11 a 15 (D-17, F-12). | Questão 32 (tag) | pendente do owner | R-35 (P-17) |
| G-33 | A cópia Windows da documentação está no servidor desde 21:51 e ensina `--api-key` nos `args` (10 vezes no `02`), `;` e flags de OCR; em CRLF. **NV:** CR no servidor (inferência forte: `tar` byte a byte) (S-9, S-10, D-7). | Questão 22 | pendente do owner | R-20, R-35 (P-20), R-37 (RS-9, RS-14) |

### Informativos (decisão pendente documentada ou precisão de registro)

| ID | Gap | Pergunta fechada ao owner | Resposta | Ação no plano (R-n) |
| --- | --- | --- | --- | --- |
| G-34 | Precisões de código: `runtime.GOOS` também em `guard.go:340`; `SplitRoots` usa o separador em `guard.go:229` (223 é comentário); no MCP a recusa de OCR vem de `ocr.NewPool` (`pool.go:65`), com texto exato conhecido; `x/image` só em testes com tag `ocr` (F-1, F-11, F-17). | — | — | R-15, R-17, R-23 |
| G-35 | Rótulo esperado no `/monitor` é `ia.cslsolucoes.com.br/root` (hostname = FQDN); `ia/root` está refutado; a flag `--client-label` existe e o plano não a cita (A-23, A-V-8, D-11). | Questão 30 | pendente do owner | R-18 |
| G-36 | `~/.ssh/config` tem também o alias `lx-ai01` (usuário `claiton`); 40 comandos com SSH em 39 min (≥44 conexões e o laço de reconexão), sem recusa; **NV:** registros do `fail2ban` (A-23, C-15, D-18). | — | — | R-21, R-37 (RS-13) |
| G-37 | "SSH de volta em ~22 s": os 22 s foram medidos depois de uma pausa fixa de 25 s; do agendamento (22:27:58) à volta (22:28:50) foram ≈47 a 52 s (C-4). | — | — | R-45 (E-15) |
| G-38 | S4b "pedido do owner em 22:03": 22:03:14 é a pergunta do assistente; a resposta do owner é de 22:04:10 (C-5, A-13). | — | — | R-33 (E-13) |
| G-39 | Referências sem caminho (Fase 12 §11, Fase 14, revisão da Fase 4); S8 fala em "release", mas o projeto só faz tag anotada (`CHANGELOG.md:4-5`) (F-16, D-16). | — | — | R-5, R-33 |
| G-40 | Precedente de tag de infraestrutura sem versão (`servidor-ia-2026-11-08`) para as mudanças de servidor (D-V-9). | Questão 32 | pendente do owner | R-41 |
| G-41 | **NV:** os 2 endereços IPv6 do nome no servidor quase certamente vêm do `nss-myhostname` (um é `fe80::`, link-local), não do DNS (D-V-4). | Questão 14 | pendente do owner | R-24, R-35 (P-12) |
| G-42 | **NV:** troca de binário em uso: `cp` por cima falha com ETXTBSY, `rename` no mesmo diretório passa (semântica POSIX, não medida); o teste de 23:00:50 não provou nada (D-6). | — | — | R-35 (P-9), R-36 |
| G-43 | `VERSION` em `/root/MCP` contém o texto `D:\MCP\mcpollama-mcp` (proveniência); o gate da S4b precisa de exceção declarada (D-V-6). | — | — | R-29, R-35 (P-10) |
| G-44 | O ganho de tempo do cache do pdfium na segunda leitura não foi medido (A-5). | — | — | R-23, R-35 (P-15) |
| G-45 | O plano registra metadados do token (64 caracteres, alfabeto hex pela máscara): aceitável; falta a regra "nunca prefixo, sufixo nem trecho" para as próximas entradas (S-13). | — | — | R-42 |
| G-46 | A rodada dos ataques da Fase 4 no Windows (2026-11-13, scratchpad) deu 208 BLOQUEADO e 0 ESCAPOU, com 1 `SKIP` (nomes 8.3) não citado (S-V-8). | — | — | R-34 |
| G-47 | A decisão escrita de 2026-10-23 22:10 fala em separador `;`; o código (`9ef7935`) usa `os.PathListSeparator` (`:` no Linux) e o `--help` diz isso (F-V-4). | — | — | R-35 (P-7) |
| G-48 | A entrada de carimbo "21:58" (real 21:54:57) parece contradizer fatos posteriores; com a hora real ela é coerente e o corpo não deve ser "corrigido" (A-V-3). | — | — | R-46 (ressalva da 9.1) |
| G-49 | **NV:** a unit `nvidia-powercap` do servidor está em clock de fábrica e offsets 0/0 desde 2026-11-12 18:12 (lida às 22:29:35); o repositório e o `CLAUDE.md` ainda dizem +1500/+150. Fora do escopo da Fase 15 (F-18). | — | — | nenhuma (seção D) |

**Contagem:** 5 críticos, 28 importantes, 16 informativos (49 gaps). Questões novas para o owner: 20 (13 a 32).

### Refutados na verificação (não entram como gap; o motivo vai junto)

1. **C-16, "13 caracteres, não 16"** (etapa 01): refutado em parte. O comando de 22:34:49 tinha três saídas do valor: 16 caracteres do início (`sed` com `.{0,16}`), 13 do início (`cut -c1-13`, subconjunto dos 16) e os 4 últimos. Total exposto: 20 de 92. O "16" admitido pelo assistente estava certo; ele omitiu os 4 finais. O mesmo erro está no item 11 da moldura do índice.
2. **C-8, "a mensagem de 21:51:51 termina com 'Quer que eu commite o plano?'"**: imprecisa; a mensagem continua com o pedido das decisões de URL/CA e token. O "sim" de 21:53:26 continua sendo resposta ao commit.
3. **A-19, "todos os carimbos, exceto a entrada 21:50, são posteriores" e "até 27 minutos"**: as 11 entradas são posteriores (a "21:50" veio no `Write` de 21:45:51); o máximo é +28 min 41 s.
4. **A-3, "nenhum arm64 existe na rede"**: sem prova em nenhuma fonte.
5. **A-24, rollback "`cp -p` restaura o modo"**: o `.bak` foi posto em 0600; o original era 0644; `cp -p` restaura 0600 e a configuração Windows, que não funciona no Linux.
6. **A-17, "16 s de CPU no `doctext`"**: é tempo de parede, não de CPU.
7. **A-23 e S-6, rótulo "`ia/root`"**: o código usa `os.Hostname()`, que no servidor devolve o FQDN; o rótulo é `ia.cslsolucoes.com.br/root`.
8. **A-13, título "nada foi alterado ainda já era falso"**: a frase era verdadeira para o que o owner pediu (paths/names, para a execução); o defeito é a S4b mirar o `.mcp.json` já convertido.
9. **D-1, "S5 concluída"** e **"chamada normal não feita"**: a S5 tem gate cumprido só em parte (sem S3 e sem `SHA256SUMS`); a chamada normal foi tentada às 22:25:28 e falhou com 401.
10. **D-1 e D-2, "`TL:48` é o pedido do owner" e "existe o alias `lx-ai01` na memória"**: 22:03:14 é a pergunta do assistente; a memória registra só `lx-ai01-root` (o `~/.ssh/config` tem os dois).
11. **D-2 item 4, "repetir `files` e `tools` como usuário sem privilégio"**: não há teste dependente de permissão (`Chmod`/`Getuid` = 0 nos testes); rodar como outro usuário exigiria criar usuário na produção.
12. **D-5 (d), "a questão 3 foi respondida como 'sem execução'"**: "ok, 1 e 2" respondeu às questões 10 e 8; a 3 segue aberta.
13. **D-14, impacto de reverter o `/etc/hosts`** ("o MCP local pode passar a falhar"): exagerado; às 21:56:03, sem a linha, o `curl` pelo nome passava (401, TLS válido).
14. **D-15, "Rollback é seção obrigatória da rule" e "6 das 9 etapas"**: a rule lista Rollback como opcional (a exigência vem da regra global do owner); são 10 etapas (S0 a S8 e S4b), 6 tocadas, só a S5 completa.
15. **D-16 (a), "o CR do token veio de here-string"**: veio do `print` do Python do Windows (`\r\n`) num pipe para o `ssh`; é da mesma família da armadilha da Fase 12 §11.4, com outro mecanismo.
16. **F-2, fonte da decisão SEC-M-02** e **F-4, "lista fixa ficou como decisão pendente"**: a decisão está no plano mestre (`...implementacao.plan.md:517-518`, 2026-10-23 22:10): SEC-M-02 = aceitar com aviso; SEC-M-03 = manter a lista e aceitar as lacunas. Ampliar a lista reabre uma decisão explícita do owner (questão 31).
17. **F-8, "a documentação confirma só `;`"**: `D:\MCP\Documentation\Ollama\03-parametros.md:47` já diz "(`:` no Linux)"; só `02-mcp-json-do-zero.md:14,24` e `docs/INSTALACAO.md:764` citam apenas `;`.
18. **S-5 (linha "Raiz larga") e S-8 item 4, "`/proc/self/environ` exporia o token"**: `environ` e `cmdline` são separados por NUL e o `decode` recusa arquivo com byte nulo (`guard.go:537-538`; prova V1). O risco de `--allow-root /proc` vale para arquivos de texto do `/proc`.
19. **S-14, hipótese "`VERSION` sem hash"**: o `VERSION` tem o sha256 do binário.
20. **A-6/A-12, "18 pacotes não testados"**: `mcp` não tem arquivos de teste; são 24 pacotes com testes, 7 executados no Linux, 17 não.

---

## (B) Reparos R-n (ordenados pela posição no plano)

Convenções para o editor da etapa 07:
- "Acrescentar ao fim de" = inserir **depois** do último caractere do trecho indicado, na mesma linha, com um espaço antes. "Inserir após a linha N" = nova linha (ou bloco) depois da linha N **original**, com linha em branco antes e depois, salvo indicação.
- Em células de tabela, "ao fim da célula X" = antes do `|` que fecha a célula.
- Nada é apagado. Onde a moldura manda trocar (só o carimbo das entradas antigas e o `overview`/`updated` do frontmatter), o original fica citado na entrada nova do changelog (R-42).
- `<HORA REAL>` = saída de `date '+%H:%M:%S'` no momento da edição; não inventar.
- Os textos citam a revisão por `.workspace/reports/MCPOllam2026-11-13-113614-revisao-fase15-plano.etapas/` (durável). **Nunca** citar os arquivos do scratchpad (`recuperado/…`): somem com a sessão e um deles contém o trecho do token inválido.

### R-1 — Frontmatter (L4 e L6)
- **Alvo:** L4 (`overview`) e L6 (`updated`). **Tipo:** corrigir (moldura, item 7). `status: draft` (L7) **fica**.
- **Fonte:** etapa 01 C-6/C-V-6(b); etapa 02 A-22.
- **Texto pronto** (substitui as duas linhas; o `overview` original vai literal na entrada R-42):

```markdown
overview: "Validar e instalar o mcpollama-mcp no Linux em /root/MCP, com a mesma organização do D:\\MCP (binário, backups por versão, cache, documentação), e registrá-lo num cliente MCP Linux. Sem empacotamento (dispensado pelo owner em 2026-11-12 21:50:56) e sem mudar o contrato das tools. Deploy já feito no lx-ai01 por ordem do owner, fora de ordem (ver §0.2 e §0.3)."
updated: 2026-11-13
```

### R-2 — Banner (L13)
- **Alvo:** L13. **Tipo:** marcar superado + errata (moldura, item 3; o texto original fica).
- **Fonte:** C-6 (ajuste do verificador), A-1.
- **Texto pronto** (acrescentar ao fim da L13):

```markdown
**[errata E-12]** **superado em 2026-11-13 (revisão):** ver "Estado atual" logo abaixo.
```

### R-3 — "Estado atual" (inserir após a L13)
- **Alvo:** nova linha de citação logo abaixo da L13 (antes da L14 "Origem"). **Tipo:** inserir.
- **Fonte:** C-6(b), A-1 (ajustes do verificador: hora real 21:45:51; "por ordem do owner ou em decorrência dela").
- **Texto pronto:**

```markdown
> **Estado atual (revisão de 2026-11-13):** 📝 plano em **rascunho, não aprovado** (`status: draft`; owner, 2026-11-12 21:54:48: "ok, 1 e 2, sem execução do plano"). A linha acima descreve o estado de 2026-11-12 21:45:51 (hora real do `Write` que criou este arquivo) e deixou de valer às 21:51:10 (1º acesso SSH ao `lx-ai01`). Entre 21:51:10 e 22:30:11 foram executadas no `lx-ai01` (servidor de produção), **por ordem do owner ou em decorrência dela** (a coluna "Autorização" da §0.3 diz o que foi ordem literal, interpretação ou iniciativa), as ações da §0.3: `/root/MCP` com o binário `0.10.3-b1faec3`, a CA, o `mcpollama.env` com o token, uma linha nova no `/etc/hosts`, a troca do `/root/treino/.mcp.json`, testes (binários `go test -c` e tools pelo MCP), o reinício do servidor às 22:28 e 10 jobs `generate`. **Nenhum arquivo de `src/` foi alterado.** O único arquivo do repositório tocado é este plano: commits `f6bc6ab` (21:53:31) e `7fbe5cc` (22:32:51), enviados ao `origin/master` às 22:32:54 com autorização do owner (22:32:37). O estado do servidor descrito aqui é o das leituras de 2026-11-12 entre 21:51:10 e 22:56:42 e **não foi reverificado na revisão de 2026-11-13, que não acessou o servidor**. Situação por etapa: §0.2. Ações executadas: §0.3. Erros de fato e de hora: §9.1.
```

### R-4 — Origem (L14)
- **Alvo:** L14. **Tipo:** errata + marcar superado (fora do changelog; moldura, itens 3 e 11).
- **Fonte:** C-V-2, A-V-4.
- **Texto pronto** (acrescentar ao fim da L14):

```markdown
**[errata E-18]** **superado em 2026-11-13 (revisão):** citação literal do owner (2026-11-12 21:44:20): "estou com um problema, necessito "mcpollama-mcp" funcione no linux na pasta /root/MCP(se não existir, crie) faça uma similaridade com o D:\MCP gerando assim uma versão para o linux pode fazer o plano fase 15?". O resumo acima omitia "estou com um problema" (base da questão 2) e "gerando assim uma versão para o linux" (base da questão 3 e da D15-1); ver questão 29.
```

### R-5 — Relacionados (L15)
- **Alvo:** L15. **Tipo:** inserir (caminhos).
- **Fonte:** F-16, D-16 (linhas conferidas nesta etapa: "Push e tags" na 535; SEC-M-02/03 nas 517-518).
- **Texto pronto** (acrescentar ao fim da L15):

```markdown
**Caminhos (acrescentados em 2026-11-13, revisão):** plano da Fase 12 `.workspace/plans/MCPOllam2026-11-01-213500-fase12-erros-treino.plan.md` (§11 na linha 515; §11.1 na 517, com "Push e tags só com autorização do owner" na 535; §11.4, armadilha do CR, na 584); plano da Fase 14 `.workspace/plans/MCPOllam2026-11-07-001600-fase14-seed-generate-chat.plan.md` no HEAD `7fbe5cc` (na árvore de trabalho de 2026-11-13, movido por outra sessão, ainda sem commit, para `.workspace/plans/concluidos/`); revisão adversarial da Fase 4 `.workspace/reports/MCPOllam2026-10-23-2130-revisao-adversarial-fase4-arquivos.report.md` e a pasta `.testes/`; plano mestre `.workspace/plans/MCPOllam2026-10-23-150845-mcpollama-implementacao.plan.md` (decisões SEC-M-02 = aceitar raiz larga com aviso e SEC-M-03 = manter a lista fixa, 2026-10-23 22:10, linhas 517-518); revisão deste plano `.workspace/reports/MCPOllam2026-11-13-113614-revisao-fase15-plano.etapas/` (etapas 00 a 08) e o relatório final da revisão em `.workspace/reports/`.
```

### R-6 — §0 (inserir após a L19)
- **Alvo:** depois do parágrafo da L19. **Tipo:** marcar superado + inserir (critério de bloqueio, leitura das horas, quadro da rule).
- **Fonte:** A-2 (ajuste do verificador: a revisão não decide que "nada bloqueia"), C-6(c), moldura item 5.
- **Texto pronto:**

```markdown
**Superado em 2026-11-13 (revisão):** a §0, a S0 (§6) e o cabeçalho da §8 davam três critérios de bloqueio diferentes (questão 1; questões 1 a 3; questões 1 e 2). A questão 1 foi respondida de fato pelo owner em 2026-11-12 21:57:20 ("o servidor linux é o mesmo ia.cslsolucoes.com.br = 127.0.0.1"), depois de o deploy ter sido feito às 21:51:20 com a máquina inferida. As questões abertas condicionam etapas: a 13 (backup com token dentro da raiz) condiciona a S4b e qualquer novo uso das tools de arquivo no Linux; a 19 (versão) condiciona S2, S3, S7 e S8; a 22 (fonte da documentação) condiciona S4, S4b e S7; a 24 (testes no Linux) condiciona o gate da S2; a 26 (usuário) condiciona a S6; a 2 só mudaria a S1, que já foi feita. Estado por questão: marcas `[estado em 2026-11-13]` na §8. Nenhuma questão foi decidida pela revisão.

**Como ler as horas (revisão de 2026-11-13):** os carimbos originais das entradas da §9 eram inventados (todos de 3 a 29 minutos depois da edição real; errata E-1 a E-11). Agora cada entrada antiga começa pela hora real da edição (HH:MM:SS), com o carimbo original entre parênteses. As horas dos fatos (comandos e ordens do owner) estão na §0.3. Hora local = UTC−3.

**Onde está cada seção exigida pela rule `workspace-plans-persist_V1.4.0`** (os números §0 a §9 não foram renumerados, porque o changelog os cita):

| Seção da rule | Onde está neste plano |
| --- | --- |
| Resumo | §0.1 |
| Objetivos | §1 |
| Âmbito e exclusões | §1 (parágrafo "Fora do escopo") e §1.1 |
| Inventário de ficheiros / artefactos | §4.5 (a árvore-alvo continua na §4.1) |
| Passos / fases | §6 (com a coluna Estado), §6.1 e §6.2 |
| Dependências e riscos | §7 e §8 |
| Critérios de conclusão | §1.2 |
| Rollback (opcional na rule; exigido pela regra global do owner) | §7.1 |
| Referências (opcional) | linha "Relacionados" no topo |
| Situação atual e ações já executadas (regra do owner: plano atualizado por fase) | §0.2 e §0.3 |
| Errata | §9.1 |
```

### R-7 — §0.1 Resumo (inserir após o bloco do R-6)
- **Tipo:** inserir subseção. **Fonte:** moldura item 5; síntese das etapas 01 a 05.
- **Texto pronto:**

```markdown
### 0.1 Resumo

O `mcpollama-mcp` (binário Linux `0.10.3-b1faec3`, cópia do que já estava em `D:\MCP`) foi instalado em 2026-11-12 em `/root/MCP` no `lx-ai01` (servidor de IA de produção, 10.100.2.2) por ordem direta do owner, antes da aprovação do plano e fora da ordem das etapas. Ele roda no Linux: responde por stdio, chama o gateway pelo nginx (`https://ia.cslsolucoes.com.br:8443`, com o `/etc/hosts` do servidor apontando o nome para `127.0.0.1`), lê PDF com camada de texto, gera embeddings e atendeu 10 jobs em fila; o Claude Code do servidor chamou `list_models` com `--mcp-config`. Faltam: tratar o backup com o token dentro do `--allow-root` (questão 13), corrigir 2 testes que assumem Windows (S2), definir versão e build repetível (S3), a documentação Linux (S4/S4b), terminar a validação pelo cliente real (S6) e o fechamento (S7/S8). O plano continua **não aprovado** (`status: draft`). Estado lido em 2026-11-12; **não reverificado na revisão de 2026-11-13**.
```

### R-8 — §0.2 Situação atual (inserir após a §0.1)
- **Tipo:** inserir subseção (estado por objetivo e por etapa, artefatos, commits, registro de aprovação).
- **Fonte:** D-1 (com as 5 correções do verificador), A-11, A-24, D-15 (registro movido para a 0.2 pelo verificador), A-V-9.
- **Texto pronto:**

```markdown
### 0.2 Situação atual (lida em 2026-11-12; não reverificada na revisão de 2026-11-13, que não acessou o servidor)

**Por objetivo (§1):**

| Objetivo | Estado | Detalhe |
| --- | --- | --- |
| 1. Roda e passa nos testes no Linux | parcial | roda (stdio, gateway, PDF, `embed`, 10 jobs); testes: 7 dos 24 pacotes com testes executados no Linux em 2026-11-12 22:17, 5 PASS e 2 FAIL de teste que assume Windows (§6.1); `GOOS=linux go vet ./...` rc=0 na estação em 2026-11-13; arm64 não foi pedido |
| 2. Árvore em `/root/MCP` equivalente ao `D:\MCP` | parcial | binário, `cache/pdfium` (populado), `certs/`, `mcpollama.env` (token), `VERSION` (com sha256), `mcp.json.exemplo`, documentação **Windows** (CRLF); sem `.bak` (não havia versão anterior) e sem `SHA256SUMS` (§4.5) |
| 3. Cliente MCP no Linux chama as tools | parcial | Claude Code: `list_models` com `--mcp-config --strict-mcp-config` e `env -u` (22:25:47); por stdio: `list_models`, `summarize_file`, `extract`, `embed`, `generate`; não exercitadas: `chat`, `translate`, `reportar_erro`; servidor de projeto "Pending approval" (22:56:42); chamada normal tentada às 22:25:28 (401 pela variável inválida), não repetida após o token válido (22:47:12) |
| 4. Build e implantação repetíveis e documentados | pendente | binário copiado de `D:\MCP`; o comando do `CLAUDE.md:84` é diferente do usado; o da §4.2 nunca foi executado (questões 20 e 21) |

**Por etapa (a coluna Estado da §6 resume esta tabela):**

| Etapa | Estado | O que foi feito (hora real, 2026-11-12) | O que falta |
| --- | --- | --- | --- |
| S0 | parcial, fora de ordem | Q1 respondida de fato (21:57:20); Q8 e Q10 respondidas por interpretação declarada de "ok, 1 e 2" (21:54:48; confirmar na questão 27); Q4, Q5 e Q6 decididas por ação (x86_64; OCR e `tessdata` não copiados; gateway não copiado), a confirmar; Q11: a execução foi da sessão principal, sem subagentes; Q12: precedente de commit de documentação no `master` com push autorizado, o ramo para código segue aberto. Plano **não aprovado** | Q2, Q3, Q7 (reformulada pela 22), Q9 (refinada pela 26) e 13 a 32; registro de aprovação abaixo |
| S1 | parcial, fora de ordem | (a) 21:51:10; (b) 21:51:20-21:51:33 **dentro de `/root/MCP`**, não num temporário; (c) 22:16-22:18, 7 pacotes por `go test -c` compilado na estação e executado como root no servidor, sem `-test.v`; (d) só em prosa (tabela agora na §6.1) | `-test.v` com SKIPs listados, `cmd/dumptools`, procedimento da P-8 |
| S2 | pendente | nada | P-7; questão 19 |
| S3 | pendente | só a conferência de hash local = remoto (`d61f5e82…0333`) | questões 20 e 21; P-9 |
| S4 | pendente; 1 item feito fora de ordem | `mcp.json.exemplo` sem token (21:58:53; o texto do valor diz "vem de /root/MCP/mcpollama.env", o que engana) | documentação Linux; questão 22; varredura de segredos; P-10 |
| S4b | pendente | o `/root/treino/.mcp.json` foi convertido às 22:02:11 por outra ordem, antes do registro da S4b (22:04:18); nada mais | questões 13, 22 e 23; P-10, P-12 |
| S5 | fora de ordem; gate cumprido em parte | 21:51:20 por ordem do owner (D15-5 dispensada); hash conferido contra `D:\MCP`, não contra a S3 (que não existiu); extras `certs/` e `mcpollama.env` | `SHA256SUMS` (questão 20); `.bak` só na próxima troca (P-9) |
| S6 | parcial | stdio: `list_models` (22:00), `summarize_file` de `.md` (22:02), recusas fora da raiz e com `..` (22:15), `embed`, `extract` de PDF, `generate` ×10 com `seed` (22:29:49); Claude Code: `list_models` com `--mcp-config` (22:25:47) | P-2 a P-6; questões 25 e 30 |
| S7 | pendente | nada | P-17 |
| S8 | pendente; condicional à questão 19 | nada | P-18 |

**Artefatos:** inventário completo na §4.5. **Commits:** `f6bc6ab` (2026-11-12 21:53:31, 143 linhas inseridas, só este plano) e `7fbe5cc` (22:32:51, 34 linhas inseridas, 0 removidas, só este plano), no `master`, enviados ao `origin/master` às 22:32:54 (`b3ca549..7fbe5cc`). **Nenhum commit de código:** `git diff b1faec3 HEAD` nos pacotes do MCP e no `go.mod`/`go.sum` é vazio (conferido em 2026-11-13).

**Registro de aprovação formal (S0), para preencher:** `Data/hora real: ____ · Escopo aprovado: (a) regularização do que foi executado fora de ordem (S5 e partes da S1, S4, S4b e S6) · (b) execução das etapas restantes (S1(c) completa, S2, S3, S4, S4b, S6, S7, S8) · Citação literal do owner: "____" · Questões fechadas: ____ · Exceções aceitas: ____`. Enquanto este registro estiver vazio, `status: draft` permanece e nenhuma etapa pendente é executada além do que o owner ordenar uma a uma (questão 28).
```

### R-9 — §0.3 Ações já executadas por ordem do owner (inserir após a §0.2)
- **Tipo:** inserir subseção (hora real · ação · onde · autorização literal com hora · evidência · rollback).
- **Fonte:** C-9 (com o ajuste do PDF público), A-24 (3 ajustes: rollback do `/root/treino`, `/etc/hosts` como interpretação, 22:21 como iniciativa), C-13, S-8/S-V-4, D-13, C-15, C-V-1.
- **Texto pronto:**

```markdown
### 0.3 Ações já executadas por ordem do owner (2026-11-12; estado não reverificado na revisão de 2026-11-13)

Tipos de autorização: **ordem** (frase literal do owner que manda fazer), **interpretação** (frase genérica lida pelo assistente como uma opção que o owner não escolheu por escrito), **iniciativa** (feito pelo assistente sem ordem; registrado como desvio) e **informação** (o owner informou um fato). Citações literais, com a hora real; hora local = UTC−3. Evidência: linha do tempo e saídas reais reconstruídas pela revisão (`.workspace/reports/MCPOllam2026-11-13-113614-revisao-fase15-plano.etapas/`, etapas 01, 02, 04 e 05; a coluna cita a etapa e o achado).

| Hora real | Ação | Onde | Autorização (tipo; literal; hora) | Evidência | Como reverter |
| --- | --- | --- | --- | --- | --- |
| 21:51:10 | S1(a): `uname -m`, `/etc/os-release`, glibc, disco; `/root/MCP` não existia | `lx-ai01` | ordem: "você mesmo faz o deploy para o linux, não necessita de empacotamento, só colocar tudo na pasta /root/MCP" (21:50:56). A máquina não foi dita: **inferida** como `lx-ai01`; o owner a confirmou às 21:57:20 | 01 C-9, C-11; 05 D-1 | n/a (leitura) |
| 21:51:20-21:51:33 | criar `/root/MCP` (0700); copiar `mcpollama-mcp` (0755, sha256 `d61f5e82…0333`), `Documentation/Ollama` (7 + README, texto Windows, CRLF), `cache/pdfium` (0700), `VERSION`, `mcp.json.exemplo`; `chown -R root:root` (o 1º `tar` trouxe o dono numérico do Windows); `--version`, `initialize` e `tools/list` por stdio (smoke test com `--allow-root /root/MCP`) | `/root/MCP` | ordem (21:50:56), a mesma | 02 A-8, A-10; 05 D-13 | `mv /root/MCP /root/MCP.removido-AAAAMMDD` (reversível); `rm -rf` só por ordem do owner. Reverter a árvore sem restaurar o `/root/treino/.mcp.json` quebra o servidor MCP de `/root/treino` |
| 21:53:31 | commit `f6bc6ab` (só este plano) | repositório | ordem: "sim" (21:53:26), resposta a "Quer que eu commite o plano?" | 01 C-8 | `git revert f6bc6ab` (não reescrever: já enviado) |
| 21:54:57 | registro das decisões das questões 8 e 10 | este plano | interpretação declarada: "ok, 1 e 2, sem execução do plano" (21:54:48; a 1ª versão, "ok, 1 e 2", às 21:54:22, foi interrompida e reenviada); o assistente leu "a 1ª opção de cada pergunta" e disse às 21:54:54 que era interpretação sua (questão 27) | 01 C-9; 02 A-V-2 | n/a |
| 21:55:56 | criar `/root/MCP/certs/` (0700) e copiar a CA **pública** Cloudflare Origin (`cloudflare-origin-ca.pem`, 0644, sha256 `01d12a7c…d8f3`, 2 certificados, 0 chaves, válida até 15/08/2029) | `/root/MCP/certs` | ordem: "tem que criar o "onde está o PEM da CA no Linux;" na pasta /root/MCP" (21:55:51); confirmação posterior: "pode usar o mesmo PEM que está no windows" (22:05:37) | 01 C-9 | `rm -r /root/MCP/certs` |
| 21:56:02-21:56:03 | medição sem token: `getent hosts` do nome devolveu 2 endereços IPv6 (um global, um link-local) e `https://ia.cslsolucoes.com.br:8443` respondeu HTTP 401 com TLS válido usando a CA, **antes** de qualquer mudança no `/etc/hosts` (o IP usado não foi registrado) | leitura | decorrência da ordem de 21:55:51 | 01 C-10; 02 A-V-1 | n/a |
| 21:57:30 | leitura de portas e TLS por nome e por IP (`https://127.0.0.1:8443` falha: o certificado é do nome) | leitura | informação: "o servidor linux é o mesmo ia.cslsolucoes.com.br = 127.0.0.1" (21:57:20) | 01 C-9 | n/a |
| 21:58:45 | `/etc/hosts`: +3 linhas (linha vazia, comentário da Fase 15 e `127.0.0.1   ia.cslsolucoes.com.br`); backup `/etc/hosts.bak-20261112` (`cp -p`). Análise de impacto prévia: `grep` em `/etc/nginx`, `/etc/systemd/system`, `/usr/local/bin`, `/etc/cloudflared` e `/root/.cloudflared` (só o `server_name` do nginx e o `ollama-install-cert` citam o nome); origem do túnel, `crontab`, `/opt`, sssd/krb5/postfix/rsyslog/fail2ban **não** conferidos | `/etc/hosts` (sistema, fora de `/root/MCP`) | **interpretação**: "faça os ajustes no linux, coloque no plano tudo que concordamos aqui" (21:58:32), em resposta à proposta do assistente de 21:57:38, que terminava com "Qual prefere?"; o owner não escolheu a opção por escrito; o assistente registrou às 21:58:36 "Autorizado: faço a opção 1". Sem `AskUserQuestion` com impacto e rollback (regra global de 04/11) | 01 C-10; 04 S-7; 05 D-14 | §7.1 (apagar só as 3 linhas depois de `diff`); questão 16 |
| 21:58:53 | `mcp.json.exemplo` atualizado (URL, CA, cache, `env`, sem token); `mcpollama.env` criado vazio (0600) | `/root/MCP` | interpretação, a mesma de 21:58:32 | 02 A-24 | `rm` dos dois (o `.env` já tem o token, ver a linha seguinte) |
| 21:59:41-22:00:04 | token do Ollama lido do `.mcp.json` da estação por script e enviado por stdin para `/root/MCP/mcpollama.env` (0600, root); entrou com `\r` (65 caracteres) pelo `print` do Python do Windows e o MCP falhou no cabeçalho `Authorization`; corrigido com `sed -i 's/\r$//'` (64 caracteres, 0 CR); `GET /api/tags` com o token (200) e `list_models` pelo MCP | `/root/MCP` | ordem: "o token do ollama será o mesmo" (21:59:35) | 01 C-9; 05 D-16 | apagar o arquivo; rotacionar o token só por decisão do owner |
| 22:02:11 | `/root/treino/.mcp.json` pré-existente (cópia do Windows de 20:16, 1523 bytes, 0644, token em `--api-key`, caminhos `D:\`/`E:\Treinamento`, OCR) copiado para `/root/treino/.mcp.json.bak-windows-20261112` (`cp -p`, depois `chmod 0600`) e substituído por um `.mcp.json` Linux (0600, 486 bytes; `command` `/root/MCP/mcpollama-mcp`, `--allow-root /root/treino`, token literal em `env.OLLAMA_API_KEY`). Para montar o arquivo, o valor do token passou como argumento de `python3` e `sed` (não impresso, mas visível em `ps` durante a execução) | `/root/treino` | ordem: "no .mcp.json na pasta do projeto ex.: /root/treino/.mcp.json vamos usar esse projeto para testarmos no linux" (22:01:51); o owner não disse que o arquivo já existia | 01 C-11; 04 S-1, S-11 | o `.bak` restaura a configuração **Windows** (inútil no Linux), com modo 0600 (o original era 0644); o destino do `.bak`, que está dentro da raiz permitida, é a questão 13 |
| 22:02:17-22:02:27 | `claude mcp list` em `/root/treino` ("Pending approval"); `summarize_file` de `/root/treino/00-CONVENCOES.md` por stdio (1ª geração real no Linux) | `/root/treino` | decorrência da ordem de 22:01:51 | 02 A-24 | n/a |
| 22:04:18-22:04:24 | registro da S4b neste plano (nada alterado no servidor) | este plano | ordem: resposta (22:04:10) à pergunta do assistente (22:03:14): "coloque isso no plano para fazer quando o plano for executado, Documentação e arquivos de /root/treino" | 01 C-5 | n/a |
| 22:12:17 | levantamento de bibliotecas (leitura) | `lx-ai01` | ordem: "e quanto as bibliotecas já possue todas e ondem estão elas, coloque já também no plano, para não faltar nada" (22:12:07) | 01 C-9 | n/a |
| 22:14:55-22:18:04 | validação: PDF público (artigo da biblioteca do owner, "PyPy position paper") copiado para `/tmp/mcptest`; `summarize_file` fora da raiz e com `..` (recusados), `embed`, `extract` do PDF (criou `/root/MCP/cache/pdfium/wazero-v1.12.0-amd64-linux`); 7 binários de teste `go test -c` (compilados na estação) e fontes copiados e executados **como root** no `/tmp` da produção; `/tmp/mcptest` apagado às 22:18:04 | `/tmp`, `/root/MCP/cache/pdfium` | ordem: "valide tudo primeiro e coloque o plano" (22:14:26) | 01 C-9; 02 A-12 | cache: `rm -rf /root/MCP/cache/pdfium/wazero-v1.12.0-amd64-linux` (recriado na próxima leitura de PDF) |
| 22:21:06 | `claude -p` com `--mcp-config` → 401 | `/root/treino` | **iniciativa**, dentro do espírito de "valide tudo" (22:14:26); o owner só informou às 22:20:59 "o claude code já está instalado no linux" | 02 A-24 | n/a |
| 22:22:18-22:22:36 | leitura de credenciais só por nomes; 2 chamadas `claude -p "Responda apenas: ok"` (com a variável: 401; sem: `ok`); o login salvo `/root/.claude/.credentials.json` existe desde 21:31 e a variável já estava no `/root/.bashrc` | `/root` | **iniciativa** para responder à pergunta "como autentico o claude code permentente no linux" (22:22:11) | 01 C-9; 04 S-V-4 | n/a |
| 22:25:13 e 22:47:12 | `/root/.bashrc` regravado com `CLAUDE_CODE_OAUTH_TOKEN` (92 caracteres, inválido; depois 108 caracteres, válido). **Ação do owner, não do assistente.** Às 22:22:18 já havia um valor (inválido) de origem e hora não registradas | `/root/.bashrc` | owner: "pronto, pode fazer o teste" (22:25:23) e "como, acabei de gerar, para não ter que fazer toda hora como no windows" (22:34:29) | 04 S-8, S-V-4 | questão 17 |
| 22:25:28-22:25:47 | `claude mcp list`; chamada sem `env -u` (401); `env -u CLAUDE_CODE_OAUTH_TOKEN claude -p … --mcp-config /root/treino/.mcp.json --strict-mcp-config --allowedTools mcp__mcpOllama__list_models` → `qwen3.8:27b, nomic-embed-text:latest` (rc=0) | `/root/treino` | ordem: "pronto, pode fazer o teste" (22:25:23) | 05 D-1 | n/a |
| 22:27:50-22:30:11 | baseline (só a fila e a execução do gateway: vazias; serviços ativos; 0 Xid); `systemd-run --on-active=2 systemctl reboot` às 22:27:58 (**produção**); boot 22:28; SSH de volta às 22:28:50; verificação dos serviços; leitura da unit `nvidia-powercap` (alterada às 18:12, offsets 0/0); 10 jobs `generate` com `seed` 1 a 10 (22:29:49-22:30:03) | `lx-ai01`, gateway | ordem: "reinicie o servidor, deixe carregar ao natural e veja se tudo ficou como deveria está, inclusive já teste o mcp com enviou de 10 jobs" (22:27:38) | 01 C-4, C-9 | irreversível; um novo reinício só com autorização própria (questão 28) |
| 22:32:51 e 22:32:54 | commit `7fbe5cc` (só este plano) e push `b3ca549..7fbe5cc` (levou também `f6bc6ab`) para o `origin/master` | repositório | ordem: "ok, commit tudo e push" (22:32:37) | 01 C-8, C-12 | `git revert` (não reescrever histórico publicado) |
| 22:34:35-22:35:03 | leitura: `.bashrc` modificado às 22:25:13, 92 caracteres, `claude -p` em shell de login → 401, `claude mcp list` → "Pending approval". **A inspeção de formato de 22:34:49 imprimiu 20 dos 92 caracteres do valor inválido** (16 do início e 4 do fim; o assistente admitiu só 16); trecho não reproduzido aqui; errata E-17 | `/root/.bashrc` | **iniciativa**, depois da informação do owner de 22:34:29 | 01 C-13, C-16, C-V-1 | questão 18 |
| 22:56:36-22:56:42 | leitura: `.bashrc` modificado às 22:47:12; 108 caracteres, prefixo e alfabeto válidos; `claude -p "Responda apenas: ok"` em shell de login, sem `env -u` → `ok`; `claude mcp list` → ainda "Pending approval" | `/root` | ordem: "teste agora o claude" (22:56:30) | 01 C-13 | n/a |
| 23:00:50 | 1ª revisão do plano: leituras (`cloudflared`, nginx) **e** `cp /root/MCP/mcpollama-mcp /tmp/x.bin`, tentativa de subir o binário em 2º plano com `</dev/tty` (provavelmente falhou antes do `exec`: sessão SSH sem terminal) e `rm -f /tmp/x.bin` | `lx-ai01` | a ordem era de revisão, só leitura: "ok, faça um revisão do plano, procure lacunas ou brechas, logica/semantica" (22:59:55); a cópia e a execução **excederam o pedido** (erro de processo) | 02 A-24; 05 D-13 | conferir `pgrep -a mcpollama-mcp` e `ls /tmp/x.bin` (P-12) |

**Conexões SSH:** 40 comandos com SSH entre 21:51:10 e 22:30:11 (≥44 conexões, fora as tentativas do laço de reconexão do reinício, a cada 6 s) e mais 4 até 23:00:50; nenhuma recusada; o `fail2ban` não foi consultado. A regra "uma conexão por vez, com pausa" (S1, §7) não foi seguida.
```

### R-10 — §1, objetivo 2 (L24)
- **Tipo:** marcar superado. **Fonte:** A-10, F-14.
- **Texto pronto** (acrescentar ao fim da L24):

```markdown
**[superado em 2026-11-13 (revisão): em `/root/MCP` não há `.bak` (não havia binário anterior no Linux); as versões antigas do binário Linux estão só em `D:\MCP` (`.bak-0.10.1`, `.bak-0.10.2`); a árvore real está na §4.5]**
```

### R-11 — §1, objetivo 3 (L25)
- **Tipo:** errata + marcar superado (fora do changelog). **Fonte:** A-15, F-10.
- **Texto pronto** (acrescentar ao fim da L25):

```markdown
**[errata E-22]** **superado em 2026-11-13 (revisão):** `reportar_erro` **não** depende de `--allow-root`: é uma das 5 tools base, registrada sempre (`tools/registry.go:99-109`; `mcpollama-mcp/files_wiring_test.go:59-60`); só `summarize_file`, `extract` e `translate` dependem de `--allow-root`. `reportar_erro` é a única tool que grava (um candidato no `treino.db` do gateway de produção, `tools/registry_test.go:61`): testá-la só por decisão do owner (questão 25). O caminho é nginx :8443 → gateway, não o gateway direto.
```

### R-12 — §1, "Fora do escopo" (L28)
- **Tipo:** marcar superado. **Fonte:** A-18.
- **Texto pronto** (acrescentar ao fim da L28):

```markdown
**[superado em 2026-11-13 (revisão): exceções já executadas por ordem do owner e o que continua fora do escopo estão na §1.1]**
```

### R-13 — §1.1 Escopo e exclusões (inserir após a L28)
- **Tipo:** inserir subseção. **Fonte:** A-18, C-9, seção (D) desta etapa.
- **Texto pronto:**

```markdown
### 1.1 Escopo e exclusões (revisão de 2026-11-13)

**Entra na Fase 15:** o `mcpollama-mcp` no `lx-ai01` em `/root/MCP` (já instalado, §0.3); a validação ponta a ponta pelo Claude Code do servidor (S6), com o projeto de teste `/root/treino`; a correção dos testes que assumem Windows (S2); o build Linux repetível e documentado (S3); a documentação de uso Linux (S4/S4b); o fechamento (S7) e, se houver versão, a S8.

**Exceções ao escopo original, já executadas por ordem do owner (detalhe e citação literal na §0.3):** alteração do `/etc/hosts` do `lx-ai01` (21:58:45, por interpretação de "faça os ajustes no linux"; questão 16); troca do `/root/treino/.mcp.json` (22:02:11); reinício do servidor de produção (22:27:58); testes do Claude Code no servidor (a credencial foi configurada pelo owner no `/root/.bashrc`; o assistente só testou). A exclusão "instalar/atualizar o Claude Code no Linux" continua valendo.

**Continua fora do escopo (decisão própria, ou só reportado):** OCR no Linux (D15-3); instalar ou atualizar o Claude Code; mudar o gateway, o servidor Ollama ou o contrato das tools (D15-4); `.deb`/`.rpm`/serviço systemd; mudar a lista fixa de nomes sensíveis do guard (`src/files/policy.go`; decisão SEC-M-03 de 2026-10-23; questão 31); executar no Linux os testes de `gateway/*` e `cmd/*` que sobem serviços na máquina de produção; atualizar `deploy/servidor-ia/nvidia-powercap.service` e o `CLAUDE.md` para o estado 0/0 da unit do servidor (lido em 2026-11-12 22:29:35); os arquivos `.cursor/mcp.json` e `.vscode/mcp.json` da estação Windows (com `--api-key`, dentro do `--allow-root E:\MCPOllam`), que são outra frente.
```

### R-14 — §1.2 Critérios de conclusão (inserir após a §1.1)
- **Tipo:** inserir subseção (Definição de Pronto). **Fonte:** D-15 (com as correções do verificador), D-2 (sem o passo "usuário sem privilégio"), D-3, D-8, D-10, S-5, F-V-1.
- **Texto pronto:**

```markdown
### 1.2 Critérios de conclusão (Definição de Pronto da Fase 15)

A fase só é "concluída" quando **todos** os itens abaixo têm evidência lida e registrada (comando, número e data/hora real), ou uma exceção aceita por escrito pelo owner na §8:

1. **Binário no Linux:** `/root/MCP/mcpollama-mcp --version` = o rótulo definido pela questão 19 (hoje `0.10.3-b1faec3`); `initialize` e `tools/list` por stdio listam as 8 tools, com `seed` no esquema de `generate` e `chat`; `sha256sum -c SHA256SUMS` = `OK`. *(Estado em 2026-11-12: atendido para o rótulo atual; falta o `SHA256SUMS`.)*
2. **Testes:** no Linux, os pacotes definidos pela questão 24 com **0 FAIL** e os `SKIP` listados com o motivo; no Windows, `go test -count=1 ./...` com 0 FAIL; na estação, `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...` e `go vet ./...` com rc=0 e `gofmt -l .` vazio (P-8).
3. **S2:** os 2 testes corrigidos sem tautologia e com prova por mutação registrada (P-7); se algum arquivo não de teste mudou, a regra de versão da questão 19 cumprida.
4. **Testes de ataque da Fase 4 no Linux** com 0 ESCAPOU nos vetores existentes e cada vetor Linux novo classificado (P-13), ou o risco aceito por escrito pelo owner antes do 1º uso real das tools de arquivo no Linux.
5. **Árvore:** `/root/MCP` conforme a §4.5 (modos 0700/0600/0755), sem arquivo fora do inventário.
6. **Build repetível** documentado num único lugar e conferido por dois builds com o mesmo hash, ou o registro de que o binário atual não segue a §4.2 (questões 20 e 21).
7. **Documentação Linux:** 7 documentos e o README conforme a questão 22, com o gate da P-10 (0 ocorrências dos padrões, 0 CR, JSON válido por `python3`, sem BOM) e cada comando documentado executado uma vez.
8. **S6:** os itens da P-5 com gate cumprido ou exceção aceita; rótulo esperado visto no `/monitor` (questão 30).
9. **Segurança:** nenhum arquivo com segredo dentro de uma raiz do `--allow-root` além do `.mcp.json` ativo (negado pela lista fixa), conferido por nome **e** por conteúdo (P-12); o `.bak` com token tratado (questão 13); `/root/treino/.mcp.json` ignorado pelo git ou fora de repositório (P-12); varredura de segredos nas linhas adicionadas ao repositório = 0.
10. **Rollback** documentado (§7.1) e, se houver troca de binário, ensaiado uma vez (trocar para o `.bak` e voltar; `--version` confere nas duas pontas).
11. **Pendências** da §6.2 zeradas ou aceitas por escrito.
12. **Documentos atualizados** (S7, P-17), com commits por caminho, sem arrastar trabalho de outra sessão; push e tag só com autorização.
13. **Aprovação do owner** registrada na §0.2 (registro de aprovação formal).
```

### R-15 — §2 item 1 (L32)
- **Tipo:** errata + marcar superado. **Fonte:** F-1.
- **Texto pronto** (acrescentar ao fim da L32):

```markdown
**[errata E-32]** **superado em 2026-11-13 (revisão):** `runtime.GOOS` aparece em `guard.go:120`, `200` e `340`; `SplitRoots` usa `os.PathListSeparator` em `guard.go:229` (comentário em 223-226). "Larga demais" quer dizer aceita **com aviso**, não recusada (errata E-27 na §7).
```

### R-16 — §2 item 2 (L33)
- **Tipo:** errata + marcar superado. **Fonte:** A-4, F-13, D-6.
- **Texto pronto** (acrescentar ao fim da L33):

```markdown
**[errata E-28]** **superado em 2026-11-13 (revisão):** o binário foi executado num Linux real em 2026-11-12 a partir de 21:51 (`--version`, `initialize`, `tools/list`; depois `list_models`, `summarize_file`, `embed`, `extract`, 10 `generate` e o Claude Code; §0.3). Ele **não** foi gerado pelo comando do `CLAUDE.md:84` (que não tem `-ldflags -X main.Version` nem `CGO_ENABLED=0`): `go version -m D:\MCP\mcpollama-mcp` (lido em 2026-11-13) mostra `-ldflags="-X main.Version=0.10.3-b1faec3"`, `CGO_ENABLED=0`, `GOOS=linux`, `GOARCH=amd64`, `vcs.revision=4ae3a87…` (commit só de documentação; o código do MCP é o de `b1faec3`), sem `-trimpath` e sem `-s -w`. Continua verdadeiro: é cópia do build do Windows, não um build repetível (S3).
```

### R-17 — §2 item 3 (L34)
- **Tipo:** errata + marcar superado. **Fonte:** F-17, F-6, A-16.
- **Texto pronto** (acrescentar ao fim da L34):

```markdown
**[errata E-34]** **superado em 2026-11-13 (revisão):** no MCP a recusa de plataforma vem de `ocr.NewPool` (`ocr/pool.go:65-66`); `OpenTesseract` só roda no worker. Texto exato, no stderr (medido no Linux em 2026-11-12 22:17): `WARNING: OCR disabled: ocr: OCR indisponível nesta plataforma; the file tools run without OCR (scanned PDFs and images are refused)`. A partida nunca falha por OCR. Com `--ocr-lib` de caminho Windows (`D:\...`) ou sem `--ocr-lib-sha256`, o aviso é outro ("must be an absolute path" ou "is required"), porque `ocrConfig` (`mcpollama-mcp/ocr.go:32-77`) valida antes da plataforma (leitura de código; não executado no Linux).
```

### R-18 — §2 item 4 (L35)
- **Tipo:** marcar superado. **Fonte:** A-23, A-V-8, D-11.
- **Texto pronto** (acrescentar ao fim da L35):

```markdown
**[superado em 2026-11-13 (revisão): o `hostname` do `lx-ai01` é o FQDN (lido em 2026-11-12), então o rótulo padrão esperado é `ia.cslsolucoes.com.br/root` (`config.go:508-533`), salvo `--client-label`, flag que existe e não estava citada; todas as sessões do root na mesma máquina entram como o mesmo cliente na fila. No `/monitor` a chave é `client_label` (e `station_computer`, `station_user`, `station_domain`, `station_mac`). Não conferido no `/monitor`: questão 30 e P-4]**
```

### R-19 — §2 item 5 (L36)
- **Tipo:** marcar superado (fato conferido). **Fonte:** F-14.
- **Texto pronto** (acrescentar ao fim da L36):

```markdown
**[superado em 2026-11-13 (revisão), conferido em 2026-11-13: backups do binário Linux em `D:\MCP` só para `0.10.1` e `0.10.2`; do `.exe`, `0.9.2`, `0.10.0`, `0.10.1` e `0.10.2`; `cache\pdfium` tem só `wazero-v1.12.0-amd64-windows`; `Documentation\` tem também `LEIA-ME-IA.md`, `README.md`, `TrueNAS\` e `VMware\` (fora desta fase); `Documentation\Ollama\` tem 14 arquivos (7 numerados, `README.md` e 6 `.bak-<data>`: os backups da documentação são por data, não por versão). Não há `VERSION` nem `SHA256SUMS` em `D:\MCP`]**
```

### R-20 — §2 item 6 (L37)
- **Tipo:** marcar superado. **Fonte:** F-15, D-7, F-8, S-10.
- **Texto pronto** (acrescentar ao fim da L37):

```markdown
**[superado em 2026-11-13 (revisão): esses documentos estão **fora do git** (`git ls-files` não os encontra) e em CRLF (100% das linhas); a cópia de 2026-11-12 21:51 em `/root/MCP/Documentation/Ollama` foi feita byte a byte por `tar` (CRLF por inferência, não reverificado). `03-parametros.md:47` já diz "separadas por `;` (`:` no Linux)"; `02-mcp-json-do-zero.md:14,24` cita só `;`. A documentação ensina `--api-key` nos `args` (10 vezes no `02`, 1 vez em `01`, `03`, `06`, `07` e `README.md`), o oposto da D15-6 (risco RS-9). Fonte da documentação: questão 22]**
```

### R-21 — §2 item 7 (L38)
- **Tipo:** marcar superado. **Fonte:** A-23, C-15.
- **Texto pronto** (acrescentar ao fim da L38):

```markdown
**[superado em 2026-11-13 (revisão): o `~/.ssh/config` da estação tem também o alias `lx-ai01` (usuário `claiton`, mesma máquina); a regra de usar só `lx-ai01-root` vem da memória do projeto. Em 2026-11-12 foram 40 comandos com SSH entre 21:51:10 e 22:30:11 (≥44 conexões, fora o laço de reconexão do reinício) e mais 4 até 23:00:50, sem recusa; o `fail2ban` não foi consultado (RS-13)]**
```

### R-22 — §2 item 8 (L39)
- **Tipo:** marcar superado. **Fonte:** A-9, S-4, S-1, F-V-3.
- **Texto pronto** (acrescentar ao fim da L39):

```markdown
**[superado em 2026-11-13 (revisão): decidido pelo owner (questão 8, 2026-11-12 21:54:48, por interpretação declarada; questão 27). Implementado com o token **literal** no bloco `env` do `/root/treino/.mcp.json` (0600): não vai nos `args` nem em `ps` no uso normal. O `/root/MCP/mcpollama.env` (0600) não é lido pelo Claude Code; só serviu de origem da cópia e para testes manuais por stdio (`set -a; . arquivo`). O token está em texto em 3 arquivos do servidor (`/root/MCP/mcpollama.env`, `/root/treino/.mcp.json` e `/root/treino/.mcp.json.bak-windows-20261112`, este dentro da raiz permitida e legível pelas tools: questão 13) e passou como argumento de processo uma vez (22:02:11, errata E-29). Mecanismo de carga: questão 15. Não reverificado em 2026-11-13]**
```

### R-23 — §2.1, células "Situação" (L47, L48, L49, L52)
- **Tipo:** marcar superado; errata na L49. **Fonte:** A-5, F-11 (com o reforço do verificador: `//go:embed`), A-12.
- **Texto pronto** (acrescentar ao fim da célula "Situação" de cada linha):

L47 (motor do PDF):
```markdown
**superado em 2026-11-13 (revisão):** ✅ provado em 2026-11-12 22:15 (`extract` de um PDF de 137.030 bytes, 4 páginas, `paginas_sem_texto=0`); prova estática: `go list -deps ./mcpollama-mcp` (GOOS=linux) inclui `go-pdfium/internal/pdfium_wasm`, que embute `pdfium.wasm` por `//go:embed` (`embed.go:11` do módulo `go-pdfium@v1.21.4`).
```
L48 (cache):
```markdown
**superado em 2026-11-13 (revisão):** populada em 2026-11-12 22:15 (`wazero-v1.12.0-amd64-linux`); o ganho de tempo na 2ª leitura **não** foi medido (P-15).
```
L49 (modernc/x/image):
```markdown
**[errata E-33]** **superado em 2026-11-13 (revisão):** `modernc.org/*` só no gateway; `golang.org/x/image` só em testes com tag `ocr` (`*/ocr_real_test.go`), não no OCR de produção.
```
L52 (Go):
```markdown
**superado em 2026-11-13 (revisão):** o servidor segue sem Go, mas "bloqueia" deixou de valer: a 3.B foi atendida em parte com binários `go test -c` compilados na estação e executados no servidor (§6.1); escopo futuro: questão 24.
```

### R-24 — §3 (L57 e L59 a L64)
- **Tipo:** marcar superado. **Fonte:** A-6 (com os 2 ajustes do verificador), C-14, A-V-1, D-V-4.
- **Texto pronto:**

Acrescentar ao fim da L57:
```markdown
**Superado em 2026-11-13 (revisão):** A, B, D, E e F foram medidos, total ou parcialmente, em 2026-11-12 (estado por item abaixo e na §6.1); C segue aberto (questão 29). O texto abaixo é o da escrita (21:45:51) e fica como histórico.
```
Ao fim da L59 (A):
```markdown
**[superado: respondido em 2026-11-12 21:51:20-21:51:33 (`--version`, `initialize` e `tools/list` por stdio no `lx-ai01`)]**
```
Ao fim da L60 (B):
```markdown
**[superado em parte: 7 dos 24 pacotes com testes executados no Linux em 2026-11-12 22:17 (5 PASS, 2 FAIL de teste que assume Windows); 17 não executados (`cmd/bakeoff`, `cmd/dumptools`, `cmd/mcpollama-gateway` e 14 `gateway/*`); `mcp` não tem testes; sem `-race` e sem `-test.v`; executado como root; ver §6.1 e questão 24]**
```
Ao fim da L61 (C):
```markdown
**[aberto: questão 29]**
```
Ao fim da L62 (D):
```markdown
**[superado em parte: x86_64, CentOS Stream 10, glibc 2.39, 88 GB livres (2026-11-12 21:51:10); kernel não registrado]**
```
Ao fim da L63 (E):
```markdown
**[superado: às 21:56:03, sem mudança no `/etc/hosts`, o nome já resolvia de dentro do servidor (2 endereços IPv6, provavelmente do `nss-myhostname`, não verificado) e a :8443 respondia 401 com TLS válido usando a CA de `/root/MCP/certs/` (copiada às 21:55:56); às 21:58:45 a linha `127.0.0.1 ia.cslsolucoes.com.br` no `/etc/hosts` fixou o loopback IPv4 (questão 16)]**
```
Ao fim da L64 (F):
```markdown
**[superado em parte: funciona como root e o cache `wazero-v1.12.0-amd64-linux` foi criado em 2026-11-12 22:15; o ganho na primeira carga não foi medido (P-15)]**
```

### R-25 — §4.1 (ao fim da L80)
- **Tipo:** marcar superado. **Fonte:** A-10, D-13, D-V-3.
- **Texto pronto:**

```markdown
**Superado em 2026-11-13 (revisão):** a árvore real (2026-11-12) tem também `certs/cloudflare-origin-ca.pem` e `mcpollama.env` (com o token) e não tem `.bak` (não havia binário anterior) nem `SHA256SUMS` (o hash está em `VERSION`, que diz "copiado de D:\MCP\mcpollama-mcp"); inventário completo na §4.5. Nenhuma etapa usa `--allow-root /root/MCP` nem `--allow-root /root` (o smoke test de 21:51 usou `/root/MCP`); backups de arquivos com segredo não ficam sob nenhuma raiz permitida (questão 13).
```

### R-26 — §4.2 (inserir após a L86)
- **Tipo:** marcar superado. **Fonte:** A-10, F-13, D-6, D-16.
- **Texto pronto:**

```markdown
**Superado em 2026-11-13 (revisão):** o binário instalado em `/root/MCP` (e o de `D:\MCP`) **não** foi gerado por este comando: tem só `-ldflags="-X main.Version=0.10.3-b1faec3"`, `CGO_ENABLED=0`, `GOOS=linux`, `GOARCH=amd64`, sem `-trimpath` e sem `-s -w` (`go version -m`, lido em 2026-11-13). Refazer o build por este comando gera **outro hash com o mesmo rótulo** `0.10.3-b1faec3`, o que não se aceita (questão 20). O `go build` embute `vcs.revision`/`vcs.time`: para provar repetibilidade, dois builds em pastas diferentes têm de dar o mesmo SHA-256 (se não derem, avaliar `-buildvcs=false`). O `SHA256SUMS` prova **cópia fiel**, não reprodutibilidade. `file` existe no servidor (rodou em 2026-11-12 21:51); `readelf` não foi testado; `ldd` diz "não é um executável dinâmico". `Build-Release.ps1` existe só em `referencias/MCPVMWare/tools/` (somente leitura); o requisito é o `MCPO-REQ-BUILD-004` (`referencias/Brief_Referencias/60-Requisitos_mcpollama.md:272`, `SHOULD`), e a prática do projeto nunca usou `-trimpath -s -w` (`CLAUDE.md:84`; `deploy/gateway/Instalar-Gateway.ps1:67-68`). Script ou comando manual: questão 21. Comandos e gates: P-9.
```

### R-27 — §4.3 (inserir após a L90)
- **Tipo:** marcar superado. **Fonte:** A-16, F-6, F-16.
- **Texto pronto:**

```markdown
**Superado em 2026-11-13 (revisão):** (a) medido no Linux em 2026-11-12 22:17: com `--ocr-lib` e `--ocr-lib-sha256` válidos o servidor sobe e avisa "OCR disabled: ocr: OCR indisponível nesta plataforma"; "falhar de forma obscura" não se confirmou. Resta, se o owner quiser, citar o Windows no texto e verificar a plataforma antes das outras validações (com caminho Windows o aviso é "must be an absolute path"; leitura de código, `mcpollama-mcp/ocr.go:32-77`); é mudança de código e conta para a versão (questão 19). (b) Ver D15-6 e a questão 15. A regra dos testes de ataque vale também para `src/doctext/` (testes da revisão da Fase 7, conforme o `CLAUDE.md`). Os testes de ataque da Fase 4 nunca rodaram no Linux, mesmo sem mudança no guard (questão 24).
```

### R-28 — §4.4 (inserir após a L94)
- **Tipo:** marcar superado. **Fonte:** A-14, F-15, D-7, D-12, A-V-5, F-9.
- **Texto pronto:**

```markdown
**Superado em 2026-11-13 (revisão):** a "fonte única no repositório" **não existe hoje**: os 7 documentos e o README de `Documentation/Ollama` só existem em `D:\MCP\Documentation\Ollama` (fora do git, CRLF, editados à mão com `.bak-<data>`) e na cópia de 2026-11-12 do `lx-ai01`; `docs/` tem 5 arquivos, nenhum de cliente Linux. A frase acima é recomendação, não decisão; a questão 7 é reformulada pela questão 22. Até lá, a S4b não edita a cópia do servidor (a S7 a sobrescreveria). A documentação Linux trata três cenários de rede: (1) no próprio `lx-ai01`: nome público na :8443, com `--tls-ca /root/MCP/certs/cloudflare-origin-ca.pem` e a linha do `/etc/hosts` (medido em 2026-11-12; questão 16); (2) outra máquina Linux da LAN: o mesmo nome pelo DNS interno (10.100.2.2), **sem** `/etc/hosts`, com a CA pública copiada e o sha256 conferido (não medido); (3) internet: o nome sem porta, pelo túnel (não medido; não se sabe se pede `--tls-ca`). Nunca `https://127.0.0.1:8443` (o TLS valida o nome); o gateway local `127.0.0.1:11500` fica fora (questão 10). Separadores: `--allow-root` usa `:` no Linux e `;` no Windows; `--allowed-models` usa `;` ou `,` nos dois (os nomes têm `:`), e não pode ser "convertido" para `:`.
```

### R-29 — §4.5 Inventário de artefatos (inserir após o bloco do R-28)
- **Tipo:** inserir subseção. **Fonte:** D-13 (com os ajustes do verificador), A-10, S-14, D-V-6.
- **Texto pronto:**

```markdown
### 4.5 Inventário de artefatos (estados lidos em 2026-11-12; não reverificados na revisão de 2026-11-13)

| Caminho | Operação | Modo / dono | Nota |
| --- | --- | --- | --- |
| `/root/MCP/` (`lx-ai01`) | criar | 0700 root | 2026-11-12 21:51:20; não existia antes |
| `/root/MCP/mcpollama-mcp` | criar (cópia de `D:\MCP\mcpollama-mcp`) | 0755 root | `0.10.3-b1faec3`, 22.770.050 bytes, sha256 `d61f5e82…0333`; sem `-trimpath`/`-s -w` (§4.2) |
| `/root/MCP/cache/pdfium/` | criar | 0700 | a 1ª leitura de PDF (22:15) criou `wazero-v1.12.0-amd64-linux` |
| `/root/MCP/Documentation/Ollama/` (7 + README) | criar (cópia) | 0644, pasta 0755 | cópia Windows em CRLF (inferido), sem os `.bak`; ensina `--api-key` nos `args`; a refazer na S4 (questão 22) |
| `/root/MCP/VERSION` | criar | 0600 | versão, sha256 do binário e o texto "copiado de D:\MCP\mcpollama-mcp em 2026-11-12" (proveniência; exceção declarada no gate da S4b); sem hora de build |
| `/root/MCP/mcp.json.exemplo` | criar e editar | 0600 | sem token; o valor do token diz "vem de /root/MCP/mcpollama.env" (o cliente não lê esse arquivo; questão 15) |
| `/root/MCP/certs/cloudflare-origin-ca.pem` | criar | pasta 0700, arquivo 0644 | CA **pública**, 2 certificados, 0 chaves, válida até 15/08/2029, sha256 `01d12a7c…d8f3` |
| `/root/MCP/mcpollama.env` | criar | 0600 root | `OLLAMA_API_KEY` (64 caracteres, sem CR desde 22:00:02); **contém o token**; negado pela lista fixa (sufixo `.env`) |
| `/etc/hosts` (fora de `/root/MCP`) | editar | — | +3 linhas (vazia, comentário da Fase 15 e `127.0.0.1   ia.cslsolucoes.com.br`); backup `/etc/hosts.bak-20261112` (`cp -p`); a linha persistiu após o reinício (questão 16) |
| `/root/treino/.mcp.json` | refazer (Linux) | 0600 root | 486 bytes; `command` `/root/MCP/mcpollama-mcp`; token literal no `env`; negado pela lista fixa (nome exato); `/root/treino` tem `.git` (P-12) |
| `/root/treino/.mcp.json.bak-windows-20261112` | criar | 0600 root | **token em texto**, caminhos `D:\`/`E:\`, OCR; **dentro do `--allow-root`** e **lido** pelas tools (questão 13) |
| `/tmp/mcptest/…` | criar e apagar | — | apagado às 22:18:04 |
| `/tmp/x.bin` | criar e apagar no mesmo comando | — | 23:00:50, 1ª revisão (excedeu o pedido); conferir na P-12 |
| processo `mcpollama-mcp` em 2º plano (23:00:50) | possível, improvável | — | conferir `pgrep -a mcpollama-mcp` (P-12) |
| reinício do servidor | executar | — | 22:27:58, boot 22:28; estado final lido às 22:30:11 |
| `/root/.bashrc` | alterado **pelo owner** (22:25:13 e 22:47:12) | — | `CLAUDE_CODE_OAUTH_TOKEN` válido desde 22:47:12; não é ação do plano (questão 17) |
| `/root/.claude/.credentials.json`, `/root/.claude.json` | só lidos (`ls`) | 0600 | preexistentes (21:31) |
| Estação: `D:\MCP\*` | nada alterado | — | hash do `mcpollama-mcp` reconferido em 2026-11-13 11:50 |
| Estação: binários de teste `go test -c` | criar (pasta temporária da sessão de 2026-11-12) | — | temporários locais |
| Repositório: este plano | criar e editar | — | commits `f6bc6ab` e `7fbe5cc` (push às 22:32:54); nenhum código |

**Faltam (previstos, não criados):** `SHA256SUMS` (questão 20); `mcpollama-mcp.bak-<versão>` (só na próxima troca, P-9); documentação Linux (S4, questão 22); build repetível documentado (questão 21); entradas de `CHANGELOG.md`, `README.md`, `CLAUDE.md` e `deploy/servidor-ia/` (S7, P-17); pasta de backups fora de qualquer raiz (questão 13).
```

### R-30 — §5, coluna "Estado" de D15-1 a D15-7 (L100 a L106)
- **Tipo:** marcar superado (ao fim da célula "Estado" de cada linha). **Fonte:** A-7, A-8, A-9, C-12, F-12, D-5, A-V-12, S-4.
- **Texto pronto:**

D15-1 (L100):
```markdown
**[em 2026-11-13: aberta. `git diff b1faec3 HEAD` nos pacotes do MCP e no `go.mod`/`go.sum` é vazio; não há 0.10.4 no `CHANGELOG.md` nem tag; a regra não diz se mudar só `*_test.go` gera versão: questão 19]**
```
D15-2 (L101):
```markdown
**[em 2026-11-13: efetivada por ação (linux/amd64 num x86_64, 2026-11-12 21:51); falta resposta formal à questão 4]**
```
D15-3 (L102):
```markdown
**[em 2026-11-13: efetivada por ação (o `/root/treino/.mcp.json` não usa OCR; `libtesseract` e `tessdata` não copiados); falta resposta formal à questão 5]**
```
D15-4 (L103):
```markdown
**[em 2026-11-13: respeitada (os 2 commits tocam só este plano); o gateway reiniciou junto com o servidor às 22:28, por ordem do owner]**
```
D15-5 (L104):
```markdown
**superado em 2026-11-13 (revisão):** a precondição "só depois de a S1 e a S2 passarem" foi dispensada pelo owner em 2026-11-12 21:50:56 ("você mesmo faz o deploy para o linux, não necessita de empacotamento, só colocar tudo na pasta /root/MCP"); a instalação foi às 21:51:20, com a máquina inferida (confirmada às 21:57:20). Continua valendo para o que falta (troca do binário, nova versão, tag, S4b, novo reinício): autorização explícita do owner por etapa, com impacto e rollback (§7.1).
```
D15-6 (L105):
```markdown
**superado em 2026-11-13 (revisão):** decidida em 2026-11-12 21:54:48 (por interpretação declarada; questão 27) e implementada de outro modo: token literal no `env` do `/root/treino/.mcp.json` (0600), fora dos `args`; o `mcpollama.env` não é carregado pelo cliente; 3 cópias no servidor. Mecanismo de carga: questão 15.
```
D15-7 (L106):
```markdown
**[em 2026-11-13: commits por caminho, mas não "por etapa verde" (são commits só deste plano): `f6bc6ab` (21:53:31, autorizado às 21:53:26) e `7fbe5cc` (22:32:51) no `master`, enviados ao `origin/master` às 22:32:54 com autorização do owner (22:32:37, "ok, commit tudo e push"); a regra continua valendo para o que vier]**
```

### R-31 — §6, frase da L110
- **Tipo:** marcar superado (acrescentar, sem substituir; ajuste do verificador da A-11). **Fonte:** A-11, D-1.
- **Texto pronto** (acrescentar ao fim da L110):

```markdown
**Superado em 2026-11-13 (revisão):** em 2026-11-12 isso não foi feito por etapa (só em prosa no changelog). O estado por etapa está na coluna Estado abaixo e na §0.2; os números medidos na §6.1; as pendências, com procedimento e gate, na §6.2 (P-n). Várias etapas foram executadas fora de ordem por ordem direta do owner (§0.3). Quem seguir a tabela ao pé da letra repetiria a S1(b) num temporário, "criaria" `/root/MCP`, trocaria o binário em uso (a §4.2 muda o hash e mantém o rótulo) e editaria `/root/treino` sem saber que o `.mcp.json` já foi convertido e que há um backup com token dentro da raiz. Antes de qualquer etapa, ler a §0.2 e a §6.2.
```

### R-32 — §6, coluna "Estado" (L112 a L123)
- **Tipo:** coluna Estado (moldura, item 6; o texto das células fica). **Fonte:** D-1 (com as correções), A-11.
- **Texto pronto:** substituir a linha de cabeçalho (L112) e a de separador (L113) por estas duas, e acrescentar ao fim de cada linha de etapa (depois do último `|`) a célula indicada:

```markdown
| Etapa | O que fazer | Quem | Gate | Estado (2026-11-13; servidor não reverificado) |
| --- | --- | --- | --- | --- |
```

| Linha | Acrescentar ao fim da linha |
| --- | --- |
| L114 (S0) | ` parcial, fora de ordem (§0.2) |` |
| L115 (S1) | ` parcial, fora de ordem: (a) e (b) em 2026-11-12 21:51, (b) dentro de /root/MCP; (c) parcial (§0.2, §6.1) |` |
| L116 (S2) | ` pendente (P-7; questão 19) |` |
| L117 (S3) | ` pendente (P-9; questões 20 e 21) |` |
| L118 (S4) | ` pendente; mcp.json.exemplo feito fora de ordem (P-10; questão 22) |` |
| L119 (S4b) | ` pendente; bloqueada pelas questões 13, 22 e 23 |` |
| L120 (S5) | ` fora de ordem; gate cumprido em parte (sem S3 e sem SHA256SUMS) |` |
| L121 (S6) | ` parcial (P-2 a P-6; questões 25 e 30) |` |
| L122 (S7) | ` pendente (P-17) |` |
| L123 (S8) | ` pendente; condicional à questão 19 (P-18) |` |

Observação para o editor: as linhas da tabela terminam em `|`; a célula nova vem depois dele, com espaço, e fecha com `|`. Nenhuma célula existente é alterada, exceto pelos acréscimos do R-33.

### R-33 — §6, acréscimos dentro das células (S1, S2, S3, S4b, S5, S6, S8)
- **Tipo:** marcar superado; errata E-13 na S4b. **Fonte:** A-11, A-12, A-13 (ajustado), C-5, F-5, F-6, F-V-1, F-V-2, S-2, S-3, S-5, S-V-2, D-8, D-9, D-V-3, D-10, D-16, A-V-5.
- **Texto pronto** (acrescentar ao fim da célula "O que fazer" da etapa indicada):

S1 (L115):
```markdown
**superado em 2026-11-13 (revisão):** (b) foi executada direto em `/root/MCP` em 2026-11-12 21:51 (§0.3); (c) seguiu outro método (binários `go test -c` compilados na estação), com cobertura menor; procedimento e gate atuais: P-8; escopo: questão 24.
```
S2 (L116):
```markdown
**superado em 2026-11-13 (revisão):** como corrigir os 2 testes sem tautologia e com mutação: P-7 (as propostas do changelog "22:45", item C(1), ficaram superadas: errata E-35). O `--help` já diz "(: on Linux)" na linha 145; mudar a 1ª linha é opcional e muda o binário (questão 19). "Verde no Linux" é definido pela questão 24; os testes de ataque da Fase 4 rodam no Linux mesmo sem mudança em `src/files/`, porque nunca rodaram lá (P-13).
```
S3 (L117):
```markdown
**superado em 2026-11-13 (revisão):** o binário atual não segue o comando da §4.2 (ver o parágrafo "Superado" da §4.2); nunca reconstruir com o rótulo `0.10.3-…`; comandos e gates: P-9; questões 20 e 21.
```
S4b (L119) — primeiro, inserir o marcador logo depois de `22:03` (dentro do parêntese "pedido do owner em 2026-11-12 22:03"):
```markdown
**[errata E-13]**
```
e acrescentar ao fim da célula:
```markdown
**superado em 2026-11-13 (revisão):** (1) "nada foi alterado ainda" vale para a documentação e para os demais arquivos de `/root/treino`; o `/root/treino/.mcp.json` já tinha sido convertido às 22:02:11 por outra ordem (22:01:51) e **não** é alvo. (2) Backups: **nunca** dentro de `/root/treino`, `/root/MCP` ou outra raiz do `--allow-root` (a lista fixa nega `.mcp.json` só por igualdade e `.env` só por sufixo, `files/policy.go:105-107`; `.mcp.json.bak-*`, `settings.local.json.bak-*`, `mcpollama.env.bak-*` e `.cursor/mcp.json` são lidos pelas tools); destino conforme a questão 13. (3) Exclusões: questão 23; o gate exclui `*.bak-*`. (4) "URL pública resolvendo para 127.0.0.1 pelo /etc/hosts" vale só no próprio `lx-ai01` (cenários de rede na §4.4). (5) Token: conforme a questão 15; o `mcpollama.env` não é lido pelo cliente; segredo nunca como argumento de comando. (6) Inventário também por conteúdo e de symlinks, antes de editar (P-12); mapeamento, gate redigido e conferência de CR: P-10.
```
S5 (L120):
```markdown
**superado em 2026-11-13 (revisão):** executada em 2026-11-12 21:51:20 por ordem do owner, antes da S1 a S4 (D15-5); o gate "hash igual ao da S3" não pôde ser cumprido (a S3 não existiu; o hash foi conferido contra `D:\MCP`). Troca futura do binário: P-9 (rename atômico no mesmo diretório; nunca `cp` por cima do binário em uso).
```
S6 (L121):
```markdown
**superado em 2026-11-13 (revisão):** itens e gates por item na P-5; rótulo esperado `ia.cslsolucoes.com.br/root`, na chave `client_label` do `/monitor` (P-4; questão 30); `reportar_erro` grava em produção (questão 25). Antes do 1º uso real das tools de arquivo pelo cliente no Linux: questão 13 resolvida e testes de ataque no Linux feitos (questão 24) ou risco aceito por escrito.
```
S8 (L123):
```markdown
**superado em 2026-11-13 (revisão):** o projeto não faz release no GitHub, só tag anotada (`CHANGELOG.md:4-5`): o gate passa a ser "tag anotada criada e conferida (`git tag -l`)", só com autorização.
```

### R-34 — §6.1 Resultado dos testes no Linux (inserir após a L123)
- **Tipo:** inserir subseção. **Fonte:** D-2 (tabela), D-3, F-19, A-V-9, F-7, S-5 (P3/P4), S-V-8, 02 "outras contradições" item 3.
- **Texto pronto:**

```markdown
### 6.1 Resultado dos testes no Linux

**Rodada de 2026-11-12 22:17-22:18 (servidor `lx-ai01`, como root; não reverificada em 2026-11-13).** Método: binários `GOOS=linux CGO_ENABLED=0 go test -c` compilados na estação e executados no servidor (o servidor não tem Go), com o diretório corrente na pasta de cada pacote; **sem `-test.v`**, por isso a contagem de `SKIP` é desconhecida. Código do MCP = `b1faec3` (nenhuma mudança depois).

| Pacote | Resultado | Nota |
| --- | --- | --- |
| `redact` | rc=0, PASS | |
| `files` | rc=1, **FAIL** `TestSplitRoots` (`guard_test.go:125`: obteve um só item) | o teste assume `;`; no Linux o separador é `:` (`os.PathListSeparator`, `guard.go:229`); correção na P-7 |
| `ollama` | rc=0, PASS (1 s) | |
| `ocr` | rc=0, PASS | só os testes de plataforma não suportada (`ocr_other.go`); o OCR não existe no Linux (D-10) |
| `doctext` | rc=0, PASS (16 s de execução) | |
| `mcpollama-mcp` | rc=1, **FAIL** `TestWrongHashServerStarts` (`ocr_test.go:173`) | no Linux o aviso é o de plataforma, não o de hash; correção na P-7. A 1ª rodada teve também `TestNoLiteralContextLimits`, por falta de `../mcp/types.go` (erro de envio); repetido com `src/mcp`, passou |
| `tools` | rc=0, PASS (8 s) | |

**Dependências de arquivo dos testes executados fora do repositório:** os `.go` do próprio pacote, `../mcp/types.go` e `../../TOOLS.md` (`config_test.go:194`, `tools_wiring_test.go:409`, `tools/registry_test.go:169`, `ollama/security_test.go:216`); não há `testdata`. Sem as fontes, `TestArgumentHelpersLiveInOneFile` passa "no vazio" (ignora o erro de `os.ReadDir`).

**Não executados no Linux:** 17 pacotes com testes (`cmd/bakeoff`, `cmd/dumptools`, `cmd/mcpollama-gateway` e 14 `gateway/*`); `mcp` não tem testes. `-race` só roda no Windows (cgo + gcc de `deps\`) e não cobre os arquivos `!windows`.

**Medições da revisão (estação Windows, 2026-11-13, somente leitura):** `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go vet ./...` em `src/` → rc=0, sem saída (compila todos os testes para Linux; não é execução); `gofmt -l .` → vazio; `go test -count=1 ./files ./mcpollama-mcp` no Windows → ok. Testes de ataque da Fase 4, rodados no Windows sobre o código atual: 208 BLOQUEADO, **0 ESCAPOU**, 2 ACHADO (`deny/gaps`, `cfg/drive-root`), 1 INFO e 1 `SKIP` (`TestDenylistWindowsShortName`, nomes 8.3); para Linux, compilam (`vet` limpo, binário de teste gerado), mas **não foram executados** (a estação não tem WSL e a revisão não acessou o servidor).
```

### R-35 — §6.2 Pendências numeradas (inserir após a §6.1)
- **Tipo:** inserir subseção (P-n, com procedimento e gate). **Fonte:** D (tabela P-1 a P-18), D-2 com D-V-1 e D-V-5, D-4/F-5/F-6, D-6, D-8 com D-V-2, D-9 com D-V-3, D-10, D-11, D-17, S-5/S-V-5, S-7, S-V-2, S-12, S-10, A-5, C-16.
- **Texto pronto:**

```markdown
### 6.2 Pendências numeradas (P-n)

Cada pendência diz o que fazer, o gate e quem decide. Nas células, `\|` é o caractere `|` escapado para a tabela markdown (nos comandos, ler como `|`). Nada aqui foi executado pela revisão; tudo que toca o servidor depende de autorização do owner (D15-5) e respeita o `fail2ban`: um `ssh` por passo, com os comandos agrupados.

| P | Pendência | Como fazer / gate | Quem |
| --- | --- | --- | --- |
| P-1 | Aprovação formal do plano (S0) | preencher o registro da §0.2; questão 28 | owner |
| P-2 | Aprovação interativa do servidor de projeto em `/root/treino` | depois da questão 13: o owner roda `cd /root/treino && claude` e aprova; gate: `claude mcp list` em `/root/treino` sem "Pending approval" (estado de partida: "Pending approval" em 2026-11-12 22:56:42) | owner |
| P-3 | Chamada normal pelo cliente | depois da P-2, em shell de login, **sem** `--mcp-config`, `--strict-mcp-config` nem `env -u`: `cd /root/treino && claude -p "Chame a ferramenta list_models do servidor mcpOllama e responda só com os nomes" --allowedTools mcp__mcpOllama__list_models`; gate: rc=0 e os nomes dos modelos. Estado de partida: tentada às 22:25:28 (401 pela variável inválida); após o token válido (22:47:12) só se testou `claude -p "ok"` | sonnet |
| P-4 | Rótulo no `/monitor` | ler `https://ia.cslsolucoes.com.br:8443/monitor/api/state` (leitura livre, só LAN e loopback) com `--cacert /root/MCP/certs/cloudflare-origin-ca.pem`, percorrer `recent[]` e `queue.running[]` com `python3` e imprimir `client_label`, `station_computer`, `station_user`, `station_domain`, `station_mac`; gate: ≥1 item com `client_label` igual ao valor da questão 30 e `station_user == root`; se os 10 jobs de 22:29 já saíram de `recent` (50 itens), gerar 1 `generate` curto e ler de novo | sonnet |
| P-5 | S6 completa | uma chamada por vez: (1) `chat`, 2 mensagens curtas, `seed` fixo → `isError` falso e linha `[uso]` com o modelo e o `job`; (2) `translate` de um `.md` pequeno da raiz para `en` → `isError` falso, identificadores e blocos de código preservados (3 por amostragem); (3) `summarize_file` de `.txt` e de PDF com texto → `formato=txt` e `formato=pdf paginas=<n> paginas_sem_texto=0`; (4) `.docx` e `.pptx` pequenos numa 2ª raiz de teste (separada por `:`, apagada depois) → `formato=docx`/`pptx`; (5) PDF digitalizado → `isError` verdadeiro com "o PDF não tem camada de texto (provavelmente é digitalizado); OCR ainda não é suportado" (`doctext.go:217`), sem chamar o modelo; (6) `.png`/`.jpg` → `isError` verdadeiro com "imagem requer OCR, que não está habilitado no servidor" (`doctext.go:230`); (7) `reportar_erro` conforme a questão 25; (8) rótulo: P-4; (9) aprovação: P-2; (10) chamada normal: P-3; (11) se houver versão nova: sessão reiniciada e `--version` visto pelo cliente | sonnet; (7) owner |
| P-6 | `reportar_erro` no Linux | questão 25; se (b): job do próprio teste, categoria `outro`, `erro` começando por `[TESTE Fase 15, descartar]`, `resposta_correta` vazia, `projeto` `mcpollama-fase15`, e descarte pelo admin no `/monitor` | owner |
| P-7 | S2: corrigir os 2 testes | **`TestSplitRoots`:** literais por plataforma, sem usar `os.PathListSeparator` no teste (`sep, outro := ":", ";"`; no Windows `";", ":"`); casos: `" a "+sep+" "+sep+"b"+sep+sep+" c "` → `a|b|c`; `"a"+outro+"b"` → **um** item (no Linux `;` é caractere válido de nome; no Windows `:` é o da unidade); `""` → vazio. Mutação obrigatória: trocar `os.PathListSeparator` por `';'` fixo em `guard.go:229` deve falhar no Linux; por `':'`, no Windows; restaurar e ver verde nos dois. Base do `:` no Linux: `9ef7935` e `config.go:145` (a decisão escrita de 2026-10-23 fala só em `;`). **`TestWrongHashServerStarts`:** **sem** `t.Skip`; fora do Windows exigir `code == 0`, 1 linha com `summarize_file`, stderr com `WARNING: OCR disabled: ` e `indisponível nesta plataforma`, sem o caminho da raiz; no Windows manter `SHA-256`. Mutação: remover o `if !platformSupported` de `ocr/pool.go:65` deve quebrar o teste no Linux. Por prudência, rodar os testes de ataque da Fase 4 depois de mexer em `src/files/guard_test.go`. Gate: `go test -count=1 ./...` no Windows com 0 FAIL; Linux conforme a P-8; `-race` (Windows) nos pacotes tocados | sonnet + revisão opus |
| P-8 | Testes no Linux (S1(c) e gates da S2/S3) | (1) na estação, a partir de `E:\MCPOllam\src`, para cada pacote do escopo da questão 24: `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c -o <tmp>/lt/<p>.test ./<p>`; (2) empacotar a partir da **raiz do repositório**: `tar cf - --exclude='*.exe' -C E:/MCPOllam src/tools src/files src/doctext src/ollama src/redact src/mcpollama-mcp src/ocr src/mcp TOOLS.md -C <tmp>/lt .` e, no destino, `mkdir -p /tmp/fase15-lt && tar xf - -C /tmp/fase15-lt`; conferir `ls /tmp/fase15-lt/TOOLS.md /tmp/fase15-lt/src/mcp/types.go` e `ls /tmp/fase15-lt/src/<p>/*.go | wc -l` > 0 em cada pacote; (3) executar: `cd /tmp/fase15-lt/src/<p> && ../../<p>.test -test.v -test.count=1 -test.timeout=10m > /tmp/fase15-lt/<p>.out 2>&1; echo rc=$?` e resumir `rc`, tempo e as contagens de `--- PASS`, `--- FAIL` e `--- SKIP`, **listando cada SKIP**; (4) limpar com `rm -rf /tmp/fase15-lt` e provar com `ls` (deve falhar). **Gate de compilação (estação):** `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...`, `go vet ./...` e `gofmt -l .` com rc=0 e saída vazia (partida medida em 2026-11-13: `vet` rc=0, `gofmt` vazio). **Gate S1(c):** rc=0 e 0 FAIL em cada pacote, salvo, até a S2, exatamente `files/TestSplitRoots` e `mcpollama-mcp/TestWrongHashServerStarts`. **Gate S2/S3:** 0 FAIL absoluto e SKIPs listados | sonnet |
| P-9 | S3 (build) e S5 (troca do binário) | conforme as questões 20 e 21. **Build** (só com versão nova): árvore limpa (`git status --short src` vazio); `cd src; GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=<versão>-<hash do commit>" -o <dir>/mcpollama-mcp ./mcpollama-mcp`; gates: `go version -m` com `-trimpath=true`, `CGO_ENABLED=0`, `GOOS=linux`, `GOARCH=amd64`, `vcs.modified=false`; dois builds em pastas diferentes com o mesmo SHA-256; no servidor `head -c 4 mcpollama-mcp \| od -An -tx1` = `7f 45 4c 46`, `file` diz ELF 64-bit estático, `ldd` "não é um executável dinâmico", `--version` correto, `sha256sum -c SHA256SUMS` = OK. **Troca:** `cd /root/MCP && sha256sum -c SHA256SUMS` (do novo); `cp -p mcpollama-mcp mcpollama-mcp.bak-<versão atual>`; `install -m 0755 <novo> .mcpollama-mcp.new && mv -f .mcpollama-mcp.new mcpollama-mcp` (rename atômico, sem ETXTBSY); `./mcpollama-mcp --version`; reiniciar as sessões do Claude Code que usam o MCP. **Prova do ETXTBSY** (opcional, só em `/tmp`): `cp /bin/sleep /tmp/etxt && /tmp/etxt 30 & cp /bin/true /tmp/etxt` deve falhar com "Text file busy"; `cp /bin/true /tmp/etxt.new && mv -f /tmp/etxt.new /tmp/etxt` deve passar; apagar os temporários | owner + sonnet |
| P-10 | S4 e S4b | depois das questões 13, 22 e 23. **Mapeamento:** `D:\MCP\mcpollama-mcp.exe` → `/root/MCP/mcpollama-mcp`; `D:\MCP\cache\pdfium` → `/root/MCP/cache/pdfium`; CA em `%USERPROFILE%\.openclaude\certs\…` → `/root/MCP/certs/cloudflare-origin-ca.pem`; `--api-key <TOKEN>` nos `args` → conforme a questão 15; `\\` nos caminhos JSON → `/`; `E:\P1;E:\P2` no `--allow-root` → `/root/p1:/root/p2`; `--ocr-*`, `libtesseract64.dll`, `tessdata` → removidos, com o texto "OCR só no Windows (D-10): PDF digitalizado e `.png`/`.jpg` são recusados" e as mensagens reais (`doctext.go:217,230`); PowerShell (`$env:`, `Copy-Item`, `Get-FileHash`, `Get-Content`) → bash (`export`, `cp`, `sha256sum`, `cat`); CRLF → LF; notas históricas de `qwen3-ia` (`01:68`, `06:26`, `07:86`, `README:4`) removidas de propósito, listadas; rede: os 3 cenários da §4.4; no topo de cada documento Linux, "Gerada de <fonte> em <data>". **Contagem de partida** (2026-11-13, nos 8 arquivos de `D:\MCP\Documentation\Ollama`): `D:\` em 50 linhas, `E:\` em 1, `C:\` em 5, `.exe` em 30, `powershell` em 13 (sem distinguir caixa). **Antes de editar `/root/treino`:** `find /root/treino -maxdepth 3 -type l -printf '%p -> %l\n'` e `find /root/treino/.claude /root/treino/.agents -maxdepth 1 -printf '%y %p\n'`; editar só arquivos reais, nunca por dois caminhos. **Gate (redação testada antes contra um arquivo de controle com `D:\` e outro que cita `mcp.json.exemplo`):** `grep -rIl -i -E 'D:\\\|E:\\\|C:\\\|[CDE]:/\|qwen3-ia\|\.exe\b\|powershell\|\$env:\|libtesseract\|USERPROFILE' /root/MCP/Documentation /root/MCP/mcp.json.exemplo <alvos de /root/treino> \| grep -v -E '\.bak-\|\.transcricao\.md$\|\.csv$'` = 0 linhas (exceção declarada: o `VERSION`, que guarda a proveniência); CR: `grep -rIl $'\r' <alvos>` = 0 linhas; JSON: `python3 -c 'import json,sys; json.load(open(sys.argv[1]))' /root/treino/.mcp.json` rc=0 (o servidor não tem `node`); BOM: `head -c 3 <arquivo> \| od -An -tx1` ≠ `ef bb bf`; backups conforme a questão 13; `find /root/treino \( -name '*.bak*' -o -name '*mcp.json*' -o -name '*.env*' \)` só com os previstos. No `--allowed-models`, não trocar `;` por `:` | sonnet |
| P-11 | `.mcp.json.bak-windows-20261112` com token dentro da raiz | questão 13; depois, conferir por `ls -la /root/treino/.mcp.json*` e registrar a hora | owner |
| P-12 | Pré-checagens somente leitura no servidor | questão 14 (lista fechada de comandos, sem imprimir valores): `git` em `/root/treino` (instalado? repositório? `.mcp.json` ignorado, versionado, com remoto?), varredura por conteúdo, symlinks, `.cursor` em `/root/treino`, origem dos IPv6 (`nsswitch`), `pgrep -a mcpollama-mcp`, `ls /tmp/x.bin`, CR na documentação copiada, `ls -la /root/treino/.mcp.json*`. Se o `.mcp.json` com token estiver versionado ou num remoto: tratar como incidente de credencial (rotação por `deploy/servidor-ia/nginx/ollama-key-rotate.sh`, por decisão do owner) | owner + sonnet |
| P-13 | Testes de ataque da Fase 4 no Linux | questão 24. Suíte: `.workspace/reports/MCPOllam2026-10-23-2130-revisao-adversarial-fase4-arquivos.testes/` (compila para Linux), mais vetores Linux: hardlink de root para fora da raiz, symlink para `/proc/...`, FIFO na troca `Stat`→`Open` (SEC-m-08, `syscall.Mkfifo`; `guard.go:394` sem `O_NONBLOCK`), `mknod`, bind mount (esperado hoje: lido, risco residual), nomes `.mcp.json.bak*` e `mcp.json` (esperado hoje: lidos, questão 31). No `lx-ai01`, só alvos-isca criados pelo teste e apagados ao fim (conferido por `ls`); hardlink para arquivo real do sistema, `mknod` e bind mount **só** em contêiner ou VM. Gate: 0 ESCAPOU nos vetores existentes, no Windows e no Linux; cada vetor novo com resultado lido e classificado; SKIPs de cada sistema listados com o motivo; log em `.workspace/reports/`. Decidir se o SEC-m-08 vira correção (muda `src/files/` e a versão) | owner + sonnet + revisão opus |
| P-14 | `/etc/hosts`: dependências e túnel | questão 16; somente leitura: `/etc/sssd`, `/etc/krb5.conf`, `/etc/postfix`, `/etc/rsyslog*`, `/etc/fail2ban`, crontabs; `getent ahosts ia.cslsolucoes.com.br`, `hostname -i`; origem do túnel no painel da Cloudflare (o `cloudflared` usa `--token-file`); teste do caminho público de fora da LAN (esperado 401 sem token) | owner + sonnet |
| P-15 | Ganho do cache do pdfium | opcional: medir duas leituras do mesmo PDF, com o cache vazio e com o cache populado | sonnet |
| P-16 | Cenários de rede 2 (LAN) e 3 (internet) | só se o owner quiser documentá-los com prova | owner |
| P-17 | S7, conteúdo por arquivo | `CHANGELOG.md`: na seção "Não lançado" (linha 53), entrada "Fase 15: `mcpollama-mcp` no Linux (`/root/MCP`, `lx-ai01`)" no formato da entrada de infraestrutura de 2026-11-08 (sem número de versão, se a questão 19 ficar em (a)), e marcar que a linha 81 ("não executado em Linux") deixou de valer; se houver 0.10.4, seção própria com data/hora real, hash e tag. `README.md`: na tabela de versões (linha 72), a instalação Linux e a nota "Linux" (OCR só no Windows; `:` no `--allow-root`; token conforme a questão 15). `CLAUDE.md`: a linha de build Linux completa (hoje a linha 84 não tem `-ldflags -X main.Version` nem `CGO_ENABLED=0`), uma frase de estado e `/root/MCP` na arquitetura, **só** depois de a outra sessão commitar as suas mudanças (ou por ordem do owner), com `git diff -- CLAUDE.md` lido antes. `deploy/servidor-ia/` (`inventario.txt`, `README.md`): a linha do `/etc/hosts` e a árvore `/root/MCP`, com o "Reverter" da §7.1. Fora do repositório: `CONEXAO-IA.md` e a memória do projeto. O plano mestre (`...implementacao.plan.md`) não registra as Fases 11 a 15 (reportar). Gate: `git status --short` e `git diff --stat` antes de qualquer `git add`; `git add` por caminho; `git diff --cached` conferido; 0 arquivos de outra frente | sonnet |
| P-18 | S8 | só se a questão 19 gerar versão; tag anotada (questão 32 para a de infraestrutura); push só com autorização | owner |
| P-19 | Valor inválido de `CLAUDE_CODE_OAUTH_TOKEN` parcialmente exposto | questão 18. Nunca rodar `env`, `printenv` nem inspeção de formato que imprima trecho de segredo; tamanho e contagem de caracteres válidos bastam | owner |
| P-20 | Documentação Windows no servidor até a S4 | só com autorização do owner: um `LEIA-ANTES.md` (ou nota no `README.md` de `/root/MCP/Documentation/Ollama`) dizendo "documentação do Windows; não usar no Linux", ou retirar a cópia (questão 22) | owner |
```

### R-36 — §7, acréscimos nas linhas existentes (L129 a L138)
- **Tipo:** marcar superado; errata E-27 na L132. **Fonte:** A-4, A-17, C-14, F-2, F-4, S-4, S-6, D-6, F-13.
- **Texto pronto** (acrescentar ao fim da célula "Mitigação" de cada linha):

L129:
```markdown
**[superado em 2026-11-13 (revisão): risco fechado; o binário foi executado no Linux em 2026-11-12 21:51 (§0.3)]**
```
L130:
```markdown
**[superado em parte: 5 dos 7 pacotes executados passam no Linux e 2 falham por teste que assume Windows; 17 pacotes com testes não executados (§6.1; questão 24)]**
```
L131:
```markdown
**[2026-11-13: os testes de ataque nunca rodaram no Linux, mesmo sem mudança no guard; RS-7 e P-13]**
```
L132:
```markdown
**[errata E-27]** **superado em 2026-11-13 (revisão):** `/` como raiz **não** é recusado: é aceito e gera só `WARNING` na partida (`server.go:178-185`; decisão SEC-M-02 de 2026-10-23 22:10, plano mestre linha 517). No Linux o aviso cobre só `/` e o home do usuário (`guard.go:170-179` só lista diretórios de sistema do Windows): `--allow-root /etc`, `/proc` ou `/usr` sobem sem aviso (em `/proc`, `environ` e `cmdline` são recusados por terem bytes nulos, mas os arquivos de texto são lidos). Com `--allow-root /root`, a lista fixa não cobre `.bashrc` (onde está o `CLAUDE_CODE_OAUTH_TOKEN`), `.claude.json`, `.netrc`, `known_hosts` nem backups com sufixo (lacunas aceitas pelo owner na SEC-M-03, 2026-10-23 22:10, linha 518). Recomendação: nunca `/root` nem `/root/MCP` como raiz. O risco maior hoje é o cliente: RS-5 e questão 26.
```
L133:
```markdown
**superado em 2026-11-13 (revisão):** cumprido para `ps` no uso normal (token no `env` do `.mcp.json`); riscos novos: token em 3 arquivos do servidor e passagem por argumento de processo às 22:02:11 (RS-4; questão 15).
```
L134:
```markdown
**[2026-11-12: não seguida (40 comandos com SSH em 39 min, sem banimento registrado); RS-13]**
```
L135:
```markdown
**superado em 2026-11-13 (revisão):** medido às 21:56:03 (o nome já respondia) e fixado em loopback com a linha do `/etc/hosts` às 21:58:45; risco dessa alteração: RS-3; questão 16.
```
L136:
```markdown
**[2026-11-13: hoje são o mesmo arquivo (sha256 `d61f5e82…0333`); a S3 pelo comando da §4.2 os tornaria diferentes com o mesmo rótulo (RS-11; questão 20)]**
```
L137:
```markdown
**[2026-11-13: a mensagem já é clara no caso válido (§4.3); com caminho Windows o aviso é outro (§2 item 3)]**
```
L138:
```markdown
**[2026-11-13: em 2026-11-12 não havia binário anterior nem sessão usando; para a próxima troca, rename atômico no mesmo diretório (P-9), nunca `cp` por cima do binário em uso (ETXTBSY, semântica POSIX não medida); as sessões abertas continuam no binário antigo até reiniciar]**
```

### R-37 — §7, linhas novas de risco (inserir ao fim da tabela, após a L138)
- **Tipo:** inserir (prefixo **RS-n**, para não colidir com os reparos R-n; ajuste do verificador da A-17). **Fonte:** S-1/S-V-3, F-V-1/S-2/D-V-3, S-7/A-V-1, S-4/S-11, S-6, S-8, S-5/S-V-5, F-2, S-10, A-17, A-10/D-6, S-12, C-15/D-18, S-9.
- **Texto pronto** (linhas da mesma tabela, 3 colunas):

```markdown
| **RS-1 (2026-11-13, crítico).** Arquivo com segredo dentro do `--allow-root` sob nome fora da lista fixa: `/root/treino/.mcp.json.bak-windows-20261112` (token em texto, 0600 root, criado 2026-11-12 22:02:11) | `summarize_file`/`extract`/`translate` o leem (provado na revisão com conteúdo falso); o token vai no prompt ao gateway (o `redact` só age na saída e só para o valor idêntico), fica 7 dias no `content.db` (visível ao admin no `/monitor`) e, com `reportar_erro`, no `treino.db` (sem prazo) e no que for exportado dele | questão 13; regra: nenhum backup de arquivo com segredo dentro de raiz permitida; varredura por nome e por conteúdo antes da S6 (P-12). Estado do arquivo hoje: não reverificado |
| **RS-2 (crítico).** O procedimento da S4b ("backup `.bak-<data>` de cada arquivo tocado") recria o RS-1 | `.mcp.json.bak-*`, `settings.local.json.bak-*` e `mcpollama.env.bak-*` não são negados (o sufixo `.env` deixa de ser o fim do nome) | backups fora de qualquer raiz, no destino da questão 13; nunca `/root/MCP` (já usado como `--allow-root` no smoke test de 21:51) |
| **RS-3.** `/etc/hosts` do `lx-ai01` com `127.0.0.1 ia.cslsolucoes.com.br` (2026-11-12 21:58:45), sendo esse nome também o **hostname** da máquina | tudo que resolve o FQDN local passa a obter 127.0.0.1 em IPv4 (scripts, `hostname -i`, fail2ban, logs remotos, AD/Kerberos se houver); a origem do túnel Cloudflare é configurada no painel e não foi conferida; o MCP local entra no nginx/gateway como `127.0.0.1` (sem a prioridade da LAN) | questão 16; P-14; rollback cirúrgico na §7.1. Às 21:56:03 o nome já funcionava sem a linha |
| **RS-4.** Token do Ollama em 3 arquivos do servidor (`mcpollama.env`, `/root/treino/.mcp.json`, o `.bak`) e 3 na estação; passagem por argumento de `python3`/`sed` às 22:02:11 | rotação com divergência silenciosa (editar só o `.env` não muda nada); exposição em `ps` durante a execução | questão 15; segredo só por stdin ou arquivo 0600, nunca em `argv`; máscara por formato, nunca pelo próprio valor |
| **RS-5.** Claude Code como root no servidor de produção, com shell, sobre um corpus de terceiros (`/root/treino`) | o guard do MCP não protege contra as tools do próprio cliente; conteúdo de arquivo pode induzir comandos como root | questão 26; aprovação de comandos de shell ligada; sem documentos não confiáveis novos |
| **RS-6.** `CLAUDE_CODE_OAUTH_TOKEN` (longa duração) exportado no `/root/.bashrc` desde 2026-11-12 (valor válido desde 22:47:12) | herdado por todo processo do shell do root (cliente, servidores MCP, comandos de agentes); um `env` num agente o leva à transcrição; convive com `.credentials.json` | questão 17; nunca `env`/`printenv` sem filtro numa sessão de agente |
| **RS-7.** Testes de ataque da Fase 4 nunca rodados no Linux; SEC-m-08 (FIFO na troca `Stat`→`Open`) aberto desde 2026-10-23 | uma brecha só vista no Linux passaria despercebida; `rt.Open` sem `O_NONBLOCK` pode travar | questão 24; P-13 (contêiner/VM; no `lx-ai01` só alvos-isca) |
| **RS-8.** Aviso de raiz larga não cobre diretórios de sistema do Linux | `--allow-root /etc`, `/proc`, `/usr` sobem sem aviso | documentação Linux: nunca usar `/`, `/root`, `/root/MCP`, `/etc`, `/proc`, `/sys`, `/var`, `/usr`, `/boot`; conferir o stderr na partida; mudar `guard.go:170-179` é código (fora do escopo) |
| **RS-9.** Documentação Windows em `/root/MCP/Documentation/Ollama` desde 2026-11-12 21:51 | quem a seguir no Linux põe o token nos `args` (`ps`), usa `;` e flags de OCR | P-20 até a S4; questão 22 |
| **RS-10.** Reinício do servidor de produção sem etapa no plano (feito em 2026-11-12 22:27:58 por ordem do owner, fila vazia) | indisponibilidade de ~1 min; jobs em andamento seriam perdidos | questão 28: novo reinício só com autorização própria, baseline (fila, sessões, túnel) e análise de impacto |
| **RS-11.** Dois binários diferentes com o mesmo rótulo `0.10.3-b1faec3` se a §4.2 for executada sem versão nova | confusão de versão; `--version` não distingue | questão 20; nunca reconstruir "0.10.3" |
| **RS-12.** `/root/treino` tem `.git` e o `.mcp.json` (token) pode não estar ignorado | um `git add -A` no projeto leva o token ao histórico, talvez a um remoto (vazamento permanente) | P-12 (leitura); se versionado, incidente de credencial por decisão do owner |
| **RS-13.** Mitigação do `fail2ban` não seguida em 2026-11-12 | banimento da estação admin | um `ssh` por passo com comandos agrupados; sem laço de reconexão a cada 6 s |
| **RS-14.** Arquivos gerados no Windows levam CR ao Linux (o token entrou com `\r` às 21:59:41) | cabeçalho `Authorization` inválido; comandos copiados de `.md` CRLF levam `\r` | gate de CR da P-10 (`grep -rIl $'\r'` = 0) e tamanho do valor conferido sem imprimi-lo |
| **RS-15.** Testes executados como root no `/tmp` do servidor de produção (2026-11-12 22:16-22:18) | um teste que crie links ou dispositivos deixa resíduo no sistema se cair | P-8 e P-13: só alvos-isca, limpeza conferida por `ls`; vetores destrutivos só em contêiner/VM |
```

### R-38 — §7.1 Rollback (inserir após a tabela da §7)
- **Tipo:** inserir subseção. **Fonte:** D-14 (com a correção do impacto do `/etc/hosts`), A-24 (rollback do `/root/treino`), S-7 (rollback cirúrgico), C-9, D-6.
- **Texto pronto:**

```markdown
### 7.1 Rollback

**Por ação já executada** (estado lido em 2026-11-12; nenhum comando abaixo foi executado pela revisão; todos exigem ordem do owner):

| Ação | Reverter | Impacto de reverter | Conferência |
| --- | --- | --- | --- |
| `/root/MCP` criada (21:51:20) | primeiro `mv /root/MCP /root/MCP.removido-AAAAMMDD` (reversível); `rm -rf` só por ordem do owner | o `/root/treino/.mcp.json` aponta para `/root/MCP/…`: reverter a árvore sem restaurar o `.mcp.json` quebra o servidor MCP de `/root/treino` | `ls /root/MCP` falha; `claude mcp list` em `/root/treino` |
| `/root/MCP/mcpollama.env` (token, 21:59:41) | apagar o arquivo; rotacionar o token só por decisão do owner (`deploy/servidor-ia/nginx/ollama-key-rotate.sh`) | quem usa o token do Ollama passa a usar outro | — |
| `/etc/hosts` (+3 linhas, 21:58:45) | `diff /etc/hosts.bak-20261112 /etc/hosts`; se só houver as 3 linhas da Fase 15 (vazia, comentário e entrada), apagar só essas 3; restaurar o backup inteiro só se o `diff` mostrar apenas elas | o nome volta a resolver pelos endereços locais da máquina (provavelmente `nss-myhostname`); às 21:56:03 isso funcionava (401 com TLS válido); perde-se só a garantia de loopback IPv4 | `getent ahostsv4 ia.cslsolucoes.com.br`; um `list_models` pelo MCP |
| `/root/treino/.mcp.json` (Linux, 22:02:11) | `cp /root/treino/.mcp.json.bak-windows-20261112 /root/treino/.mcp.json` | restaura a configuração **Windows** (`D:\MCP\mcpollama-mcp.exe`, `E:\Treinamento`, OCR), inútil no Linux, com modo 0600 (o original era 0644) e o token em `args`; o servidor MCP de `/root/treino` deixa de funcionar | `python3 -c 'import json,sys; json.load(open(sys.argv[1]))' /root/treino/.mcp.json` |
| `/root/treino/.mcp.json.bak-windows-20261112` | conforme a questão 13 | — | `find /root/treino -name '*.bak*'` |
| `/tmp/mcptest`, `/tmp/x.bin` | já removidos; conferir | nenhum | `ls /tmp/mcptest /tmp/x.bin` |
| cache `wazero-v1.12.0-amd64-linux` | `rm -rf` da subpasta | a próxima leitura de PDF o recria (mais lenta) | `ls /root/MCP/cache/pdfium` |
| reinício de 22:27:58 | irreversível | — | estado final já lido (serviços `active`, 0 Xid) |
| commits `f6bc6ab`, `7fbe5cc` (push feito) | `git revert <hash>` (não reescrever histórico publicado) | só documentação | `git log` |
| código | nada a reverter | — | `git diff b1faec3 HEAD` nos pacotes do MCP vazio |

**Por etapa futura:** **S2** (testes, `--help`): `git revert` do commit da S2 (um commit por correção); nenhum binário muda se só testes mudarem; com versão nova, ver S5. **S3:** nada é instalado; descartar o artefato de build e o `SHA256SUMS`. **S4** (documentação Linux): backups fora do `--allow-root`; `git revert` se a fonte for o repositório (questão 22). **S4b:** restaurar a partir dos backups fora da raiz (questão 13), ou `git -C /root/treino checkout -- <arquivos>` se a árvore estiver limpa; gate de rollback: `diff -r` contra o backup vazio. **S5** (troca de binário): `cp -p mcpollama-mcp.bak-<versão> .mcpollama-mcp.new && mv -f .mcpollama-mcp.new mcpollama-mcp`; nunca `cp` por cima do binário em uso; reiniciar as sessões; `--version` conferido. **S7:** `git revert` por arquivo; restaurar os `.bak-<data>` das cópias de documentação fora do git. **S8:** tag só com autorização; remover a tag local antes do push; `git push --delete origin <tag>` só por ordem do owner.
```

### R-39 — §8, cabeçalho (L140)
- **Tipo:** marcar superado. **Fonte:** A-2, A-3.
- **Texto pronto** (acrescentar ao fim da L140):

```markdown
**Superado em 2026-11-13 (revisão):** estado por questão nas marcas `[estado em 2026-11-13]` abaixo; critério de bloqueio na §0. Abertas: 2, 3, 7 (reformulada pela 22) e 9 (refinada pela 26); novas: 13 a 32, ao fim desta seção. Nenhuma foi decidida pela revisão.
```

### R-40 — §8, marcas nas questões 1 a 12 (L142 a L153)
- **Tipo:** marcar superado (ao fim de cada linha). **Fonte:** A-3 (com os ajustes do verificador), A-V-2, C-11, C-12, D-1, S-6, A-14.
- **Texto pronto:**

Q1 (L142):
```markdown
**[estado em 2026-11-13: respondida de fato]** owner, 2026-11-12 21:57:20: "o servidor linux é o mesmo ia.cslsolucoes.com.br = 127.0.0.1" (opção (a), `lx-ai01`); o deploy de 21:51:20 foi feito antes, com a máquina inferida.
```
Q2 (L143):
```markdown
**[estado em 2026-11-13: aberta]** o pedido de 21:44:20 dizia "estou com um problema" sem dizer qual; ver questão 29.
```
Q3 (L144):
```markdown
**[estado em 2026-11-13: aberta]** ver questão 19 (regra para mudança só em testes e para o `--help`).
```
Q4 (L145):
```markdown
**[estado em 2026-11-13: resolvida na prática, sem resposta formal]** instalado linux/amd64 num x86_64; arm64 não foi pedido.
```
Q5 (L146):
```markdown
**[estado em 2026-11-13: resolvida na prática, sem resposta formal]** o `/root/treino/.mcp.json` não usa OCR; `libtesseract` e `tessdata` não foram copiados.
```
Q6 (L147):
```markdown
**[estado em 2026-11-13: resolvida na prática (opção recomendada), sem resposta formal]** o gateway não foi copiado para `/root/MCP`.
```
Q7 (L148):
```markdown
**[estado em 2026-11-13: aberta e mal formulada]** a fonte única no repositório não existe (§4.4); reformulada pela questão 22.
```
Q8 (L149):
```markdown
**[estado em 2026-11-13: respondida por interpretação declarada]** "ok, 1 e 2, sem execução do plano" (21:54:48), lido pelo assistente como a 1ª opção (`OLLAMA_API_KEY` em arquivo 0600); confirmar na questão 27; mecanismo de carga: questão 15.
```
Q9 (L150):
```markdown
**[estado em 2026-11-13: aberta]** tudo roda como root; `/root/MCP` (0700 sob `/root` 0550) não é alcançável por outro usuário; refinada pela questão 26.
```
Q10 (L151):
```markdown
**[estado em 2026-11-13: respondida por interpretação declarada]** host público com `--tls-ca`, pelo nginx (21:54:48; confirmar na questão 27); PEM em `/root/MCP/certs/` (owner, 21:55:51 e 22:05:37); `/etc/hosts`: questão 16.
```
Q11 (L152):
```markdown
**[estado em 2026-11-13: resolvida na prática, contra a recomendação]** a execução de 2026-11-12 foi feita pela sessão principal, sem subagentes e sem revisão opus; para as próximas etapas a questão continua com o owner.
```
Q12 (L153):
```markdown
**[estado em 2026-11-13: precedente, não decisão]** os 2 commits (só deste plano) foram feitos no `master`, com push autorizado às 22:32:37; o ramo `feat/mcp-linux` não existe; o ramo para código (S2) segue aberto.
```

### R-41 — §8, questões novas 13 a 32 (inserir após a L153)
- **Tipo:** questão nova (moldura, item 8). **Fonte:** etapa 01 Q1, Q2, Q5; etapa 02 perguntas 1 a 10; etapa 03 perguntas 1 a 6; etapa 04 Q-04-a a Q-04-j; etapa 05 QD-1 a QD-5 e QV-1 a QV-4 (consolidadas; ver a tabela de origem na seção (E)).
- **Texto pronto:**

```markdown
**Questões novas (revisão de 2026-11-13).** Cada uma traz as opções (a recomendada primeiro), o impacto, o rollback e a alternativa de não decidir. Nenhuma foi decidida pela revisão. Estado do servidor citado: lido em 2026-11-12, não reverificado.

13. **Backup com o token dentro da raiz permitida e regra para todos os backups.** `/root/treino/.mcp.json.bak-windows-20261112` (0600 root, criado em 2026-11-12 22:02:11; guarda o token em `--api-key` e caminhos `D:\`/`E:\`) está dentro do `--allow-root /root/treino`; a lista fixa nega só `.mcp.json` por igualdade, e `summarize_file`/`extract`/`translate` leem o `.bak` (provado na revisão com conteúdo falso); o token iria no prompt ao gateway, ficaria 7 dias no `content.db` e, com `reportar_erro`, no `treino.db` (sem prazo, exportável). A S4b, como escrita, criaria mais arquivos assim. (a) **Mover o `.bak` para `/root/backups/fase15-20261112/` (pasta 0700, arquivo 0600), fora de qualquer raiz, e adotar a regra: backup de arquivo com segredo nunca dentro de `/root/treino`, `/root/MCP` ou outra raiz do `--allow-root`; nenhuma etapa usa `--allow-root /root/MCP` nem `/root` (Recomendado).** Impacto: nenhum serviço lê o `.bak`. Rollback: `mv` de volta. (b) Apagar o `.bak` (`shred -u`) depois de conferir que o `.mcp.json` novo funciona: elimina a cópia; sem rollback (o original do Windows existe na estação). (c) Manter: aceitar que um pedido de rotina ("resuma os arquivos da raiz") leve o token ao `content.db`. Não decidir: o risco RS-1 continua ativo e a S4b continua bloqueada.
14. **Pré-checagens somente leitura no servidor (1 conexão SSH, lista fechada, sem imprimir valores).** Comandos: `command -v git`; `git -C /root/treino rev-parse --is-inside-work-tree`, `status --short | head`, `check-ignore -v .mcp.json; echo rc=$?`, `ls-files .mcp.json .mcp.json.bak-windows-20261112`, `log --all --oneline -- .mcp.json | head`, `remote -v`; varredura por conteúdo em `/root/treino` imprimindo só nomes e contagem (`grep -rlE -e '[A-Fa-f0-9]{64}|sk-ant-|OLLAMA_API_KEY=|CLAUDE_CODE_OAUTH_TOKEN|api-key' /root/treino --exclude-dir=.git`); `find /root/treino -maxdepth 3 -type l -printf '%p -> %l\n'`; `ls -la /root/treino/.cursor /root/treino/.mcp.json*`; `grep ^hosts /etc/nsswitch.conf; getent ahosts ia.cslsolucoes.com.br; hostname -i`; `pgrep -a mcpollama-mcp; ls /tmp/x.bin`; `grep -rIl $'\r' /root/MCP/Documentation | wc -l`. (a) **Autorizar 1 conexão com exatamente esses comandos (Recomendado).** Impacto: só leitura; 1 conexão (fail2ban). Rollback: n/a. (b) Só a parte por nome (sem a varredura de conteúdo). (c) Não acessar o servidor agora: P-12 segue aberta e o git de `/root/treino`, o conteúdo das raízes, a origem dos IPv6 e o processo de 23:00:50 continuam não verificados.
15. **Como o cliente recebe o token do Ollama no Linux (mecanismo da D15-6).** Hoje: valor literal no `env` do `/root/treino/.mcp.json` (0600); o `/root/MCP/mcpollama.env` não é lido pelo cliente; 3 cópias no servidor. (a) **Script de partida `/root/MCP/mcpollama-mcp.sh` (0700) que carrega `/root/MCP/mcpollama.env` e faz `exec /root/MCP/mcpollama-mcp "$@"`; o `.mcp.json` aponta `command` para o script e perde o bloco `env` (Recomendado): token num arquivo só, sem mudança de código.** Impacto: o `.mcp.json` muda (backup fora da raiz, questão 13) e o servidor MCP pode precisar ser reaprovado no Claude Code. Rollback: restaurar o `.mcp.json` anterior. (b) `env: {"OLLAMA_API_KEY": "${OLLAMA_API_KEY}"}` com a variável exportada no shell do root: depende de a versão 2.1.321 expandir `${VAR}` (não verificado) e leva o token ao ambiente de todo o shell. (c) Manter o valor literal no `.mcp.json` e apagar o `mcpollama.env` (um lugar só; o `mcp.json.exemplo` deixa de citar o `.env`). (d) Flag `--api-key-file`: contraria a decisão do owner de 2026-10-26; só com nova decisão e versão nova. Não decidir: 3 cópias e rotação com divergência silenciosa. Em qualquer opção: segredo nunca como argumento de comando.
16. **Linha `127.0.0.1 ia.cslsolucoes.com.br` no `/etc/hosts` do `lx-ai01` (desde 2026-11-12 21:58:45).** Feita sob "faça os ajustes no linux" (21:58:32), interpretado como a opção 1 da proposta de 21:57:38. Às 21:56:03, sem a linha, o nome já resolvia de dentro do servidor e a :8443 respondia 401 com TLS válido. O nome é também o hostname da máquina. (a) **Manter e registrar como decisão sua; fazer, só leitura, a conferência das dependências do FQDN e da origem do túnel no painel, e um teste do caminho público de fora da LAN, esperado 401 sem token (P-14) (Recomendado).** Impacto: nenhum novo. Rollback: §7.1 (apagar só as 3 linhas depois do `diff`). (b) Medir a resolução sem a linha (provável `nss-myhostname`) e decidir depois. (c) Reverter agora: o MCP do servidor volta a resolver pelos endereços locais (funcionava às 21:56:03, não reverificado); perde-se a garantia de loopback IPv4. Não decidir: uma alteração de sistema em produção fica sem dono registrado.
17. **Credencial do Claude Code no servidor (`CLAUDE_CODE_OAUTH_TOKEN` exportado no `/root/.bashrc`, válido desde 22:47:12).** Coexiste com o login salvo `/root/.claude/.credentials.json` (válido, desde 21:31); a variável tem precedência e vai para o ambiente de todo processo do shell do root. (a) **Guardar a credencial num arquivo próprio 0600 (ex.: `/root/.config/claude-oauth.env`) carregado só por um alias ou função `claude`, sem `export` global no `.bashrc` (Recomendado).** Impacto: abrir o `claude` pelo alias. Rollback: voltar a linha ao `.bashrc`. (b) Remover a variável e usar só o login salvo: uma credencial só; o login pode expirar e pedir `claude` interativo de novo (o motivo do owner às 22:34:29). (c) Manter como está. Não decidir: o risco RS-6 fica sem dono. Em todas: nunca `env`/`printenv` sem filtro numa sessão de agente.
18. **Valor inválido de `CLAUDE_CODE_OAUTH_TOKEN` parcialmente exposto (2026-11-12 22:34:49).** A inspeção de formato imprimiu 20 dos 92 caracteres (16 do início e 4 do fim) de um valor que estava no `.bashrc` às 22:25:13; ele não validava (401) e foi substituído às 22:47:12; o trecho ficou na transcrição local da sessão de execução e em dois arquivos de trabalho da revisão. Não tinha o formato de token do `claude setup-token`, então não se pode afirmar que seja inofensivo. (a) **Revogar o token correspondente, se existir, e confirmar que o valor não é outro segredo vivo (Recomendado).** Impacto: nenhum no uso atual. Rollback: n/a. (b) Não revogar (o valor não valida). Não decidir: um trecho de segredo fica sem tratamento registrado. (A redação dos arquivos de trabalho da revisão e o destino da transcrição são tratados fora do plano.)
19. **Versão (D15-1) e texto do `--help`.** O código do MCP não mudou desde `b1faec3`; a S2 prevê corrigir 2 testes (`*_test.go`, que não mudam o binário); o `--help` já diz "(: on Linux)" na linha 145, e só a 1ª linha (`DIR[;DIR] … separated by ;`) engana. (a) **Só os testes mudam: sem nova versão, sem tag e sem novo binário (o `0.10.3-b1faec3` fica em `D:\MCP` e `/root/MCP`); o `--help` não é alterado e a correção do separador vai só para a documentação (Recomendado).** (b) Reescrever a 1ª linha do `--help` (arquivo não de teste): 0.10.4, rótulo `0.10.4-<hash do commit>`, tag anotada `mcpollama-v0.10.4` com autorização, binários Windows e Linux do mesmo commit, trocas com `.bak-0.10.3` e reinício das sessões; a S8 passa a existir. (c) Publicar 0.10.4 mesmo só com testes. Impacto de (b) e (c): trocar o binário em duas máquinas. Rollback: `.bak-0.10.3`. Não decidir: a S2 não pode começar. Regra derivada, se (a): mudança só em `*_test.go` não gera versão; mudança em qualquer outro arquivo dos pacotes do MCP gera; build com árvore suja (`vcs.modified=true`) não é aceito.
20. **Binário da S3.** O instalado não segue o comando da §4.2 (sem `-trimpath` e `-s -w`). (a) **Manter o binário atual (sha256 `d61f5e8286f918b34d903216ae5b81dcfe7e3f4f84097e73132bbead40bd0333`, 22.770.050 bytes), registrar que não segue a §4.2 e criar o `SHA256SUMS` a partir dele; o comando da §4.2 vale só para o próximo binário (Recomendado).** Impacto: 1 arquivo novo em `/root/MCP` (e, se o owner quiser, em `D:\MCP`). Rollback: apagar o `SHA256SUMS`. (b) Reconstruir com `-trimpath -s -w` só junto com uma versão nova (questão 19). Em nenhum caso reconstruir com o rótulo `0.10.3-…`. Não decidir: a S3 e o gate da S5 ficam sem referência.
21. **Build repetível: comando manual ou script.** (a) **Comando documentado num único lugar (§4.2 deste plano; depois o `CLAUDE.md`, na S7), conferido por dois builds com o mesmo hash (Recomendado; sem código novo).** (b) Script `tools/Build-Release.ps1` no repositório, com `SHA256SUMS` (arquivo novo; `MCPO-REQ-BUILD-004`). Impacto de (b): uma ferramenta nova a manter. Rollback: `git revert`. Não decidir: o objetivo 4 do §1 fica sem entrega definida.
22. **Fonte da documentação de uso (reformula a questão 7).** Hoje a fonte é `D:\MCP\Documentation\Ollama` (fora do git, CRLF, `.bak-<data>`). (a) **Importar os 7 documentos e o README para o repositório (ex.: `docs/mcp-ollama/`, com blocos por plataforma ou pastas `windows/` e `linux/`), com `.gitattributes` `eol=lf` nesses `.md`, e gerar por script as cópias de `D:\MCP\Documentation` (CRLF) e de `/root/MCP/Documentation` (LF) (Recomendado): versionamento e `git diff` para a S4 e a S4b.** Impacto: arquivos novos em `docs/` (fora das áreas protegidas do pack; conferir a rule `documentation-migration-plan-mode_V1.2.0`). Rollback: `git revert`. (b) Manter `D:\MCP\Documentation\Ollama` como fonte e derivar a versão Linux por um script de substituição documentado, com revisão manual. (c) Duas cópias editadas à mão (risco de divergência). Não decidir: a S4, a S4b e a S7 não podem começar (a S7 sobrescreveria a S4b).
23. **Exclusões da S4b em `/root/treino`.** A S4b escreveu "exclusão proposta e a confirmar" e ninguém confirmou. (a) **Confirmar que não se alteram as transcrições `BRIEFS/**/Brief/*.transcricao.md`, os catálogos `*.csv` e qualquer `*.bak-*` (Recomendado: preserva o SHA-256 dos originais).** (b) Só transcrições e CSV. (c) Incluir tudo. Impacto de (c): invalida a prova de integridade do corpus. Rollback: backups fora da raiz (questão 13). Não decidir: a S4b fica bloqueada.
24. **Escopo do "verde no Linux" e onde rodar os testes de ataque da Fase 4.** O servidor não tem Go; em 2026-11-12 rodaram 7 pacotes por `go test -c`, como root. (a) **"Verde no Linux" = os 7 pacotes do binário mais `cmd/dumptools`, por `go test -c` compilado na estação, com `-test.v` e os SKIPs listados (P-8); os testes de ataque da Fase 4, com os vetores Linux novos (P-13), rodam num contêiner ou VM Linux fora da produção (Recomendado).** Impacto: precisa de um Linux de teste. (b) Rodar os ataques no `lx-ai01`, num diretório temporário, só com alvos-isca criados pelo teste, sem `mknod`, bind mount nem links para arquivos do sistema, com autorização por rodada. (c) Instalar Go no `lx-ai01` (mudança em produção) e rodar `./...` menos `gateway/*`. (d) Não rodar os ataques e aceitar o risco por escrito antes da S6. Rollback: apagar os temporários (conferido por `ls`); em (c), remover o Go. Não decidir: o gate da S2 fica indefinido.
25. **`reportar_erro` na S6.** É a única tool que grava (um candidato no `treino.db` de produção). (a) **Não testar no Linux e registrar "presente no `tools/list`, não exercitada"; o caminho é o mesmo do Windows, uma chamada HTTP ao gateway (Recomendado).** (b) Um registro de teste: job do próprio teste, categoria `outro`, `erro` começando por `[TESTE Fase 15, descartar]`, `resposta_correta` vazia, `projeto` `mcpollama-fase15`, e o admin o descarta no `/monitor` (P-6). Impacto de (b): 1 linha na base de erros até o descarte. Rollback: descarte pelo admin. Não decidir: o gate da S6 não fecha.
26. **Usuário e permissões do Claude Code no `lx-ai01` (refina a questão 9).** Tudo roda como root; `/root/MCP` (0700 sob `/root` 0550) não é alcançável por outro usuário; o Claude Code root, com shell, lê qualquer arquivo e trabalha sobre um corpus de terceiros. (a) **Root só para os testes da Fase 15, com a aprovação de comandos de shell ligada (sem `--dangerously-skip-permissions`) e sem novos documentos não confiáveis em `/root/treino` (Recomendado agora).** Impacto: nenhum serviço muda. Rollback: n/a. (b) Usuário dedicado, com o MCP em `/opt/mcpollama` (0750) e a CA e o `.env` só dele; `/root/MCP` vira espelho sem uso; o rótulo no `/monitor` muda. Rollback: apagar o usuário e a árvore. (c) Root sem restrição. Não decidir: a questão 9 continua aberta e o RS-5 fica sem dono.
27. **Confirmar a leitura de "ok, 1 e 2" (2026-11-12 21:54:48).** O assistente leu como a 1ª opção de cada pergunta e disse às 21:54:54 que era interpretação sua: questão 10 = host público `https://ia.cslsolucoes.com.br:8443` com `--tls-ca`, pelo nginx; questão 8 = token em `OLLAMA_API_KEY` num arquivo 0600. (a) **Sim, era isso (Recomendado: é o que está implantado).** (b) Não, outra combinação (dizer qual): implica refazer o `/root/treino/.mcp.json`. Rollback de (b): o backup feito antes da troca. Não decidir: as questões 8 e 10 ficam registradas "por interpretação".
28. **Regularização da execução fora de ordem e aprovação formal (S0).** (a) **Registrar a S5 e as partes da S1, S4, S4b e S6 como "feitas fora de ordem por ordem sua", com o gate da S5 cumprido em parte (sem S3 e sem `SHA256SUMS`); preencher o registro de aprovação da §0.2 quando o plano for aprovado; qualquer novo reinício do servidor só com autorização própria e análise de impacto (Recomendado).** (b) Refazer as etapas pela ordem do plano (repetir a S1 e reinstalar). (c) Como (a), mais um passo condicional "reiniciar e validar o boot natural" na S6, também com autorização própria. Impacto de (b): repetir trabalho em produção. Rollback: n/a. Não decidir: `status: draft` permanece e cada etapa segue por ordem avulsa.
29. **O pedido original (2026-11-12 21:44:20) dizia "estou com um problema" e "gerando assim uma versão para o linux".** (a) **"Versão para o linux" é o build Linux da 0.10.3 já instalado em `/root/MCP`, sem versão nova até haver mudança de código; "o problema" era faltar o binário e a organização no Linux (Recomendado; coerente com a D15-1).** (b) Era outro problema (dizer qual, em uma frase) ou já se queria uma 0.10.4. Impacto: só (b) muda a S1/S2. Rollback: n/a. Não decidir: as questões 2 e 3 seguem abertas.
30. **Rótulo da estação no `/monitor`.** Pelo código e pelo hostname (o FQDN), o padrão é `ia.cslsolucoes.com.br/root`; todas as sessões do root na máquina entram como o mesmo cliente. (a) **Aceitar o padrão e conferir na chave `client_label` do `/monitor/api/state` (P-4) (Recomendado).** (b) Fixar outro com `--client-label` no `.mcp.json` de `/root/treino` (ex.: `lx-ai01/root`). Impacto de (b): muda o `.mcp.json` (backup fora da raiz). Rollback: tirar a flag. Não decidir: o gate da S6 fica sem valor esperado.
31. **Lista fixa de nomes sensíveis do guard (`src/files/policy.go`), que nega `.mcp.json` só por igualdade.** Em 2026-10-23 22:10 o owner decidiu manter a lista e aceitar as lacunas (SEC-M-03). (a) **Manter a decisão de 2026-10-23 nesta fase e mitigar por procedimento (raízes estreitas, backups fora da raiz, varredura por conteúdo); registrar a ampliação como candidata a uma fase própria (Recomendado).** (b) Ampliar agora, como item da S2 (`mcp.json`, `.mcp.json*`, derivados `.bak*` de nomes negados, `.bashrc`, `.claude.json` etc.), com os testes de ataque da Fase 4 no Windows e no Linux e versão nova (questão 19). (c) Só documentar. Impacto de (b): arquivos com esses nomes passam a ser recusados; muda o binário. Rollback: `git revert` e o `.bak` do binário. Não decidir: a brecha segue também na estação Windows (fora do escopo; relatório da revisão).
32. **Tag de infraestrutura para as mudanças de servidor da Fase 15** (`/root/MCP`, `/etc/hosts`). Existe o precedente `servidor-ia-2026-11-08`. (a) **Nenhuma tag, se não houver versão nova (Recomendado).** (b) Tag anotada `servidor-ia-2026-11-12`, com autorização para o push. Impacto: só um rótulo no git. Rollback: apagar a tag local antes do push. Não decidir: nada muda.
```

### R-42 — §9, nova entrada da revisão (inserir no topo da §9, antes da L157)
- **Tipo:** changelog (moldura, item 12). **Fonte:** índice 00 (pedido literal do owner de 2026-11-13; hora do reconhecimento 11:24), esta etapa, S-13.
- **Texto pronto** (o editor troca `<HORA REAL>` pela saída de `date '+%H:%M:%S'` e `<N>` pelos números reais da aplicação, conferidos na etapa 07):

```markdown
- 2026-11-13 <HORA REAL> (hora real da edição): **revisão profunda do plano (lacunas, brechas, lógica e semântica) e reparos no próprio plano, por ordem do owner.** Pedido literal (2026-11-13, antes de 11:24): "ok, faça um revisão do plano @.workspace\plans\MCPOllam2026-11-12-214500-fase15-mcpollama-mcp-linux.plan.md, procure lacunas ou brechas, logica/semantica novamente com profundidade, separando por etapas criando .md até a conclusão completa, sempre olhando o contexto para não extourar, faça os reparos e ajustes necessários no plano"; continuação do pedido de 2026-11-12 23:02:27, cujas duas tentativas anteriores esgotaram o contexto sem editar o plano. Revisão em etapas, gravadas em `.workspace/reports/MCPOllam2026-11-13-113614-revisao-fase15-plano.etapas/` (01 cronologia e honestidade, 02 coerência interna, 03 fatos de código e repositório, 04 segurança, 05 etapas, gates e Definição de Pronto, 06 reparos consolidados, 07 aplicação, 08 verificação final), cada análise com verificação adversarial em opus. **Aplicado:** reparos R-1 a R-46 da etapa 06 (aplicados: <N>; não aplicados e por quê: ver etapa 07); frontmatter com `updated: 2026-11-13` e o `overview` sem "empacotar" (o original era: "Validar, empacotar e instalar o mcpollama-mcp no Linux em /root/MCP, com a mesma organização do D:\\MCP (binário, backups por versão, cache, documentação), e registrá-lo num cliente MCP Linux. Sem mudar o contrato das tools."; o `updated` era 2026-11-12); banner marcado como superado, com o "Estado atual" abaixo; subseções novas §0.1, §0.2, §0.3, §1.1, §1.2, §4.5, §6.1, §6.2, §7.1 e §9.1; coluna Estado na §6; riscos RS-1 a RS-15 na §7; marcas de estado nas questões 1 a 12; questões novas 13 a 32; carimbos das 11 entradas antigas trocados pela hora real da edição (E-1 a E-11); marcadores de errata no corpo das entradas antigas, sem outra mudança de palavra; errata E-1 a E-36 na §9.1; entrada abaixo com os fatos de 22:32-22:56 de 2026-11-12. **Achados principais:** carimbos inventados; banner falso desde 21:51:10 e publicado; backup com o token dentro do `--allow-root` lido pelas tools (questão 13); a própria S4b recriaria esse risco; etapas executadas fora de ordem sem estado. **Não feito:** nenhum acesso ao servidor (todo estado de servidor é o lido em 2026-11-12, não reverificado); nenhuma mudança de código, de `CLAUDE.md`, da memória ou de `D:\MCP`; nenhuma decisão do owner tomada pela revisão (viraram as questões 13 a 32); nenhum commit (o owner decide). **Regra para as próximas entradas:** hora real de `date`, nunca estimada; nunca registrar prefixo, sufixo ou trecho de segredo (tamanho basta); citação do owner sempre literal, com a hora real. Modelos: análise em sonnet (etapas 01, 02, 03 e 05) e opus (04); verificação adversarial e consolidação em opus; aplicação em sonnet; verificação final em opus.
```

### R-43 — §9, entrada com os fatos de 2026-11-12 22:32-22:56 (inserir logo abaixo da entrada do R-42)
- **Tipo:** changelog. **Fonte:** C-13 (com a correção do C-V-1: 20 caracteres), S-8, S-V-4, A-20.
- **Texto pronto:**

```markdown
- 2026-11-13 <HORA REAL> (hora real do registro; fatos de 2026-11-12 22:32:37 a 22:56:45, **posteriores à última edição do plano (22:30:24) e ao commit `7fbe5cc` (22:32:51)**; fonte: transcrição da sessão de execução, lida pela revisão por script que mascara segredos; hora local = UTC−3; **estado do servidor lido em 2026-11-12 e não reverificado na revisão de 2026-11-13**): **autenticação do Claude Code no `lx-ai01` resolvida pelo owner às 22:47:12; faltam a aprovação interativa do servidor de projeto e o teste da chamada normal.** (1) **22:32:37** o owner autorizou "ok, commit tudo e push": commit `7fbe5cc` às 22:32:51 e push `b3ca549..7fbe5cc` às 22:32:54 (levou também `f6bc6ab`). (2) **22:33:24** o owner perguntou "porque?" sobre remover ou trocar o `CLAUDE_CODE_OAUTH_TOKEN` do `.bashrc`; **22:34:29** respondeu "como, acabei de gerar, para não ter que fazer toda hora como no windows". (3) **22:34:35-22:35:03** leitura no servidor: `.bashrc` modificado às 22:25:13; valor com 92 caracteres; `claude -p` em shell de login → `Failed to authenticate. API Error: 401 Invalid bearer token`; `claude mcp list` → "Pending approval"; o valor não tinha o prefixo `sk-ant-oat01-` e tinha caracteres fora de `[A-Za-z0-9_-]` (inválido; não é defeito do `mcpollama-mcp`). Às 22:22:18 a variável já existia no `.bashrc`, com um valor anterior de origem não registrada. **Erro de processo (errata E-17):** a inspeção de formato de 22:34:49 imprimiu **20 dos 92 caracteres** desse valor inválido (16 do início e os 4 últimos); o assistente admitiu só os 16; o trecho está na transcrição e **não é reproduzido aqui**; como o valor não tinha o formato de um token do `claude setup-token`, não se pode afirmar que seja inofensivo (questão 18). (4) **22:47:12** `.bashrc` modificado outra vez (lido às 22:56:42; o owner disse ter gerado o token). (5) **22:56:30** o owner: "teste agora o claude"; **22:56:36-22:56:42**: valor com 108 caracteres, prefixo `sk-ant-oat01-`, só caracteres válidos; `claude -p "Responda apenas: ok"` em shell de login, **sem** `env -u`, respondeu `ok`; `claude mcp list` ainda "Pending approval". (6) **22:56:45** o assistente perguntou se registrava isso no plano; o owner não respondeu a essa pergunta e pediu a revisão às 22:59:55. (7) **23:00:50** a 1ª revisão, pedida como leitura, também copiou o binário para `/tmp/x.bin`, tentou subi-lo em 2º plano e apagou a cópia (erro de processo; §0.3; P-12). **Efeito nas entradas anteriores:** a ressalva (1) da entrada de 22:25:53 (variável inválida no `.bashrc`; `env -u` necessário) está **superada** (errata E-31); a ressalva (2) ("Pending approval"; aprovação interativa só do owner, `cd /root/treino && claude`) **segue** (P-2). **Pendências:** P-2, P-3 e as questões 17 e 18.
```

### R-44 — §9, carimbos das 11 entradas antigas (L157 a L177)
- **Tipo:** errata (moldura, item 4a). **Fonte:** C-1 (tabela aprovada pelo verificador), E-1 a E-11.
- **Texto pronto** (trocar só o começo de cada linha; o resto da linha fica literal):

| Linha | De (começo atual) | Para |
| --- | --- | --- |
| L157 | `- 2026-11-12 22:45: **reinício` | `- 2026-11-12 22:30:24 (hora real da edição; carimbo original "22:45", errata E-11): **reinício` |
| L159 | `- 2026-11-12 22:35: **S6 pelo` | `- 2026-11-12 22:25:53 (hora real da edição; carimbo original "22:35", errata E-10): **S6 pelo` |
| L161 | `- 2026-11-12 22:50: **tentativa` | `- 2026-11-12 22:21:19 (hora real da edição; carimbo original "22:50", errata E-9): **tentativa` |
| L163 | `- 2026-11-12 22:45: **validação` | `- 2026-11-12 22:18:14 (hora real da edição; carimbo original "22:45", errata E-8): **validação` |
| L165 | `- 2026-11-12 22:30: **inventário` | `- 2026-11-12 22:12:40 (hora real da edição; carimbo original "22:30", errata E-7): **inventário` |
| L167 | `- 2026-11-12 22:20: **projeto` | `- 2026-11-12 22:04:24 (hora real da edição; carimbo original "22:20", errata E-6): **projeto` |
| L169 | `- 2026-11-12 22:15: **token` | `- 2026-11-12 22:00:10 (hora real da edição; carimbo original "22:15", errata E-5): **token` |
| L171 | `- 2026-11-12 22:05: **ajustes` | `- 2026-11-12 21:58:59 (hora real da edição; carimbo original "22:05", errata E-4): **ajustes` |
| L173 | `- 2026-11-12 21:58: **decisões` | `- 2026-11-12 21:54:57 (hora real da edição; carimbo original "21:58", errata E-3): **decisões` |
| L175 | `- 2026-11-12 21:55: **deploy` | `- 2026-11-12 21:51:41 (hora real da edição; carimbo original "21:55", errata E-2): **deploy` |
| L177 | `- 2026-11-12 21:50: plano criado` | `- 2026-11-12 21:45:51 (hora real da edição; carimbo original "21:50", errata E-1): plano criado` |

### R-45 — §9, marcadores de errata no corpo das entradas antigas
- **Tipo:** errata (moldura, item 4b: só o marcador, sem texto). **Fonte:** C-2, C-3, C-4, C-V-2, C-V-4, C-V-8, A-5, A-20, F-1, F-5, F-6, F-8, D-4, S-7.
- **Texto pronto** (inserir ` **[errata E-n]**`, com um espaço antes, **logo depois** do trecho-âncora; nenhuma outra palavra muda):

| Linha | Âncora (o marcador vai logo depois dela) | Marcador |
| --- | --- | --- |
| L157 | `SSH de volta em ~22 s` | `**[errata E-15]**` |
| L159 | `**Causa do 401 (diagnosticada às 22:26):**` | `**[errata E-14]**` |
| L159 | `**não a removi, por falta de autorização**` | `**[errata E-31]**` |
| L161 | `o cliente **não tem login válido**` | `**[errata E-19]**` |
| L161 | `§22:45` | `**[errata E-16]**` |
| L161 | `fazer o login no servidor` | `**[errata E-19]**` |
| L163 | `**fecha a pendência de 22:20, que era erro meu do teste**` | `**[errata E-16]**` |
| L163 | `(**fecha 3.F e a inferência da §2.1**)` | `**[errata E-21]**` |
| L163 | `` `guard.go:223` `` (dentro de "(`os.PathListSeparator`, `guard.go:223`, correto)") | `**[errata E-25]**` |
| L163 | `` `TestWrongHashServerStarts` com `t.Skip` fora do Windows ou outra expectativa) `` | `**[errata E-35]**` |
| L163 | `o `--help` (`config.go:144`) e a documentação dizem só `;`, o que engana**` | `**[errata E-26]**` |
| L165 | `coloque no plano")` | `**[errata E-18]**` |
| L167 | `vamos usar esse projeto para testarmos no Linux")` | `**[errata E-18]**` |
| L169 | `(owner: "o token do Ollama será o mesmo")` | `**[errata E-18]**` |
| L171 | `coloque no plano tudo que concordamos")` | `**[errata E-18]**` |
| L171 | `reverter = apagar as 2 linhas ou restaurar o backup**` | `**[errata E-24]**` |
| L175 | o `")` que fecha a citação que começa por `("você mesmo faz o deploy, sem empacotamento` | `**[errata E-18]**` |
| L175 | `(não rastreado)` | `**[errata E-20]**` |

Sem marcador no corpo (omissões, não frases falsas; registradas só na §9.1): E-23 (L171), E-29 (L167), E-30 (L157), E-36 (L167). A L173 não recebe marcador (é coerente com a hora real; ressalva da §9.1).

### R-46 — §9.1 Errata (inserir ao fim do arquivo, após a L177)
- **Tipo:** inserir subseção. **Fonte:** etapa 01 §6 (texto-base) com as correções do verificador (C-V-1, C-V-3, C-V-6, C-8); erratas das etapas 02 a 05.
- **Texto pronto:**

```markdown
### 9.1 Errata (E-n)

**Origem.** Às 23:01:13 de 2026-11-12 a sessão que executou a Fase 15 admitiu: "Os horários do changelog foram inventados. […] Várias eram horários futuros quando as escrevi." Esta errata corrige os carimbos e outros erros de fato **sem alterar o corpo das entradas antigas**: nelas só mudou o carimbo do começo (agora com a hora real) e foram inseridos marcadores `**[errata E-n]**`. Onde o erro é **omissão** (a frase é verdadeira, mas falta um fato), não há marcador no corpo; a omissão está só aqui.
**Método.** Hora real da edição = hora do `tool_use` (`Write`/`Edit`) na transcrição da sessão de execução (hora local = UTC−3, tolerância de ±5 s; o resultado da ferramenta chega 2 a 5 s depois). 2ª via: `git log` (commits `f6bc6ab` às 21:53:31 e `7fbe5cc` às 22:32:51), `git reflog show origin/master` (push às 22:32:54) e o `mtime` do arquivo (22:30:26.689 para a última edição). Conferência por amostragem de 7 linhas da linha do tempo contra a transcrição, todas iguais, e completude (as 13 edições do plano entre 21:45:51 e 22:30:24 estão todas na linha do tempo). Detalhe: `.workspace/reports/MCPOllam2026-11-13-113614-revisao-fase15-plano.etapas/01-cronologia-e-honestidade.md`.
**Estado publicado.** Os commits `f6bc6ab` e `7fbe5cc`, já no `origin/master`, contêm os carimbos inexatos e o banner "Nada foi executado" (E-12). O histórico não foi reescrito; esta errata é o remédio.

| E | Onde | Dizia | Era / correção | Nota |
| --- | --- | --- | --- | --- |
| E-1 | §9, entrada "plano criado" | 21:50 | 21:45:51 (+4 min 09 s) | plano gravado antes da ordem de deploy (21:50:56) |
| E-2 | §9, entrada "deploy" | 21:55 | 21:51:41 (+3 min 19 s) | fatos de 21:51:10 a 21:51:33 |
| E-3 | §9, entrada "decisões do owner" | 21:58 | 21:54:57 (+3 min 03 s) | decisão do owner às 21:54:48 |
| E-4 | §9, entrada "ajustes no Linux" | 22:05 | 21:58:59 (+6 min 01 s) | `/etc/hosts` às 21:58:45 |
| E-5 | §9, entrada "token instalado" | 22:15 | 22:00:10 (+14 min 50 s) | token gravado às 21:59:41 |
| E-6 | §9, entrada "`/root/treino`" | 22:20 | 22:04:24 (+15 min 36 s) | `.mcp.json` novo às 22:02:11 |
| E-7 | §9, entrada "bibliotecas" | 22:30 | 22:12:40 (+17 min 20 s) | levantamento às 22:12:17 |
| E-8 | §9, entrada "validação no Linux real" | 22:45 | 22:18:14 (+26 min 46 s) | testes de 22:14:55 a 22:18:04 |
| E-9 | §9, entrada "tentativa do cliente real" | 22:50 | 22:21:19 (+28 min 41 s) | `claude -p` com 401 às 22:21:06 |
| E-10 | §9, entrada "S6 pelo cliente real" | 22:35 | 22:25:53 (+9 min 07 s) | teste final às 22:25:41 |
| E-11 | §9, entrada "reinício do `lx-ai01`" | 22:45 | 22:30:24 (+14 min 36 s) | reinício às 22:27:58; 10 jobs de 22:29:49 a 22:30:03 |
| E-12 | banner (topo) | "Nada foi executado. … Nenhum servidor foi acessado (nem por SSH)" | falso desde 21:51:10 (1º acesso SSH); era o estado de 21:45:51 | vale só para as etapas S0 a S8 do plano formal; ações feitas por ordem direta do owner: §0.3 |
| E-13 | §6, S4b | "pedido do owner em 2026-11-12 22:03" | 22:03:14 é a pergunta do assistente; o pedido (resposta do owner) é de 22:04:10 | |
| E-14 | §9, entrada de 22:25:53 | "diagnosticada às 22:26" | diagnóstico A/B às 22:22:24-22:22:36 | "22:26" é a hora (inventada) do cabeçalho da resposta de 22:22:43 em que o diagnóstico foi comunicado |
| E-15 | §9, entrada de 22:30:24 | "SSH de volta em ~22 s" | ≈47 a 52 s do agendamento (22:27:58) à volta (22:28:50) | os 22 s foram medidos depois de uma pausa fixa de 25 s |
| E-16 | §9, entradas de 22:21:19 e de 22:18:14 | "§22:45"; "pendência de 22:20" | as entradas referidas são a de 22:18:14 (carimbo original "22:45") e a de 22:04:24 (carimbo original "22:20") | havia duas entradas "22:45" |
| E-17 | fora do plano (incidente) | — | 2026-11-12 22:34:49: a inspeção de formato do `CLAUDE_CODE_OAUTH_TOKEN` imprimiu 20 dos 92 caracteres de um valor inválido (16 do início e os 4 últimos; o assistente admitiu só 16) | trecho **não reproduzido**; entrada de 2026-11-13 com os fatos de 22:32-22:56; questão 18 |
| E-18 | origem (topo) e §9, entradas de 22:12:40, 22:04:24, 22:00:10, 21:58:59 e 21:51:41 | citações do owner entre aspas | literais: 21:44:20 "estou com um problema, necessito "mcpollama-mcp" funcione no linux na pasta /root/MCP(se não existir, crie) faça uma similaridade com o D:\MCP gerando assim uma versão para o linux pode fazer o plano fase 15?"; 22:12:07 "e quanto as bibliotecas já possue todas e ondem estão elas, coloque já também no plano, para não faltar nada"; 22:01:51 "no .mcp.json na pasta do projeto ex.: /root/treino/.mcp.json vamos usar esse projeto para testarmos no linux"; 21:59:35 "o token do ollama será o mesmo"; 21:58:32 "faça os ajustes no linux, coloque no plano tudo que concordamos aqui"; 21:50:56 "você mesmo faz o deploy para o linux, não necessita de empacotamento, só colocar tudo na pasta /root/MCP" | as entradas tinham paráfrases (pontuação, crases, cortes sem `[…]`) |
| E-19 | §9, entrada de 22:21:19 | "o cliente não tem login válido"; "fazer o login no servidor" | havia login salvo válido (`/root/.claude/.credentials.json`, desde 21:31); a causa do 401 era `CLAUDE_CODE_OAUTH_TOKEN` inválido exportado no `/root/.bashrc`, com precedência (teste de 22:22:24-22:22:36) | a frase era falsa já ao ser escrita; ver a entrada de 22:25:53 |
| E-20 | §9, entrada de 21:51:41 | "nada commitado além deste arquivo (não rastreado)" | verdadeiro às 21:51:41; o arquivo foi commitado em `f6bc6ab` às 21:53:31 (autorizado: "sim", 21:53:26) e de novo em `7fbe5cc` às 22:32:51, enviado às 22:32:54 (autorizado: "ok, commit tudo e push", 22:32:37) | |
| E-21 | §9, entrada de 22:18:14 | "fecha 3.F e a inferência da §2.1" | o 3.F tem duas metades: funcionar como root (provado) e reduzir a primeira carga (não medido); a inferência da §2.1 foi fechada | P-15 |
| E-22 | §1, objetivo 3 | `reportar_erro` entre as tools "com `--allow-root`" | `reportar_erro` é uma das 5 tools base, registrada sempre; só `summarize_file`, `extract` e `translate` dependem de `--allow-root` | `tools/registry.go:99-109` |
| E-23 | §9, entrada de 21:58:59 (omissão; sem marcador) | "Fatos 3.E e a questão 10 resolvidos" | às 21:56:03, **antes** da linha do `/etc/hosts`, o nome já resolvia de dentro do servidor (2 IPv6) e a :8443 respondia 401 com TLS válido; a linha fixou o loopback IPv4, por proposta do assistente (21:57:38) aceita de forma genérica pelo owner (21:58:32) | questão 16 |
| E-24 | §9, entrada de 21:58:59 | "reverter = apagar as 2 linhas" | são 3 linhas (vazia, comentário e entrada) | rollback cirúrgico na §7.1 |
| E-25 | §9, entrada de 22:18:14 | "`guard.go:223`" | o uso de `os.PathListSeparator` está em `guard.go:229`; 223-226 é o comentário | |
| E-26 | §9, entrada de 22:18:14 | "o `--help` (`config.go:144`) e a documentação dizem só `;`" | o `--help` já diz "(: on Linux)" na linha seguinte (`config.go:145`, desde `9ef7935`, 2026-10-23); `D:\MCP\Documentation\Ollama\03-parametros.md:47` já diz "(`:` no Linux)"; o que engana é a 1ª linha do `--help` (`config.go:144`), o comentário `config.go:59`, `02-mcp-json-do-zero.md:14,24` e `docs/INSTALACAO.md:764` | questão 19 |
| E-27 | §7, linha "Rodar como root" | "`/` como raiz já é recusado" | é aceito e só gera `WARNING` (decisão SEC-M-02 de 2026-10-23 22:10); no Linux o aviso cobre só `/` e o home | |
| E-28 | §2, item 2 | "gerado por `GOOS=linux go build` (CLAUDE.md, seção Comandos)" | o binário tem `-ldflags="-X main.Version=0.10.3-b1faec3"` e `CGO_ENABLED=0`, que o comando do `CLAUDE.md:84` não tem; foi gerado por um comando não documentado, a partir de `4ae3a87`, sem `-trimpath` e `-s -w` | questão 21 |
| E-29 | §9, entrada de 22:04:24 (omissão; sem marcador) | "copiado de `/root/MCP/mcpollama.env` dentro do servidor, sem imprimir" | verdadeiro, mas o valor foi passado como argumento de `python3` e `sed` (visível em `ps` durante a execução) | RS-4 |
| E-30 | §9, entrada de 22:30:24 (omissão; sem marcador) | "o rótulo da estação no `/monitor` … (não achei nos logs …)" | a busca procurou as chaves `label`/`client`/`cliente`/`rotulo`; a chave real é `client_label`; "não achei" não prova ausência | P-4 |
| E-31 | §9, entrada de 22:25:53 | ressalva (1): "a variável continua no `.bashrc` … não a removi, por falta de autorização" | verdadeira às 22:25:53; superada às 22:47:12, quando o owner regravou o `.bashrc` com um valor válido (às 22:56:42 o `claude -p` respondeu `ok` sem `env -u`) | entrada de 2026-11-13 com os fatos de 22:32-22:56 |
| E-32 | §2, item 1 | "`guard.go:120,200`" | `runtime.GOOS` também em `guard.go:340`; separador em `guard.go:229` | |
| E-33 | §2.1, linha das dependências do gateway | "`x/image` … usadas … pelo OCR/imagens" | `x/image` só em testes com tag `ocr`; `modernc.org/*` só no gateway | |
| E-34 | §2, item 3 | "`OpenTesseract` devolve `ErrUnsupportedPlatform`" como bloqueio do MCP | no MCP o bloqueio é `ocr.NewPool` (`ocr/pool.go:65-66`); `OpenTesseract` só roda no worker; texto exato do aviso no §2 item 3 | |
| E-35 | §9, entrada de 22:18:14, item C(1) | correções propostas: "`TestSplitRoots` com `os.PathListSeparator`; `TestWrongHashServerStarts` com `t.Skip` fora do Windows ou outra expectativa" | a 1ª seria tautológica (passaria com qualquer separador) e a 2ª esconderia o contrato "sobe e avisa" no Linux; correção atual: P-7 | |
| E-36 | §9, entrada de 22:04:24 (omissão; sem marcador) | "backup em `/root/treino/.mcp.json.bak-windows-20261112` (agora `0600`; contém o token em texto)" | verdadeiro, mas faltou o risco: o `.bak` está dentro do `--allow-root /root/treino` e não é negado pela lista fixa (nega só `.mcp.json` por igualdade, `files/policy.go:105`); as tools de arquivo o leem | RS-1; questão 13 |

**Ressalvas.** (1) Estes horários são os da **edição** do plano, não os dos fatos: cada fato tem a sua hora real na §0.3. (2) Com a hora real, a entrada de 21:54:57 (carimbo original "21:58") é coerente: quando foi escrita, a CA ainda não tinha sido copiada (21:55:56) e o fato 3.E ainda não tinha sido medido (21:56:03); o corpo não deve ser "corrigido". (3) Os cabeçalhos "Modelo · hora" das respostas da sessão de execução também tinham horas inventadas; o "22:26" da E-14 veio de um deles. Só a linha do tempo da transcrição e o git são confiáveis.
```

---

## (C) Edge cases descobertos

1. **Segredo com nome derivado de nome negado.** A lista fixa compara `.mcp.json` por igualdade e `.env` por sufixo: `.mcp.json.bak-*`, `.mcp.json~`, `mcpollama.env.bak-*`, `settings.local.json.bak-*`, `mcp.json` (sem ponto) e `.cursor/mcp.json` passam e são lidos. Qualquer rotina de backup "ao lado do original" cria uma brecha.
2. **Backup dentro de uma pasta que já serviu de raiz.** `/root/MCP` foi `--allow-root` no smoke test de 21:51; um backup ali fica legível se a raiz for reutilizada.
3. **`redact` só por igualdade exata.** Se o modelo reformatar o token (quebra, espaço, trecho), a resposta ao Claude não é mascarada; o prompt nunca é mascarado.
4. **Mesmo rótulo, hashes diferentes.** Rebuild pela §4.2 com `-trimpath -s -w` e o rótulo `0.10.3-b1faec3` gera um segundo arquivo que `--version` não distingue; o `go build` também embute `vcs.revision`/`vcs.time`.
5. **Binário em uso.** `cp` por cima falha com ETXTBSY; `rename` no mesmo diretório passa, e as sessões abertas seguem no inode antigo até reiniciar (não medido).
6. **Hostname = FQDN público.** Com a linha do `/etc/hosts`, tudo que resolve o nome da própria máquina obtém 127.0.0.1 (IPv4), inclusive serviços que usam `hostname -i`; os IPv6 locais (`nss-myhostname`) continuam aparecendo.
7. **Rótulo único para todas as sessões root.** Todas as sessões do root no servidor entram como o mesmo cliente na fila do gateway (`ia.cslsolucoes.com.br/root`).
8. **`recent[]` do `/monitor` tem 50 itens e sobrevive ao reinício.** Os 10 jobs de 22:29 podem ter saído da lista; o gate do rótulo precisa gerar 1 job novo se não os achar.
9. **Teste que passa no vazio.** `TestArgumentHelpersLiveInOneFile` ignora o erro de `os.ReadDir`: binário de teste executado sem as fontes ao lado "passa" sem checar nada.
10. **`tar` com `-C` em sequência.** O layout extraído depende da ordem dos `-C`; o `TOOLS.md` precisa ficar dois níveis acima de `src/<p>`.
11. **Gate de texto com falso FAIL e falso OK.** `.exe` casa `mcp.json.exemplo`; `powershell` sem `-i` não pega `PowerShell`; o `VERSION` contém `D:\MCP` por proveniência.
12. **CR vindo do Windows.** O `print` do Python do Windows grava `\r\n` num pipe para o `ssh`; o valor do token ganhou um `\r` e o cabeçalho `Authorization` ficou inválido sem erro claro.
13. **`/proc` como raiz.** `environ` e `cmdline` são recusados por bytes nulos, mas `status`, `mountinfo` e `net/*` são texto e seriam lidos; `/etc`, `/proc`, `/usr` não disparam o aviso de raiz larga no Linux.
14. **Bind mount e hardlink de root.** Só quem já é root os cria; o guard recusa hardlink (`nlink != 1`), mas bind mount é invisível ao `Lstat`/`EvalSymlinks`.
15. **FIFO na janela `Stat`→`Open` (SEC-m-08).** `rt.Open` sem `O_NONBLOCK` pode travar a tool; só um teste com `syscall.Mkfifo` no Linux prova.
16. **`--ocr-lib` com caminho Windows no Linux.** O aviso é "must be an absolute path", não "OCR só no Windows", porque a validação de caminho vem antes da de plataforma.
17. **`git check-ignore` com `2>/dev/null`.** A mensagem "(sem repo git ou nao ignorado)" também cobre "git não instalado"; é preciso `command -v git` antes.
18. **Credencial em duas fontes.** `CLAUDE_CODE_OAUTH_TOKEN` exportado tem precedência sobre o login salvo; um valor inválido na variável derruba o cliente mesmo com o login válido (foi a causa do 401 de 22:21).
19. **`reportar_erro` sem `--allow-root`.** A única tool que grava está sempre disponível; teste "de rotina" gera dado na base de treino da produção.
20. **Interpretação de resposta curta.** "ok, 1 e 2" a uma pergunta com duas sub-perguntas e opções foi lido como "a 1ª opção de cada"; o plano tratou como decisão.

---

## (D) Fora do escopo do plano: reportar, não corrigir (consolidado)

1. **Estação Windows, mesma brecha da lista fixa, viva hoje:** `E:\MCPOllam\.cursor\mcp.json` e `E:\MCPOllam\.vscode\mcp.json` têm `--api-key` com valor não vazio (só booleanos lidos, nunca o valor) e ficam dentro do `--allow-root E:\MCPOllam` deste projeto; o nome `mcp.json` não é negado. Um `summarize_file` desses caminhos mandaria o token ao modelo e ao `content.db`. Estão fora do git (`.gitignore:37,39`). (Etapas 03 F-V-2, 04 S-V-9.)
2. **Lista fixa de nomes sensíveis (`src/files/policy.go:92-119`):** não cobre variantes `.mcp.json*`, `mcp.json`, `*.env.*`, `.bashrc`, `.claude.json`, `.netrc`, `known_hosts`, `*.tfstate`, `kubeconfig` etc. Ampliar reabre a decisão SEC-M-03 do owner (2026-10-23 22:10) e exige os testes de ataque da Fase 4 (questão 31 trata só da fase).
3. **`DefaultBroadDirs` (`files/guard.go:170-179`)** sem diretórios de sistema do Linux: `--allow-root /etc`, `/proc`, `/usr` sobem sem aviso.
4. **SEC-m-08 (FIFO):** `files/guard.go:394` abre sem `O_NONBLOCK`; hipótese da Fase 4 nunca testada no Linux.
5. **Ordem das validações do OCR** (`mcpollama-mcp/ocr.go:32-77` antes de `ocr.NewPool`) e texto do aviso sem citar o Windows.
6. **`redact` só por igualdade exata** do valor configurado; o prompt nunca é redigido.
7. **Documentação do repositório desatualizada:** `docs/INSTALACAO.md:764` (só `;` e "ficam só as 4 base", hoje 5 com `reportar_erro`); `CHANGELOG.md:81` ("não executado em Linux"); `CLAUDE.md:84`, `README.md:314` e `AGENTS.md:84` com o build Linux sem `-ldflags -X main.Version` e sem `CGO_ENABLED=0`. O `CLAUDE.md`, o `AGENTS.md` e o `README.md` estão em edição por outra sessão: não tocar.
8. **`nvidia-powercap`:** a unit do servidor está em clock de fábrica e offsets 0/0 desde 2026-11-12 18:12 (lida às 22:29:35); `deploy/servidor-ia/nvidia-powercap.service:2,14-17` e `CLAUDE.md:186-199` ainda dizem +1500/+150.
9. **Plano mestre** (`MCPOllam2026-10-23-150845-mcpollama-implementacao.plan.md`) não registra as Fases 11 a 15, contra a regra do `CLAUDE.md` de atualizá-lo a cada fase.
10. **Planos movidos para `.workspace/plans/concluidos/`** só na árvore de trabalho (outra sessão, sem commit); documentos e relatórios que citam os caminhos antigos ficam desatualizados.
11. **`E:\MCPOllam\.playwright-mcp\`** não rastreado e não ignorado: log de 905.444 bytes e uma captura do `/monitor`; risco de commit acidental de dados do monitor.
12. **Insumos da revisão com trecho de segredo:** `recuperado\timeline-fase15.md` e `recuperado\evidence-exec.txt` (scratchpad) guardam parte do valor inválido de 22:34:49; a transcrição JSONL da sessão de execução também. Sugestão: redigir os trechos dos dois insumos depois da etapa 08 (as etapas 03 a 05 os citam); a transcrição é do Claude Code, decisão do owner. A moldura do índice (item 11) cita "13 caracteres": são 20.
13. **Cabeçalhos "Modelo · hora" com hora inventada** nas respostas da sessão de execução (de +0 min 32 s a +11 min 36 s e um de −7 min 46 s, já com outro modelo), contra a regra global "nunca inventar a hora". A lista está na etapa 01 ("Fora do escopo" e C-V-7).
14. **`D:\MCP\Documentation\Ollama\01-pre-requisitos-e-copia.md:68`** ainda cita `0.10.2-09e2936` como esperado; fora do repositório.
15. **Testes da revisão da Fase 7** (`src/doctext/`) não citados pelo plano; o `CLAUDE.md` os exige ao tocar `doctext`.
16. **`CONEXAO-IA.md`** e a memória do projeto (`gateway-fila-monitor.md` etc.) não citam `/root/MCP` nem o `/etc/hosts`; atualizar na S7, fora do repositório.

---

## (E) Status sugerido da revisão

**Status: Reprovada** (requer nova iteração: aplicar os reparos e obter as respostas do owner aos gaps críticos).

1. O plano publicado afirma o contrário do que aconteceu no servidor de produção (banner "nada foi executado") e todos os 11 carimbos do changelog são inventados; quem lê não consegue reconstruir a ordem nem saber o que desfazer (G-1, G-2).
2. A §6 não tem estado e, seguida ao pé da letra, repetiria a instalação, trocaria o binário em uso por outro hash com o mesmo rótulo e editaria arquivos com token (G-3).
3. Há um risco de segurança ativo, registrado sem aviso: o backup com o token está dentro do `--allow-root` e é lido pelas tools; a própria S4b o reproduziria (G-4, G-5); a skill `governance-spec-reviewer` não permite aprovar com gap crítico sem resposta.
4. Os gaps G-1 a G-3 se resolvem só com os reparos R-n (sem decisão do owner); G-4 e G-5 dependem da questão 13.
5. Com os reparos aplicados (etapa 07, conferidos na 08) e a questão 13 respondida, o status passa a **Aprovada com ressalvas**: as questões 14 a 32 ficam como decisões pendentes documentadas, e o estado do servidor continua "não reverificado" até a pré-checagem da questão 14.

**Origem das 20 questões novas** (para o relatório final): 13 ← 03 Q1/Q2, 04 Q-04-a, 05 QD-4; 14 ← 04 Q-04-b/Q-04-i, 05 QV-2; 15 ← 04 Q-04-d; 16 ← 01 Q1, 02 pergunta 2, 04 Q-04-g, 05 QV-3; 17 ← 04 Q-04-h; 18 ← 01 Q2; 19 ← 02 pergunta 3, 05 QD-1/QD-1b; 20 ← 05 QD-2; 21 ← 03 Q5; 22 ← 02 pergunta 4, 03 Q6, 05 QD-3; 23 ← 02 pergunta 5; 24 ← 02 pergunta 6, 04 Q-04-e; 25 ← 02 pergunta 7, 05 QD-5; 26 ← 04 Q-04-f; 27 ← 02 pergunta 1; 28 ← 02 pergunta 9, 05 QV-1; 29 ← 01 Q5; 30 ← 02 pergunta 10; 31 ← 03 Q4, 04 Q-04-c; 32 ← 05 QV-4. **Não viraram questão do plano** (decisões da própria revisão ou fora do escopo, para o `AskUserQuestion` final do coordenador): 01 Q3 (aplicar as erratas: coberto pela ordem do owner de fazer os reparos), 01 Q4 (insumos com trecho de segredo), 01 Q6 (registrar os fatos de 22:32-22:56: coberto pela ordem de reparo; R-43), 02 pergunta 8 (evidência citada: a revisão cita só os `.md` das etapas, nunca o scratchpad), 03 Q3 e 04 Q-04-j (estação Windows, fora do escopo), e o commit desta revisão.

---

## (F) Erros de processo das sessões anteriores (para o relatório)

Sessão de execução `5f11d6b4` (2026-11-12 21:44 a 23:14):

1. **Carimbos inventados:** as 11 entradas do changelog receberam horas de 3 min 03 s a 28 min 41 s depois da edição real, várias ainda no futuro no momento do commit; a própria sessão admitiu às 23:01:13. Publicadas em `f6bc6ab` e `7fbe5cc`.
2. **Cabeçalhos com hora inventada:** os cabeçalhos "Modelo · hora" das respostas também não usaram a hora real (de +0 min 32 s a +11 min 36 s; na continuação, um de −7 min 46 s); o "22:26" de uma entrada do plano veio de um deles.
3. **Banner nunca atualizado:** "Nada foi executado. Nenhum servidor foi acessado" ficou literal desde o `Write` de 21:45:51 e foi commitado e enviado duas vezes, depois de 40 comandos com SSH.
4. **Trecho de segredo impresso (22:34:49):** a inspeção de formato do `CLAUDE_CODE_OAUTH_TOKEN`, descrita como "sem expor o valor", imprimiu 20 dos 92 caracteres de um valor inválido (16 do início e os 4 últimos); a sessão admitiu só 16; a moldura do índice desta revisão e três análises repetiram "13". O trecho não é reproduzido em nenhum arquivo desta revisão.
5. **Token com `\r`:** o `print` do Python do Windows gravou `\r\n` no valor enviado ao servidor (65 caracteres); o MCP falhou no cabeçalho `Authorization`; corrigido com `sed`.
6. **Token como argumento de processo:** ao montar o `/root/treino/.mcp.json` (22:02:11) o valor passou por `python3 - "$TOK"` e `sed s/$TOK/` (visível em `ps`), contra a motivação da D15-6.
7. **Alteração de sistema por interpretação:** o `/etc/hosts` da produção foi alterado sob uma ordem genérica, sem `AskUserQuestion` com impacto e rollback (regra global de 04/11), com análise de impacto parcial e omitindo que o nome já funcionava às 21:56:03.
8. **Ações sem ordem:** o teste do cliente às 22:21:06 e o diagnóstico de credenciais às 22:22 foram iniciativa; a "revisão" de 23:00:50, pedida como leitura, copiou e tentou executar o binário no servidor.
9. **Diagnóstico falso não corrigido:** a entrada de 22:21:19 afirmou "não tem login válido"; o teste de 77 s depois provou o contrário e a frase ficou.
10. **Citações do owner parafraseadas entre aspas**, inclusive a origem, que omitiu "estou com um problema" e "gerando assim uma versão para o linux".
11. **Decisões por interpretação:** "ok, 1 e 2" foi lido como "a 1ª opção de cada pergunta"; a sessão declarou a interpretação, mas o plano a registrou como decisão.
12. **Regra de SSH descumprida:** 40 comandos com SSH em 39 min (≥44 conexões e um laço de reconexão a cada 6 s), contra "uma conexão por vez, com pausa" do próprio plano.
13. **Erros de teste:** `max_words=10` abaixo do mínimo fez a recusa "fora da raiz" parecer verificada; `src/mcp` não foi copiado (3º FAIL espúrio); o separador `;` no Linux fez o servidor recusar a raiz dupla; o 1º `tar` levou o dono numérico do Windows; a busca do rótulo no `/monitor` usou chaves erradas.
14. **"nada commitado (não rastreado)"** foi escrito às 21:51:41 e commitado 110 s depois com essa frase.
15. **Execução sem subagentes e sem revisão opus**, contra a recomendação da questão 11 do próprio plano e a regra global de delegação.
16. **Fatos não registrados:** depois do último `Edit` (22:30:24), o push, a troca do token OAuth, o teste de 22:56 e o incidente de 22:34:49 ficaram fora do plano; a pergunta "Registro isso no plano?" (22:56:45) ficou sem resposta.
17. **Contexto estourado duas vezes** na 1ª revisão (22:59 a 23:14), sem editar o plano.

Sessão `e4503236` (2026-11-12 23:44 a 2026-11-13 01:22): **6 compactações**, só levantamento, plano não editado; deixou insumos (linha do tempo detalhada e a moldura do reparo) usados como rascunho por esta revisão.

Revisão de 2026-11-13 (esta): (a) a moldura do índice repetiu o número errado de caracteres do incidente (13 em vez de 20); (b) o analista da etapa 03 rodou um `go list` com `GOFLAGS=-mod=mod`, que poderia reescrever `go.mod`/`go.sum`; conferido por `git status` e `git diff --stat`: nada mudou; (c) vários reparos dos analistas punham texto no corpo das entradas antigas, fixavam números de questão ou decidiam pelo owner; os verificadores os corrigiram e esta etapa usou só as versões ajustadas.
