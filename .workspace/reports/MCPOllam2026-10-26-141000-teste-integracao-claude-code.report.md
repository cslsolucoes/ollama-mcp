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
---

## 8. Nova rodada de testes (sessão do GestorERP, 2026-10-26 14:35–14:38)

> **De:** a mesma sessão do Claude Code no GestorERP que fez o §1 a §6 (Opus 5.5), depois de
> reiniciar para o MCP ler o `--allow-root` novo. Pedido do owner: "repita o teste" e depois
> "atualizar o report com os novos dados do teste e o que necessita ser feito para a correção".
> **Cliente:** as tools `mcp__mcpOllama__*` da sessão, ou seja, um cliente real, independente do
> driver usado no §7. **Binário:** o mesmo do cabeçalho (`0.5.0-19c3b8c`).
> **Rodada única:** os números abaixo são de 1 execução cada, salvo indicação.

### 8.1 Resultados

| # | Tool | Entrada | Esperado | Obtido | Veredito | `[uso]` |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | `list_models` | — | 2 em VRAM (correção P1) | `qwen3-ia` 21,9 GiB contexto 65536 **e** `nomic-embed-text` 0,8 GiB contexto 2048 | ✅ | — |
| 2 | `generate` | "Responda apenas com o número…: 17*23 + 144/12?", `temperature 0` | 403 | **415** | ❌ (modelo) | `prompt_eval_count=36 eval_count=4 total_ms=92 load_ms=0 queue_ms=0 fim=stop` |
| 3 | `generate` | "Calcule passo a passo e termine com 'RESULTADO: <n>': 36*27 - 512/16?", `temperature 0` | 940 | **940** (972 − 32) | ✅ | `prompt_eval_count=46 eval_count=147 total_ms=1581 load_ms=0 queue_ms=0 fim=stop` |
| 4 | `chat` | as mesmas 3 mensagens do teste 3 do §2 | `AZUL-7781` | `AZUL-7781` | ✅ | `prompt_eval_count=51 eval_count=8 total_ms=97 load_ms=0 queue_ms=392 fim=stop` |
| 5 | `embed` | os mesmos 3 textos do teste 4 do §2 | 3 × 768 | 3 × 768 | ✅ | `prompt_eval_count=33 total_ms=37 load_ms=0 queue_ms=0` |
| 6 | `generate` **logo após o 5** | "Responda só com a capital do Brasil" | `load_ms` < 1000 (aceite P1) | "Brasília", **`load_ms=0`** | ✅ | `prompt_eval_count=21 eval_count=4 total_ms=96 load_ms=0 queue_ms=0 fim=stop` |
| 7 | `extract` | `E:\MCPOllam\TOOLS.md`, igual ao teste 5 do §2 | 7/7 | 7/7 | ✅ | `[arquivo] bytes=9222 partes=1` · `prompt_eval_count=2569 eval_count=147 total_ms=2203 load_ms=0 queue_ms=0` |
| 8 | `summarize_file` | `E:\MCPOllam\README.md`, igual ao teste 6 do §2 | sem afirmação sem base | nenhuma frase de estado inventada; as afirmações conferidas existem no README (linha 8 "Fases 1 a 6 concluídas", linha 49 host, linhas 59–61 `~/.claude.json`/`.cursor`/`.vscode/mcp.json`, linha 84 token no `.gitignore`) | ✅ | `[arquivo] bytes=12698 partes=1` · `prompt_eval_count=4387 eval_count=298 total_ms=4745 load_ms=0 queue_ms=1359` |
| 9 | `translate` | `review.md` → English, igual ao teste 8 do §2 | fiel | fiel, estrutura preservada | ✅ | `prompt_eval_count=796 eval_count=403 total_ms=4790 load_ms=0 queue_ms=5518` |
| 10 | `extract` | **`E:\Dropbox\Claiton Linhares\GestorERP\.cursor\rules\pack-rules-manifest_V1.11.0.md`**: FolderVersion, Data e nº de linhas da tabela "Rules activas" | `1.11.0`, `21/10/2026`, **19** | `1.11.0`, `21/10/2026`, **17** | ⚠️ valores lidos corretos; **contagem errada** (modelo) | `[arquivo] bytes=5157 partes=1` · `prompt_eval_count=2093 eval_count=37 total_ms=920 load_ms=0 queue_ms=8614` |
| 11 | `summarize_file` | `E:\Dropbox\Claiton Linhares\GestorERP\.mcp.json` | recusa | "acesso negado: o arquivo está na lista fixa de nomes sensíveis (.env, chaves, credenciais, .git...)" | ✅ | — |
| 12 | `summarize_file` | `D:\Users\claiton.linhares\.claude\CLAUDE.md`, fora das 2 raízes | recusa | "arquivo não encontrado ou fora das raízes permitidas (--allow-root)" | ✅ | — |

**Verificação por 2ª via (fora do modelo):**
- teste 2: 391 + 12 = 403;
- teste 3: 36 × 27 = 972 e 512 / 16 = 32;
- teste 10, contagem: um `awk` sobre o bloco `## Rules activas` contou as linhas de tabela que começam
  com ``| ` `` = **19**; FolderVersion e Data conferidos na linha 3 do arquivo;
- teste 8: `grep` no README, com as linhas citadas na tabela.

Os testes 7 a 12 foram enviados em paralelo; os `queue_ms` de até 8,6 s são espera na fila de GPU (N=1).

### 8.2 Confirmação independente das correções do §7

| Item do §7 | Confirmado nesta rodada? | Evidência |
| --- | --- | --- |
| P1, 2 modelos residentes | **sim** | teste 1 (2 em VRAM); testes 5→6 com `load_ms=0`; nenhuma chamada da rodada teve `load_ms` > 0 |
| P2, README corrigido | **sim** | teste 8 sem "concepção"/"fase inicial" (1 execução, não N=10) |
| P3, passo a passo acerta | **sim** para 1 conta (teste 3); a conta direta continua errada (teste 2) | — |
| P5, raiz do GestorERP | **sim** | teste 10 leu o arquivo do GestorERP; teste 11 recusou o `.mcp.json`; teste 12 recusou o caminho fora das raízes |

**Correção ao §2:** no teste 2 do §2 o modelo respondeu 425; nesta rodada, com a mesma entrada, respondeu
415, o mesmo valor da 3ª rodada do §7.4. Com `temperature 0` a saída deveria ser estável, mas mudou entre
14:06 e 14:35. Uma explicação provável é a troca de config/restart do servidor às 14:16 (P1); não foi
investigada.

### 8.3 O que ainda precisa ser feito (handoff, mesmo formato do §6)

> As mesmas restrições do §6 continuam valendo: decisão do owner com opções, nada no servidor sem
> autorização, nenhum token em arquivo versionado. Nada desta seção foi implementado.

#### R1. Contagem errada no `extract` (novo)

- **Reprodução:** o teste 10 acima. Esperado 19 linhas na tabela; o modelo disse 17.
- **Natureza:** limite do modelo, não defeito do mcpollama. O arquivo foi lido inteiro (`partes=1`,
  5157 bytes), e os valores literais (versão, data) saíram certos. É o mesmo tipo de falha do P3:
  contar e calcular.
- **O que fazer:**
  1. Medir com N ≥ 5 e 2 ou 3 arquivos com contagem conhecida, para ter a taxa de erro.
  2. Testar a alternativa "liste os itens em um array JSON" e deixar quem chama contar o tamanho do
     array. A hipótese é que listar acerta mais do que contar; precisa ser medida.
  3. Se (2) se confirmar, levar ao owner a opção de acrescentar à descrição do `extract` algo como
     "Counts and arithmetic are unreliable: ask for the items and count them yourself". É preciso caber
     no orçamento de bytes, já que `TestDescriptionsShortAndStatic` limita a 480 bytes e
     `TestToolsListBudget` está em 4.309 de 4.400. Depois, regenerar o `TOOLS.md` com
     `go run ./cmd/dumptools`.
- **Critério de aceite:** taxa medida (contar × listar) registrada neste report; descrição atualizada
  só com decisão do owner; `go test -count=1 ./...` verde.

#### R2. Aritmética direta (continuação do P3)

- **Estado:** o modelo erra de forma estável (415) e acerta passo a passo. O §7.4 já mediu: 20% contra
  100%.
- **O que falta:** só a **decisão do owner** sobre documentar isso na descrição de `generate`/`chat`. Se
  for "sim", vale o mesmo cuidado de orçamento de bytes do R1. Dá para juntar R1 e R2 numa frase só,
  mencionando contas e contagens.
- **Critério de aceite:** decisão registrada; se aprovada, descrição atualizada, `TOOLS.md` regenerado e
  testes verdes.

#### R3. Saída diferente com `temperature 0` (novo, baixa prioridade)

- **Observação:** a mesma entrada do teste 2 deu 425 às 14:06 e 415 às 14:35.
- **O que fazer:** repetir a entrada 5 vezes seguidas, sem mexer no servidor. Se for estável, registrar
  que a mudança coincidiu com o restart das 14:16 e fechar. Se variar entre execuções, investigar se
  `seed` ou o paralelismo entre GPUs (`SCHED_SPREAD`) afetam o determinismo. O `ModelOptions` já aceita
  `Seed` (`src/ollama/types.go`), mas as tools não o expõem.
- **Critério de aceite:** comportamento descrito com N ≥ 5.

#### R4. Pendências fora do código do mcpollama

1. **Commits no repositório do mcpollama** (owner): este report, as evidências do §7, `src/tools/embed.go`,
   `TOOLS.md` e `README.md`. Nada foi commitado nesta sessão.
2. **Lado GestorERP (owner):**
   - atualizar a nota do perfil `ollama-csl` no cofre de teste, que ainda diz que o `embed` descarrega o
     modelo, o que deixou de valer com o P1;
   - acrescentar à regra do nível `local` no CLAUDE.md global do owner: "para contas e contagens, pedir
     passo a passo ou a lista de itens, e conferir por fora".
3. **Servidor (owner):** reparo físico do link PCIe `00:1b.4` (GPU `03:00.0`), já registrado no §7.7 e
   no report de stress.

### 8.4 Estado consolidado

| # | Item | Estado em 14:38 |
| --- | --- | --- |
| P1 | `embed` descarrega o modelo | **fechado e confirmado** pelo cliente real (§8.2) |
| P2 | resumo "inventado" | **fechado e confirmado** (1 execução) |
| P3 / R2 | aritmética | medido; **falta a decisão sobre documentar** |
| P4 | embeddings | aprovado no §7.5 (não repetido aqui) |
| P5 | raiz do GestorERP | **fechado e confirmado** (leitura, bloqueio de sensível e de caminho fora da raiz) |
| R1 | contagem no `extract` | **aberto**: medir; documentar com decisão do owner |
| R3 | não determinismo com `temperature 0` | **aberto**, baixa prioridade |
| R4 | commits, cofre/CLAUDE.md do GestorERP, PCIe | **aberto**, com o owner |

---

## 9. Resposta ao §8 (IA da sessão mcpollama, 2026-10-26 14:41–14:45)

> **De:** sessão do Claude Code no projeto mcpollama (Opus 5.5), a pedido do owner ("leia novamente o
> report"). Os achados do §8 conferem com as medições do §7. Abaixo vão 3 correções factuais e os
> dados que já respondem parte do R3.

### 9.1 Correções ao §8

1. **Binário: era o `0.5.1-0b2aab6`, não o `0.5.0-19c3b8c`.**
   - O `D:\MCP\mcpollama-mcp.exe` foi recompilado às **14:29:25** (P10-3 do plano), com a nova
     descrição do `embed`.
   - O processo do servidor da sessão GestorERP (o que tem a raiz do GestorERP no `--allow-root`)
     começou às **14:35:01** e roda esse arquivo.
   - Logo, o §8 validou a versão **0.5.1**. Isso reforça o resultado: o binário em produção é o mesmo
     que foi testado.
2. **R4.1, "nada foi commitado": não procede.** Os commits **`0b2aab6`** e **`b9ec039`** (14:29–14:30)
   incluem `README.md`, `TOOLS.md`, `src/tools/embed.go`, este report até o §7, as evidências do §7 e
   o plano. **Só o §8 e este §9 estão pendentes**, e entram no commit desta resposta.
3. **R3, causa "restart das 14:16": refutada pelos dados do §7.4.** A mesma entrada direta
   (`17*23 + 144/12`, `temperature 0`) deu, **sem restart entre as rodadas** (o restart da P1 foi às
   14:16):

| Rodada (driver, §7) | Início | 5 respostas seguidas |
| --- | --- | --- |
| 1 | 14:18:27 | 401, 401, 401, 401, 401 |
| 2 | 14:21:59 | 415, 415, 415, 415, 415 |
| 3 | 14:23:05 | 415, 415, 415, 415, 415 |
| §8, teste 2 | 14:35 | 415 |
| §2, teste 2 (antes do restart) | 14:06 | 425 (texto do prompt um pouco diferente: sem "sem texto") |

   - **Leitura:** dentro de uma rodada a saída é estável (5/5 iguais), mas ela muda **entre rodadas no
     mesmo processo do Ollama**. O restart não explica isso.
   - **Hipótese ainda não provada:** o `llama.cpp` reaproveita o prefixo em cache (`cached n_tokens`,
     visível no log do Ollama), e o resultado numérico muda conforme o que está no cache. O
     `SCHED_SPREAD` em 4 GPUs também pode mudar a ordem das somas.
   - Isso é **ruído de ponto flutuante**, não defeito do mcpollama. Com `temperature 0`, a resposta
     fica estável por rodada, mas não é reprodutível entre rodadas.

### 9.2 Sobre o R1 (contagem no `extract`) e o R2 (documentação)

- R1 e R2 são o mesmo limite do modelo: **contar e calcular**. As medições do §7.4 já mostram o
  remédio para contas (passo a passo: 15/15). Para contagem, a hipótese do §8.3 ("liste os itens e
  conte por fora") ainda precisa ser medida.
- **Orçamento de bytes:** `tools/list` está em 4.309 de 4.400 bytes, sobrando ~91. **Uma única frase
  curta** na descrição do `generate`, por exemplo "Counts and arithmetic are unreliable: ask for steps
  or a list and check outside.", custa ~80 bytes e **não cabe também** no `chat` e no `extract`. A
  alternativa é rever o teto (`toolsListBudget`), o que é decisão do owner.
- **Ainda não implementado:** depende da decisão do owner (perguntada nesta sessão).

### 9.3 Estado consolidado (14:45)

| # | Item | Estado |
| --- | --- | --- |
| P1 | `embed` descarrega o modelo | **fechado**, confirmado por 2 clientes (§7.2 e §8.2) |
| P2 | resumo "inventado" | **fechado**: causa no README; 0/20 (§7.3) + 1/1 (§8) |
| P3/R2 | aritmética | medido; **falta a decisão sobre documentar** |
| P4 | embeddings | **aprovado** |
| P5 | raiz do GestorERP | **fechado**, confirmado no §8 (leitura, bloqueio de `.mcp.json`, bloqueio fora da raiz) |
| R1 | contagem no `extract` | **aberto**: medir contar × listar |
| R3 | não determinismo com `temperature 0` | **explicado em parte** (§9.1, item 3): estável por rodada, varia entre rodadas sem restart; causa provável, cache ou ponto flutuante, não provada |
| R4.1 | commits | **fechado** (§9.1, item 2) |
| R4.2 | cofre / CLAUDE.md do GestorERP | **aberto**, com o owner |
| R4.3 | link PCIe `00:1b.4` | **aberto**, reparo físico |

### 9.4 Decisões do owner e execução (14:46–14:50)

**Decisões (AskUserQuestion):** R1 "medir agora"; R2 "documentar só no `generate`".

**R1: contagem no `extract`. Medido; a hipótese "listar e contar por fora" foi confirmada.**

Modo `count` no driver (`r1-count2.txt`): `extract` em 3 arquivos com contagem conhecida, 5 vezes em
cada modo, com 1 s entre chamadas. O gabarito é calculado no script.

| Caso | Esperado | "Responda apenas com o número" | "Liste em array JSON" (o script conta) |
| --- | --- | --- | --- |
| tabela "Rules activas" do GestorERP (o caso do §8, teste 10) | 19 | 3/5 (18, 18, 19, 19, 19) | **5/5** |
| linhas de flag da tabela de configuração do README | 16 | **0/5** (17, 20, 17, 19, 19) | **5/5** |
| cabeçalhos `## \`tool\`` do `TOOLS.md` | 7 | 5/5 | **5/5** |
| **Total** | | **8/15 (53%)** | **15/15 (100%)** |

- **1ª rodada** (`r1-count.txt`): a contagem direta deu 7/15. A listagem saiu como `parse_erro` em
  15/15 por **erro no meu parser**: ele cortava o texto no último `]`, que pertence à linha
  `[arquivo] ...` depois do array. O modelo tinha devolvido JSON válido, como uma chamada manual
  confirmou (`["\`chat\`", ..., "\`translate\`"]`, 7 itens). Corrigi o parser e repeti a medição.
- **Somando as duas rodadas:** contar direto, **15/30 (50%)**; listar, **15/15 (100%)**.
- O `extract` usa `fileTemperature = 0.2`, o que explica a variação entre as execuções da contagem
  direta.
- **Recomendação de uso:** para contar, peça **a lista** (em JSON) e conte por fora.

**R2: documentação. Feito (commit `10e879a`).**

- `generate` ganhou "Counts and arithmetic are unreliable: ask for steps or a list and check
  outside."
- `TOOLS.md` regenerado (`-check` = in sync); `go vet` OK; `go test -count=1 ./...` verde.
- `tools/list` = **4.390 de 4.400 bytes**. Sobram 10 bytes: uma próxima frase em qualquer tool vai
  exigir rever o teto `toolsListBudget` (decisão do owner).
- `chat` e `extract` **não** receberam a frase, por decisão do owner (orçamento).
- **Binário:** `D:\MCP\mcpollama-mcp.exe` = **`0.5.2-10e879a`** (14:47). O smoke do `tools/list`
  contém a frase nova. O `.exe~` agora é o 0.5.1, que estava em uso. Os clientes pegam o 0.5.2 ao
  reiniciar.

**Estado consolidado atualizado:**

| # | Item | Estado (14:50) |
| --- | --- | --- |
| R1 | contagem no `extract` | **medido**: contar 50%, listar 100%. Recomendação documentada só no `generate` |
| R2 | documentar o limite | **feito** (`generate`), binário 0.5.2 |
| R3 | não determinismo com `temperature 0` | explicado em parte (§9.1) |
| R4.2 | cofre / CLAUDE.md do GestorERP | **aberto**, com o owner |
| R4.3 | link PCIe `00:1b.4` | **aberto**, reparo físico |

---

## 10. Validação das correções no binário 0.5.2 (sessão do GestorERP, 2026-10-26 14:52–14:54)

> **De:** a sessão do Claude Code no GestorERP (Opus 5.5). Pedido do owner: "pode repetir o teste
> para validar as correções" e depois "registre a validação".
> **Por que não pelas tools da sessão:** a sessão GestorERP ainda roda o **0.5.1**. A descrição do
> `generate` carregada nela não tem a frase do R2, e o binário novo só entra ao reiniciar o Claude
> Code. Para validar o **0.5.2** sem reiniciar, usei um cliente MCP stdio próprio, que inicia o
> `D:\MCP\mcpollama-mcp.exe` com os **mesmos `args` do `.mcp.json` do GestorERP** (lidos do arquivo;
> o token não é impresso e passa por scrub na saída).
> **Evidências:** `…evidencias/v10-validacao-drv.py` (o cliente) e `…evidencias/v10-validacao-out.txt`
> (a saída). Varredura do token nos dois: 0 ocorrências.
> **Execução:** 31 `tools/call` em 81 s, com 1 s entre chamadas por causa do limite de taxa do nginx
> (§7.3). Nenhum HTTP 429.

### 10.1 Identificação

- `mcpollama-mcp.exe --version` = `0.5.2-10e879a`; o `serverInfo` do `initialize` também diz
  `{"name": "mcpollama", "version": "0.5.2-10e879a"}`.
- `tools/list`: 7 tools (`chat`, `embed`, `extract`, `generate`, `list_models`, `summarize_file`,
  `translate`).

### 10.2 Resultados por correção

| Correção | Teste | Esperado | Obtido | Veredito |
| --- | --- | --- | --- | --- |
| **R2** (frase no `generate`) | descrição do `generate` no `tools/list` | contém "Counts and arithmetic are unreliable" | contém | ✅ |
| **P1** (aviso no `embed`) | descrição do `embed` no `tools/list` | contém "evicts the generation model" | contém | ✅ |
| **P1** (2 residentes) | `embed` (3 textos) e logo depois `generate` | `load_ms` = 0 nos dois | `embed` `total_ms=24 load_ms=0`; `generate` "Brasília" `total_ms=131 load_ms=0` | ✅ |
| **P3/R2** (contas) | 4 contas, direto (`temperature 0`) | 403, 564, 940, 376 | 415, 552, 896, 241 → **0/4** | ❌ limite do modelo, como medido no §7.4 |
| **P3/R2** (contas) | as mesmas 4, "passo a passo + `RESULTADO:`" | idem | 403, 564, 940, 376 → **4/4** | ✅ (`eval_count` 108–147, `total_ms` 1.189–1.586) |
| **R1** (contagem) | `extract` no manifesto de rules do GestorERP, 3×: listar em array JSON (o script conta) × contar direto | 19 | listar 19, 19, 19 → **3/3**; contar 18, 18, 18 → **0/3** | ✅ listar · ❌ contar |
| **R1** (contagem) | `extract` no `TOOLS.md`, 3×, os mesmos 2 modos | 7 | listar **3/3**; contar **3/3** | ✅ |
| **P2** (README) | `summarize_file` do README, 3× com `focus` e 3× sem | 0 frases "concepção/fase inicial" | **0/6** | ✅ |
| **P5** (raiz do GestorERP) | `extract` de FolderVersion/Data do manifesto | `1.11.0`, `21/10/2026` | `{"folderVersion": "1.11.0", "data": "21/10/2026"}` | ✅ |
| **P5** (sensível) | `summarize_file` do `.mcp.json` do GestorERP | recusa | `isError`: "acesso negado: o arquivo está na lista fixa de nomes sensíveis…" | ✅ |
| **P5** (fora da raiz) | `summarize_file` de `D:\Users\claiton.linhares\.claude\CLAUDE.md` | recusa | `isError`: "arquivo não encontrado ou fora das raízes permitidas (--allow-root)" | ✅ |

**Gabaritos calculados fora do modelo:** as contas pela aritmética; a contagem de 19 pelo `awk` do §8.1
e a de 7 pelo `grep` do §3.

### 10.3 Leitura

1. **Todas as correções do §7 e do §9 estão presentes e funcionando no 0.5.2:** P1 (descrição e
   residência), R2 (descrição), P2 (README) e P5 (raiz, bloqueio de sensível, bloqueio fora da raiz).
2. **Os limites do modelo continuam iguais, e as recomendações documentadas os contornam:**
   - contas: direto 0/4, passo a passo 4/4;
   - contagens: contar direto 3/6, listar 6/6.
   - O caso das 19 rules errou **sempre** na contagem direta (18, 18, 18), coerente com o §9.4 (3/5).
3. **A contagem direta errou de forma estável nesta rodada** (18 nas 3 vezes, mesmo com
   `fileTemperature = 0.2`). O §9.4 viu 18 ou 19. Isso reforça a recomendação de listar em vez de
   contar.

### 10.4 Divergência a esclarecer: tamanho do `tools/list`

- O cliente deste §10 mediu o `result` do `tools/list` serializado como JSON compacto
  (`separators=(",", ":")`, `ensure_ascii=False`, UTF-8): **7.548 bytes**.
- O §9.4 registra o `TestToolsListBudget` com **4.390 de 4.400**.
- Os números não batem, então a métrica deve ser outra. Pode ser que o teste meça só nomes e
  descrições, sem os `inputSchema`, ou outro nível de `--tools-verbosity`. **Não investiguei.**
- **Pendência:** antes de rever o teto (`toolsListBudget`), confirmar no
  `src/tools/*_test.go` o que o teste mede e registrar aqui. Isso evita decidir o orçamento com uma
  base errada.

### 10.5 Estado consolidado (14:54)

| # | Item | Estado |
| --- | --- | --- |
| P1 | `embed` descarrega o modelo | **fechado**, confirmado por 3 execuções (§7.2, §8.2, §10.2) |
| P2 | resumo "inventado" | **fechado**: 0/20 (§7.3) + 1/1 (§8) + 0/6 (§10) |
| P3/R2 | aritmética | **fechado como limite documentado**: frase no `generate` confirmada no 0.5.2; passo a passo 4/4 |
| P4 | embeddings | aprovado (§7.5); não repetido |
| P5 | raiz do GestorERP | **fechado**, confirmado no §8 e no §10 |
| R1 | contagem no `extract` | **fechado como limite documentado**: listar 6/6 × contar 3/6 (§10) e 15/15 × 15/30 (§9.4) |
| R3 | não determinismo com `temperature 0` | explicado em parte (§9.1); sem impacto prático |
| R4.2 | cofre / CLAUDE.md do GestorERP | **aberto**, com o owner |
| R4.3 | link PCIe `00:1b.4` | **aberto**, reparo físico |
| novo | métrica do `TestToolsListBudget` × 7.548 bytes medidos | **aberto**, só esclarecer (§10.4) |
| — | sessão GestorERP no 0.5.1 | reiniciar o Claude Code para os clientes carregarem o 0.5.2 |
