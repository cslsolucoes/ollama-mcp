# Validação empírica do servidor Ollama da CSL

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.1.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B4 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` (loop principal) |

> **Atualização 2026-10-23 19:18 (ver §9).** O defeito do §7 foi diagnosticado pelo owner: a **máquina reiniciava** por falha de PCIe na GPU 3, durante o prefill. Ele foi **mitigado** com 3 GPUs e `num_ctx` **65536**, e confirmado por 2ª via (15.959 tokens em 3,2 s, sem queda). Os números de contexto dos §3 e §8 (131072) valem para a config de 4 GPUs, anterior a 19:14.

Todos os números abaixo foram **lidos da saída real** de uma sonda em Go (`b4probe`, no scratchpad da sessão, fora do repositório). A sonda usa a mesma pilha que o mcpollama vai usar (`net/http` + `crypto/x509`, Go 1.26.5, Windows 11).

- **Token:** lido do `csl-ollama.env` só em memória, nunca impresso. Toda saída passou por uma troca do token por `[omitido]`.
- **Ordem das chamadas:** **sequenciais**, uma de cada vez (fila única de GPU).
- **Cache de prompt:** cada prompt começa com um nonce único, para o Ollama não reaproveitar o cache entre chamadas.

## Execuções

| Execução | Janela | Conteúdo |
| --- | --- | --- |
| 1 | 2026-10-23 16:18:25–16:23:57 | DNS, matriz TLS, estado, auth/bloqueio, 3 generates curtos, 3 prompts grandes, embed, caminho sem porta |
| 2 (estado) | 16:24:53 | só `GET /api/ps` |
| 3 (reteste, decisão do owner às 16:25) | 18:10:47–18:12:22 | recarga + 1 prompt grande com `stream:true`, com `/api/ps` antes e depois. Lançada às ~16:27; o processo só começou a imprimir às 18:10:47 (atraso não investigado) |
| 4 (recarga, decisão PB4-4) | 18:28:31–18:29:04 | generate curto para recarregar o `qwen3-ia`: `load_ms` **31.963**, resposta "ok", `/api/ps` com o modelo residente. Número lido na hora e registrado no changelog do plano (18:29); **a saída bruta não foi arquivada** em arquivo, ao contrário das execuções 1 a 3 (nota da revisão B6) |
| 5 (estado pós-mitigação) | 19:18:03 | só `GET /api/ps` e `POST /api/show` (ver §9) |
| 6 (2ª via pós-mitigação, decisão do owner às 19:18) | 19:18:39–19:18:43 | 1 prompt grande (500 linhas) com `stream:true`, com `/api/ps` antes e depois (ver §9). Saída arquivada em `.workspace/reports/MCPOllam2026-10-23-190500-incidente-ollama-prompt-grande.evidencias/saida-b4-2via.txt` |

Endpoint usado: `https://ia.cslsolucoes.com.br:8443` (do `OPENAI_BASE_URL` do `csl-ollama.env`, sem o `/v1`).

## 1. DNS e caminhos de acesso

| Medição | Resultado |
| --- | --- |
| `ia.cslsolucoes.com.br` resolvido de dentro da LAN | `10.100.2.2` (split-horizon, como diz o guia `CONEXAO-IA.md` §5) |
| `https://ia.cslsolucoes.com.br:8443/api/tags` | 200, `server: nginx`, sem `cf-ray` (não passa pelo Cloudflare) |
| `https://ia.cslsolucoes.com.br/api/tags` (sem porta) a partir da LAN | `dial tcp 10.100.2.2:443` com **timeout de 21 s** (3 tentativas, 21.035–21.057 ms) |

**Conclusão:** de dentro da LAN, só o `:8443` funciona. O host sem porta resolve para o IP interno, que não atende na 443. **O caminho Cloudflare (internet) não foi testado.** Testá-lo exige resolver por DNS público ou estar fora da LAN.

## 2. Matriz TLS (`GET /api/tags` em `:8443`, com token)

| Pool de CAs no `tls.Config.RootCAs` | Resultado |
| --- | --- |
| `nil` (só o repositório do SO) | **falha**: `x509: certificate signed by unknown authority` (46 ms) |
| `x509.NewCertPool()` + CA de origem (`cloudflare-origin-ca.pem`) | 200 (8 ms) |
| `x509.SystemCertPool()` + `AppendCertsFromPEM(CA de origem)` | 200 (8 ms) |

**Conclusão:** `SystemCertPool()` + a CA de origem funciona no Windows com Go 1.26.5. Esse desenho atende os dois caminhos sem trocar o pool do SO (ver `truenas-mcp/80-Licoes_Aprendidas.md`, `LIC-TNS-013`). A metade "internet com certificado público" continua sem prova, por causa do item 1.

## 3. Modelo e servidor

| Medição | Valor |
| --- | --- |
| `qwen3-ia:latest` → `parent_model` | `qwen3-coder:30b-a3b-q4_K_M` |
| Família / tamanho / quantização | `qwen3moe` / 30,5B (`general.parameter_count` 30.532.122.624) / Q4_K_M |
| Experts | 128, com 8 ativos por token (`qwen3moe.expert_count`, `expert_used_count`) |
| Contexto **configurado** (`num_ctx` em `/api/show` e `context_length` em `/api/ps`) | **131072** |
| Contexto máximo do modelo (`qwen3moe.context_length`) | 262144 |
| VRAM (`size_vram`) | 25.562.028.438 bytes (100% GPU) |
| Capacidades | `completion`, `tools` |
| Parâmetros do Modelfile | `temperature 0.7`, `top_k 20`, `top_p 0.8`, `repeat_penalty 1.05`, `num_gpu 999` |
| `modified_at` | 2026-10-23T15:36:22-03:00 |

**Modelos instalados (`/api/tags`):**
- `qwen3-ia`;
- `qwen3-coder:30b-a3b-q4_K_M`;
- `nomic-embed-text` (768 dimensões, ctx 2048, 137M, F16);
- `qwen3.8:27b-64k`;
- `qwen3.8:27b-q4_K_M`.

O `OPENAI_MODEL` do nível `local` atual (`csl-ollama.env`) é `qwen3.8:27b-64k`, ou seja, **outro modelo**, e não o `qwen3-ia`.

**Divergência:** o `CLAUDE.md` do projeto e os planos dizem "`qwen3-ia` (ctx 32k)". O servidor real tem `num_ctx` **131072** desde a troca de modelo às 15:36. O guia `CONEXAO-IA.md` (atualizado às 15:37) já registra isso.

## 4. Autenticação e bloqueio

| Chamada | Resultado |
| --- | --- |
| `GET /api/tags` **sem** token | **401** `{"error":"unauthorized"}` |
| `POST /api/pull` com token (modelo inexistente `modelo-inexistente-b4:none`, para não puxar nada nem se passasse) | **403**, página HTML do nginx (`403 Forbidden`) |

## 5. Geração curta (modelo residente)

Prompt de 44 tokens, `num_predict` 64, `temperature` 0, `stream:false`:

| Rodada | Total (ms) | `load_ms` | Prompt eval | Geração |
| --- | ---: | ---: | --- | --- |
| curto-1 | 536 | 0 | 44 tokens em 64 ms (678 tok/s) | 41 tokens em 424 ms (**96,5 tok/s**) |
| curto-2 | 438 | 0 | 44 em 50 ms (866 tok/s) | 37 em 382 ms (**96,8 tok/s**) |
| curto-3 | 437 | 0 | 44 em 50 ms (878 tok/s) | 37 em 381 ms (**97,0 tok/s**) |

`load_ms` 0 porque o modelo já estava residente. Confirma os "~93 tok/s" do guia: o medido é ~97 tok/s de geração.

**Carga a frio** (execução 3, depois que o modelo tinha saído da VRAM): `load_ms` **36.449** e total de 36.675 ms para gerar "ok". Ao carregar o `qwen3-ia`, o `nomic-embed-text` foi desalojado da VRAM (`/api/ps` antes: só `nomic`; depois: só `qwen3-ia`).

## 6. Embeddings

| Medição | Valor |
| --- | --- |
| `POST /api/embed` `nomic-embed-text` | 200, **768 dimensões**, 13 tokens |
| Carga a frio | `load_ms` 3.230, `total_ms` 9.936, wall 20.896 ms |

## 7. Prompts grandes: FALHA REPRODUZIDA (defeito do lado do servidor)

Prompt sintético (linhas numeradas, sem nenhum dado real), `num_predict` 24, `temperature` 0:

| Execução | Prompt | `stream` | Resultado | `/api/ps` depois |
| --- | --- | --- | --- | --- |
| 1 | 500 linhas, 42.540 caracteres | false | conexão derrubada pelo servidor aos **45.026 ms** (`wsarecv: Foi forçado o cancelamento de uma conexão existente pelo host remoto`) | (não medido entre chamadas) |
| 1 | 800 linhas, 68.040 caracteres | false | derrubada aos **90.049 ms** | — |
| 1 | 1.333 linhas, 113.679 caracteres | false | derrubada aos **90.026 ms** | ao fim da execução: **sem `qwen3-ia`**; só o `nomic-embed-text`, carregado pelo embed seguinte |
| 3 | 500 linhas, 42.540 caracteres | **true** | derrubada aos **45.019 ms**, **antes dos cabeçalhos HTTP** | **nenhum modelo carregado** (`models: []`) |

- Nenhuma tentativa devolveu contadores. Por isso **não há medida de tokens/linha nem de prompt eval com prompt grande**. As metas de "24k" e "40k" foram calculadas com um palpite de 30 tokens/linha e **não são números medidos**.
- **Leitura dos fatos:** com 2 reproduções, um prompt de ~42 mil caracteres faz o servidor derrubar a conexão a ~45 s, e logo depois o `/api/ps` fica sem o modelo. Na execução 3, nenhum modelo sobrou.
  - Que nenhum modelo tenha sobrado é compatível com o serviço `ollama` ter **caído e reiniciado** **(inferência)**.
  - O streaming não muda nada, porque a falha acontece ainda no processamento do prompt, antes de qualquer byte de resposta.
- **Hipóteses de causa (não provadas, exigem `journalctl -u ollama` e o `error.log` do nginx):**
  - estouro de memória da GPU no buffer de computação do prompt eval;
  - incompatibilidade de `OLLAMA_KV_CACHE_TYPE=q4_0` (guia §4) com a configuração de atenção;
  - limite de tempo ou de corpo no nginx combinado com o reinício do serviço.
- **Limiar desconhecido:** funciona com 44 tokens e falha com 42.540 caracteres. O ponto de corte entre os dois **não foi buscado**, porque cada falha derruba o serviço que outros clientes usam.

## 8. Implicações para o mcpollama (a consolidar no B5)

1. **Contexto:** o servidor está configurado com 131072, mas o limite **utilizável hoje** é menor e desconhecido. As tools de arquivo (Fase 4) não podem assumir nenhum dos dois números até o defeito do item 7 ser corrigido ou o limiar ser medido. A P-2 (32k × 64k) fica superada: o modelo mudou.
2. **Resiliência:** uma falha do servidor aparece como conexão derrubada, sem corpo JSON. O cliente Ollama do mcpollama precisa traduzir isso num erro de tool claro, e nunca numa resposta vazia.
3. **Carga a frio de ~36 s**, e o modelo pode sair da VRAM. O timeout por chamada e a mensagem ao usuário precisam prever isso.
4. **TLS:** `SystemCertPool()` + `--tls-ca` funciona; adotar.
5. **Embeddings:** `nomic-embed-text` (768 dimensões) existe. O `qwen3-ia` não serve para isso (capacidades só `completion` e `tools`). A tool `embed` precisa de um modelo de embeddings próprio (default `nomic-embed-text`).
6. **Concorrência de VRAM:** carregar o `qwen3-ia` desaloja o `nomic-embed-text`. Alternar `generate` e `embed` custa recargas.

## 9. Causa do defeito, mitigação e 2ª via (2026-10-23 19:16–19:18)

**Causa** (diagnóstico do owner, com SSH ao servidor; fonte: `.workspace/reports/MCPOllam2026-10-23-191600-resposta-incidente-ollama-prompt-grande.report.md` §1):
- **não** era o `ollama` caindo (`NRestarts=0`), e sim **a máquina reiniciando**;
- `last reboot` registra 16:20, 16:21, 16:23 e 18:11, os mesmos horários das 4 requisições grandes do §7;
- o kernel registrou AER PCIe no Root Port `00:1c.5`, seguido de `NVRM: GPU lost from the bus` e `uvm ... fatal error 0x60, requiring os reboot`;
- a GPU 3 (`07:00.0`) negociava PCIe Gen1 x1.
- A inferência deste brief, de queda do serviço ou do runner (§7), estava **errada** quanto ao mecanismo. Os fatos medidos estão corretos: 45 s, RST, VRAM vazia e os 90 s = recarga + queda.

**Mitigação** (owner): `CUDA_VISIBLE_DEVICES=0,1,2` (GPU 3 excluída), `OLLAMA_CONTEXT_LENGTH=65536` e `qwen3-ia` com `num_ctx 65536`. Com 128k o modelo não cabia em 3 GPUs. O retorno a 4 GPUs e 131072 depende do reparo físico (riser, cabo ou PSU).

**2ª via deste brief, pelo caminho do cliente** (estação → nginx `:8443`, sonda `b4probe`; saída lida):

| Medição | Valor |
| --- | --- |
| `/api/ps` (19:18:03 e 19:18:39) | `qwen3-ia:latest`, `context_length` **65536**, `size_vram` **21.660.958.717** |
| `/api/show` `qwen3-ia` | `num_ctx 65536`; `parent_model` `qwen3-coder:30b-a3b-q4_K_M`; `modified_at` 2026-10-23T19:14:42 |
| Prompt de **500 linhas / 42.540 caracteres** (o mesmo do §7), `stream:true` | **200**; `prompt_eval_count` **15.959** em **3.199 ms (~4.988 tok/s de prefill)**; 1º chunk aos 3.249 ms; total de 3.577 ms; 17 tokens gerados; resposta **correta** `primeira=00001 ultima=00500` (o modelo viu o início e o fim do texto) |
| `/api/ps` depois | modelo ainda carregado, sem queda |
| Calibração | **31,92 tokens/linha**, ou seja, **~2,67 caracteres por token** neste texto sintético (números e português sem acentos). Texto real pode ter outra razão: medir antes de usar como regra |

**Implicações atualizadas (substituem os itens 1 e 2 do §8):**
1. O limite de prompt agora é o **contexto configurado**: 65536 hoje, 131072 após o reparo. O teto do mcpollama tem de ser **configurável** (`--max-prompt-chars`, P-6). O owner recomenda ~48k tokens de entrada, deixando o resto para a geração.
2. O prefill é rápido (~5.000 tok/s): 16k tokens levam ~3 s. O custo dominante de uma tool de arquivo é a geração (~97 tok/s), não a leitura do prompt.
3. Uma queda futura do host continuará aparecendo no cliente só como RST. Mantém-se o requisito de erro de tool claro (`MCPO-REQ-OLL-005`).
