# Revisão adversarial da Fase 7 (extração de `.docx`, `.pptx` e `.pdf`)

- **Projeto:** mcpollama (`E:\MCPOllam`), servidor MCP em Go
- **Alvo:** `src/doctext/` (doctext, zipsafe, xmlsafe, normalize, docx, docx_numbering, pptx, pdf), `src/files/{guard,policy}.go`, `src/mcpollama-mcp/{docs,config,main,backend,server}.go` e `src/tools/filetools.go`, no estado do commit `3d98ed2` (faixa `0f4fb0e..3d98ed2`)
- **Revisor:** subagente adversarial, modelo Opus 5.5
- **Data:** 2026-10-28, 15:39–16:40 (hora local)
- **Ambiente:** Windows 11 IoT Enterprise 10.0.28000, go1.27.0 windows/amd64, 16 CPUs, 32 GB de RAM (cerca de 5 GB livres, porque o fuzz de 30 min rodava em paralelo noutro worktree e não foi tocado)
- **Restrições respeitadas:**
  - O código de produção de `src/` não foi alterado. A árvore terminou como começou: `git status --short src` está vazio.
  - Nenhum documento de cliente foi usado. `A:\` e `.workspace\spike-pdf\corpus` não foram abertos, e todas as fixtures foram geradas em código.
  - Nenhuma chamada ao Ollama real foi feita (o servidor foi testado com `httptest`).
  - Não houve commit nem fuzz longo.

---

## 1. Veredito

**Não está pronto para o binário 0.6.0 no estado atual.** Não houve **nenhum ESCAPOU**: nenhum texto, metadado, caminho ou mensagem crua do motor vazou, nenhum alvo malicioso de `.rels` foi seguido e o WASM não escreveu em stdout/stderr. Há dois achados **Altos** de disponibilidade, porém, e o brief (§7.5) exige "sem alocação > 512 MiB":

1. **A1:** um `.docx` de **1,2 KB** faz o processo alocar **~1 GiB** (pico de heap de 286–293 MiB). O custo cresce linearmente com o número de parágrafos, e o arquivo continua minúsculo. É o caminho para derrubar o servidor por falta de memória.
2. **A2:** um `.pdf` de **33 KB** fez o processo subir **+2,9 a +5,4 GiB** de memória privada e rodar por **16–29 s**, com `--timeout` de 2 s. O timeout não interrompe uma página, e a memória do WASM não tem teto.

Há ainda **3 achados Médios** de integridade. Em todos, texto **some em silêncio**, que é o pior caso para o consumidor jurídico:

- um campo `fldChar` desbalanceado apaga o resto do documento;
- o conteúdo de `w:altChunk` é descartado;
- uma numeração que falha ao carregar remove **todos** os números de cláusula sem sinal.

Há também **6 achados Baixos**.

**A correção de todos os achados foi aplicada numa cópia de `src` e provada.** Na cópia, os 3 Altos/Médios de recurso deixam de existir:

- DOCX: pico de 0,4–6 MiB;
- PDF: o timeout corta em 2,0 s, com +223–440 MiB;
- todo SILENCIOSO vira recusa tipada ou extração fiel.

A suíte inteira fica verde na cópia, com 1 teste existente ajustado (ver §6). Os diffs estão em `…fase7.testes\correcao-proposta\` e passam em `git apply --check`.

## 2. Números

| Execução | Vetores (`VEC`) | BLOQUEADO | FIEL | INFO | ACHADO | SILENCIOSO | ESCAPOU | Suíte |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Código atual + testes de ataque (`go test -count=1 -v ./...`) | 152 | 74 | 30 | 31 | 7 | 10 | **0** | todos os testes existentes PASS; 3 FAIL, só de ataque (`TestAdvDOCXIntegrity`, `TestAdvDOCXNumbering`, `TestAdvPPTX`); 2 SKIP (`TestAdvPDFContentBomb`, opt-in; `TestDenylistWindowsShortName`, D: sem 8dot3, como na Fase 4) |
| Bomba de conteúdo PDF (rodada explícita, `MCPO_ADV_BOMB=1`, 2 vezes) | 6 | 0 | 0 | 3 | 3 | 0 | 0 | — |
| **Total do código atual** | **158** | **74** | **30** | **34** | **10** | **10** | **0** | — |
| Cópia com todas as correções + os mesmos testes de ataque (`go test -count=1 -v ./...`) | 152 | 74 | 40 | 37 | 1 (`pdf/hifen-encliticos`, rotulado ACHADO sempre; os 3 enclíticos passam a sair corretos) | **0** | **0** | **todos os pacotes `ok`, inclusive os testes de ataque**, com 1 teste existente ajustado (§6); 2 SKIP como acima (`gate-copia-corrigida.log`) |
| Bomba de PDF na cópia corrigida (rodada explícita) | 6 | 0 | 0 | 6 | 0 | 0 | 0 | timeout respeitado em 2,0 s; +223–440 MiB |
| Árvore final, sem os testes de ataque (`go vet` + `go test -count=1 ./...`) | — | — | — | — | — | — | — | **vet 0, todos os pacotes `ok`** (`gate-sem-testes.log`) |

Evidência bruta: `…fase7.testes\gate-com-testes.log`, `…fase7.testes\gate-sem-testes.log` e `…fase7.testes\gate-copia-corrigida.log`.

**Errata (2026-10-28, passo 1 da aplicação):** a 1ª versão deste report somava 18 linhas `VEC|doc/…` (todas BLOQUEADO) que **não são desta revisão**. Elas vêm do teste existente `tools/filetools_docs_test.go` (Fase 7.6) e entraram na contagem porque o gate usou `-v ./...`. Os totais corretos dos vetores desta revisão são **158** (74 BLOQUEADO, 30 FIEL, 34 INFO, 10 ACHADO, 10 SILENCIOSO, 0 ESCAPOU) no código atual e **152** sem a bomba na cópia corrigida. A 1ª versão dizia 176 e 92 BLOQUEADO. Os achados e as severidades não mudam.

## 3. Método

1. Li todo o código novo antes de atacar: `doctext.go`, `zipsafe.go`, `xmlsafe.go`, `normalize.go`, `docx.go`, `docx_numbering.go`, `pptx.go`, `pdf.go`, `guard.go`, `policy.go`, `docs.go` e `filetools.go`, além do brief §1, §4–§7 e §12.
2. Escrevi os testes de ataque como testes internos dos pacotes. Eles reaproveitam os helpers existentes (`buildZip`, `dxBuild`, `nbDocx`, `buildPPTX`, `buildPDF`, `peakHeap`, `newWorld`, `runMain`, `rootWithFiles`):
   - `doctext/fase7_common_adv_test.go`: `vec`, memória privada do processo por `GetProcessMemoryInfo` e um ZIP com entradas sobrepostas escrito à mão;
   - `doctext/fase7_docx_adv_test.go`: integridade DOCX, numeração, amplificação e determinismo;
   - `doctext/fase7_pptx_zip_adv_test.go`: PPTX, ZIP e XML;
   - `doctext/fase7_pdf_adv_test.go`: PDF escrito à mão (JS, Launch, anexo, AcroForm, ciclo, stream `/F` externo, 2001 páginas, bomba de conteúdo), stdout/stderr do WASM, panic e determinismo;
   - `files/fase7_guard_adv_test.go`: guard com extrator falso hostil;
   - `mcpollama-mcp/fase7_e2e_adv_test.go`: guard com o **extrator real**, mensagens de timeout, TOCTOU e servidor ponta a ponta pelo `realMain`, com `--debug`, um fake do Ollama que **grava os prompts** e injeção com marcador forjado.

   O pacote `tools` não ganhou arquivo próprio: o `runFile`, o `wrap` e o nonce foram exercidos ponta a ponta pelo binário.
3. Cada vetor imprime `VEC|<area>/<nome>|<resultado>|<detalhe>`. ESCAPOU e SILENCIOSO falham o teste; as outras categorias só registram.
4. Os sentinelas cobrem corpo, autor de comentário, `docProps`, metadados de PDF, JS, anexo, campo de formulário e texto puro disfarçado de PDF. Toda mensagem de recusa, linha de stderr e prompt enviado foi conferida contra esses sentinelas, contra o caminho da raiz e contra `wazero`, `pdfium`, `wasm`, `panic`, `zip:` e `xml:`.
5. Para as correções, copiei `src` para `scratchpad\advrev7\src` com robocopy, sem `.exe`, e guardei uma cópia intacta em `src-orig`. Apliquei as correções só na cópia. Rodei `go vet`, a suíte inteira sem os testes de ataque e depois os testes de ataque. Os diffs foram gerados por `diff -u` entre as duas cópias.
6. **Gate pedido:** com os testes de ataque em `src`, rodei `go vet ./...` (exit 0) e `go test -count=1 -v ./...` (só os 3 FAIL de ataque). Depois movi os arquivos para `…fase7.testes\<pacote>\` e rodei de novo: vet 0, tudo `ok`, `git status --short src` vazio.

**Erros de processo meus, todos corrigidos antes dos números finais:**

- **(a)** A 1ª versão do vetor de amplificação usava `%1` no próprio nível. O contador avança, e a partir do 2º parágrafo o prefixo virava decimal de 4 dígitos, o que subestimava o efeito. Troquei para `%2` apontando para um nível nunca incrementado, que fica fixo em 64 letras.
- **(b)** O e2e saiu uma vez com `exit=2` porque um teste anterior já tinha subido o motor de PDF no processo, e o `realMain` chama `ConfigurePDF`, que recusa nesse caso. É um artefato de ordem de teste: em produção há um `realMain` por processo. Mitiguei com `ClosePDF()` no início do teste e registrei como INFO.
- **(c)** O vetor de timeout do PPTX usava 3000 slides, acima de `MaxEntries=2000`. A recusa era por limite, não por timeout, e baixei para 1500.
- **(d)** Na cópia, duas edições feitas por script Python falharam por causa de `\\n` dentro do heredoc. Refiz essas duas com a ferramenta de edição. Isso não afetou `src`.
- **(e)** As medições da bomba de PDF variam entre rodadas (+2,9 GiB e +5,4 GiB para o mesmo arquivo; a amostragem é de 2 ms e o GC não é determinístico). Reporto as duas.

## 4. Achados

Severidade:

- **Alto:** derruba ou degrada o servidor com um arquivo pequeno.
- **Médio:** texto some em silêncio num cenário plausível.
- **Baixo:** perda ou erro silencioso só em arquivo forjado ou incomum, ou mensagem errada.
- **Info:** comportamento a decidir ou documentar.

| ID | Sev. | Vetor (`VEC`) | Evidência | Impacto para o consumidor jurídico | Correção proposta (provada na cópia) |
| --- | --- | --- | --- | --- | --- |
| **A1** | **Alto** | `rec/lvltext-amplificacao-*` (4 ACHADO) | `w:lvlText` é atributo (não conta em `MaxTextRunes`) e é copiado em **cada** parágrafo numerado. Dentro de célula, as linhas só contam no teto quando a linha da tabela termina. `.docx` de 1.264 B (32 parágrafos numa célula): pico de heap **285,6–292,8 MiB**, **1.053 MiB alocados**, 2,1–2,3 s, e a recusa TooLong só vem no fim. Escala linear: 8 parágrafos 68–86 MiB, 16 parágrafos 152–162 MiB, 32 parágrafos 286–293 MiB. Na raiz, um `lvlText` de 2 MiB (arquivo de 3.169 B) gera prefixo de 64 Mi runas: pico **157 MiB**, **523 MiB alocados** com **1 parágrafo**. Com o teto de entrada de 64 MiB, 1 parágrafo chega a ~2 Gi runas; centenas de parágrafos numa célula esgotam a RAM (OOM em Go é fatal: o servidor cai). | Um contrato hostil (anexo recebido de terceiro) derruba o servidor MCP para todos os pedidos. | `docx_numbering.go`: `lvlText` com mais de 128 runas cai em `bullet`. `docx.go`: o texto de célula e de nota é cobrado no teto **ao entrar** (`cellRunes`), sem cobrar duas vezes na linha. Na cópia: pico **0,4–6,0 MiB**, 5–96 ms, extração ok. |
| **A2** | **Alto** | `rec/pdf-bomba-conteudo-*` (3 ACHADO) | Página com conteúdo Flate grande, `--timeout`=2 s. **87 KB** (42 MiB de operadores de caminho): **10,8–17,3 s**, **+1,6–2,0 GiB** privados. **33 KB** (16 MiB de `Tj`): **16,3–28,8 s**, **+2,9–5,4 GiB**, e só então `ReasonTimeout`. **29 KB**: 3,7–5,6 s, +0,5–1,0 GiB. O `ctx` só é checado **entre** páginas (`GetPageText` não é interrompível), e o wazero permite 4 GiB de memória linear. A memória volta depois (resíduo de +0,1 a +4 MiB), mas o slot único de PDF fica preso o tempo todo e qualquer outro PDF espera. | Um PDF de dezenas de KB congela a extração de PDF por até ~30 s por chamada e pode empurrar a estação para paginação ou OOM. Repetido, é DoS. | `pdf.go`: `WithMemoryLimitPages(8192)` (512 MiB), `WithCloseOnContextDone(true)` e `context.AfterFunc(ctx, inst.Kill)` do `open` ao `close` (a API `Kill` do go-pdfium foi feita para isso). Na cópia, com timeout de 2 s: **2,0 s**, +223–440 MiB, `ReasonTimeout`. Sem timeout (120 s), o teto recusa: **17,3 s, pico +1,43 GiB** com 512 MiB; com 1 GiB, 24,2 s e +3,2 GiB. O teto precisa ser calibrado no corpus real do spike (não testável por mim). |
| **M1** | Médio | `docx/fldchar-begin-sem-fim`, `docx/fldchar-separate-end-em-del` (2 SILENCIOSO) | `fldChar begin` sem `separate`/`end`, ou com eles dentro de `w:del` (campo excluído pela metade com controle de alterações), deixa `code>0` até o fim da parte. **Todo o texto seguinte some**, sem erro, e o resultado sai como `revisoes_pendentes=nao` no 1º caso. Ex.: "CLAUSULA-TRES" e "CLAUSULA-QUATRO" sumiram (a saída teve 12 runas). | Cláusulas inteiras desaparecem do resumo e da extração sem nenhum sinal. | `docx.go`: no fim da parte, `code>0` vira recusa `ReasonMalformed`. Não dá para encerrar o campo no fim do parágrafo, porque um código de campo pode ter vários parágrafos (`IF`). Na cópia: recusa ruidosa. |
| **M2** | Médio | `docx/altchunk` (SILENCIOSO) | `<w:altChunk r:id=…>` (HTML/RTF/DOCX importado, que o Word exibe) é ignorado. "CLAUSULA-NO-ALTCHUNK" sumiu, sem erro nem rodapé. | Geradores de documento (sistemas que montam contrato a partir de HTML, docx4j, OpenXML PowerTools) usam `altChunk`, e o documento só "vira" WordprocessingML quando alguém o salva no Word. | `doctext.go` + `docx.go`: nova `ReasonAltChunk`, "o documento tem conteúdo importado (altChunk) que não é extraído; abra no Word e salve de novo como .docx". **Decisão do owner:** recusa (proposta) ou extrair o HTML/texto do chunk. |
| **M3** | Médio | `num/numbering-xml-malformado`, `num/numbering-100k-elementos` (2 SILENCIOSO) | Se `numbering.xml` não carrega (XML inválido, ou mais de 100.000 elementos `w:`, o `maxItems` fixo), `loadNumbering` devolve `nil` e **todo** o documento sai sem os números de cláusula, sem sinal. Um `numbering.xml` **válido** de 2,9 MB (50.005 `w:num`, comum em documento colado muitas vezes) perdeu toda a numeração. | O requisito forte §1.3 ("o que diz a cláusula 7?") falha em silêncio. O rodapé não diz nada. | `docx.go` + `docx_numbering.go`: o carregamento devolve erro e o erro vira recusa; `maxItems` vai para 4.000.000 (a memória já é limitada por `MaxEntryBytes`). Exige mudar o teste existente `TestNumberingUnknownAndMissing`, que **afirma** o "ignorar" atual (incluído no diff). **Decisão do owner:** recusa (proposta) ou rodapé `numeracao=indisponivel`. |
| **B1** | Baixo | `docx/rels-alvo-caixa-diferente`, `pptx/notas-alvo-caixa-diferente` (2 SILENCIOSO) | Nome de parte OPC é case-insensitive (o próprio `OpenPackage` já trata duplicata por caixa), mas `Has`/`Read` comparam exato. `Target="Header1.xml"` para a entrada `word/header1.xml` faz o cabeçalho ser **pulado sem sinal**; com notas de slide, as notas somem. Um slide com caixa diferente gera recusa ruidosa. | Timbre e cabeçalho (às vezes com partes e número do contrato) e notas do apresentador somem. | `zipsafe.go`: `lookup` com mapa minúsculo → nome; `docx.go`: `seen` por minúsculas. |
| **B2** | Baixo | `docx/cabecalho-aponta-document-xml` (SILENCIOSO) | Um rel de cabeçalho com `Target="document.xml"` (ou notas) faz o **corpo inteiro sair duas vezes**, a 1ª rotulada `[cabeçalho]` (nem o alvo nem o `Type` são conferidos). | Texto duplicado, contra o requisito §1.6 (só arquivo forjado). | `docx.go`: ignorar alvo igual a `word/document.xml` ou às partes de notas. |
| **B3** | Baixo | `pptx/sldid-duplicado` (SILENCIOSO), `rec/pptx-100k-sldid-mesmo-slide` (INFO) | O mesmo `r:id` duas vezes no `sldIdLst` sai como **2 slides** (`slides=2`) com o mesmo texto. 100.000 `sldId` para 1 slide: `.pptx` de **241 KB** custou **15–24 s** de CPU (cortado pelo `--timeout` por slide). | Slide duplicado e numeração `[slide N]` errada para citação. | `pptx.go`: `r:id` repetido gera recusa (o PowerPoint repara esse arquivo). Na cópia: recusa em 12 ms. |
| **B4** | Baixo | `pptx/ac-sem-choice` (SILENCIOSO) | No PPTX, `mc:Fallback` é **sempre** pulado. Um `AlternateContent` só com Fallback perde o texto (o DOCX, corretamente, lê esse Fallback). | Texto de forma some (arquivo incomum). | `pptx.go`: pular o Fallback só se um Choice já foi lido. |
| **B5** | Baixo | `pptx/deadline-nao-tipado`, `timeout/lenta.pptx` (ACHADO) | `extractPPTX` usa `ctx.Err()` cru, não `ctxErr`. O usuário recebe **"context deadline exceeded"** (em inglês, sem dizer `--timeout`), enquanto DOCX e PDF dão "a extração do texto excedeu o tempo limite (--timeout)". | Mensagem fora do contrato §6 (não vaza nada). | `pptx.go`: `ctxErr(ctx)` nos 2 pontos. Na cópia: mensagem pt-BR. |
| **B6** | Baixo | `pdf/hifen-encliticos` (ACHADO) | A heurística do brief §4.3 (juntar `-\n` + minúscula), nas duas implementações (`joinHyphenation` e `pdfiumSoftHyphens`), apaga hífens reais em quebra de linha: "obriga-se" vira "obrigase", "compromete-se" vira "comprometese", "ex-empregado" vira "exempregado", "pre-contrato" vira "precontrato" (todos **confirmados com o PDFium real**). Enclíticos são muito frequentes em contrato pt-BR. | Citação literal alterada (o sentido é recuperável). | `pdf.go`: `keepHyphen`, que junta a linha mas **mantém o hífen** quando a palavra seguinte é enclítico (se, lhe, o, a, lo, la…) ou a anterior é prefixo que sempre leva hífen (ex, vice, pré, pós, pró, recém…). Na cópia: os 3 primeiros corretos ("pre" sem acento não está na lista). **Decisão do owner** (muda a regra do brief). |

### Info (não são defeitos; registrar ou decidir)

| Vetor | Observação |
| --- | --- |
| `docx/texto-oculto-vanish`, `pptx/forma-oculta`, `pdf/texto-invisivel` | Texto **invisível ao leitor humano** chega ao modelo: `w:vanish`, forma com `hidden="1"`, PDF `Tr 3` e texto branco no branco. É necessário para PDF com OCR embutido, mas é um vetor de injeção que o humano não vê (o `wrap` com nonce segura o marcador). O brief não decide. |
| `pdf/acroform-valor-e-aparencia` | Valor de campo de formulário preenchido, aparência do widget e anotação FreeText **não** são extraídos (o brief §4.3 manda ignorar). Formulário de tribunal preenchido perde os valores **sem sinal no rodapé**. |
| `pptx/smartart` | Texto de SmartArt e gráfico não é extraído (fora do escopo), sem sinal. |
| `docx/rels-alvo-absoluto-opc`, `docx/entrada-word-maiuscula`, `xml/utf16-bom` | Alvo absoluto `/word/header1.xml` (válido no OPC), entrada `Word/Document.xml` e parte XML em UTF-16 **recusam o documento inteiro**. Todas são recusas ruidosas, sem perda silenciosa. |
| `num/percent-10`, `num/ilvl-*` | `%10` vira `%1` seguido de "0" (o 1º item sai "10.", igual ao Word). `ilvl` fora de 0..8 é fixado e o texto é preservado. |
| `docx/merge-continuacao-com-texto`, `pptx/hmerge-com-texto` | Texto em célula absorvida (vMerge/hMerge de continuação) é descartado de propósito. O comportamento exato do Word não foi verificado (sem Word na estação). |
| `docx/marca-paragrafo-excluida`, `docx/linha-excluida-trpr-del`, `docx/nota-aninhada`, `docx/t-fora-de-p` | Diferenças de forma, sem perda de texto, ou só em XML fora do schema. |
| `xml/atributo-30MiB`, `xml/comentario-40MiB` | Pico de 115 MiB e de 151 MiB (até ~4x a entrada). O teto é `MaxEntryBytes`=64 MiB, o que dá ~250 MiB no pior caso. |
| `zip/bomba-sobreposta-12x30MiB` | Recusada (`MaxTotalBytes`), com pico de 60 MiB, mas **4,6–6,6 s** de CPU para um arquivo de 64 KB. |
| `guard/texto-com-cabecalho.pdf` | 30 MB de texto puro começando com `%PDF-`: recusado, mas o PDFium leva **9,4 s** reconstruindo a xref. O texto nunca chega ao modelo. |
| `pdf/hifen-apos-digito-softhyphen` | `pdfiumSoftHyphens` junta também depois de dígito ("30\x02dias" vira "30dias"); `joinHyphenation` exige letra. A regra é inconsistente, mas rara. |
| `guard/mensagem-do-extrator-repassada` | O guard mostra a mensagem do extrator como está (é o contrato). Quem filtra é `userSafe` no binário; os vetores `leak/*` provam que funciona. |
| `leak/autor-comentario-no-prompt` | O autor do comentário vai para o **prompt** (por desenho, brief §4.1). Não vai para log, erro nem rodapé. |
| e2e `exit=2` | `realMain` falha se o motor de PDF já subiu no mesmo processo (`ConfigurePDF`). Só afeta testes. |

## 5. O que foi provado BLOQUEADO / FIEL (resumo)

- **ZIP:**
  - bomba sobreposta (12 nomes, uma entrada local);
  - muitas entradas pequenas acima de `MaxTotalBytes`;
  - entrada que declara 0 bytes;
  - ZIP aninhado (nunca aberto);
  - 9 grafias de nome: duplicata por caixa, `//`, `./`, `/./`, barra final, `\`, `C:`, `..`, NFKC/fullwidth;
  - 14 alvos maliciosos de `.rels`: `%2e%2e`, `..\`, `C:/`, `C:x`, `file:`, `\\server`, `%5c%5c`, `%00`, `http:` interno, `/../`, `%2F..`, `%252e` (só resolve dentro do pacote), `x%00.png`;
  - `External` para `file:///C:/Windows/win.ini` nunca seguido.
- **XML:** entidade sem DTD, DOCTYPE com entidades aninhadas, DOCTYPE depois da raiz, `&#0;`, encoding latin1, profundidade 10.000 e texto de 20 MiB, todos recusados. CDATA é lido como texto (fiel).
- **DOCX:**
  - `AlternateContent` aninhado, sem Choice e com 2 Choice: texto exatamente 1 vez;
  - `sdt` aninhado (bloco e linha);
  - campos aninhados, `end` em `w:del` e `end`/`separate` órfãos;
  - `ins` em `del`, `del` em `ins`, `moveFrom`/`moveTo`, com `revisoes_pendentes` correto;
  - hyperlink, `fldSimple`, `smartTag`, `customXml`, `dir`/`bdo`;
  - tabelas aninhadas em 3 níveis, com ordem de leitura;
  - caixa de texto em cabeçalho;
  - notas com id repetido e comentário com resposta;
  - ciclos de `styleLink`/`numStyleLink`/`basedOn` sem travar.
- **PDF:**
  - JavaScript, `Launch`, anexo embutido e metadados (`/Author`, `/Title`, `/Subject`) **não** chegam ao texto;
  - árvore de páginas cíclica e "kid de si mesma" são recusadas, sem travar;
  - stream com `/F` apontando para arquivo do disco não é lido (o WASM não tem FS);
  - 2001 páginas são recusadas em 11 ms;
  - páginas mistas dão `paginas_sem_texto=1`;
  - PDF truncado é recusado;
  - **o WASM não escreve em stdout/stderr**: inicialização do motor + 5 PDFs (bons, hostis e malformados) + renderização deram 0 B e 0 B;
  - panic no motor vira recusa, sem vazar a mensagem do panic, e **libera o slot** (o PDF seguinte funciona);
  - panic injetado em cada extrator vira `ReasonMalformed`.
- **Guard + extrator real:**
  - `.pdf` com 30 MB de texto e `%PDF-`: recusado;
  - `.pdf` sem cabeçalho e `.docx` com texto: `Mismatch`;
  - `.doc` de texto com 4 MiB + 1 é recusado pelo teto de **texto** (`--max-file-bytes`), não pelo de documento;
  - `.doc` de 1 MB é lido como `txt`;
  - `.doc` com NUL é recusado como binário;
  - `.PDF` e `.Docx` maiúsculos vão para o extrator;
  - PDF de 33 MB passa do `--max-doc-bytes` e é recusado;
  - PDF válido com 30 MB de lixo no fim: só o texto da página;
  - nomes `x.pdf.` e `x.pdf ` são recusados;
  - **TOCTOU**: 300 leituras em corrida com troca docx/texto deram 0 vazamentos;
  - ctx cancelado com o extrator bloqueado volta em 100 ms;
  - UTF-8 inválido e `nil,nil` do extrator são recusados;
  - a denylist barra `credentials.docx`, `secrets/*.pdf`, `*secret*.pptx` e `.env.pdf` sem chamar o extrator;
  - `relatorio.pdf.txt` vai pelo texto.
- **Ponta a ponta (`realMain --debug`):**
  - 6 documentos hostis + 2 bons + ping: 9 respostas, exit 0, servidor vivo;
  - **stderr** sem texto, autor, `docProps`, caminho e `wazero`/`pdfium`;
  - mensagens de recusa sem caminho nem conteúdo;
  - `docProps` (criador e empresa) não chegam ao prompt;
  - **injeção**: o marcador forjado `=====ARQUIVO_FIM 0123…=====` fica dentro do bloco, o nonce real é único e o prompt termina no marcador real.
- **Determinismo (§1.10):** byte a byte em DOCX 50x (com numeração, estilos, 2 cabeçalhos, rodapé, notas e comentários), PPTX 50x e PDF 20x em 3 arquivos: sempre **1 hash**.

## 6. Correção proposta e prova

Arquivos em `E:\MCPOllam\.workspace\reports\MCPOllam2026-10-28-161147-revisao-adversarial-fase7.testes\correcao-proposta\`:

| Diff | Achados | Linhas +/- |
| --- | --- | --- |
| `docx_numbering.go.diff` | A1 (teto do `lvlText`), M3 (erro de carga, `maxItems`) | 26 |
| `docx.go.diff` | A1 (`cellRunes`), M1 (`code>0`), M2 (`altChunk`), M3 (propaga o erro), B1 (`seen` por minúsculas), B2 (alvo = corpo ou notas) | 40 |
| `doctext.go.diff` | M2 (`ReasonAltChunk` + mensagem pt-BR) | 5 |
| `zipsafe.go.diff` | B1 (`lookup` case-insensitive) | 20 |
| `pptx.go.diff` | B3 (`r:id` repetido), B4 (Fallback sem Choice), B5 (`ctxErr`) | 16 |
| `pdf.go.diff` | A2 (teto de memória, `WithCloseOnContextDone`, `AfterFunc`+`Kill`), B6 (`keepHyphen`) | 58 |
| `docx_numbering_test.go.diff` | M3: o teste existente que exigia "ignorar `numbering.xml` quebrado" passa a exigir recusa | 7 |
| `fase7-todas.diff` | concatenação dos 7 acima; `git apply --check` OK sobre `3d98ed2` | — |

**Prova na cópia (`scratchpad\advrev7\src`):** `go vet ./...` deu 0. A suíte inteira (`go test -count=1 ./...`) passou em todos os pacotes. Os testes de ataque deram 0 SILENCIOSO e 0 ESCAPOU, com os números de recurso das colunas da tabela §4.

**Ressalvas:**

1. O teto de memória do WASM (512 MiB) **não foi calibrado com PDFs reais grandes**: o corpus do spike é de clientes e estava fora do meu escopo. Rode o corpus do spike com o teto antes de fixá-lo; se algum PDF legítimo for recusado, suba para 16384 páginas (1 GiB).
2. O pico de memória do processo fica em ~2,8x o teto do WASM. Isso porque o wazero realoca ao crescer, e o texto ainda existe também do lado Go.
3. M2, M3 e B6 mudam comportamento definido pelo brief ou por teste existente: são **decisões do owner** (alternativas no §4).

## 7. O que NÃO foi possível testar

1. **O limite de 4 GiB do WASM sem o teto:** o que acontece quando o PDFium esgota a memória linear (trap, erro ou crash). A estação tinha ~5 GB livres com o fuzz rodando em paralelo, e forçar isso arriscaria a máquina e o fuzz.
2. **O comportamento do Word real** em `fldChar` desbalanceado, texto em célula de continuação, `AlternateContent` só com Fallback e `sldId` duplicado: não há Word ou PowerPoint na estação. A classificação usa a especificação e o comportamento conhecido.
3. **Calibração do teto de memória do PDF no corpus real** (documentos de cliente, proibidos aqui).
4. **`-race`** nos testes de ataque: não rodado, por tempo. Nenhum teste novo é concorrente, exceto o TOCTOU, que é de processo.
5. **Linux:** o binário Linux não foi exercitado.
6. **Fuzz:** é atribuição do outro agente (30 min por alvo, em paralelo). Não houve interferência.
7. **Ollama real:** proibido pela tarefa. O `httptest` grava os prompts.

## 8. Recomendação antes do binário 0.6.0

1. **Obrigatório:** A1 e A2. Aplicar `docx_numbering.go.diff`, a parte `cellRunes` de `docx.go.diff` e `pdf.go.diff`, e calibrar o teto do WASM no corpus do spike. Sem isso, o critério do brief §7.5 ("sem alocação > 512 MiB") e o `--timeout` (§6) não valem para entrada hostil.
2. **Fortemente recomendado:** M1 (só recusa, sem decisão) e B1, B2, B3, B4, B5 (baratos e já provados).
3. **Decisão do owner** (usar AskUserQuestion): M2 (recusar `altChunk` ou extrair), M3 (recusar ou rodapé `numeracao=indisponivel`), B6 (manter o hífen de enclíticos e prefixos), e se texto invisível/oculto deve ir para o modelo (Info).
4. Depois de aplicar, repetir o roteiro: copiar `…fase7.testes\<pacote>\*.go` de volta para `src\<pacote>\`, rodar `go test -count=1 -v -run TestAdv ./doctext ./files ./mcpollama-mcp` (e a bomba com `MCPO_ADV_BOMB=1`) e conferir 0 SILENCIOSO e 0 ESCAPOU. Depois, retirar os arquivos de novo.
