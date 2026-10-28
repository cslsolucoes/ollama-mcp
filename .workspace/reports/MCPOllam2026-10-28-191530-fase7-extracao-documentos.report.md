# MCPOllam — Fase 7: extração de documentos (`.docx`, `.pptx`, `.pdf`) + OCR — REPORT FINAL

**Data:** 2026-10-28 · **Versão entregue:** 0.7.0 (`0.7.0-9f09aaa`) · **Escopo:** commits `07b281c^..HEAD` (`eda7664`)
**Para:** o owner e a IA que consome `summarize_file`, `extract` e `translate` em trabalho jurídico.
**Sigilo:** este report só traz métricas e ids neutros (c01…, o01…, d01…). Nenhum conteúdo, nome de arquivo de cliente, CPF/CNPJ ou valor consta aqui.
**Fontes:** brief da Fase 7 (§10 e §12), plano de implementação (seção "Fase 7"), reports de spike, ao vivo/aceite, revisão adversarial e OCR, e `git log`. Onde uma fonte discorda de outra, a divergência está marcada.

---

## 1. Resumo e estado final

**Em uma linha:** as três tools de arquivo agora leem `.docx`, `.pptx`, `.pdf` (com texto) e, com OCR ligado, PDF digitalizado e imagens `.png`/`.jpg`, sem afrouxar nenhuma defesa do guard da Fase 4 e sem cgo.

**O que já funciona nos clientes (estado em 2026-10-28):**

| Item | Estado |
| --- | --- |
| Binário em `D:\MCP\mcpollama-mcp.exe` | **0.7.0-9f09aaa** (23.032.320 B; `CGO_ENABLED=0`, `-trimpath`, gerado em worktree limpo no `9f09aaa`) |
| `--pdf-cache-dir D:\MCP\cache\pdfium` | ligado nos `.mcp.json` do **MCPOllam** e do **GestorERP** (cache de 22 MB, preenchido na 1ª chamada de PDF) |
| OCR (`--ocr-lib` + `--ocr-lib-sha256`) | ligado nos `.mcp.json` do **MCPOllam** e do **GestorERP** (DLL Tesseract 5.3.3, modelos `best`; 14 args) |
| Validação real pelo Claude Code (`claude -p`) | DOCX, PDF, PPTX e OCR (o08, 12 páginas) OK nos 2 projetos; tool longa aceita (ver §4.8) |
| Clientes já rodando | continuam com o binário anterior até **reiniciar** (ver pendências do owner, §6) |

**Importante para quem consome as tools:** o texto de PDF digitalizado vem de OCR e pode errar números, datas, nomes e CPF/CNPJ. A resposta traz sempre um `[aviso]` nesses casos. Tabelas de PDF e tabelas sombreadas no OCR não têm garantia (§5).

---

## 2. O que foi feito, por etapa

| Etapa | Estado | Commit(s) | Resumo |
| --- | --- | --- | --- |
| Decisões + DLLs | concluída | `07b281c` | `libtesseract64.dll` 5.3.3 (SHA-256 `964B0972…`, 5.965.824 B, importa só KERNEL32/msvcrt) e modelos `tessdata\best` e `\fast` (`por`, `osd`) copiados para `D:\MCP\`, com hash igual ao da origem |
| 7.0 spike PDF | concluída | `3831068` | 35 PDFs; go-pdfium WASM x ledongthuc; D-1 fixada; report só com métricas |
| 7.1 base `doctext` | concluída | `0f4fb0e` | `Detect` (extensão e assinatura, OLE), ZIP seguro, XML seguro, `NormalizeText` |
| 7.2 DOCX | concluída | `d3476a8` | corpo, cabeçalho/rodapé sem repetição, notas, comentários, tabelas, revisões aceitas, sem duplicata |
| 7.3 numeração DOCX | concluída | `9914cad` | `numbering.xml`, herança de estilo, reinícios, `startOverride`, decimal/letras/romanos/ordinal/bullet; `FuzzDOCX` |
| 7.4 PPTX | concluída | `feb5561` | ordem pelo `sldIdLst`, grupos, tabelas, notas, ocultos, layouts/masters ignorados |
| 7.5 PDF | concluída | `c216351` | marcadores `[página N]`, páginas sem texto, senha de abertura recusada e de permissão aberta, `recover`/`ctx`/limites, render em cinza pronto para OCR; FS e stdout do WASM bloqueados |
| fuzz do ZIP | artefato de ambiente | `b9e8fcc` | os "travamentos" eram a minimização do motor de fuzz; de quebra, `Package.Read` pré-dimensiona o buffer e recusa antes de descompactar |
| 7.6 integração | concluída | `a1930e8` | Guard com `DocExtractor` injetado, `--max-doc-bytes`, `--pdf-cache-dir`, rodapé ampliado, descrições mínimas (D-13), `TOOLS.md` |
| ajuste do budget | concluída | `3d98ed2` | teto base volta a 4.400 B; tools de arquivo em 7.800 B |
| 7.7b ao vivo + aceite | concluída | `50c248e`, `9148331` | rodada 1 (contratos reais, `A:` aceito pelo guard) e rodada 2 (documentos do owner, PPTX real) |
| 7.7a revisão adversarial | concluída | `e7b05be` | 158 vetores, 0 ESCAPOU; 2 achados Altos, 3 Médios, 6 Baixos |
| correções, passo 1 | concluída | `0f105ef` | A1, A2, M1, B1–B5, P7.7-3; teto do WASM calibrado em 40 PDFs reais |
| fuzz 30 min por alvo | concluída | `a8285f4` | DOCX, PPTX e PDF PASS, 0 crashers (ver §4.6) |
| correções, passo 2 | concluída | `4538523` | M2 (`altChunk`), M3 (`numeracao=indisponivel`), B6 (hífen mantido), texto oculto rotulado |
| 7.8 docs + binário 0.6.0 | concluída | `2899b7d` | README, `CLAUDE.md` (toolchain go1.27.0), binário `0.6.0-4538523` em `D:\MCP\`, cache nos clientes |
| 7.9a pacote `ocr` | concluída | `9b86439` | worker isolado (processo filho, Job Object), hash conferido, sem cgo; `FuzzOCRFrame` 120 s |
| 7.9b integração do OCR | concluída | `9f09aaa` | 9 flags `--ocr-*`, OCR de PDF digitalizado e imagens, marcadores, rodapé, `[aviso]` |
| docs do OCR | concluída | `8451449` | README e `CLAUDE.md` com as 9 flags, marcadores, rodapé, limites e limitações |
| plano + binário 0.7.0 | concluída | `eda7664` | binário `0.7.0-9f09aaa`, OCR ligado nos clientes, corpus apagado |
| commits só de documentação | — | `3b6c254`, `36c4c4e`, `51d2b30`, `5ff2e9e`, `228437f`, `ce8d248`, `49af489`, `e09db8a`, `9f39d17`, `3180af7` | atualizações do plano e do brief (7.2–7.4, 7.5, fuzz do ZIP, §12 do brief, 7.6, decisões pós-7.6, consulta Samba/FTP, decisões da rodada 2 e ajustes no servidor, decisões sobre as correções, 7.7/7.8 concluídas) |

**Detalhes que mudaram o desenho (registrados nas fontes):**

- **Motor de PDF:** go-pdfium v1.21.4 (MIT) em WASM com wazero v1.12.0 (Apache-2.0), sem cgo. É a única dependência externa relevante do `go.mod` (transitivas: go-commons-pool/v2 v2.1.2, google/uuid v1.6.0, x/sys v0.48.0, x/text v0.43.0; na 7.9a o `go.mod` ganhou ainda `x/image` e `x/sys` como dependência direta, conforme o plano). DOCX e PPTX usam só a stdlib.
- **Endurecimento do WASM (7.5):** por padrão o go-pdfium monta o volume inteiro e escreve no `os.Stdout`, o que corromperia o JSON-RPC; foi configurado com `FSConfig` vazio e stdout/stderr descartados. Provado na revisão adversarial: 0 B em stdout e stderr.
- **Guard:** `files` não importa `doctext`; o extrator é injetado e é o **último** passo, depois dos passos 1 a 10 do guard. Sem extrator, o documento continua recusado como binário (fail-closed). O teto de bytes é escolhido pela extensão do nome resolvido, antes de abrir.
- **Rodapé `[arquivo]`:** `formato=`, `paginas=`, `paginas_sem_texto=`, `slides=`, `revisoes_pendentes=` e, quando se aplicam, `numeracao=indisponivel`, `conteudo_importado=sim`, `texto_oculto=sim`, `paginas_ocr=`, `conf_ocr_media=`, `paginas_ocr_falhas=` (+ `paginas_ocr_nao_processadas=` se > 0).
- **Marcadores de posição:** `[página N]`, `[slide N]` (`— oculto`), `[notas do slide N]`, `[cabeçalho]`, `[rodapé]`, `[nota de rodapé N]`, `[nota de fim N]` (desvio: ids colidem com as de rodapé), `[comentário]`, `[quebra de página]`, `[oculto]`, `[página N — OCR conf=..]`, `[imagem — OCR conf=..]`.
- **OCR:** só nas páginas sem texto; em paralelo e em ordem; processo filho `--ocr-worker` (nasce suspenso, entra no Job Object com teto de memória e kill-on-close, só então roda); hash da DLL conferido no pai e no worker; caminho UNC recusado; ambiente do filho sem segredos; qualquer problema na partida vira `WARNING: OCR disabled` e o servidor sobe sem OCR.

---

## 3. Decisões do owner

### 3.1 Decisões de produto (D-1 a D-15)

| # | Decisão | Valor final |
| --- | --- | --- |
| D-1 | Motor de PDF | go-pdfium v1.21.4 (WASM, wazero v1.12.0), fixada após o spike ampliado (35 PDFs) |
| D-2 | Dependência externa | só a do PDF; DOCX e PPTX só stdlib |
| D-3 | Teto de documento | `--max-doc-bytes` / `MCPOLLAMA_MAX_DOC_BYTES`, 32 MiB (33.554.432 B), só `.docx/.pptx/.pdf` (e imagens) |
| D-4 | PDF digitalizado | OCR na etapa 7.9; até lá, recusa clara |
| D-5 | Notas do apresentador e comentários | incluídos, sempre rotulados |
| D-6 | Versão | 0.6.0 (0.7.0 com OCR) |
| D-7 | Ligar OCR | opt-in por `--ocr-lib` + `--ocr-lib-sha256` |
| D-8 | Procedência da DLL | `libtesseract64.dll` 5.3.3 pronta (compilar a 5.5.2 ficou como pendência opcional) |
| D-9 | Isolamento | processo filho `--ocr-worker` + Job Object |
| D-10 | OCR no Linux | não; "OCR indisponível nesta plataforma" (só Windows) |
| D-11 | Modelo padrão | `best` por padrão; `fast` copiado; `--ocr-tessdata` escolhe |
| D-12 | Imagens soltas | sim, `.png` e `.jpg` (`.tif` fora) |
| D-13 | Orçamento do `tools/list` | acréscimo mínimo nas descrições; orientação vai na resposta. Revista em 28/10: base volta a **4.400 B**, tools de arquivo em **7.800 B** |
| D-14 (nova) | Cache de compilação do WASM | só por `--pdf-cache-dir` (sem a flag, nada em disco) |
| D-15 (nova) | Motor de PDF | **residente**, sem liberação por ociosidade (a `--pdf-idle` caiu: liberar devolveria só ~20–30 MB) |

### 3.2 Outras decisões do owner

- **Denylist:** mantida a política atual (casa por prefixo ou sufixo do nome inteiro). `prod.env.pdf` e `chave.pem.docx` **não** são barrados; faltam `.npmrc`, `.pypirc`, `.netrc`, `.pgpass`, `.htpasswd`, `putty.ppk`. Registrada como P7.6-3 (aceita).
- **Dados de cliente:** texto extraído apagado já; corpus mantido até o fim da Fase 7 e depois **apagado** (`.workspace\spike-pdf\`, 287 arquivos, 208,7 MB, mais 10 arquivos de driver no scratchpad).
- **7.8 aprovada:** binário 0.6.0 e cache de PDF nos `.mcp.json` do MCPOllam e do GestorERP.
- **Projeto jurídico:** "o caminho será informado no contexto, nada hardcode"; nada preparado em `.mcp.json` de projeto jurídico (implicação em §6).
- **Servidor de IA:** 3 ajustes autorizados e feitos (ciclo `nvidia-powercap`/`ollama-warm`, journald, `CONEXAO-IA.md`).
- **Correções da revisão adversarial (16:28):**
  1. aplicar A1, A2, M1, B1–B5 e P7.7-3, com o teto do WASM calibrado no corpus real;
  2. M2 (`altChunk`): extrair texto/HTML/DOCX importado; se não suportado, `[aviso]`; rodapé `conteudo_importado=sim`;
  3. M3 (numeração que falha): avisar no rodapé (`numeracao=indisponivel`);
  4. B6 (hífen): manter sempre o hífen real; texto oculto entra rotulado `[oculto]`.
- **Fase 8 (candidata, só resposta):** não montar Samba/FTP no servidor de IA agora; se a triagem em volume virar rotina, worker Linux do mcpollama numa máquina da LAN (não a da GPU), somente leitura, jobs assíncronos, OCR com a `.so` (reabre a D-10).

---

## 4. Métricas

### 4.1 Tempo de extração por formato

Extração local, sem o modelo. Cache de compilação do PDF preenchido. Dois conjuntos de medição; os tempos podem estar inflados pelo fuzz que rodava em paralelo.

**Direta (`doctext.Extract`, 2 execuções, mesmo SHA-256):**

| id | formato | tamanho do documento | t1 / t2 |
| --- | --- | --- | --- |
| o01 | pptx | 9 slides | 7 / 10 ms |
| o02 | pptx | 150 slides (43 MB; acima do teto do servidor, só na extração direta) | 1.541 / 1.439 ms |
| o03 | docx | 17.285 runas | 37 / 36 ms |
| o07 | pdf | 15 páginas | 152 / 158 ms |
| o09 | pdf | 14 páginas | 1.887 / 1.414 ms |
| o05 | pdf | 550 páginas | 4.909 / 4.264 ms |
| o06 | pdf | 338 páginas | 4.644 / 5.369 ms |

**Pelo servidor (extra = parede − total do Ollama; inclui leitura e extração):** PDF 91–148 ms (até 12 páginas de 5,7 MB); DOCX 65–114 ms; PPTX (o01) 128 ms; DOCX o03 66 ms; PDF o07 497 ms; o livro o05 (550 páginas) 5,3 s.

**Spike (go-pdfium, 35 PDFs):** 40,8 ms por PDF em média (26 com texto, 133 páginas); 7,14 ms por página nos PDFs com 5 ou mais páginas.

**Determinismo:** mesmo arquivo gera o mesmo texto, byte a byte: 8 de 8 extrações do aceite; DOCX e PPTX 50× e PDF 20× na revisão adversarial.

### 4.2 Init do motor de PDF, com e sem cache

| Cenário | Init | Fonte |
| --- | --- | --- |
| sem cache | ~5 s (4,6 a 10,8 s no spike; 6,0 a 6,75 s na 7.5) | spike, 7.5 |
| com cache de compilação | 113 a 150 ms | spike (§12.7), 7.5 (123 ms) |
| 1ª chamada de PDF no servidor, cache vazio | +8,9 s (e preenche o cache) | ao vivo |
| 1ª chamada, sem `--pdf-cache-dir` | +7,7 s | ao vivo |
| 1ª chamada, cache preenchido | +0,6 s | ao vivo |

O motor é preguiçoso: a partida do servidor não o inicia.

### 4.3 Memória

**Motor de PDF (WASM):**

| Situação | Working set |
| --- | --- |
| compilando, sem cache | 317 a 339 MB (pico) |
| motor aberto, com cache | 68 a 81 MB |
| motor fechado | 47 a 51 MB residuais (base do processo: 7,6 MB) |
| maior uso real de memória do WASM em 40 PDFs | **25,4 MiB** (o09); o livro de 550 páginas usou 18,1 MiB em ~14 s |
| teto do WASM adotado | 512 MiB (8192 páginas), ~20× o maior uso real |

Calibração do teto: 40 PDFs com tetos de 4 GiB, 512, 256 e 128 MiB, **0 divergências** (30 extraem iguais; 3 senhas e 7 sem texto recusados nos 4).

**Worker de OCR (processo filho):** pico de working set de **82 MiB** (7.9a) e de **83 a 119 MiB** nas 7 medições da 7.9b; teto por filho via Job Object `--ocr-max-mem` (padrão 1 GiB).

**Outras medições:** zip bomb de 100 MiB, pico de heap 156 → 0 MiB (recusa antecipada); parte de 64 MiB, 157 → 64 MiB alocados. Amplificação do DOCX (A1) 286–293 MiB → 0,4–6 MiB; bomba de PDF (A2) +2,9 a +5,4 GiB → +223 a +440 MiB, parando em 2,0 s.

### 4.4 Tamanho do binário

| Versão | Bytes | Observação |
| --- | --- | --- |
| 0.5.2 (antes da Fase 7) | 10.903.552 | ~10,9 MB |
| 7.6 (worktree) | 22.507.008 | o plano cita também 22.510.592 B no worktree da 7.6 (divergência pequena entre fontes) |
| 0.6.0-4538523 | 22.608.384 | ~22,6 MB, partida 44 ms |
| 0.7.0-9f09aaa | 23.032.320 | ~23,0 MB |

Spike (projeção, não medida): ~+10 MB com o PDFium WASM; o `.wasm` tem 5.828.543 B.

### 4.5 Tamanho do `tools/list` (com `--allow-root`)

| Medida | Antes | 0.6.0 | 0.7.0 |
| --- | --- | --- | --- |
| payload do teste de budget, tools de arquivo (teto 7.800) | 7.548 | 7.728 | **7.800** (folga 0) |
| `tools/list` compacto (`--tools-verbosity compact`) | 4.291 | 4.381 | 4.417 |
| `tools/list` completo, linha JSON-RPC pelo binário | 7.582 | 7.762 | 7.834 |
| payload das 4 tools base (teto 4.400) | 4.390 | 4.390 | 4.390 (inalterado) |

Atenção: as linhas "payload do teste" e "linha JSON-RPC pelo binário" são **medidas diferentes** (nas fontes, 7.548 → 7.728 → 7.800 é o teste; 7.582 → 7.762 → 7.834 é o binário). Qualquer acréscimo futuro nas descrições estoura o teto D-13.

### 4.6 Fuzz

Em worktree isolado em `3d98ed2` (código **anterior** às correções da revisão), `-fuzzminimizetime 5s -parallel 8`:

| Alvo | Duração | Execuções | Corpus final | Resultado |
| --- | --- | --- | --- | --- |
| `FuzzDOCX` | 30,1 min | 28.299.843 (28,3 M) | 648 | PASS |
| `FuzzPPTX` | 30 min | 40.984.381 (41,0 M) | 438 | PASS |
| `FuzzPDF` | 30 min | 615.504 (615 mil) | 168 | PASS |

FuzzOCRFrame 30 min: **PASS**, 56.586.481 execuções, 36 entradas no corpus, 0 crashers (commit `9f09aaa`, `-fuzzminimizetime 5s -parallel 8`; resumo em `…fase7.testes/fuzz30/FuzzOCRFrame-resumo.log`)

0 entradas de falha salvas. Antes: `FuzzOCRFrame` de 120 s, 1,46 M execuções, PASS (7.9a).

**Leitura honesta:** o fuzz guiado por cobertura **não** achou A1 nem A2, que a revisão adversarial achou com vetores construídos. O critério do brief (§7.5) foi cumprido, mas fuzz verde não substitui a revisão. O `FuzzPDF` é pouco guiado porque a cobertura do Go não enxerga dentro do WASM. O código corrigido ainda não passou por fuzz de 30 min (P7.7-5).

### 4.7 Revisão adversarial

**Fase 7 (Opus, commit `3d98ed2`):** **158 vetores** — 74 BLOQUEADO, 30 FIEL, 34 INFO, 10 ACHADO, 10 SILENCIOSO, **0 ESCAPOU**. Nada vazou; o servidor ficou de pé em todos. Errata: a 1ª versão dizia 176 vetores e 92 BLOQUEADO porque somava 18 linhas `VEC|doc/` de um teste existente da 7.6.

| Achado | Gravidade | Correção |
| --- | --- | --- |
| A1: DOCX de 1,2 KB aloca ~1 GiB (`lvlText` copiado em cada parágrafo) | Alto | `lvlText` > 128 runas vira `- `; célula conta no teto ao entrar |
| A2: PDF de 33 KB roda 16–29 s com `--timeout` de 2 s e +2,9 a 5,4 GiB | Alto | teto do WASM de 512 MiB, interrupção pelo `ctx`, `Kill` da instância |
| M1: `fldChar` desbalanceado apaga o resto da parte | Médio | campo aberto no fim da parte → recusa |
| M2: `w:altChunk` descartado | Médio | extrai TXT/HTML/DOCX importado; senão `[aviso]`; `conteudo_importado=sim` |
| M3: `numbering.xml` quebrado remove todos os números | Médio | texto sem números + `numeracao=indisponivel`; teto de itens 300.000 |
| B1–B5 | Baixo | partes sem diferenciar caixa; alvo de cabeçalho ≠ corpo; `r:id` repetido recusado; Fallback sem Choice lido; timeout tipado no PPTX |
| B6: junção de hífen apagava hífens reais ("obriga-se") | Baixo | hífen sempre mantido (custo: "con-trato" também mantém) |

**Números finais (após os passos 1 e 2):** Fase 7: 158 vetores (74 BLOQUEADO, 43 FIEL, 41 INFO) com **0 ACHADO, 0 SILENCIOSO e 0 ESCAPOU**; bomba de PDF para em 2,0 s. Depois da 7.9b: 166 vetores (8 de imagem novos), **0 ESCAPOU e 0 SILENCIOSO**.
**Fase 4 sem regressão:** 227 vetores (223 BLOQUEADO, 2 ACHADO e 2 INFO de política já aceitos), **0 ESCAPOU**; na 7.9b, 245 linhas `VEC` (241 BLOQUEADO, 2 INFO, 2 ACHADO), 0 ESCAPOU. Os 2 ACHADO são os de política: a denylist sem `.npmrc` e afins, e `--allow-root C:\` aceito com aviso (SEC-M-02).
**Prova das correções:** os 7 testes de regressão do passo 1 **falham** no código antigo e **passam** no novo (7/7 nos dois sentidos). Gate: `gofmt` vazio, `vet` OK, suíte verde, `-race` com 0 DATA RACE.
**Não testado:** limite de 4 GiB do WASM sem o teto; Word/PowerPoint reais nos casos-limite de forma; binário Linux.

### 4.8 Aceite

| Verificação | Resultado |
| --- | --- |
| Guard aceita `--allow-root` em unidade de rede com espaços (`A:`, caminho UNC por trás) | aceito, sem afrouxar nada |
| Numeração DOCX contra o Word (COM `ListFormat.ListString`) | d01 **83/83**; d02 **83/84** (o 84º é parágrafo numerado vazio, sem texto a alinhar); o03 **22/22** (sequência do corpo idêntica) |
| Marcadores de lista saem como `- ` | 20/20 (d01), 20/20 (d02), 3/3 (o03) |
| PPTX real contra o PowerPoint (COM) | o01 9 = 9 e o02 150 = 150 slides; ocultos 1 = 1; com notas 144 = 144; razão de palavras por slide **1,000** em todos os 146 slides com texto |
| `paginas=` contra o PyMuPDF | 4 de 4 nos contratos (c02, c04, c24, c22) e 4 de 4 nos do owner (550, 338, 15, 14) |
| Recusas ao vivo (mensagens opacas, 7–70 ms) | PDF digitalizado ×2 e PDF com senha ×2 corretos; `.doc` OLE com mensagem própria |
| OCR direto | **26 páginas OCR em 7 PDFs**, 0 falhas, conf de 54 a 92 (média 88 ponderada); o08: 12 páginas em 18,8 s |
| OCR: valores conferidos contra a imagem (o08, página do owner) | **3/3 conferiram** (2 números e 1 nome) |
| `extract` de "partes, CPF/CNPJ, valor e data" no digitalizado c07 | `paginas_ocr=9 conf_ocr_media=91 paginas_ocr_falhas=0` + `[aviso]` presente; JSON de 891 caracteres **não exibido**; 75 s (OCR + 13,6 s de modelo) |
| `summarize_file` ao vivo, o08 | `paginas_ocr=12 conf_ocr_media=90`, `[aviso]` presente, 31,7 s de parede (OCR + 1ª init do PDFium sem cache + 5,2 s de modelo) |
| Desempenho do OCR (A4 a 300 dpi) | `best` 4,6 a 6,0 s por página (7.9a; a 7.9b registra 4,5 a 6,7 s), conf 95; `fast` 1,6 a 1,7 s, conf 95 |
| Bomba PNG 100.000×100.000 | recusada com 1,2 KB alocados |
| Tool longa pelo Claude Code (`claude -p`) | livro de 550 páginas, 10 partes, 11 chamadas, `total_ms=170302`, parede 206,6 s (~3,5 min), **OK** (ao vivo direto: 192,8 s, sem estourar o `--timeout` de 120 s, que vale por chamada) |
| Recusas de tamanho e vazio | PPTX de 43 MB recusado (teto 32 MiB); DOCX sem texto recusado ("o arquivo está vazio") |

**Defeitos de produção no aceite:** nenhum. Divergência menor da rodada 2 (`revisoes_pendentes=sim` com Word 0, por `w:ins` vazio em `w:numPr`) corrigida como P7.7-3.

---

## 5. Limitações conhecidas

1. **Tabela de PDF sem garantia.** Não há referência `pdftotext` na estação; a leitura de tabelas divergiu entre motores em 4 de 6 PDFs com indício de tabela (a ordem em c30 e c33, LCS 0,739 e 0,683). Não tratar linhas de tabela de PDF como contrato de saída.
2. **Linhas de tabela sombreada perdidas no OCR.** No o08 (página 10), 4 das 11 linhas de uma tabela de preços sobre fundo sombreado, incluindo o total, não apareceram, e 1 símbolo de moeda saiu trocado. Parágrafos corridos saíram íntegros.
3. **Hífen de quebra silábica mantido.** O PDFium devolve o mesmo `\x02` para hífen real e soft hyphen; a decisão do owner mantém sempre o hífen ("obriga-se" correto, "con-trato" também fica com hífen).
4. **Texto invisível de PDF não rotulado como oculto.** O modo de renderização 3 também é a camada de OCR dos PDFs digitalizados; rotulá-lo erraria no caso mais comum. Texto branco sobre branco e `Tr 3` chegam ao modelo (vetor de injeção; o `wrap` com nonce segura o marcador).
5. **OCR sem detecção de orientação.** A DLL 5.3.3 não exporta OSD; com conf < 60 faz uma 2ª passada girada 180° (de cabeça para baixo recuperada; 90° é lido direto). Só `girada=180` aparece nos marcadores.
6. **OCR só no Windows.** Linux responde "OCR indisponível nesta plataforma" (D-10). O OCR exige a DLL, os modelos e o hash por flag.
7. **Denylist por prefixo/sufixo do nome inteiro (política aceita).** `prod.env.pdf` e `chave.pem.docx` não são barrados.
8. **`.doc`, `.ppt`, `.xls` recusados** com mensagem própria; também fora: `.xlsx`, `.odt`, `.rtf`, `.html`, `.eml/.msg`, `.docm/.pptm`, `.tif`.
9. **PPTX validado com 2 apresentações reais** (o01 e o02), além das fixtures; o acervo de contratos não tem `.pptx`.
10. **Não extraídos, sem sinal no rodapé:** valores de formulário (AcroForm) de PDF, anotações, texto de SmartArt e gráficos, imagens, anexos e macros.
11. **Numeração DOCX:** `ordinal` sai sempre `1º` (simplificação, qualquer idioma); formato não suportado vira `- `; `w:vanish` alternante entre estilos aninhados não é calculado; `%10` vira `%1` seguido de "0" (igual ao Word).
12. **Outras:** `revisoes_pendentes` é conservador; `nome=` do arquivo aparece na linha `[arquivo]` (quem chama já enviou o `path`, mas fica registrado); o `ctx` só interrompe a página em curso via `Kill` da instância; `tools/list` com folga zero (7.800/7.800).

---

## 6. Pendências numeradas

### 6.1 Do owner

1. **Reiniciar os clientes** (Claude Code, VS Code, Cursor, Claude no GestorERP) para carregarem o binário 0.7.0.
2. **Apagar os 2 `.exe~` em `D:\MCP\`** depois de reiniciar: `mcpollama-mcp-0.6.0.exe~` e o `.exe~` (0.5.x). A proteção da pasta bloqueou o `Remove-Item` (correto); o 0.7.0 foi instalado só por renomeação e cópia.
3. **Caminho "informado no contexto" x `--allow-root`.** O servidor só lê dentro de uma raiz permitida. Para a IA jurídica usar um caminho que só chega no contexto, ele precisa estar numa raiz já configurada. **Proposta:** suportar as `roots` do protocolo MCP (raízes declaradas pelo cliente), com o mesmo guard e a mesma validação de cada raiz. Decisão pendente.
4. **GPU e PSU do servidor de IA.** Falha de PCIe no link `00:1b.4`/GPU 3 e fonte de alimentação (configuração provisória de 3 GPUs e `num_ctx` 65536; o servidor reiniciou em 27/10 às 12:04, fora desta sessão). A prova do ajuste `nvidia-powercap` x `ollama-warm` no boot real fica para o próximo reboot.
5. **Rotação de segredos antigos** (item levantado pelo owner; as fontes lidas não detalham quais segredos). Observação do `git status`: `.claude/`, `.cursor/` e `.vscode/` estão não rastreados; antes de qualquer commit, conferir que nenhuma config com `--api-key` entre.
6. **Decidir sobre os `.rar` na raiz** (`MCPOllam.rar`, `mcpollama-mcp.rar`, não rastreados): ficam ou saem.
7. Itens herdados: denylist (P7.6-3, política aceita, reavaliar se quiser `.npmrc` etc.); decidir se `nome=` deve sair do rodapé.

### 6.2 Técnicas

1. **P7.9-1: partida com OCR.** Com o fuzz ocupando a CPU, `initialize` levou 0,7 a 3,6 s (e 3 a 8,6 s para encerrar) com OCR ligado; o agente tinha afirmado que a sonda não atrasava o handshake e isso não se confirmou. Medir de novo **sem carga**; se persistir, tirar o trabalho síncrono da partida. (Smoke do agente: `initialize` em 59 ms com OCR.)
2. **P7.5-x restantes:** **P7.5-5** (estratégia de fuzz para o WASM, que a cobertura não guia) segue aberta. P7.5-1 (`ctx` entre páginas) e P7.5-2 (teto de memória do WASM) foram tratadas no A2; P7.5-3 (teto de megapixels) na 7.9b; P7.5-4 na 7.6.
3. **P7.7-5:** repetir `FuzzDOCX`, `FuzzPPTX` e `FuzzPDF` de 30 min **no código já corrigido** (o fuzz rodou em `3d98ed2`).
4. **P7.0-4:** identificar a versão do PDFium dentro do `.wasm` (5,8 MB) e auditar os componentes de terceiros (licenças); a licença Apache-2.0 do PDFium vem do README do go-pdfium.
5. **Compilar o Tesseract 5.5.2** (opcional, D-8; P7.0-2), com Leptonica 1.88; ganharia OSD e qualidade; a licença do `tessdata` (Apache-2.0) ainda está "a confirmar".
6. **Cache de OCR em disco** (fora do escopo; ajudaria na triagem repetida; exige flag, pois o servidor não grava em disco sem pedido).
7. **Pré-processamento do OCR para tabelas sombreadas** (binarização/limiar adaptativo ou outro PSM), medindo no corpus antes de mudar. Remedir `fast` x `best` no corpus real (D-11 fixou `best`).
8. **Folga zero do `tools/list`** (7.800/7.800): qualquer descrição nova exige rever o D-13.
9. **P7.4-1:** avaliar filtrar `dt`/`ftr`/`sldNum` dentro do slide (ruído).
10. **Fase 8 / fora do escopo:** worker Linux, inventário de pasta, `.xlsx/.odt/.rtf/.html/.eml`, `.tif`, gravar documento traduzido.
11. ~~Teste ainda aberto: `FuzzOCRFrame` de 30 min~~ — **fechado** às 19:19 (PASS, 56,6 M execuções, 0 crashers; ver §4.6).

---

## 7. Erros de processo (registrados com honestidade)

1. **Commits intermediários que não compilavam (pegos em worktree).** Na 1ª sequência, o commit da 7.2 (`aee916a`) não compilava sozinho: o `FuzzDOCX` do `fuzz_test.go` usa o helper `nbAbs`, que estava no teste da 7.3. O agente afirmou o contrário e eu commitei sem verificar. A verificação em worktree pegou; `git reset --soft feb5561` e nova sequência, antes de qualquer push. Lição: verificar cada commit intermediário num worktree **antes**.
2. **Horários estimados em vez de lidos.** Os cabeçalhos "14:42" e "14:46" do plano foram estimados (o real era 14:41:50), e o "18:57" também (o real era ~18:55). Regra: usar só o hook ou `Get-Date`.
3. **Armadilhas de PowerShell.** (a) Usei uma função chamada `R`, que é alias de `Invoke-History` no PowerShell; as substituições no `CONEXAO-IA.md` não foram aplicadas; restaurei do backup e refiz com `Rep1` (10/10). (b) `Application.Quit(0)` do Word falhou no PowerShell 5.1 (passagem de argumento); os 2 `WINWORD` do agente foram encerrados com `Stop-Process`, sem salvar nada; o PowerPoint também ficou vivo após `Quit`. (c) O `Remove-Item` em `D:\MCP\` foi bloqueado pela proteção da pasta.
4. **Contagem errada de vetores (errata).** A 1ª versão do report adversarial dizia 176 vetores e 92 BLOQUEADO; a correta é 158 e 74, porque 18 linhas `VEC|doc/` eram de um teste existente. Corrigido no report, no brief e no plano.
5. **Parser do R1 (Fase 6, anterior à Fase 7).** A 1ª rodada da listagem em JSON falhou 15/15 por bug no parser que cortava no `]` da linha `[arquivo]`; corrigido e refeito (15/15). Registrado aqui por constar da lista de erros da sessão; a origem está no plano, em 26/10.
6. **Fixture JPEG do agente (7.9b).** Na 1ª rodada, 2 vetores JPEG deram `ESCAPOU` por critério do próprio teste: o fixture escrito à mão tinha só o SOF e o `DecodeConfig` da stdlib lê até o SOS. Já estavam recusados (13 KB alocados, OCR não chamado). Refiz o fixture a partir de um JPEG real da stdlib; os 2 passaram a BLOQUEADO com `ReasonImageTooLarge`.
7. **Outros erros dos agentes, registrados nas fontes:** afirmação de que a sonda do OCR não atrasava o handshake (não se confirmou, P7.9-1); vetor de amplificação com `%1` no próprio nível que subestimou o efeito (corrigido para `%2`); vetor de timeout do PPTX com 3000 slides acima de `MaxEntries` (baixado para 1500); `TestOCRParallelKeepsOrder` falhou sob `-race` por defeito do teste, não do código; a 1ª leitura do "travamento" do fuzz (era artefato do motor, comprovado por instrumentação).
8. **Integridade do aceite:** o agente de aceite **não** leu o resumo para conferir se cita as cláusulas certas (sigilo); a numeração entregue ao modelo foi conferida, mas a qualidade do resumo não foi avaliada por leitura.

---

## 8. Checklist final (§10 do brief), marcado pelo estado real

- [x] D-1..D-6 e D-13 decididas pelo owner e registradas no plano (+ D-14 e D-15)
- [x] Spike de PDF com report só de métricas
- [x] DOCX: corpo, tabelas, numeração, revisões, cabeçalho/rodapé, notas, comentários, sem duplicata
- [x] PPTX: ordem por `sldIdLst`, grupos, tabelas, notas, ocultos
- [x] PDF: marcadores de página, sem texto, senhas, malformado sem panic
- [x] Extensão + assinatura; `.doc`/OLE com mensagem própria
- [x] Limites de ZIP/XML/texto; sem DTD; sem `External`; sem I/O no `doctext` (provas: testes de segurança e roteiro adversarial com 0 ESCAPOU; conferido agora que o `doctext` não importa `os`, `net/http`, `os/exec` nem `net` fora dos testes)
- [x] Guard: todas as defesas antes da extração; `--max-doc-bytes`; `ctx`
- [x] Rodapé `[arquivo]` com `formato`, `paginas`/`slides`, `paginas_sem_texto`, `revisoes_pendentes`
- [x] Descrições mínimas (D-13) + `TOOLS.md` regerado + teto do `TestToolsListBudget` ajustado + medição full/compact do `tools/list`
- [x] Unitários, segurança, fuzz 30 min/alvo, `-race`, revisão adversarial da Fase 4 repetida (ressalva: o fuzz rodou antes das correções; repetir no código corrigido é P7.7-5)
- [x] Ao vivo (uma chamada por vez) e aceite §7.7 com o caso real
- [x] README, CLAUDE.md, plano e report atualizados; binário 0.6.0 em `D:\MCP\` (e 0.7.0)
- [x] Nenhum segredo, binário ou documento de cliente no git (conferido agora: 0 binários/documentos/imagens/`.wasm`/`.dll`/`.rar` rastreados; varredura de padrão de token em arquivos rastreados, fora de `referencias/`, sem ocorrências; corpus apagado)
- [x] (7.9) OCR: D-7..D-12 decididas; worker isolado; DLL por caminho absoluto + SHA-256; marcadores e rodapé de OCR; aviso de números; testes e aceite §11.9 — **concluído** (`FuzzOCRFrame` 30 min PASS, 56,6 M execuções)

---

## 9. Lembrete operacional (§8.5 do brief)

- O cliente MCP da pasta de contratos precisa incluir `A:\Monitor\CSL Telecom\Contratos` no `--allow-root` (o guard aceitou essa raiz de rede). A decisão do owner foi não preparar nada em `.mcp.json` de projeto jurídico; ver pendência 3 do owner (§6.1) sobre `roots` do MCP.
- O binário em uso precisa ser trocado: o `.exe` em uso vira `.exe~` no build. **Reinicie os clientes e apague os 2 `.exe~` em `D:\MCP\`.**
- Ao consumir as tools em trabalho jurídico: ler sempre o rodapé `[arquivo]` (`formato`, `paginas`, `paginas_sem_texto`, `revisoes_pendentes`, `numeracao`, `conteudo_importado`, `texto_oculto`, `paginas_ocr`, `conf_ocr_media`) e o `[aviso]`. Com OCR, confira no original números, datas, nomes e CPF/CNPJ. Com `revisoes_pendentes=sim`, o contrato pode não estar fechado.
- Cite posição pelos marcadores (`[página N]`, `[slide N]`, número da cláusula do texto); no DOCX não há número de página (o Word não guarda a paginação).

## Adendo (19:21) — partida com OCR medida sem carga (fecha a P7.9-1)

Com a CPU livre (fuzz encerrado), 3 rodadas alternadas com o binário 0.7.0 instalado: `initialize` **74–238 ms com OCR** contra 33–35 ms sem OCR; o processo encerra em ~0,6 s com OCR (espera a sonda da DLL). Os 0,7–3,6 s medidos antes eram contenção de CPU pelo fuzz paralelo. Partida aceitável; P7.9-1 fechada.
