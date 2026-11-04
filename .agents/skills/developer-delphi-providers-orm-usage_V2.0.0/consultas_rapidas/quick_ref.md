---
description: "Referência rápida — ProvidersORM v3.0.0 (Connection, ITable, IQueryBuilder, EntityManager, Pool)"
alwaysApply: false
---

# Quick Reference — developer-delphi-providers-orm-usage (v2.0.0)

> Consumo: pasta única (Deploy; aqui `src/modules/Providers.v3/src/`). Usar as fachadas `Providers.pas`, `Providers.Interfaces.pas`, `Providers.<Modulo>.pas`. Assinaturas confirmadas no código v3 (unit:linha).

| Área | Quando usar | Unit(s) do v3 (nome do ficheiro) |
|------|-------------|----------------------------------|
| **Connection** | Ligar a uma base, executar SQL parametrizado, transações | `Connections.Interfaces.pas`, `Connections.pas` |
| **ITable / IFields** | DML/DDL gerado pelo ORM (`ExecuteInsert/Update/Delete`, `CreateTableSQL`) | `Databases.Interfaces.pas`, `Database.Table.pas` |
| **Attributes → ITable** | Mapear classe `[Table]/[Field]/[PrimaryKey]` para `ITable`; requer `USE_ATTRIBUTES` | `Attributers.Database.Interfaces.pas`, `Attributers.Database.Registry.pas` |
| **EntityManager** | `Find/List/ListWhere/Save/Update/Delete` sobre `ITable`; requer `USE_ENTITY_MANAGER` | `Database.EntityManager.pas` |
| **QueryBuilder** | SELECT/INSERT/UPDATE fluente com valores bindados; requer `USE_QUERY_BUILDER` | `Database.QueryBuilder.pas` |
| **Synchronize** | Aplicar/planear diferenças de esquema (diff-based) | `Database.Synchronize.pas` |
| **Pool** | Múltiplas conexões concorrentes; requer `USE_POOLCONNECTIONS` | `PoolConnections.Interfaces.pas`, `PoolConnections.pas` |

## Operações Connection + Tables — visão 1-linha

| Operação | Chamada real |
|----------|--------------|
| Criar conexão | `LConn := TProviders.NewConnection` (`Providers.pas:136`) |
| Configurar | `LConn.Host('h').Port(5432).Username('u').Password('p').Database('db')` ou `.FromIniFile(APath, ASection)` / `.FromConfig` / `.FromJSON(AJSON)` |
| Conectar / desconectar | `LConn.Connect` (l.123) / `LConn.Disconnect` (l.124) |
| Executar comando | `LConn.ExecuteCommand(ASQL)` (l.130); parametrizado: `ExecuteCommand(ASQL, [v0, v1])` com `:param0`, `:param1` (l.136) |
| Executar query | `LDS := LConn.ExecuteQuery(ASQL)` (l.129); parametrizado (l.134) |
| Escalar | `LVal := LConn.ExecuteScalar(ASQL)` (l.131) |
| Transação | `LConn.BeginTransaction` (l.142) / `LConn.Commit` (l.143) / `LConn.Rollback` (l.144) |
| Carregar tabelas | `LTables := TTables.New.Connection(LConn).LoadFromConnection` (`Databases.Interfaces.pas:1405`; `Database.Tables.pas:141`) |
| DML de 1 registo | `LTable.ExecuteInsert(LConn)` / `ExecuteUpdate(LConn)` / `ExecuteDelete(LConn)` (`Databases.Interfaces.pas:1070-1072`) |
| DDL | `LTable.CreateTableSQL` (l.1091) / `LTable.CreateTable(LConn)` (l.1093) |

(`Connections.Interfaces.pas` para as linhas de `IConnection`.)

## Regras rápidas

- Input do utilizador: **sempre** `IConnection.ExecuteQuery/ExecuteCommand(SQL, [params])` — nunca no `IQueryBuilder` (política D-N; o v3 liga os valores do `IQueryBuilder` como parâmetros, mas a política do GestorERP mantém a proibição).
- `ITable.ExecuteInsert/Update` usam literais (`QuotedStr`, `Database.Table.pas:980/1019`).
- Nomes que não existem: `GetSQLCreateTable` (é `CreateTableSQL`), `SelectSum` (é `Sum`), `FindAll/FindWhere`, `MapClass` (é `FromClass`), `FromParameters`.

→ Exemplos com atributos: [exemplos/roteiro_attributes.md](../exemplos/roteiro_attributes.md)
→ Skill completa: [SKILL.md](../SKILL.md)
