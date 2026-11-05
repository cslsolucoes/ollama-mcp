# Relatar os erros do modelo local (qwen3.8:27b): instruções para o Claude

> ✅ **Em vigor desde 2026-11-04** (mcpollama 0.10.0 e gateway 0.13.0, Fase 12; release de ajuste implantada em 2026-11-05: gateway 0.13.2, mcpollama 0.10.1, modelo `qwen3.8:27b`). Sessão com mcpollama antigo, sem a tool
> `reportar_erro`, **não** relata por outro meio: avise o usuário e siga.
>
> Fonte única destas instruções. Vale para **todo projeto que usa o servidor MCP `mcpOllama`** (tools `generate`, `chat`,
> `summarize_file`, `extract`, `translate`, `embed`).
> **Para quê:** cada erro registrado entra numa base de erros do servidor de IA. **Depois que o administrador confirma a correção**, o
> registro passa a servir para três coisas:
> - **medir** o modelo por tipo de erro;
> - **melhorar as respostas sem treino** (instruções melhores e exemplos de correções parecidas);
> - mais adiante, **treinar** o modelo (ajuste fino LoRA).
>
> Registro sem resposta certa, ou não confirmado, serve só para estatística. Plano:
> `.workspace/plans/MCPOllam2026-11-01-213500-fase12-erros-treino.plan.md` (repositório `cslsolucoes/ollama-mcp`).

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
| `jobs` | o `job=` (ou a lista `jobs=<papel>:<id>,…`) da linha `[uso]` da resposta errada, inteira e sem editar | sim |
| `job_do_erro` | qual chamada produziu o erro. Padrão: a `final`. Num arquivo grande, se o erro já estava numa parte (`map3`, por exemplo), indique essa parte. No **`translate`** não existe `final` (as partes são só concatenadas): para o relato servir ao treino, aponte a parte (`mapN`), ou a chamada `unico` se o arquivo coube numa só, e dê a tradução certa **daquela chamada**; um erro no texto montado conta só na estatística | se não for a `final` |
| `categoria` | uma de: `numeracao_errada`, `contagem_errada`, `calculo_errado`, `inventou`, `omitiu`, `formato_invalido`, `resumo_infiel`, `traducao_errada`, `idioma_errado`, `truncado`, `ignorou_instrucao`, `outro` | sim |
| `erro` | o que estava errado, objetivo e curto (ex.: "pulou a seção 7.2 e numerou 7.3 como 7.2") | sim |
| `resposta_correta` | o que **aquela chamada** (`job_do_erro`) deveria ter respondido, completo e no mesmo formato pedido. Na `final`, é a resposta inteira que o usuário deveria ter recebido. É o que ensina o modelo | sim, se você conseguiu corrigir |
| `correcao_aplicada` | o que foi feito para seguir o trabalho (ex.: "conferi no documento e corrigi a numeração à mão"; "refiz com `think:true`"; "subi para o haiku") | sim |
| `severidade` | `baixa` (cosmético), `media` (exigiu correção), `alta` (teria causado dano se não conferido) | sim |
| `origem` | `claude` ou `usuario` | sim |

## 3. Como registrar

Use a tool **`reportar_erro`** do `mcpOllama` (mcpollama 0.10.0 em diante), com os campos da §2 e os `jobs` da linha `[uso]` da
resposta errada. O servidor de IA guarda o **pedido exato** e a **resposta errada** daquele job junto com a sua correção, como **candidato**:
o registro só fica elegível para medir, para os exemplos e para o treino depois que o administrador conferir a correção e confirmar. A tool devolve o número do registro; cite-o no seu relato ao usuário.

- **Não** copie o conteúdo do documento, a resposta errada ou a resposta certa para pastas de projeto, relatórios, commits ou chat: o
  servidor já guarda o necessário, com acesso restrito.
- Sem a tool disponível (mcpollama antigo ou servidor fora), **não** crie registro alternativo. Avise o usuário de que o erro não pôde
  ser registrado e siga o trabalho. É decisão do owner: o registro só vale pela tool.

## 4. Boa correção = bom treino

- A `resposta_correta` deve ser **a resposta inteira que aquela chamada (`job_do_erro`) deveria dar**, não só o trecho corrigido. No
  `translate`, com `job_do_erro=mapN`, é a tradução inteira **daquela parte**, não do documento.
- Siga o **formato pedido** (se a tool pediu JSON, a resposta certa é JSON válido).
- Se você não tem certeza da resposta certa, deixe `resposta_correta` vazia e descreva o erro: o registro serve para estatística,
  mas não entra no treino.
- Não "melhore" além do pedido. O exemplo ensina o que foi pedido, não um estilo novo.

## 5. Limites

Um pedido com mais de **64 chamadas** ao modelo (arquivo muito grande com `--max-chunks` acima do padrão de 40) não pode ser relatado: a tool `reportar_erro` recusa com mensagem clara. Nesse caso, avise o usuário e siga o trabalho.
- Campos: `erro` (vai como `descricao`, obrigatório, até 4 KB), `resposta_correta` até 128 KB, `correcao_aplicada` até 4 KB, `projeto` até 128 bytes. Campo desconhecido é recusado.
- Só se relata um job **já terminado**. Logo depois da resposta, o servidor pode pedir para tentar de novo em alguns segundos.
- Só os jobs da sua estação, relatados da mesma máquina e do mesmo endereço de rede, entram na base. Os outros ficam só como referência.
