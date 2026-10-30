# Aviso de uso

**mcpollama** e **mcpollama-gateway** são **software proprietário da CSL Soluções, de uso interno**. O repositório não tem arquivo
de licença e **não concede** direito de cópia, redistribuição, sublicenciamento ou uso fora da CSL sem autorização escrita da empresa.

## Conteúdo sensível

O repositório é privado e contém dados e configurações de infraestrutura da CSL: nomes de host, endereços da LAN, portas, regras de
nginx, unidades de systemd, limites de energia das GPUs, planos e relatórios de operação. **Não há segredos versionados** (token
do Ollama, senha do `/monitor`, chaves de certificado e token do túnel ficam fora do git), mas os arquivos continuam sendo
informação interna: não publique nem compartilhe o repositório ou trechos dele.

## Componentes de terceiros

O código usa as bibliotecas abaixo (versões de `src/go.mod`). Cada uma é distribuída sob a licença do **próprio repositório de
origem**, que vale para ela; este aviso não as relicencia.

| Componente | Uso | Origem |
| --- | --- | --- |
| go-pdfium (`github.com/klippa-app/go-pdfium`) e **PDFium** | leitura de `.pdf` (motor de PDF em WebAssembly) | <https://github.com/klippa-app/go-pdfium>, <https://pdfium.googlesource.com/pdfium/> |
| wazero (`github.com/tetratelabs/wazero`) | execução do PDFium em WASM, sem cgo | <https://github.com/tetratelabs/wazero> |
| modernc SQLite (`modernc.org/sqlite`, e `libc`, `mathutil`, `memory`) | banco do gateway, sem cgo | <https://gitlab.com/cznic/sqlite> |
| `golang.org/x/image`, `x/sys`, `x/text` | imagens do OCR, chamadas de sistema, texto | <https://go.googlesource.com/> |
| Demais dependências indiretas | ver `src/go.mod` e `src/go.sum` | repositório de cada módulo |
| **Tesseract OCR** 5.3.3 | OCR (opcional): a DLL (`libtesseract64.dll`) e os modelos `tessdata` ficam **fora do repositório**, em `D:\MCP`, e são carregados por `--ocr-lib` com conferência de SHA-256 | <https://github.com/tesseract-ocr/tesseract> |

Os modelos de linguagem (`qwen3.5:9b`, publicado no servidor como `qwen3-ia`, e `nomic-embed-text`) e o **Ollama** são de terceiros e
seguem as licenças dos respectivos autores. Eles rodam no servidor de IA e não fazem parte deste repositório.

Antes de redistribuir qualquer binário ou parte do código, confira as licenças de todos os componentes acima nos repositórios
de origem.
