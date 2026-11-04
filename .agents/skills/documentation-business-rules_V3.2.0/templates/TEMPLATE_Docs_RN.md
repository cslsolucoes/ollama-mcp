{Projeto} · RN-{Prefix}{nn}[.<letra>]-{nnn} — Título descritivo da regra | V1.0
=====================================================================

{Projeto} · Regra de Negócio

**ID da Regra**: RN-{Prefix}{nn}[.<letra>]-{nnn}
**Módulo**: {Prefix}{nn}[.<letra>] — Nome do Módulo
**Taxonomia**: <Core | Segurança | Base | Auxiliar | IA | Vertical | Cross-cutting | Gateway | Tooling>
**Origem (legado)**: RN-Mxx-NNN (se migrada de M01-M33) | "novo"
**Origem (GDoc)**: M0X-NomeModulo (se herdada do GDoc) | "n/a"
**URL preservada (D25)**: /api/v1/... (se herdada do GDoc) | "rotas novas — naming livre" | "n/a"
**Fase**: Fase {n} ({Descrição da fase})
**Prioridade**: Alta / Média / Baixa
**Status**: Proposto / Em detalhamento / Aprovado / Implementado / Testado
**Título**: Título descritivo da regra
**Ref. Arquitetura**: {Documento de Arquitetura} · Cap. {n} §{seção}
**Multi-tenant (D19)**: aplicável (entity persiste BD) | n/a (config/meta/sem persistência)

## PRÉ-CONDIÇÕES — O que deve ser verdadeiro antes desta regra ser aplicada

1. [Condição que deve ser verdadeira antes da execução desta regra]
2. [Configuração necessária em config/banco/ambiente]
3. [Serviços ou endpoints que devem estar acessíveis]

## FLUXO PRINCIPAL — Sequência feliz (passo a passo quando tudo funciona)

1. [Passo 1 — descrição detalhada da ação, quem faz, resultado]
2. [Passo 2 — dados processados, chamadas, transformações]
3. [Passo 3 — persistência, retorno, evento disparado]

> **Para módulos herdados do GDoc:** declarar literais de URL preservada conforme `Documentation/Canonical/api-contracts/url-freeze_V1.0.md` (D25). Não alterar URL/método/JSON shape/status code/headers sem sprint coordenado de cutover.

## FLUXOS DE EXCEÇÃO — O que acontece quando algo dá errado

- **E1. [Título da exceção]**
  - `HTTP {código} { "error": "{erro_code}" }`
  - [Ação do sistema / mensagem ao usuário]

- **E2. [Título da exceção]**
  - [Descrição da condição de erro e resposta]

- **E3. [Título da exceção]**
  - [Descrição da condição de erro e resposta]

## VALIDAÇÕES

| Campo / Dado | Condição / Regra | Mensagem de Erro | HTTP |
|---|---|---|---|
| [nome do campo] | [condição que deve ser verdadeira] | [mensagem exibida ao usuário] | [código HTTP] |
| [nome do campo] | [condição] | [mensagem] | [código] |

## TABELAS / CAMPOS DO BANCO DE DADOS

| Tabela | Op. | Campos Relevantes | tenant_id (D19) |
|---|---|---|---|
| `{schema}.{tabela}` | R/W | `campo1`, `campo2`, `campo3` | obrigatório (filtrado por C06-Tenant_Context FAIL-CLOSED) |
| `{schema}.{tabela}` | W | `campo1`, `campo2` | obrigatório |

> **D19 (multi-tenant):** todas as tabelas têm `tenant_id BIGINT NOT NULL DEFAULT 0`. Interceptor C06-Tenant_Context força filtro automático; queries sem tenant em sessão são bloqueadas.

## IMPACTO EM OUTRAS RNs

- **RN-{Prefix}{nn}-{yyy}** — [Descrição do impacto e dependência]
- **RN-{Prefix2}{nn}-{www}** — [Descrição do impacto e dependência]

## LGPD — Dados pessoais envolvidos, base legal e prazo de retenção

- **Dados tratados**: [lista de dados pessoais envolvidos]
- **Base legal**: [artigo e inciso da LGPD aplicável]
- **Retenção**: [prazo de retenção e política de expurgo]

## ESBOÇO DE IMPLEMENTAÇÃO — {Stack tecnológica}

```pascal
// {Unit} — {Descrição}
procedure T{Classe}.{Metodo};
begin
  // Esboço de implementação demonstrando o fluxo principal
  // Para módulos GDoc-herdados: preservar literais de URL/JSON shape conforme url-freeze
  // Para queries: usar IConnection do ProvidersORM via C06-Tenant_Context (não filtrar tenant manualmente)
end;
```

## NOTAS / OBSERVAÇÕES

- [Decisões de design, justificativas, observações relevantes]
- [Parâmetros configuráveis e seus valores padrão]
- **Origem legado:** (se aplicável) referência à RN-Mxx-NNN substituída — ex.: `Esta RN substitui RN-M01-001 V2.0.0 (legado em RN-M01 - Segurança e Acesso/) na taxonomia V3.2.0 (D24).`
- **Histórico de versões deste arquivo:** registar mudanças relevantes desta RN aqui (em vez de seção `## Changelog` separada — proibida pelo formato padrão).

## Assinaturas

- **Elaborado por**: Equipe {Projeto} — ___/___/______
- **Revisado por**: ___________________ — ___/___/______
- **Aprovado por**: ___________________ — ___/___/______
