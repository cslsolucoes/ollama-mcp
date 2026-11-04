---
description: "Exemplos de uso do módulo Loggers — fábrica, níveis, hooks, canal próprio (ProvidersORM 3.0.0)"
alwaysApply: false
---

# Exemplos — developer-delphi-providers-loggers (2.0.0)

> Requer `{$DEFINE USE_LOGGERS}` em `ORM.Defines.inc`.
> Os exemplos usam apenas assinaturas confirmadas em `Loggers.Interfaces.pas`, `Loggers.pas` e `Providers.pas`; **não foram compilados** nesta revisão (documentação) — validar na IDE antes de copiar.
> A ativação e a configuração de cada canal (pasta, rotação, host, etc.) vêm de Parameters (`USE_PARAMETERS`), não de chamadas fluentes no logger.

## 1. Logger com nível mínimo

```pascal
uses Providers, Providers.Loggers;   // fachadas

var
  LLog: ILogger;
begin
  LLog := TProviders.NewLogger;      // Providers.pas:170 — ou TLoggers.New (Loggers.pas:131)
  LLog.MinLevel(llInfo);             // Loggers.Interfaces.pas:99

  LLog.Info('Aplicação iniciada');
  LLog.Warning('Pool quase esgotado');
  LLog.Error('Falha ao carregar config');
  LLog.Flush;                        // :143 — descarrega a fila assíncrona
end;
```

## 2. Mensagem com argumentos e com exceção

```pascal
try
  // ...
except
  on E: Exception do
  begin
    LLog.Error('Falha na operação %s', ['importar']);   // (AMessage; const AData: array of const)
    LLog.Exception('Falha inesperada', E);              // :122
  end;
end;
```

## 3. Nível por ambiente

```pascal
{$IFDEF DEBUG}
LLog.MinLevel(llDebug);
{$ELSE}
LLog.MinLevel(llWarning);
{$ENDIF}
LLog.Debug('Variável X = 42');          // só em DEBUG
LLog.Warning('Memória acima de 80%');   // sempre
```

## 4. Hooks (cancelar / observar entradas)

```pascal
// TLoggerBeforeWriteEvent = procedure(const AEntry: TLoggerEntry; var ACancel: Boolean) of object
procedure TMeuServico.AntesDeGravar(const AEntry: TLoggerEntry; var ACancel: Boolean);
begin
  // ex.: descartar entradas com dado sensível
  ACancel := Pos('senha', LowerCase(AEntry.Message)) > 0;
end;

LLog.OnBeforeWrite(AntesDeGravar);   // Loggers.Interfaces.pas:150-151
```

## 5. Canal próprio (substitui o antigo "Custom")

```pascal
type
  TCanalMeu = class(TInterfacedObject, ILoggerChannel)
  public
    function Name: string;
    function Write(const AEntry: TLoggerEntry): Boolean;
    function Enabled: Boolean; overload;
    function Enabled(const AValue: Boolean): ILoggerChannel; overload;
    function IsHealthy: Boolean;
  end;

LLog.RegisterChannel(TCanalMeu.Create);   // :95
```

## 6. Canal em memória para testes

Os 13 canais de baseline são registados por `TLoggers.New` (`Loggers.pas:326-340`) e ficam ativos/inativos pela configuração (parâmetro `Enabled` de cada canal). Para testes, ativar o canal `Memory` (`USE_LOGGERS_MEMORY`) na configuração em vez de montar canais à mão; canais de rede (Redis, ElasticSearch, Email) foram validados contra os perfis de teste do cofre `.workspace/DADOS_CONEXAO_TESTE.md` — consultar o cofre, sem copiar credenciais.

## Referência canônica

- Units: `Loggers.pas`, `Loggers.Interfaces.pas`, `Loggers.Channel.*.pas`, `Providers.Loggers.pas`, `Providers.pas` (pasta única de consumo; a árvore por módulos é só estrutura interna da biblioteca).
- Código vivo na cópia do projeto: `src/modules/Providers.v3/src/`.
