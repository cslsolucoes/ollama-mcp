# Report final — Fase 10 (log de jobs, pool de backends e fallback; faixa CPU descartada)

- **Data:** 2026-10-30 (execução de 14:48 a 20:50, hora local).
- **Estado:** 10.1 a 10.8 **concluídas**. Plano **congelado** por decisão do owner ("finalizar a fase até onde é possível, e
  congelar o plano"). 10.S (secundário) **pendente, sem data**.
- **Plano (congelado):** [`.workspace/plans/congelados/MCPOllam2026-10-30-144800-fase10-faixa-cpu-pool.plan.md`](../plans/congelados/MCPOllam2026-10-30-144800-fase10-faixa-cpu-pool.plan.md)
  (changelog detalhado por etapa e a lista numerada "Pendências sem data (ao descongelar)").
- **Em produção ao final:** gateway **`0.12.1-c4031f7`** (desde 20:46) e mcpollama **`0.9.0-c4031f7`** gerado em
  `D:\MCP\mcpollama-mcp.exe` (vale após reiniciar as sessões do Claude Code). Tags `gateway-v0.12.1` e `mcpollama-v0.9.0`
  criadas depois deste commit, em `c4031f7`.

## 1. Relatórios da Fase 10

| Etapa | Relatório |
| --- | --- |
| 10.1 e 10.2: medição da CPU, interferência e energia | [fase10-1-medicao-cpu](MCPOllam2026-10-30-161904-fase10-1-medicao-cpu.report.md) |
| 10.L: revisão adversarial do log de jobs | [revisao-adversarial-10L](MCPOllam2026-10-30-173000-revisao-adversarial-10L.report.md) (testes em `…10L.testes/`) |
| 10.5: revisão adversarial do pool | [revisao-adversarial-10-5-pool](MCPOllam2026-10-30-190200-revisao-adversarial-10-5-pool.report.md) (testes em `…10-5-pool.testes/`) |
| 10.7: aceite do pool em instância de teste | [fase10-7-aceite-pool](MCPOllam2026-10-30-200400-fase10-7-aceite-pool.report.md) (evidências em `…aceite-pool.testes/`) |
| 10.8 e fechamento | este relatório |

(Não há relatório próprio da 10.4 nem da 10.6: ficam no changelog do plano.)

## 2. O que cada etapa entregou

### 10.1 e 10.2 e a decisão 10.3: CPU medida e descartada
Medição com uma `ollama-cpu` temporária (porta 11435, sem tráfego de produção). Números do relatório da 10.1/10.2:
- a CPU (i5-10500T) é limitada por **potência**: o pacote fica cravado em **35 W** (PL1), com pico curto de 56,8 W; mais
  threads não ajudam (6 threads é o melhor);
- `qwen3-ia` na CPU: prefill de **~24 tok/s** (~95× mais lento que a GPU) e geração de **~4,6 tok/s** (~11× mais lenta); uma
  extração S2 de ~4,9 mil tokens leva ~5 min na CPU contra ~10 s na GPU;
- qualidade equivalente: S2 84,8% na CPU contra 86,4% na GPU; S3 3/3 JSON estritos nas duas;
- a carga na CPU tira de **4% a 10%** da geração da GPU (−10% sem afinidade; −4,3% com `CPUAffinity=2-5 8-11`); pico somado
  GPU + CPU de 391,6 W, abaixo do teto de 650 W;
- conclusão: saldo de vazão ~0. **Decisão do owner: faixa CPU descartada** (10.3); só o pool pronto para um secundário.

### 10.L: log de jobs com conteúdo completo
Em produção desde o gateway `0.11.0`/`0.11.1`. Grava **prompt e resposta completos** em `content.db` (modo 0600, 7 dias;
texto só para o admin) e um extrato das GPUs a 1 s por job. A revisão adversarial achou **5 Médios e 5 Baixos, todos
corrigidos** (commits `cd71ed8` e `998a70d`), sem nenhum Alto: entre eles, H1 (hash do prompt e tamanhos exatos em rota
aberta) e H2 (bytes exatos nas rotas abertas, hoje em KiB), D4/D6 (teto de tamanho do `content.db`).

### 10.4 e 10.5: pool de backends e revisão adversarial
Pool de backends: registro `backends.json` (0600, lido só na partida), saúde ativa e passiva, **disjuntor** (só falha de
conexão e checagem ativa contam), repasse por sobrecarga (regras de elegibilidade F1–F8; sobrecarga S1 = espera estimada,
S2 = fila, S3 = primário sem saúde), **histerese** na volta (`--overload-hold`, 30 s com a carga abaixo de 50%), **drenar e
retomar** pelo monitor (sessão + CSRF + mesma origem), quadro **Backends**, tabela `backend_samples` e coluna
`failover_rule`. Não há repasse durante a execução. O mcpollama ganhou o **fallback** opcional (`--ollama-url-fallback`,
`--fallback-api-key`, `--fallback-tls-ca`), tentado uma vez em erro de conexão ou 503 de fila cheia; desligado por padrão.
Revisão 10.5: 26 vetores novos; no código anterior **7 falhavam** (B1, B3, S1, S2, T2, FB1, N1), no corrigido (`b1d8418`)
0 falhas; os ataques das fases 9.5, 10.L e 10.5 somaram **79 testes completos, 0 falhas**.

### 10.6: implantação do pool
Às 19:43: backup do `gateway.db` e do `content.db` pela API de backup do SQLite; gateway `0.12.0-736c205` pelo
instalador (`-Modo Atualizar`); conferido o pool com um único `gpu-local` (ok, disjuntor fechado, 100% GPU), um pedido real
200 com `backend=gpu-local` e a migração aditiva (`failover_rule`, `backend_samples`) aplicada. Tag `gateway-v0.12.0`.

### 10.7: aceite em instância de teste
Gateway de teste com o mesmo binário da produção, secundário temporário na 11435; produção intacta (provada na §5 do
relatório), boot `2026-10-30 16:42:03` do começo ao fim. **Todos os cenários PASS:** testes Linux como `mcpgw` (modo de
arquivo, kill no meio do stream), secundário 100% CPU sem tocar GPU, repasse por sobrecarga, volta com histerese
(`overload_off` 11,3 s depois do critério), disjuntor (`breaker_close` 20,0 s depois da abertura, igual ao
`--breaker-open`), drenar e retomar com negativos (sem sessão 401, etc.), `alta` nunca vai a secundário `cpu` (e vai a um
`remoto`), fallback do mcpollama (dispara em erro de conexão, não em 4xx), e custo da checagem ativa (340 amostras com a GPU
gerando: `/api/ps` p50 0,6–0,8 ms, máx 10 ms). Achado principal: 10.7-1, a S1 contava o próprio pedido.

### Correções pós-aceite (`4a54c8e`, `c4031f7`)
- **10.7-1:** S1 e S2 deixam de contar o próprio pedido em despacho (regressões `TestS1DoesNotCountTheArrivingRequest`,
  `TestS2IgnoresRequestsThatFitInFreeSlots`, que falhavam antes e passam depois).
- "Último erro" do backend limpo ao ficar saudável; eventos `breaker_open`/`breaker_close` limitados a 1 por minuto por
  backend, com contagem de suprimidos.
- mcpollama: 403 do gateway com JSON "não permitido neste servidor" vira `ollama.ErrModelNotAllowed` ("modelo não permitido
  pelo servidor"); o 403 HTML do nginx não mudou.
- `TestConnectionDroppedMidStream` determinístico (antes falhava em 2 de 12 processos × 150 execuções simultâneas; depois
  0 de 12) e novo `TestConnectionClosedMidStreamKeepsPartial`.

### 10.8: implantação e fechamento
1. **Gate no `master` `c4031f7`:** `go build` e `go vet` ok, `gofmt` limpo, `go test -count=1 ./...` com **20 pacotes ok, 0
   FAIL** (antes, o agente de correções fez 3 rodadas, `-race`, build Linux e os ataques 9.5/10.L/10.5 PASS).
2. **Gateway `0.12.1-c4031f7` implantado às 20:46** por `deploy/gateway/Instalar-Gateway.ps1 -Modo Atualizar`;
   `gateway_api_version=200`, `gateway_monitor=200`; `/monitor/api/state` relata `version 0.12.1-c4031f7`.
3. **mcpollama `0.9.0-c4031f7`** gerado em `D:\MCP\mcpollama-mcp.exe` (`--version` confere).
4. **2ª via independente (20:5x, pelo alias `lx-ai01-root`):** boot ainda 2026-10-30 16:42:03 (sem reboot);
   `mcpollama-gateway`, `nginx` e `ollama` active; `mcpollama-gateway --version` = `0.12.1-c4031f7`; `qwen3-ia` e
   `nomic-embed-text` residentes; power limit 100 W.
5. **Limpeza:** worktree `agent-a9532…` removido; ramo `wip/fase10-4-pool` apagado (commit `d38fb61` preservado na tag
   **só local** `archive/wip-fase10-4-pool`).
6. Documentação atualizada: `CHANGELOG.md`, `README.md`, `CLAUDE.md`, `docs/MANUAL-GATEWAY.md`, plano geral e plano da Fase 10.

## 3. O que NÃO deu certo (erros de processo, sem embelezar)

1. **Incidente de SSH depois da instalação (erro meu, do coordenador).** O SSH ao 10.100.2.2 falhou/travou e a primeira
   leitura foi "banner exchange: Unknown error" com a porta 22 aberta e o HTTPS ok, o que sugeriu throttling do
   sshd/fail2ban. **Isso estava errado.** A causa foi usar `ssh root@10.100.2.2` direto em vez do alias `lx-ai01-root` do
   `~/.ssh/config`, que tem a chave: sem a chave o ssh caiu no prompt de senha e travou. Pelo alias a verificação funcionou
   de primeira, sem ban e sem reboot. Lição: conferir o alias antes de culpar o fail2ban.
2. Erros dos agentes já registrados nos relatórios de origem: rodada A da 10.7 inválida (prompt curto demais, corrigida
   com A2), veredito errado do driver nos cenários a e e (conferido pelo quadro de 1 Hz e pelo SQLite), e a primeira edição
   do teste instável que gravou uma quebra de linha dentro de uma string Go (o `gofmt` acusou antes de rodar).

## 4. Pendências sem data

Lista numerada completa no fim do plano congelado. Resumo: (1) 10.S secundário i5-10500T + 4× RTX 2060 6 GB (IP/SSH do
owner, mesmo digest do `qwen3-ia`, nginx com token próprio, `backends.json`, ligar o fallback nos `.mcp.json`, aceite com
carga real, e medir a carga a frio do primário); (2) segunda fonte (revalidar 130–150 W e avaliar tirar a trava de clock);
(3) backup do `content.db`; (4) divisão da RAM 64/32 GB; (5) arredondar tokens nas rotas abertas (opcional); (6) herdadas
da 10.4/10.5/10.7 (`max_prompt_tokens`, `host:porta` no monitor aberto, S4, 5xx passivo, nota do MANUAL §16.1, backup do
`gateway.db`).
