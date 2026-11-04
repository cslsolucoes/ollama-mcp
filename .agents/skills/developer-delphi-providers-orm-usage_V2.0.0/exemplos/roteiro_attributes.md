---
description: "Exemplos ProvidersORM v3 — atributos → ITable, EntityManager sobre ITable, QueryBuilder fluente"
alwaysApply: false
---

# Roteiro Attributes — atributos → ITable + EntityManager + QueryBuilder

> Requer `USE_ATTRIBUTES` (mapeamento) e `USE_ENTITY_MANAGER` (EntityManager) em `ORM.Defines.inc` (`USE_ENTITY_MANAGER` depende de `USE_ATTRIBUTES`); `USE_QUERY_BUILDER` para o builder.
> Delphi XE7+ ou FPC 3.3.1+ com RTTI habilitado nas classes.
> **Estado dos exemplos:** as assinaturas usadas foram confirmadas por `grep -n` em `src/modules/Providers.v3/src/` (unit:linha na secção final); os trechos abaixo **não foram compilados** nesta revisão — validar no projecto antes de copiar.

## Modelo de uso (v3)

No v3 o `IEntityManager` **opera sobre `ITable`**, não sobre instâncias de entidades: o atributo mapeia a *classe* para uma `ITable` (`IAttributeMapper.FromClass`), os valores vivem nos campos da `ITable` (`IField.Value`) e o EntityManager lê/grava `ITable`.

## 1. Declarar a classe com atributos (3 atributos reais)

```pascal
uses
  Attributers.Database.Interfaces; // TableAttribute, FieldAttribute, PrimaryKeyAttribute

type
  [Table('produtos')]
  TProduto = class
  private
    FId: Integer;
    FNome: string;
    FPreco: Currency;
    FAtivo: Boolean;
  public
    [Field('id')] [PrimaryKey]
    property Id: Integer read FId write FId;

    [Field('nome')]
    property Nome: string read FNome write FNome;

    [Field('preco')]
    property Preco: Currency read FPreco write FPreco;

    [Field('ativo')]
    property Ativo: Boolean read FAtivo write FAtivo;
  end;
```

Só existem `[Table]`, `[Field]` e `[PrimaryKey]`. **Não existem** `[Nullable]` nem `[AutoIncrement]`; a coluna autoincremento é propriedade do `IField` (`IsIdentity`, `Databases.Interfaces.pas:845`).

## 2. Classe → ITable (IAttributeMapper.FromClass)

```pascal
uses
  Providers, Providers.Attributers, Commons.Types;

var
  LTable: ITable;
begin
  LTable := TProvidersAttributers.NewDatabaseMapper
              .FromClass(TProduto, dtPostgreSQL);
end;
```

Alternativa com cache por classe/tipo de banco: `GetAttributeRegistry.GetTable(TProduto, dtPostgreSQL)` (não há `Register`; o registo é preenchido em `GetTable`).

## 3. EntityManager.Save (ITable)

```pascal
var
  LEM: IEntityManager;
begin
  LEM := TProviders.NewEntityManager
           .Connection(LConn)
           .Table(LTable);

  LTable.Fields('nome').Value('Caneta Azul');
  LTable.Fields('preco').Value(2.50);
  LTable.Fields('ativo').Value(True);

  LEM.Save(LTable);   // devolve Integer
end;
```

## 4. EntityManager.Find / List / ListWhere

```pascal
var
  LRow: ITable;
  LRows: TArray<ITable>;
begin
  LRow := LEM.Find(42);                       // Find(AId: Variant): ITable
  if Assigned(LRow) then
    Writeln(VarToStr(LRow.Fields('nome').Value));

  LRows := LEM.List;                          // todas as linhas
  LRows := LEM.ListWhere('ativo', '=', True); // filtro interno (ver política abaixo)
end;
```

`FindAll`/`FindWhere` **não existem**. Com `FieldIsDeleted` configurado na `ITable`, `Find/List/ListWhere` excluem soft-deletes salvo `IncludeDeleted(True)`.

## 5. EntityManager.Delete

```pascal
begin
  LEM.Delete(42);          // Delete(AId: Variant): Integer
  // ou: LEM.Delete(LRow); // Delete(ATable): Integer
end;
```

## 6. DML direto na ITable (literais)

```pascal
begin
  LTable.ExecuteInsert(LConn);   // gera INSERT com literais (QuotedStr) e executa
  LTable.ExecuteUpdate(LConn);   // só campos alterados (IsChanged)
  LTable.ExecuteDelete(LConn);
end;
```

`ITable.Execute*` usa literais: para dados vindos do utilizador usar o item 8.

## 7. QueryBuilder — SELECT fluente (valores internos do sistema)

```pascal
uses
  Providers;

var
  LQB: IQueryBuilder;
  LDS: TDataSet;
begin
  LQB := TProviders.NewQueryBuilder;   // requer USE_DATABASE + USE_QUERY_BUILDER
  LDS := LQB
    .Connection(LConn)
    .DatabaseTypes(dtPostgreSQL)
    .Select(['id', 'nome', 'preco'])
    .From('produtos')
    .Where('ativo', True)
    .Where('preco', '>', 1.00)
    .OrderBy('nome')
    .Limit(10)
    .Execute;                          // SELECT parametrizado (valores bindados como :paramN)
  try
    while not LDS.Eof do
    begin
      Writeln(LDS.FieldByName('nome').AsString);
      LDS.Next;
    end;
  finally
    LDS.Free;
  end;
end;
```

Agregados reais: `Count` e `Sum(AColumn, AAlias)` (**não** `SelectSum`). Mutations: `InsertInto('t').Value('c', v).ExecuteMutation`, `Update('t').SetValue('c', v)...ExecuteMutation`.

## 8. Input do utilizador — sempre via IConnection (política D-N)

```pascal
var
  LDS: TDataSet;
begin
  // placeholders :param0, :param1, ... (não :p0)
  LDS := LConn.ExecuteQuery(
    'SELECT id, nome FROM produtos WHERE nome LIKE :param0 AND preco > :param1',
    ['%' + LTextoDigitado + '%', 1.00]);
end;
```

Facto técnico: o `IQueryBuilder` do v3 liga os valores como parâmetros (`Database.QueryBuilder.pas:14, 1764`). **Política do GestorERP:** input do utilizador não entra no `IQueryBuilder`; vai por `ExecuteQuery/ExecuteCommand(SQL, [params])`.

## Referência canónica (nomes de unit; pasta única do Deploy)

Assinaturas usadas acima (todas confirmadas por `grep -n`):

- `Providers.pas`: `NewEntityManager` l.154, `NewQueryBuilder` l.150.
- `Providers.Attributers.pas`: `TProvidersAttributers.NewDatabaseMapper` l.160.
- `Attributers.Database.Interfaces.pas`: `TableAttribute` l.56, `FieldAttribute` l.72, `PrimaryKeyAttribute` l.81, `IAttributeMapper.FromClass` l.98, `IAttributeRegistry.GetTable` l.104.
- `Attributers.Database.Registry.pas`: `GetAttributeRegistry` l.42.
- `Databases.Interfaces.pas`: `IField.Value` l.861-862, `IField.IsIdentity` l.845; `ITable.Fields` l.957; `ITable.ExecuteInsert/Update/Delete` l.1070-1072; `IEntityManager` l.2097 (`Connection` l.2100, `Table` l.2103, `Find` l.2118, `List` l.2119, `ListWhere` l.2120, `Save` l.2122, `Delete` l.2123-2124, `IncludeDeleted` l.2115); `IQueryBuilder` l.2296 (`Select` l.2325, `From` l.2337, `Where` l.2359-2360, `OrderBy` l.2397, `Limit` l.2402, `Execute` l.2433, `ExecuteMutation` l.2434).
- `Connections.Interfaces.pas`: `ExecuteQuery` com params l.134.
- Units de implementação (internas da biblioteca): `Database.EntityManager.pas`, `Database.QueryBuilder.pas`, `Database.IdentityMap.pas`, `Database.UnitOfWork.pas`.
