# Plano — Fase 11: jobs assíncronos, lotes e registro integral (HTTPS primeiro; WebSocket depois)

> **Reescrito em 2026-11-04 18:25** (ordem do owner: "reescreve o antigo atualizando data e hora"), a partir da conversa de 2026-11-04
> 16:56–17:51, do report de entendimento e da **avaliação independente do Codex (18:11)**:
> `.workspace/reports/MCPOllam2026-11-04-175900-fase11-reestruturacao-entendimento-para-revisao.report.md` (§1–§13 entendimento;
> §14 avaliação). Criado em 2026-10-30 22:45 com o título "túnel WebSocket entre o MCP da estação e o servidor da IA, com jobs
> assíncronos".
>
> **Estado:** 🧊 **CONGELADO — em redesenho.** Nada a executar sem ordem explícita do owner. Desde o congelamento (2026-10-30 23:50)
> nenhuma mudança de código, deploy ou servidor foi feita **por esta fase**. Só existe o spike 11.1 (2026-10-30, commit `f8c723e`).
> **Produção hoje (para referência, não é desta fase):** gateway `0.13.1-8f9cf6a` (0.13.2 pronto, não implantado), mcpollama
> `0.10.0-5372f4c`, Claude Code 2.1.296.
> **Versões-alvo propostas:** gateway `0.14.0` · mcpollama `0.11.0` (os números 0.13.0/0.10.0 do plano original já foram usados pela
> Fase 12).
> **Critério do owner que governa o desenho (2026-11-04 15:19):** qualidade e economia de IA paga **acima do tempo de execução**.
> **Cliente-alvo (2026-11-04 17:26):** Claude Code (extensão no Cursor); o owner não usa a IA do Cursor.

## 1. Objetivo e entendimento

### 1.1 Pedido original (2026-10-30 22:40)

"Um websocket para ter comunicação bidirecional, sendo o servidor da IA o servidor e a estação o MCP; o MCP não fica esperando o
response; quando a IA finalizar o job, envia para a estação solicitante."

### 1.2 Ampliação (2026-11-04, conversa 16:56–17:51; detalhe literal no report §6)

1. A fila hoje é **síncrona**: a resposta volta pela mesma conexão; com `--gen-slots 1`, um job por vez; se a IA desiste (timeout de
   120 s do mcpollama), o gateway **cancela** o job e o trabalho se perde; a regra global manda subir para IA paga → custo.
   Evidência medida hoje: 10 agentes × 3 tarefas no `gemma4:26b` → **25/30 ok, 5 timeouts a 300 s, p50 228 s**; no `gemma4:31b`
   **6/30 ok, 24 timeouts**.
2. O owner quer: **enviar jobs e lotes**, seguir trabalhando, **buscar os resultados mesmo no dia seguinte**, ser **avisado sem gastar
   tokens na espera**, e **registro de 100% do que entra e sai** com dados de projeto/sessão/usuário/modelo, sem perder informação
   (dentro de limites explícitos, ver D11-31).
3. Webhook não alcança a IA (stdio, NAT); WebSocket é **transporte**, não acorda o modelo; o que acorda o Claude Code é um **comando em
   segundo plano** ou **subagente** que termina enquanto a sessão está ativa (medido nesta sessão) e, no dia seguinte, um **hook
   `SessionStart`** que informa os lotes prontos. `claude --resume <sessão>` abre rodada nova automaticamente, mas **consome uso pago**
   (Codex §14.2).
4. A espera de uma ferramenta não consome tokens; consultas repetidas do modelo consomem (report §7.5).

### 1.3 Fronteiras (Codex §14.1, adotadas)

- **Job assíncrono** desacopla a geração da conexão original.
- **Aviso** (WebSocket/notificação MCP) entrega um evento a um processo; **não** cria rodada do modelo.
- **Nova rodada do Claude** só acontece se uma execução ativa receber uma tarefa concluída, ou se outra execução for iniciada.
- **Fase 11 ≠ Fase 13:** a Fase 11 resolve espera, persistência e retomada; não melhora a precisão nem reduz GPU. O aceite roda com o
  modelo escolhido na Fase 13 (não com servidor temporário de bake-off).

## 2. Decisões

### 2.1 Decisões do plano original (2026-10-30) e situação após o redesenho

| # | Decisão (resumo fiel) | Situação em 2026-11-04 |
| --- | --- | --- |
| D11-1 | Tools enviar/resultado: a tool devolve o ID na hora; o Claude busca com outra tool | **mantida** |
| D11-2 | Estação/sessão cai antes do fim → guardar e entregar ao voltar (não cancela) | **mantida**, reforçada por fila durável (D11-35) |
| D11-3 | Só o mcpollama usa WebSocket | **mantida para quando houver WS**; WS adiado (D11-33) |
| D11-4 | LAN (nginx :8443) e internet (túnel Cloudflare), mesmo Bearer | **ajustada**: mesmo caminho, com **token por usuário** (D11-32) |
| D11-5 | Resultado de job não terminado responde na hora com o estado; opcional esperar N s | **mantida** (espera longa por HTTPS, D11-33) |
| D11-6 | **Todas** as tools assíncronas (`generate`, `chat`, `summarize_file`, `extract`, `translate`, `embed`) devolvem ID | **mantida até o owner decidir o contrário** (Codex §14.4-8); a alternativa compatível `assincrono:true` é a pendência P11-7 |
| D11-7 | Quem busca = mesma máquina/usuário + ID | **substituída** por D11-32 (dono = token do usuário; mesma pessoa busca de qualquer máquina) |
| D11-8 | `list_models` síncrona | mantida |
| D11-9 | `embed` também assíncrona | mantida (segue D11-6) |
| D11-10 | Biblioteca `github.com/coder/websocket` | **adiada** com o WS (D11-33); se entrar, registrar no NOTICE |
| D11-11 | Segredo por job no perfil do usuário (`%LOCALAPPDATA%\mcpollama\jobs.json`), servidor guarda só o hash | **substituída** por D11-32 (token por usuário); segredo por job fica como opção futura de reforço |
| D11-12 | Resultado guardado 30 min para entrega | **substituída** por D11-31 (90 dias) |
| D11-13 | Mesma fila do gateway (prioridade, justiça, limites, pool, políticas) | mantida, + faixa de lote (D11-38) |
| D11-14 | Sem túnel WS, fallback HTTP em segundo plano local com o mesmo ID | **obsoleta**: o transporte principal já é HTTPS (D11-33) |
| D11-15 | Map-reduce como job composto **local** na estação | **reaberta** → D11-37 |
| D11-16 | Servidor empurra o `result` assim que conclui | **adiada** com o WS (D11-33) |
| D11-17 | Toda resposta de tool leva "jobs concluídos aguardando leitura: N" | mantida |
| D11-18 | Notificação do Windows (toast sem conteúdo, `--notify`) | mantida como opcional |
| D11-19 | `aguardar_jobs(ids[], timeout_s)` em ciclo (owner 5 min; spike recomendou 100 s, máx. 110 s) | **reaberta** → D11-36 (`--esperar` em segundo plano como padrão; ciclo ≤ 100 s como reserva) |
| D11-20 | Fallback HTTP com garantia menor | obsoleta (ver D11-14) |
| D11-21 | Limpeza do `jobs.json` local | obsoleta (sem `jobs.json`; D11-32) |
| D11-22 | Composto interrompido = "reenvie" | **reaberta** → D11-37 |
| D11-23 | Entrega "pelo menos uma vez"; `ack` quando o Claude lê; idempotência por ID; **nenhum conteúdo em disco na estação** | mantida (vale para HTTPS e, depois, WS) |
| D11-24 | Cancelar composto | segue D11-37 |
| D11-25 | Fallback com ≤ 2 HTTP simultâneos e 10 min por parte | obsoleta |
| D11-26 | Cancelamento idempotente e tolerante a cruzamento | mantida |
| D11-27 | Esperas sobre registro, sem erro espúrio na reconexão | mantida (adaptada a HTTPS) |
| D11-28 | `listar_jobs` só pendentes/não lidos, máx. 20, filtros | mantida, + filtros por lote/rótulo/data |
| D11-29 | Justiça por IP real + rótulo também no novo transporte | mantida, + dono por token (D11-32) |

### 2.2 Decisões novas do owner (2026-11-04 18:24, após a avaliação do Codex)

| # | Decisão | Escolha do owner |
| --- | --- | --- |
| D11-30 | **Texto extraído pelo OCR** é conteúdo (não metadado) | **guardar**, com o prazo do registro (D11-31), mesmo acesso (admin e dono); junto os metadados (hash do arquivo e por página, páginas totais/lidas, dimensões/DPI/megapixels, motor/versão/idioma/tessdata, confiança e tempo por página, se a imagem foi direto ao modelo). **Bytes da imagem não são guardados** (decisão de 17:49) |
| D11-31 | **"100% registrado" = cobertura de toda chamada aceita, dentro de limites explícitos** | **90 dias** (configurável), compactação mensal, **backup cifrado no TrueNAS**, **teto de disco com alerta** (ao atingir avisa e não apaga sozinho), todo corte por tamanho **marcado**; **exclusões expressas**: bytes de imagem e vetores de embedding |
| D11-32 | **Dono do job** | **token por usuário** (ou por estação) no gateway, revogável e rotacionável; o dono é o token; a mesma pessoa busca de qualquer máquina com o seu token. Cabeçalho `X-Mcpollama-Station` segue como informação declarada, **não** credencial |
| D11-33 | **Transporte** | **HTTPS primeiro** (envio, consulta com espera longa, cancelamento, listagem); **WebSocket depois**, só se um aceite provar que o aviso imediato agrega valor. Substitui por enquanto D11-16 e adia D11-10/D11-3 |

### 2.3 Decisões técnicas propostas (a confirmar na 11.0)

| # | Proposta | Motivo (Codex §14.3) |
| --- | --- | --- |
| D11-34 | **Uma fonte única de estado durável**: um banco novo do gateway (nome provisório `jobs.db`, 0600, `mcpgw`) é a **autoridade** de estado do job assíncrono/lote, dono, idempotência, tentativas, resultado e texto integral (90 dias). `gateway.db` (métricas) e `content.db` (log síncrono de 7 dias) passam a ser **projeções**; falha de projeção não altera o estado; reconciliação documentada | SQLite não faz transação entre arquivos; evitar máquina de estados duplicada (achado 4) |
| D11-35 | **Execução pelo menos uma vez, resultado exatamente uma vez**: chave de idempotência por envio, contador de tentativas, *lease* do job em execução, **resultado gravado de forma durável antes de anunciar conclusão**; job `interrompido_reboot`/`interrompido_gateway` volta à fila até N tentativas; duplicação de cálculo permitida, duplicação/mistura de resultado não | achado 5 |
| D11-36 | **Espera da IA**: padrão = `mcpollama-mcp.exe --esperar <job|lote>` em `Bash run_in_background` (zero tokens na espera; o Claude Code reativa o modelo ao terminar, com a sessão ativa); reserva = `job_resultado(aguardar_s ≤ 100)` (volta antes do auto-background de 120 s); dia seguinte = hook `SessionStart` com `--lotes-prontos` devolvendo **só IDs/estado/resumo mínimo**, nunca texto de documento | achados 14.2; spike 11.1 |
| D11-37 | **Extração e OCR continuam na estação.** Map-reduce: o mcpollama envia as partes de texto já extraídas e o gateway orquestra as partes e a junção (sobrevive ao fim da sessão), com vínculo a hash do arquivo, versão das instruções, IDs/ordem das partes e resultado da redução. O texto das partes é retido pelo prazo da D11-31 (aprovado em 18:24) | achado 8 |
| D11-38 | **Lotes**: prioridade baixa por padrão, **limite de lote admitido e ativo** (jobs por lote, lotes por dono, bytes), envelhecimento existente; sem preempção (um job de lote já iniciado não é interrompido): **medir a espera interativa no aceite** | achado 10 |
| D11-39 | **Fixar no job**: modelo resolvido + **digest**, `think`, contexto/parâmetros efetivos e versão da instrução da tool | achado 9 |
| D11-40 | **Fronteira do registro integral**: o gateway registra o corpo enviado ao Ollama e a resposta (incluindo `[raciocínio]`, que **já é gravado** por `respTee`, `src/gateway/proxy/journal.go:214-250`); o mcpollama envia **envelope** (tool, argumentos sem segredos, caminho e hash do arquivo, versão da tool, cliente/versão do `initialize`, projeto/diretório) e **fechamento** (texto final entregue ao Claude, estado de entrega, marcadores de truncamento). Sem tokens/segredos em nenhum registro | achados 2 e 7 |

## 3. Arquitetura alvo (rascunho)

```
Claude Code ──stdio──▶ mcpollama 0.11.0 ──HTTPS (token do usuário)──▶ nginx ──▶ gateway 0.14.0 ──▶ Ollama
     ▲                     │  --esperar <id> (processo local, long-poll)        │ jobs.db (autoridade: estado, dono, lote,
     │ notificação de      │  --lotes-prontos (hook SessionStart)               │   idempotência, tentativas, resultado, texto 90 d)
     └─ tarefa concluída ◀─┘                                                    │ gateway.db (métricas, projeção)
                                                                                │ content.db (log síncrono 7 d, projeção)
```

Endpoints HTTPS (rascunho, viram `docs/PROTOCOLO-JOBS.md`; todos exigem o token do usuário, dono = token):

| Método e caminho | Função |
| --- | --- |
| `POST /api/jobs` | envia um job (corpo do pedido + envelope D11-40 + `idempotency_key`); devolve `job_id`, posição, espera estimada |
| `GET /api/jobs/{id}?aguardar_s=N` | estado ou resultado; segura até concluir ou N s (≤ 100 por padrão do cliente) |
| `GET /api/jobs?estado=&lote=&rotulo=&desde=&limite=` | lista do dono |
| `POST /api/jobs/{id}/cancelar` | cancelamento idempotente (D11-26) |
| `POST /api/jobs/{id}/fechamento` | o mcpollama registra o texto final entregue ao Claude (D11-40) |
| `POST /api/lotes` · `GET /api/lotes/{id}` · `GET /api/lotes?prontos=1` | lotes |

Tools do mcpollama (D11-6 mantida): as tools atuais devolvem `{job_id, estado, posicao}`; novas `resultado_job(job_id, aguardar_s)`,
`listar_jobs(filtros)`, `cancelar_job(job_id)`, `enviar_lote(rotulo, pedidos[])`, `resultado_lote(lote_id, pagina)`. CLI
`--esperar <id>` e `--lotes-prontos`. Descrições das tools ensinam: enviar → seguir → esperar em segundo plano → ler. `TOOLS.md`
regenerado por `src/cmd/dumptools`.

## 4. Etapas

| # | Etapa | Modelo | Estado |
| --- | --- | --- | --- |
| 11.0 | Aprovação deste plano: confirmar D11-34…D11-40 e as pendências P11-x; contrato `docs/PROTOCOLO-JOBS.md` + contrato de registro (completude, limites, exclusões, truncamento, retenção, backup, disco cheio) | owner + opus | **pendente** |
| 11.1 | Spike de espera (2026-10-30, `claude -p`): **feito** (300 s sem background devolvem o resultado; background aos 120 s com `CLAUDE_AUTO_BACKGROUND_TASKS=1`; notificação final não provada em `-p`) | sonnet | **concluído (parcial)** |
| 11.1b | **Prova no Claude Code dentro do Cursor** (Codex §14.4-6): comando local de espera em segundo plano concluindo com o turno ativo; sessão fechada e reaberta com job pronto antes do `SessionStart`; medir o uso do Claude na conclusão; registrar versões | sonnet + owner | pendente |
| 11.2 | Tokens por usuário no gateway (emissão, revogação, rotação, nginx, `.mcp.json` das estações; compatibilidade com o token único atual durante a transição) | sonnet | pendente |
| 11.3 | `jobs.db` (D11-34): esquema, máquina de estados, idempotência, *lease*, tentativas (D11-35), projeções e reconciliação | sonnet | pendente |
| 11.4 | Endpoints HTTPS (§3) com espera longa, listagem e cancelamento; dono por token | sonnet | pendente |
| 11.5 | Registro integral (D11-30/31/40): envelope e fechamento, metadados e texto OCR, campos `TEXT` (tratar bytes inválidos), 90 dias, compactação mensal, teto com alerta, marcação de cortes, backup cifrado no TrueNAS | sonnet | pendente |
| 11.6 | Map-reduce orquestrado no gateway com partes extraídas na estação (D11-37) | sonnet | pendente |
| 11.7 | Lotes (D11-38): faixa, limites, rótulo, `enviar_lote`/`resultado_lote` | sonnet | pendente |
| 11.8 | mcpollama: tools assíncronas (D11-6), `--esperar`, `--lotes-prontos`, aviso D11-17, toast D11-18 opcional; fixar modelo/digest (D11-39) | sonnet | pendente |
| 11.9 | Hook `SessionStart` (modelo de configuração para as estações; só IDs/estado) | haiku | pendente |
| 11.10 | `/monitor`: jobs assíncronos, lotes, "pronto não buscado", tokens por usuário (sem expor segredo) | sonnet | pendente |
| 11.11 | Revisão adversarial (dono/IDOR, token revogado, idempotência, flood de lotes, lote gigante, disco cheio, envelope/fechamento mentiroso, reconciliação de projeções, reboot no meio) + testes de ataque guardados em `.workspace/reports/...testes/` | **opus** | pendente |
| 11.12 | Gate: build/vet/gofmt, `go test -count=1 ./...`, `-race`, build Linux, ataques 9/10.5/12/11 | sonnet | pendente |
| 11.13 | Implantação com rollback (instalador `-Modo Atualizar`), mcpollama em `D:\MCP`, reiniciar sessões | coordenador | pendente |
| 11.14 | Aceite ao vivo com o modelo escolhido na Fase 13: 30 jobs de uma vez; lote de madrugada e busca no dia seguinte; reboot do servidor no meio (job volta e conclui uma vez); outra pessoa não lê; espera interativa medida com lote ativo; via internet | coordenador + owner | pendente |
| 11.15 | Documentação: README, CLAUDE.md (projeto e global: novo fluxo do nível `local`), MANUAL-GATEWAY, `TOOLS.md`, `D:\MCP\Documentation\Ollama`, CHANGELOG; relatório final | sonnet | pendente |
| 11.16 | (Depois, condicional) WebSocket como otimização de aviso (D11-33), mantendo D11-23 | sonnet | condicional |
| 11.17 | (Independente) opção `-think` no bake-off e medição com raciocínio (útil a lotes) | sonnet | pendente |

## 5. Riscos

1. **Energia (fonte de 850 W):** lotes longos mantêm as GPUs sob carga por horas; reinícios já ocorreram; a fila durável mitiga perda,
   não a causa.
2. **1 slot sem preempção:** um job de lote iniciado segura o interativo (D11-38); medir.
3. **Disco:** 90 dias de texto integral + compactação; teto com alerta (D11-31).
4. **Dados de cliente acumulados** 90 dias + backup cifrado: LGPD, acesso só admin/dono.
5. **Transição de token único para token por usuário:** quebra de estações não atualizadas; período de convivência.
6. **Execução repetida** em reboot (custo de GPU), aceito por D11-35.
7. **Comportamento do cliente:** depende do Claude Code (auto-background, notificação de tarefas, hooks); mudanças de versão.
8. **Mudança incompatível das tools (D11-6):** sessões antigas seguem no síncrono; servidor aceita os dois.

## 6. Pendências numeradas (P11-x)

1. P11-1 — Confirmar D11-34…D11-40 (11.0).
2. P11-2 — Executar a prova 11.1b no Cursor + Claude Code.
3. P11-3 — Definir o limite de disco/alerta e o destino exato do backup cifrado no TrueNAS (caminho, chave, rotação).
4. P11-4 — Definir limites de lote (D11-38) e prioridade padrão.
5. P11-5 — Esquema de emissão/distribuição dos tokens por usuário (quem emite, como revoga, como a estação guarda — sem colocar em git).
6. P11-6 — Verificar se o Claude Code expõe ID de sessão e modelo da IA chamadora ao servidor MCP (para o envelope D11-40); hoje só o
   `clientInfo` do `initialize` é garantido.
7. P11-7 — Owner decidir se mantém D11-6 (tudo assíncrono, incompatível) ou adota `assincrono:true` + tools novas (compatível).
8. P11-8 — Atualizar o Claude Code para ≥ 2.1.304 (MCP *tasks*) e reavaliar o desenho.
9. P11-9 — Webhook para integrações com servidor (n8n/ERP): fora do escopo inicial.
10. P11-10 — `BLOB` × `TEXT` no `content.db`: confirmar o motivo do `BLOB` e a estratégia para bytes inválidos.

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
  uma vez", ack quando o Claude lê, reenvio automático e descarte de repetidos (D11-23, owner).
- 2026-10-30 23:25 — 2ª rodada da revisão externa (pontos 6–11): 6, 8, 9 e 10 procedem → D11-24, D11-26, D11-27, D11-28; 7 procede
  em parte → D11-25; 11 já é tratado no HTTP (`queue.clientKey`, `proxy.clientIP`, `$ollama_client_ip`) → estender (D11-29).
- 2026-10-30 23:46 — **11.1, parte "spike"** (`claude -p`, Claude Code 2.1.296, haiku); relatório
  `.workspace/reports/MCPOllam2026-10-30-234600-fase11-1-spike-espera-longa.report.md`. **Deu certo:** 40 s, 170 s e 300 s sem a
  variável voltaram íntegros; com `CLAUDE_AUTO_BACKGROUND_TASKS=1` a chamada foi para segundo plano aos ~120 s com aviso ao modelo.
  **Não provado:** se a notificação final acorda o modelo (o `-p` terminou antes). Erro de processo: laço órfão disparou um C1 extra
  em paralelo; repetido como C1b. **Proposta:** `aguardar_jobs` 100 s (máx. 110 s).
- 2026-10-30 23:50 — **CONGELADO** por decisão do owner; nada de código/implantação; plano movido para `.workspace/plans/congelados/`.
- 2026-11-04 16:56–17:51 — conversa com o owner (somente conversa, sem ação) sobre fila síncrona, timeout, webhook, WebSocket,
  raciocínio, notificação no Claude Code, custo de tokens da espera, consulta por ID, tools MCP, lotes no dia seguinte, hook
  `SessionStart`, armazenamento (`lotes.db`), estados de job, histórico com texto em `TEXT`, registro 100% e metadados de OCR.
  **Deu certo:** consolidado num report de entendimento (`...-175900-fase11-reestruturacao-entendimento-para-revisao.report.md`, 508
  linhas, varredura de segredos 0). **Não deu certo (erros meus, apontados pelo Codex):** (a) classifiquei o texto do OCR como
  metadado; (b) afirmei que o raciocínio não era registrado — é (`journal.go:214-250`); (c) propus `lotes.db` sem fonte única de
  estado; (d) propus "consulta de qualquer máquina" com dono por hash do token, que não separa usuários (todas as estações usam o
  mesmo token); (e) propus trocar a D11-6 por `assincrono:true` sem marcar como decisão do owner. **Solução:** §2.2/§2.3.
- 2026-11-04 18:11 — **avaliação independente do Codex** anexada ao report (§14): 10 achados, todos procedentes (conferidos no código
  os de raciocínio e do cabeçalho declarado); recomendações: separar texto OCR, definir "100%" com limites, fonte única de estado,
  idempotência e tentativas, dono por credencial real, fronteira do registro, map-reduce com retenção aprovada, desacoplar Fase 13,
  lote sem preempção, HTTPS antes do WebSocket, provar a espera no Cursor.
- 2026-11-04 18:24 — **decisões do owner:** D11-30 (guardar o texto OCR com o prazo do registro), D11-31 (90 dias + backup cifrado,
  teto com alerta, exclusões expressas), D11-32 (token por usuário), D11-33 (HTTPS primeiro, WebSocket depois).
- 2026-11-04 18:25 — **plano reescrito** neste arquivo (ordem do owner), título novo, decisões antigas com situação (§2.1), decisões
  novas (§2.2), propostas D11-34…D11-40 (§2.3), arquitetura e endpoints HTTPS (§3), etapas 11.0–11.17 (§4), riscos (§5) e
  pendências P11-1…P11-10 (§6). **Continua congelado**; próximo passo: 11.0 (aprovação) quando o owner mandar.
