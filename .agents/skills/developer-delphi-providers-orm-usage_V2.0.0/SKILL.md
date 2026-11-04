---
name: developer-delphi-providers-orm-usage
description: Use when the user asks how to use the ProvidersORM v3.0.0 framework — Connection (IConnection), ITable/IFields (DML e DDL), IQueryBuilder, IEntityManager, ISynchronize, mapeamento por atributos ([Table]/[Field]/[PrimaryKey] → ITable), IdentityMap e UnitOfWork. A biblioteca é consumida como pasta única (Deploy) via as fachadas Providers.pas / Providers.Interfaces.pas / Providers.<Modulo>.pas. Toda assinatura citada foi confirmada no código v3 (unit:linha).
model: sonnet
thinking: extended
category: developer-delphi
license: MIT
copyright: "Copyright (c) 2026 CSL Tech Solutions"
company: "CSL Tech Solutions"
author: "Claiton de Souza Linhares"
---

# developer-delphi-providers-orm-usage

## Versão interna (ficheiro)

| Campo           | Valor |
| --------------- | ----- |
| **FileVersion** | 2.0.0 |
| **Política**    | `.cursor/VERSION.md` |

## Responsabilidade única

Esta skill é a referência de uso prático do framework **ProvidersORM v3.0.0** (`ORM.Version.inc`): ligar a uma base (`IConnection`), manipular dados e estrutura (`ITable`), consultar (`IQueryBuilder`), persistir (`IEntityManager`, `IUnitOfWork`, `IIdentityMap`), sincronizar esquema (`ISynchronize`) e mapear classes por atributos. Todo nome de método citado aqui foi confirmado por `grep -n` em `src/modules/Providers.v3/src/` (unit:linha entre parênteses); nada é citado de memória.

A skill orienta o desenvolvedor a consultar a documentação do próprio projecto (tipicamente `Documentation/` e `.workspace/context.json`) e o código da biblioteca antes de responder sobre units, fluxos ou trechos de código, garantindo que nenhuma API seja inventada.

Não cobre estrutura de pastas do projecto, compilação, padrões de nomenclatura ou documentação de produto — essas responsabilidades pertencem a skills dedicadas.

## Como a biblioteca é consumida (pasta única)

O ProvidersORM é distribuído como **pasta única** (Deploy; neste repositório `src/modules/Providers.v3/src/`). O consumidor referencia as **unidades pelo nome do ficheiro** (o path de compilação aponta para essa pasta) e usa **só as fachadas**:

| Fachada | Papel |
| ------- | ----- |
| `Providers.pas` | `TProviders`: fábricas estáticas — `NewConnection` (l.136), `NewPoolConnections` (l.143), `NewSynchronize(AConnection)` (l.148), `NewQueryBuilder` (l.150), `NewEntityManager` (l.154), `NewActiveDirectory` (l.165) |
| `Providers.Interfaces.pas` | re-export unificado das interfaces públicas (`IConnection` l.79, `IQueryBuilder` l.92, `IEntityManager` l.95) |
| `Providers.<Modulo>.pas` | fachada por módulo: `Providers.Connections`, `Providers.Database`, `Providers.PoolConnections`, `Providers.Attributers`, `Providers.Parameters`, `Providers.Loggers`, `Providers.ActiveDirectory`, `Providers.GraphQL`, `Providers.Printers`, `Providers.ZipFile` |

As restantes units (`Connections.pas`, `Database.*.pas`, `Databases.Interfaces.pas`, ...) são detalhe interno: a biblioteca organiza-as por módulo (estrutura interna da biblioteca), o que não é caminho a configurar no consumidor.

## When to use

- Usuário pergunta **como usar** o ProvidersORM no dia a dia (conexão, queries, transações, tabelas).
- Usuário pergunta sobre **mapeamento por atributos**: `[Table]` / `[Field]` / `[PrimaryKey]`, `IAttributeMapper`, `IAttributeParser`, `IAttributeRegistry`.
- Usuário pergunta sobre `IEntityManager`, `IIdentityMap`, `IUnitOfWork`, `IQueryBuilder`, `ISynchronize`.
- Usuário pede **exemplos de código** para conectar, criar tabela, inserir/atualizar/deletar, consultar.
- Usuário quer escolher quais flags activar (`USE_ATTRIBUTES` + `USE_ENTITY_MANAGER` + `USE_QUERY_BUILDER`).

**Ação obrigatória:** antes de responder com units, fluxos ou trechos de código, confirmar a assinatura na unit real (ver tabelas abaixo) ou na documentação do projecto.

## When NOT to use

- Localizar pastas, units ou paths do repositório → usar `documentation-project-structure`.
- Compilação e diretivas `USE_*` → usar `developer-delphi-programming-conditional-defines` + `developer-delphi-build-toolchain`.
- Arquitectura de domínio, padrões `T*` / `I*`, factory e fluent → usar `documentation-project-expert`.
- Parâmetros (INI/JSON/DB) → `developer-delphi-providers-parameters`; logging → `developer-delphi-providers-loggers`.
- Geração/atualização de documentação de produto → usar família `documentation-*`.

## Dependências (skills prévias)

| Skill                                                 | Quando executar antes                                     |
| ----------------------------------------------------- | --------------------------------------------------------- |
| `documentation-project-structure`                     | Para confirmar localização de units antes de citar paths  |
| `documentation-project-expert`                        | Para padrões de nomenclatura e interfaces ao implementar  |
| `developer-delphi-programming-conditional-defines`    | Para confirmar flags `USE_*` activas no projecto actual   |

## Flags e tipos de banco

Flags em `ORM.Defines.inc` (`src/modules/Providers.v3/src/`): `USE_DATABASE`, `USE_QUERY_BUILDER`, `USE_ENTITY_MANAGER` (l.166-169), `USE_ATTRIBUTES` (l.227), `USE_POOLCONNECTIONS`, `USE_PARAMETERS`, `USE_LOGGERS`, `USE_ACTIVEDIRECTORY`. Dependência: `USE_ENTITY_MANAGER` requer `USE_ATTRIBUTES` (comentário em l.169); com compilador abaixo do mínimo, `USE_ATTRIBUTES`/`USE_ENTITY_MANAGER` degradam automaticamente (l.327-330).

`TDatabaseTypes` (`Commons.Types.pas:116-127`) tem **10 valores**: `dtNone, dtFireBird, dtMySQL, dtPostgreSQL, dtSQLite, dtSQLServer, dtAccess, dtSQLAnywhere, dtODBC, dtLDAP`. `TDatabaseEngine` (`Commons.Types.pas:107-113`): `teNone, teUnidac, teFireDAC, teZeos, teSQLdb`.

## Conexão (`IConnection`, `Connections.Interfaces.pas`)

Fábrica: `TProviders.NewConnection` (`Providers.pas:136`) → `TConnection.New` (`Connections.pas:504`). Configuração fluente (`Host/Port/Username/Password/Database/Schema/...`) ou por `FromIniFile(APath, ASection)` (l.115), `FromConfig` (l.116), `FromJSON(AJSON)` (l.117), `FromClass(AClass)` (l.119, só com `USE_ATTRIBUTES`). **Não existe** `FromParameters`.

| Operação | Assinatura real |
| -------- | --------------- |
| Conectar | `Connect: IConnection` (l.123); `Disconnect`, `IsConnected`, `Ping` (l.124-126) |
| Query | `ExecuteQuery(ASQL): TDataSet` (l.129); com params `ExecuteQuery(ASQL, AParams: array of Variant)` (l.134) |
| Comando | `ExecuteCommand(ASQL): Integer` (l.130); com params (l.136) |
| Escalar | `ExecuteScalar(ASQL): Variant` (l.131); com params (l.138) |
| Transação | `BeginTransaction` (l.142), `Commit` (l.143), `Rollback` (l.144), `InTransaction` (l.145) |

Placeholders do overload com params: **`:param0`, `:param1`, ...** (`Connections.pas:3706`) — não `:p0`. Os eventos `OnBeforeConnect/OnAfterConnect/...` existem **só na classe `TConnection`** (`Connections.pas:490-494`), não em `IConnection`; **não existem** `OnBeforeExecute`/`OnAfterExecute`.

Pool: `TProviders.NewPoolConnections` (`Providers.pas:143`) → `TPoolConnections.New` (`PoolConnections.pas:261`); `AddDatabase(AName, AConfig: TConnectionData)` (`PoolConnections.Interfaces.pas:81`), `GetFromPool(ADatabaseName)` (l.87), `ReturnToPool(AConnection)` (l.70).

## Mapeamento por atributos

Atributos de mapeamento — **exactamente 3** (`Attributers.Database.Interfaces.pas`): `TableAttribute` (l.56, `[Table('nome', 'schema')]`), `FieldAttribute` (l.72, `[Field('coluna')]`), `PrimaryKeyAttribute` (l.81, `[PrimaryKey]`). **Não existem** `[Nullable]` nem `[AutoIncrement]`. Os atributos não são re-exportados pelas fachadas `Providers.*`: usar `Attributers.Database.Interfaces` no `uses`.

| Interface | Métodos reais |
| --------- | ------------- |
| `IAttributeParser` (l.87) | `HasTableAttribute(AClass)`, `GetTableName(AClass)`, `GetSchema(AClass)`, `GetFieldMappings(AClass)` (l.89-92) |
| `IAttributeMapper` (l.96) | `FromClass(AClass: TClass; ADatabaseType: TDatabaseTypes): ITable` (l.98) |
| `IAttributeRegistry` (l.102) | `GetTable(AClass, ADatabaseType): ITable` (l.104), `Unregister(AClass)` (l.105), `Clear` (l.106) — **sem `Register`**; instância global via `GetAttributeRegistry` (`Attributers.Database.Registry.pas:42`) |

Fábricas: `TProvidersAttributers.NewDatabaseParser` / `NewDatabaseMapper` (`Providers.Attributers.pas:159-160`). O resultado do mapeamento é uma **`ITable`** — não existem `ITableDef` nem `ITableData`.

## Persistência (`IEntityManager`, `IUnitOfWork`, `IIdentityMap`)

O `IEntityManager` (`Databases.Interfaces.pas:2097`) trabalha sobre **`ITable`**, não sobre entidades POCO. Fábricas: `TProviders.NewEntityManager` (`Providers.pas:154`), `TEntityManager.New` (`Database.EntityManager.pas:122`) ou `ITable.EntityManager` (`Databases.Interfaces.pas:1375`, reusa campos/valores da `ITable`, sem `MapTable`).

| Método | Assinatura real |
| ------ | --------------- |
| Ligar conexão/tabela | `Connection(AConnection)` (l.2100), `Table(ATable)` (l.2103), `MapTable(ATableName, ASchema='')` (l.2109, descobre a estrutura pelo catálogo) |
| Ler | `Find(AId: Variant): ITable` (l.2118), `List: TArray<ITable>` (l.2119), `ListWhere(AColumn, AOp: string; AValue: Variant): TArray<ITable>` (l.2120) |
| Gravar | `Save(ATable): Integer` (l.2122), `Update(ATable): Integer` (l.2125) |
| Apagar | `Delete(ATable): Integer` (l.2123), `Delete(AId: Variant): Integer` (l.2124) |
| Soft delete | `IncludeDeleted(AValue)` (l.2115) |

**Não existem** `FindAll`, `FindWhere`, `Save(Entity)` com `TObject`.

`IUnitOfWork` (`Databases.Interfaces.pas:2078`; `TUnitOfWork.New`, `Database.UnitOfWork.pas:91`): `Connection` (l.2081), `RegisterNew/RegisterDirty/RegisterDeleted(ATable)` (l.2084-2086), `PendingCount`, `HasPending` (l.2088-2089), `Commit`, `Rollback` (l.2091-2092). **Não tem** `BeginTransaction/AddDirty/Remove` (a transação é do `IConnection`).

`IIdentityMap<T: class>` (`Databases.Interfaces.pas:1832`; `TIdentityMap<T>.New`, `Database.IdentityMap.pas:44`): `Add, Get, TryGet, Contains, Remove, RemoveEntity, Update, Clear, Count, GetAll`.

## Sincronização de esquema (`ISynchronize`)

`TProviders.NewSynchronize(AConnection)` (`Providers.pas:148`) → `ISynchronize` (`Databases.Interfaces.pas:2204`): `Connection(AConn)` (l.2206), `VerifyAfterSync(AValue)` (l.2214), `Plan(ADef: ISchemaDefinition): TArray<TSchemaChange>` (l.2216, só relatório) e `Sync(ADef): Integer` (l.2220, aplica em transação; idempotente). É migração **por diferença** (diff-based), não changelog versionado por nome.

## Métodos reais de manipulação de dados (sem SQL escrito à mão)

> Guia de receitas antes→depois: `Documentation/RegrasNegocio/000-Banco_de_Dados/GestorERP_000-Banco_de_Dados_ProvidersORM_UsageGuide_V1_0_0.md` (se existir na árvore actual do projecto).

| Tarefa | Use (assinatura real, `Databases.Interfaces.pas`) | Em vez de |
|---|---|---|
| Inserir/atualizar/apagar 1 registo | `ITable.ExecuteInsert/ExecuteUpdate/ExecuteDelete(AConnection): Integer` (l.1070-1072); `ITable.Execute(AConnection)` despacha pelo estado da linha (l.1086) | `TFDQuery`+`ExecSQL` |
| CRUD orquestrado | `IEntityManager.Find/List/ListWhere/Save/Update/Delete` (l.2118-2125) | SQL manual |
| Criar tabela (DDL) | `ITable.CreateTableSQL(ASchema='')` (l.1091) devolve o texto; `ITable.CreateTable(AConnection)` (l.1093) executa; FK via `IField.ReferencedTable/OnDeleteRule` (l.851/857) | `CREATE TABLE` × dialetos |
| Auditoria/soft-delete | `ITable.AuditFields(AValue)` (l.1227) + `FieldDateCreated` (l.1229) / `FieldIsDeleted` (l.1233) | setar `updated_at` à mão |
| Consulta com filtros/joins/agregação | `IQueryBuilder.Select/From/Join/Where/GroupBy/OrderBy/Limit`; agregados `Count` (l.2330), `Sum(AColumn, AAlias='')` (l.2331); `Execute: TDataSet` (l.2433) | SELECT à mão |
| INSERT/UPDATE por builder | `InsertInto` (l.2415) + `Value` (l.2416) / `SetValue` (l.2418) + `ExecuteMutation: Integer` (l.2434) | SQL manual |
| SQL ad-hoc com input do utilizador | `IConnection.ExecuteQuery/ExecuteCommand(ASQL, [params])` com `:param0..` | concatenação |
| Ler schema de um banco existente | `ITables.LoadFromConnection(ASchema='')` (l.1405), `ITables.GetTableStructure(ATableName, ASchema='')` (l.1415); `IConnection.GetTableStructure` (`Connections.Interfaces.pas:169`) | inspeção manual |

**Nomes que NÃO existem** (não usar): `GetSQLCreateTable` (o nome real é `CreateTableSQL`), `SelectSum` (é `Sum`), `IEntityManager.FindAll/FindWhere`, `IAttributeMapper.MapClass` (é `FromClass`), `ITableDef`, `ITableData`, `IAttributeRegistry.Register`, `IUnitOfWork.BeginTransaction/AddDirty/Remove`, `[Nullable]`, `[AutoIncrement]`, `FromParameters`.

### Literais x parâmetros (facto técnico) e política D-N

- **Facto — `ITable.ExecuteInsert/ExecuteUpdate/ExecuteDelete` usam literais:** `TTable.InsertSQL` monta os valores com `QuotedStr(VarToStr(...))` (`Database.Table.pas:980`) e `UpdateSQL` idem (`:1019`); o comando segue por `AConnection.ExecuteCommand(LSQL)` sem params. Não há binds nesse caminho.
- **Facto — o `IQueryBuilder` do v3 liga os valores como parâmetros** (`Database.QueryBuilder.pas:14` "PARAMETROS FIRST-CLASS BINDADOS"; `:1764` `FConnection.ExecuteQuery(LSql, LParams)`; `ToSQL` devolve `:paramN` e `Params` os valores, `Databases.Interfaces.pas:2431-2432`). Excepções sem bind: `WhereRaw` (l.2380), `SelectRaw` (l.2327), `FromVirtual` (l.2339).
- **Política do GestorERP (decisão D-N, 20/10/2026):** input digitado pelo utilizador **não entra** no `IQueryBuilder` (nem em `ITable.Execute*`); vai **sempre** por `IConnection.ExecuteQuery/ExecuteCommand(SQL, [params])`. O `IQueryBuilder` fica para valores internos do sistema. A engine é definida por **diretiva de compilação** (`USE_*`), nunca instanciando driver.

## Regra de uso

1. **Ao responder sobre uso do ORM:** confirmar a assinatura no código da biblioteca (`grep -n` na unit) ou na documentação do projecto antes de citar. Não inventar APIs.
2. **Ao alterar** os roteiros (novas seções, novos exemplos, novas APIs): manter esta skill alinhada ao código v3. Se a alteração for genérica (qualquer projecto que use ProvidersORM), propagar para esta skill; se for específica do clone, manter só em `Documentation/` local ou em `.workspace/`.

## Anti-padrões

| Anti-padrão | Por que é errado | Como corrigir |
| ----------- | ---------------- | ------------- |
| Inventar métodos/APIs sem consultar o código | Gera código que não existe no ORM (ver lista de nomes inexistentes) | Confirmar com `grep -n` na unit antes de citar |
| Referenciar units internas (`Database.*.pas`, `Connections.pas`) em código consumidor | Viola encapsulamento — são detalhe interno da biblioteca | Usar as fachadas `Providers.pas`, `Providers.Interfaces.pas`, `Providers.<Modulo>.pas` |
| Activar `USE_ENTITY_MANAGER` sem `USE_ATTRIBUTES` | `USE_ENTITY_MANAGER` depende de `USE_ATTRIBUTES` (`ORM.Defines.inc:169`) | Activar sempre ambas em conjunto |
| Passar input do utilizador ao `IQueryBuilder`/`ITable.Execute*` | Viola a política D-N (e `ITable.Execute*` usa literais) | `IConnection.ExecuteQuery/ExecuteCommand(SQL, [params])` |
| Usar `:p0` nos placeholders | O v3 usa `:param0`, `:param1`, ... (`Connections.pas:3706`) | Usar `:paramN` |
| Instanciar `TFDConnection`/`TZConnection` directamente | Contorna o ORM e a escolha de engine por diretiva | `TProviders.NewConnection` |

## Métricas de sucesso

- Nenhuma API ou método citado que não exista no código v3 (unit:linha confirmada).
- Desenvolvedor consegue implementar uma entidade mapeada + primeira operação CRUD em menos de 30 minutos.
- Code review confirma zero input de utilizador no `IQueryBuilder` e zero referência a units internas fora das fachadas.

## Responsável principal

| Papel    | Quem                              |
| -------- | --------------------------------- |
| Executor | Desenvolvedor ORM                 |
| Revisor  | `documentation-project-expert`    |

---

## Changelog (este arquivo)

- 2.0.0 (21/10/2026): **MAJOR — skill reescrita contra o código real do ProvidersORM v3.0.0** (plano `GestorERP2026-10-20-152500-artefatos-providersorm-v3` v1.2.0, Onda 3, S1; evidência: report `GestorERP2026-10-21-1130-artefatos-providersorm-v3-onda1`). Pasta renomeada com `git mv` (sufixo 1.0.0 → 2.0.0). (a) Removida toda a narrativa do modo legado descontinuado da v2 (o v3 não o tem). (b) Forma de consumo = pasta única (Deploy), unidades pelo nome do ficheiro, fachadas `Providers.pas`/`Providers.Interfaces.pas`/`Providers.<Modulo>.pas`; a árvore por módulos aparece só como "estrutura interna da biblioteca" (decisão D-M). (c) Tabela de métodos reescrita contra as assinaturas reais, cada uma com unit:linha: `GetSQLCreateTable` → `CreateTableSQL`; `SelectSum` → `Sum`; `IEntityManager.FindAll/FindWhere/Save(Entity)` removidos (o EntityManager opera sobre `ITable`: `Find/List/ListWhere/Save/Update/Delete`); `IAttributeMapper.MapClass` → `FromClass`; `ITableDef`/`ITableData`, `IAttributeRegistry.Register`, `IUnitOfWork.BeginTransaction/AddDirty/Remove`, `[Nullable]`, `[AutoIncrement]` removidos (inexistentes); atributos reais = 3 (`Table`, `Field`, `PrimaryKey`); placeholders `:param0..` (não `:p0`); `UnitOfWork` real = `RegisterNew/RegisterDirty/RegisterDeleted/PendingCount/HasPending/Commit/Rollback`. (d) Acrescentados: `ISynchronize`/`TProviders.NewSynchronize` (diff-based), `TDatabaseTypes` com 10 valores, `BeginTransaction/Commit/Rollback` do `IConnection`, pool, `IIdentityMap`. (e) Corrigida a afirmação falsa sobre o `IQueryBuilder`: facto (o v3 liga os valores como parâmetros; é `ITable.ExecuteInsert/Update` que usa literais `QuotedStr`) + política D-N (input do utilizador nunca no `IQueryBuilder`; vai por `ExecuteQuery/ExecuteCommand(SQL,[params])`). (f) Removidas as referências a rules e skills inexistentes no pack (regra de paths locais, rules Inicial/roadmap, skill de diretivas de compilação antiga). (g) FileVersion da tabela = frontmatter = sufixo da pasta = 2.0.0. Links cruzados internos actualizados.
- 1.3.0 (27/06/2026): Nova secção **"Métodos de manipulação de dados (sem SQL puro)"** — tabela de métodos (`ITable`/`IEntityManager`/`IQueryBuilder`/`IConnection`) para CRUD/DDL/consultas, com ponteiro ao guia `ProvidersORM_UsageGuide`. Reforça evitar SQL puro/drivers diretos. Acompanha a remoção da família FireDAC (skills-pack-manifest E15). *Nota (reconciliação 21/10/2026): esta versão foi aplicada ao conteúdo sem renomear a pasta, que ficou com sufixo 1.0.0 e a tabela FileVersion em 1.0.0 — dessincronização tripla, corrigida na 2.0.0.*
- 1.2.0 (11/04/2026): Criados `consultas_rapidas/quick_ref.md` e os roteiros em `exemplos/`; bump FileVersion 1.1.0 → 1.2.0. *(Em 21/10/2026 o roteiro do modo legado foi retirado; resta `roteiro_attributes.md`.)*
- 1.1.0 (09/04/2026): Migração V2 — `thinking: extended`, `category: project`, Responsabilidade única, When NOT to use, Dependências, Anti-padrões, Métricas de sucesso, Responsável principal adicionados.
- 1.0.1 (30/03/2026): Rubrica de versionamento interno (política: `.cursor/VERSION.md`).
- 1.0.0 (30/03/2026): Versionamento interno inicial (pacote `.cursor`). *Nota: a entrada de 17/04/2026 (renomeação da pasta, remoção do modo legado, documentação canónica por projecto) foi absorvida e superada pela 2.0.0.*
