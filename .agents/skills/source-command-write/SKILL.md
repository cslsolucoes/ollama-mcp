---
name: "source-command-write"
description: "Inicia sessão de escrita de código Delphi novo com todos os padrões aplicados"
---

# source-command-write

Use this skill when the user asks to run the migrated source command `write`.

## Command Template

Inicie uma sessão de escrita de código Delphi padronizado.

**Idioma de saída:** Detecte o idioma da primeira mensagem do usuário e produza
perguntas, propostas e explicações nesse idioma. Padrão: pt-BR. Suportados: pt-BR, en-US.
Honre overrides: "respond in English" / "responda em português".
Identificadores Delphi seguem a convenção do projeto e não são traduzidos.

**Passo 1 — Entender o que será criado**

Pergunte ao usuário (se não informado):
- Tipo de elemento: classe de serviço / repositório / model / form / unit utilitária / interface?
- Responsabilidade principal (deve ser única — SRP)?
- Implementa alguma interface? Qual?
- Depende de outras classes? Quais serão injetadas?
- Nome sugerido ou você pode propor?

**Passo 2 — Propor estrutura**

Antes de escrever código, apresente:
```
Proposta: [TNomeClasse]
- Herda de: [TInterfacedObject / TObject]
- Implementa: [INomeInterface]
- Injeta via construtor: [IOutraDependencia]
- Responsabilidade: [uma frase]
- Métodos públicos: [lista]
```

Aguarde aprovação antes de continuar.

**Passo 3 — Escrever código completo**

Gere o código completo, compilável, com:
- Unit header com uses organizado
- Seção interface completa
- Seção implementation com todos os métodos
- Todos os padrões aplicados automaticamente

Use o agente `developer-delphi-agent-writer` para implementação.
