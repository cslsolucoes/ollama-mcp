# W4 — Ollama vs LocalAI (artigo da Contabo)

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B3 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |
| **URL(s) consultadas** | <https://contabo.com/blog/ollama-vs-localai-best-self-hosted-openai-compatible-llm-server/> |
| **Status da coleta** | **OK** |

## O que é

Artigo de blog comparando dois servidores de LLM self-hosted, Ollama e LocalAI. É uma fonte de contexto, não uma referência de código. Não há implementação de MCP.

## Tools expostas

Não aplicável: o artigo não trata de MCP.

## Transporte e linguagem

Não aplicável ao artigo. Ele descreve os dois servidores como HTTP com API compatível com OpenAI:

- Ollama: porta 11434, endpoints `/v1/…` com o subconjunto mais comum (chat completions, completions e embeddings).
- LocalAI: porta 8080 por padrão, "designed from day one as a drop-in OpenAI replacement", com chat, embeddings, geração de imagens, transcrição de áudio e TTS.

## Configuração (flags/env, auth, TLS)

- **TLS e token:** o artigo recomenda: "Put TLS (Caddy or Nginx) and a token-based auth proxy in front of either endpoint before exposing it to the internet." Vale para os dois. O texto **não detalha** como configurar TLS nem como funciona o proxy com token.
- **Ollama:** instalação com um comando curl, rodando como serviço systemd. Detecta CUDA, ROCm e Apple Metal sem configuração.
- **LocalAI:** "Docker-first"; em produção, usa arquivos de configuração para backend, caminhos de modelos e opções por modelo. Na GPU, exige escolher a variante de imagem Docker e definir variáveis de ambiente de GPU.

## Ideias aproveitáveis para o mcpollama

1. **Confirmação da recomendação de TLS e proxy com token na frente do Ollama.** O artigo corrobora o desenho que o mcpollama já adota (nginx com Bearer e TLS). Não acrescenta detalhe de implementação.
2. **Escolha de servidor por uso.** Segundo o artigo, o Ollama é indicado para chat e embeddings em volume moderado, e o LocalAI para alto throughput ou endpoints multimodais. Para o `qwen3-ia` atual (único modelo residente, GPU em fila), a escolha do Ollama se mantém. Isso é conclusão do artigo, e não verificado no servidor da CSL.
3. **Ollama e LocalAI podem rodar lado a lado** (portas diferentes, segundo a FAQ). Não é necessário para o mcpollama, mas explica a separação de endpoints.

## Limitações / o que não serve

- Artigo genérico, sem números de benchmark verificados e sem código.
- Não detalha TLS, token nem proxy, que são justamente o que o mcpollama precisa.
- Não trata de MCP.
- Comparativos de desempenho são opinião do autor, não medição.

## Achados

Nenhuma instrução dirigida a agentes de IA foi encontrada no artigo.

## Fontes

- <https://contabo.com/blog/ollama-vs-localai-best-self-hosted-openai-compatible-llm-server/> (coleta 2026-10-23; OK)
