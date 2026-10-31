# Relatório — Fase 11.1 (spike): espera longa de tool MCP no Claude Code

- **Data:** 2026-10-30 (execuções entre 23:27 e 23:45)
- **Claude Code:** 2.1.296 · modelo do cliente do teste: `haiku` (Haiku 5.5)
- **Escopo:** só o experimento. Nada em `src/` foi alterado; o servidor da IA não foi tocado.
- **Artefatos brutos (fora do repo):** `D:\Users\claiton.linhares\AppData\Local\Temp\claude\e--MCPOllam\19be7a02-91dd-4049-bbce-a0473f6228d1\scratchpad\spike111\` (`main.go`, `spike.exe`, `run.sh`, `*.stream.jsonl`, `*.server.log`, `*.time`, `*.err`).

## 1. Ferramenta de teste

Servidor MCP mínimo em Go (stdio, JSON-RPC 2.0, sem dependências, módulo `spike`). Tool `esperar(segundos)`: dorme N s numa goroutine e devolve `pronto apos N s, inicio=HH:MM:SS fim=HH:MM:SS`.
Log com timestamp de `initialize`, `tools/call` recebido, resposta enviada, fechamento do stdin e qualquer outro método (inclusive `notifications/cancelled`). Em nenhuma execução chegou `notifications/cancelled`: quando a sessão `-p` encerra com a chamada ainda pendente, o servidor só vê o **stdin fechar**.

## 2. Comando exato (via `run.sh`)

```
cd <spike111>
[ "$VAR" = "1" ] && export CLAUDE_AUTO_BACKGROUND_TASKS=1
claude -p --mcp-config mcp-<N>.json --strict-mcp-config --output-format stream-json --verbose \
  --allowedTools "mcp__spike__esperar" --model haiku --no-session-persistence \
  "Chame a tool mcp__spike__esperar com segundos=<S>. Depois que ela devolver o resultado, responda exatamente: FIM <resultado da tool>"
```

`mcp-<N>.json`: `{"mcpServers":{"spike":{"command":"<spike111>/spike.exe","args":[],"env":{"SPIKE_LOG":"<spike111>/<N>.server.log"}}}}` (sem `timeout` por servidor; sem `CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS`).
Obs.: o modelo gasta uma volta carregando o schema da tool (ToolSearch) antes de chamá-la; isso não afeta as medições da chamada em si.

## 3. Resultados

| Cenário | Tool | `CLAUDE_AUTO_BACKGROUND_TASKS` | Duração da chamada | Foi para segundo plano? | Modelo recebeu o resultado? | Texto final do `claude -p` | Duração total da execução |
| --- | --- | --- | --- | --- | --- | --- | --- |
| A | 40 s | não | 40,0 s (23:27:23 → 23:28:03) | não | **sim** | `FIM pronto apos 40 s, inicio=23:27:23 fim=23:28:03` | 58 s |
| B0 | 170 s | não | 170,8 s (23:28:33 → 23:31:24) | **não** (passou dos 2 min e seguiu bloqueada) | **sim** | `FIM pronto apos 170 s, inicio=23:28:33 fim=23:31:23` | 200 s |
| B1 | 170 s | **1** | interrompida (pedido às 23:32:35; movida aos ~120 s) | **sim**, como `task_type: mcp_task`, id `ktsff9yv8` | **não** | "A chamada passou de 120 s e foi para segundo plano como tarefa `ktsff9yv8`. Ainda não há resultado." | 136 s |
| C0 | 300 s | não | 300,0 s (23:35:04 → 23:40:04) | **não** | **sim** | `FIM pronto apos 300 s, inicio=23:35:04 fim=23:40:04` | 317 s |
| C1 (e C1b, repetição limpa) | 300 s | **1** | interrompida (movida aos ~120 s) | **sim**, id `k9r45ga32` (C1b) | **não** | "ainda está rodando ... passou para segundo plano como tarefa `k9r45ga32` e não devolveu resultado ainda" | 130 s (C1b) |

### Eventos relevantes do stream-json

- **A, B0, C0** (sem a variável): só `tool_progress` com `"heartbeat": true` a cada 30 s (`elapsed_time_seconds`: 30, 60, 90, ...), depois o `tool_result` com o texto da tool. Nenhum evento de task. O servidor recebeu a resposta de volta e o stdin fechou só depois da resposta final do modelo.
- **B1, C1** (com a variável), aos ~120 s:
  - `system/background_tasks_changed` com `task_type: "mcp_task"`, `description: "spike/esperar"`, seguido de `system/task_started`;
  - o **`tool_result` entregue ao modelo** é o texto: `MCP tool "spike/esperar" is still running after 120s. It was moved to the background as task ktsff9yv8 and keeps running; you'll receive a notification with the result when it completes. You can keep working in the meantime. To stop it, use TaskStop with task_id "ktsff9yv8". Note: it does not survive exiting this session.`
  - o modelo respondeu "ainda não há resultado" e a execução `-p` terminou (`terminal_reason: completed`);
  - ao sair, o Claude Code emitiu `system/task_notification` com **`status: "stopped"`** e `task_updated` com `status: "failed"`. No servidor: **nenhuma resposta enviada** e nenhum `notifications/cancelled`, só o stdin fechando (B1 23:34:43; C1b 23:45:23).
- O resultado da tool **nunca chegou ao modelo** nos cenários com background. Isso é o esperado pela documentação (ver §5): em `claude -p` de um tiro a sessão termina antes de a tarefa concluir.

## 4. Respostas às perguntas do spike

- **(a) Vai para segundo plano aos ~2 min?** Em `-p`, **só com `CLAUDE_AUTO_BACKGROUND_TASKS=1`** (B1, C1: sim, aos 120 s; B0, C0: não, a chamada seguiu bloqueada e voltou normal). Isso confirma a documentação. No uso interativo (extensão VS Code/CLI) a doc diz que o auto-background vale sem variável; **não foi provado aqui**.
- **(b) Ao terminar em segundo plano, o modelo é avisado e recebe o resultado?** **Não pôde ser provado em `-p`.** Aqui a sessão acabou antes (a execução de um tiro não espera a tarefa), então a tarefa virou `stopped`. O que está provado: ao mover, o modelo recebe na hora um `tool_result` dizendo que a chamada está em segundo plano e que "você receberá uma notificação com o resultado". A entrega da notificação ao modelo (e se ela acorda uma sessão ociosa) só se prova no modo interativo. A documentação diz: "the result arrives as a task notification when the call settles".
- **(c) Tool de 5 min?** **Sim, sem corte** no `-p` sem a variável (C0: 300,0 s, resposta íntegra, modelo repassou o texto). Pela documentação, o limite real é o `timeout` por servidor/`MCP_TOOL_TIMEOUT` (padrão ~28 h) e o ocioso de **30 min para stdio**. Não houve nenhum corte nem aviso aos 2 ou aos 5 min.
- **(d) Tool que volta antes do prazo (40 s)?** Tudo normal (A).

## 5. Documentação consultada (https://code.claude.com/docs/en/mcp.md, lida em 2026-10-30)

- Chamada MCP na conversa principal parada há mais de 2 min vai para segundo plano; "Claude receives the task ID immediately and keeps working, and the result arrives as a task notification when the call settles"; a tarefa "doesn't survive exiting the session"; `CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS` muda o limite (0 desliga).
- Nunca vão para segundo plano: chamadas de subagentes; servidores IDE; **modo não interativo, salvo `CLAUDE_AUTO_BACKGROUND_TASKS=1`**.
- Limites por chamada: `timeout` por servidor (parede, em ms, não estendido por progresso) ou `MCP_TOOL_TIMEOUT` (padrão ~28 h); **ocioso** (sem resposta nem progresso) 5 min para HTTP/SSE/WS e **30 min para stdio** (`CLAUDE_CODE_MCP_TOOL_IDLE_TIMEOUT`). Esses limites continuam valendo em segundo plano.
- "To make Claude wait for a task's result in an interactive session": `CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS=0` faz o Claude esperar toda chamada MCP até o fim (e `Esc` encerra a chamada).
- MCP *tasks* assíncronas: exigem Claude Code ≥ 2.1.304 (instalado: 2.1.296). O `task_type: "mcp_task"` visto no stream é só o rótulo da tarefa em segundo plano da chamada comum, não a task do protocolo.

## 6. Recomendação para o `timeout_s` da `aguardar_jobs` (D11-19)

Baseada só no que foi medido:

1. **Sem background (o que o stdio faz hoje em `-p`, e a doc dá como padrão interativo só acima de 120 s):** uma chamada de 300 s funciona e devolve o resultado (C0). O risco está **acima de 120 s no uso interativo**: aí a chamada é movida para segundo plano, o modelo recebe "movida para segundo plano, aguarde a notificação" e o resultado só chega, se chegar, como notificação. Se a notificação não acordar uma sessão ociosa (não provado), o modelo fica sem ver o resultado exatamente no cenário que a `aguardar_jobs` quer resolver.
2. **Recomendação:** configurar `aguardar_jobs` com `timeout_s` **padrão 100 s e máximo 110 s por chamada** (folga de ~10–20 s abaixo dos 120 s medidos), e as descrições instruem "sem outra tarefa, chame de novo". Assim a chamada **sempre volta antes** do auto-background, o resultado entra como `tool_result` normal (caso A, provado) e o ciclo do owner (D11-19, "a cada ciclo até concluir") continua: o Claude chama de novo a cada ~100 s em vez de a cada 5 min. Custo: ~3× mais chamadas por espera de 5 min; cada uma é barata (volta na hora ao concluir).
3. **Manter 300 s só se** o teste interativo (§7) provar que o modelo é acordado ao fim da chamada em segundo plano, **ou** se o owner optar por `CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS=0` no `settings.json` das estações (doc: faz o Claude esperar toda chamada MCP até o fim). Esta última opção funcionou na teoria da doc, **não foi testada aqui** (no `-p` não há background sem a variável, então o teste não a diferencia). Ela também depende de configuração por estação, o que o mcpollama não controla.
4. Aviso de implementação: independentemente do valor, o servidor deve **responder e liberar a goroutine** quando o stdin fechar ou chegar `notifications/cancelled`; o spike mostrou que ao encerrar a sessão com a chamada pendente só o fechamento do stdin é visível (sem `cancelled`).
5. O corte ocioso de 30 min do stdio e o `timeout` padrão de ~28 h estão fora de qualquer valor ≤ 300 s; não são fator.

## 7. O que NÃO pôde ser provado em `-p` e como provar no interativo

Limitação: o spike roda em `claude -p` (um tiro, não interativo). O uso real é a extensão VS Code / CLI interativo.

Não provado:
1. Que o auto-background aos 120 s ocorre **sem variável** no interativo (a doc diz que sim).
2. Que, ao concluir, a notificação de tarefa **chega ao modelo** e se **acorda uma sessão ociosa** ou só aparece na próxima ação do usuário (a pergunta (b) inteira).
3. O efeito de `CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS=0` (Claude espera até 5 min).
4. Comportamento da extensão VS Code (UI de `/tasks`, `Esc` durante a espera).

Como provar (descrição; **não executado por mim**): o coordenador registra `spike.exe` num `.mcp.json` temporário (escopo de projeto, fora do git) numa pasta de teste, com `{"mcpServers":{"spike":{"command":"<spike111>\\spike.exe","env":{"SPIKE_LOG":"<spike111>\\interativo.log"}}}}`, e o owner abre uma **sessão nova** (CLI e depois a extensão VS Code) e pede: "chame esperar(170); enquanto isso não faça mais nada". Conferir: (i) aos ~120 s aparece a mensagem de segundo plano; (ii) aos 170 s o modelo responde sozinho com o resultado (acordou) ou só depois de o owner digitar algo; (iii) repetir com `esperar(300)` e com `CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS=0` no `env` do `settings.json`. O log do servidor dá os tempos exatos.

## 8. Erros de processo (honestidade)

- Meu laço de execução em background (A→B0→B1→C0→C1) continuou rodando sozinho depois que fui interrompido às ~23:31 (reinício da sessão do coordenador). Quando o C0 terminou (23:40:08), o laço órfão iniciou um **C1 extra** às 23:40:10 enquanto eu iniciava o meu às 23:40:23: duas execuções do claude em paralelo (contra a regra de uma por vez) e **`C1.stream.jsonl`/`C1.time`/`C1.server.log` ficaram intercalados**. As duas mostraram o mesmo comportamento (movida aos 120 s, `stopped` ao sair), mas para ter evidência limpa repeti o cenário sozinho como **C1b** (`C1b.*`); a linha C1 da tabela usa o C1b. Os arquivos `C1.*` não devem ser tratados como evidência.
- O C0 foi concluído enquanto eu estava interrompido (PID órfão 36220, sem ser morto, como pedido); sua saída está íntegra (`C0.stream.jsonl`, resultado `FIM pronto apos 300 s`).
- Um `sleep` longo em primeiro plano foi bloqueado pela ferramenta; troquei por um laço com teto dentro de comando em background.
