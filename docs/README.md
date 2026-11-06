# Documentação

Guias de instalação e operação do mcpollama e do gateway. A visão geral do projeto está no [README.md](../README.md) da raiz.

| Documento | Para quê |
| --- | --- |
| [INSTALACAO.md](INSTALACAO.md) | Guia completo **do zero**: servidor de IA (Linux, nginx, Ollama, powercap, cloudflared), gateway, estação Windows com OCR e clientes MCP, com a conferência de cada etapa |
| [MANUAL-GATEWAY.md](MANUAL-GATEWAY.md) | Manual de operação do `mcpollama-gateway`: instalação e reinstalação, flags, fila, políticas, `/monitor`, API, SQLite, rotina, diagnóstico, rollback, segurança, pool de backends e fallback, e a Fase 12 (seção 17: `treino.db`, chave HMAC, aba Erros, bakeoff, rollback; implantada com o gateway 0.13.x) |
| [RELATAR-ERROS-IA.md](RELATAR-ERROS-IA.md) | Instruções para o Claude dos projetos relatarem os erros do modelo local (tool `reportar_erro`; em vigor desde 2026-11-04, com o mcpollama 0.10.0 e o gateway 0.13.0) |
| [CONTRATO-DADOS-TREINO.md](CONTRATO-DADOS-TREINO.md) | Contrato de dados da Fase 12 (1.1.7, aprovado): regras `C-x.y` testáveis para captura do pedido, `treino.db`, exportação, partição, avaliação, retenção, histórico e exemplos |

Relacionados:

- [`../deploy/`](../deploy/README.md): instalador e arquivos de configuração (gateway e servidor de IA).
- [`../TOOLS.md`](../TOOLS.md): catálogo das tools MCP (gerado do código).
- [`../CHANGELOG.md`](../CHANGELOG.md): histórico de versões.
- [`../CLAUDE.md`](../CLAUDE.md): guia do projeto para o Claude Code.
- `../.workspace/`: planos e reports históricos (decisões, medições, revisões adversariais).
