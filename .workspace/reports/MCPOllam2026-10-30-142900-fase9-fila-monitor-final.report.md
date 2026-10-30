# Report final: Fase 9, fila de espera (gateway) e `/monitor` com SQLite

> **Data:** 2026-10-30, 14:29 (hora local, `Get-Date`). **Autor:** agente de documentação (Sonnet 5.5), a partir do plano, dos reports e do `git log`; os fatos do aceite 9.7 são os registrados pelo loop principal.
> **Plano de origem:** `.workspace/plans/MCPOllam2026-10-28-223938-fase9-fila-monitor.plan.md`.
> **Estado:** **CONCLUÍDA em 2026-10-30 (9.1 a 9.8).** A extensão da §11 (faixa CPU/RAM e repasse a secundário) **não foi implementada** e passa a ser a **Fase 10**, futura.
> **Sigilo:** só metadados; nenhuma senha, token ou conteúdo de documento neste report.
> **Sem commit:** este report e as edições dos dois planos ficam no working tree, para o owner commitar.

---

## 1. Resumo executivo

| Pergunta | Resposta |
| --- | --- |
| O que entrou em produção? | O binário `mcpollama-gateway` (fila + `/monitor` + SQLite), no servidor `10.100.2.2`, entre o nginx e o Ollama. Versão em produção: **`0.10.1-e0d273c`** |
| Desde quando? | Implantação em **2026-10-30 12:12:40**; ajustes até as 14:xx do mesmo dia |
| Como é o tráfego agora? | Cliente → nginx (TLS, Bearer, `limit_conn 12`) → gateway `127.0.0.1:11500` → Ollama `127.0.0.1:11434`. A porta 11434 deixou de ser acessível pela LAN |
| O que o administrador vê? | `/monitor`: em execução, fila, concluídos recentes e eventos (em abas), GPUs (W e limite de W), CPU/RAM por thread, modelos carregados e instalados. Atualização a 1 s por SSE |
| O que o administrador pode fazer? | Promover, despromover, remover (pedido em espera) e cancelar (pedido em execução), com login, CSRF e auditoria |
| Aceite 9.7? | Feito em 2026-10-30. Duas cargas reais (12 e 11 agentes) pela rota de produção, 22 respostas 200 e a ordem de prioridade conferida; as 4 ações do administrador provadas numa **instância de teste** (ver §5) |
| Segurança? | Revisão adversarial 9.5: 30 vetores, 12 achados (0 Alto), todos corrigidos. Depois da implantação, mais 5 achados, todos tratados (§6) |
| O que não foi feito? | A faixa CPU/RAM com segundo Ollama e o repasse a secundário (§11 do plano): **Fase 10**, decisão do owner de 2026-10-30 (fechar a Fase 9 antes) |

---

## 2. Arquitetura em produção

```
clientes (mcpollama em N agentes/estações, OpenClaude ia-local, curl)
   │ HTTPS :8443 (Bearer)         LAN ou Cloudflare Tunnel (cloudflared)
   ▼
nginx (TLS, Bearer, limit_conn 12 e 600 req/min por IP, 403 em /api/pull|push|create|delete|copy|blobs)
   ├── /api/* e /v1/*     ──► mcpollama-gateway 127.0.0.1:11500 ──► Ollama 127.0.0.1:11434
   └── /monitor           ──► gateway (HTML, JSON, SSE e ações). LAN: 200. Com CF-Connecting-IP (túnel): 403
                                 ├── fila em memória (faixas geração e embed, prioridade, justiça por IP)
                                 ├── gravador assíncrono ──► SQLite WAL /var/lib/mcpollama-gateway/gateway.db (só metadados)
                                 └── coletores: nvidia-smi + AER, CPU/RAM (/proc e /sys), modelos (/api/ps e /api/tags)
```

| Item | Valor em produção (fonte: unit, `MANUAL.md` e plano) |
| --- | --- |
| Modelo | `qwen3-ia` = alias do `qwen3.5:9b` (D8-3, 2026-10-29); embeddings `nomic-embed-text` |
| Slots | geração **1** (o Ollama roda essa arquitetura com 1 slot; a espera aparece na fila do gateway); embed **2** |
| Prioridade `alta` | aceita de toda a LAN `10.100.2.0/24` (`--priority-ips`); de outro IP vira `normal` em silêncio |
| Escalonamento de partida | 300 ms (`--start-stagger`), só na faixa de geração |
| Políticas (9.6a) | raciocínio **desligado** por padrão; liga por `think`, header `X-Mcpollama-Think` ou o nome `qwen3-ia-think`. `keep_alive` e `num_ctx` do cliente são removidos, assim como `num_gpu`, `main_gpu`, `low_vram`, `use_mmap`, `use_mlock` e `numa` |
| Banco | `/var/lib/mcpollama-gateway/gateway.db`, `mcpgw:mcpgw` 0600; retenção de `requests` 90 d e `gpu_samples` 7 d; auditoria e eventos sem expiração; teto de 3.000.000 linhas em `requests` |
| Unit systemd | `User=mcpgw`, `NoNewPrivileges`, `ProtectSystem=strict`, `ReadWritePaths=/var/lib/mcpollama-gateway`, `PrivateTmp`, `MemoryMax=1G`, `Restart=always` |
| Backups e rollback | Backups em `/root/srv-backup-20261028/fase96-*`; rollback com `bash <pasta>/rollback.sh <pasta>`; instalador `deploy/fase9-gateway/Instalar-Gateway.ps1` (modos Atualizar, Senha, Reverter) |
| Manual | `deploy/fase9-gateway/MANUAL.md` (15 seções) e `README.md` da mesma pasta |

---

## 3. Decisões do owner (D9-*)

| Id | Decisão | Data |
| --- | --- | --- |
| D9-1 | `/monitor` só na LAN, **ver sem login**; as **ações** exigem login do administrador | 2026-10-28 22:43 |
| D9-2 | Ollama só no loopback; todo o tráfego passa pela fila | 2026-10-28 22:43 |
| D9-3 | Fila de 64, 16 por cliente, espera máxima de 10 min, `embed` = 2. Slots de geração = **1**, porque o `qwen3.5:9b` roda em 1 slot (8.9) | 2026-10-28 e 2026-10-29 |
| D9-4 | Retenção: `requests` 90 d, `gpu_samples` 7 d, auditoria sem expiração | 2026-10-28 |
| D9-5 | Dependência nova `modernc.org/sqlite` (Go puro, sem cgo) | 2026-10-28 |
| D9-6 | Desenvolver em paralelo à 8.9 e implantar depois da escolha do modelo | 2026-10-28 22:43 |
| D9-7 | Escalonamento de partida de 300 ms | 2026-10-28 |
| D9-8 | Cancelar pedido em execução: sim; `alta` pedida pelo cliente só de IPs autorizados (de outro IP é ignorada); um único administrador | 2026-10-28 22:43 |
| D9-9 | Allowlist de modelos também no gateway | 2026-10-28 |
| Políticas | "Políticas antes, instalar uma vez só" (9.6a); `--priority-ips` = toda a LAN `10.100.2.0/24`; fechar a porta 11434 | 2026-10-29 e 2026-10-30 |
| Senha | O owner digitou e depois trocou a senha do admin (15 caracteres); **manteve** `/root/mcpgw_password.txt` em texto puro (root 0600) | 2026-10-30 |
| NAT | Restringir a regra 0 (WAN:8443) do MK-ESCRITORIO-02 a `PERMITIDOS` (aplicado às 12:38, com export salvo antes) | 2026-10-30 |
| Ordem das fases | **Fechar a Fase 9 antes da §11** (faixa CPU/RAM e repasse); a §11 vira a Fase 10 | 2026-10-30 |

---

## 4. Etapas 9.1 a 9.8

| Etapa | Resultado | Commits |
| --- | --- | --- |
| 9.0 | Decisões D9 e levantamento do SSOT (só leitura) | plano `ced50f8`; extensão §11 `eb920e2` |
| 9.1 `src/gateway/queue` | Faixas, prioridade, justiça por IP e por label, envelhecimento, limites, escalonamento, ações atômicas. 24 provas de unidade; fuzz 20 s sem falha. Correção do alerta do coordenador (o label forjado furava o limite) | `418b028`, `75a1999` |
| 9.2 `src/gateway/proxy` | `ReverseProxy` com streaming, contadores só do último chunk, rotas diretas e bloqueadas, cancelamento em execução. 20 testes | `b47d763` |
| 9.3 `store` e `gpu` | SQLite sem cgo (WAL, gravador em lote, retenção, `boot_id`); coletor de `nvidia-smi` e AER. Teste-sentinela: o conteúdo nunca chega ao `.db`, `-wal` ou `-shm` | `f6a9b9c` |
| 9.4 `monitor` e entrypoint | HTML embutido com CSP por hash, SSE, login PBKDF2, CSRF, auditoria; mudanças no mcpollama (label, tool, `X-Queue-Ms`, 503 e 409) | `315f996` |
| 9.5 Revisão adversarial (opus) | 30 vetores, 12 achados (5 Médios, 7 Baixos), todos corrigidos; fuzz de 10 min em 5 alvos, PASS; OCR com DLL real: 385 PASS, 0 SKIP | `3146afd`, `8814d7a`, `2f70849`; merge `fa8c95e` |
| 9.6a Políticas | Reescrita do corpo antes de enfileirar (raciocínio, `keep_alive`, `num_ctx`, opções); fuzz de 5 min, 7.029.893 execuções, PASS; 30 vetores da 9.5 reexecutados, 30 OK | `6d51d48` |
| 9.6 Implantação | Instalador e etapas 1 e 2, `MANUAL.md`, gateway ativo às 12:12:40, Ollama no loopback, nginx apontando ao gateway | `2cb599d`, `7e47707`, `f2d0162`, `6f7c4c2` |
| 9.6 Ajustes depois da implantação | CSP/CRLF, instalador, senha, NAT e Cloudflare, abas, quadro CPU/RAM e modelos | `7a86350`, `1e5e70c`, `00cf311`, `2a16888`, `dfc38ac`, `32b40ac`, `9d90ba6`, `ea4b4dc`, `e0d273c` |
| 9.7 Aceite ao vivo | Ver §5 | sem código novo |
| 9.8 Documentação | Este report; plano da Fase 9 e plano geral atualizados; `MANUAL.md` e `README.md` em `deploy/fase9-gateway/`. O `CLAUDE.md` já traz "Fase 9 CONCLUÍDA" e a Fase 10 futura (modificação ainda sem commit no working tree). O `CONEXAO-IA.md` não foi conferido nesta tarefa | este report |

Commits de apoio da Fase 8 que a Fase 9 usa: `e519221` (8.8, `think:false`), `26c8bc1` (8.8b, argumento `think` por chamada), `0e56779` (8.9, carga multiagente), `5a537e4` (CLAUDE.md com `qwen3-ia` = `qwen3.5:9b`).

---

## 5. Aceite 9.7 (2026-10-30)

Feito pelo loop principal. Todos os agentes de carga passaram pela rota real (nginx, gateway, Ollama).

| Prova | Quando | Condição | Resultado |
| --- | --- | --- | --- |
| **Carga 1** | 12:44–12:49 | 12 agentes. Prioridades: 05 e 11 `alta`; 03 e 07 `baixa`; os demais `normal` | **11 respostas 200**, 1.200 tokens cada. O agente-12 recebeu **429 do nginx** (`limit_conn 12` por IP): os 11 agentes mais o `/monitor` aberto no navegador da mesma estação somaram 12 conexões |
| Ordem de atendimento (carga 1) | idem | idem | 01 (chegou primeiro) → **05, 11** (alta; esperas de 21 s e 41 s) → **02, 04, 06, 08, 09, 10** (normal, por ordem de chegada; 68–178 s) → **03, 07** (baixa; 204 s e 225 s) |
| **Carga 2** | 14:05–14:10 | 11 agentes, `num_predict` 3000 (o modelo parou em ~1.400–2.000 tokens), mesma ordem de prioridade | **11×200**, espera de até 303 s. O owner não usou os botões a tempo: a auditoria da produção ficou com **0 ações** |
| **Ações do administrador** | 14:13 | **Instância de teste** do gateway (127.0.0.1:11599, banco e senha temporários e aleatórios, nunca exibidos), 8 agentes. A produção e a senha do owner não foram tocadas | ver as linhas abaixo |
| Login pela API | idem | | 200 |
| **Promover** | idem | agente-07: baixa → normal | passou à frente do 06 |
| **Despromover** | idem | agente-06: normal → baixa | atendido por último |
| **Remover** | idem | agente-08 (em espera) | o cliente recebeu **409** "removido da fila pelo administrador" |
| **Cancelar** | idem | pedido em execução (agente-01) | o cliente recebeu **409** "cancelado pelo administrador" |
| Sem sessão / sem CSRF | idem | | **401** / **403** |
| Auditoria | idem | | 4 ações, com usuário, IP, antes → depois e motivo |
| **2ª via, SQLite** | idem | consulta ao banco da instância de teste | status `cancelado_admin` e `removido`, `priority_in`/`priority_final` corretos, os demais `ok` |
| Limpeza | idem | | instância encerrada e apagada; porta 11599 liberada |

**Limites do aceite, ditos sem rodeio:**
1. As ações do administrador foram provadas por automação numa instância de teste, **não** pelo owner no navegador da produção (a carga 2 terminou sem que os botões fossem usados). O login na produção foi conferido separadamente (certa 200, errada 401).
2. Dois itens do desenho original da 9.7 não têm evidência registrada nesta rodada: o gateway reiniciado no meio da carga (e) e a medição do pico de W com e sem escalonamento (f). Ficam como pendência opcional (§9, item 9).
3. O 12º agente da mesma estação recebe 429 quando o `/monitor` está aberto. É o limite do nginx funcionando, não um erro do gateway; a decisão de subir o limite é do owner (pendência 3).

### 5.1 Ajustes de 2026-10-30 depois da implantação

| Ajuste | Commit | Observação |
| --- | --- | --- |
| CSP e CRLF do `/monitor` | `7a86350` | página sem estilo e parada em "conectando…" |
| Abas "Concluídos recentes" e "Eventos" | `9d90ba6` | a aba escolhida fica lembrada no navegador |
| Quadro CPU/RAM por thread, limite de W das GPUs, quadro Modelos, atualização a 1 s | `ea4b4dc` | medido no servidor: `nvidia-smi` **0,10 s** por leitura; gateway com **0,7% de CPU e 24 MB**; SSE com 1 mensagem/s; 12 threads e 6 núcleos com temperatura; limite de **100 W** nas 4 GPUs; modelos carregados e instalados com "permitido" |
| "permanente" no lugar de uma data de 2319 | `e0d273c` | modelo fixo na memória (`keep_alive -1`); versão em produção `0.10.1-e0d273c` |

---

## 6. Segurança

### 6.1 Revisão adversarial 9.5 (2026-10-28 23:24 a 2026-10-29 00:15)

Report: `.workspace/reports/MCPOllam2026-10-28-235633-revisao-adversarial-fase9.report.md`. Evidência em `….testes/`.

| Número | Valor |
| --- | --- |
| Vetores executáveis (VEC) | **30** |
| Achados | **12** (0 Alto, 5 Médios, 7 Baixos), **todos corrigidos**, cada um com regressão |
| Médios | M1 TOCTOU do login; M2 bloqueio global do administrador por 40 IPs; M3 SSE só com teto global; P6 sem `--run-timeout`; S2 histórico sem teto de linhas |
| Baixos | P2 rotação IPv6 no /64; C1 `--priority-ips ::ffff:`; Q1 registro perdido; S1 `Stats` sem índice (48,2 ms → 0,56 ms); S3 eventos de descarte; M8 `/stats` sem cache; C5 SIGTERM com painel aberto esperava 30 s |
| Sem achado (18 vetores) | `X-Real-IP`, smuggling (CL/TE), 17 variantes de rota bloqueada, contadores, vazamento de slot, inanição de `baixa`, corrida das ações, SQLi, sentinela, XSS, CSRF, fixação de sessão, `hash-password` |
| Fuzz de 10 min por alvo | 5 alvos, todos PASS (`FuzzConcurrentActions` 213.938; `FuzzFairIP` 6.972.864; `FuzzCountersFromModelText` 3.282.757; `FuzzParseNets` 3.729.010; `FuzzLoginLimiter` 3.165.797) |
| Reexecução dos ataques | depois da 9.6a e depois do quadro CPU/RAM: **29 a 30 VEC, todos OK**, 0 FAIL |

### 6.2 Achados depois da implantação (2026-10-30)

| Id | Achado | Causa | Correção e prova |
| --- | --- | --- | --- |
| N1 | O gateway recusava o arquivo de senha | Os scripts gravavam a senha com `0640`; o gateway só aceita `0600` | Scripts passaram a `mcpgw:mcpgw 0600`. Provado no servidor: 0640 → código 2; 0600 → `/monitor` 200 |
| CSP/CRLF | `/monitor` sem estilo e parado em "conectando…" | O binário saiu de uma cópia com `index.html` em CRLF; o gateway fazia o hash dos bytes brutos e o navegador faz o do texto em LF | `7a86350`: normaliza para LF antes do CSP. Regressão `csp_newline_test.go`: o código antigo não casa com CRLF e o novo casa. Conferido no Chrome do owner |
| `/monitor` pela internet (**grave**) | O `/monitor` respondia 200 pela internet | O túnel `cloudflared` entrega como `::1`, e a `location /monitor` liberava o loopback (viola a D9-1) | `2a16888`: 403 em `/monitor` para qualquer pedido com `CF-Connecting-IP`, e o loopback sai da LAN no `geo`. Prova: internet `/monitor` 200 → 403; `/api/tags` com token pela internet 200; LAN 200 |
| IP forjado via `CF-Connecting-IP` | Risco de o cliente da LAN se passar por outro IP (e herdar `alta`) | O nginx confiava no header | Correção no `ollama.conf`. Prova pela estação 10.100.2.30: `CF-Connecting-IP: 203.0.113.77` forjado da LAN foi registrado como `10.100.2.30` |
| Porta 11434 aberta na LAN sem token | Uma máquina `…da01::4` chamava o Ollama direto: **541** `/api/generate` em 27–28/10, sem token | Ollama escutando em `0.0.0.0` | **Fechada** na 9.6 (D9-2): `10.100.2.2:11434` → sem conexão. A máquina é a **TV LG Bancada** (`10.100.2.55`) |
| NAT regra 0 | WAN:8443 aberta a todos, usada só por varredores (recebiam 400/401) | Regra sem restrição | Restrita a `PERMITIDOS` às 12:38, com export salvo antes |

---

## 7. Erros de processo registrados no plano (sem embelezar)

| Etapa | Erro | Correção |
| --- | --- | --- |
| 9.1 | A 1ª versão dos arquivos foi recusada pela ferramenta (comando composto) | Recriados por Write, sem perda |
| 9.1 | `clientKey = ip+"\|"+label` deixava o label forjado furar o limite (achado do coordenador) | Rodízio em dois níveis, teto por IP e limite de labels por IP |
| 9.2 | Teste travou em `httptest.Server.Close` (Ollama falso não lia o corpo); `t.Fatal` em goroutine | Falso passou a ler o corpo; criado `postBg` |
| 9.3 | `Stats` misturava rejeitados no cálculo de espera; um teste de inundação dependia da velocidade da máquina | Espera só conta quem chegou a um slot; teste determinístico |
| 9.4 | A camada de exibição reescreveu o escape unicode nos testes (falso FAIL do XSS); `readSmallJSON` exigia `Content-Type` sem corpo; o hook de segurança acusou `eval(` por falso positivo | Texto montado por bytes; exigência só com corpo; falso positivo identificado |
| 9.5 | Vetor Q4 com prazo de 0 ms; regressão do Q1 passava no código antigo; regressão de SSE travou a suíte por 400 s; caso do O1 mal nomeado; BOM do PowerShell 5.1 num teste; 1ª exportação limpa sem o `TOOLS.md` | Todos corrigidos e provados na cópia do `315f996` |
| 9.6a | Dois scripts Python escreveram `\t\r\n` literal numa string Go; teste esperava 200 onde o código, certo, devolve 400; heredoc grande falhou | Ajuste com a ferramenta de edição; teste ajustado ao desenho |
| 9.6 | O instalador ficou parado pedindo senha numa sessão sem terminal (encerrado com `TaskStop` às 11:31, sem efeito no servidor) | Instalador roda no terminal do owner |
| 9.6 | **Corrida do reload do nginx:** o 1º teste caiu num worker antigo e o `grep` encerrou o script **depois** da ativação, sem rollback | `sleep 5` depois do reload; o `grep` dos headers deixou de ser fatal |
| 9.6 | O owner digitou 10 caracteres e o script exigia 12 (recusou "senha curta") | O script passou a mostrar a contagem |
| 9.6 | **O binário saiu de uma cópia com CRLF** e o `/monitor` ficou sem estilo | `7a86350` e teste de regressão |
| 9.6 | O PowerShell 5.1 removia as aspas do comando remoto e o `curl -w` se desfazia | `1e5e70c` |
| 9.6 | Um `cat >>` sem entrada travou um comando por 2 min e criou um arquivo vazio em `.workspace/plans/` | Apagado e refeito |
| Quadro CPU/RAM | Heredocs longos falharam duas vezes; helper `columns` colidiu com outro; os 2 testes de RAPL usam `intel-rapl:0` e o `:` é inválido no Windows; duas expectativas minhas erradas nos testes | Scripts e Write; renomeado para `tableColumns`; testes de RAPL em `Skip` no Windows; expectativas corrigidas |
| 9.7 | O owner não usou os botões a tempo na carga 2 (auditoria da produção com 0 ações) | As ações foram provadas numa instância de teste |

---

## 8. Números de operação

| Número | Valor | Fonte |
| --- | --- | --- |
| Gateway em produção | 0,7% de CPU, 24 MB; SSE 1 mensagem/s | medido no servidor, 2026-10-30 |
| `nvidia-smi` por leitura | 0,10 s | idem |
| CPU monitorada | 12 threads, 6 núcleos com temperatura | idem |
| GPUs | 4, limite de 100 W cada | idem; D8 e `CLAUDE.md` |
| Vazão do `qwen3.5:9b` (8.9) | ~700–790 tok/s somados (~2.000 com documentos grandes); 10 agentes × 32k com 0 erro; S2 80–85% e S3 100% | report `fase8-9-carga-multiagente` |
| `gemma4:12b` (8.9, para comparar) | 107–128 tok/s somados, 2 timeouts em 10×32k; S2 100% | idem |
| Carga 1 (aceite) | 11×200 em ~5 min; espera de `alta` 21–41 s, `normal` 68–178 s, `baixa` 204–225 s | aceite 9.7 |
| Carga 2 (aceite) | 11×200, espera de até 303 s | aceite 9.7 |
| Limite do nginx | `limit_conn 12` e 600 req/min com rajada de 60, por IP | `ollama.conf` |
| `Stats` com 207 mil linhas | 15 min: 48,2 → 0,56 ms; 7 d: 70,8 → 33,6 ms | 9.5 |
| Gate da última etapa de código | `go build`, `vet` e `gofmt` limpos; 17 pacotes `ok`; `-race` em 7 pacotes `ok`; build Linux ok | plano, entrada de 14:22 |
| Implantação | etapa 2 às 12:12:40; binário inicial `0.9.0-2cb599d`; produção `0.10.1-e0d273c` | plano |

---

## 9. Pendências (numeradas)

| # | Pendência | Dono | Tipo |
| --- | --- | --- | --- |
| 1 | Atualizar o `cloudflared` (2026.10.3 → 2026.10.6) | owner/admin | operação |
| 2 | O owner verificar o app de IA na **TV LG** (`10.100.2.55`), que perdeu o acesso direto à porta 11434 e deve usar `:8443` com token | owner | verificação |
| 3 | Decidir se sobe o `limit_conn` do nginx (12 por IP): o 12º agente da mesma estação recebe 429 quando o `/monitor` está aberto | owner | decisão |
| 4 | Trocar a fonte de 850 W (para voltar a 130–150 W); até lá, 4 GPUs a 100 W | owner | hardware |
| 5 | Opcional: importar a CA Cloudflare nas estações e usar o nome `ia.cslsolucoes.com.br`, para tirar o aviso "Não seguro" | owner | opcional |
| 6 | Opcional: nginx responder 403 antes do 401 para quem está fora da LAN e do Cloudflare | admin | opcional |
| 7 | Os testes RAPL (`gateway/cpu`) rodam só em Linux (`Skip` no Windows): rodar `go test ./gateway/cpu` em Linux | dev | teste |
| 8 | **Fase 10 (futura):** faixa CPU/RAM com segundo Ollama e repasse a secundário (§11 do plano da Fase 9) | owner | fase |
| 9 | Itens do desenho da 9.7 sem evidência registrada nesta rodada: gateway reiniciado no meio da carga; pico de W com e sem escalonamento de partida; validar o teto de 3 milhões de linhas e o `--run-timeout` de 10 min com volume real (9.5-7) | dev | opcional |
| 10 | Pendências herdadas e de baixo risco da 9.1 a 9.4: fairness só pelo último início (9.1-2) e `Retry-After` fixo em 5 s (9.1-1); `X-Queue-Position` não atualizada (9.2-2); 502 sem `Retry-After` (9.2-3); `Stats` na memória (9.3-2) e AER ausente como erro (9.3-4); teste de ponta a ponta mcpollama → gateway real (9.4-1); `Secure` do cookie exige HTTPS (9.4-2); login e SSE por IP completo, não /64 (9.5-3) | dev | opcional |
| 11 | O `CLAUDE.md` traz o estado novo, mas a modificação ainda está sem commit; o `CONEXAO-IA.md` (local e do servidor) não foi conferido nesta tarefa | owner | documentação |

Itens que **fecharam** durante o aceite: pendência 9.6-1 (aceite ao vivo), a senha do admin (trocada pelo owner), a identificação da máquina `…da01::4` (TV LG), e as medições do quadro CPU/RAM (custo do `nvidia-smi`, CPU do gateway, 12 threads e 6 núcleos, limite de 100 W). O arquivo `/root/mcpgw_password.txt` **permanece** por decisão do owner.

---

## 10. Fase futura: Fase 10 (faixa CPU/RAM e repasse a secundário)

**Estado:** **NÃO implementada.** Decisão do owner em 2026-10-30: fechar a Fase 9 antes dela. Proposta: chamá-la de **Fase 10**.

| Item | Conteúdo (§11 e §11.9 do plano da Fase 9) |
| --- | --- |
| Faixa CPU/RAM | Segunda instância `ollama-cpu` (127.0.0.1:11435, sem GPU), roteamento por pedido inteiro com as regras R1–R9 (`alta` sempre GPU; `baixa` na CPU se couber; `normal` transborda por espera estimada; roubo de trabalho pela GPU; queda da CPU leva tudo à GPU) |
| Repasse a secundário | Pool de backends, detecção de sobrecarga S1–S4, regras F1–F8, fallback no mcpollama (`--ollama-url-fallback`) |
| Medição antes de ligar | Velocidade, qualidade, interferência e **energia** (teto GPU+CPU ≈ 650 W, por causa da fonte de 850 W) |
| Etapas previstas | 9.9 a 9.16 do plano atual, a renumerar como 10.x |
| Decisões | D9-C1..C5 e D9-B1..B5 (a D9-B1, qual máquina é o secundário, destrava o remoto) |
| Dependência | O teto de energia da fonte de 850 W (pendência 4) é a restrição que a medição da §11.3 precisa respeitar |

---

## 11. Índice dos artefatos

| Artefato | Caminho |
| --- | --- |
| Plano da Fase 9 | `E:\MCPOllam\.workspace\plans\MCPOllam2026-10-28-223938-fase9-fila-monitor.plan.md` |
| Plano geral | `E:\MCPOllam\.workspace\plans\MCPOllam2026-10-23-150845-mcpollama-implementacao.plan.md` |
| Revisão adversarial 9.5 | `E:\MCPOllam\.workspace\reports\MCPOllam2026-10-28-235633-revisao-adversarial-fase9.report.md` |
| Carga multiagente 8.9 | `E:\MCPOllam\.workspace\reports\MCPOllam2026-10-28-235002-fase8-9-carga-multiagente.report.md` |
| Bake-off de modelos (Fase 8) | `E:\MCPOllam\.workspace\reports\MCPOllam2026-10-28-220428-fase8-bakeoff-modelos.report.md` |
| Manual e instalador | `E:\MCPOllam\deploy\fase9-gateway\MANUAL.md`, `README.md`, `Instalar-Gateway.ps1` |
| Código | `E:\MCPOllam\src\gateway\` (`queue`, `proxy`, `store`, `gpu`, `cpu`, `models`, `monitor`) e `E:\MCPOllam\src\cmd\mcpollama-gateway\` |
