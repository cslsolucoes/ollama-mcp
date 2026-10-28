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
