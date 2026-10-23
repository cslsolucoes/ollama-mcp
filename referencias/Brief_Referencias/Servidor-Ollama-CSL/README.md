# Servidor Ollama da CSL

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B4 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` (loop principal) |

Servidor de inferência que o mcpollama expõe como tools. Foi descrito a partir do guia de operação `CONEXAO-IA.md` (atualizado em 2026-10-23 15:37) e do `ia-local.ps1`, e validado ao vivo em [dados/validacao-empirica.md](dados/validacao-empirica.md). **Sem nenhum valor secreto:** os segredos aparecem só pelo caminho, como faz o próprio guia (`CONEXAO-IA.md:6-8`).

## Fontes

| Fonte | Trechos usados |
| --- | --- |
| `D:\Users\claiton.linhares\.ssh\.workspace\Backup\ia\CONEXAO-IA.md` (217 linhas) | §1 identificação (`:12-21`), §4 Ollama (`:99-136`), §5 acesso remoto (`:140-172`), §6 mapa de arquivos (`:176-195`), §7 checklist (`:199-207`), §8 resumo (`:211-217`). As §2 (SSH) e §3 (GPU) **não** foram usadas |
| `%USERPROFILE%\.claude\scripts\ia-local.ps1` | cabeçalho e fluxo (`:1-60`) |
| Servidor ao vivo | [dados/validacao-empirica.md](dados/validacao-empirica.md) |

## Identificação (`CONEXAO-IA.md:14-21`)

| Campo | Valor |
| --- | --- |
| Host | `ia.cslsolucoes.com.br` = `10.100.2.2` (LAN) |
| SO | CentOS Stream 10 |
| Hardware | 4× NVIDIA RTX 3070 8 GB, 93 GB RAM |
| Ollama | v0.40.8, bind `0.0.0.0:11434`, com firewall restrito a `10.100.0.0/16` (`:103-106`) |

## Modelos (guia + medido)

| Modelo | Papel | Fonte |
| --- | --- | --- |
| **`qwen3-ia`** → `qwen3-coder:30b-a3b-q4_K_M` (MoE `qwen3moe`, 30,5B, Q4_K_M) | modelo principal do mcpollama. `num_ctx` **131072**, 100% GPU (25,6 GB), **~97 tok/s medidos** | `CONEXAO-IA.md:107`, `:133-136`; validação §3 e §5 |
| `nomic-embed-text` | embeddings, 768 dimensões, ctx 2048 | `CONEXAO-IA.md:108`, `:131`; validação §6 |
| `qwen3.8:27b-64k` | modelo do nível `local` atual (`OPENAI_MODEL` do `csl-ollama.env`), via OpenClaude | `ia-local.ps1:3`, `:44-46`; validação §3 |
| `qwen3-coder:30b-a3b-q4_K_M`, `qwen3.8:27b-q4_K_M` | bases | validação §3 |

Config de performance (`CONEXAO-IA.md:111-113`): 4 GPUs, `OLLAMA_CONTEXT_LENGTH=131072`, `OLLAMA_KV_CACHE_TYPE=q4_0`, `OLLAMA_KEEP_ALIVE=-1` (modelo residente).

A troca de modelo foi feita em 2026-10-23, motivada por um "bake-off": ~93 contra ~22 tok/s do generalista 27B, "com qualidade equivalente" (`CONEXAO-IA.md:133-136`). A qualidade é afirmação do guia, **não** medida aqui.

## Acesso (`CONEXAO-IA.md:140-172`)

- O Ollama não fica exposto diretamente. A entrada é o **nginx `:8443`** (HTTPS + Bearer + rate-limit), e o acesso pela internet é pelo **Cloudflare Tunnel** (`:142-143`, `:147`).
- **Certificado:** Cloudflare Origin CA, que o SO não reconhece (`:151`, `:165`). O guia usa `curl -k`; o mcpollama usa `--tls-ca` (validação §2).
- **Token:** é um Bearer. A fonte e as cópias ficam no servidor, referenciadas só pelo caminho (`:148-150`). No cliente, o token fica no `%USERPROFILE%\.openclaude\csl-ollama.env` (chave `OPENAI_API_KEY`, valor `[omitido]`).
- **Split-horizon DNS** (`:160-170`):
  - de dentro da LAN, usar `:8443`;
  - de fora, o host sem porta (Cloudflare);
  - **medido:** de dentro da LAN, o host sem porta dá timeout (validação §1).
- **Respostas** (`:171-172`): 200 com token, **401** sem token, **403** em `/api/pull|push|create|delete|copy|blobs`. **Medido:** 401 e 403 confirmados (validação §4).

## Nível `local` atual (`ia-local.ps1`)

- Roda o OpenClaude headless com `qwen3.8:27b-64k` (64k), lendo o provedor do `csl-ollama.env` e a CA de `certs\cloudflare-origin-ca.pem` (`:3`, `:31-46`).
- Antes de tudo faz um health check em `GET <OPENAI_BASE_URL>/models` (endpoint OpenAI-compatível) com 15 s de limite (`:48-51`).
- Códigos de saída:
  - **2:** arquivo ausente ou prompt vazio (`:36`, `:55`);
  - **3:** servidor indisponível (`:51`);
  - **124:** timeout (default 900 s) (`:11`, `:26`).
- É isto que a tool MCP do mcpollama substitui: uma chamada à API nativa `/api/*` sem o OpenClaude no meio.

## Limites e riscos medidos (resumo da validação)

0. **Atualização 2026-10-23 19:18:** o item 1 foi **diagnosticado e mitigado**. Era reboot da máquina por falha de PCIe na GPU 3. A config atual é 3 GPUs com `num_ctx` **65536** (provisório); a 2ª via passou com 15.959 tokens em 3,2 s (validação §9).
1. **(Histórico, até 19:14.) Prompts grandes derrubam o serviço.** Um prompt de ~42 mil caracteres teve a conexão derrubada a ~45 s e o modelo saiu da VRAM, reproduzido 2× (validação §7). Na última vez, nenhum modelo sobrou carregado. **Defeito aberto do lado do servidor**, com causa a diagnosticar nos logs.
2. **Carga a frio de ~36,4 s** (validação §5).
3. **Fila única de GPU.** Carregar o `qwen3-ia` desaloja o `nomic-embed-text` (validação §5).
4. **O caminho internet (Cloudflare) não foi validado** a partir da LAN (validação §1).
