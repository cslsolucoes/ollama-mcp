# Brief das referências do mcpollama

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.4.0 |
| **Data** | 2026-10-23 |
| **Origem** | plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md`, mesmo método de `E:\GestorERP\.workspace\referencias\` |

Síntese detalhada, rastreável e sem segredos das referências do **mcpollama** (servidor MCP em Go que expõe o Ollama da CSL como tools). Serve para levantar padrões já provados, armadilhas já pagas, contratos de tools, limites reais do servidor e GAPs, e alimentar as Fases 1 a 6 do plano `.workspace/plans/MCPOllam2026-10-23-150845-mcpollama-implementacao.plan.md`. Convenções em [00-CONVENCOES.md](00-CONVENCOES.md).

## Fontes

| Fonte | Tipo | O que dá |
| --- | --- | --- |
| `referencias/ollama-mcp-go/` | local | MCP mínimo para Ollama: `generate`/`chat`/`embed`/`list_models`, streaming interno, trace, negociação de versão, testes `httptest` |
| `referencias/04-what-is-mcp/` | local | exemplo didático de servidor MCP em Go |
| `referencias/MCPNas/truenas-mcp/` | local (CSL) | molde CSL: `--tls-ca`/`--insecure`, `proxy/` stdio↔HTTP, `tasks/`, lições em `.wolf/` |
| `referencias/MCPVMWare/` | local (CSL) | molde CSL: layout `src/`, `mcp/types.go` à mão, flags→env, `--tools-verbosity`, gate destrutivo, gerador `src/gen` → `TOOLS.md`, release |
| 7 links do `README.md` §5 | web | ideias de tools, transporte, configuração |
| `CONEXAO-IA.md`, `ia-local.ps1`, servidor ao vivo | infra CSL | endpoints, auth, limites, nível `local` atual |

## Dimensionamento (medido em 2026-10-23)

Linhas **físicas** (`wc -l`, incluem as brancas).

| Referência | Arquivos Go | Linhas de Go | Núcleo a analisar |
| --- | ---: | ---: | --- |
| `ollama-mcp-go` | 4 | 1.426 | tudo |
| `04-what-is-mcp` | 1 | 47 | tudo |
| `MCPNas/truenas-mcp` | 116 | 109.126 | `cmd/`, `mcp/`, `proxy/`, `tasks/`, `tools/` (amostrado por família) |
| `MCPVMWare/src` | 176 | 85.397 | 23 arquivos não-gerados e não-teste + `gen/` (134 `generated_*` entram só como mecanismo) |

## Índice e estado por onda

| Arquivo / pasta | Conteúdo | Onda | Estado |
| --- | --- | --- | --- |
| [00-CONVENCOES.md](00-CONVENCOES.md) | idioma, citação, segredos, IDs, áreas | B0 | feito |
| `ollama-mcp-go/` (6 docs) | README, 10, 20, 30, 40, 50 | B1 | feito |
| `what-is-mcp/` (3 docs) | README, 10, 20 | B1 | feito |
| `mcpvmware/` (7 docs) | README, 10, 20, 30, 40, 50, 80 | B2 | feito |
| `truenas-mcp/` (7 docs) | README, 10, 20, 30, 40, 50, 80 | B2 | feito |
| `Web/` (7 docs) | W1 a W7 | B3 | feito |
| `Servidor-Ollama-CSL/` (2 docs) | README + `dados/validacao-empirica.md` | B4 | feito |
| `60-Requisitos_mcpollama.md` | requisitos `MCPO-REQ-<AREA>-nnn` | B5 | feito |
| `61-Matriz_Cobertura.md` | requisito × fase (0–6) × referência, GAPs | B5 | feito |
| `Comparativo-Referencias-mcpollama.md` | tabela comparativa, o que tirar de cada, GAPs | B5 | feito |
| [70-Revisao.md](70-Revisao.md) | revisão adversarial e veredito | B6 | feito |

Total previsto: **38 documentos** (este README incluído).

## Changelog

- **1.4.0 (2026-10-23):** B7 (fecho). O defeito do servidor foi **diagnosticado pelo owner**: reboot da máquina por falha de PCIe na GPU 3, e não queda do `ollama`. Foi **mitigado** com 3 GPUs e `num_ctx` 65536. A 2ª via deste brief pelo cliente deu 42.540 caracteres = 15.959 tokens em 3,2 s (~4.988 tok/s de prefill), sem queda; ~2,67 caracteres por token. Atualizados `Servidor-Ollama-CSL/dados/validacao-empirica.md` (1.1.0, nova §9 e execuções 5 e 6), `Servidor-Ollama-CSL/README.md` e `61` (nota do PB5-1). A inferência anterior do brief (queda do serviço) foi registrada como errada.
- **1.3.0 (2026-10-23):** B6 (revisão adversarial, Opus) em [70-Revisao.md](70-Revisao.md). Veredito **COM RESSALVAS**, confiança alta (~85%). 84 citações e afirmações conferidas: 71 confirmadas, 4 imprecisas, 9 não sustentadas; das 64 citações de código, 62 exatas. Todas as recontagens batem (dimensionamento, 171 requisitos, 104/53/14, MUST por fase, lições 10/36/30, testes e tools); os números da validação batem com a saída bruta. Achados: 0 críticos, 8 médios, 12 menores, quase todos na coluna "Cobertura" do `61`, que estava otimista: recontada de 53/45/73 para **48/47/76** (resolvido/parcial/GAP). `PROTO-009` conferido na especificação MCP 2025-11-25 (tool desconhecida = erro de protocolo, `-32602`). Correções aplicadas em `60` (1.1.0), `61` (1.1.0), `Comparativo` (1.1.0), `truenas-mcp/README.md` (1.0.1, timeout de 120 s é do cliente WebSocket), `Web/W1` (1.0.1) e `validacao-empirica.md` (1.0.1, execução 4). 6 pendências `PB6-1` a `PB6-6`. Gate: `sources=15 loaded=25 allowlisted=1`, `files=38 known_hits=0` (exit 0); os genéricos novos são nomes de flag e de método, sem valor; 0 links quebrados; LF e sem BOM nos 38 documentos.
- **1.2.0 (2026-10-23):** B4 (servidor ao vivo: `qwen3-ia` com `num_ctx` 131072 e ~97 tok/s; **defeito aberto**: prompt de ~42 mil caracteres derruba o serviço) e B5 (171 requisitos, sendo 104 MUST, 53 SHOULD e 14 COULD; matriz com 73 GAPs; 13 pendências PB5-n; comparativo) concluídas. Ressalva "desatualizado" acrescentada nas 6 linhas que diziam ctx 32k; `ollama-mcp-go/30` alinhado à P-1 = não. Gate: `known_hits=0`, 37 documentos, todos em LF e sem BOM.
- **1.1.0 (2026-10-23):** B1 (`ollama-mcp-go`, `what-is-mcp`), B2 (`mcpvmware`, `truenas-mcp`) e B3 (`Web/`) concluídas: 30 documentos ao todo. Gate de segredos: `known_hits=0` (1 falso positivo triado por SHA-256 na allowlist). Detalhes e pendências no changelog do plano.
- **1.0.0 (2026-10-23):** B0. Criados `00-CONVENCOES.md` e este README (estrutura, dimensionamento medido, estado por onda).
