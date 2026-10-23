# 20 · Tools e contratos (what-is-mcp)

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B1 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

Fontes: caminhos relativos a `referencias/`. Linhas lidas em 2026-10-23.

## 1. Catálogo

O servidor expõe **uma** tool. Não há recursos (`resources`), prompts nem outras capacidades MCP.

| Tool | Descrição (`main.go:22`) | Parâmetros | Handler |
| --- | --- | --- | --- |
| `hello_world` | `Say hello to someone` | `name` (string, obrigatório) | `helloHandler` (`main.go:40`) |

## 2. Contrato de entrada

Declaração (`04-what-is-mcp/mcp-server/main.go:21-27`):

```go
mcp.NewTool("hello_world",
    mcp.WithDescription("Say hello to someone"),
    mcp.WithString("name",
        mcp.Required(),
        mcp.Description("Name of the person to greet"),
    ),
)
```

| Campo | Valor | Fonte |
| --- | --- | --- |
| Nome | `hello_world` | `main.go:21` |
| Descrição da tool | `Say hello to someone` | `main.go:22` |
| Parâmetro | `name` | `main.go:23` |
| Tipo | `string` | `main.go:23` (`mcp.WithString`) |
| Obrigatório | sim | `main.go:24` (`mcp.Required()`) |
| Descrição do parâmetro | `Name of the person to greet` | `main.go:25` |

`inputSchema` esperado (**inferência**: gerado pelo SDK `mark3labs/mcp-go` v0.8.2 a partir das opções acima; não foi capturado de uma execução nem lido no código do SDK):

```json
{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "Name of the person to greet"
    }
  },
  "required": ["name"]
}
```

O formato de `tools/list` (`name`, `description`, `inputSchema`) aparece no README só como ilustração, com `add_numbers` e `subtract_numbers` (`04-what-is-mcp/README.md:53-80`). Essas duas tools **não existem** no código.

## 3. Contrato de saída

Sucesso (`main.go:47`):

```go
mcp.NewToolResultText(fmt.Sprintf("Hello, %s!", name))
```

- Tipo do conteúdo: texto. Valor: `Hello, <name>!`, sem tratamento do valor de `name`.
- Exemplo: `name = "Ana"` → `Hello, Ana!` (inferência do formato de `fmt.Sprintf`).
- Não há campo estruturado, contador nem metadado.

## 4. Erros

| Situação | Comportamento | Fonte |
| --- | --- | --- |
| `name` não é string (ex.: número) | Resultado de tool com texto `name must be a string`, erro Go `nil` | `main.go:42-44` |
| `name` ausente | Mesmo caminho do caso anterior (`ok` = falso). Só inferência: o SDK pode barrar antes, por causa de `Required()` | `main.go:42-44`, `:24` |
| `name` vazio (`""`) | Aceito: `Hello, !` | `main.go:47` (inferência) |

- O erro é **resultado da tool**. O campo de erro JSON-RPC não é usado pelo handler (`main.go:44`, retorno `nil`). Se o SDK marca `isError` no resultado, isso é comportamento do SDK e não foi verificado (pendência em `10-Arquitetura.md`).
- Não há tratamento de contexto (`ctx` não é usado, `main.go:40`), timeout ou cancelamento.

## 5. Limites e segurança

- Nenhum limite de tamanho para `name`.
- Nenhuma validação além de tipo. Não há sanitização (o valor volta no texto).
- Não há escrita em disco, rede, nem chamada a serviço externo. Por isso a superfície é mínima.

## 6. O que não está documentado aqui

- As tools do servidor `mcp/sqlite` usadas nas demos (`README.md:203-225`, `:241-279`) estão no Docker Hub, fora deste repositório. Não foram coletadas nesta onda. Não se sabe quais tools ele expõe.
- Os exemplos `add_numbers` e `subtract_numbers` (`README.md:53-80`, `:84-133`) são ilustrativos e não têm implementação na pasta.

## 7. Comparação com o contrato do mcpollama

| Aspecto | what-is-mcp (`hello_world`) | mcpollama (alvo, `CLAUDE.md` e README do projeto) |
| --- | --- | --- |
| Número de tools | 1 | Várias, uma por arquivo em `src/tools/` |
| Parâmetros | 1 string obrigatória | Argumentos com tipos e padrões (`model`, `prompt`, `options`) |
| Saída | texto simples | Texto mais contadores do Ollama (`prompt_eval_count`, `eval_count`) para medir economia |
| Erros | resultado com texto | Resultado de tool com mensagem de domínio (padrão a seguir, ver `10-Arquitetura.md` seção 6, item 5) |
| Descrição | 1 frase em inglês | Descrição curta, com `--tools-verbosity full\|compact` para controlar o tamanho do `tools/list` |

Conclusão: este exemplo cobre só o formato de declaração e de retorno de texto. Não cobre contadores, catálogo grande, erros de rede ou de modelo, nem compactação do `tools/list`.
