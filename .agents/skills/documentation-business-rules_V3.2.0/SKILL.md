---
name: documentation-business-rules
description: Cria ou atualiza documentos de Regras de Negócio em `Documentation/RegrasNegocio/` — um arquivo por regra, subpasta por módulo, padrão ERP modular com taxonomia M/S/B/A/I/Z/C/G/T (V3.2.0).
model: sonnet
thinking: extended
category: documentation
license: MIT
copyright: "Copyright (c) 2026 CSL Tech Solutions"
company: "CSL Tech Solutions"
author: "Claiton de Souza Linhares"
---

# Documentation Business Rules — V3.2.0

## Responsabilidade única

Esta skill é a responsável exclusiva pela criação e atualização de documentos de Regras de Negócio no formato padrão — um arquivo por regra, subpasta por módulo, em `Documentation/RegrasNegocio/`. Resolve o problema de regras de negócio dispersas em código-fonte, comentários ou documentos genéricos, centralizando-as em artefactos rastreáveis com fluxo principal, exceções, validações, impactos e assinaturas. Existe separada de `documentation-class-analysis-generator` porque RNs descrevem **comportamento de negócio** (invariantes, pré/pós-condições, políticas) enquanto a análise de classes descreve **estrutura técnica** (API, campos, herança). O formato padrão com 12 secções obrigatórias garante que cada RN seja aprovável e implementável sem ambiguidade.

**V3.2.0 (2026-05-18):** taxonomia ampliada para suportar os 9 prefixos M/S/B/A/I/Z/C/G/T do GestorERP (decisões D01-D25), incluindo sub-letras para verticais ZXX (D24). Compatibilidade com a numeração legada M01-M33 preservada via tabela de mapeamento.

## When NOT to use

- Quando o objetivo for documentar a estrutura técnica de classes/interfaces — usar `documentation-class-analysis-generator`.
- Quando o objetivo for documentar fluxos de arquitetura (diagramas, ADRs, decisões de design) — usar `documentation-architecture`.
- Quando o objetivo for criar esboços de telas ou wireframes — usar `documentation-screen-sketches`.
- Quando o objetivo for apenas identificar lacunas documentais sem criar novos documentos — usar `documentation-project-scan`.
- Quando as RNs ainda não tiverem análise de classe de suporte — executar `documentation-class-analysis-generator` primeiro para ler `Documentation/Analise/` antes de redigir.

## When to use

- Quando o usuário pedir para documentar regras de negócio, políticas, contratos de comportamento e invariantes.
- Quando o scan identificar lacunas em `Documentation/RegrasNegocio/`.
- Quando um módulo da taxonomia M/S/B/A/I/Z/C/G/T precisar de SPECs de regras de negócio detalhadas.

## Estrutura física obrigatória

### Layout canónico (V3.2.0)

```
Documentation/RegrasNegocio/
├── README.md                                                ← hub com índice de todos os módulos
├── GestorERP_Matriz_Rastreabilidade_RN_V*.md                ← RF → RN
├── GestorERP_Matriz_Maturidade_RN_V*.md                     ← maturidade [ ]/[P]/[M]/[X]
├── GestorERP_Status_RNs_Estrutura_V*.md                     ← árvore de módulos
├── README_GOVERNANCA_V*.md                                  ← políticas + tabela legado→canónico
├── README_PRIORIDADE_EXECUCAO_V*.md                         ← ordem P0..Pn + DAG
│
├── RN-S01 - Seguranca_Acesso/                               ← Segurança
│   ├── README.md
│   ├── GestorERP_RN-S01-001_S01_V1_0.md
│   ├── GestorERP_RN-S01-002_S01_V1_0.md
│   └── ...
│
├── RN-M11 - Geografia_CEP/                                  ← Core ERP
│   ├── README.md
│   └── GestorERP_RN-M11-001_M11_V1_0.md
│
├── RN-Z01 - Escritorio_Contabil/                            ← Vertical raiz
│   ├── README.md
│   ├── GestorERP_RN-Z01-001_Z01_V1_0.md                     ← raiz do vertical
│   └── SubModules/
│       ├── a_Clientes_Contabeis/
│       │   ├── README.md
│       │   └── GestorERP_RN-Z01.a-001_Z01.a_V1_0.md         ← sub-letra (D24)
│       ├── b_Certidoes/
│       ├── ...
│       └── i_eSocial_RH/
│
├── RN-B01 - Dexion_Integration/
├── RN-A01 - PDF_Engine/
└── ...
```

**Regra fundamental:** cada regra de negócio é **um arquivo separado**. Nunca agrupar múltiplas regras em um único arquivo.

### Layout legado (preservado como compatibilidade)

Pastas pré-V3.2.0 com numeração antiga M01-M33 (GestorERP-legacy) **permanecem inalteradas** como histórico activo:

```
Documentation/RegrasNegocio/
├── RN-M01 - Segurança e Acesso/         ← legado (mapeia para S01)
├── RN-M02 - Cadastros Base/             ← legado (split em M11-M14)
├── RN-M05 - Financeiro/                 ← legado (split em M16-M22)
├── ...
└── RN-M26 - Caixa/                      ← legado (mapeia para M16)
```

Migração de uma RN legada para a nova taxonomia segue o protocolo da §"Migração de RNs legadas" abaixo.

## Nomenclatura de arquivo individual (V3.2.0)

**Para módulos simples (M/S/B/A/I/C/G/T):**

```
{Projeto}_RN-{Prefix}{nn}-{nnn}_{Prefix}{nn}_V{X}_{Y}.md
```

**Para sub-módulos de verticais ZXX (D24):**

```
{Projeto}_RN-Z{nn}.{letra}-{nnn}_Z{nn}.{letra}_V{X}_{Y}.md
```

| Parte | Descrição | Exemplo |
|---|---|---|
| `{Projeto}_` | Prefixo canônico do projeto | `GestorERP_` |
| `RN-{Prefix}{nn}` | Código do módulo (Prefix + 2 dígitos) | `RN-S01`, `RN-M11`, `RN-Z01` |
| `[.{letra}]` | Sub-letra (opcional; só ZXX/IXX com sub-módulos) | `.a`, `.b`, ... `.z` |
| `-{nnn}` | Número da regra (3 dígitos, pode ter gaps) | `-001`, `-003`, `-007` |
| `_{Prefix}{nn}[.{letra}]` | Repetição do código do módulo | `_S01`, `_Z01.a` |
| `_V{X}_{Y}` | Versão major.minor com underscores | `_V1_0`, `_V2_1` |

**Exemplos:**

- `GestorERP_RN-S01-001_S01_V1_0.md` — Segurança/Acesso, RN 001
- `GestorERP_RN-M11-003_M11_V1_0.md` — Geografia/CEP, RN 003
- `GestorERP_RN-B02-007_B02_V1_0.md` — Serpro Integration, RN 007
- `GestorERP_RN-A01-001_A01_V1_0.md` — PDF Engine, RN 001
- `GestorERP_RN-I01-001_I01_V1_0.md` — IA Core, RN 001
- `GestorERP_RN-Z01-001_Z01_V1_0.md` — Escritório Contábil **raiz**, RN 001
- `GestorERP_RN-Z01.a-001_Z01.a_V1_0.md` — Clientes Contábeis (sub-letra `a` de Z01), RN 001
- `GestorERP_RN-Z01.e-001_Z01.e_V1_0.md` — Execução de Serviços (D20 PRIORITÁRIO; sub-letra `e` de Z01)
- `GestorERP_RN-Z04.a-001_Z04.a_V1_0.md` — OS Ordem de Serviço (sub-letra `a` de Z04 Oficina)
- `GestorERP_RN-C06-001_C06_V1_0.md` — Tenant_Context (multi-tenant interceptor)
- `GestorERP_RN-G01-001_G01_V1_0.md` — Email SMTP
- `GestorERP_RN-T05-001_T05_V1_0.md` — Auto Updater Builder

**Gaps na numeração são propositais** — permitem inserir novas RNs sem renumerar.

## Mapeamento de módulos GestorERP (V3.2.0 — taxonomia M/S/B/A/I/Z/C/G/T)

Fonte canónica: [`Documentation/Canonical/taxonomy-modules_V1.0.md`](../../../Documentation/Canonical/taxonomy-modules_V1.0.md).

### MXX — Core ERP (operacional + financeiro + cadastros base)

| Prefixo | Nome | Origem legado |
|---|---|---|
| M11 | Geografia_CEP | GestorERP-legacy ADR-0001 + Careli/COPA/OFICINAS |
| M12 | Fornecedores | COPA + OFICINAS |
| M13 | Clientes_Base | Careli + COPA + OFICINAS (D18) |
| M14 | Mensageria_Interna | Careli + COPA MSG/MSGTXT/MSGDST |
| M16 | Caixa_Banco | OFICINAS MOV+MOVDIS+TRF (D22) |
| M17 | Contas_Receber_Pagar | COPA + OFICINAS |
| M18 | Reporting_Financeiro | Careli FlxCx + COPA DRE |
| M19 | Estoque_Almoxarifado | COPA EME/EVD + OFICINAS |
| M20 | Chamados_Internos | Careli CE_Chamado + COPA + OFICINAS |
| M21 | Veiculos_Base | OFICINAS VEICULO + COPA |
| M22 | Plano_Contas | OFICINAS SGD/GRD |
| M23 | Formas_Pagamento | COPA UrPos |

### SXX — Segurança

| Prefixo | Nome | Origem legado |
|---|---|---|
| S01 | Seguranca_Acesso | GDoc M01 + M01-ERP (renomeio de M01) |
| S02 | Certificado_Digital | GDoc M08 |
| S03 | Auditoria | Novo |
| S04 | OBAC | Novo |

### BXX — Base / Integrações 3ºs / BD

| Prefixo | Nome | Origem legado |
|---|---|---|
| B01 | Dexion_Integration | GDoc M30 |
| B02 | Serpro_Integration | GDoc M40 |
| B03 | ACBr_Engine | Careli + COPA + OFICINAS (D21) |

### AXX — Auxiliares Estruturais (libs internas)

| Prefixo | Nome |
|---|---|
| A01 | PDF_Engine (Gnostice + eDocEngine + PDFium) |
| A02 | ScriptEngine_P4D |
| A03 | Python_Infra |
| A04 | Report_Engine (FortesReport — D23) |
| A05 | Cripto_Engine |
| A06 | Updater_Client |

### IXX — Inteligência Artificial

| Prefixo | Nome | Sub-módulos |
|---|---|---|
| I01 | IA_Core | SubModules/Classificador, OCR_Semantico (futuro), Chatbot (futuro) |

### ZXX — Verticais / Especializações (D24: vertical = Z?? + sub-letras a-z)

| Prefixo | Nome | Sub-letras |
|---|---|---|
| **Z01** | Escritorio_Contabil | a_Clientes_Contabeis, b_Certidoes, c_DocumentosFiscais, d_Assinatura_ICP, **e_Execucao_Servicos ⭐ D20**, f_Cobranca_Boletos, g_Tributos_Calculo_Fiscal, h_Obrigacoes_Acessorias, i_eSocial_RH |
| **Z02** | Venda_Varejo | a_POS_NFCe, b_Catalogo_Produtos_Varejo, c_Caixa_Frente |
| **Z03** | CRM_Atendimento_Clientes | a_Leads, b_Oportunidades, c_Pipeline, d_Campanhas, e_Atendimento_Cliente |
| **Z04** | Oficina | a_OS_Ordem_Servico, b_Veiculos_Frota_Oficina, c_Catalogo_Produtos_Servicos, d_Garantia, e_Orcamento |
| **Z05** | Escritorio_Imobiliario | a_Imoveis, b_Aluguel_Locacao, c_Contratos_Imobiliarios, d_Reajustes |
| **Z06** | Vendas_em_Campo | a_Roteiros, b_Frota_Vendas, c_Propostas, d_Comissoes, e_Agenda_Visitas |
| Z07+ | (reservado) | Jurídico, Logística, Saúde, etc. |

### CXX — Cross-cutting

| Prefixo | Nome |
|---|---|
| C01 | Logging |
| C02 | Cache |
| C03 | Notificacoes |
| C04 | Health |
| C05 | Geo (alternativa a M11 — P08) |
| C06 | Tenant_Context (D19: interceptor multi-tenant FAIL-CLOSED) |

### GXX — Gateways (comunicação saída)

| Prefixo | Nome |
|---|---|
| G01 | Email_SMTP |
| G02 | WhatsApp |
| G03 | Webhooks |
| G04 | SMS |

### TXX — Tooling

| Prefixo | Nome |
|---|---|
| T01 | Smoke_Runner |
| T02 | DUnitX_Suite |
| T03 | Migrations_Tool |
| T05 | Auto_Updater_Builder |
| T06 | Import_Tooling |

## Mapeamento legado M01-M33 → taxonomia M/S/B/A/I/Z/C/G/T

Pastas `RN-M01..RN-M26` actualmente em `RegrasNegocio/` (legado GestorERP-legacy) **permanecem inalteradas** como histórico activo. Novas RNs devem usar os prefixos da taxonomia V3.2.0.

| Legado | Nova taxonomia (destino preferido) |
|---|---|
| RN-M01 - Segurança e Acesso | **RN-S01 - Seguranca_Acesso** |
| RN-M02 - Cadastros Base | **RN-M11..M14** (split) |
| RN-M03 - Clientes | **RN-M13 - Clientes_Base** + extensões ZXX |
| RN-M04 - Empresas | **RN-Z01.a - Clientes_Contabeis** ou subset de M13 |
| RN-M05 - Financeiro | **RN-M16..M18, M22** (split em 4) |
| RN-M06 - Fiscal e NF-e | **RN-Z01.c + RN-Z02.a + RN-B03 + RN-Z04.a** (cross-vertical) |
| RN-M07..M10 (Documentos *) | a definir — possivelmente sub-módulos de IXX ou módulos novos |
| RN-M11 - Tarefas e Agenda | **RN-Z06.e - Agenda_Visitas** + parte CRM |
| RN-M12 - Atendimento Técnico | **RN-M20 - Chamados_Internos** |
| RN-M13 - Helpdesk e Chamados | **RN-M20 - Chamados_Internos** ou **RN-Z03.e - Atendimento_Cliente** |
| RN-M14 - Mensageria | **RN-M14 - Mensageria_Interna + RN-G01 - Email_SMTP + RN-G02 - WhatsApp** |
| RN-M15 - LGPD e Auditoria | **RN-S03 - Auditoria + cross-cutting** |
| RN-M16 - Estoque e Produtos | **RN-M19 - Estoque_Almoxarifado + RN-Z02.b + RN-Z04.c** |
| RN-M17 - Ordens de Serviço | **RN-Z04.a - OS_Ordem_Servico** |
| RN-M18 - Orçamentos | **RN-Z04.e - Orcamento + RN-Z06.c - Propostas** |
| RN-M19 - Veículos | **RN-M21 - Veiculos_Base + RN-Z04.b + RN-Z06.b** |
| RN-M20 - Vendas | **RN-Z06.c - Propostas + RN-Z02 - Venda_Varejo** |
| RN-M21 - Proposta | **RN-Z06.c - Propostas** |
| RN-M22 - Comissões | **RN-Z06.d - Comissoes** |
| RN-M23 - Execução de Serviços | **RN-Z01.e - Execucao_Servicos ⭐ D20** |
| RN-M24 - Frota | **RN-M21 - Veiculos_Base + RN-Z06.b - Frota_Vendas** |
| RN-M25 - Roteiros | **RN-Z06.a - Roteiros** |
| RN-M26 - Caixa | **RN-M16 - Caixa_Banco** |

(M27-M33 já movidos para `Documentation/Legacy/RegrasNegocio_Pre_D24/`.)

## Migração de RNs legadas (M01-M33 → nova taxonomia)

Quando migrar uma RN legada para a nova taxonomia:

1. **Não eliminar** a RN antiga em `RegrasNegocio/RN-Mxx - Nome/` — manter como histórico.
2. **Criar** a nova RN em `RegrasNegocio/RN-{Prefix}{nn}[.<letra>] - <Nome>/` com naming V3.2.0.
3. Na secção `## NOTAS / OBSERVAÇÕES` da nova RN, declarar origem:
   ```
   **Origem (legado):** RN-M01-001 (GestorERP_RN-M01-001_Overview_V2_0_0.md) — V2.0.0 do legado migrada para taxonomia V3.2.0 em 2026-MM-DD.
   ```
4. Atualizar matrizes (Rastreabilidade, Maturidade, Status) com nova entry + coluna "Origem legada".
5. **Para módulos herdados do GDoc** (S01, B01, B02, Z01.a/b/c/d, Z01.e/f): a secção `## FLUXO PRINCIPAL` deve referenciar URL preservada D25:
   ```
   **URL preservada (D25):** `/api/v1/security/login` (EN — P18=A; conviver permanente)
   **Fonte:** Documentation/Canonical/api-contracts/url-freeze_V1.0.md
   ```

## Cada RN = uma regra de negócio

Princípio fundamental da organização:

- Cada **ARQUIVO** descreve **UMA** regra de negócio específica e atómica.
- Cada **PASTA** (`RN-{Prefix}{nn}[.<letra>] - Nome/`) agrupa as regras de **UM** módulo (ou sub-módulo ZXX).
- **Gaps na numeração são propositais** — permitem inserir novas RNs sem renumerar as existentes (ex.: S01-001, S01-003, S01-007 sem 002, 004-006).
- Nunca agrupar múltiplas regras num único arquivo; nunca misturar regras de módulos diferentes na mesma pasta.

## Inputs

1. `<modulo>`: módulo ao qual as regras pertencem com prefixo nova taxonomia (ex.: `S01-Seguranca_Acesso`, `Z01.a-Clientes_Contabeis`).
2. `<regras>`: lista de regras ou descrição do comportamento esperado.
3. `<contexto>`: objetivo e termos do domínio.
4. `<analise_path>`: caminho de `Documentation/Analise/<Modulo>/` — ler antes de redigir.
5. `<inherits_from>` (se aplicável): nome do módulo GDoc original + URL preservada (D25).
6. `<inherits_legacy>` (se aplicável): ID de RN legada (M01-M33) sendo migrada.

## Exemplo de referência (gold standard)

A pasta `exemplos/` dentro desta skill contém ficheiros de referência no formato padrão completo V3.2.0:

- `Padrao_RN-S01-001_exemplo.md` — exemplo de RN do módulo Segurança (S01)
- `Padrao_RN-Z01.a-001_exemplo.md` — exemplo de RN de sub-módulo vertical (Z01.a Clientes Contábeis)
- `Padrao_RN-M01-001_exemplo.md` *(legado, formato antigo)* — preservado para comparação histórica
- `Padrao_RN-M05-001_exemplo.md` *(legado)* — idem

Estes ficheiros servem como **modelo de qualidade** para qualquer nova RN gerada. Ao criar RNs, consultar estes exemplos para garantir que todas as 12 secções obrigatórias estão preenchidas com o nível de detalhe esperado.

## Dependências (skills prévias)

| Skill | Quando executar antes |
| --- | --- |
| `documentation-class-analysis-generator` | Quando `Documentation/Analise/<Modulo>/` ainda não existir ou estiver com placeholders — as RNs são derivadas dos invariantes documentados ali. |
| `documentation-general_rules` | Sempre — verificar convenções de nomenclatura, versionamento e idioma antes de criar arquivos. |
| `documentation-project-bootstrap` | Quando a pasta `Documentation/RegrasNegocio/` ainda não existir no projeto. |
| `gestorerp-namespace-taxonomy` (workspace) | Para decidir o prefixo M/S/B/A/I/Z/C/G/T do módulo antes de criar a primeira RN. |

## Fonte primária: Documentation/Analise/

Antes de redigir qualquer RN, **ler** os documentos de análise do módulo em `Documentation/Analise/<Modulo>/`. Cada `{ClassName}.md` contém invariantes, contratos e restrições.

**Workflow:**
1. Listar `Documentation/Analise/<Modulo>/*.md`
2. Para cada arquivo, extrair: pré-condições, pós-condições, invariantes de estado, restrições de uso
3. Cada invariante/contrato coeso = uma RN separada (um arquivo)
4. Criar arquivo `{Projeto}_RN-{Prefix}{nn}[.<letra>]-{nnn}_{Prefix}{nn}[.<letra>]_V1_0.md` para cada RN
5. Criar/atualizar `RN-{Prefix}{nn}[.<letra>] - Nome/README.md` com link para a nova RN
6. Atualizar hub `Documentation/RegrasNegocio/README.md`

## Estrutura interna de cada arquivo RN — Formato padrão (MANDATÓRIO)

Usar `templates/TEMPLATE_Docs_RN.md` (dentro desta skill) como base. **Todas as seções abaixo são MANDATÓRIAS** — nenhuma pode ser omitida. Referência local: `exemplos/` (dentro desta skill).

Cada arquivo RN **deve** seguir esta estrutura exata:

~~~markdown
{Projeto} · RN-{Prefix}{nn}[.<letra>]-{nnn} — Título descritivo | V1.0
=====================================================================

{Projeto} · Regra de Negócio

**ID da Regra**: RN-{Prefix}{nn}[.<letra>]-{nnn}
**Módulo**: {Prefix}{nn}[.<letra>] — Nome do Módulo
**Taxonomia**: <Core | Segurança | Base | Auxiliar | IA | Vertical | Cross-cutting | Gateway | Tooling>
**Origem (legado)**: RN-Mxx-NNN (se migrada) | "novo" (se sem precedente)
**Origem (GDoc)**: M0X-Nome (se herdada do GDoc) | "n/a"
**URL preservada (D25)**: /api/v1/... (se aplicável) | "rotas novas — naming livre" | "n/a"
**Fase**: Fase {n} ({Descrição})
**Prioridade**: Alta / Média / Baixa
**Status**: Proposto / Em detalhamento / Aprovado / Implementado / Testado
**Título**: Título descritivo da regra
**Ref. Arquitetura**: {Documento} · Cap. {n} §{seção}
**Multi-tenant (D19)**: aplicável | n/a

## PRÉ-CONDIÇÕES — O que deve ser verdadeiro antes desta regra ser aplicada

1. [Condição 1]
2. [Condição 2]

## FLUXO PRINCIPAL — Sequência feliz (passo a passo quando tudo funciona)

1. [Passo 1 — descrição detalhada]
2. [Passo 2 — dados, chamadas, transformações]

## FLUXOS DE EXCEÇÃO — O que acontece quando algo dá errado

- **E1. [Título]**
  - `HTTP {código} { "error": "{erro}" }`
  - [Ação do sistema]

- **E2. [Título]**
  - [Descrição]

## VALIDAÇÕES

| Campo / Dado | Condição / Regra | Mensagem de Erro | HTTP |
|---|---|---|---|
| [campo] | [condição] | [mensagem] | [código] |

## TABELAS / CAMPOS DO BANCO DE DADOS

| Tabela | Op. | Campos Relevantes | tenant_id (D19) |
|---|---|---|---|
| `{schema}.{tabela}` | R/W | `campo1`, `campo2` | obrigatório / n/a |

## IMPACTO EM OUTRAS RNs

- **RN-{Prefix}{nn}-{yyy}** — [descrição do impacto]

## LGPD — Dados pessoais envolvidos, base legal e prazo de retenção

- **Dados tratados**: [lista]
- **Base legal**: [artigo LGPD]
- **Retenção**: [prazo e política]

## ESBOÇO DE IMPLEMENTAÇÃO — {Stack}

```pascal
// Código demonstrativo do fluxo principal
```

## NOTAS / OBSERVAÇÕES

- [Decisões de design, justificativas]
- **Origem legado:** (se aplicável) referência à RN-Mxx-NNN substituída

## Assinaturas

- **Elaborado por**: Equipe {Projeto} — ___/___/______
- **Revisado por**: ___________________ — ___/___/______
- **Aprovado por**: ___________________ — ___/___/______
~~~

**IMPORTANTE — seções que NÃO fazem parte do formato padrão (não usar):**
- ~~`## Descrição`~~ → substituída pelo cabeçalho de identificação
- ~~`## Regras` / subrules~~ → substituída por FLUXO PRINCIPAL + VALIDAÇÕES
- ~~`## Critérios de aceite`~~ → substituída por VALIDAÇÕES
- ~~`## Rastreabilidade`~~ → substituída pelos campos de cabeçalho (Origem legado, Origem GDoc, URL preservada)
- ~~`## Changelog`~~ → substituída por NOTAS / OBSERVAÇÕES

## Campos de cabeçalho — extensões V3.2.0

Em relação à V3.1.0, o cabeçalho ganhou 5 campos opcionais para acomodar a nova taxonomia + D25:

| Campo (V3.2.0) | Quando preencher | Valores |
|---|---|---|
| **Taxonomia** | sempre | Core / Segurança / Base / Auxiliar / IA / Vertical / Cross-cutting / Gateway / Tooling |
| **Origem (legado)** | se migrada de M01-M33 | `RN-M01-001` ou `"novo"` |
| **Origem (GDoc)** | se módulo herdado do GDoc | `M01-Autenticacao`, `M02-Clientes`, etc. ou `"n/a"` |
| **URL preservada (D25)** | se módulo herdado do GDoc | `/api/v1/security/login` ou `"rotas novas — naming livre"` ou `"n/a"` |
| **Multi-tenant (D19)** | sempre | `aplicável` (entity tem `tenant_id`) ou `n/a` (config/meta) |

## README.md por pasta de módulo

Cada `RN-{Prefix}{nn}[.<letra>] - Nome/README.md` deve conter:

```markdown
# RN-{Prefix}{nn}[.<letra>] — Nome do Módulo

> Breve descrição do escopo do módulo.
> **Taxonomia:** <Core | Segurança | Base | ...>
> **Origem (legado):** RN-Mxx-NNN (se aplicável)
> **Origem (GDoc):** Mxx-Nome (se aplicável)

## Regras de Negócio

| RN | Título | Status |
|---|---|---|
| [RN-{Prefix}{nn}-001]({Projeto}_RN-{Prefix}{nn}-001_{Prefix}{nn}_V1_0.md) | Título | [P] |
| [RN-{Prefix}{nn}-002]({Projeto}_RN-{Prefix}{nn}-002_{Prefix}{nn}_V1_0.md) | Título | [ ] |

## Sub-módulos (só ZXX/IXX)

(quando aplicável: tabela apontando para subpastas SubModules/)

## Migrações de RNs legadas

| RN legada | RN nova (esta pasta) | Data |
|---|---|---|
| RN-M01-001 | RN-S01-001 | 2026-MM-DD |
```

## Skills de domínio a consultar

- `gestorerp-namespace-taxonomy` (workspace) — decidir prefixo M/S/B/A/I/Z/C/G/T do módulo
- `documentation-project-expert` — hierarquia ORM, engines, diretivas
- `developer-delphi-programming-conditional-defines` — diretivas USE_* e blocos condicionais

## Matriz de cobertura (estado actual GestorERP 2026-05-18)

| Status | Prefixo | Módulos | RNs existentes |
|---|---|---|---|
| Legado (M01-M26) | M01-M26 | 26 pastas em `RegrasNegocio/RN-Mxx - Nome/` | ~102 RNs no formato V2.0 (RN antigo) |
| Legado arquivado | M27-M33 | em `Documentation/Legacy/RegrasNegocio_Pre_D24/` | 23 RNs (M27 Bancos, M28 Boletos, ..., M33 Aluguéis) |
| Nova taxonomia | M/S/B/A/I/Z/C/G/T | 0 pastas criadas | 0 RNs (FASE 1 do plano master cria-as) |

> **Migração nova taxonomia:** começa na FASE 1 da migração GDoc→GestorERP. SPECs prioritárias: S01, C06, A05 (Tier 0). Ver `.workspace/plans/gestorerp-master-migration-plan_v1.0.plan.md`.

## Critérios de aceite da skill

- Cada RN é um arquivo separado com nomenclatura `{Projeto}_RN-{Prefix}{nn}[.<letra>]-{nnn}_{Prefix}{nn}[.<letra>]_V{X}_{Y}.md`.
- Cada arquivo segue o **formato padrão MANDATÓRIO** com todas as seções: Cabeçalho de identificação (incluindo Taxonomia, Origem legado, Origem GDoc, URL preservada, Multi-tenant), PRÉ-CONDIÇÕES, FLUXO PRINCIPAL, FLUXOS DE EXCEÇÃO, VALIDAÇÕES, TABELAS/CAMPOS BD, IMPACTO EM OUTRAS RNs, LGPD, ESBOÇO DE IMPLEMENTAÇÃO, NOTAS/OBSERVAÇÕES, Assinaturas.
- Nenhum arquivo pode usar o formato antigo (Descrição, Regras/subrules, Critérios de aceite, Rastreabilidade, Changelog).
- Cada pasta de módulo tem `README.md` com tabela de links para as RNs.
- Hub `Documentation/RegrasNegocio/README.md` lista todos os módulos (legados + novos).
- Para módulos herdados do GDoc, o cabeçalho declara URL preservada D25.
- Para entities BD, tabelas declaram `tenant_id (D19)` na coluna apropriada.
- Não há duplicação com `Documentation/Arquitetura/` (arquitetura = fluxos; RN = invariantes).
- Naming/versioning conforme skill `documentation-general_rules` (naming conventions).

## Anti-padrões

| Anti-padrão | Por que é errado | Como corrigir |
| --- | --- | --- |
| Agrupar múltiplas regras em um único arquivo | Viola o princípio fundamental (uma RN = um arquivo); impede rastreabilidade individual e aprovação atómica | Separar cada invariante/contrato num arquivo `{Projeto}_RN-{Prefix}{nn}[.<letra>]-{nnn}_{Prefix}{nn}[.<letra>]_V1_0.md` distinto |
| Usar o formato antigo (seções Descrição, Regras, Critérios de aceite, Changelog) | Formato incompatível com o formato padrão; impede comparação e migração automática | Reescrever usando as 12 secções obrigatórias do formato padrão com `templates/TEMPLATE_Docs_RN.md` |
| Criar RNs sem ler `Documentation/Analise/<Modulo>/` antes | RNs baseadas em suposições, não em invariantes reais do código; geram inconsistências | Seguir o workflow: listar e ler todos os `{ClassName}.md` do módulo antes de redigir |
| Misturar regras de módulos diferentes na mesma pasta | Quebra a organização por módulo; dificulta manutenção e busca | Criar subpasta `RN-{Prefix}{nn}[.<letra>] - Nome/` por módulo e mover/criar arquivos no local correto |
| Renomear pastas legadas RN-M01..M26 in-place | Quebra 200+ referências internas em RNs, matrizes, BRIEFINGs, READMEs | Preservar pastas legadas; criar pastas novas RN-S01/RN-Z01.a/etc. ao lado |
| Esquecer de declarar URL preservada D25 em RNs herdadas do GDoc | Quebra contrato com Careli cliente em produção; pode gerar regressão silenciosa | Adicionar campo `**URL preservada (D25)**` no cabeçalho com referência a `Canonical/api-contracts/url-freeze_V1.0.md` |
| Esquecer de declarar `tenant_id` em tabelas (D19) | Multi-tenant FAIL-CLOSED bloqueia queries; bug em runtime | Adicionar coluna "tenant_id (D19)" na tabela de TABELAS/CAMPOS BD |

## Métricas de sucesso

- Cada RN gerada possui arquivo próprio com nomenclatura V3.2.0 e todas as 12 secções obrigatórias preenchidas (zero secções ausentes ou com placeholder vazio).
- Hub `Documentation/RegrasNegocio/README.md` atualizado com link para cada nova RN (zero links quebrados após execução).
- Cada pasta de módulo tem `README.md` com tabela de links para todas as RNs do módulo (cobertura de índice = 100%).
- Para módulos herdados do GDoc, 100% das RNs declaram URL preservada D25.
- Para entities com persistência BD, 100% das tabelas declaram coluna `tenant_id (D19)`.

## Responsável principal

| Papel | Quem |
| --- | --- |
| Agent executor | `documentation-agent-orchestrator` |
| Revisão humana | Analista de negócio ou tech lead do módulo |
| Aprovação final | Product owner / dono do requisito |

---

**Changelog (este arquivo):**

- **3.2.0 (18/05/2026):** Taxonomia ampliada para 9 prefixos M/S/B/A/I/Z/C/G/T (decisões D01-D25 do GestorERP). Naming adaptado para sub-letras ZXX (D24): `RN-Z01.a-001_Z01.a_V1_0.md`. Cabeçalho ganha 5 campos novos: Taxonomia, Origem legado, Origem GDoc, URL preservada (D25), Multi-tenant (D19). Tabela TABELAS/CAMPOS BD ganha coluna `tenant_id (D19)`. Protocolo de migração de RNs legadas M01-M33 documentado. Exemplos novos: `Padrao_RN-S01-001_exemplo.md` (taxonomia nova) e `Padrao_RN-Z01.a-001_exemplo.md` (sub-letras D24). Exemplos legados M01/M05 preservados para comparação. Mapeamento M01-M33 → nova taxonomia adicionado.
- 3.1.0 (09/04/2026): Migração V2 — adicionadas seções Responsabilidade única, When NOT to use, Dependências (skills prévias), Anti-padrões, Métricas de sucesso, Responsável principal; frontmatter expandido com thinking e category.
- 3.0.0 (04/04/2026): Mapeamento genérico de módulos; exemplos ProvidersORM (9) e GestorERP (26); secção "Cada RN = uma regra de negócio"; referência a `exemplos/`.
- 2.2.0 (04/04/2026): Template inline completo no formato padrão.
- 2.1.0 (04/04/2026): Formato padrão com 12 seções MANDATÓRIAS.
- 2.0.0 (04/04/2026): Reescrita completa para padrão GestorERP (um arquivo por RN, subpastas).
- 1.0.2 (03/04/2026): Fonte primária Analise/, mapeamento módulo→prefixo, matriz de cobertura.
- 1.0.1 (30/03/2026): Rubrica de versionamento interno.
- 1.0.0 (30/03/2026): Versionamento interno inicial.

---

## Versão interna (ficheiro)

| Campo | Valor |
|-------|-------|
| **FileVersion** | 3.2.0 |
| **Política** | `.cursor/VERSION.md` |
| **Data** | 2026-05-18 |
