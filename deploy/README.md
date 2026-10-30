# Deploy

Arquivos de implantação do servidor de IA da CSL (`10.100.2.2`, `lx-ai01`). Os passos completos estão em
[`../docs/INSTALACAO.md`](../docs/INSTALACAO.md) e a operação do gateway em [`../docs/MANUAL-GATEWAY.md`](../docs/MANUAL-GATEWAY.md).

| Pasta | Conteúdo |
| --- | --- |
| [`gateway/`](gateway/README.md) | Instalador `Instalar-Gateway.ps1` (roda no PC), scripts de etapa (`stage1.sh`, `stage2.sh`, `rollback.sh`, `mcpgw-definir-senha.sh`), a unit `mcpollama-gateway.service` e o `ollama.conf` do nginx (fonte da configuração de produção) |
| [`servidor-ia/`](servidor-ia/README.md) | Cópias das configurações do servidor, coletadas em 2026-10-30: Ollama, powercap, warm, nginx, cloudflared. **Sem segredos** (token mascarado) |

Regras comuns:

- Os arquivos usam finais de linha **LF**; envie ao Linux sem converter para CRLF.
- Nunca grave token, senha, chave de certificado nem token do túnel aqui.
- `servidor-ia/` são **cópias**; a produção é o servidor. Se alterar lá, atualize aqui.
