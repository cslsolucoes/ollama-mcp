# Brief de implementação — Fase 7: extração de texto de `.docx`, `.pptx` e `.pdf`

> **Para:** a IA que vai implementar (Claude Code ou Cursor, aberto em `E:\MCPOllam`).
> **De:** o orquestrador (Claude), que vai **consumir** estas tools em projetos jurídicos
> (pasta de contratos `A:\Monitor\CSL Telecom\Contratos`).
> **Data:** 2026-10-28 · **Estado do projeto:** Fases 1 a 6 concluídas, binário 0.5.2.
> **Leia antes de tudo:** `CLAUDE.md`, `README.md`, `TOOLS.md`, `src/files/guard.go`,
> `src/files/policy.go`, `src/tools/filetools.go`, `src/tools/chunk.go` e o report
> `.workspace/reports/MCPOllam2026-10-23-2130-revisao-adversarial-fase4-arquivos.report.md`.
> Todas as regras do `CLAUDE.md` continuam valendo (segredos, `.cursor/` como SSOT, áreas
> protegidas, plano atualizado por fase, uma chamada ao vivo por vez).

---

## 0. Por que esta fase existe (o problema medido)

Hoje `summarize_file`, `extract` e `translate` só leem **texto UTF-8**: `files.decode()` recusa
qualquer arquivo com byte nulo (`ReasonBinary`). Na pasta de contratos, que é o uso real
seguinte, o inventário (2026-10-28) é:

| Extensão | Arquivos | Situação hoje |
| --- | ---: | --- |
| `.pdf` | 672 | recusado (binário) |
| `.docx` | 61 | recusado (é um ZIP) |
| `.doc` (Word 97-2003) | 21 | recusado — **fora do escopo** desta fase (ver §9) |
| `.txt` | 30 | funciona |

Amostra de 80 PDFs: **72 (90%) têm camada de texto** (`/Font` presente); ~10% são
digitalizações. 5 PDFs e 1 `.docx` passam de 4 MB (o `--max-file-bytes` atual).

Ou seja: para o caso jurídico, as tools de arquivo hoje servem a ~4% dos arquivos. O objetivo
é que sirvam a ~90%, **sem afrouxar nenhuma defesa do guard**.

---

## 1. O que eu (orquestrador) preciso receber — requisitos de consumo

Estes pontos são o "afinamento". Eles vêm de como eu uso o resultado em trabalho jurídico:
eu cito cláusulas, comparo um contrato com outro e preciso confiar que nada sumiu ou foi
inventado na extração.

1. **Ordem de leitura fiel.** O texto sai na ordem do documento: parágrafos, tabelas no
   lugar onde estão, slides na ordem da apresentação. Nada de "todo o texto das tabelas no fim".
2. **Marcadores de posição** que eu possa citar e que sobrevivam ao map-reduce:
   - PDF: uma linha `[página N]` no início de cada página (N começa em 1, numeração física).
   - PPTX: `[slide N]` no início de cada slide; notas do apresentador como `[notas do slide N]`.
   - DOCX: `[cabeçalho]`, `[rodapé]`, `[nota de rodapé N]`, `[comentário]` antes de cada bloco
     desses (o corpo não leva marcador). Quebra de página manual (`w:br w:type="page"`) vira
     `[quebra de página]` — o Word não guarda a paginação real, então **não** invente número
     de página para DOCX.
3. **Numeração automática de cláusulas preservada no DOCX (requisito forte).** Contratos usam
   lista numerada do Word ("CLÁUSULA 1ª", "5.2.", "a)"). Esse número **não está** em `w:t`; está
   em `w:numPr` + `word/numbering.xml`. Se ele se perder, "o que diz a cláusula 7?" fica sem
   resposta. Implementar a renderização dos formatos `decimal`, `lowerLetter`, `upperLetter`,
   `lowerRoman`, `upperRoman`, `ordinal` (gera `1º`/`2º`… em pt-BR como o Word mostra; ver
   nota), `bullet` (vira `- `) e `none`, com `w:lvlText` (ex.: `%1.%2.`), `w:start`,
   contadores por `numId`+`ilvl`, reinício de níveis inferiores e `w:lvlOverride/w:startOverride`.
   Formato não suportado → prefixo `- ` e segue (nunca falhar o documento por isso).
   *Nota:* `ordinal` no Word em pt-BR renderiza `1º`; aceite gerar `1º` para todo idioma —
   documente a simplificação.
4. **Tabelas legíveis.** Cada linha da tabela vira uma linha de texto com células separadas por
   ` | `; células com vários parágrafos têm os parágrafos unidos por espaço. Uma linha em branco
   antes e depois da tabela. Células mescladas (`w:gridSpan`, `w:vMerge` continuação) não
   repetem conteúdo.
5. **Revisões (controle de alterações) no estado "aceito".** `w:ins` entra; `w:del`/`w:delText`
   **não** entra; `w:moveFrom` não entra, `w:moveTo` entra. Se o documento tiver revisões
   pendentes, acrescente ao rodapé técnico `revisoes_pendentes=sim` — para mim isso é sinal de
   contrato não fechado.
6. **Nada de texto duplicado.** `mc:AlternateContent` traz `mc:Choice` e `mc:Fallback` com o
   mesmo texto (caixas de texto, formas): leia **só** o `mc:Choice`. Campos (`w:fldChar` /
   `w:instrText`): ignore o código do campo, mantenha o resultado exibido. `w:fldSimple`:
   mantenha o texto filho.
7. **Texto que importa e costuma sumir:** caixas de texto (`w:txbxContent`), controles de
   conteúdo (`w:sdt`/`w:sdtContent`), hiperlinks (texto do link), símbolos (`w:sym` → ignore,
   não vale o custo), tabulação (`w:tab` → `\t`), quebra de linha (`w:br`, `w:cr` → `\n`),
   hífen fixo (`w:noBreakHyphen` → `-`), hífen opcional (`w:softHyphen` → nada).
8. **Rodapé técnico ampliado** (a linha `[arquivo] ...` que `runFile` já emite) com:
   `formato=txt|docx|pptx|pdf`, `paginas=N` (PDF) ou `slides=N` (PPTX),
   `paginas_sem_texto=K` (PDF), `revisoes_pendentes=sim|nao` (DOCX). Eu uso isso para decidir se
   confio no resumo ou se abro o arquivo eu mesmo.
9. **Recusas explicativas, em pt-BR, sem caminho nem conteúdo** (mesma regra das recusas
   atuais). Casos e mensagens no §6.
10. **Determinismo.** Mesmo arquivo → mesmo texto extraído, byte a byte (ordem de mapas Go não
    pode vazar para a saída). Isso é testável e me permite comparar duas extrações.

---

## 2. Decisões que o owner precisa tomar ANTES de codar

Pergunte ao owner e registre as respostas no plano. Minha recomendação vem marcada com ★.

| # | Decisão | Opções | Recomendação |
| --- | --- | --- | --- |
| D-1 | Biblioteca de PDF | (a) **`github.com/klippa-app/go-pdfium` em modo WebAssembly** (pdfium do Chrome rodando em `wazero`, Go puro, sem cgo); (b) `github.com/ledongthuc/pdf` (Go puro, leve, extração mais frágil com fontes CID/ToUnicode); (c) chamar `pdftotext` (poppler) externo via `exec` | ★ (a), condicionada ao spike do §5.4. É a de melhor fidelidade para acentuação pt-BR e ordem de leitura, e mantém o binário único e sem cgo. (c) obriga instalar poppler em cada estação e abre processo externo — descartar salvo se (a) e (b) falharem no spike. |
| D-2 | Primeira dependência externa no `go.mod` | O `go.mod` hoje não tem nenhuma. DOCX e PPTX dão para fazer **só com stdlib** (`archive/zip` + `encoding/xml`); PDF não dá. | ★ Aceitar **uma** dependência só para PDF (+ as transitivas dela), com versão fixada e licença verificada (pdfium BSD-3, go-pdfium MIT, wazero Apache-2.0 — **confirmar no momento da implementação**). DOCX/PPTX sem dependência. |
| D-3 | Teto de tamanho para documentos binários | Manter `--max-file-bytes` (4 MB) para tudo, ou flag nova | ★ Flag nova `--max-doc-bytes` / `MCPOLLAMA_MAX_DOC_BYTES`, default **33554432** (32 MB), aplicada **só** a `.docx/.pptx/.pdf` (o tamanho em disco de um PDF não diz nada sobre o texto). O teto efetivo de texto continua sendo `--max-chunks` × parte (lógica atual de `runFile`). |
| D-4 | PDF digitalizado (sem texto) | recusar, ou OCR | ★ OCR com Tesseract, como **etapa separada (7.9)**, especificada no §11. Até a 7.9 entrar, recusar com mensagem clara. Sem `--ocr-lib` configurado, o comportamento continua sendo a recusa (fail-closed). |
| D-5 | Notas do apresentador e comentários do Word | incluir ou não | ★ Incluir, sempre rotulados (`[notas do slide N]`, `[comentário]`). Em contrato, comentário de revisão é informação relevante. |
| D-6 | Versão | — | ★ `0.6.0` (feature nova, sem quebra de contrato das tools). |
| D-13 | Orçamento do `tools/list` (`TestToolsListBudget`, hoje 4.390 de 4.400 bytes) | subir o teto, ou compensar cortando texto | ★ Subir o teto para **4.700 bytes** (~+75 tokens por sessão), com o acréscimo mínimo: só a frase curta "text (UTF-8), .docx, .pptx or .pdf file" no campo `path` e uma vez por tool. Toda a orientação detalhada (marcadores, rodapé, avisos) vai **na resposta da tool**, não na descrição — custa zero quando a tool não é usada. Medir full e compact e registrar. |

---

## 3. Arquitetura

### 3.1 Princípio: o guard continua sendo o único lugar que abre arquivo

**Não** crie um segundo caminho de abertura de arquivo. Toda a sequência de defesas do
`Guard.Read` (passos 1 a 10 do comentário em `guard.go`) roda igual para documentos. A mudança é
só **no fim**: depois de ler os bytes com segurança, em vez de sempre chamar `decode()`, o guard
despacha pelo formato.

### 3.2 Pacote novo: `src/doctext/`

```
src/doctext/
  doctext.go        // API: Detect, Extract, Limits, Result, erros tipados
  zipsafe.go        // abertura segura de ZIP (OOXML) — usado por docx e pptx
  xmlsafe.go        // decoder XML com limites (profundidade, tokens, bytes)
  docx.go           // extração DOCX
  docx_numbering.go // numbering.xml → prefixos de lista
  pptx.go           // extração PPTX
  pdf.go            // extração PDF (atrás de uma interface, ver 3.4)
  *_test.go, fuzz_test.go, testdata_gen_test.go (gera fixtures em memória)
```

Regras do pacote (mesmo espírito de `src/files`):
- **Puro e sem I/O:** recebe `[]byte` (já lido pelo guard) e devolve texto. Não abre arquivo,
  não grava em disco, não cria arquivo temporário, não chama rede, não lê variável de ambiente.
- Não conhece Ollama, MCP nem prompts.
- Toda mensagem de erro em pt-BR, sem caminho, sem trecho de conteúdo.

API sugerida (ajuste nomes ao estilo do projeto, mas mantenha a forma):

```go
type Format int // FormatText, FormatDOCX, FormatPPTX, FormatPDF

type Limits struct {
    MaxEntries        int   // entradas no ZIP (default 2000)
    MaxEntryBytes     int64 // descompactado por entrada (default 64 MiB)
    MaxTotalBytes     int64 // descompactado somado (default 256 MiB)
    MaxXMLDepth       int   // default 256
    MaxTextRunes      int   // teto do texto extraído (passado por quem chama)
    MaxPDFPages       int   // default 2000
}

type Result struct {
    Text             string // UTF-8, LF, sem BOM — mesmo contrato de files.File.Text
    Format           Format
    Pages            int    // PDF
    Slides           int    // PPTX
    PagesWithoutText int    // PDF
    PendingRevisions bool   // DOCX
}

// Detect decide o formato pela EXTENSÃO do nome resolvido E pelos bytes mágicos;
// as duas precisam concordar (ver 3.3).
func Detect(name string, head []byte) (Format, error)

func Extract(ctx context.Context, f Format, data []byte, lim Limits) (*Result, error)
```

### 3.3 Detecção de formato (extensão **e** assinatura)

| Extensão (case-insensitive, do nome **resolvido**) | Assinatura obrigatória | Formato |
| --- | --- | --- |
| `.docx` | `PK\x03\x04` e o ZIP contém `word/document.xml` | DOCX |
| `.pptx` | `PK\x03\x04` e contém `ppt/presentation.xml` | PPTX |
| `.pdf` | `%PDF-` nos primeiros 1024 bytes (há PDFs com lixo antes do cabeçalho) | PDF |
| qualquer outra | — | texto (caminho atual, `decode()`) |

- Extensão de documento + assinatura errada → recusa (`ReasonFormat`, ver §6). Ex.: um `.docx`
  que na verdade é um OLE (`D0 CF 11 E0`) é **documento Office protegido por senha** (o Office
  cifrado salva em contêiner OLE) ou `.doc` renomeado — mensagem específica para cada um.
- Sem extensão de documento, mesmo começando com `PK` ou `%PDF`: segue o caminho de texto e
  será recusado como binário, como hoje. **Não** detecte formato só por conteúdo: o owner
  controla o comportamento pela extensão, e isso evita surpresa.
- `.docm`/`.pptm`/`.dotx`/`.potx`: **fora do escopo** (recusar como hoje). Ler o texto deles é
  inofensivo, mas não há demanda; fica registrado como pendência.

### 3.4 Integração com `src/files`

- Em `files.Guard`, acrescente um campo opcional injetado no `backend.go`, por exemplo
  `Extract func(ctx, name string, data []byte) (*doctext.Result, error)` **ou** uma interface
  pequena declarada em `files` (preferível, para `files` não importar `doctext` — mantém o
  pacote puro e testável). Sem extrator injetado → comportamento exatamente igual ao atual
  (fail-closed: documento continua recusado como binário).
- O teto de bytes passa a depender do formato: `--max-file-bytes` para texto e
  `--max-doc-bytes` para documento. Atenção: hoje o teto é verificado **antes** de abrir
  (`pre.Size()`) e de novo na leitura (`LimitReader`). Para escolher o teto certo antes de
  abrir, decida pela **extensão do nome resolvido** (já disponível no passo 6); a assinatura é
  conferida depois, nos bytes lidos.
- `files.File` ganha os campos do `Result` que `runFile` precisa para o rodapé técnico
  (`Format`, `Pages`, `Slides`, `PagesWithoutText`, `PendingRevisions`). `Bytes` continua sendo
  o tamanho em disco.
- `Read` hoje não recebe `context`. A extração de PDF pode ser lenta: passe a aceitar `ctx`
  (`ReadContext(ctx, path)`, mantendo `Read` como atalho com `context.Background()` se isso
  evitar mexer em muitos testes) e respeite cancelamento/timeout dentro do extrator.
- Depois do extrator, aplique ao texto as **mesmas** normalizações de `decode()`: CRLF→LF, sem
  BOM, e **valide UTF-8** (`utf8.ValidString`); runa inválida → substitua por U+FFFD e conte
  (se mais de 1% do texto for U+FFFD, recuse como "texto extraído corrompido": é sinal de fonte
  sem ToUnicode e o resumo sairia lixo). Remova caracteres de controle exceto `\n` e `\t`.
- Texto extraído vazio (só espaços e marcadores) → para PDF, recusa de "sem camada de texto";
  para DOCX/PPTX, a recusa atual de `runFile` ("o arquivo está vazio") já cobre.

### 3.5 `src/tools/filetools.go`

- Descrições das 3 tools e do campo `path`, **seguindo a D-13 (acréscimo mínimo)**: trocar
  "text file" por "text (UTF-8), .docx, .pptx or .pdf file" — e nada além disso nas descrições.
- O que era orientação longa vai na **resposta** da tool: em `translate` com documento, acrescentar
  após o rodapé `[aviso] tradução devolvida como texto; nenhum documento foi gravado`.
- Rodapé `[arquivo]`: acrescentar os campos do §1.8, só os que se aplicam ao formato.
- Regerar `TOOLS.md` com `go run ./cmd/dumptools` (o `TestCatalogInSync` falha se divergir).
  Conferir também o modo `--tools-verbosity compact`: as descrições novas não podem inflar o
  `tools/list` compacto mais que o necessário (meça o tamanho antes e depois e registre).
- `splitChunks` (`chunk.go`): verifique se ele quebra em fronteira de linha. Os marcadores
  `[página N]`/`[slide N]` são linhas próprias; ótimo se uma parte começar num marcador.
  **Não** mude o algoritmo de chunk nesta fase, só confirme que um marcador nunca é cortado ao
  meio (teste).

### 3.6 `src/mcpollama-mcp/config.go`

- Flag `--max-doc-bytes` / `MCPOLLAMA_MAX_DOC_BYTES` (D-3), com validação de positivo junto às
  demais e linha no texto de ajuda. Atualizar a tabela de flags no `README.md` §3 e no
  `CLAUDE.md`.
- Não crie flag para desligar a extração: sem `--allow-root` as tools de arquivo já não
  existem; com `--allow-root` a extração é parte da tool.

---

## 4. Especificação por formato

### 4.1 DOCX (só stdlib)

Entradas a ler, nesta ordem de saída:
1. `word/document.xml` (corpo) — **obrigatória**.
2. Cabeçalhos e rodapés: descubra quais são **de verdade** usados pelas `w:sectPr`
   (`w:headerReference`/`w:footerReference` → `r:id` → `word/_rels/document.xml.rels`).
   Emita cada um **uma vez** (dedupe pelo destino), antes do corpo para cabeçalho e depois do
   corpo para rodapé, rotulados. Timbre de escritório costuma estar no cabeçalho como imagem:
   sem texto → não emita o rótulo vazio.
3. `word/footnotes.xml` e `word/endnotes.xml`: ignore as notas de separador
   (`w:type="separator"`, `continuationSeparator`); emita as demais após o corpo como
   `[nota de rodapé N]` (N = `w:id`). No corpo, a referência `w:footnoteReference` vira `[N]`.
4. `word/comments.xml`: após as notas, `[comentário] <autor>: <texto>`. **Não** emita data.
5. Ignore: `docProps/*` (metadados, autor, empresa — não vazar), `customXml/*`, `word/glossary/*`,
   imagens, `vbaProject.bin`, fontes embutidas.

Resolução dos caminhos dentro do ZIP: siga os `.rels` (`Target` relativo a `word/`), normalize
com `path.Clean` e **recuse** alvo que saia da raiz do pacote (`..`, absoluto) ou que seja
`TargetMode="External"` — ver §5.1.

Parágrafos: cada `w:p` termina em `\n`. Parágrafo vazio vira linha vazia, mas colapse mais de 2
linhas vazias seguidas em 2. Estilo de título (`w:pStyle` = `Heading*`/`Ttulo*`/`Title`; **`Ttulo*` não é erro de digitação**: o Word grava o styleId sem os caracteres não ASCII, então "Título 1" vira `Ttulo1` em documentos pt-BR) não
precisa de marcação especial.

Numeração (§1.3): leia `word/numbering.xml` (`w:abstractNum` → `w:lvl`; `w:num` →
`w:abstractNumId` + `w:lvlOverride`). O `w:numPr` pode vir no parágrafo **ou herdado do estilo**
(`word/styles.xml`, `w:style/w:pPr/w:numPr`, incluindo `w:basedOn` em cadeia — limite a cadeia
a 32 saltos para evitar ciclo). Contadores: um por (`numId` resolvido para `abstractNumId`,
`ilvl`); ao incrementar o nível k, zere os níveis > k (respeitando `w:lvlRestart` quando
presente). Prefixo = `w:lvlText` com `%1..%9` substituídos, seguido de espaço ou `\t`
conforme `w:suff` (default `tab` → use `\t`).

### 4.2 PPTX (só stdlib)

- Ordem dos slides: `ppt/presentation.xml` → `p:sldIdLst/p:sldId` (`r:id`) →
  `ppt/_rels/presentation.xml.rels` → `slideN.xml`. **Nunca** ordene pelo nome do arquivo
  (`slide10.xml` < `slide2.xml` em ordem lexical e a numeração do arquivo não é a ordem de exibição).
- Slides ocultos (`show="0"`): extraia e rotule `[slide N — oculto]`.
- Dentro do slide: percorra a árvore de formas (`p:spTree`, incluindo `p:grpSp` aninhado) na
  ordem do XML. `a:p` → linha; `a:t` → texto; `a:br` → `\n`; `a:tab` não existe — tabulação
  vem como texto. Tabelas `a:tbl` → mesma regra do §1.4. `mc:AlternateContent` → só `mc:Choice`.
- Notas: `slideN.xml.rels` → `notesSlide` → texto do placeholder de corpo (`p:ph type="body"`),
  sem o número de slide do placeholder `sldNum`.
- Ignore layouts e masters (`slideLayout*`, `slideMaster*`): o texto deles é "Clique para editar
  o título", lixo para o modelo.

### 4.3 PDF (dependência decidida em D-1)

- Esconda a biblioteca atrás de uma interface no `pdf.go`
  (`type pdfEngine interface { Pages(ctx, data) (int, error); PageText(ctx, i) (string, error) }`)
  para poder trocar de motor e para testar sem o motor real.
- Instância única e **preguiçosa** do motor (se for go-pdfium WASM: inicializar no primeiro PDF,
  não na partida do servidor — a partida tem de continuar instantânea; meça e registre o custo
  da 1ª chamada). Acesso serializado (a fila de GPU já serializa as tools; ainda assim proteja
  com mutex).
- Por página: `[página N]\n` + texto. Junte hifenização de fim de linha **só** quando a linha
  termina em `-` e a próxima começa com minúscula (heurística conservadora; registre no teste).
  Não tente reconstruir parágrafos além disso.
- Página com menos de 20 runas não-brancas conta em `paginas_sem_texto`. Se **todas** as
  páginas estiverem assim → recusa "PDF sem camada de texto". Se só algumas → segue, e o rodapé
  mostra `paginas_sem_texto=K` (contrato com páginas de assinatura digitalizadas é comum).
- PDF com senha de abertura → recusa específica. PDF com senha só de permissão (senha de
  usuário vazia, muito comum) → **deve abrir** e extrair (teste obrigatório).
- Limites: `MaxPDFPages`; timeout via `ctx`; PDF malformado → recusa, **nunca** panic (envolva o
  motor com `recover` e converta em recusa genérica "PDF corrompido ou não suportado").
- Anotações, formulários (AcroForm), anexos embutidos e JavaScript do PDF: **ignorar**. Não
  extrair anexo, não executar nada.

---

## 5. Segurança (a parte que não pode falhar)

Este servidor manda o texto para um modelo remoto e roda na estação do owner. Documento é a
maior superfície de parser do projeto. Trate cada item como requisito com teste.

### 5.1 ZIP (DOCX/PPTX)
- Abra com `zip.NewReader(bytes.NewReader(data), int64(len(data)))` — nada em disco.
- Limite de entradas (`MaxEntries`) **antes** de iterar.
- **Não confie em `UncompressedSize64`** do cabeçalho: leia cada entrada por
  `io.LimitReader(rc, MaxEntryBytes+1)` e recuse se passar; some os bytes descompactados reais e
  recuse ao passar de `MaxTotalBytes` (zip bomb, inclusive a "sobreposta" que reutiliza dados).
- Leia **somente** as entradas que a spec acima nomeia, buscadas por nome exato normalizado.
  Nunca itere extraindo tudo.
- Nomes de entrada com `..`, `\`, absolutos ou duplicados (duas entradas com o mesmo nome
  normalizado) → recuse o documento (duplicata permite esconder um `document.xml` atrás de outro).
- Entrada cifrada (bit 0 do flag) ou método diferente de Store/Deflate → recusa.
- Relações `TargetMode="External"` (links para fora, imagens remotas, `attachedTemplate`
  remoto): **ignorar sempre**, nunca resolver, nunca acessar rede.

### 5.2 XML
- `encoding/xml` do Go não resolve entidades externas (sem XXE) e não expande DTD; mantenha
  assim: **não** configure `Decoder.Entity` com nada além do padrão, e recuse o documento se
  encontrar `<!DOCTYPE` (OOXML legítimo não tem DTD).
- Decoder em streaming (`Token()`), nunca `Unmarshal` do documento inteiro numa árvore.
- Limite de profundidade (`MaxXMLDepth`) e de texto acumulado (`MaxTextRunes`): ao passar,
  pare e recuse com "documento extenso demais" (coerente com a recusa de `--max-chunks`).
- Namespaces: compare por **URI** (`Name.Space`), não pelo prefixo (`w:` pode vir com outro
  prefixo). URIs: WordprocessingML `http://schemas.openxmlformats.org/wordprocessingml/2006/main`,
  DrawingML `.../drawingml/2006/main`, PresentationML `.../presentationml/2006/main`,
  relationships, markup-compatibility. Documentos "Strict OOXML" usam URIs
  `http://purl.oclc.org/ooxml/...`: aceite os dois conjuntos.

### 5.3 Geral
- Denylist e todas as defesas de caminho do guard: **inalteradas** e rodando antes da extração.
- Nenhum texto extraído, nome de autor, metadado ou caminho vai para log, nem com `--debug`
  (o debug continua só com metadados: formato, páginas, bytes, ms de extração).
- Panic em qualquer extrator → recusa genérica, servidor segue vivo (teste com `recover`).
- O texto extraído entra no prompt com o mesmo `nonce`/`wrap` atual — documento é dado não
  confiável e pode conter injeção de prompt ("ignore as instruções…"); não mude essa proteção.

### 5.4 Spike obrigatório antes de escolher o motor de PDF (D-1)
1. Monte localmente (**fora do git**, ex.: `.workspace/spike-pdf/`, adicionar ao `.gitignore`) um
   corpus de 15 PDFs da pasta de contratos, escolhidos para variar: gerados pelo Word, gerados
   por sistema de tribunal/cartório, com tabela, com senha de permissão, digitalizado, com
   assinatura digital, maior que 4 MB. **São documentos de clientes: nunca commitar, nunca
   colar trecho em report, chat ou log.** O report registra só métricas.
2. Rode cada motor candidato e meça: acentuação pt-BR correta (contagem de `ç ã õ é` vs
   `pdftotext -layout` como referência — existe no ambiente Linux do owner), ordem de leitura
   (inspeção visual por você, sem reproduzir texto no report), tempo, memória de pico, tamanho
   do binário final, falhas/panics.
3. Report em `.workspace/reports/MCPOllam<timestamp>-spike-motor-pdf.report.md` com a tabela de
   métricas e a recomendação. Leve ao owner antes de fixar a dependência.

---

## 6. Recusas novas (adicionar a `files.Reason` ou a um tipo equivalente em `doctext`)

Todas as mensagens em pt-BR, sem caminho, sem conteúdo, acionáveis:

| Caso | Mensagem sugerida |
| --- | --- |
| Extensão de documento, assinatura não confere | "o conteúdo não corresponde à extensão do arquivo (.docx/.pptx/.pdf); o arquivo pode estar corrompido ou renomeado" |
| `.docx`/`.pptx` em contêiner OLE (cifrado) | "documento do Office protegido por senha não é suportado; salve uma cópia sem senha" |
| `.doc` / `.ppt` (OLE legado) | "formato Word/PowerPoint 97-2003 não é suportado; salve como .docx ou .pptx" |
| PDF com senha de abertura | "PDF protegido por senha não é suportado; gere uma cópia sem senha" |
| PDF sem texto | "o PDF não tem camada de texto (provavelmente é digitalizado); OCR ainda não é suportado" |
| ZIP/XML malformado, limites estourados, DTD, entrada duplicada | "documento corrompido, malformado ou acima dos limites de segurança" (uma mensagem só, sem dizer qual limite — mesma lógica opaca do FILE-014) |
| Texto extraído > teto | "documento extenso demais: o texto extraído passa do limite; divida o documento" |
| Texto extraído com > 1% U+FFFD | "o texto extraído do PDF está corrompido (fontes sem mapeamento de caracteres); não é possível resumi-lo com segurança" |
| Timeout da extração | "a extração do texto excedeu o tempo limite (--timeout)" |

Não esquecer: `.doc` precisa ser **detectado** para dar a mensagem boa (hoje cai em "binário").
Detectar `.doc`/`.ppt`/`.xls` pela extensão + assinatura OLE `D0 CF 11 E0 A1 B1 1A E1`.

---

## 7. Testes (definição de pronto)

Fixtures **geradas em código** (`archive/zip` + XML escrito no teste) — nada de `.docx`/`.pdf`
binário de terceiros no repositório. Para PDF, gere PDFs mínimos à mão no teste (um PDF de 1
página com fonte Type1 padrão e texto ASCII + um com `ç`/`ã` via `WinAnsiEncoding`) ou, se o
motor exigir, um fixture pequeno **criado por você** e versionado em `src/doctext/testdata/`
com o script que o gerou.

### 7.1 Unitários — DOCX
- parágrafos, tab, br, quebra de página → marcador
- tabela 2×3, célula mesclada horizontal e vertical
- numeração: `CLÁUSULA %1ª` decimal; `%1.%2.` multinível com reinício; `lowerLetter` `a)`;
  `upperRoman`; `bullet`; numeração herdada de estilo; `startOverride`
- revisões: `w:ins` entra, `w:del` não, `revisoes_pendentes=sim`
- `mc:AlternateContent` com caixa de texto → texto aparece **uma** vez
- campo `PAGE`/`HYPERLINK` com `instrText` → só o resultado
- cabeçalho usado por 2 seções → emitido uma vez; cabeçalho só com imagem → nada
- notas de rodapé (separadores ignorados) e comentários com autor
- Strict OOXML (URIs purl)
- prefixo de namespace não padrão
- determinismo: extrair 20 vezes → mesmo hash

### 7.2 Unitários — PPTX
- ordem por `sldIdLst` com arquivos `slide1, slide2, slide10` fora de ordem
- grupo aninhado, tabela, slide oculto, notas, layout/master ignorados

### 7.3 Unitários — PDF
- 1 página ASCII; acentuação; 3 páginas com marcadores; página em branco no meio
  (`paginas_sem_texto=1`); senha de permissão abre; senha de abertura recusa; PDF truncado
  recusa sem panic; "todas as páginas sem texto" recusa

### 7.4 Segurança (arquivo `doctext/security_test.go` + extensão dos testes de `files`)
- zip bomb: entrada declarando 1 KB que descompacta 100 MB → recusa, memória de pico limitada
  (meça com `testing.AllocsPerRun` ou `runtime.ReadMemStats`, registre o número)
- 10 000 entradas → recusa antes de iterar
- entrada `../../evil`, entrada duplicada, entrada cifrada → recusa
- `<!DOCTYPE` com entidade → recusa; XML com profundidade 10 000 → recusa
- relação `External` para `http://` e para `file:///C:/...` → ignorada (e o teste prova que não
  houve acesso: use um `http.RoundTripper` que falha o teste se chamado — embora o pacote não
  tenha rede, deixe o teste como sentinela)
- `.pdf` com conteúdo ZIP, `.docx` com conteúdo PDF, `.docx` OLE → mensagens do §6
- documento dentro de diretório `secrets/` e nome `credentials.docx` → **denylist recusa antes
  de ler** (regressão do guard)
- hardlink e symlink para `.docx` fora da raiz → recusa (regressão)
- **Rodar de novo todo o roteiro de ataques da revisão adversarial da Fase 4** (exigência do
  `CLAUDE.md` para qualquer mudança em `src/files/`) e anexar o resultado ao report da fase.

### 7.5 Fuzzing (Go nativo)
`FuzzDOCX`, `FuzzPPTX`, `FuzzPDF` com seeds das fixtures. Critério: **30 minutos por alvo sem
panic, sem travar (>5 s por entrada) e sem alocação > 512 MiB**. Registrar no report o tempo
rodado e o corpus final (o corpus de fuzz pode ser versionado em `testdata/fuzz/` se for
gerado e não contiver dado de cliente).

### 7.6 Integração e ao vivo
- `tools`: `summarize_file` com `.docx` gerado em teste + `httptest` simulando o Ollama: o
  prompt enviado contém o texto extraído dentro do `wrap` com nonce e o rodapé tem
  `formato=docx`.
- `-tags live`, **uma chamada por vez**: `summarize_file` e `extract` reais em 1 `.docx` e 1 `.pdf`
  do corpus do spike (local, fora do git). No report: só contadores (`[uso]`, `[arquivo]`) e
  tempos, nada de conteúdo.
- `go vet ./...`, `go test -count=1 ./...` e `go test -race -count=1 ./...` (com o gcc de `deps\`,
  conforme `CLAUDE.md`) verdes.
- Smoke test stdio do `CLAUDE.md` com o binário novo: `tools/list` mostra as descrições novas.

### 7.7 Aceite com o caso real
Com o servidor iniciado com `--allow-root "A:\Monitor\CSL Telecom\Contratos"` (atenção: caminho
com espaços, e `A:` pode ser unidade mapeada/rede — confirme que `NewGuard` aceita; se a unidade
for de rede e o guard recusar, **não afrouxe o guard**: registre e leve ao owner):
1. `summarize_file` num `.docx` de contrato → resumo cita cláusulas pelo número correto.
2. `extract` com instrução "liste as cláusulas com número e título" num `.docx` com numeração
   automática → números batem com o que o Word mostra (você confere abrindo o documento; não
   reproduza o conteúdo no report).
3. `summarize_file` num PDF com texto → rodapé com `paginas=` coerente.
4. PDF digitalizado → recusa do §6.
5. `.doc` → recusa do §6.

---

## 8. Entregáveis e processo

1. Atualizar `.workspace/plans/MCPOllam2026-10-23-150845-mcpollama-implementacao.plan.md`:
   nova **Fase 7 — Extração de documentos**, com as decisões D-1..D-6 e D-13, as pendências e o
   changelog. (Está em UTF-8 sem BOM; confira antes de editar e preserve a codificação.)
2. Etapas sugeridas, cada uma com commit próprio e testes verdes:
   - 7.0 spike PDF + report + decisão do owner
   - 7.1 `doctext` base (`Detect`, `zipsafe`, `xmlsafe`) + testes de segurança
   - 7.2 DOCX sem numeração → 7.3 numeração DOCX
   - 7.4 PPTX
   - 7.5 PDF
   - 7.6 integração no guard + flag `--max-doc-bytes` + rodapé + descrições + `TOOLS.md`
   - 7.7 fuzzing, revisão adversarial repetida, ao vivo, aceite §7.7
   - 7.8 docs (`README.md` §1/§3, `CLAUDE.md` estado e flags) + binário `0.6.0` em `D:\MCP\`
   - 7.9 OCR (§11) + binário `0.7.0`
3. Commits no padrão do repositório (`feat:`, `fix:`, `docs:`, `test:`), em português, sem
   segredo, sem binário, sem documento de cliente. Antes de cada commit:
   `git status` + varredura de segredos (padrão do brief) + conferir que `.workspace/spike-pdf/`
   está ignorado.
4. Report final em `.workspace/reports/MCPOllam<timestamp>-fase7-extracao-documentos.report.md`:
   o que foi feito, métricas (tempo de extração por formato, memória, tamanho do binário antes e
   depois, tamanho do `tools/list` full e compact antes e depois, resultado do fuzz e da
   revisão adversarial), limitações conhecidas e pendências.
5. Lembrete operacional para o owner no report (não é tarefa sua editar): o cliente MCP da pasta
   de contratos precisa incluir `A:\Monitor\CSL Telecom\Contratos` no `--allow-root`, e o binário
   em uso precisa ser trocado (o `.exe` em uso vira `.exe~` no build — apagar após reiniciar o
   cliente).

---

## 9. Fora do escopo (registrar como pendência, não implementar)

- `.doc`, `.ppt`, `.xls` binários (OLE/CFB) — só a recusa com mensagem boa.
- `.xlsx`, `.odt`, `.rtf`, `.html`, `.eml`/`.msg` — candidatos a fase futura.
- OCR: especificado no §11 como etapa 7.9 (pode virar fase própria se o owner preferir).
- Gravar documento traduzido (`translate` devolve texto).
- Tool nova de "listar/inventariar uma pasta" — útil para triagem de volume no caso jurídico,
  mas é outra superfície de ataque; propor ao owner como fase separada.
- Extração de imagens, anexos de PDF, macros.

---

## 10. Checklist final (copie para o report e marque)

- [x] D-1..D-6 e D-13 decididas pelo owner e registradas no plano (+ D-14 e D-15, ver §12)
- [x] Spike de PDF com report só de métricas
- [x] DOCX: corpo, tabelas, numeração, revisões, cabeçalho/rodapé, notas, comentários, sem duplicata
- [x] PPTX: ordem por `sldIdLst`, grupos, tabelas, notas, ocultos
- [x] PDF: marcadores de página, sem texto, senhas, malformado sem panic
- [x] Extensão + assinatura; `.doc`/OLE com mensagem própria
- [ ] Limites de ZIP/XML/texto; sem DTD; sem `External`; sem I/O no `doctext`
- [x] Guard: todas as defesas antes da extração; `--max-doc-bytes`; `ctx`
- [x] Rodapé `[arquivo]` com `formato`, `paginas`/`slides`, `paginas_sem_texto`, `revisoes_pendentes`
- [x] Descrições mínimas (D-13) + `TOOLS.md` regerado + teto do `TestToolsListBudget` ajustado + medição full/compact do `tools/list`
- [ ] Unitários, segurança, fuzz 30 min/alvo, `-race`, revisão adversarial da Fase 4 repetida
- [ ] Ao vivo (uma chamada por vez) e aceite §7.7 com o caso real
- [ ] README, CLAUDE.md, plano e report atualizados; binário 0.6.0 em `D:\MCP\`
- [ ] Nenhum segredo, binário ou documento de cliente no git
- [ ] (7.9) OCR: D-7..D-11 decididas; worker isolado; DLL por caminho absoluto + SHA-256; marcadores e rodapé de OCR; aviso de números; testes e aceite §11.9

---

## 11. Etapa 7.9 — OCR de PDF digitalizado (e, opcionalmente, imagens)

### 11.1 O que já existe na estação (verificado em 2026-10-28)

| Item | Onde | Observação |
| --- | --- | --- |
| `libtesseract64.dll` (Tesseract **5.3.3**, x64) | `referencias/tessract-group/` | **Autocontida**: só importa `KERNEL32.dll` e `msvcrt.dll` (Leptonica embutida). Exporta a API C completa: `TessVersion`, `TessBaseAPICreate`, `TessBaseAPIInit3`, `TessBaseAPISetImage`, `TessBaseAPISetPageSegMode`, `TessBaseAPISetSourceResolution`, `TessBaseAPISetVariable`, `TessBaseAPIRecognize`, `TessBaseAPIGetUTF8Text`, `TessBaseAPIMeanTextConf`, `TessBaseAPIAllWordConfidences`, `TessDeleteText`, `TessBaseAPIClear`, `TessBaseAPIEnd`, `TessBaseAPIDelete`. **Não** exporta `pixReadMem` — use `TessBaseAPISetImage` com pixels crus (é o que queremos). SHA-256 começa com `964b0972348f6283`. |
| `libtesseract64.so` (Linux x64) | idem | para o binário Linux, se o owner quiser OCR lá (D-10). SHA-256 começa com `3c71e2ef2f2f0ae6`. |
| `libtesseract32.dll`, `.dylib`, `.so.aarch64` | idem | não usar. |
| Modelos `por`, `eng`, `osd` | `tessdata/` (15 MB), `tessdata_best/` (7,8 MB), `tessdata_fast/` (1,9 MB) | `osd` serve para detectar página girada. |
| Fontes Tesseract **5.5.2** + Leptonica 1.88 + vcpkg | `tesseract/`, `leptonica/`, `install/`, `vcpkg/` | permitem compilar uma DLL própria (D-8). |
| zlib (1.2.13, 1.3.2, master) | `ZLibSource/` e `sdk/zlib*` | só seria necessário para compilar a Leptonica; com pixels crus não precisamos de libpng/libjpeg/libtiff. |
| MinGW-w64 gcc | `deps/gcc-mingw-w64/` | para compilar Tesseract/Leptonica (D-8). **Não** é preciso cgo no mcpollama (ver 11.3). |
| `sdk/arj, bzip2, cabnet, lha, lzma2601, unrar` | `sdk/` | **irrelevantes** para OCR; não usar. |
| `Tesseract5/*.pas` (wrapper Delphi) | `Tesseract5/` | útil só como referência da ordem de chamadas da API C. |

**Prova de viabilidade (VM Linux do owner, Tesseract 4.1.1, `tessdata_best/por`)**: 1 página de um
contrato digitalizado da pasta de contratos, renderizada a 300 dpi em cinza → **96 palavras,
confiança média 93%, 1 palavra abaixo de 60%**, acentos reconhecidos, **~7 s por página** e ~140 MB
de memória. Com o 5.x na estação a qualidade deve ser igual ou melhor; o tempo precisa ser medido
lá. Nenhum conteúdo foi reproduzido.

### 11.2 Pipeline

```
PDF ──(pdfium, já da etapa 7.5)──► página sem texto (< 20 runas)
     └─ renderizar a página em bitmap cinza 8 bits a --ocr-dpi (300)
           └─ pixels crus ──► Tesseract (API C) ──► texto UTF-8 + confiança
```

- **Isto fecha a D-1 a favor do pdfium**: o OCR exige **renderizar** a página, e `ledongthuc/pdf`
  não renderiza. Se o spike tiver escolhido outro motor, a única alternativa é `pdftoppm`
  externo — levar ao owner.
- OCR **só** nas páginas sem texto; páginas com texto seguem pela extração normal (PDF misto:
  contrato digitado com página de assinatura digitalizada é o caso mais comum).
- Página toda branca (nenhum pixel com valor < 200 em uma amostra) → não chamar o Tesseract;
  conta como sem texto.

### 11.3 Como chamar a DLL a partir do Go — **sem cgo**

- Windows: `golang.org/x/sys/windows` (`windows.LoadLibraryEx` + `GetProcAddress`, ou `syscall.SyscallN`).
  Nada de cgo: o binário continua compilando com `CGO_ENABLED=0`.
- Linux (se D-10 = sim): `github.com/ebitengine/purego` (`Dlopen`/`RegisterLibFunc`), também sem cgo.
  Atrás de build tags (`ocr_windows.go`, `ocr_linux.go`, `ocr_other.go` = OCR indisponível).
- Ordem de chamadas por página: `Create` → `Init3(api, tessdataDir, "por")` (uma vez por worker) →
  `SetPageSegMode(api, 3 /*PSM_AUTO*/ )` → `SetSourceResolution(api, dpi)` →
  `SetImage(api, &pix[0], w, h, 1 /*bytes por pixel*/, stride)` → `GetUTF8Text` → copiar até o NUL →
  `TessDeleteText` → `MeanTextConf` → `Clear`. No fim: `End` → `Delete`.
- Strings C: `[]byte` terminado em NUL, mantido vivo com `runtime.KeepAlive` até a chamada
  voltar; o buffer de pixels idem. Nunca guardar ponteiro Go dentro de memória C.
- Variáveis recomendadas via `SetVariable`: `debug_file` = `NUL` (Windows) / `/dev/null` (impede o
  Tesseract de escrever no stderr/stdout — **stdout é exclusivo do JSON-RPC**), `user_defined_dpi`.
- Uma instância `TessBaseAPI` **não** é thread-safe: um por worker.

### 11.4 Isolamento: o OCR roda num processo filho (requisito de segurança)

Diferente do pdfium em WASM (sandbox do `wazero`, panic recuperável), o Tesseract é **código
nativo**: um crash ou corrupção de memória dentro da DLL **mata o servidor MCP inteiro** e o
`recover` do Go não pega. Por isso:

- ★ O próprio `mcpollama-mcp.exe` ganha um modo interno `--ocr-worker` (não documentado no
  `--help` público, recusado se stdin for terminal). O servidor sobe N filhos sob demanda, envia
  por stdin um frame binário simples (`largura, altura, stride, dpi, pixels`) e recebe por stdout
  `confiança + texto UTF-8`. O filho só carrega a DLL; não lê arquivo, não abre rede, não herda
  variáveis com o token (limpe `OLLAMA_API_KEY` e afins do `Env` do filho; não passe `--api-key`).
- Timeout por página (`--ocr-page-timeout`, default 60 s): estourou → mata o filho, página vira
  `[página N — OCR falhou]`, o documento segue.
- Filho morreu → recria uma vez; morreu de novo → OCR indisponível para essa chamada (recusa
  clara), servidor vivo.
- Windows: o filho em Job Object com limite de memória (`--ocr-max-mem`, default 1 GiB) e
  `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` (filho morre junto se o servidor cair).
- Alternativa mais simples (in-process) só se o owner aceitar explicitamente o risco (D-9).

### 11.5 Carga da DLL (anti-sequestro de DLL e procedência)

- **Caminho absoluto obrigatório** em `--ocr-lib` / `MCPOLLAMA_OCR_LIB`. Nunca buscar no `PATH` nem
  no diretório corrente. Windows: `LoadLibraryEx(path, 0, LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR | LOAD_LIBRARY_SEARCH_SYSTEM32)`.
- **Hash fixado**: `--ocr-lib-sha256` / `MCPOLLAMA_OCR_LIB_SHA256` obrigatório quando `--ocr-lib` é
  dado; o servidor calcula o SHA-256 do arquivo na partida e **não** liga o OCR se divergir
  (WARNING no stderr, tools seguem sem OCR). Idem para o `.traineddata` (`--ocr-tessdata-sha256`
  opcional, recomendado).
- A DLL e os modelos ficam fora do projeto, junto do executável: `D:\MCP\ocr\libtesseract64.dll`
  e `D:\MCP\ocr\tessdata\{por,osd}.traineddata`. `referencias/` é **somente leitura**: copie de lá
  (ou compile, D-8), nunca carregue direto de `referencias/`.
- Logar na partida (stderr): versão (`TessVersion`), idiomas, modelo, workers. Sem caminhos de
  documentos, sem texto.

### 11.6 Desempenho e limites

- Medido na VM: ~7 s/página com `tessdata_best`. Um contrato digitalizado de 20 páginas = ~2,5 min,
  **acima** do `--timeout` (120 s) e do que um cliente MCP costuma esperar (~87 s validado).
- Medidas obrigatórias:
  - workers paralelos `--ocr-workers` (default `min(NumCPU/2, 4)`, mínimo 1);
  - teto de páginas OCR por chamada `--ocr-max-pages` (default 30) → acima disso, recusa
    "documento digitalizado extenso demais para OCR nesta chamada";
  - orçamento de tempo total do OCR derivado do `ctx` da tool; ao estourar, as páginas restantes
    viram `[página N — OCR não processado: tempo esgotado]` e o rodapé diz quantas;
  - medir `tessdata_fast` vs `tessdata_best` na estação (tempo e confiança média em 5 páginas
    reais) e registrar; default do modelo decidido pelo owner (D-11) com base nisso.
- Teto de pixels por página: `--ocr-max-megapixels` (default 40 MP; A4 a 300 dpi ≈ 8,7 MP). Página
  gigante → reduzir o dpi até caber, não recusar.
- Cache de OCR em disco: **fora do escopo** (o servidor não grava em disco sem flag). Registrar
  como pendência — ajudaria muito em triagem repetida.

### 11.7 Saída (o que eu, orquestrador, preciso)

- Marcador por página: `[página N — OCR conf=93]` em vez de `[página N]`.
- Confiança < 60 → `[página N — OCR baixa confiança conf=41]`.
- Rodapé `[arquivo]`: `paginas_ocr=K conf_ocr_media=X paginas_ocr_falhas=F`.
- **Aviso na resposta, não na descrição** (orçamento do `tools/list`, D-13): sempre que houver
  `paginas_ocr>0`, acrescentar após o rodapé a linha `[aviso] texto obtido por OCR: números, datas,
  nomes e CPF/CNPJ podem conter erros de reconhecimento; confira os valores críticos no original`.
  Em contrato, um CPF, valor ou data trocado pelo OCR é o erro mais caro — eu preciso ser lembrado
  exatamente quando ele acontece.
- O texto OCR passa pelas mesmas normalizações (UTF-8, LF, controle) e pela mesma regra de
  U+FFFD do §3.4 (exceto que conf baixa não recusa: só marca).
- Rotação: com `osd.traineddata`, detectar orientação da página (PSM 0) quando a confiança da
  primeira passada for < 50 e reprocessar girada; registrar `girada=90|180|270` no marcador.

### 11.8 Decisões do owner para a 7.9

| # | Decisão | Recomendação |
| --- | --- | --- |
| D-7 | Ligar OCR | ★ Sim, opt-in por flag (`--ocr-lib`); sem a flag, nada muda. |
| D-8 | Procedência da DLL | ★ Compilar o Tesseract **5.5.2** a partir de `referencias/tessract-group/tesseract` + Leptonica 1.88 (+ zlib) com o MinGW de `deps\`, numa pasta de build fora de `referencias/` (ex.: `.workspace/build-ocr/`, ignorada no git), sem libs de imagem. Alternativa aceitável: usar a `libtesseract64.dll` 5.3.3 pronta (binário de repositório Delphi de terceiro) depois de registrar o hash e a procedência no report. |
| D-9 | Isolamento | ★ Processo filho `--ocr-worker` (§11.4). |
| D-10 | OCR no binário Linux | ★ Não nesta etapa (o uso real é Windows); `ocr_other.go` responde "OCR indisponível nesta plataforma". |
| D-11 | Modelo padrão | Decidir após a medição do §11.6 (`fast` provavelmente basta para triagem; `best` para extração de cláusulas). Permitir escolha por flag `--ocr-tessdata`. |
| D-12 | Imagens soltas (`.png`, `.jpg`, `.tif`) | Opcional. A pasta de contratos tem 57 `.jpg` e 27 `.png` (podem ser fotos de documentos). Se sim: `.png`/`.jpg` com `image/png`/`image/jpeg` da stdlib (`DecodeConfig` **antes** de decodificar, para barrar bomba de descompressão pelo teto de megapixels), converter para cinza e mandar ao mesmo worker. `.tif` exige `golang.org/x/image/tiff`. Mesma regra extensão + assinatura do §3.3. |

Licenças a registrar no report: Tesseract Apache-2.0, Leptonica BSD-2-Clause, tessdata Apache-2.0
(confirmar nos arquivos `LICENSE` das fontes).

### 11.9 Testes e aceite do OCR

- Unitários sem DLL: interface `ocrEngine` com implementação falsa; testar marcadores, rodapé,
  confiança baixa, timeout por página, filho que morre (implementação falsa que dá `os.Exit`),
  orçamento de tempo, teto de páginas, teto de megapixels, página branca pulada.
- Protocolo do worker: frame truncado, dimensões absurdas, stride inconsistente → filho recusa sem
  crash; fuzz `FuzzOCRFrame` 30 min.
- Carga da DLL: caminho relativo recusado; hash divergente → OCR desligado com WARNING; DLL
  ausente → idem; servidor sobe normalmente nos três casos.
- stdout limpo: teste que roda o worker real (tag `ocr`) e verifica que **nada** além do frame de
  resposta sai no stdout (Tesseract adora imprimir avisos).
- Ao vivo (`-tags ocr,live`, variável `MCPOLLAMA_TEST_OCR_LIB` apontando a DLL, uma chamada por
  vez): fixture PNG gerada por script versionado (texto com `ç ã õ é` em fonte OFL), conferir
  confiança > 85 e acentos; depois 3 PDFs digitalizados do corpus do spike (fora do git) — report só
  com métricas (páginas, conf média, tempo por página, memória do filho).
- Aceite com o caso real: `summarize_file` num contrato digitalizado da pasta de contratos →
  resumo coerente, rodapé com `paginas_ocr`, e `extract` de "partes, CPF/CNPJ, valor e data" com o
  aviso de verificação presente na resposta. **Você confere 3 valores contra a imagem original** e
  registra só "conferiu / não conferiu".

---

## 12. Estado da execução (atualizado por quem conclui cada etapa)

> **Regra (owner, 2026-10-28 15:02):** cada agente, ao concluir a sua etapa, acrescenta aqui uma linha na tabela e
> marca o checklist do §10. O detalhe completo (números, desvios, pendências) fica no plano de implementação
> `.workspace/plans/MCPOllam2026-10-23-150845-mcpollama-implementacao.plan.md` (seção "Fase 7").
> **Decisões do owner:** D-1 = go-pdfium v1.21.4 (WASM, wazero v1.12.0), fixada após o spike; D-2 = só essa
> dependência; D-3 = `--max-doc-bytes` 32 MiB; D-4 = OCR na 7.9; D-5 = notas e comentários incluídos; D-6 = 0.6.0
> (0.7.0 com OCR); D-13 = teto do `tools/list` em 4.700 bytes; **D-14 (nova)** = cache de compilação do WASM só por
> `--pdf-cache-dir`; **D-15 (nova)** = motor de PDF residente (sem liberação por ociosidade); D-8 = DLL do Tesseract
> 5.3.3 pronta, em `D:\MCP\libtesseract64.dll` + `D:\MCP\tessdata\{best,fast}\`; D-9/D-10 = processo filho, só
> Windows; D-11 = best e fast copiados, padrão após medição; D-12 = **sim**, `.png` e `.jpg` na 7.9.

| Etapa | Estado | Commit(s) | Resumo |
| --- | --- | --- | --- |
| DLLs em `D:\MCP\` | concluída | `07b281c` | `libtesseract64.dll` 5.3.3 (SHA-256 `964B0972…`, importa só KERNEL32/msvcrt) + `tessdata\best` e `\fast` (`por`, `osd`), hashes iguais à origem |
| 7.0 spike PDF | concluída | `3831068` | 35 PDFs (26 com texto): go-pdfium 255.506 runas, 0 U+FFFD, sem perda de página; ledongthuc 138.171, 9,5 U+FFFD/1000, perdeu 15 páginas; cache de compilação: init 0,11–0,15 s e 68–81 MB (sem cache ~5 s e ~330 MB). Report só com métricas |
| 7.1 base `doctext` | concluída | `0f4fb0e` | `Detect` (extensão + assinatura, OLE), ZIP seguro (limites reais, nomes perigosos/duplicados/cifrados), XML seguro (sem DTD, profundidade), `NormalizeText` |
| 7.2 DOCX | concluída | `d3476a8` | `Extract`; corpo, cabeçalho/rodapé sem repetição, notas, comentários, tabelas, revisões aceitas, sem duplicata. Desvio: notas de fim como `[nota de fim N]` (ids colidem com as de rodapé) |
| 7.3 numeração DOCX | concluída | `9914cad` | `numbering.xml`, herança de estilo, reinícios, `startOverride`, decimal/letras/romanos/ordinal/bullet |
| 7.4 PPTX | concluída | `feb5561` | ordem pelo `sldIdLst`, grupos, tabelas, notas, ocultos, layouts/masters ignorados |
| 7.5 PDF | concluída | `c216351` | marcadores `[página N]`, páginas sem texto, senha de abertura recusada e de permissão aberta, hifenização conservadora, `recover`/`ctx`/limites, renderização em cinza pronta para o OCR; FS e stdout do WASM bloqueados |
| fuzz do caminho ZIP | artefato de ambiente | `b9e8fcc` | sem defeito: os períodos em 0 exec/s coincidem com a minimização do motor de fuzz (as execuções só são somadas no fim; até 60 s por entrada, e a cortada pelo tempo perde a contagem); instrumentado com `-parallel 1`, ~2,1 M execuções e pior caso de 29 ms por entrada (com 16 workers, 48 entradas de 0,2–2 s rodam em ≤ 1,05 ms isoladas: contenção). À parte, `Package.Read` passou a pré-dimensionar pelo tamanho declarado (64 MiB: pico 156 → 64 MiB) e recusa antes de descompactar o que declara mais que o limite |
| 7.6 integração | concluída | `a1930e8` | Guard com `DocExtractor` injetado (teto por extensão, `ReadContext`), `--max-doc-bytes`/`--pdf-cache-dir`, rodapé `formato=`/`paginas=`/`slides=`/`revisoes_pendentes=`, descrições mínimas (D-13). Binário 22.507.008 B (era 10.903.552 B); `tools/list` full 7.582→7.762 B e compact 4.291→4.381 B (com `--allow-root`); partida 21 ms; roteiro adversarial da Fase 4 repetido: 227 vetores, 223 BLOQUEADO, 0 ESCAPOU (2 ACHADO e 2 INFO de política, já conhecidos) + 18 vetores novos de documento todos BLOQUEADO |
| 7.7 fuzz 30 min, revisão adversarial, ao vivo, aceite | pendente | — | atenção: `A:` é unidade de rede (`\\WN-DC03\Financeiro`) |
| 7.8 docs + binário 0.6.0 | pendente | — | inclui corrigir no `CLAUDE.md` a toolchain (instalada: go1.27.0) |
| 7.9 OCR + binário 0.7.0 | pendente | — | — |