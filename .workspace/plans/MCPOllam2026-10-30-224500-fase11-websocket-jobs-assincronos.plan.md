# Plano — Fase 11: túnel WebSocket entre o MCP da estação e o servidor da IA, com jobs assíncronos

> **Estado:** ✅ APROVADO pelo owner em 2026-10-30 23:25 ("podemos iniciar o plano?"). 11.1 (spike) em andamento.
> **Versões alvo:** gateway `0.13.0` · mcpollama `0.10.0` (mudança incompatível nas tools: passam a devolver ID).
> **Origem:** pedido do owner (2026-10-30 22:40): "um websocket para ter comunicação bidirecional, sendo o servidor da IA o
> servidor e a estação o MCP; o MCP não fica esperando o response; quando a IA finalizar o job, envia para a estação solicitante".

## 1. Entendimento fechado com o owner (perguntas de 22:41–22:44)

Fluxo combinado (palavras do owner): o túnel WebSocket fica aberto para comunicação bidirecional; o MCP da estação envia o job e
recebe o ID do job; o MCP fica escutando o servidor; a cada job concluído o servidor devolve o ID e o resultado; o MCP devolve a
quem solicitou.

| # | Decisão | Escolha do owner |
| --- | --- | --- |
| D11-1 | Como o Claude recebe o resultado | **Tools enviar/resultado**: a tool devolve o ID na hora; o Claude busca o resultado com outra tool |
| D11-2 | Estação ou sessão cai antes do fim | **Guardar e entregar ao voltar** (o job não é cancelado pela queda) |
| D11-3 | Quem usa o WebSocket | **Só o MCP** (mcpollama). ia-local, curl e outros continuam no HTTP de hoje, sem mudança |
| D11-4 | Por onde conecta | **LAN (nginx :8443) e internet (túnel Cloudflare)**, mesmo token Bearer |
| D11-5 | Tool de resultado com job ainda não terminado | **Responde na hora com o estado** (na fila, posição, rodando há); opcional esperar até N s se o Claude pedir |
| D11-6 | Tools atuais | **Tudo assíncrono**: `generate`, `chat`, `summarize_file`, `extract`, `translate` (e `embed`, ver D11-9) devolvem o ID |
| D11-7 | Quem busca um resultado guardado | **Mesma máquina/usuário + ID** |

Fatos verificados na documentação do Claude Code (2026-10-30, `code.claude.com/docs/en/mcp.md` e `channels.md`) que moldaram as
opções: (1) o modelo só vê um resultado quando uma tool devolve: um servidor MCP não acorda a sessão (os *channels* que fazem isso
estão em preview e só aceitam plugins de allowlist); (2) as *tasks* MCP assíncronas exigem Claude Code ≥ 2.1.304 (instalado:
2.1.296) — não escolhidas; (3) chamada MCP > 2 min vai sozinha para segundo plano; o corte real hoje é o `--timeout` 120 s do
mcpollama, e com a fila de 64 a espera pode passar disso — o modelo assíncrono resolve.

## 2. Decisões técnicas propostas (confirmar na aprovação)

| # | Proposta | Motivo |
| --- | --- | --- |
| D11-8 | `list_models` continua síncrona | não entra na fila; responde em ms |
| D11-9 | `embed` também vira assíncrona — **confirmado pelo owner (22:47)** | consistência com D11-6 |
| D11-10 | Biblioteca WebSocket em Go: `github.com/coder/websocket` (ex-nhooyr), nos dois lados | mantida, sem cgo, `context` nativo; a stdlib não tem WebSocket e `x/net/websocket` está desaconselhado. Nova dependência → registrar no NOTICE |
| D11-11 | **Confirmado pelo owner (22:47).** "Mesma máquina/usuário" garantido por **segredo por job**: o mcpollama gera um segredo aleatório por job, manda só o hash ao servidor e guarda o segredo num arquivo do perfil do usuário (`%LOCALAPPDATA%\mcpollama\jobs.json`, ACL só do usuário) | o rótulo `<máquina>/<usuário>` é declarado pelo cliente e qualquer um com o token poderia forjá-lo; o segredo no perfil é o que de fato prende o resultado àquela máquina e usuário, e sobrevive a reiniciar a sessão |
| D11-12 | **Confirmado pelo owner (22:47).** Resultado guardado para entrega por **30 min** depois do fim (configurável), em memória + SQLite do gateway; entregue = apagado da área de entrega (o `content.db` de 7 dias continua como hoje, só admin) | reaproveita o armazenamento existente (SSOT); limita o que fica no servidor |
| D11-13 | O job **entra na mesma fila** do gateway (prioridade, justiça por cliente, limites 64/128, pool de backends, políticas `think`/`keep_alive`) | uma fila só; o `/monitor` mostra os jobs do WebSocket como os demais |
| D11-14 | Se o WebSocket não conectar, o mcpollama roda o job pelo HTTP de hoje **em segundo plano local** e devolve o mesmo ID ao Claude | o contrato com o Claude (enviar/resultado) não depende do transporte; nada para de funcionar se o túnel cair |
| D11-16 | **Owner (23:03): entrega imediata.** Assim que o job termina, o servidor **empurra** `result` para a conexão da estação que pediu, sem esperar ser perguntado (a notificação do Windows sai nessa hora). O `ack_result` e a remoção no servidor seguem a D11-23 (quando o Claude lê); a retenção de 30 min (D11-12) é o teto | é o "envia para a estação assim que concluir" do owner |
| D11-17 | **Owner (23:05): aviso na próxima resposta.** Toda resposta de tool do mcpollama leva no fim "Jobs concluídos aguardando leitura: N (ids…)" enquanto houver resultado recebido e não lido; o `resultado_job` entrega na hora (já está na estação) | o Claude só vê algo quando chama uma tool; assim percebe sem perguntar ID a ID |
| D11-18 | **Owner (23:05): notificação do Windows.** Ao receber um resultado, o mcpollama mostra um aviso na área de notificação do Windows ("Job a1b2c3 concluído — extract, 12,4 s"). Sem dependência nova: via PowerShell/WinRT (`Windows.UI.Notifications`) em processo filho; flag `--notify` (padrão ligado no Windows, sem efeito no Linux); sem conteúdo do resultado no aviso (só id, tool, tempo, ok/erro) | o owner vê a conclusão mesmo sem olhar o Claude; o texto do aviso não expõe dados de cliente |
| D11-19 | **Owner (23:20), ponto 1 da revisão externa:** nova tool **`aguardar_jobs(ids[], timeout_s≤300)`**: bloqueia até **algum** job da lista terminar e volta **na hora** com o resultado; se der 5 min sem conclusão, devolve o estado de cada um ("na fila, posição N" / "rodando há X") e o Claude chama de novo, **a cada 5 min, até concluir**. As descrições das tools instruem: "sem outra tarefa, chame aguardar_jobs". Interação com o auto-background do Claude Code (chamada > 2 min vai para segundo plano, `CLAUDE_CODE_MCP_AUTO_BACKGROUND_MS`): **provar no spike da 11.1** se o modelo é avisado ao fim; se não for, usar timeout < 2 min por chamada ou parar e consultar o owner | resolve o "Claude ocioso nunca vê o aviso" (D11-17 sozinho não basta) |
| D11-20 | **Owner (23:20), ponto 2:** fallback HTTP **com garantia menor**. Sem túnel, o mcpollama roda o job por HTTP numa goroutine local e, ao terminar, **ele mesmo** dispara a notificação do Windows (D11-18) e o aviso (D11-17), com o mesmo ID. Não há push do servidor nem retenção (D11-2/D11-12): se o processo cair, o servidor cancela o job (499). A resposta da tool informa "modo HTTP, sem retenção" | o push (D11-16) só existe no WebSocket; o evento de conclusão no fallback é local |
| D11-21 | **Ponto 3 (correção, sem decisão do owner):** limpeza do registro local `jobs.json`: a entrada sai quando o Claude lê o resultado ou quando expira (fim + 30 min + 1 h de folga); compactação na partida; gravação atômica (arquivo temporário + rename); teto de entradas (ex.: 1000, as mais antigas saem primeiro) | evita crescimento sem fim do arquivo |
| D11-22 | **Owner (23:20), ponto 4:** job composto (map-reduce de arquivo grande) que estava em andamento quando o processo reiniciou fica **"interrompido — reenvie"**; o Claude reenvia. Registro local do composto: `{id, tool, caminho, sha256 do texto, partes[{sub_id, estado}], etapa}` só para listar/mostrar e cancelar os sub-jobs no servidor (`cancel`); sem retomada | simples e sem risco de juntar partes de versões diferentes do arquivo |
| D11-23 | **Owner (23:20), ponto 5:** entrega **"pelo menos uma vez"**. O `ack_result` só é enviado **quando o Claude lê** o resultado (`resultado_job`/`aguardar_jobs`); até lá o servidor guarda (máx. 30 min). Na (re)conexão, depois do `hello`/`resume` com os claims, o servidor **reenvia sozinho** todo resultado concluído e não confirmado daquela estação; a estação **descarta repetidos pelo ID** (idempotente). Nenhum conteúdo de resultado é gravado em disco na estação (fica só em memória do processo) | fecha a corrida de rede no envio do `result`; sem dados de cliente no disco da estação |
| D11-24 | **Revisão externa, ponto 6 (correção):** cancelar job composto. O registro local mapeia ID mestre → sub-IDs; `cancelar_job(mestre)` envia `cancel` a cada sub-job pendente e `ack_result` a cada sub-job já concluído (o servidor apaga da área de entrega); o mestre fica "cancelado" | sem lixo no servidor |
| D11-25 | **Ponto 7 (correção):** fallback HTTP de job composto não precisa de polling (cada parte é um pedido HTTP que espera a sua resposta, como hoje). Limites no fallback: **no máximo 2 pedidos HTTP simultâneos por processo** (a GPU atende 1 por vez) e **timeout de 10 min por parte** (o Claude não está bloqueado; o corte de 120 s era para a espera síncrona) | não esgota `limit_conn` do nginx nem corta por espera na fila |
| D11-26 | **Ponto 8 (correção):** cancelamento idempotente e tolerante a cruzamento. Estação: `result` de job marcado cancelado → descarta e envia `ack_result`. Servidor: `cancel` de job já concluído → responde `cancel_result {id, outcome:"ja_concluido"}` e apaga; de job desconhecido → `outcome:"desconhecido"`. Toda mensagem é idempotente por ID | pacotes cruzados não deixam estado pendurado |
| D11-27 | **Ponto 9 (correção):** esperas (`resultado_job` com `esperar_s`, `aguardar_jobs`) são sobre o **registro local em memória**, não sobre o túnel. Queda do túnel durante a espera: a reconexão (backoff 1→30 s + `resume`) corre em segundo plano; a espera devolve o resultado se ele chegar, ou, ao fim do prazo, o estado com "túnel reconectando". Erro só se, após reconectar, o servidor disser que o job não existe ("perdido") | sem erro espúrio nem bloqueio além do prazo pedido |
| D11-28 | **Ponto 10 (correção):** `listar_jobs` mostra só pendentes e não lidos (mais recentes primeiro), **no máximo 20**, com filtro opcional (`estado`, `desde`); jobs lidos saem do registro (D11-21) | não polui o contexto do Claude |
| D11-29 | **Ponto 11 (já tratado no HTTP; estender ao WebSocket):** a justiça da fila usa `IP real + rótulo` (`queue.clientKey`); o nginx calcula o IP real (`$ollama_client_ip`: `remote_addr` na LAN, `CF-Connecting-IP` no túnel) e o gateway só confia no `X-Real-IP` vindo do loopback (`proxy.clientIP`). A `location /api/ws` (11.3) repete `proxy_set_header X-Real-IP $ollama_client_ip`, os mesmos `limit_conn`/`limit_req` e o Bearer. Ressalva documentada: estações atrás de NAT (ex.: 10.100.2.1) dividem o limite de 64 por IP, mas seguem separadas na justiça pelo rótulo `<máquina>/<usuário>` | mesma regra de justiça nos dois transportes |
| D11-15 | `summarize_file`/`extract`/`translate` continuam lendo o arquivo **na estação** (o texto não sai do lado do cliente antes do envio); o map-reduce de arquivo grande é um **job composto** local: um ID para o Claude, vários sub-jobs no servidor | o arquivo só existe na estação; o servidor guarda cada sub-resultado (D11-12) |

## 3. Protocolo (rascunho, vira `docs/PROTOCOLO-WEBSOCKET.md`)

- Endpoint: `wss://ia.cslsolucoes.com.br:8443/api/ws` (LAN) e `wss://ia.cslsolucoes.com.br/api/ws` (internet). Auth: `Authorization: Bearer` no handshake (nginx, igual ao HTTP). Cabeçalhos `X-Mcpollama-Client` e versão.
- Mensagens JSON, uma por frame:
  - estação → servidor: `submit {ref, endpoint, body, claim_hash, priority?}` · `status {ids[]}` · `resume {ids[], claims[]}` · `cancel {id, claim}` · `ack_result {id}` · `ping`
  - servidor → estação: `accepted {ref, id, position}` · `rejected {ref, reason}` · `progress {id, state, position, running_ms}` (opcional, ao mudar) · `result {id, status, http_status, body, prompt_eval_count, eval_count}` · `cancel_result {id, outcome}` (D11-26) · `pong`
- Entrega "pelo menos uma vez" (D11-23): `ack_result` só quando o Claude lê; reenvio automático na reconexão; idempotência por ID.
- Keepalive: ping a cada 25 s (o Cloudflare fecha WebSocket ocioso por ~100 s). Reconexão com backoff (1 s → 30 s) e `resume` dos IDs pendentes.
- Limites: tamanho máximo de mensagem = `--max-prompt-chars`/corpo atual; jobs pendentes por conexão sujeitos aos limites da fila.

## 4. Tools do mcpollama (contrato com o Claude)

- `generate`, `chat`, `summarize_file`, `extract`, `translate`, `embed` → devolvem `{job_id, estado: "na_fila", posicao}` em ms.
- **Nova** `resultado_job(job_id, esperar_s?=0)` → `{estado, posicao?, rodando_ms?}` ou o resultado completo (mesmo formato de
  hoje, com os contadores `[uso]`); `esperar_s` até 60.
- **Nova** `aguardar_jobs(job_ids[], timeout_s≤300)` → volta assim que algum terminar (com o resultado) ou, aos 5 min, com o
  estado de cada um; "sem outra tarefa, chame de novo" (D11-19).
- **Nova** `listar_jobs()` → jobs desta máquina/usuário (pendentes e prontos para entrega).
- **Nova** `cancelar_job(job_id)`.
- Descrições das tools ensinam o Claude a seguir trabalhando e buscar depois; `TOOLS.md` regenerado por `src/cmd/dumptools`.

## 5. Etapas

| # | Etapa | Modelo | Estado |
| --- | --- | --- | --- |
| 11.0 | Aprovação deste plano e das D11-8…D11-29 | owner | **concluído** (2026-10-30 23:25) |
| 11.1 | **Spike primeiro (gate da fase):** um servidor MCP mínimo com uma tool que bloqueia 3 min, chamado de uma sessão real do Claude Code — provar (a) se a chamada vai para segundo plano aos 2 min, (b) se o modelo é avisado/acordado quando ela termina, (c) o comportamento com timeout de 5 min (D11-19). Resultado decide o timeout da `aguardar_jobs`. Depois: protocolo escrito (`docs/PROTOCOLO-WEBSOCKET.md`, com entrega "pelo menos uma vez", `resume` e reenvio automático — D11-23) + testes de contrato | sonnet | pendente |
| 11.2 | Gateway: endpoint `/api/ws`, ligação com a fila, jobs independentes da conexão, área de entrega 30 min, dono por `claim_hash`, `resume`, `cancel`, métricas | sonnet | pendente |
| 11.3 | nginx: `location /api/ws` (Upgrade/Connection, `proxy_read_timeout` longo, mesmos limites e Bearer); conferir o túnel Cloudflare | sonnet | pendente |
| 11.4 | mcpollama: cliente WebSocket persistente (reconexão, keepalive), recebimento imediato com `ack_result` (D11-16), registro local de jobs com segredo no perfil do usuário, fallback HTTP em segundo plano (D11-14), jobs compostos (D11-15), aviso na próxima resposta (D11-17) e notificação do Windows (D11-18) | sonnet | pendente |
| 11.5 | mcpollama: tools assíncronas + `resultado_job`/`listar_jobs`/`cancelar_job`; `TOOLS.md` | sonnet | pendente |
| 11.6 | `/monitor`: conexões WebSocket ativas por cliente; estado "aguardando entrega" | sonnet | pendente |
| 11.7 | Revisão adversarial (dono do resultado, forja de rótulo, `resume` com claim errado, flood de submit, mensagens gigantes, reconexão em massa) + testes de ataque guardados em `.workspace/reports/...testes/` | **opus** | pendente |
| 11.8 | Gate: build/vet/gofmt, `go test -count=1 ./...`, `-race`, build Linux, ataques 9.5/10.5/10.L/11.7 | sonnet | pendente |
| 11.9 | Implantação: gateway 0.13.0 (instalador), nginx, mcpollama 0.10.0 em `D:\MCP`; reiniciar sessões | coordenador | pendente |
| 11.10 | Aceite ao vivo: 30 jobs enviados de uma vez → 30 IDs na hora; cada conclusão chega na estação na hora (notificação do Windows + aviso na próxima resposta); Claude segue trabalhando; resultados buscados; queda de rede no meio (desligar Wi-Fi/cabo) → resultado entregue ao voltar; reiniciar a sessão do Claude → `listar_jobs` recupera; via internet (Cloudflare); outra máquina/usuário NÃO lê o resultado | coordenador + owner | pendente |
| 11.11 | Documentação: README, CLAUDE.md, MANUAL-GATEWAY, CHANGELOG, **CLAUDE.md global** (a seção do nível `local` ensina o uso das tools: passa a ser enviar → seguir → `resultado_job`), CONEXAO-IA; relatório final | sonnet | pendente |

## 6. Riscos e pendências conhecidas

1. Mudança incompatível: sessões com mcpollama ≤ 0.9.x continuam no HTTP síncrono (servidor aceita os dois); o GestorERP precisa
   reiniciar para pegar o 0.10.0.
2. O modelo precisa lembrar de buscar o resultado (D11-1). Mitigação: a resposta do envio diz "busque com resultado_job"; o
   `listar_jobs` mostra o que ficou pendente.
3. Job composto (map-reduce): se o processo reiniciar no meio, fica "interrompido — reenvie" (D11-22); os sub-jobs pendentes são
   cancelados no servidor.
6. A `aguardar_jobs` depende de como o Claude Code trata chamadas longas (auto-background aos 2 min); o spike da 11.1 é o gate:
   se o modelo não for avisado no fim, o timeout por chamada fica abaixo de 2 min ou o owner é consultado.
4. Dados sensíveis: o resultado fica até 30 min na área de entrega do gateway, além do `content.db` (7 dias, só admin) que já
   existe. A área de entrega segue a mesma regra de backup pendente (excluir ou cifrar).
5. Cloudflare: WebSocket pelo túnel precisa estar liberado na zona (padrão: liberado); conferir na 11.3.

## 7. Changelog do plano

- 2026-10-30 22:45 — criado após o fechamento do entendimento com o owner (D11-1…D11-7). Decisões técnicas D11-8…D11-15 propostas,
  aguardando aprovação.
- 2026-10-30 22:47 — owner confirmou D11-11 (segredo por job no perfil), D11-12 (30 min) e D11-9 (**embed assíncrono também**).
  Pediu **ajustes antes de aprovar** o plano; aguardando quais.
- 2026-10-30 23:05 — ajuste do owner: o servidor **envia o resultado à estação assim que o job conclui** (D11-16); a retenção de
  30 min fica só para estação desconectada. Avisos escolhidos: na próxima resposta de tool (D11-17) e notificação do Windows
  (D11-18); o `resultado_job` entrega o que já chegou.
- 2026-10-30 23:20 — revisão externa (outra IA, trazida pelo owner) apontou 5 lacunas; as 5 procedem. Correções: (1) Claude ocioso
  → `aguardar_jobs` com ciclo de 5 min escolhido pelo owner (D11-19) + spike obrigatório na 11.1; (2) fallback HTTP sem push →
  conclusão local dispara aviso/notificação, sem retenção (D11-20, owner); (3) `jobs.json` sem limpeza → D11-21; (4) job composto
  sem modelo de dados → "interrompido — reenvie" + registro mínimo (D11-22, owner); (5) corrida no reconectar → entrega "pelo menos
  uma vez", ack quando o Claude lê, reenvio automático e descarte de repetidos (D11-23, owner). Risco 3 da §6 substituído pela D11-22.
- 2026-10-30 23:25 — 2ª rodada da revisão externa (pontos 6–11): 6, 8, 9 e 10 procedem → D11-24, D11-26, D11-27, D11-28; 7 procede
  em parte (não precisa de polling; o risco real é conexões e o corte de 120 s) → D11-25; 11 já é tratado no HTTP (conferido em
  `src/gateway/queue/queue.go` `clientKey` e `src/gateway/proxy/proxy.go` `clientIP` + `ollama.conf` `$ollama_client_ip`) →
  estender ao WebSocket (D11-29). Protocolo da §3 ganha `cancel_result`.
