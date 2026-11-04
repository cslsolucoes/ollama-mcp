GestorERP · RN-Z01.a-001 — Cadastro de Cliente Contábil (CNPJ + Sócios + Responsabilidades) | V1.0
=====================================================================

GestorERP · Regra de Negócio

**ID da Regra**: RN-Z01.a-001
**Módulo**: Z01.a — Clientes_Contabeis (sub-letra `a` do vertical Z01 Escritório Contábil — D24)
**Taxonomia**: Vertical (estende M13-Clientes_Base via D18)
**Origem (legado)**: RN-M03-001 (Clientes V1.1.0 do GestorERP-legacy migrada parcialmente)
**Origem (GDoc)**: M02-Clientes (referência conformidade audit GDoc)
**URL preservada (D25)**: `/api/v1/clients/...` (EN — P18=A; conviver permanente; ver `Documentation/Canonical/api-contracts/url-freeze_V1.0.md`)
**Fase**: Fase 7 (B7 Vertical Z01 Escritório Contábil — sprint S11.1)
**Prioridade**: Alta
**Status**: Em detalhamento
**Título**: Cadastro completo de cliente contábil — empresa (CNPJ ativa/inativa/baixada), sócios com QSA, responsabilidades fiscais (PIS/COFINS/CSLL/IRF/ISS), regime tributário, vínculo Dexion
**Ref. Arquitetura**: GestorERP_Arquitetura_PROJETO_V1_0_0.md · §11; ADR-Multiempresa-001
**Multi-tenant (D19)**: aplicável (escritório contábil = 1 tenant; cliente contábil = 1 empresa atendida pelo escritório)

## PRÉ-CONDIÇÕES — O que deve ser verdadeiro antes desta regra ser aplicada

1. **M13-Clientes_Base** scaffoldado (sprint S9.3) — Z01.a estende M13 com features contábeis (D18).
2. **S01-Seguranca_Acesso** activo — operador autenticado com role `contador` ou `operador` + permissão OBAC para `clients:create`.
3. **B01-Dexion_Integration** disponível (sprint S5) — cache de empresas Dexion para auto-fill de razão social / endereço por CNPJ.
4. **C06-Tenant_Context** activo — todas as queries filtradas por `tenant_id` do escritório contábil em sessão (FAIL-CLOSED).
5. Tabelas Z01.a populadas: `dbo.ClientesContabeis`, `dbo.Socios`, `dbo.Responsabilidades`, `dbo.TributosCliente`, `dbo.RegimesTributarios` (seeds: Simples Nacional, Lucro Presumido, Lucro Real, MEI).
6. Multi-tenant: o `tenant_id` no JWT corresponde ao escritório contábil; o cliente contábil cadastrado é "filho" desse escritório.

## FLUXO PRINCIPAL — Sequência feliz (passo a passo quando tudo funciona)

1. Operador autenticado envia `POST /api/v1/clients` com `Authorization: Bearer <jwt>` e body JSON contendo: `cnpj`, `razaoSocial`, `nomeFantasia`, `regimeTributarioId`, `dataAbertura`, `endereco` (objeto), `contatos` (array), `socios` (array), `responsabilidadesFiscais` (objeto: `pis, cofins, csll, irf, iss` com flags ativo/aliquota), opcional `idEmpresaDexion`.
   > **URL preservada D25:** path literal `/api/v1/clients` (EN — herdado do GDoc M02-Clientes); JSON shape `{cnpj, razaoSocial, ...}` preservado. Tag Swagger interna pode reorganizar para `Z01-Contabil/Clientes` (refatoração apenas de tags).
2. `TZ01aControllersClientes.HandleCreate` parseia body e delega ao Service.
3. `TZ01aServicesClientes.Cnpj(c).Validar`:
   a. Validar dígito verificador CNPJ (via `A05-Cripto_Engine.Validators`).
   b. Consultar Dexion via `B01-Dexion_Integration` para auto-fill (se `idEmpresaDexion` fornecido OU buscar por CNPJ).
   c. Verificar unicidade `(tenant_id, cnpj)` em `dbo.ClientesContabeis` (D19 — FAIL-CLOSED filtra automaticamente por tenant).
4. Begin transaction.
5. Insert em `dbo.ClientesContabeis` (M13.cliente_base_id + campos Z01.a específicos: regime, data_abertura, classificacao_porte).
6. Insert em `dbo.Socios` para cada sócio (CPF/CNPJ, percentual_capital, qualificacao).
7. Insert em `dbo.Responsabilidades` (PIS/COFINS/CSLL/IRF/ISS — flags + alíquotas + base de cálculo).
8. Insert em `dbo.TributosCliente` (relacionamento N:N com `dbo.Tributos`).
9. Commit transaction.
10. Audit `asInfo` action=`clients.create` (RN-S01-009 + RN-S03-001).
11. Response 201 com `TResponse<TClienteContabil>` JSON shape preservado.
12. Evento `ClienteContabilCriado` (Outbox) para consumidores Z01.b (Certidões), Z01.c (DocumentosFiscais), Z01.e (Execução de Serviços).

## FLUXOS DE EXCEÇÃO — O que acontece quando algo dá errado

- **E1. CNPJ inválido (DV errado)**
  - `HTTP 400 { "error": "invalid_input", "message": "cnpj_dv_invalid" }`
  - Não chama Dexion.

- **E2. CNPJ já cadastrado (no tenant)**
  - `HTTP 409 { "error": "conflict", "message": "cnpj_already_registered", "existingClientId": "..." }`
  - FAIL-CLOSED garante que duplicação só é detectada dentro do mesmo tenant.

- **E3. Dexion indisponível**
  - `HTTP 502 { "error": "dexion_unavailable" }` — apenas se `idEmpresaDexion` fornecido obrigando consulta.
  - Se não-obrigatório: auto-fill skipped; segue fluxo normal com warning no audit.

- **E4. Regime tributário inexistente**
  - `HTTP 400 { "error": "invalid_input", "message": "regime_tributario_not_found", "id": "..." }`

- **E5. Sócio com CPF/CNPJ inválido**
  - `HTTP 400 { "error": "invalid_input", "message": "socio_documento_invalid", "index": N }`
  - Toda a transação aborta (atomicidade).

- **E6. Sem permissão OBAC**
  - `HTTP 403 { "error": "forbidden", "message": "obac_denied: clients:create" }`

- **E7. BD indisponível**
  - `HTTP 503 { "error": "database_unavailable" }`
  - Rollback automático da transação.

## VALIDAÇÕES

| Campo / Dado | Condição / Regra | Mensagem de Erro | HTTP |
|---|---|---|---|
| `cnpj` | DV CNPJ válido + 14 dígitos | `cnpj_dv_invalid` ou `cnpj_format` | 400 |
| `razaoSocial` | Não vazio; length 3-200 | `razaoSocial_required` | 400 |
| `regimeTributarioId` | Existe em `dbo.RegimesTributarios` AND `ativo = TRUE` | `regime_tributario_not_found` | 400 |
| `dataAbertura` | Data válida; ≤ hoje | `data_abertura_invalid` | 400 |
| `socios[].documento` | DV CPF (11d) ou CNPJ (14d) válido | `socio_documento_invalid` | 400 |
| `socios[].percentualCapital` | 0 < x ≤ 100; soma de todos = 100 ± 0.01 | `socios_capital_sum_invalid` | 400 |
| `responsabilidadesFiscais.iss.aliquota` | 0 ≤ x ≤ 10 (range Simples Nacional) | `iss_aliquota_invalid` | 400 |
| Unicidade `(tenant_id, cnpj)` | Sem duplicação dentro do tenant | `cnpj_already_registered` | 409 |
| OBAC | Permissão `clients:create` no `tenant_id` da sessão | `obac_denied` | 403 |

## TABELAS / CAMPOS DO BANCO DE DADOS

| Tabela | Op. | Campos Relevantes | tenant_id (D19) |
|---|---|---|---|
| `dbo.ClientesContabeis` | W | `id, cliente_base_id (FK M13), tenant_id, cnpj, razao_social, nome_fantasia, regime_tributario_id, data_abertura, classificacao_porte, id_empresa_dexion, ativo, criado_em` | obrigatório (FAIL-CLOSED) |
| `dbo.Socios` | W | `id, cliente_contabil_id, tenant_id, documento (CPF/CNPJ), nome, percentual_capital, qualificacao, data_entrada` | obrigatório |
| `dbo.Responsabilidades` | W | `id, cliente_contabil_id, tenant_id, pis_ativo, pis_aliquota, cofins_ativo, cofins_aliquota, csll_ativo, csll_aliquota, irf_ativo, irf_aliquota, iss_ativo, iss_aliquota` | obrigatório |
| `dbo.TributosCliente` | W | `cliente_contabil_id, tributo_id, tenant_id, ativo, data_vigencia_inicio, data_vigencia_fim` | obrigatório |
| `dbo.RegimesTributarios` | R | `id, nome, descricao, ativo` (sem tenant — config global) | n/a (seed global) |
| `dbo.AuditLog` | W | `tenant_id, action=clients.create, user_id, entity_id, ip, timestamp, payload_hash` | obrigatório |
| `dbo.Outbox` | W | `tenant_id, event_type=ClienteContabilCriado, payload, criado_em` | obrigatório |

## IMPACTO EM OUTRAS RNs

- **RN-M13-001 (Clientes_Base)** — Z01.a herda M13 (cliente_base_id FK); criação aqui pode auto-criar registo em M13 (cascata) ou exigir M13 pré-existente.
- **RN-Z01.b-001 (Certidoes)** — consome evento `ClienteContabilCriado` para emitir job de polling de certidões iniciais (CND, CNDT, FGTS).
- **RN-Z01.c-001 (DocumentosFiscais)** — usa `cliente_contabil_id` + `regime_tributario_id` para parametrizar parser NF-e/NFSe.
- **RN-Z01.d-001 (Assinatura_ICP)** — quando cliente sobe certificado A1/A3 (S02-Certificado_Digital), associação é feita via `cliente_contabil_id`.
- **RN-Z01.e-001 (Execução de Serviços ⭐ D20)** — workflow Careli depende de `cliente_contabil_id` + `responsabilidadesFiscais` para gerar tarefas mensais.
- **RN-Z01.g-001 (Tributos_Calculo_Fiscal)** — alimentado por `responsabilidadesFiscais` e `regime_tributario_id`.
- **RN-Z01.h-001 (Obrigacoes_Acessorias)** — DIMOB/RAIS/DCTF derivam de `regime_tributario_id` + `socios`.
- **RN-Z01.i-001 (eSocial_RH)** — folha de pagamento puxa CNPJ + `responsabilidadesFiscais` daqui.
- **RN-B01-001 (Dexion_Integration)** — leitura read-only consultada no auto-fill.
- **RN-C06-001 (Tenant_Context)** — todas as queries acima filtradas automaticamente por `tenant_id` (FAIL-CLOSED).
- **RN-S03-001 (Auditoria)** — toda criação escreve linha em AuditLog.
- **RN-S04-001 (OBAC)** — verifica permissão `clients:create` antes de delegar ao Service.

## LGPD — Dados pessoais envolvidos, base legal e prazo de retenção

- **Dados tratados**: CNPJ, razão social, endereço, contatos (email/telefone do responsável), **dados pessoais dos sócios (CPF, nome)** — categoria pessoal sensível tratada como cadastro contábil-fiscal.
- **Base legal**:
  - Para CNPJ/empresa: execução de contrato (Art. 7° V LGPD — contrato de prestação de serviços contábeis).
  - Para sócios: cumprimento de obrigação legal (Art. 7° II LGPD — Receita Federal exige QSA em DIMOB/DCTF).
- **Retenção**:
  - Cliente activo: indefinida enquanto contrato vigente.
  - Cliente baixado: 6 anos após baixa (prazo prescricional fiscal).
  - Audit log: 6 anos.
  - Após retenção: anonimização (CPF/nome → hash) mantendo apenas CNPJ + métricas agregadas.

## ESBOÇO DE IMPLEMENTAÇÃO — Delphi + Horse + ProvidersORM + Z01.a Vertical

```pascal
// Modules/Controllers/Controllers.Clientes.pas (Z01.a)
unit Controllers.Clientes;

interface

uses Horse, Horse.Jwt;

type
  TZ01aControllersClientes = class
  public
    class procedure RegisterRoutes;
    class procedure HandleCreate(Req: THorseRequest; Res: THorseResponse);
  end;

implementation

uses
  Services.Clientes.Interfaces,    // IZ01aServicesClientes
  Commons.ClientesContabeis.Types, // TClienteContabil, TSocio, TResponsabilidades
  Commons.Message.Response;

class procedure TZ01aControllersClientes.RegisterRoutes;
begin
  // D25 — URL literal preservada do GDoc M02-Clientes
  THorse.Post('/api/v1/clients', RequireAuth, RequireObac('clients:create'), HandleCreate);
end;

class procedure TZ01aControllersClientes.HandleCreate(Req: THorseRequest; Res: THorseResponse);
var
  Svc:     IZ01aServicesClientes;
  Input:   TClienteContabilInput;
  Cliente: TClienteContabil;
begin
  Input := TClienteContabilInput.FromJson(Req.Body);

  Svc := GContainer.Resolve<IZ01aServicesClientes>;
  Cliente := Svc
    .Cnpj(Input.Cnpj)
    .RazaoSocial(Input.RazaoSocial)
    .RegimeTributario(Input.RegimeTributarioId)
    .Socios(Input.Socios)
    .Responsabilidades(Input.Responsabilidades)
    .TenantId(Req.Token.GetClaim('tenant_id'))  // D19 + P20=A
    .Create;  // exception → HTTP error

  Res.Status(201).Send<TResponse<TClienteContabil>>(TResponse<TClienteContabil>.Ok(Cliente));
end;

end.
```

```pascal
// Modules/Services/Services.Clientes.pas (Z01.a)
function TZ01aServicesClientes.Create: TClienteContabil;
var
  Empresa: TDexionEmpresa;
begin
  // 1. Validar CNPJ DV via A05-Cripto_Engine
  if not TA05Validators.IsValidCnpj(FCnpj) then
    raise EInvalidInput.Create('cnpj_dv_invalid');

  // 2. Auto-fill via B01-Dexion (opcional)
  if FIdEmpresaDexion > 0 then
    Empresa := FDexionConn.GetEmpresa(FIdEmpresaDexion);

  // 3. Unicidade — C06-Tenant_Context filtra automaticamente
  if FConnection.EntityManager.Count<TClienteContabil>('cnpj = :c', [FCnpj]) > 0 then
    raise EConflict.Create('cnpj_already_registered');

  // 4. Transaction begin
  FConnection.StartTransaction;
  try
    Result := TClienteContabil.Create;
    Result.Cnpj := FCnpj;
    Result.RazaoSocial := FRazaoSocial;
    Result.RegimeTributarioId := FRegimeTributarioId;
    Result.TenantId := FTenantId;  // D19
    FConnection.EntityManager.Insert(Result);

    // Sócios, Responsabilidades, TributosCliente (omitido por brevidade)
    InsertSocios(Result.Id, FSocios);
    InsertResponsabilidades(Result.Id, FResponsabilidades);

    // Outbox event
    FConnection.EntityManager.Insert(TOutboxEvent.New('ClienteContabilCriado', Result.ToJson));

    FConnection.Commit;
  except
    FConnection.Rollback;
    raise;
  end;
end;
```

## NOTAS / OBSERVAÇÕES

- **Origem legado:** Esta RN substitui partes de `RN-M03-001 Clientes V1.1.0` (em `Documentation/RegrasNegocio/RN-M03 - Clientes/`) na taxonomia V3.2.0 (D24). A pasta legada `RN-M03/` permanece como histórico e cobre **clientes genéricos** (M13-Clientes_Base agora) — a especialização contábil migra para Z01.a.
- **Origem GDoc:** M02-Clientes (audit GDoc); URL `/api/v1/clients/...` preservada literal (D25 — P18=A; EN herdado).
- **Estende M13 (D18):** `cliente_base_id` (FK) garante reutilização do cadastro genérico; outros verticais (Z04.b Oficina, Z06.c Vendas) podem referenciar o mesmo cliente_base sem duplicação.
- **Sub-letra `a` (D24):** primeira sub-letra de Z01 — significa "fundação do vertical contábil". Outras sub-letras (b-i) dependem de Z01.a estar pronta.
- **Sprint S11.1 (FASE 5):** pré-requisitos S-1 (URL-Freeze), S0 (M01→S01), S0b (C06-Tenant_Context), S9.3 (M13-Clientes_Base).
- **Dependência crítica D20 ⭐:** Z01.e (Execução de Serviços) depende deste cadastro para gerar tarefas mensais — workflow Careli não fecha sem cliente cadastrado.
- **OBAC permissões:** `clients:read`, `clients:create`, `clients:update`, `clients:delete`, `clients:export` — definidas em RN-S04-001.
- **Padrão multi-tenant:** escritório contábil = 1 tenant; pode ter N clientes; cada cliente é "filho" do tenant. Tenant switching → relogin (RN-S01-004).

## Assinaturas

- **Elaborado por**: Equipe GestorERP — 2026-05-18
- **Revisado por**: ___________________ — ___/___/______
- **Aprovado por**: ___________________ — ___/___/______
