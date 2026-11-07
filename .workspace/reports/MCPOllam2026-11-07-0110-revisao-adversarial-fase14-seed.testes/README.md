# Testes de ataque — S4 da Fase 14 (`seed` em `generate` e `chat`)

Revisão adversarial de 2026-11-07 (Opus 5.5), sobre a mudança não commitada do ramo `feat/seed-generate-chat`.

**Como rodar:** copiar `seed_attack_test.go` para `src/tools/zz_seed_attack_test.go`, rodar
`go test -count=1 -run TestAtkSeed -v ./tools` (dentro de `src/`) e **apagar a cópia**. Usa os auxiliares de
`helpers_test.go`, `think_test.go` e `seed_test.go` (`newMock`, `call`, `seedTools`, `seedOnWire`).
Saída da rodada: `ataques-rodada1.log` (9 testes, 15 linhas `VEC|...|OK`, PASS). Também passou num binário de teste
`GOARCH=386` (int de 32 bits).

| Ataque | O que prova |
| --- | --- |
| A01 | Corpo **cru** enviado: o `seed` sai como literal inteiro (`2147483647`, `1e3` vira `1000`), e um `seed` sozinho gera `"options":{"seed":N}` |
| A02 | 31 literais: `1E3`, `1.0e0`, `21474836.47e2` aceitos; **`2147483647.0000000001` e `0.99999999999999999` aceitos por arredondamento de float64** (viram 2147483647 e 1); `1e308`, 2^53+1, negativos, `-0.0`, `0.0`, `0e10`, `1e-400` (vira 0) recusados por faixa; decimal, string, `"7"`, array, objeto e booleano recusados por tipo; `1e400`, `NaN`, `Infinity`, `+1`, `01`, `0x10`, `.5`, `1.` caem na mensagem genérica "os argumentos devem ser um objeto JSON"; nenhum recusado chega ao Ollama |
| A03 | Chave repetida: vale a **última** (`{"seed":0,"seed":5}` é aceito como 5; `{"seed":5,"seed":0}` é recusado; `{"seed":5,"seed":null}` = omitido) |
| A04 | `Seed`, `SEED`, `"seed "` e o homóglifo cirílico são recusados como argumento desconhecido; `"seed"` decodifica para `seed` e é aceito |
| A05 | Argumentos com BOM, como array, string ou número: mensagem genérica; `null`: "`prompt` is required"; `seed` antes de `prompt` funciona |
| A06 | Ordem de validação: `temperature`, `num_predict`, `seed`, `think`; mensagem de faixa no mesmo formato dos irmãos |
| A07 | Com o primário fora (conexão recusada), o **fallback** recebe o mesmo `options.seed` |
| A08 | `seed` + `think:false` + modelo por chamada chegam juntos; modelo proibido com `seed` válido é recusado antes do Ollama |
| A09 | `samplingOptions` nas 8 combinações de presença: `nil` só quando os três faltam; `top_p` nunca preenchido; `temperature:0` preservado; `numPredictOf` inalterado |

Mutações feitas (todas restauradas, `git diff` conferido igual ao inicial): (1) tirar `seed == nil` da condição de
`nil` em `samplingOptions`; (2) mínimo do `seed` de 1 para 0; (3, extra) tirar `"seed"` do `parseArgs` do `generate`.
