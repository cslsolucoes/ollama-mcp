# Testes de ataque da revisão adversarial da Fase 12 (etapa 12.9)

Report: `../MCPOllam2026-11-04-022324-revisao-adversarial-fase12.report.md`. Alvo: HEAD `fe8be32` (gateway 0.13.0 e mcpollama 0.10.0 ainda não implantados).

Cada arquivo imprime uma linha `VEC|<id>|OK|<descrição>` ou `VEC|<id>|FALHA|<severidade> <arquivo:linha> <descrição>` por vetor.
Um vetor `FALHA` também falha o teste (`t.Error`): **no `fe8be32` os testes abaixo marcados "falha esperada" falham de propósito**,
até o achado ser corrigido. Depois da correção, o mesmo arquivo vira teste de regressão.

## Arquivos (caminho = caminho do pacote a partir de `src/`)

| Arquivo | Testes | Falha esperada no `fe8be32` | Helpers de outros `_test.go` do pacote |
| --- | --- | --- | --- |
| `gateway/treino/fase12_adv_feedback_test.go` | `TestAdv12F*` | `FForeignLinkForcesQuarantine`, `FNoTextNoPlainHashAndRemanence`, `FBodyAndJSON` | `treino_test.go` (`openStore`, `testKey`, `newFake`/`fakeSrc`, `mkID`, `defJob`, `count`), `state_test.go` (`ctx`, `admin`, `must`, `dumpTable`), `race_*_test.go` (`raceEnabled`) |
| `gateway/treino/fase12_adv_export_test.go` | `TestAdv12E*` | `EE1JSONLInjection`, `EE2SinglePredicate`, `EE3LeakAcrossTime`, `EE3CotaShownAboveThirty` | `treino_test.go` (`openStore`, `testKey`, `newFake`, `mkID`, `pjOf`, `report`, `jr`, `mustValid`, `stationO`), `state_test.go` (`ctx`, `admin`, `must`), `fase126_test.go` (`sideOf`, `gidOf`) |
| `cmd/mcpollama-gateway/fase12_adv_feedback_test.go` | `TestAdv12F*` | nenhuma | `fase12_feedback_main_test.go` (`startGW`, `gw.jobDone`), `fase12_main_test.go` (`writeDocKey`, `docKeyHex`) |
| `cmd/mcpollama-gateway/fase12_adv_proxy_test.go` | `TestAdv12P*` | nenhuma | `main_test.go` (`noEnv`), `fase12_main_test.go` (`docKeyHex`) |
| `gateway/monitor/fase12_adv_monitor_test.go` | `TestAdv12M*` | `MM4M5ExportOverRealServer`, `MM7ActionBodies` | `monitor_test.go` (`newFixture`, `do`, `login`, `adminUser`/`adminPass`), `treino_test.go` (`tid`, `tsrc`, `tfix`, `newTreinoFixture`, `occ`, `adm`, `count`, `openDB`, `stOrigin`); lê os `.go` do pacote por go/ast (rodar no pacote) |
| `gateway/proxy/fase12_adv_proxy_test.go` | `TestAdv12P*` | `PDocTrailerOverHTTP2Backend`, `PMelhoraSearchesChargeTheLoopbackQuota` | `proxy_test.go` (`setup`, `env`, `post`, `postBg`, `finalChunk`), `policy_test.go` (`capture`, `defaultPolicy`), `promptjson_test.go` (`testKey`), `melhora_test.go` (`newFakeOllama`, `withMelhora`, `fileReq`, `exampleIndex`) |
| `gateway/hmackey/fase12_adv_hmackey_test.go` | `TestAdv12P*` | `PKeyNeverPrinted` | nenhum |
| `gateway/melhora/fase12_adv_melhora_test.go` | `TestAdv12P*` | `PExampleCannotCarryAFileMarker`, `PApplyKeepsWhatTheOllamaSees`, `PIndexSwappedDuringTheSearch` | nenhum |
| `gateway/melhora/feed/fase12_adv_melhora_feed_test.go` | `TestAdv12P*` | nenhuma | `feed_test.go` (`env`, `src`, `jobID`, `admin`, `ctx`, `fakeEmbed`, `env.occurrence`) |
| `cmd/bakeoff/fase12_adv_bakeoff_test.go` | `TestAdv12B*` | `BS2S3OfACandidateWithInstructionsIsNeverMeasured`, `BUnequalNIsSkippedNotRejected`, `BStreamShapes`, `BNumberScoring`, `BBaseURLPathIsNotAnEndpointBypass` | `erros_test.go`, `erros_fix_test.go`, `erros_eval_test.go`, `erros_melhora_test.go` |
| `tools/fase12_adv_tools_test.go` | `TestAdv12B*` | `BSchemaMatchesTheHandler` | `helpers_test.go` (`newMock`, `call`, `entry`, `text`, `sentinel`), `jobs_test.go` (`jobID`), `reportar_erro_test.go` (`newFBMock`, `fbToken`, `repArgs`), `autoreport_test.go` (`newAutoMock`, `stopWith`) |
| `ollama/fase12_adv_ollama_test.go` | `TestAdv12B*` | nenhuma | `client_test.go` (`newMock`, `newClient`, `token`), `feedback_test.go` (`fbReport`, `fbTok`) |

Todos os nomes de nível de pacote têm prefixo próprio (`adv12F`, `adv12E`, `adv12M`, `adv12P`, `adv12B`, e `vec12X`), então os
arquivos convivem no mesmo pacote entre si e com os ataques 9.5, 10.L e 10.5.

## Como rodar

Copie os arquivos para os pacotes (o caminho dentro desta pasta é o caminho a partir de `src/`), rode e **remova as cópias**.
Sem rede externa, sem servidor real, sem `-tags live`. Para não escrever no D:, use cache e temporários no E:.

```bash
cd /e/MCPOllam
S=.workspace/reports/MCPOllam2026-11-04-022324-revisao-adversarial-fase12.testes
(cd $S && find . -name '*_test.go') | while read f; do cp "$S/$f" "src/${f#./}"; done
cd src
export GOCACHE='E:\MCPOllam\.tmp-adv\gocache' TMP='E:\MCPOllam\.tmp-adv\tmp' TEMP='E:\MCPOllam\.tmp-adv\tmp' GOTMPDIR='E:\MCPOllam\.tmp-adv\tmp'
go test -count=1 -v -run 'TestAdv12' ./cmd/mcpollama-gateway ./cmd/bakeoff ./gateway/... ./tools ./ollama | grep 'VEC|'
# depois: apagar as cópias e conferir `git status --short src` vazio
```

Corrida (PowerShell):

```powershell
$env:PATH="E:\MCPOllam\deps\gcc-mingw-w64\bin;$env:PATH"; $env:CC="gcc"; $env:CGO_ENABLED='1'
go test -race -count=1 -run 'TestAdv12' ./gateway/treino ./gateway/monitor ./gateway/proxy ./gateway/melhora/... ./tools
Remove-Item Env:CC,Env:CGO_ENABLED
```

Para a revisão completa, rode também os ataques guardados da 9.5 (`../MCPOllam2026-10-28-235633-revisao-adversarial-fase9.testes`,
corrigidos nesta revisão para o `swrCache` da 0.12.6), da 10.L e da 10.5 com `-run 'TestAdv'`.

## Notas

- Testes de modo 0600 não valem no Windows (o `checkMode` ignora o modo); rodar no Linux do servidor na 12.10.
- `logs/`: saídas das rodadas desta revisão (`todos-rodada1.log`: ataques antigos + novos juntos; `fase12-rodada2.log`; `fase12-race.log`;
  `antigos-completo.log`; e os logs por revisor, `<X>-<pacote>.log`).
