---
name: "source-command-gestorerp-stack-relink"
description: "Recria symlinks `stack/` em cada pasta RN do GestorERP apontando para `Documentation/Stack/`. Idempotente — só altera onde `stack/` é pasta real (resultado de cp -r) ou onde está ausente. Requer privilégios admin no Windows."
---

# source-command-gestorerp-stack-relink

Use this skill when the user asks to run the migrated source command `gestorerp-stack-relink`.

## Command Template

# /gestorerp-stack-relink

Aciona o script `.workspace/scripts/gestorerp-rebuild-stack-symlinks.ps1`.

## Quando usar

- Após copiar uma pasta RN (`cp -r RN-XXX/` ou similar) — o symlink `stack/` interno foi resolvido em pasta real
- Após clonar o repositório em máquina sem `core.symlinks=true` configurado
- Após restaurar de backup que não preservou symlinks
- Validação periódica do estado dos links

## Lógica idempotente

Para cada RN listado em `.workspace/config/gestorerp-stack-symlinks.json`:

| Estado actual de `stack/` | Acção |
|---|---|
| Symbolic link válido | Nenhuma (idempotente) |
| Pasta real (não-link) | **Apaga + recria como symlink** |
| Não existe | Cria symlink |
| RN não existe | Pula (warning) |

## Argumentos

- `-ValidateOnly` — apenas reporta estado, sem alterações
- `-Verbose` — log detalhado por entry

## Pré-condições

- Privilégios admin no Windows (ou Developer Mode activo)
- `Documentation/Stack/` existe na raiz do repo
- Manifest `.workspace/config/gestorerp-stack-symlinks.json` existe e válido

## Execução

```powershell
powershell -ExecutionPolicy Bypass -File .workspace/scripts/gestorerp-rebuild-stack-symlinks.ps1
```

Ou com validação prévia:

```powershell
powershell -ExecutionPolicy Bypass -File .workspace/scripts/gestorerp-rebuild-stack-symlinks.ps1 -ValidateOnly
```

## Integração VS Code

Via Tasks Runner (Ctrl+Shift+P → "Run Task" → "GestorERP: Rebuild Stack Symlinks"):
- Definição em `.vscode/tasks.json`
- Não auto-roda — sempre invocado manualmente

## Exit codes

| Code | Significado |
|---|---|
| 0 | Sucesso (todos symlinks OK ou recriados) |
| 1 | Erro (manifest ausente, Stack/ ausente, admin denied, ou alguma operação falhou) |

## Output esperado

```
[OK]   00-Bibliotecas_Base/RN-A05 - Cripto_Engine/stack (symlink)
[OK]   02-Acesso_Seguranca/RN-S01 - Seguranca_Acesso/stack (symlink)
[RECREATE] 08-Vertical_Contabil/RN-Z01.d - Assinatura_ICP/stack (era pasta real, recriando link)
[CREATE] 13-Vertical_CRM/RN-Z03.a - Leads/stack
...

=== Sumário ===
  OK (sem alteração):     65
  Recriados (pasta→link): 5
  Criados (novos):        4
  Pulados (RN ausente):   0
  Erros:                  0
```

## Cross-references

- Script: `.workspace/scripts/gestorerp-rebuild-stack-symlinks.ps1`
- Manifest: `.workspace/config/gestorerp-stack-symlinks.json`
- Skill canónica: `gestorerp-stack-prerrogativas_V1.3.1`
- Plano: `.workspace/plans/gestorerp-doc-reorganization_v1.0.plan.md`
