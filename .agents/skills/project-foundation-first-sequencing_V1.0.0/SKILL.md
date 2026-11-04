---
name: project-foundation-first-sequencing
description: Algoritmo de decisão para escolher a próxima tarefa/sprint/passo em qualquer projeto técnico baseado em dependências satisfeitas + criticidade downstream + risco de fundação fraca. Princípio rector — "não subir paredes sem a base, pois cairá". Quando o owner pergunta "qual é o próximo?" / "por onde começar?" / "qual a sequência?", esta skill aplica algoritmo de 4 passos (inventário tarefas → mapear pré-requisitos → filtrar elegíveis cujas dependências estão 100% concluídas → ranquear por impacto downstream). Bloqueia tentativas de saltar sprints com upstream incompleto. Codifica metáfora arquitetural (fundação → estrutura → cobertura) aplicada a backlog técnico — útil em migrações por sprint, scaffold de módulos, refatorações cross-cutting, releases sequenciais.
model: sonnet
category: project
scope: any-project
version: 1.0.0
---

# project-foundation-first-sequencing — V1.0.0

> **Skill genérica de método de trabalho.** Aplicável a qualquer projeto técnico que opere por sprints/fases/tarefas com dependências.
> Cristalizada em 2026-05-19 a partir de diretiva explícita do owner (Claiton Linhares — GestorERP V1.0).

## Princípio rector

> **"Não tem como subir paredes sem a base feita — porque cairá."**

Em construção civil, paredes erguidas sobre fundação incompleta caem. Em software, sprints executados antes dos seus pré-requisitos quebram em produção (cascata de bugs, refatorações forçadas, retrabalho).

**Regra dura:** nunca iniciar uma tarefa cujas dependências não estejam **100% concluídas e validadas**. Nem "quase concluídas". Zero atalho.

---

## Quando aplicar esta skill

Sempre que o owner, decision-maker ou agente perguntar:

- "Qual é o próximo passo?"
- "Por onde começar?"
- "Qual a sequência lógica?"
- "Qual o sprint X ou Y primeiro?"
- "Estamos prontos para [tarefa Z]?"

…ou quando se observa uma tendência a saltar para tarefa "mais visível/empolgante" antes das fundações estarem prontas.

---

## Algoritmo de decisão (4 passos)

### Passo 1 — Inventariar tarefas/sprints/passos pendentes

Listar **todas** as tarefas backlog (não só as "em foco"). Fontes:

- Plan persistente (`.workspace/plans/*-execution.plan.md`)
- Roadmap canónico (`Documentation/*roadmap*.md`)
- Issue tracker (Jira/GitHub Issues)
- Lista mental do owner (perigoso — preferir escrita)

**Output:** lista N de candidatos. Se N = 1, salta para Passo 4.

### Passo 2 — Mapear pré-requisitos de cada tarefa

Para cada candidato, identificar **explicitamente** os pré-requisitos (upstream dependencies):

| Tarefa | Pré-requisitos | Outputs que produz | Bloqueia downstream |
|---|---|---|---|
| Sprint A | (FASE 0, S-X) | artefacto `foo.md` | Sprints B, C, D |
| Sprint B | A | scaffold `Bar/` | Sprint D, E |
| Sprint C | A | endpoints `/api/v1/baz` | Sprint F |

Pré-requisitos podem ser:
- **Fases/tarefas anteriores** (mais comum)
- **Artefactos canónicos** (documentos, schemas, contratos)
- **Decisões de produto/arquitetura** (ADRs pendentes)
- **Acessos/recursos externos** (cert digital, API key, ambiente)
- **Aprovações humanas** (owner, security, legal)
- **Build/deploy infra** (CI, ambiente staging)

### Passo 3 — Filtrar elegíveis (dependências 100% concluídas)

Para cada candidato, verificar:

```
elegível ← TRUE
para cada pré-req em tarefa.pré-requisitos:
  se pré-req.status != "✅ CONCLUÍDO E VALIDADO":
    elegível ← FALSE
    break
```

**Validação estrita:**
- ✅ "Concluído" = artefacto existe, foi aprovado, gates passados
- ❌ "Quase pronto" = não elegível
- ❌ "Pode-se começar em paralelo" = não elegível (a menos que pré-req seja `parallel-ok` explicitamente)

**Output:** subconjunto de tarefas elegíveis (com dependências satisfeitas).

### Passo 4 — Ranquear elegíveis e escolher

**Se 0 elegíveis:** sistema em deadlock — investigar bloqueio mais alto na cadeia.

**Se 1 elegível:** essa é a próxima. Decisão trivial.

**Se ≥2 elegíveis:** ranquear por critérios em ordem:

1. **Caminho crítico** — tarefa que destrava maior número de downstream (alto impacto)
2. **Risco de obsolescência** — tarefa que se adiada exige retrabalho (legislação, contrato externo expirando)
3. **Custo** — tarefa mais barata primeiro (entrega valor rápido + libera dependentes)
4. **Risco de execução** — tarefa mais arriscada primeiro (descobrir problemas cedo)
5. **Compromisso temporal** — sprint com prazo externo definido

**Output:** **1 tarefa** marcada como "próxima a executar".

---

## Anti-padrões (NUNCA fazer)

### ❌ Saltar para tarefa "mais empolgante"

```
[FASE 0 e FASE 1 incompletas]
"Vamos começar pelo módulo de IA porque é mais interessante!"
→ Sem fundação (taxonomia, RNs base), o módulo IA herda decisões erradas
  que terão de ser desfeitas → retrabalho garantido
```

### ❌ "Vai dar tempo" / "podemos paralelizar"

```
Sprint S0b depende de S0.
"Vou começar S0b já porque parece simples — alinho com S0 depois"
→ S0 muda nomes de pastas/classes. S0b refactor inteiro perdido.
```

### ❌ "Mocks resolvem"

```
Sprint depende de B02-Serpro (real).
"Vou mockar Serpro e seguir"
→ Mock cobre 30% dos casos. Em integração real, ressurgem bugs estruturais
  que mudariam arquitetura inteira → arquitetura tem de ser refeita.
```

### ❌ Ignorar gates

```
Sprint marcado [X] sem cobertura ≥70% (G2 falhou)
"O importante é entregar — gate é detalhe"
→ Próximo sprint herda bugs invisíveis. Gates existem para validar
  fundação antes de subir paredes.
```

---

## Exemplos do GestorERP V1.0 (2026-05-19)

### Caso 1 — "Qual sprint da FASE 5?"

**Inventário (Passo 1):** 61 sprints em 17 blocos (`Documentation/migration-roadmap_V1.0.md`).

**Mapeamento pré-requisitos (Passo 2):**

| Sprint | Pré-requisitos | Estado dos pré-req |
|---|---|---|
| S-1 URL-Freeze | FASE 0 | ✅ FASE 0 ✅ |
| S0 rename M01→S01 | FASE 0 + **S-1** | ⚠️ S-1 pendente |
| S0b C06-Tenant_Context | S0 | ⚠️ S0 pendente |
| S1a A01-PDF_Engine | S0 | ⚠️ S0 pendente |
| S4 S02-Certificado | S-1 + S0 + S3b | ❌ múltiplos |

**Filtro elegíveis (Passo 3):** apenas **S-1** tem dependências 100% concluídas.

**Ranquear (Passo 4):** N=1 → S-1 é a resposta direta.

**Decisão:** **Sprint S-1 (URL-Freeze do GDoc)**. Destrava 9 sprints downstream + é pré-condição absoluta D25.

### Caso 2 — Múltiplos sprints elegíveis (hipotético)

Após Sprint S0 concluído, todos os Auxiliares (S1a, S2, S3b, S3c) ficam elegíveis simultaneamente porque dependem só de S0.

**Ranquear (Passo 4):**

| Sprint | Downstream que destrava | Custo | Risco | Prazo externo |
|---|---:|---:|---|---|
| S1a A01-PDF | S1b A04, S11.3 Z01.c, S11.5 Z01.b | 1-2d | médio | — |
| S2 A03-Python | S3 A02, S7 I01, S8 I01.a | 1d | alto (~2.5GB venv) | — |
| S3b A05-Cripto | S3c A06, S4 S02, S2.c TEF | 1d | baixo | — |
| S3c A06-Updater | T05 builder | 1d | baixo | — |

**Critério 1 (caminho crítico):** S1a destrava 3 sprints da cadeia Z01 (PRIORITÁRIO contábil) — escolher.

**Decisão:** S1a primeiro, depois S3b (libera S4), depois S2/S3 em paralelo, depois S3c.

---

## Decision tree visual

```
                 ┌──────────────────────────┐
                 │  "Qual o próximo passo?" │
                 └─────────────┬────────────┘
                               │
                               ▼
               ┌───────────────────────────────┐
               │ Inventariar TODAS as tarefas  │
               │ pendentes (backlog completo)  │
               └───────────────┬───────────────┘
                               │
                               ▼
               ┌───────────────────────────────┐
               │ Mapear pré-requisitos de cada │
               └───────────────┬───────────────┘
                               │
                               ▼
               ┌───────────────────────────────┐
               │ Filtrar: dependências 100% ✅ │
               └───────────────┬───────────────┘
                               │
                  ┌────────────┴────────────┐
                  │                         │
                  ▼                         ▼
        ┌─────────────────┐       ┌──────────────────┐
        │  0 elegíveis    │       │  ≥1 elegíveis    │
        │  → DEADLOCK     │       └─────────┬────────┘
        │  Investigar     │                 │
        │  bloqueio mais  │                 ▼
        │  alto na cadeia │       ┌──────────────────┐
        └─────────────────┘       │ Ranquear por:    │
                                  │ 1. Caminho crít. │
                                  │ 2. Obsolescência │
                                  │ 3. Custo         │
                                  │ 4. Risco execução│
                                  │ 5. Prazo externo │
                                  └─────────┬────────┘
                                            │
                                            ▼
                                 ┌──────────────────┐
                                 │ Executar 1 tarefa│
                                 │ (a vencedora)    │
                                 └─────────┬────────┘
                                           │
                                           ▼
                                 ┌──────────────────┐
                                 │ Validar gates,   │
                                 │ marcar concluída,│
                                 │ destravar        │
                                 │ downstream       │
                                 └─────────┬────────┘
                                           │
                                           └─→ (volta ao topo)
```

---

## Multi-critério (quando há ≥2 elegíveis)

Tabela canónica de critérios em ordem:

| Prioridade | Critério | Pergunta-chave | Peso típico |
|---:|---|---|---:|
| 1 | **Caminho crítico** | Esta tarefa destrava quantos downstream? | 40% |
| 2 | **Risco obsolescência** | Adiar provoca retrabalho ou perda de janela? | 20% |
| 3 | **Custo entrega** | Esforço × valor — quão rápido entrega? | 15% |
| 4 | **Risco execução** | Descobrir problema cedo evita desperdiçar follow-ups? | 15% |
| 5 | **Prazo externo** | Há SLA/compromisso temporal já firmado? | 10% |

Score = Σ (peso × critério_normalizado_0a10). Maior score = próxima.

**Tie-breaker** (empate): escolher tarefa com **menos pré-requisitos transitivos** (mais "raiz" na árvore de dependências).

---

## Como integrar com outras skills

| Skill companion | Como integra |
|---|---|
| `gestorerp-modules-review-checklist` (gate G1-G6) | Gates validam "dependências satisfeitas" antes de marcar concluído |
| `governance-artifact-dependency-map` | Constrói o grafo de dependências entre artefactos |
| `project-master-orchestrator` | Orquestra a execução sequencial validando esta skill em cada decisão |
| Skills `mandatory: true` (FASE 4 GestorERP) | Bloqueiam saltar gates — coerente com princípio "fundação primeiro" |

---

## Quando NÃO aplicar (excepções)

Esta skill é mandatória, mas há 3 cenários onde flexibilidade é aceitável:

1. **Spike/PoC isolado** — investigação técnica de 1-4h para reduzir incerteza sobre uma decisão. NÃO consome plan oficial; só serve para informar decisão futura.
2. **Bug-fix crítico produção (P0)** — quebra de SLA cliente exige intervenção imediata mesmo violando ordem. Documentar como divida técnica.
3. **Trabalho paralelo explicitamente independente** — tarefas que comprovadamente não dependem umas das outras podem rodar simultâneo (regra `parallel-ok`). Exemplo: documentação narrativa em paralelo a código (não se afectam).

**Em todos os 3 casos:** documentar a excepção. Não vira hábito.

---

## Sinal de alerta — quando esta skill deve disparar

- Owner ou colaborador pergunta "qual o próximo?" → aplicar imediatamente
- Plan persistente mostra tarefa marcada `[/]` (em progresso) sem que pré-req `[X]` esteja completo → bloquear
- PR/commit toca módulo cujo upstream tem gate vermelho → bloquear merge
- Roadmap mostra cadeias críticas paralelas → orientar 1 dev por cadeia (não pular)

---

## Referências

- `Documentation/migration-roadmap_V1.0.md` §1 Cadeias críticas (exemplo aplicado GestorERP)
- `.workspace/plans/gestorerp-migration-execution_v1.0.plan.md` (plan persistente vivo)
- `governance-artifact-dependency-map_V1.0.0` (skill complementar para grafo)
- Engineering folklore: "Conway's law" + "Mythical Man-Month" (Brooks) — fundamento histórico

---

## Changelog

- **V1.0.0 (2026-05-19):** Criação inicial. Cristalizada a partir de diretiva explícita do owner GestorERP (Claiton Linhares) — "não tem como subir paredes sem a base feita, pois cairá". Algoritmo 4-passos + 5 critérios de ranqueamento + 4 anti-padrões + 3 excepções documentadas + 2 exemplos concretos GestorERP FASE 5.
