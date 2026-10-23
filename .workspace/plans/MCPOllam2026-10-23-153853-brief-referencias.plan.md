---
title: "Brief das referências do mcpollama (referencias/Brief_Referencias)"
name: "brief-referencias"
overview: "Síntese detalhada, rastreável e sem segredos das referências locais, web e de infraestrutura, no molde de E:\\GestorERP\\.workspace\\referencias, para levantar requisitos e decisões de construção do mcpollama."
created: 2026-10-23
updated: 2026-10-23
status: approved
locale: pt-BR
---

# Brief das referências do mcpollama

## Resumo

Produzir em `referencias/Brief_Referencias/` uma síntese **detalhada e rastreável** de todas as referências do projeto.
- **Locais:** `ollama-mcp-go`, `04-what-is-mcp`, `MCPNas/truenas-mcp` e `MCPVMWare`.
- **Web:** os 7 links do README.
- **Infraestrutura:** o servidor Ollama da CSL e o nível `local` atual.

O método é o mesmo de `E:\GestorERP\.workspace\referencias`: uma pasta por referência com documentos numerados, uma matriz de cobertura contra o plano de implementação, um comparativo transversal e uma revisão adversarial. O objetivo é **levantar o que importa para construir o mcpollama**:
- padrões já provados;
- armadilhas já pagas (cerebrum/buglog);
- contratos das tools;
- limites reais do servidor;
- GAPs.

Resultado: requisitos com ID que alimentam as Fases 1 a 6 de `MCPOllam2026-10-23-150845-mcpollama-implementacao.plan.md`.

## Objetivos

1. Uma pasta por referência local, com fichas numeradas e afirmações citadas como `caminho:linha`.
2. Fichas das 7 referências web, com tools, transporte, configuração e ideias aproveitáveis. Nada inventado: o que não carregar fica marcado.
3. Brief do servidor Ollama (endpoints, auth, limites), com **validação empírica** (latência, tokens/s, contexto, 401/403) medida ao vivo.
4. Lições aprendidas extraídas de `.wolf/cerebrum.md`, `.wolf/buglog.json` e `.workspace/` do MCPVMWare e do MCPNas.
5. `60-Requisitos` (IDs `MCPO-REQ-<AREA>-nnn`) e `61-Matriz_Cobertura` (requisito × fase do plano de implementação).
6. `Comparativo-Referencias-mcpollama.md`: tabela comparativa, o que tirar de cada referência e GAPs.
7. `70-Revisao`: revisão adversarial com veredito. Gate: **0 segredos**, 0 afirmações sem fonte.

## Âmbito e exclusões

**Entra:**
- leitura (só leitura) de `referencias/**`;
- leitura das páginas web e dos repositórios GitHub citados;
- `CONEXAO-IA.md` e `ia-local.ps1`, cabeçalho e fluxo, sem segredos;
- chamadas **só de leitura** ao Ollama: `/api/tags`, `/api/ps`, um `generate` curto e a checagem de 401/403.

**Fica fora:**
- código em `src/` (é o plano de implementação);
- qualquer escrita, pull ou delete no servidor;
- SSH em loop (fail2ban);
- os 6 GB de `MCPVMWare/referencia/`, `Develop/` e `dist/`, além dos 134 arquivos `generated_*`. Estes últimos só entram como "como são gerados", via `src/gen`;
- `.remember/` e `.wolf/hooks|token-ledger`, por serem ruído de sessão.

**Nunca lidos por agentes Anthropic** (lista de exclusão, vai literal no prompt de todo subagente):
- `**/.mcp.json`, `**/.claude/settings.local.json`, `**/*.env`, `**/*.pem`, `**/*.key`;
- `MCPVMWare/.workspace/DADOS_CONEXAO_TESTE.md` e `MCPVMWare/.workspace/config_teste.json`;
- `MCPVMWare/.workspace/*.postman_collection.json` (5 coleções);
- `MCPVMWare/.workspace/reports/*configuracao-servidores-teste*` e `MCPNas/truenas-mcp/.workspace/reports/*servidor-teste-config*`.

Os nomes acima foram levantados por nome de arquivo e por termos (`password`, `senha`, `api_key`, `Bearer`), **sem abrir o conteúdo**. Se algum deles tiver lição útil, quem lê é o **nível local (Qwen)**, conforme a regra global de dados sensíveis, e só a lição (sem valores) volta para o brief. Os `docs/` do truenas-mcp com `credential` no nome são documentação da API, não credenciais, e podem ser lidos.

## Dimensionamento (medido em 2026-10-23)

Convenção de contagem: **linhas físicas, como `wc -l`** (inclui linhas em branco). O `Measure-Object -Line` do PowerShell ignora as brancas e dá números menores (1.242 / 38 / 99.991 / 77.363); não é divergência.

| Referência | Arquivos Go | Linhas de Go | Núcleo a analisar | Documentos de valor |
| --- | ---: | ---: | --- | --- |
| `ollama-mcp-go` | 4 | 1.426 | tudo | `README.md` |
| `04-what-is-mcp` | 1 | 47 | tudo | `README.md`, `.devcontainer` |
| `MCPNas/truenas-mcp` | 116 | 109.126 | `cmd/`, `mcp/`, `proxy/`, `tasks/`, `tools/` (amostrado por família) | `.wolf/cerebrum.md`, `.wolf/buglog.json`, `.workspace/plans` (1), `.workspace/reports` (6+), `docs/` |
| `MCPVMWare/src` | 176 | 85.397 | **23** arquivos não-gerados e não-teste + `gen/` | `CLAUDE.md`, `README.md`, `MCP-GAPS-VMWARE.md`, `TOOLS.md` (1 MB), `.wolf/*`, `.workspace/*`, `tools/Build-Release.ps1`, `scripts/gen_tools_md.py` |
| Web | — | — | 7 links + repositórios GitHub citados (KTCrisis, ollama-tlms-golang, NightTrek) | — |
| Infra CSL | — | — | `CONEXAO-IA.md`, `ia-local.ps1`, servidor ao vivo | — |

## Estrutura de saída (molde GestorERP adaptado a MCP/Go)

```
referencias/Brief_Referencias/
  README.md                          índice, fontes, dimensionamento, estado por onda, changelog
  00-CONVENCOES.md                   idioma, citação, segredos, IDs, domínios
  Comparativo-Referencias-mcpollama.md   tabela comparativa + o que tirar de cada + GAPs
  60-Requisitos_mcpollama.md         requisitos consolidados MCPO-REQ-<AREA>-nnn
  61-Matriz_Cobertura.md             requisito × fase do plano de implementação × referência-fonte
  70-Revisao.md                      revisão adversarial (veredito, críticos/médios/menores)
  <Referencia>/                      ollama-mcp-go/, what-is-mcp/, truenas-mcp/, mcpvmware/
    README.md                        o que é, fontes, dimensionamento, índice
    10-Arquitetura.md                entrypoint, loop stdio, dispatch, camadas, fluxo de uma chamada
    20-Tools_e_Contratos.md          catálogo de tools: nome, inputSchema, saída, erros, destrutiva?
    30-Configuracao_e_Seguranca.md   flags→env, TLS/CA/insecure, auth, gate destrutivo, segredos, audit
    40-Codigo_Arquivo_a_Arquivo.md   ficha por arquivo do núcleo (papel, tipos, funções, caminho:linha)
    50-Testes_Build_Release.md       padrão de testes, mocks, build, cross-compile, release, versão
    80-Licoes_Aprendidas.md          (CSL) cerebrum + buglog + reports: armadilha → causa → solução
  Web/
    W1-dxeo-ollama-mcp.md ... W7-ollama-tlms-golang.md
  Servidor-Ollama-CSL/
    README.md                        endpoints, auth, limites, nível local atual (sem segredos)
    dados/validacao-empirica.md      medições ao vivo (latência, tokens/s, ctx, 401/403, fila)
```

Documentos por referência (o critério de conclusão 1 se mede contra esta tabela):

| Referência | README | 10 | 20 | 30 | 40 | 50 | 80 | Total |
| --- | :-: | :-: | :-: | :-: | :-: | :-: | :-: | ---: |
| `ollama-mcp-go/` | x | x | x | x | x | x | — | 6 |
| `what-is-mcp/` | x | x | x | — | — | — | — | 3 |
| `truenas-mcp/` | x | x | x | x | x | x | x | 7 |
| `mcpvmware/` | x | x | x | x | x | x | x | 7 |

Mais os 6 da raiz, os 7 da `Web/` e os 2 do `Servidor-Ollama-CSL/`: **38 documentos**.

Áreas de requisito (`<AREA>`): `PROTO` (MCP/JSON-RPC), `CFG` (flags/env), `SEC` (segredos/TLS/gate),
`OLL` (cliente Ollama), `TOOL` (catálogo/contratos), `FILE` (leitura local), `TOK` (economia/contadores/verbosity),
`OBS` (log/trace/debug), `TEST`, `BUILD` (build/release/versão), `INTEG` (Claude Code/Cursor/VS Code), `PERF` (timeouts/fila/chunking).

## Inventário de arquivos / artefatos

| Caminho | Operação | Nota |
| --- | --- | --- |
| `.gitignore` | editar | `referencias/*` + `!referencias/Brief_Referencias/`. **Feito e verificado** em 2026-10-23 |
| `referencias/Brief_Referencias/**` | criar | conforme a estrutura e a tabela de documentos acima (38 documentos) |
| `.workspace/scripts/scan-segredos-brief.ps1` | criar | varredura de segredos (fora do brief; não contém valores, só os carrega em memória). Modos `-BriefDir` (controle positivo) e `-Diagnose` (forma do valor, sem revelá-lo) |
| `.workspace/scripts/scan-segredos-allowlist.txt` | criar | falsos positivos triados, por SHA-256 (nunca o valor), com origem, motivo e data |
| `CLAUDE.md`, `README.md` | editar | ponteiro para o brief; ajustes que vierem dos achados (onda B7) |
| `.workspace/plans/MCPOllam2026-10-23-150845-mcpollama-implementacao.plan.md` | editar | vincular `MCPO-REQ-*` às fases (onda B7) |
| este plano | editar | changelog **por onda** |

## Passos / fases (ondas, com nível de modelo)

### B0 — Convenções e esqueleto (loop principal)
1. Criar `00-CONVENCOES.md`:
   - pt-BR com acentos; código, paths e identificadores entre crases, fiéis ao original;
   - citação `caminho:linha`;
   - **0 segredos**: valores → `[omitido]`, só o nome do campo;
   - IDs de requisito `MCPO-REQ-<AREA>-nnn` e IDs de lição `LIC-<REF>-nnn`.
2. Criar o `README.md` do brief com a tabela de dimensionamento acima e o estado por onda.
3. Gate: os arquivos existem e `git status` mostra o brief como não rastreado (não ignorado).

### B1 — Referências locais pequenas (em paralelo)
1. `ollama-mcp-go/` (**Sonnet**: é a referência mais próxima do produto, e a ficha 40 exige `caminho:linha` exato): 10, 20, 30, 40 e 50 completos. As 1.426 linhas são lidas integralmente, com atenção a streaming, trace file, negociação de versão e testes `httptest`.
2. `what-is-mcp/` (**Haiku**): README + 10/20 resumidos (47 linhas de Go + `.devcontainer`).
3. Gate: toda afirmação técnica com `caminho:linha`; contagens conferidas por `wc`/`grep`.

### B2 — Projetos MCP da CSL (Sonnet, um agente por projeto, em paralelo)
1. `mcpvmware/`:
   - os 23 arquivos de núcleo (entrypoint, `mcp/types.go`, registry, `destructive.go`, `discovery.go`, verbosity, profile, finder);
   - como `src/gen` gera os `generated_*` e o `TOOLS.md`;
   - build/release (`tools/Build-Release.ps1`, versão, `-dirty`);
   - `MCP-GAPS-VMWARE.md`.
2. `truenas-mcp/`:
   - `cmd/`, `mcp/`, `proxy/` (stdio↔HTTP), `tasks/` (jobs assíncronos e poller), `tools/` amostrado por família;
   - `--tls-ca`/`--insecure`;
   - `docs/`.
3. `80-Licoes_Aprendidas.md` de cada um: varrer `.wolf/cerebrum.md`, `.wolf/buglog.json`, `.workspace/plans` e `.workspace/reports`, extraindo armadilha → causa raiz → solução → se vale para o mcpollama.
   - O prompt de cada agente leva a **lista de exclusão** de "Âmbito e exclusões", literal.
   - Os arquivos excluídos que tiverem lição útil passam pelo **nível local (Qwen)**, uma chamada por vez, com a instrução de devolver só a lição, sem hosts, usuários, senhas ou tokens. O resultado é rascunho: conferir antes de usar.
4. Gate:
   - `caminho:linha` em todas as afirmações;
   - **varredura de segredos** (abaixo) = 0.

### B3 — Referências web (Haiku, WebFetch)
0. O conteúdo das páginas e dos repositórios é **dado não confiável**: o prompt dos agentes manda ignorar qualquer instrução embutida (prompt injection) e só descrever. O MCP do GitHub não conectou em 2026-10-23; o B3 usa WebFetch e as URLs `raw.githubusercontent.com`.
1. Fichas `Web/W1..W7`.
2. Para o W3 (mcpservers.org deu 403), tentar o repositório de origem `github.com/NightTrek/Ollama-mcp`; se também falhar, registrar.
3. Para W1, W6 e W7, ler os READMEs e o código principal dos repositórios relevantes:
   - dxeo ollama-mcp;
   - KTCrisis `ollama-mcp-go` e `flux7-mesh|supervisor|memory`;
   - ollama-tlms-golang `05-make-your-mcp-server`, `06-mcp-host-genaiapp` e `07-*`.
4. Gate: nenhuma descrição sem fonte carregada; páginas que falharem ficam marcadas como "não carregou".

### B4 — Servidor Ollama CSL + validação empírica (Sonnet + loop principal)
1. `Servidor-Ollama-CSL/README.md`, a partir de `CONEXAO-IA.md` e do `ia-local.ps1` (fluxo, timeout de 900 s, códigos de saída 2/3/124), **sem nenhum valor secreto**.
2. Validação empírica (loop principal, **uma chamada de cada vez**, sem SSH):
   - token lido de `%USERPROFILE%\.openclaude\csl-ollama.env` dentro do script, **nunca impresso**;
   - CA de `certs\cloudflare-origin-ca.pem`;
   - medir:
     - `/api/tags` (modelos);
     - `/api/ps` (residente, contexto, GPU);
     - `generate` curto com `stream:false`, **3 repetições**: latência total, `eval_count`/`eval_duration` → tokens/s de geração, `prompt_eval`, e `load_duration` separado (frio na 1ª, quente nas seguintes);
     - `generate` com prompt grande (~20–28k tokens, texto sintético sem dados reais): tokens/s de **prompt eval**, que é o custo dominante das tools de arquivo;
     - `generate` com prompt **acima de 32k**: comparar o `prompt_eval_count` com o tamanho enviado para provar se o servidor **trunca em silêncio** (comportamento conhecido do Ollama quando passa de `num_ctx`, ainda não confirmado na v0.40.8). Resposta à P-B1 e insumo do chunking da Fase 4;
     - sem token → 401;
     - `/api/pull` → 403;
     - LAN `:8443` contra a internet sem porta. Registrar o **IP resolvido** de cada host e a presença do cabeçalho `cf-ray`: de dentro da LAN, o host sem porta pode resolver para o IP interno (split DNS) e não passar pelo Cloudflare.
3. Gravar `dados/validacao-empirica.md` com os números medidos e o comando usado, com o token como `[omitido]`.
4. Gate: os números vêm da saída **lida**, não estimados; varredura de segredos = 0.

### B5 — Consolidação (Sonnet)
1. `60-Requisitos_mcpollama.md`: requisitos por área, cada um com a referência-fonte e a prioridade (MUST/SHOULD/COULD).
2. `61-Matriz_Cobertura.md`: requisito × fase do plano de implementação × referência. As fases são 0–6; a Fase 0 (ambiente e repositório) já está concluída e entra só como referência. Inclui os GAPs, ou seja, o que nenhuma referência resolve e o mcpollama precisa inventar (ex.: tools de leitura local com allowlist, chunking em 32k).
3. `Comparativo-Referencias-mcpollama.md`:
   - tabela comparativa (transporte, SDK ou tipos à mão, configuração, segurança, testes, release, economia de tokens);
   - o que tirar de cada referência;
   - a conclusão em uma linha.

### B6 — Revisão adversarial (Opus)
1. `70-Revisao.md`: amostrar afirmações e conferi-las contra o código e as páginas (`caminho:linha` verbatim), recontar números, procurar requisito sem fonte e "coberto" otimista.
2. Rodar a varredura de segredos e a checagem de links.
3. Veredito (FIÁVEL / COM RESSALVAS / NÃO FIÁVEL), com achados críticos, médios e menores.
4. Aplicar as correções e registrar no changelog do README.

### B7 — Fecho (loop principal)
1. Ligar os `MCPO-REQ-*` às fases do plano de implementação.
2. Atualizar `CLAUDE.md` e `README.md` com o que o brief mudar, por exemplo:
   - confirmar ou corrigir flags, timeouts e o modelo default;
   - responder à **P-2** (modelo de 32k ou 64k) com os números da validação empírica;
   - levar ao owner, via AskUserQuestion e com os achados do brief como insumo, as decisões **P-3** (config do Cursor), **P-4** (nível `local` global) e **P-5** (o que entra no 1º commit). Estas não se resolvem com dados. A P-1 já foi decidida (não) em 2026-10-23 15:50.
3. Commit **só com ordem do owner**, com stage por caminho.

### Varredura de segredos (gate de toda onda)

Script `.workspace/scripts/scan-segredos-brief.ps1` (fora do brief). Ele lê as fontes **localmente**, sem passar valor nenhum por agente Anthropic:
- carrega em memória os valores sensíveis conhecidos:
  - `args` e `env` de todo `.mcp.json` em `referencias/`;
  - os valores de `MCPVMWare/.claude/settings.local.json`, `MCPVMWare/.workspace/config_teste.json` e `DADOS_CONEXAO_TESTE.md`, das 5 coleções Postman e dos dois reports de servidor de teste (a lista de exclusão de "Âmbito e exclusões");
  - o token do `csl-ollama.env`;
- procura cada valor em `referencias/Brief_Referencias/**`.

A saída tem **dois contadores** e mostra só o arquivo:linha, nunca o valor:
1. **Valores conhecidos:** a meta é **0**, e isso é gate duro (a onda não fecha com 1).
2. **Padrões genéricos** (`Bearer [A-Za-z0-9]`, `api[-_]key`, `password`, `senha`, `BEGIN .*PRIVATE KEY`): vão para triagem. A linha com `[omitido]` é aceita, porque a convenção manda manter o nome do campo. Qualquer outra ocorrência é revisada à mão e corrigida ou justificada no changelog.

## Dependências e riscos

- **Segredos nas fontes** (`MCPVMWare/.mcp.json` com senhas de ESXi e TrueNAS; `settings.local.json`, `config_teste.json`, `DADOS_CONEXAO_TESTE.md`, coleções Postman e reports de servidor de teste; `.remember/`): o maior risco do brief. São dois riscos distintos:
  - **gravar no brief**, mitigado pela varredura em toda onda;
  - **expor ao contexto Anthropic** ao ler, mesmo sem gravar, mitigado pela lista de exclusão no prompt dos agentes e pela leitura pelo nível local.
- **Volume:** 109 mil linhas no truenas-mcp e 85 mil no MCPVMWare. Mitigação: analisar só o núcleo e amostrar por família; os gerados entram só como mecanismo.
- **Web:** páginas podem falhar (403) ou mudar. Fica registrada a data da coleta e a falha, sem inventar.
- **Validação empírica:**
  - fila única de GPU;
  - fail2ban (sem SSH);
  - a sessão do owner pode estar usando o modelo → as medições levam a hora e a carga observada (`/api/ps`).
- **Custo:** o grosso vai para Haiku/Sonnet (B1–B5); Opus só em B6; o nível local (Qwen) só para os arquivos sensíveis, uma chamada por vez.
- **Protected areas:** o brief não toca `.cursor/` nem `Documentation/`.

## Critérios de conclusão

1. Os 38 documentos da tabela "Documentos por referência" existem, e o README do brief tem o estado de cada onda.
2. Toda afirmação técnica tem fonte (`caminho:linha` ou URL com data).
3. Varredura de segredos em `referencias/Brief_Referencias/**`: valores conhecidos = **0**, e todas as ocorrências de padrões genéricos triadas (com `[omitido]` ou justificadas no changelog).
4. `70-Revisao.md` com veredito FIÁVEL e os críticos corrigidos.
5. `60`/`61` cobrem as 6 fases do plano de implementação, e cada GAP tem uma decisão ou pendência numerada.
6. Validação empírica com números lidos da saída real.

## Rollback

O brief é aditivo e fica numa pasta própria. Para reverter, basta apagar `referencias/Brief_Referencias/**` e voltar a linha `referencias/` no `.gitignore`. Nenhuma fonte é alterada.

## Referências

- Molde: `E:\GestorERP\.workspace\referencias\`:
  - `Financeiro-ESoft/` (00-CONVENCOES, README, 10..70);
  - `Comparativo-Habil-Esoft-GestorERP.md`.
- Plano de implementação: `.workspace/plans/MCPOllam2026-10-23-150845-mcpollama-implementacao.plan.md`.
- `README.md` §5 (links web) e `CLAUDE.md` (referências).

## Pendências

1. **P-B1:** se a validação empírica mostrar que o `qwen3-ia` (32k) não dá conta do tamanho típico de arquivo, reabrir a P-2 do plano de implementação (modelo de 64k).
2. ~~**P-B2:** o `TOOLS.md` do MCPVMWare (1 MB, gerado) só entra como mecanismo. Confirmar com o owner se quer um catálogo equivalente gerado para o mcpollama.~~ **Decidida (2026-10-23 15:50): sim, gerado do registro de tools.** Com isso, o B2 detalha o mecanismo `MCPVMWare/src/gen` → `TOOLS.md` (entrada, saída, como é disparado, como evita divergir do código) em `mcpvmware/50-Testes_Build_Release.md`, como requisito `MCPO-REQ-BUILD-*`.

## Changelog

### 2026-10-23 15:38 — criação (status: draft)
- **Decisões do owner (via AskUserQuestion):**
  - brief em `referencias/Brief_Referencias`, versionado por exceção no `.gitignore`;
  - pt-BR com acentos;
  - incluir a validação empírica só de leitura.
- **Feito:**
  - `.gitignore` trocado de `referencias/` para `referencias/*` + `!referencias/Brief_Referencias/`;
  - verificado com um arquivo de teste: brief visível ao git = 1, resto de `referencias/` visível = 0 (o arquivo de teste foi removido);
  - dimensionamento medido (tabela acima).
- **Não deu certo:** nada nesta etapa.
- **Ondas:**

  | Onda | Estado |
  | --- | --- |
  | B0 a B7 | pendente (aguardam aprovação do owner) |

### 2026-10-23 15:47 — revisão do plano (status: draft)
- **Decisão do owner (via AskUserQuestion):** aplicar todas as correções da revisão (críticos, médios e menores).
- **Verificado no disco (deu certo):**
  - dimensionamento confere com `wc -l`: 1.426 / 47 / 109.126 / 85.397 linhas; MCPVMWare com 23 arquivos de núcleo e 134 `generated_*`;
  - `git check-ignore`: `referencias/MCPVMWare/.mcp.json` ignorado pela linha 12 do `.gitignore`;
  - molde GestorERP existe (`Financeiro-ESoft/00-CONVENCOES.md` e demais, `Comparativo-Habil-Esoft-GestorERP.md`);
  - MCPNas: 7 reports, 1 plano, `docs/` presente; `.wolf/cerebrum.md` e `buglog.json` dos dois projetos, `MCP-GAPS-VMWARE.md`, `tools/Build-Release.ps1`, `scripts/gen_tools_md.py` e `src/gen` existem;
  - `ia-local.ps1`: saídas 2 (linhas 36 e 55), 3 (linha 51) e 124 (timeout, linha 11);
  - `referencias/Brief_Referencias/` existe e está vazia.
- **Problemas achados e solução aplicada:**
  1. **C1 (crítico):** a varredura só carregava valores do `.mcp.json` e do `csl-ollama.env`, mas o B2 leria arquivos com credenciais prováveis (`settings.local.json`, `config_teste.json`, `DADOS_CONEXAO_TESTE.md`, 5 coleções Postman, 2 reports de servidor de teste; levantados só por nome e por termo, sem abrir). Solução: lista de exclusão em "Âmbito e exclusões" e no prompt dos agentes; a varredura também extrai valores desses arquivos; lição útil deles passa pelo nível local (Qwen).
  2. **C2 (crítico):** o gate "0" contradizia a convenção (`password: [omitido]` casa com o padrão genérico). Solução: dois contadores, valores conhecidos = 0 (gate duro) e padrões genéricos em triagem.
  3. **M1:** a validação empírica não media o que decide a P-B1. Solução: 3 repetições com `load_duration`, prompt grande (~20–28k) para prompt eval, prompt acima de 32k para provar truncamento silencioso.
  4. **M2:** LAN × internet podia não passar pelo Cloudflare (split DNS). Solução: registrar o IP resolvido e o `cf-ray`.
  5. **M3:** conjunto de documentos por referência indefinido e contagem errada (~35). Solução: tabela referência × documento, total de 38.
  6. **M4:** `ollama-mcp-go` em Haiku. Solução: Sonnet para ele, Haiku para `what-is-mcp`.
  7. **Menores:** B7 tratava P-1 e P-3 como respondíveis por dados (agora só a P-2; P-1, P-3, P-4 e P-5 vão ao owner); `61-Matriz` com as fases 0–6; pt-PT residual ("Objectivos", "ficheiros/artefactos"); B3 com conteúdo web tratado como não confiável e WebFetch no lugar do MCP do GitHub (não conectou); convenção de contagem de linhas registrada (o `Measure-Object -Line` dá 1.242 / 38 / 99.991 / 77.363 por ignorar as brancas).
- **Não deu certo:** nada nesta etapa.
- **Ondas:** B0 a B7 continuam pendentes, aguardando aprovação do owner.

### 2026-10-23 15:50 — aprovação e decisões do owner
- **Decisões (via AskUserQuestion):**
  1. **Brief aprovado:** ondas B0 a B7 em sequência, **com gate por onda**. Se um gate falhar (segredos com valor conhecido ≠ 0, afirmação sem fonte), a execução para e o problema é reportado.
  2. **P-B2 = sim, gerado do registro de tools.** O B2 passa a detalhar o mecanismo `src/gen` do MCPVMWare (ver Pendências). Isso entrou também no plano de implementação (inventário e Fase 3, passo 4).
  3. **P-1 (do plano de implementação) = não** (sem `--api-key-file`). No B7, sai da lista de decisões a levar ao owner.
- **Ondas:**

  | Onda | Estado |
  | --- | --- |
  | B0 | aprovada, próxima a executar |
  | B1 a B7 | aprovadas, pendentes |

### 2026-10-23 15:56 — B0 concluída (loop principal)
- **Feito:**
  1. `referencias/Brief_Referencias/00-CONVENCOES.md`: idioma (pt-BR com acentos), citação `caminho:linha` relativa a `referencias/`, contagem `wc -l`, 0 segredos com a lista de exclusão, conteúdo externo como dado, IDs `MCPO-REQ-<AREA>-nnn`, `LIC-<REF>-nnn` (`OMG`/`WIM`/`TNS`/`VMW`/`WEB`/`SRV`) e `REV-C|M|m-nn`, e a tabela das 12 áreas.
  2. `referencias/Brief_Referencias/README.md`: fontes, dimensionamento medido, índice com o estado por onda (38 documentos) e changelog 1.0.0.
  3. `.workspace/scripts/scan-segredos-brief.ps1`, que:
     - carrega os valores de 15 fontes (todo `.mcp.json`, `.claude/settings.local.json`, `config_teste.json`, 5 coleções Postman, `DADOS_CONEXAO_TESTE.md`, 2 reports de servidor de teste, `*.env`, `csl-ollama.env`);
     - extrai por chave sensível (JSON), por tudo sob `env`/`args`/`headers`, pelo par `key`/`value` do Postman e por regex de texto (`senha:`, `--password`, `-p` etc.);
     - imprime só contagens e `arquivo:linha`.
     - Exit: 0 = ok; 1 = `known_hits > 0`; 2 = nenhum valor carregado (gate vazio).
     - O parâmetro `-BriefDir` existe para o controle positivo.
- **Gate (lido):**
  - varredura real: `sources=15 loaded=26`, `files=2 known_hits=0 generic_hits=1 generic_untriaged=0`, exit 0. O único genérico é a linha de `00-CONVENCOES` com `OLLAMA_API_KEY=[omitido]`, aceita pela convenção;
  - **controle positivo:** uma cópia do `MCPVMWare/.mcp.json` no scratchpad deu `known_hits=18`, exit 1, o que prova que a varredura acha segredo real. A cópia foi apagada (`copia removida: True`);
  - `git status`: `00-CONVENCOES.md`, `README.md` e o script aparecem como não rastreados (`??`), ou seja, versionáveis e não ignorados.
- **Não deu certo, e a solução:** a 1ª rodada deu `known_hits=1` em `README.md:17`. O diagnóstico, feito só pela forma do valor, sem imprimi-lo, mostrou que eram os nomes de flag `--insecure`/`-insecure` dos `args` dos 5 servidores do `.mcp.json`, e não segredos. Correção: o script ignora valores que são nome de flag puro (`^--?[A-Za-z][\w-]*$`). Com isso, `loaded` caiu de 35 para 26 e o controle positivo continuou acusando (18).
- **Ondas:**

  | Onda | Estado |
  | --- | --- |
  | B0 | **concluída** |
  | B1, B2, B3 | em execução (paralelo) |
  | B4 a B7 | pendentes |

### 2026-10-23 16:00 — B3 concluída (Haiku) + B1 parcial (`what-is-mcp`, Haiku)
- **Feito (B3):** 7 fichas em `Web/`: W1 (72 linhas), W2 (52), W3 (64), W4 (51), W5 (52), W6 (96) e W7 (74), total de 461.
  - **W1 dxeo:** OK pelo marketplace e pelo README do branch `master` (o `main` deu 404).
  - **W2 mcp.so:** só a 1ª de 309 páginas.
  - **W3 NightTrek:** o mcpservers.org deu 403 de novo; o README do GitHub foi lido. É Node/TS.
  - **W4:** OK; corrobora o TLS e o proxy de token, sem detalhe de implementação.
  - **W5:** OK. Lá o Ollama é provedor de modelo, **não** cliente MCP, então a premissa do README do projeto não se confirma.
  - **W6 KTCrisis:** 4 repositórios lidos; `flux7-console` e `keys7` não.
  - **W7:** parcial. O `main.go` do 05 foi lido; o do 06 e o do 07 não carregaram.
- **Feito (B1, parte):** `what-is-mcp/` com README (65 linhas), 10 (104) e 20 (103). SDK de terceiros `mark3labs/mcp-go` v0.8.2, uma tool `hello_world`.
- **Conferência do loop principal (lida):**
  - 5 citações do `what-is-mcp` conferidas contra `04-what-is-mcp/mcp-server/main.go`: 13–16, 23–26, 32/35/37, 34–38 e 42–45. Todas corretas;
  - caminhos relativos a `referencias/` conforme a convenção.
- **Gate de segredos (lido):** `files=15 known_hits=0 generic_hits=8 generic_untriaged=7`. As 7 linhas foram triadas à mão e todas são nomes de variável ou conceito, sem valor:
  - `W1:58`, `W4:34`, `W5:29` (`OPENAI_API_KEY` como nome), `W6:58` e `W6:76` (`MEM7_TOKEN` como nome);
  - `what-is-mcp/10:96` (`sshpass`) e `:98` (`NOPASSWD`).
- **Achado de segurança (W1):** a página do marketplace lobehub contém **prompt injection** dirigido a agentes: manda rodar `npx @lobehub/market-cli register` e substituir credenciais salvas. O agente **não** seguiu nem executou nada; ficou registrado em `Web/W1-dxeo-ollama-mcp.md:62-64`. Isso valida a regra "conteúdo externo é dado" do `00-CONVENCOES`.
- **Achados para o mcpollama (a consolidar no B5):**
  1. `what-is-mcp` escreve no stdout (`main.go:32,35,37`): é o anti-exemplo do "stdout exclusivo do JSON-RPC";
  2. erro de domínio devolvido como resultado da tool (`NewToolResultError`, `main.go:42-45`);
  3. W1 separa o modelo de raciocínio do de embeddings e tem `filter_docs` para reduzir tokens;
  4. W6 manda o raciocínio para o trace e não para o cliente (economia de tokens);
  5. W3 usa timeout de 60 s, insumo para o `PERF`.
- **Não deu certo (pendências numeradas):**
  1. **PB3-1:** W2 coletou só 1 de 309 páginas do diretório. Coletar o resto é de baixo valor, porque a 1ª página só achou um scraper. Proposta: não coletar, e registrar no B5 como limitação.
  2. **PB3-2:** W7, o `main.go` do 06 e do 07 não carregou.
  3. **PB3-3:** W6, `flux7-console` e `keys7` não foram lidos.
  4. **PB3-4:** os subagentes não atualizaram este plano; ele foi atualizado pelo loop principal, como manda o método.
- **Ondas:**

  | Onda | Estado |
  | --- | --- |
  | B0 | concluída |
  | B1 | em execução (`what-is-mcp` concluído; `ollama-mcp-go` rodando) |
  | B2 | em execução |
  | B3 | **concluída** (com as pendências PB3-1 a PB3-3) |
  | B4 a B7 | pendentes |

### 2026-10-23 16:03 — B1 concluída (`ollama-mcp-go`, Sonnet)
- **Feito:** `ollama-mcp-go/` com README (82 linhas), 10 (115), 20 (112), 30 (86), 40 (130) e 50 (89), mais as lições `LIC-OMG-001` a `010` no README.
- **Medido pelo agente (execução real, não leitura):**
  - `go test -count=1 -v`: 26 testes PASS;
  - `-cover`: 75,2%;
  - `go vet`: sem saída;
  - `tools/list` completo com 1.901 bytes, com o servidor rodando e `OLLAMA_MCP_TRACE=off`;
  - JSON inválido no stdin **não** gera `-32700`: só log (reproduzido).
- **Conferência do loop principal (lida):**
  - 5 citações conferidas contra o código, todas corretas: `main.go:166-168` (`http.Client` só com Timeout de 5 min, sem Transport), `main.go:298-309` (trace com default `/tmp/ollama-mcp-trace.log`, `off` desliga), `main.go:644-648` (JSON inválido → `log` + `continue`), `main.go:86-87` (contadores em campos de topo) e `protocol.go:97-109` (`negotiate`);
  - **2ª via de que a referência ficou intacta:** nenhum arquivo de código foi modificado após 15:55 (`LastWriteTime`), e não há trace em `C:\tmp`, `E:\tmp` nem `%TEMP%`. O `git status` citado pelo agente **não** prova isso, porque `referencias/` é ignorada pelo git do projeto. Por isso a checagem foi feita por data de arquivo.
- **Não deu certo:** o `ollama-mcp-go/.git/index` foi reescrito às 15:57:44. É efeito colateral do `git status` rodado pelo agente, que atualiza o cache de datas do índice. O conteúdo não mudou e a árvore de trabalho está limpa (0 alterações). **Não revertido** (reverter também é tocar); fica registrado. Ação para as próximas ondas: os prompts proíbem rodar `git` dentro de `referencias/`.
- **Gate de segredos (lido):** `files=18 known_hits=0 generic_hits=9 generic_untriaged=7`. As 7 são as mesmas já triadas na B3; os novos documentos não trouxeram nenhuma ocorrência sem triagem. O agente trocou um placeholder em `30:33` por `[omitido]`.
- **Achados para o mcpollama (a consolidar no B5):**
  1. **Sem auth nem TLS:** não há `Authorization` nem `Transport` customizado. A referência não fala com o Ollama da CSL sem alteração; é GAP do `SEC`/`OLL`.
  2. **Trace ligado por padrão** em `/tmp`, com permissão `0644`, gravando prompt, resposta e raciocínio em texto puro. Incompatível com dados sensíveis: no mcpollama, trace só com opt-in.
  3. **Contadores fora do padrão MCP:** `prompt_eval_count`/`eval_count` estão no topo, e o `embed` não os devolve. No mcpollama, devolver no conteúdo ou em `_meta` e testar nos clientes reais.
  4. O `embed` devolve só as dimensões e os 16 primeiros valores.
  5. **Loop sequencial sem `context`:** uma chamada de até 5 min bloqueia o `ping` e o `notifications/cancelled`. É relevante para o `PERF`, porque a GPU já é fila única.
  6. O chunk de stream não tem `error`/`done_reason`, então um erro no meio vira resposta vazia (inferência).
  7. Molde de teste aproveitável: `httptest` com NDJSON real (`main_test.go:83-159`). Mas o `main()` não é testado (o teste copia o loop).
  8. **Modelo obrigatório em toda tool:** sem default. O mcpollama tem `--model` com default `qwen3-ia`.
- **Pendências (agente, não confirmadas):** spec MCP `2026-07-28` não conferida com a oficial; comportamento do trace em `/tmp` no Windows e do `bufio.ErrTooLong` (linha acima de 4 MB) não executados; clientes reais lendo campos de topo não testados.
- **Ondas:**

  | Onda | Estado |
  | --- | --- |
  | B0 | concluída |
  | B1 | **concluída** |
  | B2 | em execução (`mcpvmware`, `truenas-mcp`) |
  | B3 | concluída (pendências PB3-1 a PB3-3) |
  | B4 a B7 | pendentes |

### 2026-10-23 16:14 — B2 concluída (`truenas-mcp` e `mcpvmware`, Sonnet)
- **Feito:**
  - `truenas-mcp/` com 7 documentos: README (96 linhas), 10 (91), 20 (85), 30 (77), 40 (83), 50 (63) e 80 (183), com 30 lições `LIC-TNS`;
  - `mcpvmware/` com 7 documentos: README (81), 10 (108), 20 (94), 30 (106), 40 (99), 50 (106) e 80 (221), com 36 lições `LIC-VMW`;
  - builds, testes e execuções feitos pelos agentes em **cópias no scratchpad**, com URL e chave fictícias.
- **Medido pelos agentes (execução real):**
  - truenas-mcp: `tools/list` com 730 tools e 892.704 bytes; 64 testes PASS; cobertura de `truenas/` 70,6%; `ping` devolve `-32601`;
  - MCPVMWare: linha de entrada acima de 64 KB **mata o servidor** (exit 1); `compact` reduz o `tools/list` em 63% a 67% (924 tools: 902.083 → 323.811 bytes); o audit log grava `password` fictício em texto puro; um segredo na URL vaza no erro; os 1.047 nomes do registro e do `TOOLS.md` são idênticos.
- **Conferência do loop principal (lida), 5 citações, todas corretas:**
  - `MCPVMWare/src/mcpvmware-mcp/main.go:310-316`: `bufio.NewScanner` sem `Buffer()`;
  - `MCPVMWare/src/gen/main.go:9-11`: "It does NOT generate any tools/*.go file";
  - `MCPVMWare/src/cmd/dumptools/main.go:23-30`: `NewRegistry(..., nil, ...)`;
  - `MCPNas/truenas-mcp/truenas/tls.go:18-34`: `RootCAs: pool` com `x509.NewCertPool()`, que substitui o pool do SO;
  - `MCPNas/truenas-mcp/tasks/types.go:42`: `Result` com `json:"-"`.
- **Integridade das referências (2ª via, por data de arquivo):** 0 arquivos modificados após 15:55 fora de `.git/`. O `MCPVMWare/.git/index` foi reescrito às 16:02:18 por 4 `git log`/`git describe` do agente, rodados antes de a restrição chegar; o conteúdo não mudou e nada foi revertido. O truenas-mcp não tem `.git` próprio, então o `git status` do agente bateu no repositório do projeto, sem efeito em `referencias/`.
- **Não deu certo, e a solução:**
  1. **Gate de segredos falhou:** `known_hits=6` em `mcpvmware/`. O diagnóstico foi feito sem revelar o valor, com um novo modo `-Diagnose` no script (mostra a origem e a forma do valor) e uma busca do caminho JSON com o valor mascarado. Os 6 vinham do mesmo valor: `args[0]` de `_mcpServersDesativados.mcpVmwareTest` no `MCPVMWare/.mcp.json`, com 7 caracteres, minúsculas e hífen, que aparece **107 vezes** no código e na doc públicos do MCPVMWare. Ou seja, não é credencial.
     - Solução: allowlist por **SHA-256** em `.workspace/scripts/scan-segredos-allowlist.txt`, com origem, motivo e data, sem o valor em claro. O script registra `allowlisted=n`.
     - Resultado: `loaded=25 allowlisted=1`, `files=32 known_hits=0`. O controle positivo continua acusando `known_hits=14`, e a cópia foi removida.
  2. **1ª tentativa do `-Diagnose` quebrou o script:** um `''` virou `'` na substituição em massa e deixou uma string sem fechar na linha 39. Corrigido com edição direta. O script não chegou a rodar com o erro.
  3. **Brief contradizia a decisão P-1:** `truenas-mcp/30:24`, `truenas-mcp/80:114` e `mcpvmware/30:38` recomendavam `--api-key-file` (os agentes foram lançados antes da decisão). Corrigido para env `OLLAMA_API_KEY`, citando a P-1 = não.
  4. **A premissa do `src/gen` estava errada:** ele é um classificador de uso único, e o `TOOLS.md` vem de `src/cmd/dumptools` + `scripts/gen_tools_md.py`, disparados à mão, sem teste de sincronia. O plano de implementação foi corrigido (inventário e Fase 3, passo 4): `src/cmd/dumptools`, com um teste que compara o `TOOLS.md` com o registro.
- **Triagem dos genéricos (lida):** 29 novos em `mcpvmware/` e `truenas-mcp/`, todos nomes de flag e de campo (`--password`, `VCENTER_PASSWORD`, `api-key`, `auth.login_with_api_key`), conceitos, ou sentinelas públicas de teste (`hunter2`, `1-verysecretapikey`, de `client_test.go`). Nenhum valor real.
- **Achados que mais pesam para o mcpollama (a consolidar no B5):**
  1. `bufio.Scanner` sem `Buffer()` nos dois moldes: linha acima de 64 KB mata o servidor, e um prompt de 32k tokens passa disso. **MUST:** `Buffer` grande ou `bufio.Reader`.
  2. Loop sequencial sem prazo por chamada nos dois moldes; `ping` → `-32601`; `protocolVersion` fixo em `2024-11-05`.
  3. Autenticação ou login no arranque derruba o cliente ("Connection closed"). O mcpollama **não** chama o Ollama no `initialize`.
  4. `tasks/` do truenas como desenho para gerações longas: espera limitada de 100 s, abaixo dos 120 s do cliente, com fallback para `task_id`. Mas é preciso expor o `Result` (bug do `json:"-"`).
  5. `--tls-ca` substitui o pool do SO (`tls.go:33`). Como o host sem porta usa o certificado público da Cloudflare, o mcpollama precisa de `SystemCertPool()` + a CA de origem. **Validar no B4** contra os dois caminhos (LAN `:8443` e internet).
  6. Redação: trocar o valor exato do segredo (`ReplaceAll`) é mais barato que uma lista de campos. O audit e o erro do MCPVMWare vazam `args` e URL.
  7. Tool de arquivo do MCPVMWare sem allowlist, symlink ou limite: o buraco que o `--allow-root` do mcpollama precisa fechar.
  8. `compact` reduz o `tools/list` em ~64%, e o Cursor limita `len(servidor)+len(tool) ≤ 60`.
  9. BOM e CRLF quebraram testes no truenas: `.gitattributes` no scaffold.
  10. Versão via `-ldflags`, conferida no binário (`Build-Release.ps1`).
- **Pendências (B2):**
  1. **PB2-1:** suíte completa do MCPVMWare não rodada (~90 s, intermitente no Windows); só discovery, gate e modo (ok).
  2. **PB2-2:** cobertura de `tools/` do truenas não medida (o BOM em `tools/kmip.go` quebra o `-cover`); `-race` não rodado.
  3. **PB2-3:** divergência entre 909 tools / 910.314 bytes (medição anterior) e 924 / 902.083 (agente), não reconciliada.
  4. **PB2-4:** `MCPVMWare/CLAUDE.md` é o template Delphi (`LIC-VMW-035`).
  5. **PB2-5:** a lição dos arquivos excluídos (`DADOS_CONEXAO_TESTE.md`, `*configuracao-servidores-teste*`) é sobre como guardar dados de conexão de teste. Candidata ao nível local, de baixa prioridade.
  6. **PB2-6:** o truenas-mcp não tem `go.mod`, `README` nem `Makefile` na cópia; a receita de release está só no texto do bug-021.
- **Ondas:**

  | Onda | Estado |
  | --- | --- |
  | B0 a B3 | **concluídas** |
  | B4 | próxima (loop principal, ao vivo, uma chamada por vez) |
  | B5 a B7 | pendentes |

### 2026-10-23 16:25 — B4 em progresso, BLOQUEADA aguardando o owner (loop principal)
- **Feito:**
  - leitura do `CONEXAO-IA.md`, só das seções 1, 4, 5, 6, 7 e 8. As seções 2 (SSH) e 3 (GPU) ficaram fora. Antes, as 13 linhas com termos sensíveis foram mapeadas sem exibir conteúdo: nenhuma tinha string de 24 caracteres ou mais, e o guia só dá os **caminhos** dos segredos;
  - leitura do cabeçalho do `ia-local.ps1` (linhas 1–60);
  - sonda Go `b4probe` no scratchpad (fora de `src/`), com `go vet` e build limpos. Lê o token do `csl-ollama.env` só em memória, toda saída passa por um scrub do token, e as chamadas são **sequenciais**, cada prompt com um nonce único para não reaproveitar o cache de prompt;
  - saída completa em `scratchpad/b4probe/saida-b4.txt` (16:18:25–16:23:57).
- **Medido (saída lida):**
  1. **DNS:** de dentro da LAN, `ia.cslsolucoes.com.br` → `10.100.2.2` (split-horizon).
  2. **Matriz TLS em `:8443`** (`GET /api/tags` com token):
     - pool só do SO: `x509: certificate signed by unknown authority`;
     - só a CA de origem: 200 (8 ms);
     - **SO + CA de origem (`SystemCertPool()` + `AppendCertsFromPEM`): 200 (8 ms)**. Funciona no Windows com Go 1.26.5, então o desenho que a `LIC-TNS-013` pede é viável;
     - `server: nginx`, sem `cf-ray` (não passa pelo Cloudflare).
  3. **Caminho "internet" (`https://host` sem porta) a partir da LAN:** `dial tcp 10.100.2.2:443` com timeout de 21 s, nos 3 pools. O DNS interno manda para o IP da LAN, que não atende na 443. **O caminho Cloudflare não foi testado** (exige resolver por DNS público).
  4. **Modelo (`/api/ps` e `/api/show`):** `qwen3-ia:latest` → `parent_model` `qwen3-coder:30b-a3b-q4_K_M`, família `qwen3moe`, 30,5B, Q4_K_M, 128 experts com 8 ativos. `num_ctx` **131072** (o modelo suporta 262144), `context_length` 131072 no `/api/ps`, `size_vram` 25.562.028.438 (100% GPU). Capacidades `completion` e `tools`. Parâmetros: `temperature 0.7`, `top_k 20`, `top_p 0.8`, `repeat_penalty 1.05`. Modificado em 2026-10-23 15:36. **Contradiz o `CLAUDE.md` e os planos (32k).**
  5. **Modelos instalados (`/api/tags`):** `qwen3-ia`, `qwen3-coder:30b-a3b-q4_K_M`, `nomic-embed-text` (768 dimensões, ctx 2048, 137M, F16), `qwen3.8:27b-64k` e `qwen3.8:27b-q4_K_M`. O `OPENAI_MODEL` do `csl-ollama.env` (nível local) é `qwen3.8:27b-64k`, **outro** modelo, e não o `qwen3-ia`.
  6. **Auth e bloqueio:** sem token → **401** `{"error":"unauthorized"}`; `POST /api/pull` → **403** (página HTML do nginx).
  7. **3 generates curtos** (44 tokens de prompt, `num_predict` 64, `temperature` 0):

     | Rodada | Total | Geração | Prompt eval | `load_ms` |
     | --- | ---: | ---: | ---: | ---: |
     | 1 | 536 ms | 41 tokens a 96,5 tok/s | 678 tok/s | 0 |
     | 2 | 438 ms | 37 tokens a 96,8 tok/s | 866 tok/s | 0 |
     | 3 | 437 ms | 37 tokens a 97,0 tok/s | 878 tok/s | 0 |

     `load_ms` 0 nas três porque o modelo já estava residente. Confirma os "~93 tok/s" do guia.
  8. **Embeddings (`nomic-embed-text`):** 200, **768 dimensões**, `load_ms` 3.230, `total_ms` 9.936, wall 20.896 ms (carga a frio), 13 tokens.
- **Não deu certo:**
  1. **Prompts grandes, conexão derrubada pelo servidor** (`wsarecv: Foi forçado o cancelamento de uma conexão existente pelo host remoto`), sem nenhum contador devolvido:
     - calibração de 500 linhas (42.540 caracteres) aos **45 s**;
     - ~24k tokens (800 linhas, 68.040 caracteres) aos **90 s**;
     - ~40k tokens (1.333 linhas, 113.679 caracteres) aos **90 s**.
     - Por causa da 1ª falha, a calibração não mediu tokens por linha, e as metas de 24k e 40k usaram um palpite de 30 tokens/linha.
  2. **Depois disso, o `qwen3-ia` saiu da VRAM**, apesar de `expires_at` infinito (`keep_alive -1`). O `/api/ps` às 16:23:57 e de novo às 16:24:53 mostra só o `nomic-embed-text`. **Pode ter sido provocado por esta sonda.**
  3. **Hipóteses, nenhuma provada:**
     - **H1:** o runner caiu ao processar o prompt grande (ex.: falta de memória no buffer de computação) e o modelo foi descarregado;
     - **H2:** timeout de inatividade no nginx ou num firewall no meio. Com `stream:false` nenhum byte trafega até o fim; os 90 s exatos repetidos sugerem um timeout configurado. Mas a H2 sozinha não explica o descarregamento.
     - Discriminar exige os logs do servidor (`journalctl -u ollama`, `error.log` do nginx) ou um reteste com `stream:true`.
  4. **Erro de processo meu:** a sonda não checou o `/api/ps` entre as chamadas grandes. Se tivesse checado, saberia em qual delas o modelo caiu.
- **Estado:** **BLOQUEADA, aguardando a decisão do owner** sobre o diagnóstico e o reteste (mexe no servidor). Nenhuma nova chamada pesada foi feita após 16:23:57; só um `/api/ps` às 16:24:53.

### 2026-10-23 18:12 — B4: reteste em streaming (decisão do owner às 16:25: "Recarregar + 1 reteste em streaming")
- **Execução:** `b4probe stream`, lançada às ~16:27. O processo só começou a imprimir às **18:10:47**, um atraso de ~1h43 entre o lançamento e a 1ª linha. A causa não foi investigada (o processo em background pode ter ficado suspenso). Saída em `scratchpad/b4probe/saida-b4-stream.txt`.
- **Medido (saída lida):**
  1. `/api/ps` às 18:10:48: só `nomic-embed-text`, **`qwen3-ia` não carregado** (estado igual ao das 16:24:53).
  2. **Recarga:** generate curto às 18:11:24, `load_ms` **36.449** (carga a frio do MoE), resposta "ok".
  3. `/api/ps` após a recarga: só `qwen3-ia` (25.562.028.438 bytes, ctx 131072). O **`nomic-embed-text` foi desalojado** pela carga.
  4. **1 prompt de 500 linhas (42.540 caracteres) com `stream:true`:** conexão derrubada pelo servidor aos **45.019 ms**, **antes dos cabeçalhos HTTP**. Nenhum chunk recebido.
  5. `/api/ps` às 18:12:22: **`models: []`, nenhum modelo carregado.**
- **Conclusão (com prova, 2 reproduções):** um prompt de ~42 mil caracteres derruba o processamento do lado do servidor a ~45 s, e os modelos saem da VRAM.
  - Na 2ª vez, nenhum modelo sobrou, o que é compatível com o serviço `ollama` ter caído e reiniciado **(inferência)**.
  - O **streaming não resolve**: a falha acontece no processamento do prompt, antes de qualquer byte.
  - A **H1** (falha do servidor no prompt grande) fica sustentada.
  - A **H2** (timeout de inatividade) **não explica** o descarregamento e não é a causa principal.
  - A causa exata (estouro de memória da GPU, `KV_CACHE_TYPE=q4_0`, nginx + reinício) **só aparece nos logs do servidor**.
- **Parei na 1ª falha**, como combinado: o modelo **não** foi recarregado depois. Hoje o servidor está **sem nenhum modelo residente**; o próximo cliente que chamar paga ~36 s de carga.
- **Documentos gravados (B4):**
  - `Servidor-Ollama-CSL/README.md`: identificação, modelos, acesso, nível `local`, limites medidos. Cita o `CONEXAO-IA.md` e o `ia-local.ps1` por linha, sem nenhum valor secreto;
  - `Servidor-Ollama-CSL/dados/validacao-empirica.md`: os 3 conjuntos de execução, as tabelas medidas, a falha reproduzida e 6 implicações para o mcpollama.
- **Gate de segredos (lido):** `files=34 known_hits=0`. Os 2 documentos do servidor não têm genérico sem triagem; a linha com o token tem `[omitido]`.
- **Achados que mudam o projeto (a levar ao B5/B7):**
  1. **`qwen3-ia` NÃO é mais 32k:** é o MoE `qwen3-coder:30b-a3b` com `num_ctx` 131072, desde as 15:36. O `CLAUDE.md` (seção "Servidor Ollama") e a P-2 do plano de implementação estão desatualizados.
  2. **O limite utilizável hoje é menor e desconhecido** (defeito do item acima). As tools de arquivo da Fase 4 dependem dele.
  3. A tool `embed` precisa de um modelo próprio (`nomic-embed-text`): o `qwen3-ia` só tem `completion` e `tools`.
  4. `SystemCertPool()` + `--tls-ca` funciona no Windows.
  5. Carga a frio de ~36 s, e os modelos se desalojam entre si na VRAM.
- **Pendências (B4):**
  1. **PB4-1:** diagnosticar e corrigir o defeito do prompt grande no servidor (logs `journalctl -u ollama` e `error.log` do nginx). Depende do owner.
  2. **PB4-2:** medir o limiar de prompt utilizável e a velocidade de prompt eval com prompt grande, depois do PB4-1.
  3. **PB4-3:** validar o caminho internet (Cloudflare) a partir de fora da LAN ou com DNS público.
  4. **PB4-4:** decidir se o `qwen3-ia` deve ser recarregado agora (está fora da VRAM).
- **Ondas:**

  | Onda | Estado |
  | --- | --- |
  | B0 a B3 | concluídas |
  | B4 | **concluída com defeito aberto** (documentos gravados; PB4-1 a PB4-4 abertas) |
  | B5 a B7 | pendentes |

### 2026-10-23 18:29 — decisões do owner sobre a B4 + recarga
- **Decisões (via AskUserQuestion):**
  1. **PB4-1/PB4-2:** o owner investiga os logs do servidor (`journalctl -u ollama` e `error.log` do nginx, janelas ~16:19–16:24 e 18:11–18:12). O brief segue para a B5, tratando o limite de prompt como **GAP aberto**.
  2. **PB4-4 = sim, recarregar.** Feito: `b4probe curto` às 18:28:31–18:29:04, com `load_ms` **31.963**, resposta "ok", e `/api/ps` com `qwen3-ia:latest` residente (`size_vram` 25.562.028.438, ctx 131072, `expires_at` infinito). **PB4-4 fechada.**
- **Ondas:**

  | Onda | Estado |
  | --- | --- |
  | B0 a B4 | concluídas (B4 com PB4-1 a PB4-3 abertas, a cargo do owner ou para depois da correção) |
  | B5 | em execução (Sonnet) |
  | B6 e B7 | pendentes |

### 2026-10-23 18:52 — B5 concluída (Sonnet)
- **Feito (agente):**
  - `60-Requisitos_mcpollama.md` (353 linhas): **171 requisitos** (104 MUST, 53 SHOULD, 14 COULD) nas 12 áreas, cada um com fonte e critério de verificação, mais 24 anti-padrões medidos ligados ao requisito que os proíbe;
  - `61-Matriz_Cobertura.md` (384 linhas): 53 resolvidos, 45 parciais, 73 GAPs, com MUST por fase e matriz fase × área;
  - `Comparativo-Referencias-mcpollama.md` (108 linhas).
  - Os três foram gerados de um único conjunto de dados para não divergirem.
- **Conferência do loop principal (lida):**
  1. 171 IDs únicos no `60`. Por área: BUILD 12, CFG 12, FILE 17, INTEG 12, OBS 10, OLL 16, PERF 12, PROTO 22, SEC 16, TEST 18, TOK 9, TOOL 15. Por prioridade: MUST 104, SHOULD 53, COULD 14 (soma 171). **Bate com o relato.**
  2. `61` × `60`: 171 IDs em cada, 0 só num lado (`Compare-Object`).
  3. Varredura: `files=37 known_hits=0`. Os 16 genéricos novos foram triados à mão: `OLLAMA_API_KEY` como nome, "Bearer"/"password" como conceito e menções à P-1; nenhum valor.
- **GAPs com decisão proposta (PB5-1 a PB5-13, em `61` §GAPs):**
  - **PB5-1:** limite de prompt com `--max-prompt-chars` e default provisório abaixo de 42.540 caracteres; depende de PB4-1/PB4-2;
  - **PB5-2:** leitura de arquivo com `--allow-root`, `EvalSymlinks`, casos do Windows, denylist e chunking;
  - **PB5-3:** timeout dos clientes MCP, a medir na Fase 5;
  - **PB5-4:** semáforo de GPU N=1;
  - **PB5-5:** contadores no texto e em `_meta`;
  - **PB5-6:** contrato do `embed`;
  - **PB5-7:** caminho internet e TLS público;
  - **PB5-8:** stream × não-stream;
  - **PB5-9:** loop concorrente e `cancelled`;
  - **PB5-10:** erro específico para queda do serviço;
  - **PB5-11:** qualidade e economia na Fase 6;
  - **PB5-12:** protocolo `2026-07-28` fora desta versão;
  - **PB5-13:** sem escrita em disco.
  - O agente sugere ainda as pendências **P-6 a P-10** para o plano de implementação (teto de prompt, timeout e assíncrono, conjunto de tools, `openWorldHint`, denylist e regras de caminho do Windows). Elas serão levadas ao owner no B7.
- **Contradições apontadas pelo agente e corrigidas pelo loop principal:**
  1. **Ressalva "desatualizado" acrescentada** nas 6 linhas que diziam ctx 32k (`mcpvmware/10:59`, `mcpvmware/80:20`, `ollama-mcp-go/30:53` e `:85`, `Web/W1:53`, `Web/W7:53`), apontando para a validação §3 e §7. O texto original foi mantido, porque era a premissa da época.
  2. `ollama-mcp-go/30:33` dizia "token ... **ou de arquivo**", contra a P-1. Alinhado: env `OLLAMA_API_KEY` preferida, `--api-key` aceito, `--api-key-file` descartado.
  3. README do brief: B4 e B5 marcadas como feitas (v1.2.0).
  4. **Ficam para o B6/B7** (não corrigidas aqui): `-32602` × `isError` para tool desconhecida (PROTO-009, conferir na spec); a confusão do "timeout de 120 s do cliente" no `truenas-mcp/README` item 7 (o cliente é o de backend, não o MCP); `openWorldHint` (P-9); build Linux × MCPVMWare só Windows; `CLAUDE.md`, `README.md` e o plano de implementação ainda dizem 32k (é o B7).
- **Não deu certo, e a solução:** ao acrescentar as ressalvas, gravei com `WriteAllLines`, que converteu os 5 arquivos editados de **LF para CRLF**, enquanto os outros 32 estão em LF. Detectado na checagem seguinte e revertido para LF. Agora: 0 arquivos com CRLF e 0 com BOM nos 37.
- **Ondas:**

  | Onda | Estado |
  | --- | --- |
  | B0 a B5 | concluídas |
  | B6 | em execução (Opus) |
  | B7 | pendente |

### 2026-10-23 19:06 — relatório do incidente do servidor (pedido do owner)
- **Feito:** `.workspace/reports/MCPOllam2026-10-23-190500-incidente-ollama-prompt-grande.report.md`, com:
  - o que eu fazia, a linha do tempo das 3 execuções com as portas de origem de cada requisição grande, os fatos × inferências e os comandos só de leitura para o owner rodar (`journalctl -u ollama`, kernel Xid/OOM, nginx);
  - os meus erros de processo.
  - Evidência bruta preservada em `...incidente-ollama-prompt-grande.evidencias/`: `saida-b4.txt` (9.689 bytes) e `saida-b4-stream.txt` (1.383 bytes). Varridas antes da cópia: `known_hits=0` e 0 `Bearer` literal.
- **Achado novo (provado):** o relógio do servidor está alinhado com o da estação. O `expires_at` com `keep_alive` infinito é "última requisição + `math.MaxInt64` ns" (106.751 dias + 23:47:16,854). Subtraindo, `nomic` → 16:23:36 (= fim do embed) e `qwen3-ia` → 18:29:04 (= fim da recarga). Os horários do relatório valem direto nos logs.
- **Inferência registrada no relatório (não provada):** os 90 s das 2 requisições seguintes da execução 1 batem com "recarga a frio (~32–36 s) + os mesmos ~45 s até a queda". Isso indica que o serviço já tinha caído na 1ª (16:20:15).

### 2026-10-23 19:09 — B6 concluída (Opus, revisão adversarial)
- **Feito (agente):**
  - `70-Revisao.md` com veredito **COM RESSALVAS**, confiança ~85%;
  - **84 citações conferidas**: 71 confirmadas, 4 imprecisas, 9 não sustentadas. Das 64 de código, 62 exatas;
  - recontagens: todas batem;
  - **0 críticos, 8 médios** (REV-M-01 a M-08, quase todos coluna "Cobertura" otimista no `61`) **e 12 menores**;
  - correções aplicadas em `60` (1.1.0), `61` (1.1.0), `Comparativo` (1.1.0), `truenas-mcp/README.md:89`, `Web/W1:51`, `validacao-empirica.md:22` (execução 4) e README do brief (1.3.0);
  - pendências PB6-1 a PB6-6.
- **Conferência do loop principal (lida):**
  1. Cobertura no `61`: 171 linhas de requisito únicas, **Resolvido 48, Parcial 47, GAP 76**. Bate com o relato.
  2. `SEC-002` passou a GAP, com a evidência `MCPNas/truenas-mcp/truenas/client.go:305` (chave no corpo).
  3. Varredura: `files=38 known_hits=0`. LF e sem BOM nos 38.
  4. **Integridade de `referencias/`:** 0 arquivos de trabalho alterados após 18:50. Achei 15 `.git/FETCH_HEAD` (MCPVMWare e 13 repositórios em `MCPVMWare/referencia/`) escritos em **19:06:28–19:06:29**, todos no mesmo segundo, e `MCPVMWare/.git/gk/config` às 19:00:46, com processos `gk` (GitKraken) e Cursor ativos. **Origem: auto-fetch do tooling do IDE, não dos agentes** (`referencia/` nunca esteve no âmbito, e um agente não faria fetch simultâneo em 14 repositórios). O fetch só atualiza o `FETCH_HEAD`, sem tocar nos arquivos de trabalho. Sinalizado ao owner e não combatido, pela regra de não brigar com o tooling.
- **Pendências PB6 (do owner):**
  1. **PB6-1:** subir `PROTO-009` para MUST? A spec MCP 2025-11-25 trata tool desconhecida como erro de protocolo (`-32602`).
  2. **PB6-2:** valor de `openWorldHint` (= P-9).
  3. **PB6-3:** critério para os 7 GAPs que têm referência parcial.
  4. **PB6-4:** o script de segredos não guarda a triagem dos genéricos, que reaparecem a cada rodada.
  5. **PB6-5:** `CLAUDE.md`, `README.md` e o plano ainda diziam 32k (**feito no B7**).
  6. **PB6-6:** arquivar a saída bruta das próximas sondas (**adotado**: a 2ª via de 19:18 foi arquivada).

### 2026-10-23 19:21 — B7 (fecho) + desfecho do incidente do servidor
- **Relatório de resposta do owner** (`.workspace/reports/MCPOllam2026-10-23-191600-resposta-incidente-ollama-prompt-grande.report.md`):
  - causa: **reboot da máquina** por falha de PCIe na GPU 3 (`07:00.0`, Root Port `00:1c.5`, Gen1 x1), com `GPU lost from the bus` / `uvm fatal error 0x60`;
  - `last reboot` registra 16:20, 16:21, 16:23 e 18:11, os mesmos horários das 4 requisições;
  - `NRestarts=0` no `ollama`;
  - mitigação: 3 GPUs e `num_ctx` 65536;
  - journal passou a ser persistente.
- **Minha inferência estava errada** (queda e reinício do serviço ou do runner): era o host. Os fatos medidos estão confirmados. Correção registrada no relatório do incidente, no `validacao-empirica.md` §9 e aqui.
- **Verificação por 2ª via (decisão do owner às 19:18: "Sim, 1 chamada"), pelo caminho do cliente** (estação → nginx `:8443`):
  1. **19:18:03:** `/api/ps` com `qwen3-ia`, `context_length` **65536**, `size_vram` **21.660.958.717**; `/api/show` com `num_ctx 65536` e `modified_at` 19:14:42.
  2. **19:18:39–19:18:43:** o mesmo prompt de 500 linhas / 42.540 caracteres, com `stream:true`, deu **200**, `prompt_eval_count` **15.959** em **3.199 ms (~4.988 tok/s)**, 1º chunk aos 3.249 ms, total de 3.577 ms, resposta correta `primeira=00001 ultima=00500`. O `/api/ps` depois mostrou o modelo ainda carregado.
  3. **Calibração:** 31,92 tokens/linha = **~2,67 caracteres por token** (texto sintético).
  4. Evidência arquivada em `.workspace/reports/...incidente...evidencias/saida-b4-2via.txt`, varrida antes (`known_hits=0`).
- **Feito no B7:**
  1. **Plano de implementação:**
     - nota de SSOT apontando para os MUST por fase do `61` §3 (43/29/37/23/14/6), sem duplicar requisitos;
     - itens "Do brief" nas Fases 1–5 (vindos de `61` §5);
     - Objetivo 5, Fase 4 (teto configurável, ~2,67 caracteres/token, ~5.000 tok/s) e Riscos (hardware mitigado, contexto muda após o reparo, carga a frio) atualizados;
     - P-2 marcada **superada**; P-4 com a nota de troca de modelo; **P-5 a P-10** registradas;
     - Fase 3: `embed` com modelo próprio.
  2. **`CLAUDE.md`, seção do servidor:** modelo MoE, ~97 tok/s e ~5.000 tok/s de prefill, `nomic-embed-text`, **config provisória de 3 GPUs e `num_ctx` 65536**, "não fixar o contexto no código", carga a frio, DNS split-horizon, `SystemCertPool` + `--tls-ca` e o modelo do nível `local`. Link 5 (Dolphin) corrigido.
  3. **`README.md` da raiz:** diagrama (3× RTX 3070, `num_ctx` 65536 provisório), tabela do servidor (linha "Contexto"), link 3 (NightTrek lido no GitHub) e link 5 (Dolphin) corrigidos.
  4. **Brief:** `validacao-empirica.md` 1.1.0 (banner, execuções 5 e 6, §9 com causa, mitigação, 2ª via e implicações); README do servidor (item 0 de atualização); `61` (nota do PB5-1: o teto "abaixo de 42.540" deixa de valer); README do brief 1.4.0.
  5. Relatório do incidente: status "mitigado", bloco "Desfecho" e correção da inferência.
- **Gates (lidos):** `files=38 known_hits=0`; 0 CRLF; 0 BOM; nenhum "32k" desatualizado no `CLAUDE.md`, no `README.md` ou no plano, fora a P-2 riscada como histórico.
- **Pendências que ficam com o owner:** P-3 a P-10 (plano de implementação); PB6-1 a PB6-4; PB4-3 (caminho internet); reparo físico da GPU 3 (relatório de resposta §5); senhas em `referencias/MCPVMWare/.mcp.json`; **commit (só com ordem)**.
- **Ondas:**

  | Onda | Estado |
  | --- | --- |
  | B0 a B6 | concluídas |
  | B7 | **concluída**, exceto o commit (aguarda ordem do owner) |

### 2026-10-23 19:28 — decisões finais do owner + commit
- **Decisões (via AskUserQuestion, 19:21):**
  - P-5 = só o projeto;
  - P-6 = `--max-prompt-chars` ≈ 128.000 caracteres (~48k tokens);
  - **PB6-1 = `PROTO-009` vira MUST**;
  - P-9 = `openWorldHint:false`.
- **Aplicado:** `60` (1.2.0) e `61` (1.2.0) agora com **105 MUST, 52 SHOULD, 14 COULD**; MUST por fase: Fase 1 = 44, Fase 3 = 38.
  - Conferido: 105 linhas MUST no `60` e no `61`; listas por fase com contagem declarada igual à contada; LF nos dois.
  - Também atualizados: `CFG-010` (default ≈ 128.000 caracteres; `65536` na lista de literais proibidos) e `TOOL-005` (`openWorldHint:false`).
- **Não deu certo, e a solução:**
  1. **1ª tentativa:** a função auxiliar se chamava `R`, que no PowerShell é alias de `Invoke-History` e tem precedência sobre a função. **Nenhuma substituição foi feita**; os dois arquivos foram regravados só com o `FileVersion` trocado para 1.2.0, com o conteúdo intacto (contagens continuaram 104/53/14) e em LF. Refeito com a função `Rep1`, que exige exatamente 1 ocorrência por troca.
  2. **1º `git commit`:** a here-string passada a `-F -` virou pathspec no PowerShell 5.1, e o commit falhou com o índice intacto. Refeito com a mensagem num arquivo UTF-8 (`-F <arquivo>`).
- **Pré-commit (lido):**
  - varredura de segredos sobre **os 52 arquivos** a commitar (e não só o brief): `known_hits=0`, 0 `Bearer` com valor;
  - 2 ocorrências de `BEGIN .*PRIVATE KEY`, que são o **texto da regex** (plano, linha 220, e `scan-segredos-brief.ps1:154`), não uma chave;
  - índice: 52 arquivos, 0 fora do conjunto aprovado, 0 de `referencias/` fora do brief, 0 `.mcp.json`/`.exe`/`.env`/`settings.local.json`.
- **Commit `0fba414`** ("docs: brief das referências e planos do mcpollama (fase de concepção)"): 52 arquivos, stage por caminho, sem push (não há remoto). Fora do commit, por decisão: `.claude/`, `.cursor/`, `.vscode/`.
- **Aviso registrado:** o `core.autocrlf` do Git global converte LF → CRLF no checkout. O índice guarda LF. Recomenda-se um `.gitattributes` (`* text=auto eol=lf`), já registrado no brief (lição de CRLF do truenas-mcp). **Não criado:** decisão do owner, a tratar no scaffold da Fase 1.
- **Este registro** foi feito depois do commit, então fica pendente para o próximo.
- **Ondas:** B0 a B7 **concluídas**. Brief encerrado.
