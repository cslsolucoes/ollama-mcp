---
name: developer-delphi-providers-loggers
description: Use when the user asks about the Loggers module of ProvidersORM 3.0.0 — multi-channel logging (TextFile, Json, CSV, Console, Memory, EventLog, Database, Http, Email, WebSocket, SysLog, Redis, ElasticSearch); ILogger/ILoggerChannel API; TLoggers.New / TProviders.NewLogger; log levels (llInfo..llDone); USE_LOGGERS and USE_LOGGERS_* directives. Units Loggers.*.pas + facades Providers.Loggers.pas / Providers.pas.
model: haiku
thinking: extended
category: developer-delphi
license: MIT
copyright: "Copyright (c) 2026 CSL Tech Solutions"
company: "CSL Tech Solutions"
author: "Claiton de Souza Linhares"
---

# developer-delphi-providers-loggers
## Versão interna (ficheiro)

| Campo           | Valor |
| --------------- | ----- |
| **FileVersion** | 2.0.0 |
| **Política**    | `.cursor/VERSION.md` |

## Responsabilidade única

Esta skill documenta o módulo **Loggers** do ProvidersORM 3.0.0 — logging multi-canal com nível mínimo, fila assíncrona e fail-over. Define a interface pública `ILogger`/`ILoggerChannel`, as fábricas `TLoggers.New` / `TProviders.NewLogger`, os 13 canais reais e as defines de ativação. Todos os nomes abaixo foram confirmados no código (`src/modules/Providers.v3/src/`, unit:linha); em caso de dúvida, o código vence a skill.

## When to use

- Ao implementar logging em qualquer módulo do projeto.
- Ao escolher/ativar canais (TextFile, Json, CSV, Console, Memory, EventLog, Database, Http, Email, WebSocket, SysLog, Redis, ElasticSearch).
- Ao definir nível mínimo (`MinLevel`) ou hooks (`OnBeforeWrite`, `OnAfterWrite`, `OnLevelCheck`).
- Ao ativar `USE_LOGGERS` / `USE_LOGGERS_*` em `ORM.Defines.inc`.

## When NOT to use

- Para carregamento de configuração (INI/JSON/Database) → `developer-delphi-providers-parameters`.
- Para exceções centralizadas → `documentation-project-expert` / seção Exceptions.
- Para diretivas `{$IF DEFINED(...)}` → `developer-delphi-programming-conditional-defines`.

## Dependências (skills prévias)

| Skill | Quando executar antes |
|-------|-----------------------|
| `developer-delphi-programming-conditional-defines` | Ao ativar `USE_LOGGERS*` em `ORM.Defines.inc` |
| `developer-delphi-providers-parameters` | Os canais leem a própria configuração via Parameters (`USE_PARAMETERS`) |

---

## Ativação

Em `ORM.Defines.inc` (`ORM.Defines.inc:180`):

```pascal
{$DEFINE USE_LOGGERS}
```

Cada canal tem a sua define (tabela abaixo); os canais de rede dependem também de `USE_LOGGERS_NET` (`ORM.Defines.inc:204`), que fica **desligado sob FPC** (`:425`, canais Delphi-only). Quase todos os canais precisam de `USE_PARAMETERS` para ler a configuração (sem ele, usam os padrões de `Commons.Loggers.Consts.pas`).

---

## Interface pública

O consumidor usa as **fachadas**; as units do módulo ficam na pasta única de consumo do ProvidersORM v3 (a árvore por módulos `src/Modulos/` é apenas a estrutura interna da biblioteca — nunca é caminho a configurar).

| Unit | Papel |
|------|-------|
| `Providers.pas` | `TProviders.NewLogger: ILogger` (`Providers.pas:170`, impl. `:249-253`, gate `USE_LOGGERS`) |
| `Providers.Loggers.pas` | slice da fachada: aliases `ILogger, ILoggerChannel, TLogLevel, TLoggerEntry, TLoggerBeforeWriteEvent/AfterWrite/LevelCheck` (`:68-78`); `TProvidersLoggers.New` (`:87`) |
| `Loggers.pas` | `TLoggers = class` com `class function New: ILogger` (`:129-131`, impl. `:809`) |
| `Loggers.Interfaces.pas` | `ILogger` (`:91`), `ILoggerChannel` (`:71`) e assinaturas dos hooks |
| `Loggers.Channel.*.pas` | 14 ficheiros: 13 canais + `FileBase` (base dos canais de arquivo, não é canal) |
| `Loggers.{Connections,NullLogger,Queue,Version}.pas` | suporte (fila assíncrona = `TLoggerQueue`, interna) |
| `Commons.Loggers.Types.pas` / `Commons.Loggers.Consts.pas` | `TLogLevel`, `TLoggerEntry`; padrões de configuração |

Total: 20 `Loggers*.pas`.

---

## Fábricas

- `TLoggers.New: ILogger` — `Loggers.pas:131`; `TProvidersLoggers.New: ILogger` — `Providers.Loggers.pas:87`; `TProviders.NewLogger` — `Providers.pas:170`. Todas equivalentes (delegam em `TLoggers.New`).
- **A fábrica do v2 (classe com sufixo `Factory`) não existe no v3** (nem `ILoggers`, `NewMultiple`, `.Destination(...)`, `.FilePath(...)`, `.Field(...)`, `Fatal`, `Warn`).
- `TLoggers.New` devolve o logger **com os 13 canais de baseline já registados** (`Loggers.pas:326-340`), cada um em `try/except` (um canal indisponível fica ausente, não derruba o logger) e cada um configurado/ativado pela **configuração via Parameters** (parâmetro `Enabled` por canal). Padrões do núcleo: assíncrono `True`, fila 10000, flush 500 ms, fail-over `True` (`Commons.Loggers.Consts.pas:366-373`).

---

## `ILogger` (`Loggers.Interfaces.pas:91`)

| Membro | Linha |
|--------|-------|
| `RegisterChannel(const AChannel: ILoggerChannel): ILogger` | `:95` |
| `MinLevel: TLogLevel` / `MinLevel(const AValue: TLogLevel): ILogger` | `:98-99` |
| `Info`, `Success`, `Warning`, `Error`, `Critical`, `Debug`, `Trace`, `Done` — cada um com 3 overloads: `(AMessage)`, `(AMessage; const AData: array of const)`, `(AMessage; AException: TObject)` | `:102-135` |
| `Exception(AMessage; E: TObject)` / `Exception(AMessage; AData; E)` | `:122-123` |
| `Log(ALevel; AMessage)` e overloads com `AData` / `AException` | `:138-140` |
| `Flush` (descarrega a fila assíncrona) | `:143` |
| `OnBeforeWrite` (cancelável), `OnAfterWrite`, `OnLevelCheck` (get/set) | `:150-159` |

`ILoggerChannel` (`:71`): `Name`, `Write(const AEntry: TLoggerEntry): Boolean`, `Enabled` (get/set), `IsHealthy`.

**Não há** `AddChannel`, `Level(...)` nem método público de fila em `ILogger`: canais extra entram por `RegisterChannel`; a fila é interna (`TLoggerQueue`, `Loggers.Queue.pas:64-75`).

## Níveis (`TLogLevel`, `Commons.Loggers.Types.pas:82-92`) — 9 valores

`llInfo, llSuccess, llWarning, llError, llCritical, llException, llDebug, llTrace, llDone`. O filtro de `MinLevel` compara por `LOG_LEVEL_SEVERITY` (não pela ordem do enum) — `Loggers.pas:566`. Padrão do núcleo: `Info` (`FMinLevel := llInfo`, `Loggers.pas:304`).

---

## 13 canais reais — defines de cada um (`ORM.Defines.inc`)

| Canal (unit `Loggers.Channel.*`) | Define / gate |
|---------------------------------|---------------|
| `TextFile` (`TLoggerChannelTextFile`) | só `USE_LOGGERS` |
| `Json` | só `USE_LOGGERS` |
| `CSV` | `USE_LOGGERS_CSV` (`:186`) |
| `Console` | `USE_LOGGERS_CONSOLE` (`:187`) + `USE_PARAMETERS` |
| `Memory` (buffer circular) | `USE_LOGGERS_MEMORY` (`:188`) + `USE_PARAMETERS` |
| `EventLog` (Windows) | `USE_LOGGERS_EVENTLOG` (`:185`) + `USE_PARAMETERS` |
| `Database` | **sem define própria**: `USE_LOGGERS` + `USE_PARAMETERS` + `USE_DATABASE` (`Loggers.Channel.Database.pas:130-135`); canal inerte se faltar perfil de conexão |
| `Http` | `USE_LOGGERS_HTTP` (`:205`) sob `USE_LOGGERS_NET` + `USE_PARAMETERS` (Delphi/Windows) |
| `Email` | `USE_LOGGERS_EMAIL` (`:206`) sob `USE_LOGGERS_NET` + `USE_PARAMETERS` |
| `WebSocket` | `USE_LOGGERS_WEBSOCKET` (`:207`) sob `USE_LOGGERS_NET` + `USE_PARAMETERS` |
| `SysLog` | `USE_LOGGERS_SYSLOG` (`:211`) sob `USE_LOGGERS_NET` + `USE_PARAMETERS` |
| `Redis` | `USE_LOGGERS_REDIS` (`:212`) sob `USE_LOGGERS_NET` + `USE_PARAMETERS` |
| `ElasticSearch` | `USE_LOGGERS_ELASTICSEARCH` (`:213`) sob `USE_LOGGERS_NET` + `USE_PARAMETERS` |

**Não existem** canais XML nem Custom. Para um destino próprio, implementar `ILoggerChannel` e chamar `RegisterChannel`. Defines `USE_LOGGERS_INFLUXDB/GRAYLOG/LOGSTASH/SENTRY/SLACK/TELEGRAM/TWILIO` (`:214-220`) estão comentadas e sem canal correspondente.

Validação em ambiente de teste (referência, sem credenciais aqui): os canais Redis (Memurai), ElasticSearch (9.4.4, HTTPS) e Email (Zimbra, SMTPS 465) foram validados contra os perfis de teste catalogados no cofre `.workspace/DADOS_CONEXAO_TESTE.md` (seções `redis`, `elasticsearch`, `email`) — consultar o cofre; nunca copiar credenciais para código ou skill.

---

## Estrutura (units, pasta única)

```
Providers.pas · Providers.Loggers.pas        ← fachadas (consumo)
Loggers.pas · Loggers.Interfaces.pas         ← fábrica TLoggers e contratos
Loggers.Channel.{TextFile,Json,CSV,Console,Memory,EventLog,Database,
                 Http,Email,WebSocket,SysLog,Redis,ElasticSearch}.pas
Loggers.Channel.FileBase.pas                 ← base dos canais de arquivo
Loggers.{Connections,NullLogger,Queue,Version}.pas
```

A árvore por módulos existe apenas como estrutura interna da biblioteca canônica.

---

## Anti-padrões

| Anti-padrão | Por que é errado | Como corrigir |
|-------------|-----------------|---------------|
| Fábrica `...Factory.New` do v2 / `.Destination(ldXxx)` | Não existe no v3 | `TLoggers.New` / `TProviders.NewLogger` + configuração via Parameters; extras por `RegisterChannel` |
| `LLog.Warn`, `LLog.Fatal`, `llWarn`, `llFatal` | Nomes inexistentes | `Warning`/`Critical`; `llWarning`/`llCritical` |
| Esperar canal XML ou Custom | Não existem | Implementar `ILoggerChannel` próprio |
| Criar logger novo a cada chamada | Cria 13 canais + fila/thread a cada vez | Reutilizar a instância `ILogger` por módulo/serviço |
| Esquecer `Flush` antes de encerrar | Entradas pendentes na fila assíncrona perdem-se | `LLog.Flush` no shutdown |
| Log de dados sensíveis (senhas, tokens) | Risco de segurança | Mascarar antes de logar (ou `OnBeforeWrite` com `ACancel`) |
| Habilitar canais de rede sob FPC | `USE_LOGGERS_NET` fica desligado (Delphi-only) | Usar canais locais no FPC |

---

## Métricas de sucesso

- Logs gravados nos canais corretos sem overhead perceptível (fila assíncrona).
- Nível mínimo configurado — sem `llDebug`/`llTrace` em produção.
- Nenhuma ocorrência da fábrica `...Factory` do v2, `Destination(`, `Fatal`/`Warn` ou canais XML/Custom no código do projeto.

---

## Responsável principal

| Papel | Quem |
|-------|------|
| Executor | `developer-delphi-agent-loggers-expert` |
| Revisor | `documentation-project-expert` |

---

## Changelog (este arquivo)

- 2.0.0 (21/10/2026): MAJOR — reescrita a partir do código real do ProvidersORM 3.0.0 (Onda 3 do plano `GestorERP2026-10-20-152500-artefatos-providersorm-v3`, §7.7; fatos no report da Onda 1). (1) Fábrica `...Factory` do v2 (inexistente) → `TLoggers.New: ILogger` (`Loggers.pas:129-131`) e `TProviders.NewLogger` (`Providers.pas:170`, gate `USE_LOGGERS`); removidos `ILoggers`, `NewMultiple`, `.Destination/.FilePath/.Field`, `Warn`, `Fatal`. (2) Lista de destinos corrigida: de "10 destinos" (com XML e Custom, inexistentes) para os 13 canais reais (TextFile, Json, CSV, Console, Memory, EventLog, Database, Http, Email, WebSocket, SysLog, Redis, ElasticSearch) + `FileBase` (base) = 14 ficheiros `Loggers.Channel.*`, com a define de cada um; canal Database sem define própria (`USE_LOGGERS` + `USE_PARAMETERS` + `USE_DATABASE`); canais de rede sob `USE_LOGGERS_NET` (desligado em FPC). (3) `ILogger` real documentado: `RegisterChannel`, `MinLevel`, 8 níveis com 3 overloads, `Exception`, `Log`, `Flush`, hooks `OnBeforeWrite/OnAfterWrite/OnLevelCheck`; sem `AddChannel`/fila pública; `ILoggerChannel` documentado. (4) Níveis: 5 (DEBUG/INFO/WARN/ERROR/FATAL) → 9 reais (`llInfo..llDone`). (5) Caminhos: removidos os caminhos `Main` e `Modulos/Loggers` do v2; units pelo nome do ficheiro + fachadas `Providers.Loggers.pas`/`Providers.pas`, pasta única de consumo (árvore por módulos citada só como estrutura interna). (6) Perfis de teste (Memurai, Elasticsearch, Zimbra) citados apenas como referência de validação, apontando para o cofre, sem credenciais. (7) "Providers ORM v2" → "ProvidersORM 3.0.0". (8) FileVersion da tabela = frontmatter = sufixo da pasta = 2.0.0.
- 1.0.0 (17/04/2026): Onda 3 do refactor — skill renomeada de `project-loggers_V*` para `developer-delphi-providers-loggers_V1.0.0`. Conteúdo generificado (remoção de referências literais a 'Projeto v2.0 deste clone', paths absolutos, MXX concreto). Versão anterior arquivada em `.cursor/Backup/renamed-skills-20260417/skills/`.
- 1.0.0 (11/04/2026): Criação — skill do módulo Loggers (10 destinos, 5 níveis, ILogger, ativação USE_LOGGERS, tabela de decisão por destino).
