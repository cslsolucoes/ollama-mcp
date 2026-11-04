---
description: "Referência rápida — módulo Loggers (ProvidersORM 3.0.0)"
alwaysApply: false
---

# Quick Reference — developer-delphi-providers-loggers (2.0.0)

## Ativação

```pascal
// ORM.Defines.inc:180
{$DEFINE USE_LOGGERS}
// canais: USE_LOGGERS_CSV/CONSOLE/MEMORY/EVENTLOG (:185-188); rede: USE_LOGGERS_NET (:204) + HTTP/EMAIL/WEBSOCKET/SYSLOG/REDIS/ELASTICSEARCH
```

## Fábrica

`TLoggers.New: ILogger` (`Loggers.pas:131`) · `TProviders.NewLogger` (`Providers.pas:170`) · `TProvidersLoggers.New` (`Providers.Loggers.pas:87`). A fábrica `...Factory` do v2 **não existe**.

## Níveis (`TLogLevel`, 9 valores)

`llInfo, llSuccess, llWarning, llError, llCritical, llException, llDebug, llTrace, llDone` — métodos `Info/Success/Warning/Error/Critical/Exception/Debug/Trace/Done` (3 overloads cada) e `Log(ALevel, ...)`. `MinLevel(llWarning)` filtra por severidade.

## 13 canais reais (+ `FileBase`, base) — tabela de decisão

| Canal | Define | Melhor para |
|-------|--------|-------------|
| TextFile | `USE_LOGGERS` | arquivo `.log`, debug rápido |
| Json | `USE_LOGGERS` | parse por máquina |
| CSV | `USE_LOGGERS_CSV` | planilhas |
| Console | `USE_LOGGERS_CONSOLE` | TUIs/serviços em console |
| Memory | `USE_LOGGERS_MEMORY` | buffer circular (testes) |
| EventLog | `USE_LOGGERS_EVENTLOG` | Windows EventLog |
| Database | `USE_LOGGERS` + `USE_PARAMETERS` + `USE_DATABASE` | auditoria consultável |
| Http | `USE_LOGGERS_HTTP` (NET) | agregador REST |
| Email | `USE_LOGGERS_EMAIL` (NET) | alertas críticos |
| WebSocket | `USE_LOGGERS_WEBSOCKET` (NET) | tempo real |
| SysLog | `USE_LOGGERS_SYSLOG` (NET) | RFC5424 |
| Redis | `USE_LOGGERS_REDIS` (NET) | PUBLISH/LPUSH |
| ElasticSearch | `USE_LOGGERS_ELASTICSEARCH` (NET) | Bulk Index |

Sem XML nem Custom. Canais de rede: Delphi-only (`USE_LOGGERS_NET` off em FPC). Canal próprio: implementar `ILoggerChannel` + `RegisterChannel`.

## Uso básico

```pascal
uses Providers, Providers.Loggers;   // fachadas

var LLog: ILogger;
begin
  LLog := TProviders.NewLogger.MinLevel(llInfo);   // 13 canais de baseline já registados; ativação via Parameters
  LLog.Info('Aplicação iniciada');
  LLog.Error('Falha ao conectar');
  LLog.Flush;                                      // no shutdown
end;
```

→ Exemplos completos: [exemplos/loggers_uso.md](../exemplos/loggers_uso.md)
