GestorERP · RN-S01-001 — Autenticação via LDAPS/AD + JWT HS256 | V1.0
=====================================================================

GestorERP · Regra de Negócio

**ID da Regra**: RN-S01-001
**Módulo**: S01 — Seguranca_Acesso
**Taxonomia**: Segurança
**Origem (legado)**: RN-M01-002 (Authentication V1.0.0 do GestorERP-legacy migrada para taxonomia V3.2.0)
**Origem (GDoc)**: M01-Autenticacao (referência 13/13 conformidade — audit GDoc 2026-05-08)
**URL preservada (D25)**: `/api/v1/security/login` (EN — P18=A; conviver permanente; ver `Documentation/Canonical/api-contracts/url-freeze_V1.0.md`)
**Fase**: Fase 1 (B0 Fundação — sprint S0)
**Prioridade**: Alta
**Status**: Em detalhamento
**Título**: Autenticação de utilizador via LDAPS/AD (pilar amLdap) ou hash local (pilar amLocal) — emissão de par JWT HS256 (access + refresh)
**Ref. Arquitetura**: GestorERP_Arquitetura_PROJETO_V1_0_0.md · Cap. 2 §Auth; ADR-OBAC-001; ADR-JWT-001
**Multi-tenant (D19)**: aplicável (JWT injecta `tenant_id` no claim — P20=A)

## PRÉ-CONDIÇÕES — O que deve ser verdadeiro antes desta regra ser aplicada

1. `TSecurityMain.Bootstrap` executado com `SetLdap(...)` e `SetDb(...)` concluídos.
2. `[auth] auth_mode` em `database.ini` define `ldap` (pilar amLdap) ou `local` (pilar amLocal).
3. Para `amLdap`: service-account LDAP com bind-DN válido + porta 636 (LDAPS) acessível + certificado TLS do AD válido no truststore.
4. Para `amLocal`: tabela `dbo.Users` com `password_hash` + `salt` (bcrypt/argon2) populada.
5. Horse activo com `-DUSE_HORSE` definido; middleware JWT registado para rotas `/api/v1/*` (excepto `/auth/login` e `/module`).
6. Multi-tenant (D19): `S01-Seguranca_Acesso` injecta `tenant_id` no JWT claim no login; backend extrai via `Req.Token.GetClaim('tenant_id')`.

## FLUXO PRINCIPAL — Sequência feliz (passo a passo quando tudo funciona)

1. Cliente envia `POST /api/v1/security/login` com `Content-Type: application/json` e body `{username, password, hostname, empresaId}`.
   > **URL preservada D25:** path literal `/api/v1/security/login` (EN — herdado do GDoc M01); JSON shape preservado; Careli cliente não precisa atualizar.
2. `TS01ControllersAuth.HandleLogin` parseia body (parse-only; zero SQL em Controllers).
3. (Opcional) `TS01ServicesMachine.New.Hostname(h).IsEnabled` — gate de máquina (RN-S01-008).
4. `TS01ServicesAuth.New.Mode(amLdap).Username(u).Password(p).Login`:
   - `amLdap` → `IActiveDirectoryConnection.Authenticate(u, p)` via ActiveDirectoryORM (bind-then-search; LDAPS porta 636).
   - `amLocal` → `IConnection.EntityManager.Find<TUser>(uname)` + `bcrypt-compare(password, hash, salt)` (via A05-Cripto_Engine).
5. Em sucesso: popula `TAuthResult{Success, UserName, Email, DistinguishedName, Groups, EmpresaId, TenantId}`.
6. Controller delega a `TS01ServicesSession.FromAuth(R).Secret(SecretFromIni).Issue` → par JWT (`access_token` 1h + `refresh_token` 7d) com claims: `sub`, `email`, `groups`, `tenant_id` (P20=A), `exp` (RN-S01-004).
7. Response 200 com `TResponse<TSessionInfo>` JSON (shape preservado do GDoc).
8. Audit `asInfo` com `action=auth.login`, `success=true`, IP, hostname (RN-S01-009).
9. Cliente armazena JWT em memória (nunca em arquivo/registry); envia em `Authorization: Bearer <jwt>` em pedidos subsequentes.

## FLUXOS DE EXCEÇÃO — O que acontece quando algo dá errado

- **E1. username/password vazio**
  - `HTTP 400 { "error": "invalid_input", "message": "username/password required" }`
  - Não consulta LDAP/BD.

- **E2. Credenciais erradas (LDAP ou local)**
  - `HTTP 401 { "error": "unauthorized" }`
  - Audit registado com `success=false`.
  - Incrementa `dbo.Users.failed_login_count` (RN-S01-005).

- **E3. LDAP indisponível (amLdap)**
  - Timeout 5s ou erro de bind.
  - `HTTP 502 { "error": "ldap_bind_failed" }`
  - Fallback NÃO automático para amLocal (segurança).

- **E4. Máquina não autorizada (se gate ligado — RN-S01-008)**
  - `HTTP 403 { "error": "forbidden", "message": "machine not authorized" }`
  - Bloqueia ANTES do bind LDAP.

- **E5. Utilizador bloqueado/disabled**
  - `HTTP 403 { "error": "forbidden: user disabled" }`
  - `dbo.Users.enabled = FALSE` ou `dbo.Users.locked_until > now()`.

- **E6. BD indisponível (amLocal)**
  - `HTTP 503 { "error": "database_unavailable" }`
  - Retry com backoff exponencial até 3 tentativas.

## VALIDAÇÕES

| Campo / Dado | Condição / Regra | Mensagem de Erro | HTTP |
|---|---|---|---|
| `username` | Não vazio; length ≥ 3 chars | `invalid_input: username required` | 400 |
| `password` | Não vazio; length ≥ 1 char | `invalid_input: password required` | 400 |
| `hostname` | Não vazio; obtido cross-platform via `TIdStack.LocalHostName` | `invalid_input: hostname required` | 400 |
| `empresaId` | Integer > 0 | `invalid_input: empresaId` | 400 |
| LDAPS TLS | Certificado válido e não expirado | `ldap_security_error` | 502 |
| `account status` | `dbo.Users.enabled = TRUE` AND `locked_until` IS NULL OR `< now()` | `account_disabled` ou `account_locked` | 403 |

## TABELAS / CAMPOS DO BANCO DE DADOS

| Tabela | Op. | Campos Relevantes | tenant_id (D19) |
|---|---|---|---|
| `dbo.Users` | R (amLocal) | `username, password_hash, salt, enabled, empresa_id, tenant_id, locked_until` | obrigatório (filtrado por C06-Tenant_Context FAIL-CLOSED) |
| `dbo.Users` | W | `last_login_at, failed_login_count, locked_until` | obrigatório |
| `dbo.AuditLog` | W | `action=auth.login, user_id, ip, hostname, success, timestamp` | obrigatório |
| `dbo.TokenBlocklist` | (n/a aqui) | — | obrigatório |

## IMPACTO EM OUTRAS RNs

- **RN-S01-002** — não aplicável (esta é a 002 actual).
- **RN-S01-003 (OBAC)** — JWT emitido aqui é consumido pelo middleware OBAC para decidir autorização por endpoint.
- **RN-S01-004 (Session Lifecycle)** — par JWT (access + refresh) emitido aqui segue ciclo de vida com rotação de refresh + blocklist.
- **RN-S01-005 (Users)** — `dbo.Users.failed_login_count` incrementado em cada falha; bloqueio temporário após 5 tentativas.
- **RN-S01-008 (Machines)** — gate de máquina executado antes do bind LDAP/local (se ativo).
- **RN-S01-009 (Audit)** — cada tentativa (sucesso ou falha) escreve linha em `dbo.AuditLog`.
- **RN-C06-001 (Tenant_Context)** — `tenant_id` injetado no JWT alimenta o interceptor multi-tenant em todas as queries subsequentes (FAIL-CLOSED).

## LGPD — Dados pessoais envolvidos, base legal e prazo de retenção

- **Dados tratados**: `username`, `email`, `hostname`, IP do cliente, DN LDAP, grupos AD, `tenant_id`, timestamp de acesso.
- **Base legal**: execução de contrato + legítimo interesse em segurança — Art. 7°, incisos V e IX, LGPD.
- **Retenção**:
  - Tentativas de login em `dbo.AuditLog` por 6 anos (RN-S01-009).
  - `failed_login_count` purgado após reset (login bem-sucedido).
  - `TokenBlocklist` purga após TTL máximo do refresh_token (7d).

## ESBOÇO DE IMPLEMENTAÇÃO — Delphi + Horse + ProvidersORM + ActiveDirectoryORM

```pascal
// Modules/Controllers/Controllers.Auth.pas (S01-Seguranca_Acesso)
unit Controllers.Auth;

interface

uses Horse;

type
  TS01ControllersAuth = class
  public
    class procedure RegisterRoutes;
    class procedure HandleLogin(Req: THorseRequest; Res: THorseResponse);
  end;

implementation

uses
  Services.Auth.Interfaces,    // IS01ServicesAuth (DI)
  Services.Session.Interfaces, // IS01ServicesSession
  Commons.Security.Types,      // TAuthResult, TSessionInfo
  Commons.Message.Response;    // TResponse<T>

class procedure TS01ControllersAuth.RegisterRoutes;
begin
  // D25 — URL literal preservada do GDoc M01: NÃO alterar
  THorse.Post('/api/v1/security/login', HandleLogin);
end;

class procedure TS01ControllersAuth.HandleLogin(Req: THorseRequest; Res: THorseResponse);
var
  AuthSvc:    IS01ServicesAuth;
  SessionSvc: IS01ServicesSession;
  AuthResult: TAuthResult;
  Session:    TSessionInfo;
begin
  // Zero SQL aqui — Controllers só parseia/serializa
  AuthSvc := GContainer.Resolve<IS01ServicesAuth>;
  AuthResult := AuthSvc
    .Mode(AuthModeFromString(Parameters.Get('auth', 'auth_mode', 'ldap')))
    .Username(Req.Body.Field('username').AsString)
    .Password(Req.Body.Field('password').AsString)
    .Hostname(Req.Body.Field('hostname').AsString)
    .EmpresaId(Req.Body.Field('empresaId').AsInteger)
    .Login;  // exception se falha; controller só captura HTTP errors

  SessionSvc := GContainer.Resolve<IS01ServicesSession>;
  Session := SessionSvc
    .FromAuth(AuthResult)
    .Secret(Parameters.Get('jwt', 'secret', ''))
    .TtlAccess(3600)
    .TtlRefresh(7 * 24 * 3600)
    .TenantId(AuthResult.TenantId)  // D19 + P20=A: claim tenant_id no JWT
    .Issue;

  Res.Send<TResponse<TSessionInfo>>(TResponse<TSessionInfo>.Ok(Session));
end;

end.
```

```pascal
// Modules/Services/Services.Auth.pas (S01)
function TS01ServicesAuth.Login: TAuthResult;
begin
  case FMode of
    amLdap:  Result := FLdapConn.Authenticate(FUsername, FPassword);  // ActiveDirectoryORM
    amLocal: Result := AuthenticateLocal(FConnection, FUsername, FPassword);  // ProvidersORM + A05 bcrypt
  end;

  if not Result.Success then
    raise EUnauthorized.Create('unauthorized');

  // Tenant resolution (D19)
  Result.TenantId := ResolveTenantForUser(FConnection, Result.UserName, FEmpresaId);
end;
```

## NOTAS / OBSERVAÇÕES

- **Origem legado:** Esta RN substitui `RN-M01-002 Authentication V1.0.0` (em `Documentation/RegrasNegocio/RN-M01 - Segurança e Acesso/GestorERP_RN-M01-002_Authentication_V1_0_0.md`) na taxonomia V3.2.0 (D24+D25). Pasta legada mantida como histórico.
- **Origem GDoc:** M01-Autenticacao (referência 13/13 conformidade). URL `/api/v1/security/login` preservada literal (D25 — P18=A; EN herdado).
- **TryLogin vs Login:** `Login` lança exception em falha; `TryLogin` retorna `Result.Success = false`. Usar `TryLogin` em contextos CLI/script onde exceções são inadequadas.
- **Não confundir com OBAC** — esta RN decide *quem é*; RN-S01-003 (OBAC) decide *o que pode*.
- **Plugin FMX (futuro):** cliente desktop usa `THTTPClient` (cross-platform Windows/macOS/Linux; sem WinAPI). JWT armazenado em `TPluginSession.Current.Token` em memória — nunca em arquivo/registry/NSUserDefaults.
- **Hardening pendente (P21):** 78 rotas no GDoc actual estão sem `RequireAuth` (R24=A — aceito); hardening retroativo só com sprint coordenado Careli + bump V1→V2 do url-freeze.
- **Sprint S0 da FASE 5:** rename M01-Seguranca_Acesso → S01-Seguranca_Acesso + Modulos/ → Modules/ (D05) será feito antes desta RN entrar em produção.

## Assinaturas

- **Elaborado por**: Equipe GestorERP — 2026-05-18
- **Revisado por**: ___________________ — ___/___/______
- **Aprovado por**: ___________________ — ___/___/______
