# Plano — Fase 12: o MCP organiza os erros do modelo local e gera o histórico para treinar o servidor de IA

> **Estado:** 📝 PLANO, aguardando aprovação do owner (2026-11-01 21:35). Nada implementado.
> **Versões alvo:** mcpollama `0.10.0` (tool nova + ID do job na resposta) e gateway `0.13.0` (base de treino + aba Erros).
> **Origem (owner, 2026-11-01 21:31):** "preparar o MCP para se organizar e gerar as informações necessárias para que possamos
> evoluir e treinar a IA para que determinados erros não ocorram", mais um md que instrua o Claude dos projetos a relatar os erros e
> o que foi feito para corrigir, gerando histórico para treinar o servidor de IA.

## 1. Objetivo e princípio

O modelo (`qwen3-ia` = `qwen3.5:9b`) não aprende sozinho. Ele evolui em **ciclos**: erro no uso → correção registrada → base de
treino → ajuste fino (LoRA) → avaliação → troca do modelo se ganhar. Esta fase faz **a parte que falta hoje: o registro**. Cada erro
do modelo local vira um **exemplo de treino completo**:
- **pedido exato** que o modelo recebeu, que o gateway já guarda no `content.db` por 7 dias;
- **resposta errada**, também já guardada;
- **resposta certa**, **categoria do erro** e **o que foi feito**, que hoje **não existem**.

O treino em si (LoRA) fica para a §6, **congelado** até haver volume de exemplos e hardware.

## 2. O que já existe (SSOT, reaproveitar)

| Peça | Onde | Uso nesta fase |
| --- | --- | --- |
| Pedido e resposta completos de cada job, por 7 dias | gateway `src/gateway/content` (`content.db`, 0600) | fonte do pedido exato e da resposta errada |
| ID do job | gateway devolve `X-Request-Id` (`proxy.go:417`) | chave que liga o erro ao job; o mcpollama **ainda não repassa** ao Claude |
| Máquina, usuário, domínio e MAC por job | gateway 0.12.5 (`station_*`) | quem reportou e de onde |
| Linha `[uso]` no fim de cada resposta de tool | mcpollama `src/tools/usage.go` | lugar para mostrar o `job=<id>` |
| Corpus com gabarito (S2/S3) e a ferramenta de carga e avaliação | `src/cmd/bakeoff`, `.workspace/bakeoff/` | banco de provas para medir antes e depois |
| Embeddings `nomic-embed-text` | servidor | (futuro) achar erros parecidos |

## 3. Decisões propostas (aprovar ou ajustar)

| # | Proposta | Motivo |
| --- | --- | --- |
| D12-1 | **ID do job visível:** o mcpollama lê o `X-Request-Id` e acrescenta `job=<id>` à linha `[uso]` de toda tool. No map-reduce (arquivo grande) vai a lista `jobs=<id1>,<id2>,...` | sem o ID não há como ligar o erro ao pedido gravado |
| D12-2 | **Tool nova `reportar_erro`** no mcpollama. Argumentos: `job_id` (um ou lista), `categoria` (lista fechada, D12-3), `descricao`, `resposta_correta`, `correcao_aplicada`, `severidade` (baixa/media/alta), `origem` (`claude`/`usuario`/`automatico`). Ela envia ao gateway e devolve o número do registro | é o Claude, que já confere o Qwen, quem melhor gera o histórico |
| D12-3 | **Categorias fixas:** `numeracao_errada`, `contagem_errada`, `calculo_errado`, `inventou` (alucinação), `omitiu`, `formato_invalido` (JSON/estrutura), `resumo_infiel`, `traducao_errada`, `idioma_errado`, `truncado`, `ignorou_instrucao`, `outro` | lista fechada permite medir por categoria e montar o treino por tipo de erro |
| D12-4 | **Base de treino permanente no gateway** (`treino.db`, 0600, sem expiração automática): ao receber um relato, o gateway **copia** do `content.db` o pedido e a resposta errada daquele job, antes que os 7 dias expirem, e grava junto a correção, a categoria, a estação e a origem. Relato de job já expirado é aceito, marcado `sem_pedido` | o `content.db` apaga em 7 dias; o treino precisa do pedido exato |
| D12-5 | **Endpoint `POST /api/feedback`** no gateway: Bearer via nginx, como o resto; corpo limitado (ex.: 256 KB); só aceita `job_id` que existe; mesma estação/IP do job **ou** admin; um relato por (job, origem) com atualização permitida | não deixar qualquer um poluir a base |
| D12-6 | **Monitor:** aba nova **"Erros (treino)"** com lista, filtros por categoria/tool/período e contagens. No **Detalhes** do job, botão **"Marcar erro"** (só admin) com o mesmo formulário da tool | o owner também corrige pelo painel |
| D12-7 | **Exportação para treino:** botão/rota (admin) que gera **JSONL no formato de chat** (`system` = instrução da tool, `user` = pedido exato, `assistant` = **resposta certa**), já separando ~15% para **teste** (fixo por hash do job, para não mudar entre exportações). Só entram relatos com `resposta_correta` preenchida | é o formato que as ferramentas de treino (Unsloth, LLaMA-Factory) leem |
| D12-8 | **Detecção automática de candidatos** no mcpollama: JSON inválido quando o formato pedido é JSON, resposta truncada (`done_reason=length`), resposta vazia e idioma diferente do pedido viram relato `origem=automatico`, **sem** resposta certa, só para revisão | erros mecânicos não dependem de alguém lembrar de reportar |
| D12-9 | ~~Registro provisório em arquivo local antes do código~~ — **descartado pelo owner (21:45): o registro começa só com a tool pronta.** A etapa 12.4 sai do plano | decisão do owner |
| D12-10 | **Privacidade:** a base de treino contém **texto de documentos de clientes**, de forma permanente. Exportar é só admin. Treinar fora da rede (nuvem) exige **anonimização** antes (§6). Backup segue a regra do `content.db` (excluir ou cifrar) | mesmos cuidados do log de jobs, agora sem prazo |
| D12-12 | **Owner (21:40): preferência pelo `mcpOllama` no `~\.claude\CLAUDE.md` global.** Se as tools `mcp__mcpOllama__*` existirem na sessão, elas são a **primeira escolha** para o trabalho do nível `local`, antes de ler o arquivo no contexto e antes de um subagente haiku. Texto proposto na §5.1, aplicado na 12.9 (**não agora**) | o MCP já é a "via principal" do nível local, mas a regra atual não obriga a olhar primeiro para ele |
| D12-11 | **Sem dependências de terceiros**, tudo em Go (regra do owner). A única exceção futura é o treino em si (§6), que roda em Python **fora** deste projeto | consistência com o resto do mcpollama e do gateway |

## 4. Etapas

| # | Etapa | Modelo | Estado |
| --- | --- | --- | --- |
| 12.0 | Aprovação deste plano e das D12-1…D12-11 | owner | pendente (owner pediu ajustes, 21:45) |
| 12.1 | mcpollama: `job=<id>` na linha `[uso]` (D12-1) + testes | sonnet | pendente |
| 12.2 | gateway: `treino.db` + `POST /api/feedback` + cópia do pedido e da resposta do `content.db` (D12-4, D12-5) + testes | sonnet | pendente |
| 12.3 | mcpollama: tool `reportar_erro` (D12-2, D12-3) + detecção automática (D12-8) + `TOOLS.md` regenerado | sonnet | pendente |
| 12.4 | ~~importar o arquivo provisório~~ — fora do plano (D12-9 descartada) | — | cancelado |
| 12.5 | monitor: aba "Erros (treino)", botão "Marcar erro", exportação JSONL treino/teste (D12-6, D12-7) | sonnet | pendente |
| 12.6 | Banco de provas: `src/cmd/bakeoff` ganha o modo `-suites erros`, que roda o modelo atual contra a parte de **teste** e mede o acerto por categoria (linha de base para o treino) | sonnet | pendente |
| 12.7 | Revisão adversarial: forjar relato de job de outro, flood, corpo gigante, injeção no JSONL exportado, vazamento de conteúdo na aba aberta + testes de ataque guardados | **opus** | pendente |
| 12.8 | Gate completo (build, vet, testes, `-race`, Linux, ataques 9.5/10.L/10.5/12.7) + implantação (gateway 0.13.0, mcpollama 0.10.0) + aceite: relatar um erro real pela tool e pelo monitor, ver na aba, exportar o JSONL e conferir o par pedido/resposta certa | coordenador + owner | pendente |
| 12.9 | Documentação: README, MANUAL, CLAUDE.md, CHANGELOG, CONEXAO-IA. **No `~\.claude\CLAUDE.md` global** (owner, 21:40–21:45), junto com a tool e não antes: (a) a seção de **preferência pelo `mcpOllama`** (D12-12, texto na §5.1), logo antes de "Nível `local` — como chamar"; (b) uma seção curta **"relatar erros"**, dentro da parte do nível `local`, apontando para `docs/RELATAR-ERROS-IA.md` | sonnet + coordenador | pendente |

## 5. Instruções para o Claude dos projetos

Arquivo: `docs/RELATAR-ERROS-IA.md`, a fonte única. **Onde o Claude lê (owner, 21:45): no `~\.claude\CLAUDE.md` global**, numa
seção curta dentro da parte do nível `local` (Qwen), que aponta para o documento. Assim vale em todo projeto que usa o `mcpOllama`,
sem copiar em cada um. Ele diz **quando** relatar (toda vez que o Claude conferir uma resposta do Qwen e achar erro), **o que**
registrar (categoria, o que estava errado, resposta certa, o que foi feito) e **como** (pela tool `reportar_erro`, quando ela existir;
**sem registro provisório**, por decisão do owner).

### 5.1 Texto proposto para o `~\.claude\CLAUDE.md` global (D12-12, aplicar na 12.9)

```markdown
### MCP `mcpOllama` ativo no projeto → PREFERÊNCIA (owner, 01/11/2026)

**Se as tools `mcp__mcpOllama__*` existirem na sessão, elas são a PRIMEIRA escolha** para o trabalho que cabe ao nível
`local`, antes de ler o arquivo no meu contexto e antes de delegar a um subagente haiku:
- **Ler, resumir ou extrair de arquivo** (sobretudo grande): `summarize_file` / `extract`, em vez de `Read` + resumir eu mesmo.
  O texto não entra no meu contexto, e é aí que está a economia.
- **Traduzir, converter formato, rascunhar, reescrever:** `translate` / `generate` / `chat`.
- **Dados sensíveis** (documentos de clientes, cofre): **sempre** pelo `mcpOllama`, porque não saem da rede local.
- **Busca semântica:** `embed`.
- **Subagentes:** ao delegar uma coleta ou leitura em volume, instruir o subagente a usar as tools `mcp__mcpOllama__*` quando
  estiverem disponíveis para ele.
- **Erros do modelo:** ao conferir um resultado e achar erro, relatar com `reportar_erro` (ver `E:\MCPOllam\docs\RELATAR-ERROS-IA.md`).

**Não usar o `mcpOllama`** (ir direto ao nível Anthropic adequado) quando a tarefa for decisão, código final, verificação crítica ou
revisão adversarial; quando o arquivo estiver fora do `--allow-root`; ou quando a tool falhar ou o servidor estiver fora (aí sobe para
haiku, ver "Regras de uso"). Ao usar, indicar "nível local (Qwen)" no texto.
```

## 6. Treino (congelado até haver dados e hardware)

Começa quando a base tiver volume (meta inicial: **≥ 300 relatos com resposta certa**, com pelo menos 50 por categoria a corrigir) **e**
houver onde treinar:
- **Hardware:** QLoRA do 9B pede uma GPU de 16–24 GB por algumas horas. As RTX 3070 de 8 GB e a fonte de 850 W não comportam
  treino longo a 100%. Opções: GPU nova, 2ª fonte + divisão entre placas, ou nuvem com **dados anonimizados**.
- **Ferramentas:** Unsloth ou LLaMA-Factory (Python), fora deste repositório. Antes, confirmar o suporte ao `qwen3.5` e à conversão
  do adaptador para GGUF.
- **Volta ao Ollama:** `FROM qwen3.5:9b` + `ADAPTER <lora.gguf>` → `qwen3-ia-v2` → banco de provas da 12.6 e S2/S3 da Fase 8 → só
  substitui se ganhar sem piorar o resto (A/B pelo gateway).

## 7. Riscos

1. Relatos ruins (correção errada) ensinam errado → a exportação só leva `resposta_correta` confirmada; a aba tem "descartar".
2. Volume baixo → a detecção automática (D12-8) e o registro provisório (D12-9) aceleram.
3. Conteúdo sensível permanente → D12-10.
4. O treino pode piorar o que o modelo já fazia bem → a parte de teste fica fixa e a troca só acontece com ganho medido.

## 8. Changelog do plano

- 2026-11-01 21:35: criado a pedido do owner. Decisões D12-1…D12-11 propostas, aguardando aprovação.
- 2026-11-01 21:45: decisões do owner:
  - instruções no `~\.claude\CLAUDE.md` global, entrando na 12.9 junto com a tool;
  - **sem registro provisório**: D12-9 e 12.4 saem.
  - O owner pediu **ajustes** antes de aprovar; aguardando quais.
- 2026-11-01 21:40: ajuste do owner: **preferência pelo `mcpOllama`** no `CLAUDE.md` global (D12-12, texto na §5.1), **só no
  plano**, aplicada na 12.9.
  - Erro de processo do coordenador: aplicou o texto no global antes da hora. O owner interrompeu, e o texto foi **revertido na hora**
    (o global voltou ao estado anterior, conferido).
