---
description: "Referência rápida — módulo Parameters (ProvidersORM 3.0.0)"
alwaysApply: false
---

# Quick Reference — developer-delphi-providers-parameters (2.0.0)

| Fonte | Fábrica | Notas |
|-------|---------|-------|
| INI | `TParameters.NewIniFile.FilePath('Data/config.ini').Section('S')` | `NewIniFile` sem argumento; padrão `<EXE>\data\config.ini` |
| JSON | `TParameters.NewJsonObject.LoadFromFile` / `.LoadFromString(S)` | padrão `<EXE>\data\config.json` |
| Database | `TParameters.NewDatabase.FromConfig('perfil')` ou `.Connection(LConn)` | `EnsureTable` cria/migra a tabela |
| Agregador | `TProviders.NewParameters` | sem argumentos; padrão `[psDatabase, psInifiles]` |

## Operações (todas as fontes e o agregador)

`Select(AName): TParameter` (liberar com `Free`) · `Select: TParameterList` · `Insert/Update/Save(TParameter)` · `Delete(AName)` · `Exists(AName)` · `Count` · `Refresh` · `ToJSON`.
`Getter/List/Setter` = `deprecated`. `IParameters` **não tem** `Load/Section/FromConfig/GetString`.

## Ativação

```pascal
// ORM.Defines.inc:170
{$DEFINE USE_PARAMETERS}
```

> Nota histórica: o alias `USE_PARAMENTERS` (typo) foi removido em 05/07/2026 (`ORM.Defines.inc` 1.2.0, `:123-125`). Não usar.

## Units para consumo externo

```pascal
uses Providers, Providers.Parameters;   // fachadas (TProviders.NewParameters, TProvidersParameters)
```

## Integração com Connection (substitui `FromParameters`, que não existe)

```pascal
TProvidersParameters.RegisterConnectionLoader(LParams);   // Providers.Parameters.pas:80
LConn := TProviders.NewConnection.ConfigFilePath('perfil').FromConfig.Connect;
```

→ Exemplos completos: [exemplos/parametros_uso.md](../exemplos/parametros_uso.md)
