# Spike 7.0 - Escolha do motor de extração de texto de PDF (somente métricas)

Data: 2026-10-28 · Projeto: mcpollama · Código e dados do spike: `E:\MCPOllam\.workspace\spike-pdf\` (ignorado pelo git; contém cópias do corpus, por isso nada disso vai para o repositório).

## 1. Objetivo

Comparar dois motores Go de extração de texto de PDF sobre contratos reais, só com métricas calculadas em código, para o owner decidir a dependência da Fase 7:
- **A**: `github.com/klippa-app/go-pdfium` em modo WebAssembly (runtime wazero, Go puro, sem cgo).
- **B**: `github.com/ledongthuc/pdf` (Go puro).

Nenhum texto extraído, nome de arquivo original, nome de parte ou valor consta neste report. Nada foi alterado em `src/` nem na unidade de rede; nada foi commitado.

## 2. Versões e licenças (conferidas nos arquivos LICENSE do módulo baixado)

| Componente | Versão | Licença | Observação |
|---|---|---|---|
| go-pdfium | v1.21.4 (publicada em 2026-10-21; go.mod exige go >= 1.26.0) | MIT (LICENSE do módulo) | wrapper Go + `pdfium.wasm` embutido (5.828.543 bytes) |
| PDFium (dentro do .wasm) | versão não identificada no módulo | Apache-2.0 segundo o README do go-pdfium (não há LICENSE do PDFium dentro do módulo; o .wasm pode conter outras dependências de terceiros que não foram auditadas) | |
| wazero | v1.12.0 | Apache-2.0 (LICENSE do módulo) | única dependência de runtime pesada que entrou no binário mínimo |
| go-commons-pool/v2 | v2.1.2 | Apache-2.0 (LICENSE do módulo) | dependência do pacote webassembly |
| google/uuid | v1.6.0 | BSD-3-Clause (texto do LICENSE) | |
| ledongthuc/pdf | v0.0.0-20261020125853-5e46805f718e (pseudo-versão, 2026-10-20; o módulo não tem tags) | BSD-3-Clause (cabeçalho "The Go Authors", mesmo texto do Go) | sem dependências externas no binário mínimo |

Toolchain usada: go1.27.0 windows/amd64 (o go.mod do projeto declara `go 1.26`; o go-pdfium v1.21.4 declara go 1.26.0).

## 3. Corpus (15 PDFs, copiados para `corpus\c01..c15.pdf`; mapa só em `mapa.txt`)

Seleção por varredura de bytes sobre 673 PDFs da pasta (limite de 80 MB por arquivo). Contagem por categoria (um PDF pode cair em várias):
- Word/Microsoft (por /Producer ou /Creator em texto claro): **2** (c10, c11). Só 6 dos 673 PDFs têm essa marca; os 6 são só imagem (testei os 3 que sobraram: também sem texto). Não houve PDF "Word com texto" na pasta, dentro do que a varredura enxerga (metadados em object stream/XMP não são lidos).
- Outros geradores: 13.
- /Encrypt: **2** (c01, c02). Havia 8 na pasta.
- Sem /Font no arquivo (heurística de bytes; escaneados): **6** (c03, c05, c06, c07, c10, c11). Resultado dos motores confirma: os 6 têm todas as páginas sem texto.
- Assinados (/ByteRange ou /Sig): **3** (c04, c08, c09).
- Maiores que 4 MB: **2** (c03 = 4,59 MB; c04 = 5,76 MB). Havia 5 na pasta.
- Prováveis tabelas: **não avaliado** (sem como detectar por bytes; inclui apenas PDFs de geradores diversos).
- Composição por resultado: 8 PDFs com texto (c02, c04, c08, c09, c12 a c15), 6 só imagem, 1 que nenhum motor abriu (c01).
- Páginas: 47 no total (c04 = 12, c09 = 10, c07 = 9, c15 = 5, c02 = 2, demais 1). O corpus é pequeno em páginas com texto (33 páginas com texto nos dois motores).

## 4. Metodologia

- Cada motor roda em processo próprio, 2 execuções sobre os 15 PDFs (30 extrações por motor); recover por PDF e por página; limite de 240 s por PDF (não houve estouro).
- Tempo = abrir + extrair todas as páginas + fechar (A não inclui a inicialização do WASM, medida à parte). Páginas "sem texto" = menos de 20 runas não brancas.
- Memória: HeapAlloc de pico (amostrado a cada 3 ms) menos o HeapAlloc antes (após GC), e pico de working set do processo (GetProcessMemoryInfo). **A memória do WASM não aparece no heap do Go**, por isso o working set é a medida válida para o motor A.
- Acentos = ç ã õ á é í ó ú â ê ô à (e maiúsculas) por 1000 runas; U+FFFD, mojibake ("Ã§", "Ã£", "Ã©", "Ãµ", "â€" e similares) e controles (exceto \n \r \t \f) por 1000 runas.
- Concordância por página (A vs B, só páginas com texto nos dois): Jaccard dos conjuntos de palavras normalizadas (minúsculas, só letras/dígitos) e LCS das sequências sobre o tamanho da menor (limitado a 8000 palavras por página). Não há motor de referência (ver limitações).
- Determinismo: SHA-256 do texto por PDF comparado entre as 2 execuções.

## 5. Métricas por PDF (A = go-pdfium WASM, B = ledongthuc)

Colunas "pág." = páginas A/B; "determ." = texto idêntico entre as 2 execuções.

| PDF (categoria) | pág. | A: pág. sem texto | B: pág. sem texto | A: falha | B: falha | A: ms | B: ms | A: pico heap MB | B: pico heap MB | A: acentos/1000 | B: acentos/1000 | A: FFFD/1000 | B: FFFD/1000 | A: mojibake/1000 | B: mojibake/1000 | A: ctrl/1000 | B: ctrl/1000 | Jaccard | LCS | determ. A | determ. B |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| c01 (/Encrypt) | 0/0 | 0 | 0 | senha/encrypt | senha/encrypt | 0 | 0 | 0.0 | 0.0 | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | n/a | n/a | idêntico | idêntico |
| c02 (/Encrypt) | 2/2 | 0 | 0 | ok | ok | 17 | 3 | 0.5 | 0.4 | 16.3 | 9.7 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 1.000 | 1.000 | idêntico | idêntico |
| c03 (sem /Font, >4 MB) | 1/1 | 1 | 1 | ok | ok | 5 | 0 | 4.4 | 0.0 | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | n/a | n/a | idêntico | idêntico |
| c04 (assinado, >4 MB) | 12/12 | 0 | 0 | ok | ok | 36 | 3 | 5.6 | 1.4 | 15.7 | 15.3 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.710 | 0.951 | idêntico | idêntico |
| c05 (sem /Font) | 1/1 | 1 | 1 | ok | ok | 2 | 1 | 0.0 | 0.0 | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | n/a | n/a | idêntico | idêntico |
| c06 (sem /Font) | 1/1 | 1 | 1 | ok | ok | 1 | 0 | 0.0 | 0.0 | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | n/a | n/a | idêntico | idêntico |
| c07 (sem /Font) | 9/9 | 9 | 9 | ok | ok | 2 | 1 | 0.0 | 0.0 | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | n/a | n/a | idêntico | idêntico |
| c08 (assinado) | 1/1 | 0 | 0 | ok | ok | 4 | 1 | 0.4 | 0.0 | 24.3 | 25.1 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.889 | 0.968 | idêntico | idêntico |
| c09 (assinado) | 10/10 | 0 | 0 | ok | ok | 50 | 2 | 2.6 | 0.0 | 21.9 | 15.7 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.414 | 0.964 | idêntico | idêntico |
| c10 (Word, sem /Font) | 1/1 | 1 | 1 | ok | ok | 36 | 29 | 0.2 | 2.7 | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | n/a | n/a | idêntico | idêntico |
| c11 (Word, sem /Font) | 1/1 | 1 | 1 | ok | ok | 100 | 73 | 0.4 | 2.9 | 0.0 | 0.0 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | n/a | n/a | idêntico | idêntico |
| c12 (outros) | 1/1 | 0 | 0 | ok | ok | 10 | 2 | 0.3 | 0.0 | 30.2 | 29.3 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 1.000 | 1.000 | idêntico | idêntico |
| c13 (outros) | 1/1 | 0 | 0 | ok | ok | 8 | 2 | 0.4 | 0.0 | 30.2 | 29.2 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 1.000 | 1.000 | idêntico | idêntico |
| c14 (outros) | 1/1 | 0 | 0 | ok | ok | 8 | 2 | 0.3 | 0.0 | 30.3 | 29.3 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 0.00 | 1.000 | 1.000 | idêntico | idêntico |
| c15 (outros) | 5/5 | 0 | 0 | ok | ok | 43 | 17 | 0.6 | 2.4 | 24.0 | 16.6 | 0.00 | 51.69 | 0.00 | 0.00 | 15.34 | 0.00 | 0.796 | 0.899 | idêntico | idêntico |

c01: os dois motores devolveram erro de senha/criptografia (provável senha de usuário não vazia; não verificado de outra forma). c02 (também /Encrypt) abriu e extraiu nos dois (senha de usuário vazia).

## 6. Agregados

```
AGG A: extraiu_com_texto=8 sem_texto_aberto_ok=6 falhas=1 paginas=47 paginas_sem_texto=14 runas=52288 acentos/1000=21.46 FFFD/1000=0.000 mojibake/1000=0.000 ctrl/1000=2.391 tempo_medio_ms(ok)=23.0 heap_pico_medio_MB=1.1 determinismo=15/15
AGG B: extraiu_com_texto=8 sem_texto_aberto_ok=6 falhas=1 paginas=47 paginas_sem_texto=14 runas=36861 acentos/1000=17.42 FFFD/1000=13.239 mojibake/1000=0.000 ctrl/1000=0.000 tempo_medio_ms(ok)=9.7 heap_pico_medio_MB=0.7 determinismo=15/15
CONCORDANCIA: paginas_com_texto_nos_dois=33 paginas_texto_so_em_um=0 Jaccard_medio_por_pagina=0.6826 LCS_medio_por_pagina=0.9550 | media_por_PDF Jaccard=0.8511 LCS=0.9727 (n_pdfs=8)
INIT_A_ms=5479.1 primeira_chamada_A_ms=0.0
PICO_PROCESSO: A workingset_MB=325.6 Sys_MB=305.6 | B workingset_MB=16.0 Sys_MB=12.7
```

Resumo (15 PDFs, 2 execuções):

| Métrica | A go-pdfium WASM | B ledongthuc |
|---|---|---|
| PDFs com texto extraído | 8 | 8 |
| PDFs abertos sem texto (só imagem) | 6 | 6 |
| Falhas (não abriu) | 1 (c01) | 1 (c01) |
| Panics / timeouts | 0 / 0 | 0 / 0 |
| Runas extraídas (mesmos 8 PDFs com texto) | 52.288 | 36.861 |
| Acentos por 1000 runas (agregado) | 21,46 | 17,42 |
| U+FFFD por 1000 runas | 0,000 | 13,239 (c15: 51,69; demais 0) |
| Mojibake por 1000 runas | 0,000 | 0,000 |
| Controles indevidos por 1000 runas | 2,391 (só c15: 15,34) | 0,000 |
| Tempo médio por PDF aberto ok (ms) | 23,0 | 9,7 |
| Pico de working set do processo (MB) | 325,6 (316,8 já após a inicialização) | 16,0 (8,4 após o início) |
| Pico de heap Go médio por PDF (MB) | 1,1 (não inclui WASM) | 0,7 |
| Determinismo (SHA-256, 2 execuções) | 15/15 idêntico | 15/15 idêntico |

Concordância A vs B: Jaccard médio por página 0,683 (por PDF 0,851); LCS médio por página 0,955 (por PDF 0,973); 33 páginas com texto nos dois, 0 páginas com texto só em um. Jaccard baixo com LCS alto (c09: 0,414 e 0,964) indica mesma ordem de leitura com vocabulário diferente (segmentação de palavras/espaços); o spike não mede qual dos dois está certo.

## 7. Custo de inicialização do WASM (motor A)

- Init (Init + GetInstance, compila o .wasm de 5,8 MB com o wazero): 5.479 ms nesta execução (medições anteriores do mesmo código: 6.072 ms e 10.801 ms; no programa de teste do cache: 4.617 ms). Varia de ~4,6 a ~10,8 s.
- Primeira chamada de extração (PDF c01, que falha rápido): 0,0 a 1,1 ms; o custo está todo no Init.
- Working set de pico logo após o Init: 316,8 MB (vs 8,4 MB do motor B).
- Teste extra com cache de compilação do wazero em disco (`WithCompilationCache`): 1ª execução 4.617 ms (preenche o cache), execuções seguintes **137 ms e 189 ms**; o cache ocupou 21.556.590 bytes em disco. O working set com cache não foi medido.

## 8. Renderização em bitmap (necessária para o OCR da etapa 7.9)

Teste do go-pdfium WASM com `RenderPageInDPI` (DPI 300, `ImageFormat = RenderImageFormatGrayscale`), em PDFs digitalizados do corpus:

| PDF | página | resultado | tamanho (px) | bytes | tempo (ms) | pixels não brancos | cinza médio |
|---|---|---|---|---|---|---|---|
| c03 | 1 | *image.Gray (8 bits) | 2480x3509 | 8.702.320 | 1.280 | 90,8% | 177,0 |
| c07 | 1 | *image.Gray | 2480x3505 | 8.692.400 | 229 | 8,3% | 245,1 |
| c07 | 2 | *image.Gray | 2480x3505 | 8.692.400 | 249 | 12,3% | 242,0 |
| c07 | 3 | *image.Gray | 2480x3505 | 8.692.400 | 263 | 9,0% | 245,9 |
| c05 | 1 | *image.Gray | 2480x3509 | 8.702.320 | 477 | 100% | 140,8 |

**Confirmado**: o go-pdfium WASM renderiza página em bitmap cinza 8 bits a 300 dpi (A4 = ~2480x3508), entregando `*image.Gray` direto, sem cgo. Imagem não vazia (pixels escuros presentes). Foi medido só o tempo de renderização; não foi feito OCR nem comparação visual de qualidade. O ledongthuc **não renderiza** (a biblioteca só lê conteúdo/texto; não há API de rasterização).

## 9. Tamanho de binário (`go build -ldflags "-s -w"`, programas mínimos)

| Binário | Bytes |
|---|---|
| Base atual `D:\MCP\mcpollama-mcp.exe` | 9.950.208 |
| Mínimo com go-pdfium WASM (abre PDF, extrai 1 página) | 12.372.480 |
| Mínimo com ledongthuc (idem) | 2.380.288 |

Projeção (não medida): somar ao servidor atual aproximadamente o que o mínimo A tem a mais que um "Hello" Go, na ordem de +10 MB com A e +1 MB com B. O .wasm tem 5,8 MB; o binário cresce mais que isso por causa do wazero. `InitWithWASM` permitiria não embutir o .wasm (~5 MB a menos) se o arquivo fosse distribuído à parte; não testado.

## 10. Limitações do spike

1. Sem referência pdftotext/poppler (`where.exe pdftotext` não achou nada; nada foi instalado). Corretude do texto foi avaliada só por métricas indiretas (acentos, U+FFFD, controles, concordância entre motores). Concordância alta não prova que ambos estão certos.
2. Corpus pequeno em texto: só 8 PDFs com texto, 33 páginas com texto, nenhum PDF grande com texto (os PDFs > 4 MB são 1 escaneado e 1 assinado de 12 páginas). Nenhum "Word com texto"; tabelas não avaliadas; 1 PDF (c01) não foi aberto por nenhum motor, e a causa (senha) não foi confirmada por segunda via.
3. A heurística "sem /Font" é de bytes; o resultado dos motores a confirmou nos 6 casos.
4. Memória do motor A só é visível pelo working set do processo (pico global, não por PDF, e acumulado com a inicialização de ~317 MB).
5. Tempos medidos com poucas execuções (2) numa estação, sem isolamento de carga; o Init do WASM variou de 4,6 a 10,8 s.
6. Qualidade do OCR e do renderizado (nitidez) não foram avaliados; só dimensão, tempo e conteúdo não vazio.
7. Mojibake: 0 nos dois motores neste corpus, então a métrica não discriminou.

## 11. Recomendação (versão inicial, corpus de 15 PDFs; a versão final está na seção 13)

**Adotar o go-pdfium WASM (A)** como motor de extração da Fase 7, com cache de compilação do wazero.

Fundamentos nos números:
- Empate técnico em abrir/falhar (8 com texto, 6 só imagem, 1 falha em ambos, 0 panics) e determinismo 15/15 nos dois.
- Qualidade: A extraiu 52.288 runas contra 36.861 do B nos mesmos 8 PDFs, com U+FFFD 0,000 contra 13,239 por 1000 (B chega a 51,69 em c15) e mais acentos por 1000 runas (21,46 contra 17,42). Em contrapartida A mostrou 2,391 controles por 1000 (todos em c15), que B não mostrou; esses controles são filtráveis na camada do servidor. Sem pdftotext, não é possível afirmar qual texto é o correto, mas B perde caracteres/texto onde A não perde.
- A única opção que **renderiza** (300 dpi, cinza 8 bits, 229 a 1.280 ms por página), o que a etapa 7.9 (OCR) exige. Escolher B obrigaria a trazer outra dependência só para renderizar, e então A já estaria no binário.
- Custos de A: binário mínimo de 12,4 MB contra 2,4 MB do B (ordem de +10 MB no servidor, projeção), ~317 MB de working set após o Init (vs 8 MB de B) e Init de 4,6 a 10,8 s sem cache. O cache do wazero derrubou o Init a 137 a 189 ms (custo: ~21,6 MB de cache em disco, memória com cache não medida). Para um MCP que sobe por sessão, vale decidir onde guardar o cache e se o Init pode ser preguiçoso (só no primeiro PDF).
- Risco a aceitar: 300+ MB de RAM residente ao usar PDF; considerar inicializar o WASM sob demanda e fechar o pool após ocioso.
- B só faz sentido se o owner abrir mão de OCR e de memória/binário maiores e aceitar perda de texto e U+FFFD em alguns PDFs; para PDFs só texto simples, B é 2x mais rápido e 20x mais leve em memória.


---

## 12. Ampliação do corpus (c16…c35)

Pedido do owner: o corpus inicial tinha só 8 PDFs com texto. Foram adicionados 20 PDFs (c16 a c35), copiados e renomeados do mesmo modo (mapa só em `mapa.txt`), com as mesmas regras de sigilo. Os 35 PDFs foram reprocessados pelos dois motores, 2 execuções cada (a seção 5 permanece com a primeira rodada de c01 a c15; os tempos dessa reexecução variaram, ver nota de ruído abaixo).

### 12.1 Composição do corpus novo

Seleção por varredura de bytes (658 PDFs elegíveis, 611 com /Font; 1 deles acima de 4 MB). Contagem por categoria (um PDF pode cair em várias):
- Com camada de texto (/Font): 20 de 20 selecionados (18 abrem e têm texto; 2 são os /Encrypt com senha de abertura, ver 12.2).
- Produtor (classificação por texto de /Producer e /Creator): 7 "outros", 6 "web/geradores de relatório", 7 "impressoras/conversores PDF". **Office/Word com texto: 0**: nesta pasta todo PDF marcado como Word/Microsoft não tem texto (só imagem), então a categoria pedida não existe. Tribunal/cartório: não identificável pelo produtor, não reivindicado.
- Várias páginas (>= 5): **7** (c18 8 pág., c20 11, c24 9, c25 12, c26 14, c27 9, c28 5).
- Indício de tabela: **6** (c21, c22, c23, c29, c30, c33), por densidade de operadores de linha/retângulo (`re`/`l`) acima de 300 por página, medida só nos fluxos Flate legíveis; nenhum produtor de planilha/ERP foi identificado. É um indício, não confirmação de tabela.
- /Encrypt: **2** (c16, c17), mas **ambos com senha de abertura**, não só de permissão (12.2). Nenhum outro PDF /Encrypt da pasta abre com senha vazia além de c02: dos 8 PDFs com /Encrypt da pasta, só c02 é "senha só de permissão". A cota de 2 permissão-só não pôde ser atendida por inexistência.
- Maior que 4 MB com texto: **1** (c18, 4,24 MB; só existe esse na pasta com /Font).
- Assinados com texto: **3** (c18, c19, c20).
- Duplicata exata detectada por hash e substituída (a seleção inicial trouxe 2 cópias idênticas e 3 quase idênticas); os 35 PDFs do corpus final têm hashes distintos.

### 12.2 Investigação do c01 (e dos outros /Encrypt)

**Conclusão: senha de ABERTURA exigida; recusa legítima, não falha do motor.** Evidências por metadados/bytes (sem ler conteúdo):
- c01: filtro Standard, V=1, R=3, chave de 40 bits, /O e /U com 32 bytes cada, /ID presente.
- Verificação independente do algoritmo de validação da senha de usuário (chave derivada com senha vazia, /O, /P, /ID, 50 rodadas de MD5, RC4 com 20 passos) implementada à parte no spike: **a senha vazia não valida contra o /U do c01**. No controle c02 (R=2) a mesma verificação **valida** (e o c02 abre nos dois motores). Limitação: não há controle positivo para R=3, então a verificação R=3 não está provada isoladamente; ela concorda com o PDFium, abaixo.
- go-pdfium, abertura explícita com `Password` nulo e com `Password` vazio: ambas devolvem "invalid password" (código 4 do PDFium), idêntico para c01, c16 (R=2) e c17 (R=3). O c02 abre nas duas formas.

### 12.3 Resultados por PDF (c16 a c35; A = go-pdfium WASM, B = ledongthuc)

Colunas: "/Count" lido dos bytes do PDF (raiz /Type /Pages); "pág. A/B" = páginas devolvidas por cada motor; ms/pág = tempo da extração total dividido pelo número de páginas.

| PDF (categoria) | /Count | pág. A/B | sem texto A/B | falha A | falha B | A ms | B ms | A ms/pág | B ms/pág | acentos/1000 A | B | FFFD/1000 A | B | mojib. A/B | ctrl/1000 A | B | Jaccard | LCS | determ. A/B |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| c16 (other,enc) | 3 | 0/0 | 0/0 | senha/encrypt | senha/encrypt | 1 | 1 | 0.6 | 1.2 | 0.0 | 0.0 | 0.00 | 0.00 | 0/0 | 0.00 | 0.00 | n/a | n/a | id./id. |
| c17 (other,enc) | 4 | 0/0 | 0/0 | senha/encrypt | senha/encrypt | 1 | 1 | 0.5 | 0.6 | 0.0 | 0.0 | 0.00 | 0.00 | 0/0 | 0.00 | 0.00 | n/a | n/a | id./id. |
| c18 (web,big) | 8 | 8/8 | 0/0 | ok | ok | 35 | 3 | 4.4 | 0.4 | 18.7 | 15.7 | 0.00 | 0.00 | 0/0 | 0.00 | 0.00 | 0.498 | 0.964 | id./id. |
| c19 (other,signed) | 1 | 1/1 | 0/0 | ok | ok | 5 | 3 | 4.5 | 2.9 | 24.2 | 25.0 | 0.00 | 0.00 | 0/0 | 0.00 | 0.00 | 0.889 | 0.968 | id./id. |
| c20 (other,signed) | 11 | 11/11 | 0/0 | ok | ok | 154 | 6 | 14.0 | 0.5 | 21.8 | 15.5 | 0.00 | 0.00 | 0/0 | 0.00 | 0.00 | 0.436 | 0.964 | id./id. |
| c21 (web,table) | 1 | 1/1 | 0/0 | ok | ok | 21 | 34 | 21.1 | 33.9 | 28.8 | 27.6 | 0.00 | 0.00 | 0/0 | 0.00 | 0.00 | 1.000 | 1.000 | id./id. |
| c22 (printer,table) | 2 | 2/2 | 0/0 | ok | ok | 30 | 17 | 14.8 | 8.6 | 30.1 | 29.5 | 0.00 | 0.00 | 0/0 | 0.00 | 0.00 | 0.995 | 0.999 | id./id. |
| c23 (printer,table) | 2 | 2/2 | 0/2 | ok | paginas_com_erro:2 | 50 | 2 | 25.0 | 1.0 | 29.6 | 0.0 | 0.00 | 0.00 | 0/0 | 0.00 | 0.00 | n/a | n/a | id./id. |
| c24 (printer,multi) | 9 | 9/9 | 0/0 | ok | ok | 53 | 42 | 5.9 | 4.7 | 28.0 | 28.3 | 0.00 | 0.00 | 0/0 | 0.00 | 0.00 | 0.955 | 0.976 | id./id. |
| c25 (printer,multi) | 12 | 12/12 | 0/2 | ok | paginas_com_erro:2 | 110 | 46 | 9.1 | 3.9 | 28.0 | 27.5 | 0.00 | 0.00 | 0/0 | 0.00 | 0.00 | 0.956 | 0.976 | id./id. |
| c26 (web,multi) | 14 | 14/14 | 0/9 | ok | ok | 98 | 15 | 7.0 | 1.0 | 25.9 | 9.8 | 0.00 | 0.00 | 0/0 | 0.00 | 0.00 | 0.165 | 0.991 | id./id. |
| c27 (other,multi) | 9 | 9/9 | 0/0 | ok | ok | 57 | 4 | 6.3 | 0.4 | 24.3 | 18.8 | 0.00 | 0.00 | 0/0 | 0.48 | 0.00 | 0.416 | 0.966 | id./id. |
| c28 (printer,multi) | 5 | 5/5 | 1/1 | ok | ok | 17 | 9 | 3.3 | 1.9 | 28.8 | 28.4 | 0.00 | 0.00 | 0/0 | 0.00 | 0.00 | 1.000 | 1.000 | id./id. |
| c29 (printer,g-printer) | 2 | 2/2 | 0/2 | ok | paginas_com_erro:2 | 58 | 2 | 28.9 | 0.8 | 29.9 | 0.0 | 0.00 | 0.00 | 0/0 | 0.00 | 0.00 | n/a | n/a | id./id. |
| c30 (other,g-other) | 4 | 4/4 | 1/1 | ok | ok | 41 | 11 | 10.3 | 2.8 | 21.7 | 21.1 | 0.00 | 93.91 | 0/0 | 7.70 | 1.14 | 0.551 | 0.739 | id./id. |
| c31 (web,g-web) | 1 | 1/1 | 0/0 | ok | ok | 9 | 6 | 8.9 | 5.5 | 31.0 | 30.0 | 0.00 | 0.00 | 0/0 | 0.00 | 0.00 | 1.000 | 1.000 | id./id. |
| c32 (other,g-other) | 1 | 1/1 | 0/0 | ok | ok | 6 | 2 | 5.6 | 2.0 | 26.4 | 24.5 | 0.00 | 0.00 | 0/0 | 0.00 | 0.00 | 0.800 | 0.929 | id./id. |
| c33 (web,g-web) | 2 | 2/2 | 0/0 | ok | ok | 39 | 16 | 19.4 | 8.0 | 35.5 | 34.4 | 0.00 | 0.00 | 0/0 | 0.00 | 29.23 | 0.514 | 0.683 | id./id. |
| c34 (web,g-web) | 1 | 1/1 | 0/0 | ok | ok | 9 | 5 | 8.9 | 5.1 | 28.9 | 28.0 | 0.00 | 0.00 | 0/0 | 0.00 | 0.00 | 1.000 | 1.000 | id./id. |
| c35 (printer,g-printer) | 1 | 1/1 | 0/0 | ok | ok | 7 | 4 | 7.4 | 3.6 | 16.9 | 15.3 | 0.00 | 0.00 | 0/0 | 0.00 | 0.00 | 0.992 | 0.994 | id./id. |

Notas: c16 e c17 não abrem em nenhum motor (senha de abertura). B devolveu erro em páginas de c23, c25 e c29 (2 páginas cada) e nenhum texto nelas; A não teve erro de página em nenhum PDF.

### 12.4 Agregados (corpus ampliado e total)

| Métrica | A c16-c35 | B c16-c35 | A total (35) | B total (35) |
|---|---|---|---|---|
| PDFs | 20 | 20 | 35 | 35 |
| Com texto extraído | 18 | 16 (3 com páginas com erro) | 26 | 24 (3 com páginas com erro) |
| Só imagem | 0 | 0 | 6 | 6 |
| Não abriu (senha de abertura) | 2 | 2 | 3 (c01, c16, c17) | 3 |
| Páginas / páginas sem texto | 86 / 2 | 86 / 17 | 133 / 16 | 133 / 31 |
| Runas extraídas | 203.218 | 101.310 | 255.506 | 138.171 |
| Acentos por 1000 | 25,89 | 25,03 | 24,99 | 23,00 |
| U+FFFD por 1000 | 0,000 | 8,153 | 0,000 | 9,510 |
| Mojibake por 1000 | 0,000 | 0,000 | 0,000 | 0,000 |
| Controles por 1000 | 0,374 | 1,056 | 0,787 | 0,774 |
| Tempo médio por PDF que abriu (ms)* | 53,6 | 12,0 | 40,8 | 29,6 |
| Pico de heap Go médio por PDF (MB) | 1,00 | 1,06 | 1,05 | 1,02 |
| Determinismo (SHA-256, 2 execuções) | 20/20 | 20/20 | 35/35 | 35/35 |

\* média das 2 execuções, sobre todos os PDFs que abriram (inclui os só imagem), definição diferente da seção 6; no B os tempos dos PDFs só imagem c10 e c11 oscilaram de 41/100 ms (rodada anterior) para 231/675 ms nesta rodada (mesmo arquivo), então a média do B no corpus antigo é ruidosa. Pico de working set do processo nos 35 PDFs: A 320,6 MB, B 15,9 MB (A, sem cache de compilação).

Pontos específicos:
- Onde B perde texto: c26 (14 pág.) tem 9 páginas sem texto no B e 0 no A (acentos por 1000: 9,8 contra 25,9); c23, c25 e c29 têm páginas com erro no B. O motor B perdeu 15 páginas com texto que o A extraiu, em 4 PDFs.
- U+FFFD no B: c30 com 93,91 por 1000 (A: 0). Controles: A c30 7,70 por 1000, B c33 29,23 por 1000.
- Concordância A vs B (c16 a c35, 16 PDFs e 79 páginas com texto nos dois): Jaccard médio por página 0,612 (por PDF 0,761); LCS médio por página 0,961 (por PDF 0,947). Mínimos de LCS por PDF: c30 0,739 e c33 0,683 (justamente os que têm U+FFFD/controles no B ou indício de tabela).

### 12.5 (a) Tabelas (6 PDFs com indício)

| PDF | linhas de texto A | linhas B | páginas comparadas | Jaccard médio/pág | LCS médio/pág |
|---|---|---|---|---|---|
| c21 | 24 | 67 | 1 | 1.000 | 1.000 |
| c22 | 128 | 205 | 2 | 0.995 | 0.999 |
| c23 | 126 | 0 | 0 | 0.000 | 0.000 |
| c29 | 122 | 0 | 0 | 0.000 | 0.000 |
| c30 | 239 | 21 | 3 | 0.551 | 0.739 |
| c33 | 81 | 108 | 2 | 0.514 | 0.683 |

Leitura: c21 e c22 batem quase perfeitamente na ordem (LCS 1,000 e 0,999), mas o B conta mais linhas de texto (67 contra 24 e 205 contra 128), isto é, quebra a mesma página em mais linhas (a contagem de linhas depende da segmentação do motor; este spike não mede qual é a correta). Em c23 e c29 o B não devolve texto (0 linhas) e o A devolve 126 e 122 linhas. Em c30 e c33 a ordem diverge (LCS 0,739 e 0,683) e a contagem de linhas também (A 239 contra B 21 em c30). Nenhum juízo de qual leitura de tabela é a correta (sem referência).

### 12.6 (b) Multi-página

- Contagem de páginas: nos 18 PDFs do corpus novo que abriram, o /Count lido dos bytes **confere com as páginas devolvidas pelos dois motores em 18 de 18** (0 divergências; o /Count ficou visível nos bytes dos 20, inclusive dos 2 cifrados).
- Tempo por página nos 7 PDFs com 5 ou mais páginas: A 7,14 ms/pág., B 1,83 ms/pág. (B ~3,9x mais rápido por página; ambos abaixo de 15 ms/pág. em média; o maior tempo total foi 154 ms no A para c20 com 11 páginas).

### 12.7 (c) Cache de compilação do wazero: working set e custo de reabrir o motor

Programa de teste: `WithCompilationCache(NewCompilationCacheWithDir(...))`, 3 ciclos de "inicializar, extrair 23 páginas (c20 e c25), fechar instância e pool, GC e devolução de memória ao SO, esperar 1,5 s". Working set atual do processo (não o pico histórico), em MB:

| Cenário | Init (ms) | WS após init | WS após 23 páginas | WS após fechar + GC |
|---|---|---|---|---|
| Processo 1, cache vazio, ciclo 1 (compila) | 4.935 | 330,9 | 339,0 (pico) | 50,8 |
| Processo 1, ciclo 2 (reabrir, cache em uso) | 113 | 80,4 | 81,3 | 50,9 |
| Processo 1, ciclo 3 | 150 | 80,4 | 81,0 | 50,9 |
| Processo 2 (novo, cache já em disco), ciclo 1 | 149 | 68,2 | 70,6 | 46,6 |
| Processo 2, ciclo 2 | 126 | 76,9 | 79,7 | 46,9 |
| Processo 2, ciclo 3 | 133 | 76,4 | 77,1 | 46,9 |

Base do processo sem nenhum motor: 7,6 MB. Cache em disco: 21.556.590 bytes. Fechar instância e pool leva 0 a 1 ms.

Conclusões medidas: (1) com cache o working set em uso cai de ~317-339 MB (compilação) para **68-81 MB** com o motor aberto e extraindo; (2) o custo de **reabrir** o motor depois de fechado com cache é **113 a 150 ms** (reaberturas com cache: 113, 126, 133, 149 e 150 ms; 4.935 ms só na primeira compilação, sem cache); (3) depois de fechar, o processo ainda retém ~47-51 MB (contra 7,6 MB da base), por isso liberar o motor após 10 min ocioso devolve ~20-30 MB do uso com motor aberto (68-81 MB), não todo o excedente; esse resíduo não foi investigado (provavelmente cache de código compilado mapeado/retido pelo runtime); (4) o pico de ~339 MB só ocorre na compilação sem cache (primeira execução do usuário, ou após apagar o cache).

### 12.8 Limitações adicionais deste ciclo

1. Ainda sem referência pdftotext; a correção do texto de tabelas e da ordem não foi julgada.
2. Nenhum Word/Office com texto existe na pasta; a cobertura de "Office com texto" ficou sem teste.
3. O indício de tabela é por operadores gráficos (não confirma tabela); nenhum produtor de planilha/ERP foi detectado.
4. A cota de 2 PDFs /Encrypt de permissão-só não é atingível (só c02 existe).
5. Verificação R=3 da senha vazia sem controle positivo próprio; confirmada pelo PDFium.
6. Tempos com 2 execuções, sem isolamento de carga; ruído observado de até ~6x em tempos curtos do B nos PDFs só imagem.

## 13. Recomendação final (atualizada com o corpus ampliado, 35 PDFs)

**Mantida: adotar o go-pdfium WASM (A) com cache de compilação do wazero e inicialização sob demanda.** O corpus ampliado reforça a decisão:
- **Cobertura de texto:** no total, A extraiu texto de 26 dos 32 PDFs que abrem (os demais 6 são só imagem), sem nenhuma página com erro; B extraiu de 24 e teve 3 PDFs com páginas com erro. B perdeu 15 páginas com texto em 4 PDFs (c23, c25, c26, c29) que o A extraiu.
- **Qualidade:** U+FFFD 0,000 (A) contra 9,510 por 1000 (B); runas extraídas 255.506 (A) contra 138.171 (B); acentos por 1000 24,99 contra 23,00; mojibake 0 nos dois; controles equivalentes no total (0,787 e 0,774). Determinismo 35/35 nos dois.
- **Estabilidade:** nenhum panic e nenhum timeout nos dois motores em 35 PDFs x 2 execuções.
- **Renderização:** só o A renderiza (seção 8: 300 dpi, cinza 8 bits, 229 a 1.280 ms por página), necessária à etapa 7.9 (OCR).
- **Custo do A, agora com números do cache:** 68 a 81 MB de working set com o motor aberto (contra 317-339 MB na compilação sem cache e 8-16 MB do B); reabrir o motor depois de fechado custa 113 a 150 ms; resíduo de ~47-51 MB após fechar (investigar se o resíduo importa); +10 MB de binário (projeção) e ~21,6 MB de cache em disco (decidir o diretório; sem cache o primeiro uso custa ~5 s).
- **Política de ociosidade (10 min):** viável. Fechar e reabrir sai por ~0,1 a 0,15 s com cache, então o custo de reiniciar após o ocioso é pequeno; sem cache seria 4,6 a 10,8 s e 330 MB de pico, portanto **o cache de compilação é pré-requisito**.
- **Onde A custa mais:** 7,14 ms/pág. contra 1,83 do B nos PDFs com 5+ páginas; irrelevante frente ao tempo do modelo local.
- **Pendências para o owner/Fase 7:** (1) mapear PDFs de senha de abertura como recusa explícita ("PDF protegido por senha"), já que 3 dos 35 (c01, c16, c17) são assim e o c02 (permissão-só) abre; (2) filtrar caracteres de controle na saída do A (c30: 7,70 por 1000); (3) tabelas: a ordem de leitura de c30 e c33 diverge entre motores e não há referência para julgar; considerar teste com poppler antes de prometer qualidade em tabelas; (4) a contagem de linhas de tabela difere entre motores, então não usar linhas como contrato de saída.
- B só se justifica sem OCR e com tolerância a perda de texto e U+FFFD; no corpus ampliado ele falhou em mais PDFs sem avisar (texto vazio em páginas), o que é o pior caso para um extrator.

