# Fase 7 (extração .docx/.pdf) - testes ao vivo e aceite com o caso real

Data: 2026-10-28 (execução 15:43-15:50, hora local). Binário: HEAD `ce8d248` compilado em caminho temporário
(22.507.008 B), com `--pdf-cache-dir` próprio. Servidor Ollama `qwen3-ia`, modelo já residente (load_ms 0-1 em todas as chamadas).
Uma chamada por vez, ~1,2 s entre chamadas; 0 respostas HTTP 429. **Ressalva de ambiente:** um fuzz pesado rodava em paralelo
na máquina; os tempos de parede podem estar inflados (a parcela de extração local é pequena, ver abaixo, então o efeito é limitado).

Sigilo: documentos de clientes; identificadores neutros (cNN = corpus de PDFs, dNN = DOCX, doc01 = .doc). Nenhum corpo de
resumo/extração foi impresso ou gravado em arquivo; nenhum nome de arquivo original, parte, CPF/CNPJ ou valor consta aqui.
O token foi lido do `.env` em memória e passado por `--api-key`; saída do driver com scrub.

## 1. Chamadas ao vivo (etapa 1, raiz `.workspace/spike-pdf`, cache de PDF vazio no início)

Colunas: P = prompt_eval_count, E = eval_count, T = total_ms (Ollama), parede = tempo até a resposta no driver,
extra = parede - T (extração local + leitura + sobrecarga). Todas com `partes=1 chamadas=1 omitidas=0 fim=stop`.

| # | id | tool | fmt | bytes | caracteres | páginas | sem_texto | rev_pend | P | E | T ms | load | queue | parede ms | extra ms |
| - | -- | ---- | --- | ----- | ---------- | ------- | --------- | -------- | - | - | ---- | ---- | ----- | --------- | -------- |
| 1 | c02 | summarize_file | pdf | 372.964 | 4.119 | 2 | 0 | - | 2.368 | 469 | 6.393 | 0 | 0 | 15.338 | 8.945 (1ª chamada de PDF, init do motor com cache vazio) |
| 2 | c04 | summarize_file | pdf | 5.763.896 | 12.277 | 12 | 0 | - | 5.228 | 235 | 4.070 | 0 | 0 | 4.203 | 133 |
| 3 | c04 | extract (JSON array) | pdf | 5.763.896 | 12.277 | 12 | 0 | - | 5.259 | 57 | 1.793 | 0 | 0 | 1.884 | 91 |
| 4 | c24 | summarize_file | pdf | 110.924 | 22.975 | 9 | 0 | - | 7.392 | 467 | 8.320 | 0 | 0 | 8.468 | 148 |
| 5 | c22 | extract (JSON array) | pdf | 69.021 | 8.746 | 2 | 0 | - | 3.236 | 284 | 4.296 | 0 | 0 | 4.387 | 91 |
| 6 | d01 | summarize_file | docx | 84.373 | 21.774 | - | - | não | 6.624 | 449 | 7.617 | 0 | 0 | 7.683 | 66 |
| 7 | d02 | summarize_file | docx | 109.519 | 21.843 | - | - | não | 6.642 | 380 | 6.711 | 0 | 0 | 6.825 | 114 |
| 8 | d01 | extract (JSON {numero,titulo}) | docx | 84.373 | 21.774 | - | - | não | 6.657 | 285 | 5.275 | 1 | 0 | 5.340 | 65 |

Recusas (tool_error, mensagem opaca, sem caminho; tempo de parede):

| id | tool | caso | mensagem (pt-BR, conforme Brief §6) | parede ms |
| -- | ---- | ---- | ----------------------------------- | --------- |
| c07 (9 pág.) | summarize_file | PDF digitalizado | "o PDF não tem camada de texto (provavelmente é digitalizado); OCR ainda não é suportado" | 26 |
| c03 (1 pág.) | extract | PDF digitalizado | idem | 70 |
| c01 | summarize_file | PDF com senha de abertura | "PDF protegido por senha não é suportado; gere uma cópia sem senha" | 7 |
| c01 | extract | PDF com senha de abertura | idem | 9 |

### Motor de PDF: 1ª chamada com e sem o cache de compilação do WASM

Mesma chamada (c02, summarize_file, 2 páginas), servidor novo a cada execução; "extra" = parede - total_ms do Ollama.

| Execução | `--pdf-cache-dir` | parede ms | total_ms | extra ms |
| -------- | ----------------- | --------- | -------- | -------- |
| A (1ª chamada de PDF) | vazio (preenchido por esta chamada) | 15.338 | 6.393 | 8.945 |
| B | preenchido | 7.923 | 7.314 | 609 |
| C | ausente | 12.829 | 5.153 | 7.676 |

Leitura: o cache reduz o custo de partida do motor de ~7,7-8,9 s para ~0,6 s (consistente com o spike: ~5 s sem cache,
0,11-0,15 s com cache; aqui com a máquina carregada pelo fuzz e com a extração inclusa). Depois da 1ª chamada o extra por chamada
de PDF fica em 91-148 ms (inclui extração de até 12 páginas de 5,7 MB).

## 2. Aceite §7.7 com o caso real

Raiz `--allow-root "A:\Monitor\CSL Telecom\Contratos"` (unidade de rede `\\WN-DC03\Financeiro`, caminho com espaços).

**Pré-condição: o `NewGuard` aceita a raiz de rede? SIM.** O servidor subiu ("file tools enabled: 1 allowed root(s)") e leu
arquivos da raiz; nada foi afrouxado. Nenhum arquivo foi copiado para fora, alterado ou apagado em `A:`.
Os dois DOCX e os PDFs da etapa 1 foram copiados para `.workspace/spike-pdf/` (ignorado pelo git: `git check-ignore` confirmado);
os mapas ficam só em `mapa-docx.txt` e `mapa.txt` (locais).

| Item | Resultado | Evidência (só números) |
| ---- | --------- | ---------------------- |
| 1. `summarize_file` em .docx de contrato com numeração automática (d01) | PASSOU (execução) | formato=docx, 21.774 caracteres, revisoes_pendentes=não, P 6.620 / E 377 / T 6.554 ms, parede 7.025 ms. Que o resumo cite as cláusulas pelo número certo não foi conferido por leitura (sigilo); o texto entregue ao modelo teve a numeração conferida no item 2 |
| 2. `extract` "liste as cláusulas com número e título" (d01, JSON {numero,titulo}) | PASSOU (execução); **numeração conferida: 83 de 83** | P 6.657 / E 285 / T 5.280 ms, parede 5.399 ms; conferência abaixo |
| 3. `summarize_file` em PDF com texto (c24, 9 páginas) | PASSOU | `paginas=9` = contagem independente do PyMuPDF (9); 22.975 caracteres; P 7.390 / E 508 / T 8.993 ms, parede 9.967 ms |
| 4. PDF digitalizado (c07, 9 páginas sem texto) | PASSOU | recusa do Brief §6 (mensagem acima), parede 552 ms (inclui leitura em rede) |
| 5. `.doc` (doc01, contêiner OLE) | PASSOU | "formato Word/PowerPoint 97-2003 não é suportado; salve como .docx ou .pptx", parede 122 ms |

Confirmações de `paginas=` com contagem independente (PyMuPDF `page_count`): c02 2=2, c04 12=12, c24 9=9, c22 2=2
(4 de 4); para os digitalizados c07 e c03 o PyMuPDF também encontra 0 caracteres de texto (9 e 1 páginas).

### Conferência da numeração (2ª via independente)

Método: Word 16 (Office16) via COM, invisível, `Documents.Open(path, ReadOnly=true)`, sobre as cópias locais de d01 e d02;
coleta de `Range.ListFormat.ListString` e `ListType` de cada parágrafo (d01: 305 parágrafos; d02: 304). O extrator foi rodado
direto (`doctext.Extract`, programa Go temporário fora do git em `.workspace/spike-pdf/numchk/`) e o prefixo de cada
parágrafo numerado foi comparado automaticamente (alinhamento pelo texto do parágrafo, em ordem; nada foi lido nem impresso).

| Documento | Parágrafos numerados no Word | Números (não marcador) | Conferiu | Divergiu | Marcadores | Marcador emitido como "- " | Numerado sem texto |
| --------- | ---------------------------- | ---------------------- | -------- | -------- | ---------- | -------------------------- | ------------------- |
| d01 | 103 | 83 | **83 de 83** | 0 | 20 | 20 de 20 | 0 |
| d02 | 104 | 84 | 83 de 84 | 0 | 20 | 20 de 20 | 1 |

- Formatos de rótulo presentes e conferidos: inteiro simples (11/12), `N.N.` em 2 níveis (66), letra entre parênteses (2),
  romano entre parênteses (4), mais 20 marcadores por documento.
- Tipos de divergência: **nenhum de número.** As 40 diferenças brutas (20 por documento) são todas marcadores: o Word devolve o glifo
  da fonte de símbolo ("o" ou caractere privado) e o extrator emite `- ` conforme Brief §1.3 (`bullet` vira `- `); conversão esperada.
  O único "não conferido" de d02 é um parágrafo numerado vazio (só rótulo, sem texto), sem texto para alinhar; não é divergência.
- Revisões pendentes: 0 nos dois (Word e extrator coincidem: `revisoes_pendentes=não`).
- Limitação do método: o alinhamento é por texto, então um parágrafo de texto repetido poderia, em tese, casar com o prefixo de outra
  ocorrência; o resultado 83/83 torna isso improvável.
- Incidente de execução (meu script, não do produto): `Application.Quit(0)` falhou por passagem de argumento no PowerShell 5.1; os dois
  processos WINWORD iniciados por este script (nenhum havia antes) foram encerrados com `Stop-Process`; 0 WINWORD restantes; os arquivos
  foram abertos somente leitura e nada foi salvo.

## 3. Observações e defeitos

1. **Nenhum defeito de produção encontrado** nesta rodada (nenhuma recusa indevida, `paginas=` correto, numeração 83/83, rodapé
   `formato=`/`paginas=`/`paginas_sem_texto=`/`revisoes_pendentes=` presente).
2. Observação (comportamento atual, não regressão da Fase 7): a linha `[arquivo]` devolve `nome=<nome original do arquivo>`; em acervo de
   clientes isso leva o nome do arquivo ao contexto de quem chama. Se não for desejado, avaliar emitir só extensão/identificador. Registrado para o owner, sem ação.
3. Não há `.pptx` no acervo de contratos (0 dos 82 arquivos Office); PPTX ao vivo só por testes sintéticos da 7.4. Não é possível
   testar PPTX com caso real nesta pasta.
4. Os resumos/extrações usaram modelo residente (load_ms 0-1); nenhuma chamada pagou carga a frio.
5. Os `caracteres` do servidor ficam ~1-2% abaixo do spike (4.119 x 4.226 em c02; 12.277 x 12.335 em c04; 22.975 x 23.230 em c24) por causa da
   normalização; coerente, sem perda de página.

## 4. Artefatos locais (todos fora do git)

Driver `acceptdrv` e binário temporário no scratchpad da sessão; `.workspace/spike-pdf/docx/d01.docx`, `d02.docx`, `mapa-docx.txt`,
`numchk/` (programa Go, texto extraído e JSON do Word com conteúdo de cliente; apagar quando não forem mais necessários).
Nada gravado em `D:\MCP\`; sem commit; `src/` não foi alterado por esta execução.

## Rodada 2 — documentos do owner (o01…)

Pedido do owner: fechar a lacuna de PPTX real com arquivos de `D:\Users\claiton.linhares\Documents` (somente leitura; **a pasta
não foi usada como `--allow-root`**; só os 9 arquivos abaixo foram copiados, por caminho explícito, para
`.workspace/spike-pdf/owner-docs/`, ignorado pelo git — `git check-ignore` confirmado; mapa só em `mapa-owner.txt`).
Sigilo igual ao da rodada 1: só métricas, identificadores neutros, nenhum trecho nem nome de arquivo original.

| id | formato | bytes | observação do arquivo |
| -- | ------- | ----- | --------------------- |
| o01 | pptx | 1.276.664 | apresentação pequena |
| o02 | pptx | 43.100.197 | acima do teto de 32 MiB do servidor |
| o03 | docx | 44.144 | especificação, com numeração automática |
| o04 | docx | 35.520 | documento sem texto algum |
| o05 | pdf | 3.618.342 | livro, 550 páginas |
| o06 | pdf | 2.944.455 | tabelas, 338 páginas |
| o07 | pdf | 208.828 | 15 páginas |
| o08 | pdf | 11.298.580 | digitalizado |
| o09 | pdf | 4.480.069 | 14 páginas (acrescentado para ter um 4º PDF de texto de origem distinta, já que o08 é digitalizado) |

### R2.1 Extração direta (`doctext.Extract`, 2 execuções por arquivo, mesmo SHA-256)

Programa Go temporário fora do git (`.workspace/spike-pdf/extall/`). Cache de compilação do PDF preenchido.

| id | formato | slides | páginas | sem texto | rev. pendentes | runas | t1 ms | t2 ms | determinismo (2 SHA iguais) |
| -- | ------- | ------ | ------- | --------- | -------------- | ----- | ----- | ----- | --------------------------- |
| o01 | pptx | 9 | - | - | - | 1.965 | 7 | 10 | sim |
| o02 | pptx | 150 | - | - | - | 351.221 | 1.541 | 1.439 | sim |
| o03 | docx | - | - | - | **sim** | 17.285 | 37 | 36 | sim |
| o04 | docx | - | - | - | não | 0 | 0 | 0 | sim |
| o05 | pdf | - | 550 | 1 | - | 1.147.970 | 4.909 | 4.264 | sim |
| o06 | pdf | - | 338 | 0 | - | 348.279 | 4.644 | 5.369 | sim |
| o07 | pdf | - | 15 | 0 | - | 30.583 | 152 | 158 | sim |
| o08 | pdf | - | recusado | - | - | - | - | - | recusa "sem camada de texto" (digitalizado, 12 páginas pelo PyMuPDF) |
| o09 | pdf | - | 14 | 0 | - | 18.013 | 1.887 | 1.414 | sim |

Determinismo: 8 de 8 extrações bem-sucedidas com SHA-256 igual nas duas execuções. O extrator direto não aplica o teto de 32 MiB
(do servidor): o02 extraiu normalmente (150 slides, 1,5 s).

### R2.2 PPTX — 2ª via independente (PowerPoint COM, somente leitura, janela oculta)

`Presentations.Open(path, ReadOnly=true, WithWindow=false)`; por slide: `SlideShowTransition.Hidden`, notas (placeholder de corpo da
página de notas) e contagem de palavras das formas, inclusive tabelas (todas as células) e grupos (recursivo).

| Critério | o01 | o02 |
| -------- | --- | --- |
| `Slides.Count` x nosso `slides=` | 9 = 9 | 150 = 150 |
| blocos `[slide N]` nossos | 9 | 150 |
| slides ocultos PowerPoint x marcadores `[slide N — oculto]` | 0 = 0 | 1 = 1 (mesmo slide) |
| slides com notas não vazias x `[notas do slide N]` | 0 = 0 | 144 = 144 (mesmo conjunto) |
| palavras por slide, razão nossa/PowerPoint | média 1,000; 9 de 9 slides com razão 1,00 | média 1,000; 146 slides com texto, todos 1,00 (mín 1,00, máx 1,00); 4 slides vazios nos dois |
| slides fora de ±10% | 0 | 0 |
| total de palavras (PowerPoint) | 292 | 10.883 |

Resultado PPTX real: **passou, sem divergência** (inclusive oculto e notas). Observação: o PowerPoint deixou o processo POWERPNT vivo
após `Quit`; encerrei com `Stop-Process` só o processo aberto por mim (nenhum existia antes; 0 restantes). Abertura do o02 (43 MB) levou ~71 s no PowerPoint.

### R2.3 DOCX — 2ª via independente (Word COM, somente leitura)

| Critério | o03 | o04 |
| -------- | --- | --- |
| parágrafos no Word | 449 | 1 |
| palavras (`ComputeStatistics(0)`) x nossas (split por espaço) | 2.449 x 2.539 (razão 1,037) | 0 x 0 |
| numeração: números (não marcador) com texto | **22 de 22** conferem (sequência do corpo idêntica à do Word) | - |
| marcadores emitidos como `- ` | 3 de 3 | - |
| numerados sem texto | 3 (parágrafos vazios, só rótulo) | - |
| revisões: Word `Revisions.Count` x nosso `revisoes_pendentes` | 0 x **sim** (divergência) | 0 x não |

- Razão de palavras 1,037: nossa saída inclui marcadores de bloco e rodapé; coerente.
- Numeração: um primeiro alinhamento ingênuo por texto deu 4 "divergências" (1.1 x 1.2 …). Eram artefato do método: o documento tem um bloco
  tipo sumário (linhas curtas, texto digitado com resultado de campo, 39 linhas) cujos números digitados (ex.: 1.2) diferem da
  numeração viva da lista; o extrator preserva fielmente o resultado de campo, como o Brief §1.6 manda. Comparando a **sequência de rótulos
  do corpo** (a partir do fim do bloco tipo sumário) com a sequência de `ListString` do Word: **22 = 22, listas idênticas.**
- **Divergência real (menor): `revisoes_pendentes=sim` em o03, mas o Word informa 0 revisões.** Causa (sem conteúdo): existe um único elemento
  `<w:ins .../>` vazio dentro de `<w:numPr>` de um parágrafo do título (marcador legado de alteração de numeração de um modelo da Microsoft,
  autor "ms", data de 1999). O Word não o conta como revisão; o extrator o conta como revisão pendente (a causa exata no código não foi investigada). Efeito: falso positivo do sinal
  "contrato não fechado" do Brief §1.5, em documento sem revisão real. Severidade baixa (erra para o lado conservador); sugestão: não sinalizar
  `w:ins`/`w:del` que sejam filhos de `w:numPr`/`w:rPr` sem conteúdo (marca de parágrafo) ou só sinalizar `w:ins` que contenha `w:r`.
  Não alterei código (`src/` intocado).
- o04: documento vazio (1 parágrafo vazio no Word, 0 palavras; extrator devolve 0 runas) e o servidor recusa com "o arquivo está vazio; nada a processar".

### R2.4 PDF — 2ª via (PyMuPDF `page_count`)

| id | nosso `paginas=` | PyMuPDF | sem texto (nosso x PyMuPDF) |
| -- | ---------------- | ------- | --------------------------- |
| o05 | 550 | 550 | 1 x 1 |
| o06 | 338 | 338 | 0 x 0 |
| o07 | 15 | 15 | 0 x 0 |
| o09 | 14 | 14 | 0 x 0 |
| o08 | recusado | 12 (12 sem texto) | coerente com a recusa |

4 de 4 PDFs de texto conferem em páginas e em páginas sem texto; o digitalizado foi recusado corretamente.

### R2.5 Ao vivo (servidor com `--allow-root` só da pasta `owner-docs`, uma chamada por vez, ~1,2 s entre elas)

Mesmas ressalvas de ambiente da rodada 1 (fuzz em paralelo). `load_ms` 0 em todas (16 ms na maior).

| id | tool | formato | bytes | caracteres | partes / chamadas / omitidas | slides / páginas / sem texto / rev. | P | E | T ms | parede ms |
| -- | ---- | ------- | ----- | ---------- | ---------------------------- | ----------------------------------- | - | - | ---- | --------- |
| o01 | summarize_file | pptx | 1.276.664 | 1.965 | 1 / 1 / 0 | slides=9 | 820 | 268 | 3.443 | 3.571 |
| o03 | summarize_file | docx | 44.144 | 17.285 | 1 / 1 / 0 | revisoes_pendentes=sim | 5.313 | 449 | 7.008 | 7.074 |
| o07 | summarize_file | pdf | 208.828 | 30.583 | 1 / 1 / 0 | paginas=15, sem texto 0 | 8.417 | 610 | 13.646 | 14.143 |
| o05 | summarize_file | pdf (livro) | 3.618.342 | 1.147.970 | **10 / 11 / 0** | paginas=550, sem texto 1 | 255.690 | 5.449 | 187.542 | 192.815 |
| o02 | summarize_file | pptx | 43.100.197 | - | - | recusa por tamanho (teto 33.554.432 B, `--max-doc-bytes`) | - | - | - | 2 |
| o04 | summarize_file | docx | 35.520 | - | - | recusa "o arquivo está vazio; nada a processar" | - | - | - | 1 |

- O livro de 550 páginas (1,15 M de caracteres) foi resumido em map-reduce, 10 partes, 11 chamadas ao modelo, 0 omitidas (teto de 40 chunks), 192,8 s
  de parede, sem estourar o `--timeout` padrão de 120 s (que vale por chamada ao Ollama, não pela tool inteira).
- Extra de extração local (parede - T) para o05: 5,3 s (inclui 550 páginas de PDF); o01: 128 ms; o03: 66 ms; o07: 497 ms.
- Rodapé `[arquivo]` do PPTX traz `formato=pptx slides=9` (sem `revisoes_pendentes`), como no brief.

### R2.6 Resumo da rodada 2

- PPTX real: **passou** (slides 9=9 e 150=150; oculto 1=1; notas 144=144; razão de palavras 1,000 em todos os slides com texto).
- DOCX: numeração 22 de 22 no corpo; **1 divergência real menor** (`revisoes_pendentes=sim` com Word 0 revisões, causa: `w:ins` vazio em `w:numPr`).
- PDF: 4 de 4 páginas conferem; digitalizado recusado; livro de 550 páginas resumido ao vivo.
- Determinismo: 8 de 8. Recusas ao vivo corretas: tamanho (o02) e vazio (o04).
- Artefatos locais com conteúdo (fora do git): `.workspace/spike-pdf/owner-docs/`, `owner-out/`, `extall/`, `mapa-owner.txt` (apagar quando não forem mais necessários).
  Sem commit; `src/` intocado; nada em `D:\MCP\`.
