# Relatar os erros do modelo local (qwen3-ia): instruções para o Claude

> Fonte única destas instruções. Vale para **todo projeto que usa o servidor MCP `mcpOllama`** (tools `generate`, `chat`,
> `summarize_file`, `extract`, `translate`, `embed`).
> **Para quê:** cada erro registrado vira um exemplo de treino. Com ele, o modelo do servidor de IA é ajustado (ajuste fino LoRA)
> para não repetir aquele tipo de erro. Plano: `.workspace/plans/MCPOllam2026-11-01-213500-fase12-erros-treino.plan.md`
> (repositório `cslsolucoes/ollama-mcp`).

## 1. Quando relatar

Sempre que você (Claude) **conferir** um resultado do modelo local e ele estiver **errado**. A conferência já é regra: o resultado do
nível local é rascunho. Relate também quando o usuário apontar o erro.

**Não relate:**
- quando a falha foi do pedido (instrução ambígua, arquivo errado) e não do modelo;
- erro de infraestrutura (timeout, 429, 503, servidor fora). Esse vai para o relatório de incidente, não para o treino;
- a mesma ocorrência duas vezes.

## 2. O que registrar

| Campo | Conteúdo | Obrigatório |
| --- | --- | --- |
| `projeto` | nome do projeto/pasta (ex.: `GestorERP`) | sim |
| `job_id` | o `job=` (ou a lista `jobs=`) da linha `[uso]` da resposta errada | sim |
| `categoria` | uma de: `numeracao_errada`, `contagem_errada`, `calculo_errado`, `inventou`, `omitiu`, `formato_invalido`, `resumo_infiel`, `traducao_errada`, `idioma_errado`, `truncado`, `ignorou_instrucao`, `outro` | sim |
| `erro` | o que estava errado, objetivo e curto (ex.: "pulou a seção 7.2 e numerou 7.3 como 7.2") | sim |
| `resposta_correta` | o que o modelo **deveria** ter respondido, completo, no mesmo formato pedido. É o que ensina o modelo | sim, se você conseguiu corrigir |
| `correcao_aplicada` | o que foi feito para seguir o trabalho (ex.: "conferi no documento e corrigi a numeração à mão"; "refiz com `think:true`"; "subi para o haiku") | sim |
| `severidade` | `baixa` (cosmético), `media` (exigiu correção), `alta` (teria causado dano se não conferido) | sim |
| `origem` | `claude` ou `usuario` | sim |

## 3. Como registrar

Use a tool **`reportar_erro`** do `mcpOllama` (mcpollama 0.10.0 em diante), com os campos da §2 e o `job_id` da linha `[uso]` da
resposta errada. O servidor de IA guarda o **pedido exato** e a **resposta errada** daquele job junto com a sua correção, e o registro
entra na base de treino. A tool devolve o número do registro; cite-o no seu relato ao usuário.

- **Não** copie o conteúdo do documento, a resposta errada ou a resposta certa para pastas de projeto, relatórios, commits ou chat: o
  servidor já guarda o necessário, com acesso restrito.
- Sem a tool disponível (mcpollama antigo ou servidor fora), **não** crie registro alternativo. Avise o usuário de que o erro não pôde
  ser registrado e siga o trabalho. É decisão do owner: o registro só vale pela tool.

## 4. Boa correção = bom treino

- A `resposta_correta` deve ser **a resposta inteira que o modelo deveria dar**, não só o trecho corrigido.
- Siga o **formato pedido** (se a tool pediu JSON, a resposta certa é JSON válido).
- Se você não tem certeza da resposta certa, deixe `resposta_correta` vazia e descreva o erro: o registro serve para estatística,
  mas não entra no treino.
- Não "melhore" além do pedido. O exemplo ensina o que foi pedido, não um estilo novo.
