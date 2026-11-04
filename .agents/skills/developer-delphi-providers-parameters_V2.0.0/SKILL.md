---
name: developer-delphi-providers-parameters
description: Use when the user asks about the Parameters module of ProvidersORM 3.0.0 — configuration read/written through three sources (Database, IniFile, JsonObject); IParameters/IParametersSource/IParametersDatabase/IParametersIniFile/IParametersJsonObject API; TParameters.NewDatabase/NewIniFile/NewJsonObject; TProviders.NewParameters; TParameter; loader de configuração de conexão (IConnection.FromConfig); USE_PARAMETERS directive. Units Parameters.*.pas + facades Providers.Parameters.pas / Providers.pas.
model: haiku
thinking: extended
category: developer-delphi
license: MIT
copyright: "Copyright (c) 2026 CSL Tech Solutions"
company: "CSL Tech Solutions"
author: "Claiton de Souza Linhares"
---

# developer-delphi-providers-parameters
## Versão interna (ficheiro)

| Campo           | Valor |
| --------------- | ----- |
| **FileVersion** | 2.0.0 |
| **Política**    | `.cursor/VERSION.md` |

## Responsabilidade única

Esta skill documenta o módulo **Parameters** do ProvidersORM 3.0.0 — leitura e escrita de parâmetros de configuração (`TParameter`: nome, valor texto, tipo, título, contrato/produto) a partir de três fontes (Database, IniFile, JsonObject), agregadas por `IParameters`. Todos os nomes abaixo foram confirmados no código (`src/modules/Providers.v3/src/`, unit:linha); em caso de dúvida, o código vence a skill.

## When to use

- Ao ler/gravar parâmetros via `Select`/`Insert`/`Update`/`Save`/`Delete` em INI, JSON ou tabela de banco.
- Ao escolher a fonte (`TParameters.NewIniFile`, `NewJsonObject`, `NewDatabase`) ou o agregador (`TProviders.NewParameters`).
- Ao alimentar `IConnection.FromConfig` a partir de parâmetros (loader de conexão).
- Ao ativar `USE_PARAMETERS` em `ORM.Defines.inc`.

## When NOT to use

- Para configurar a conexão manualmente (Host, Port, etc.) → `developer-delphi-providers-orm-usage`.
- Para logging → `developer-delphi-providers-loggers`.
- Para diretivas `{$IF DEFINED(...)}` → `developer-delphi-programming-conditional-defines`.

## Dependências (skills prévias)

| Skill | Quando executar antes |
|-------|-----------------------|
| `developer-delphi-programming-conditional-defines` | Ao ativar `USE_PARAMETERS` em `ORM.Defines.inc` |
| `developer-delphi-providers-orm-usage` | Ao integrar Parameters com Connection ou Database |

---

## Ativação

Em `ORM.Defines.inc` (`ORM.Defines.inc:170`):

```pascal
{$DEFINE USE_PARAMETERS}
```

> **Nota histórica:** o alias com typo `USE_PARAMENTERS` foi **removido** na 1.2.0 do `ORM.Defines.inc` (05/07/2026; `ORM.Defines.inc:123-125`: "v3 code uses USE_PARAMETERS only"). Código ou skills antigos com `USE_PARAMENTERS` estão errados no v3 — usar sempre `USE_PARAMETERS`.

---

## Interface pública

O consumidor usa as **fachadas**; as units do módulo ficam na mesma pasta única de consumo do ProvidersORM v3 (a árvore por módulos `src/Modulos/` é apenas a estrutura interna da biblioteca — nunca é caminho a configurar).

| Unit | Papel |
|------|-------|
| `Providers.pas` | `TProviders.NewParameters: IParameters` (sem argumentos; `Providers.pas:160`, impl. `:235-239`) |
| `Providers.Parameters.pas` | slice da fachada: aliases `TParameter, TParameterList, TParameterSource, TParameterConfig, TParameterSourceArray` e `IParameters*` (`:60-64`); `TProvidersParameters` (`:70`) |
| `Parameters.Interfaces.pas` | `IParametersSource` (`:88`), `IParametersDatabase` (`:152`), `IParametersIniFile` (`:221`), `IParametersJsonObject` (`:239`), `IParameters` (`:267`) |
| `Parameters.pas` | `TParameters` — fábricas do módulo (`:59-63`) |
| `Parameters.Connections.pas` | `TParametersConnectionLoader` (`:94`) — liga Parameters a `IConnection.FromConfig` |
| `Parameters.{Database,IniFile,JsonObject,SourceBase,Version}.pas` | implementações por fonte (internas) |
| `Commons.Types.pas` | `TParameter` (`:280`), `TParameterSource` (`:269`), `TParameterConfig` (`:277`) |

Units do módulo: `Parameters.pas` + 7 `Parameters.*.pas` (inclui `Parameters.Connections.pas`) + `Attributers.Parameters*.pas`, `Commons.Parameters.Consts.pas`, `Exceptions.Parameters.pas`.

---

## Fábricas (assinaturas reais)

| Fábrica | Onde |
|---------|------|
| `TProviders.NewParameters: IParameters` | `Providers.pas:160` — **sem argumentos** |
| `TParameters.New: IParameters` / `TParameters.New(const AConfig: TParameterConfig): IParameters` | `Parameters.pas:59-60` |
| `TParameters.NewDatabase: IParametersDatabase` | `Parameters.pas:61` |
| `TParameters.NewIniFile: IParametersIniFile` | `Parameters.pas:62` — **sem argumento** (caminho via `.FilePath(...)`) |
| `TParameters.NewJsonObject: IParametersJsonObject` | `Parameters.pas:63` |
| `TProvidersParameters.New / New(AConfig) / NewDatabase / NewIniFile / NewJsonObject` | `Providers.Parameters.pas:72-76` |
| `TProvidersParameters.RegisterConnectionLoader(const AParameters: IParameters): IConnectionConfigureLoader` | `Providers.Parameters.pas:80` (`UnregisterConnectionLoader` em `:81`) |

Padrões: `DEFAULT_PARAMETER_SOURCE = psDatabase` (`Commons.Consts.pas:424`); `DEFAULT_PARAMETER_CONFIG = [psDatabase, psInifiles]` (`:426`). `TParameterSource = (psNone, psDatabase, psInifiles, psJsonObject)`.

---

## Métodos por interface

**`IParametersSource` (base comum, `Parameters.Interfaces.pas:88`)** — `Source`, `ContratoID`, `ProdutoID`, `Title` (get/set, `:92-100`); `Select(AName): TParameter` / `Select: TParameterList` (+ overloads `out`, `:103-106`); `Insert`/`Update`/`Save(const AParameter: TParameter)` (+ `out ASuccess`, `:109-114`; `Save` = upsert); `Delete(AName)` (`:115-116`); `Exists(AName)` (`:119`); `Count` (`:121`); `Refresh` (`:123`); `ToJSON: TJSONObject` (`:124`); `ImportFrom`/`ExportTo(ASource)` (`:127-130`).

**`IParametersDatabase` (`:152`)** — `TableName`, `Schema`, `AutoCreateTable`, `Connection(const AConnection: IConnection)`, `Engine/DatabaseType/Host/Port/Username/Password/Database/DllBasePath/FirebirdVersion` (`:156-185`); `FromConfig: IParametersDatabase` e `FromConfig(const APath: string)` (`:188-189`); `Connect/Disconnect/IsConnected` (`:192-194`); `EnsureTable` (idempotente, cria + migra, `:197`); `TableExists` (`:198`).

**`IParametersIniFile` (`:221`)** — `FilePath` (padrão `<EXE>\data\config.ini`, `:223-224`), `Section` (get/set, `:225-226`), `AutoCreateFile`, `FileExists`.

**`IParametersJsonObject` (`:239`)** — `FilePath` (padrão `<EXE>\data\config.json`), `ObjectName`, `JsonObject(const AValue: TJSONObject)`, `AutoCreateFile`, `FileExists`, `ToJSONString`, `SaveToFile`, `LoadFromFile`, `LoadFromString` (`:241-253`).

**`IParameters` (agregador, `:267`)** — `SetLogger`, `Source/AddSource/RemoveSource/HasSource`, `Priority(const ASources: TParameterSourceArray)` (ordem de leitura), `ContratoID/ProdutoID`, `Select/Insert/Update/Save/Delete/Exists/Count/Refresh/ToJSON` (`:290-304`) e as sub-fontes **`Database`**, **`IniFile`**, **`JsonObject`** (`:307-309`).

### O que NÃO existe (erros comuns de skills antigas)

- `IParameters` **não tem** `Load`, `Section`, `FromConfig`, `FromIniFile`, `FromJSON`, `FromJSONObject`, `FromDatabase`, nem getters tipados (`GetString`, `GetInteger`...). O valor vem de `TParameter.Value` (**string**) — converter no chamador.
- `Section` existe **só** em `IParametersIniFile`; `FromConfig` **só** em `IParametersDatabase`.
- `Getter`, `List`, `Setter` estão marcados `deprecated` (`:134-139`, `:313-315`) — não usar em código novo.
- `Select(AName)` devolve `TParameter` **de propriedade do chamador** (liberar com `Free`; `nil` se não existir).
- `IConnection.FromParameters` **não existe** (ver abaixo).

---

## Integração com IConnection (loader — substitui o antigo `FromParameters`)

`IConnection.FromConfig` (`Connections.Interfaces.pas:116`, impl. `Connections.pas:1842`) não lê nada sozinho: consulta o loader registado. O loader que lê `IParameters` é `TParametersConnectionLoader` (`Parameters.Connections.pas:94`), registado **uma vez** na inicialização:

```pascal
TProvidersParameters.RegisterConnectionLoader(LParams);   // Providers.Parameters.pas:80
LConn := TProviders.NewConnection
  .ConfigFilePath('perfil')   // IConnection.ConfigFilePath — Connections.Interfaces.pas:87
  .FromConfig                 // :116
  .Connect;                   // :123
```

Sem loader registado, `FromConfig` lança `EConnectionConfigurationException` (`Connections.pas:1846-1851`). O "perfil" lido é o `ConfigFilePath` (vazio = seção padrão; caminho físico = nome-base sem `.ini/.json/.db`, `Parameters.Connections.pas:141-155`). Alternativas sem Parameters: `FromIniFile(AFilePath, ASection)` (`:115`), `FromJSON(AJSON)` (`:117`), setters fluentes.

---

## Padrão de fallback (leitura em cascata)

Preferir a prioridade declarativa do agregador a `try/except` aninhado:

```pascal
LParams := TProviders.NewParameters;            // [psDatabase, psInifiles] por padrão
LParams.Priority([psDatabase, psInifiles, psJsonObject]);
LVal := LParams.Select('host');                 // lê na ordem de Priority; liberar LVal
```

---

## Anti-padrões

| Anti-padrão | Por que é errado | Como corrigir |
|-------------|-----------------|---------------|
| `{$DEFINE USE_PARAMENTERS}` | Alias removido no v3; a define nunca ativa o módulo | Usar `USE_PARAMETERS` |
| `TParameters.New.FromIniFile(...)` / `.FromJSON(...)` / `.FromDatabase(...)` | Não existem no v3 | `TParameters.NewIniFile.FilePath(...)`, `NewJsonObject.LoadFromString(...)`, `NewDatabase.FromConfig(...)` |
| `TConnection.New.FromParameters(...)` | Não existe | `RegisterConnectionLoader` + `FromConfig` |
| Esquecer o `Free` do `TParameter` devolvido por `Select` | Vazamento de memória | `try ... finally LVal.Free end` |
| Ler valores esperando tipo (Integer/Boolean) | `Value` é string | `StrToIntDef(LVal.Value, padrão)` |
| Tratar a árvore interna por módulos como caminho de projeto | É apenas estrutura interna da biblioteca | Usar as fachadas `Providers*.pas` |

---

## Métricas de sucesso

- Configuração lida/gravada via `TParameter` sem duplicar parse de INI/JSON.
- `FromConfig` funciona com o loader registado e falha com mensagem clara sem ele.
- Nenhuma ocorrência de `USE_PARAMENTERS`, `FromParameters` ou getters inexistentes no código do projeto.

---

## Responsável principal

| Papel | Quem |
|-------|------|
| Executor | `developer-delphi-agent-parameters-expert` |
| Revisor | `documentation-project-expert` |

---

## Changelog (este arquivo)

- 2.0.0 (21/10/2026): MAJOR — reescrita a partir do código real do ProvidersORM 3.0.0 (Onda 3 do plano `GestorERP2026-10-20-152500-artefatos-providersorm-v3`, §7.6; fatos no report da Onda 1). (1) `USE_PARAMENTERS` → `USE_PARAMETERS` (alias removido em `ORM.Defines.inc:123-125`; define real `:170`), com nota histórica. (2) API de fábrica trocada: removidos `TParameters.New.FromIniFile/FromJSON/FromJSONObject/FromConfig/FromDatabase` (inexistentes); documentados `TParameters.New/NewDatabase/NewIniFile/NewJsonObject` (`Parameters.pas:59-63`), `TProviders.NewParameters` sem argumentos (`Providers.pas:160`), `IParametersDatabase.FromConfig` (`Parameters.Interfaces.pas:188-189`), `IParametersIniFile.Section` (`:225`), `IParametersJsonObject.LoadFromFile/LoadFromString` (`:252-253`). (3) Documentado o contrato real de `IParameters`/`IParametersSource` (Select/Insert/Update/Save/Delete/Exists/Count/Refresh/ToJSON, sub-fontes, `Priority`) e a lista do que não existe (Load, getters tipados; `Getter/List/Setter` deprecated). (4) `IConnection.FromParameters` (inexistente) substituído pelo loader `TProvidersParameters.RegisterConnectionLoader` (`Providers.Parameters.pas:80`) + `IConnection.ConfigFilePath(..).FromConfig`. (5) Caminhos: removidos os caminhos `Main` e `Modulos/Parameters` do v2; units citadas pelo nome do ficheiro + fachadas `Providers.Parameters.pas`/`Providers.pas`, pasta única de consumo (a árvore por módulos mencionada apenas como estrutura interna). (6) "Providers ORM v2" → "ProvidersORM 3.0.0". (7) FileVersion da tabela = frontmatter = sufixo da pasta = 2.0.0. Referências quebradas removidas (regra de caminhos local inexistente).
- 1.0.0 (17/04/2026): Onda 3 do refactor — skill renomeada de `project-parameters_V*` para `developer-delphi-providers-parameters_V1.0.0`. Conteúdo generificado (remoção de referências literais a 'Projeto v2.0 deste clone', paths absolutos, MXX concreto). Versão anterior arquivada em `.cursor/Backup/renamed-skills-20260417/skills/`.
- 1.0.0 (11/04/2026): Criação — skill do módulo Parameters (INI, JSON, Database; IParameters; fallback cascade; ativação com a define de nome com typo, removida no v3).
