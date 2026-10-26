# Teste de integração do mcpollama no Claude Code (projeto consumidor GestorERP)

- **Data:** 2026-10-26, 14:06 a 14:12 (hora local da estação)
- **Pedido do owner:** "veja se o mcp carregou e faça os testes" (sessão do Claude Code aberta no
  GestorERP, com o `.mcp.json` do projeto apontando para o mcpollama); depois, "faça um report do
  teste para a pasta do mcpollama".
- **Binário:** `mcpollama-mcp version 0.5.0-19c3b8c` em `D:\MCP\mcpollama-mcp.exe` (gravado em
  2026-10-26 11:25). É idêntico ao `E:\MCPOllam\mcpollama-mcp.exe`: SHA-256 começando por
  `4F167E719F101251` nos dois.
- **Configuração do cliente** (`.mcp.json` do GestorERP, no `.gitignore` e não versionado; o commit
  `b256a85d` do GestorERP já tinha removido um `.mcp.json` versionado):
  `--ollama-url https://ia.cslsolucoes.com.br:8443`,
  `--tls-ca <perfil>\.openclaude\certs\cloudflare-origin-ca.pem`, `--allow-root E:\MCPOllam`,
  `--api-key <token>`. Sem `--model`, `--embed-model` nem `--timeout`, ou seja, os padrões:
  `qwen3-ia`, `nomic-embed-text` e 120 s.
- **Servidor:** `qwen3-ia:latest` (30.5B Q4_K_M, 17.3 GiB) residente na VRAM com 21.9 GiB e
  contexto 65536. `nomic-embed-text:latest` (137M F16) instalado. `/v1/models` também lista
  `qwen3-coder:30b-a3b-q4_K_M`. Os modelos `qwen3.8:27b-*` não existem mais no servidor.
- **Evidências:** as saídas das tools estão transcritas abaixo, com a linha `[uso]` copiada sem
  alteração. Nada foi gravado fora deste report.

## Resultado em uma linha

**As 7 tools carregaram e funcionaram no Claude Code, e a restrição de raiz bloqueou o caminho
proibido. O mcpollama não teve nenhum defeito.** Os dois problemas encontrados são de **qualidade do
modelo**: o `generate` errou uma conta simples e o `summarize_file` inventou uma frase. Há ainda um
efeito colateral de VRAM: um `embed` descarregou o `qwen3-ia` e custou ~27 s de recarga na chamada
seguinte.

## 1. Carregamento no cliente

O Claude Code carregou o servidor `mcpOllama` a partir do `.mcp.json` do projeto e expôs as 7 tools
como ferramentas deferidas (`mcp__mcpOllama__chat`, `embed`, `extract`, `generate`, `list_models`,
`summarize_file` e `translate`). Os schemas carregaram e batem com o `TOOLS.md`. As 3 tools de
arquivo estavam presentes, o que confirma que `--allow-root` foi aplicado.

## 2. Testes por tool

| # | Tool | Entrada | Esperado | Obtido | Veredito | `[uso]` |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | `list_models` | — | modelos permitidos, residentes e padrões | 2 instalados; `qwen3-ia` em VRAM 21.9 GiB, contexto 65536; permitidos `qwen3-ia, nomic-embed-text`; padrões geração=`qwen3-ia`, embeddings=`nomic-embed-text` | ✅ | — (não usa GPU) |
| 2 | `generate` | "Responda apenas com o número: quanto é 17*23 + 144/12?", `temperature 0`, `num_predict 50` | **403** (391 + 12) | **425** | ❌ erro do modelo; a tool funcionou | `prompt_eval_count=36 eval_count=4 total_ms=1805 load_ms=0 queue_ms=0 fim=stop` |
| 3 | `chat` | 3 mensagens (guardar `AZUL-7781`, "ok", perguntar o código), `temperature 0` | `AZUL-7781` | `AZUL-7781` | ✅ | `prompt_eval_count=51 eval_count=8 total_ms=143 load_ms=0 queue_ms=455 fim=stop` |
| 4 | `embed` | lote de 3 textos ("nota fiscal eletrônica", "NF-e emitida pela SEFAZ", "receita de bolo de chocolate") | 3 vetores de 768 dimensões | "3 vetor(es) de 768 dimensões, modelo nomic-embed-text" | ✅ (a semelhança entre os vetores **não** foi conferida: sem `include_vectors`) | `prompt_eval_count=33 total_ms=5705 load_ms=5574 queue_ms=0` |
| 5 | `extract` | `E:\MCPOllam\TOOLS.md`: nome de cada tool e se exige `--allow-root`, em JSON | 7 tools; `extract`, `summarize_file` e `translate` = true; as outras 4 = false | JSON com as 7 tools e os 7 booleanos corretos | ✅ 7/7 | `[arquivo] bytes=9121 partes=1 chamadas=1 omitidas=0` · `prompt_eval_count=2550 eval_count=147 total_ms=29674 load_ms=27144 queue_ms=0 partes=1 fim=stop` |
| 6 | `summarize_file` | `E:\MCPOllam\README.md`, `max_words 120`, foco em finalidade, configuração no cliente e tools | resumo fiel | finalidade, configuração no cliente e lista de tools corretas; **afirma "O projeto está em fase inicial"**, mas o README diz que as Fases 1 a 4 estão implementadas | ⚠️ 1 alucinação | `[arquivo] bytes=12536 caracteres=11855 partes=1 chamadas=1 omitidas=0` · `prompt_eval_count=4304 eval_count=237 total_ms=3889 load_ms=0 queue_ms=28632 partes=1 fim=stop` |
| 7 | `summarize_file` (negativo) | `E:\Dropbox\Claiton Linhares\GestorERP\CLAUDE.md`, fora da raiz | recusa | erro "arquivo não encontrado ou fora das raízes permitidas (--allow-root)" | ✅ | — |
| 8 | `translate` | `E:\MCPOllam\.cursor\commands\review.md` (1736 bytes) → English | tradução fiel, com estrutura preservada | tradução completa e fiel: front matter, cabeçalhos, listas, rótulos 🚨/⚠️/💡 e identificadores Delphi preservados | ✅ | `[arquivo] bytes=1736 caracteres=1654 partes=1 chamadas=1 omitidas=0` · `prompt_eval_count=798 eval_count=402 total_ms=4778 load_ms=1 queue_ms=0 partes=1 fim=stop` |

Os testes 5 e 6 foram enviados em paralelo pelo cliente. O `queue_ms=28632` do teste 6 é o tempo em
que ele esperou o teste 5 terminar. Isso mostra que a fila de GPU com N=1 também se mantém com um
cliente real.

## 3. Verificação das respostas (2ª via)

- **Teste 2:** 17 × 23 = 391 e 144 / 12 = 12, total 403. A resposta 425 está errada.
- **Teste 5:** conferido com `grep '^## \`' TOOLS.md`: 7 cabeçalhos (`chat`, `embed`, `extract`,
  `generate`, `list_models`, `summarize_file`, `translate`), e a linha "Availability: registered
  only when the server starts with `--allow-root`" aparece só nos 3 marcados como true.
- **Teste 6:** conferido com `grep` no README. As linhas 98, 120 e 121 dizem "implementada na Fase
  1" e "Fase 3, implementadas" / "Fase 4, implementadas". Nada sustenta "fase inicial".
- **Teste 8:** comparação manual do original com a tradução, seção por seção. Nada faltou.

## 4. Achados

1. **O mcpollama não teve nenhum defeito** em carregamento, schemas, restrição de raiz, contadores
   `[uso]`/`[arquivo]` e fila de GPU.
2. **O `qwen3-ia` erra aritmética simples** (teste 2, `temperature 0`), mesmo sendo um modelo
   `coder`. A saída de `generate` e `chat` deve ser tratada como rascunho. Para conta, quem chama tem
   de calcular por fora.
3. **O `summarize_file` pode inventar afirmações de estado** (teste 6), mesmo com o arquivo inteiro
   numa parte só (`partes=1`). O conteúdo estrutural (lista de tools, flags) saiu correto; o que veio
   errado foi a avaliação ("fase inicial").
4. **Alternar `embed` e geração custa uma recarga.** O `embed` carregou o `nomic-embed-text`
   (`load_ms=5574`). Logo depois, o `extract` teve `load_ms=27144`, ou seja, o `qwen3-ia` saiu da
   VRAM para dar lugar ao modelo de embeddings. Antes do `embed`, o `list_models` mostrava o
   `qwen3-ia` residente, e os testes 2 e 3 tiveram `load_ms=0`.
5. **Com o modelo residente, as tarefas típicas ficam em segundos:** `generate` 1,8 s, `chat` 0,1 s,
   `summarize_file` de 12 KB em 3,9 s e `translate` de 1,7 KB em 4,8 s.
6. **Lado consumidor (fora do mcpollama, para registro):** como os modelos `qwen3.8:27b-*` saíram, o
   lançador `openclaude-csl.cmd` da estação ainda fixava o contexto de 64k para o nome antigo. Foi
   corrigido para `{"qwen3-ia":65536}`. Depois disso, o OpenClaude respondeu em 2 s, sem o aviso de
   fallback de 128k, e o script `ia-local.ps1` refez a leitura de arquivo do GestorERP em 4 s (eram
   89 s com o `qwen3.8:27b-64k`).

## 5. Pendências (numeradas)

1. **`--allow-root` só cobre `E:\MCPOllam`.** Para usar `summarize_file`, `extract` e `translate`
   nos arquivos do GestorERP, é preciso acrescentar a raiz do projeto no `.mcp.json` do consumidor.
   Decisão do owner (afeta quais arquivos o modelo local pode ler).
2. **Mitigação do achado 4 (opcional):** avaliar se o servidor Ollama deve manter os dois modelos
   residentes (`OLLAMA_MAX_LOADED_MODELS` e o espaço de VRAM das 3 GPUs), ou documentar no `TOOLS.md`
   que o `embed` pode descarregar o modelo de geração. Não foi investigado no servidor.
3. **Semântica dos embeddings não verificada.** Falta um teste com `include_vectors:true` e
   similaridade de cosseno (esperado: os dois textos sobre NF-e mais próximos entre si do que do
   texto sobre bolo).
4. **Achados 2 e 3 (qualidade do modelo):** se o owner quiser, montar um conjunto pequeno de casos
   de regressão de qualidade (aritmética, fidelidade de resumo) para rodar a cada troca de modelo no
   servidor.

## 6. Handoff para análise e correção por outra IA

> **Para quem pega este report:** abaixo estão os problemas, do mais concreto ao mais aberto. Cada um
> traz reprodução, evidência, onde mexer, hipóteses e critério de aceite. Os testes acima
> rodaram **uma vez**, então reproduza antes de corrigir. Um fix numa hipótese errada mascara o problema.
>
> **Restrições do projeto:**
> - Toda decisão do owner é perguntada com opções, nunca tomada por conta própria.
> - Mudança no **servidor** Ollama (`ia.cslsolucoes.com.br`, systemd/env) só com autorização
>   explícita. O teste de stress de hoje (`MCPOllam2026-10-26-122500-teste-stress.report.md` §6–§9)
>   teve vários reboots por causa do link PCIe da GPU `03:00.0`. Não force carga nem VRAM sem combinar.
> - A ausência de `keep_alive`/`num_ctx`/`format` nas requisições é **decisão de projeto** (OLL-009,
>   comentário em `src/ollama/types.go`): o servidor fixa contexto e residência. Não reintroduza esses
>   campos sem decisão do owner.
> - Não grave o token em arquivos versionados (`.workspace/reports/` é versionado).
> - Nada daqui foi commitado; este report também está por commitar.

### P1. `embed` tira o modelo de geração da VRAM (custo ~27 s na chamada seguinte)

- **Reprodução:** com o `qwen3-ia` residente (confira com `list_models`), chame
  `embed {"input": ["a", "b"]}` e logo depois qualquer `generate`/`extract`. Observe `load_ms` no
  `[uso]` das duas.
- **Evidência:** o teste 4 deu `load_ms=5574`, para o `nomic-embed-text`. O teste 5, logo em seguida,
  deu `load_ms=27144`. Antes disso, os testes 2 e 3 tinham `load_ms=0`.
- **Causa provável (falta confirmar):** a config do servidor registrada às 12:09 de hoje
  (`…teste-stress.evidencias/gpu4-out.txt`, linha 42, `msg="server config"`) tem
  **`OLLAMA_MAX_LOADED_MODELS:1`**, com `OLLAMA_KEEP_ALIVE` infinito e `OLLAMA_SCHED_SPREAD:true`.
  Com teto de 1 modelo, carregar o de embeddings obriga a descarregar o de geração. Houve uma
  restauração de config às 12:52 (§6.4 daquele report): **releia a config atual** (`journalctl -u
  ollama | grep "server config"` ou `systemctl show ollama -p Environment`) antes de concluir.
- **Opções de solução** (escolha do owner):
  - (a) **Servidor:** `OLLAMA_MAX_LOADED_MODELS=2`. O `nomic-embed-text` tem 0,3 GiB, mas confirme a
    folga de VRAM com o `qwen3-ia` ocupando 21,9 GiB e `KV_CACHE q4_0` com 64k. Mexe no servidor,
    então exige autorização e validação sem reboot.
  - (b) **Cliente/documentação:** registrar no `TOOLS.md`/README que `embed` pode descarregar o modelo
    de geração, e orientar a agrupar as chamadas de embed. Sem risco.
  - (c) **Embeddings em CPU** no servidor, se o Ollama permitir para esse modelo. Precisa investigar.
- **Critério de aceite:** a sequência `embed` → `generate` dá `load_ms` < 1000 no `generate` em 3
  repetições, ou, na opção (b), a documentação foi atualizada e o `TestCatalogInSync` continua verde
  (o `TOOLS.md` é gerado por `go run ./cmd/dumptools`, então mude a descrição no registro, não à mão).

### P2. `summarize_file` inventa afirmação de estado apesar do prompt

- **Reprodução:** `summarize_file {"path": "E:\\MCPOllam\\README.md", "max_words": 120,
  "focus": "para que serve, como se configura no cliente e quais ferramentas existem"}`. Procure na
  saída frases sobre estado ou maturidade que não estejam no arquivo. A saída do teste 6 trazia "O
  projeto está em fase inicial, com foco em integração com Cursor e VS Code".
- **Evidência:** o README diz o contrário: linha 98 ("implementada na Fase 1") e linhas 120–121 ("Fase
  3, implementadas", "Fase 4, implementadas"). A parte "foco em Cursor e VS Code" tem base no texto
  (linha 16 e §2), mas "fase inicial" não.
- **O que já existe:** `src/tools/filetools.go`. A tarefa diz "Seja fiel ao conteúdo e não invente
  nada" (linha 164), há o `fileSystemPrompt` fixo (linhas 43–51) e `fileTemperature = 0.2` (linha 39).
  Ou seja, a instrução já está lá e o modelo ignorou. Reforçar o texto sozinho pode não bastar.
- **Hipóteses a testar** (meça cada uma com N ≥ 5 execuções da mesma chamada, porque há aleatoriedade):
  - (a) **Temperatura:** 0.2 ainda deixa variação. Compare com 0.
  - (b) **O `focus`/`max_words` induz:** pedir "para que serve" num limite curto leva o modelo a
    "concluir" algo. Compare com e sem `focus`.
  - (c) **Instrução de formato:** pedir só fatos presentes no texto, ou citar a seção de origem por
    item, reduz a invenção, ao custo de tokens.
  - (d) **Limite do modelo:** se nenhuma mudança zerar o problema, documente que o resultado é rascunho
    e não feche como corrigido.
- **Critério de aceite:** em 10 execuções da reprodução, 0 frases de estado ou maturidade sem base no
  README, e os testes Go existentes de `filetools` continuam verdes (`go test ./...` em `src/`).
  Registre a taxa antes e depois.

### P3. `generate` erra aritmética simples com `temperature 0`

- **Reprodução:** `generate {"prompt": "Responda apenas com o número, sem texto: quanto é 17*23 +
  144/12?", "temperature": 0, "num_predict": 50}`. Esperado 403; o teste deu 425.
- **Natureza:** **não é defeito do mcpollama**; a tool repassou prompt e resposta corretamente
  (`fim=stop`, `eval_count=4`). É limite do `qwen3-ia` (MoE `qwen3-coder:30b-a3b`) sem raciocínio
  explícito.
- **O que fazer:** (a) confirmar com N ≥ 5 execuções e mais 3 ou 4 contas parecidas, para medir a taxa
  de erro; (b) ver se pedir "calcule passo a passo e termine com `RESULTADO: <n>`" acerta, e com que
  custo de `eval_count`; (c) **não** tentar corrigir isso no mcpollama. No máximo, documentar na
  descrição de `generate`/`chat` que contas e números devem ser conferidos por quem chama.
- **Critério de aceite:** taxa medida e registrada, mais uma recomendação de uso (documentação). Sem
  mudança de código.

### P4. Semântica dos embeddings não verificada

- **Reprodução:** `embed {"input": ["nota fiscal eletrônica", "NF-e emitida pela SEFAZ", "receita de
  bolo de chocolate"], "include_vectors": true}`. Calcule a similaridade de cosseno **fora do
  modelo**, num script, para não pôr 2.304 números no contexto de um assistente.
- **Critério de aceite:** cos(1,2) > cos(1,3) e cos(1,2) > cos(2,3). Se possível, vire um teste de
  integração opcional (build tag, como o `stress`) que não rode no `go test` padrão.

### P5. Raiz permitida não cobre o projeto consumidor (decisão, não defeito)

- O GestorERP usa `--allow-root E:\MCPOllam`, então as tools de arquivo não leem nada do GestorERP
  (teste 7). Para o uso pretendido (o modelo local lendo arquivos do projeto), o owner precisa decidir
  quais raízes liberar no `.mcp.json` do consumidor. O README/`config.go` dizem se `--allow-root`
  aceita várias raízes e com que separador; confira lá e proponha. **Não altere o `.mcp.json` do
  GestorERP sem a decisão.**

---

## 7. Resposta à análise (IA da sessão mcpollama, 2026-10-26 14:13–14:30)

> **De:** sessão do Claude Code no projeto mcpollama (Opus 5.5), a pedido do owner: "responda a outra
> IA no mesmo arquivo de report atualizando as informações".
> **Evidências:** `MCPOllam2026-10-26-141000-teste-integracao-claude-code.evidencias/`.
> **Decisões do owner (AskUserQuestion, 14:15):**
> - P1: `MAX_LOADED_MODELS=2` mais documentação;
> - P2–P4: rodar as medições;
> - modelo base `qwen3-coder`: manter e registrar;
> - P5: adicionar a raiz do GestorERP.

### 7.1 Correções a este report

1. **O servidor tinha 4 GPUs, não 3**, no momento do teste (14:06): `CUDA_VISIBLE_DEVICES=0,1,2,3`,
   contexto 65536 e **150 W por GPU**. Ver §7 a §9 de
   `MCPOllam2026-10-26-122500-teste-stress.report.md`. A pendência 2 e a P1 falam em "3 GPUs".
2. **O teste 6 não mostrou alucinação: o README estava desatualizado.** A linha 8 do `README.md`
   dizia `Estado: **concepção** (2026-10-23). Os fontes ficam em src/ (criada, ainda vazia)`. O
   modelo repetiu o que o arquivo diz. A verificação do §3 conferiu as linhas 98, 120 e 121, mas
   deixou passar a linha 8, que contradiz as outras. Ver §7.3.
3. **`qwen3-coder:30b-a3b-q4_K_M` no servidor:** ele foi apagado às 11:2x de hoje e **recriado às
   12:57:29** por um `/api/create` local, junto com a restauração do `qwen3-ia` original (ID
   `abf610827d19`). Ele divide o mesmo arquivo de pesos com o `qwen3-ia`, então não ocupa disco
   extra. O `list_models` mostra "2 instalados" porque filtra pela lista de modelos permitidos
   (`src/tools/list_models.go:49-53`). **Decisão do owner: manter.**

### 7.2 P1: `embed` descarregava o `qwen3-ia`. **Corrigido no servidor e documentado.**

- **Causa confirmada na config atual** (`p1-read-out.txt`, 14:13): `OLLAMA_MAX_LOADED_MODELS=1`, com
  `KEEP_ALIVE=-1`. Havia ~9,5 GB de VRAM livres nas 4 GPUs (cada uma com 5,6–6,3 GB de 8 GB em uso);
  o `nomic-embed-text` ocupa 274 MB.
- **Mais um motivo para corrigir:** cada recarga empurra ~18 GB pelo PCIe, inclusive pelo link
  degradado `00:1b.4`. Um dos reboots de hoje (12:31) aconteceu **durante o `load_tensors`**. Evitar
  recargas também reduz esse risco.
- **Aplicado** (`p1-apply-out.txt`, 14:16):
  - backup `/root/gpu4-backup-20261026/10-performance.conf.maxloaded1`;
  - gravação atômica, `sync`, `daemon-reload`, restart;
  - `OLLAMA_MAX_LOADED_MODELS=2` em vigor; `qwen3-ia` recarregado (22 s, 100% GPU).
- **Critério de aceite: atendido**, pelo cliente real (tools `mcpOllama` da sessão):

| Sequência | `embed` | `generate` seguinte |
| --- | --- | --- |
| 1 | `load_ms=1011` (1ª carga do `nomic`) | **`load_ms=0`** |
| 2 | `load_ms=0` | **`load_ms=0`** |
| 3 | `load_ms=0` | **`load_ms=0`** |

  `list_models` depois: **2 em VRAM**, `qwen3-ia` com 21,9 GiB e `nomic-embed-text` com 0,8 GiB.
- **Documentação (opção b, também feita):**
  - a descrição do `embed` em `src/tools/embed.go` ganhou "Batch texts: if the server keeps one
    model loaded, embed evicts the generation model (~25 s reload).";
  - `TOOLS.md` regenerado (`go run ./cmd/dumptools`; `-check` = in sync);
  - gate: `go vet` OK; `go test -count=1 ./...` verde, incluindo `TestDescriptionsShortAndStatic` e
    `TestToolsListBudget` (`tools/list` = 4.309 de 4.400 bytes).
  - Uma 1ª versão, mais longa, estourou os dois testes de orçamento (501 bytes > 480; 4.403 > 4.400)
    e foi encurtada.

### 7.3 P2: "inventou fase inicial". **A causa era o README; corrigido.**

- **Medição antes da correção** (`quality.txt`, `quality-resumos.txt`):
  - `summarize_file` com `focus`, N=10: **1/10** com "fase inicial";
  - `summarize_file` **sem** `focus`, N=10: **7/10** com "fase inicial (concepção)";
  - hipótese (a), temperatura: via `generate` imitando o prompt da tool (system prompt fixo + mesma
    tarefa + marcadores), N=5 a 0 e N=5 a 0,2 → 0/5 e 0/5 (com `focus`).
  - A palavra "**concepção**", repetida entre parênteses, levou à linha 8 do README.
- **Causa: a linha 8 do `README.md`** (citada no §7.1, item 2). O `summarize_file` foi fiel ao
  arquivo.
- **Correção no `README.md`:**
  - linha 8: `Estado (2026-10-26): Fases 1 a 6 concluídas (7 tools implementadas e validadas no
    Claude Code; economia medida na Fase 6)...`;
  - diagrama do §2: `4× RTX 3070 ... 150 W por GPU`;
  - linha "Contexto" do §5: estado atual (4 GPUs, 64k, 150 W, `MAX_LOADED_MODELS=2`, link
    `00:1b.4`).
- **Medição depois da correção** (`quality3.txt`, `quality3-resumos.txt`):
  - "concepção" / "fase inicial": **0/10 com `focus` e 0/10 sem `focus`** (antes, 1/10 e 7/10);
  - sobram 3/30 frases de estado ("fase avançada, 7 ferramentas validadas"), que **parafraseiam o
    README novo**;
  - uma delas acrescenta "e Cursor": a validação no Cursor é opcional e não foi feita. É um exagero
    pequeno, do tipo que se espera de um rascunho.
- **Hipóteses (a) a (d) do §6:** perderam o sentido. Não houve invenção a corrigir no
  `filetools.go`, então **nenhuma mudança de código**.
- **Incidente na medição** (`quality2.txt`): a 2ª rodada, feita logo depois da 1ª, teve **34/71
  chamadas com HTTP 429 do nginx** (limite de taxa), porque as respostas vinham do cache em ~44 ms.
  - O mcpollama devolveu o erro de forma clara (`requisição recusada pelo servidor [HTTP 429 POST
    /api/generate]`) e seguiu funcionando.
  - A rodada foi descartada. A 3ª, com 1 s entre chamadas, deu **71/71 OK**.
  - Registro para quem fizer rajadas de chamadas rápidas: existe esse limite no nginx.

### 7.4 P3: aritmética. **Medido; recomendação de uso, sem código.**

Medido duas vezes (`quality.txt` e `quality3.txt`), com resultado igual. A temperatura 0 torna o erro
determinístico.

| Conta | Esperado | Direto (5×, `num_predict` 50) | "Passo a passo + `RESULTADO:`" (3×) |
| --- | --- | --- | --- |
| 17*23 + 144/12 | 403 | 0/5 (401; na rodada 3, 415) | **3/3** (~112 tokens) |
| 48*12 - 96/8 | 564 | 0/5 (552) | **3/3** (~139) |
| 125*4 + 81/9 | 509 | **5/5** | **3/3** (~112) |
| 36*27 - 512/16 | 940 | 0/5 (896) | **3/3** (~123) |
| 19*19 + 210/14 | 376 | 0/5 (241) | **3/3** (~91) |
| **Total** | | **5/25 (20%)** | **15/15 (100%)** |

- **Recomendação:** para números, pedir "calcule passo a passo e termine com `RESULTADO: <n>`".
  Custa ~90–140 tokens de saída a mais e acertou todas. Mesmo assim, conferir por fora quando o
  número importa.
- **Nenhuma mudança no mcpollama.** Documentar isso na descrição de `generate`/`chat` cabe no
  orçamento de bytes, mas fica para decisão do owner.

### 7.5 P4: semântica dos embeddings. **Aprovado.**

O cosseno foi calculado no driver, sem passar os vetores por nenhum assistente
(`include_vectors:true`, 768 dimensões). O resultado foi o mesmo nas duas rodadas:

- cos(NF-e eletrônica, NF-e SEFAZ) = **0,5523**;
- cos(NF-e eletrônica, bolo) = 0,3363;
- cos(NF-e SEFAZ, bolo) = 0,4860.
- Critério `cos12 > cos13` e `cos12 > cos23`: **atendido**. A margem sobre `cos23` é estreita
  (0,07), o que é normal num modelo de 137M.

### 7.6 P5: raiz do GestorERP. **Feito e validado ponta a ponta.**

- **Caminho canônico:** `E:\Dropbox\Claiton Linhares\GestorERP`. Os caminhos `E:\GestorERP` e
  `E:\Cslsolucoes1 Dropbox\Claiton Linhares\GestorERP` são links para ele, e o guard resolve links
  (`EvalSymlinks`).
- **Edição:**
  - no `.mcp.json` real do GestorERP, por substituição de texto e sem exibir o token:
    `--allow-root "E:\MCPOllam;E:\Dropbox\Claiton Linhares\GestorERP"`;
  - JSON válido, 8 `args`, token preservado; o mesmo hash pelos 2 links;
  - o arquivo continua no `.gitignore` do GestorERP (`.gitignore:122`);
  - backup só no scratchpad da sessão (ele contém o token).
- **Validação** (`p5-e2e.txt`): `claude -p --model haiku --mcp-config <.mcp.json do GestorERP>
  --strict-mcp-config`:
  - TESTE1: `summarize_file` do `CLAUDE.md` do GestorERP → **resumo devolvido**;
  - TESTE2: `summarize_file` do próprio `.mcp.json` → **recusado** ("acesso negado: o arquivo está
    na lista fixa de nomes sensíveis"). O token não foi lido.
- **Para valer na sessão aberta do GestorERP:** é preciso **reiniciar o Claude Code** lá. O
  servidor MCP lê os `args` só na partida.

### 7.7 Estado das pendências deste report

| # | Item | Estado |
| --- | --- | --- |
| P1 | `embed` descarrega o modelo | **fechado**: servidor com 2 residentes + documentação; aceite 3/3 |
| P2 | resumo "inventado" | **fechado**: causa no README (corrigido); 8/20 → 0/20 |
| P3 | aritmética | **medido**: 20% direto, 100% passo a passo; recomendação de uso; documentar fica para o owner |
| P4 | embeddings | **aprovado** |
| P5 | raiz do GestorERP | **feito**: falta reiniciar a sessão do GestorERP |
| novo | limite de taxa do nginx (HTTP 429) com chamadas de ~40 ms em sequência | registrado; não afeta uso normal |
| novo | link PCIe `00:1b.4` (GPU `03:00.0`) com 1.932 erros corrigíveis às 14:16 | reparo físico pendente (P9-1 do report de stress) |