---
description: "Exemplos de uso do módulo Parameters — INI, JSON, Database, agregador, loader de conexão (ProvidersORM 3.0.0)"
alwaysApply: false
---

# Exemplos — developer-delphi-providers-parameters (2.0.0)

> Requer `{$DEFINE USE_PARAMETERS}` em `ORM.Defines.inc`.
> Os exemplos usam apenas assinaturas confirmadas em `Parameters.pas:59-63`, `Parameters.Interfaces.pas` e `Providers.Parameters.pas:80`; **não foram compilados** nesta revisão (documentação) — validar na IDE antes de copiar. Valores são placeholders.

## 1. Ler de INI (caminho e seção explícitos)

```pascal
uses Providers.Parameters;   // fachada; aliases IParametersIniFile, TParameter

var
  LIni: IParametersIniFile;
  LVal: TParameter;
begin
  LIni := TParameters.NewIniFile.FilePath('Data/config.ini').Section('Database');
  LVal := LIni.Select('host');          // nil se não existir
  try
    if LVal <> nil then
      Writeln(LVal.Value);              // Value é string
  finally
    LVal.Free;
  end;
end;
```

## 2. JSON (string ou arquivo)

```pascal
var
  LJson: IParametersJsonObject;
begin
  LJson := TParameters.NewJsonObject;
  LJson.LoadFromString('{"Database":{"host":"localhost"}}');   // Parameters.Interfaces.pas:253
  // ou: LJson.FilePath('Data/config.json').LoadFromFile;       // :252
end;
```

## 3. Banco de dados (tabela de parâmetros)

```pascal
var
  LDb: IParametersDatabase;
begin
  LDb := TParameters.NewDatabase.FromConfig('perfil');   // :189 — perfil resolvido pelo loader/config
  LDb.EnsureTable;                                       // :197 — idempotente (cria + migra)
  Writeln(LDb.Count);                                    // :121
end;
```

## 4. Gravar / consultar (qualquer fonte)

```pascal
var
  LPar: TParameter;
begin
  LPar := TParameter.Create;
  try
    LPar.Name := 'timeout';
    LPar.Value := '30';
    LDb.Save(LPar);                   // upsert — Insert/Update/Save em :109-114
  finally
    LPar.Free;
  end;
  if LDb.Exists('timeout') then       // :119
    LDb.Delete('timeout');            // :115
end;
```

## 5. Agregador com prioridade (fallback declarativo)

```pascal
var
  LParams: IParameters;
  LVal: TParameter;
begin
  LParams := TProviders.NewParameters;                          // Providers.pas:160 — sem argumentos
  LParams.Priority([psDatabase, psInifiles, psJsonObject]);     // Parameters.Interfaces.pas:281
  LVal := LParams.Select('host');                               // :290
  try
    // usar LVal.Value
  finally
    LVal.Free;
  end;
  // sub-fontes: LParams.Database / LParams.IniFile / LParams.JsonObject (:307-309)
end;
```

## 6. Conexão a partir de parâmetros (loader + FromConfig)

```pascal
uses Providers, Providers.Parameters;

var
  LParams: IParameters;
  LConn: IConnection;
begin
  LParams := TProviders.NewParameters;
  TProvidersParameters.RegisterConnectionLoader(LParams);   // uma vez, na inicialização
  LConn := TProviders.NewConnection
    .ConfigFilePath('perfil')    // perfil lido pelo loader
    .FromConfig
    .Connect;
  try
    // usar LConn
  finally
    LConn.Disconnect;
  end;
end;
```

Sem o loader registado, `FromConfig` lança `EConnectionConfigurationException` (`Connections.pas:1846-1851`). `FromParameters` **não existe** no v3.

## Referência canônica

- Units: `Parameters.Interfaces.pas`, `Parameters.pas`, `Parameters.Connections.pas`, `Providers.Parameters.pas`, `Providers.pas` (pasta única de consumo; a árvore por módulos é só estrutura interna da biblioteca).
- Código vivo na cópia do projeto: `src/modules/Providers.v3/src/`.
