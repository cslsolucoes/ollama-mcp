# Contrato de dados da Fase 12 (erros do modelo local → base de treino)

> **Versão:** 1.1.4 · **Estado:** ✅ APROVADO pelo owner (1.0.0 em 2026-11-02 20:05; 1.1.0 às 20:33; 1.1.1 às 20:40; 1.1.2 às 23:40; 1.1.3 em 2026-11-03 09:59; 1.1.4 às 10:21) (etapa 12.1) · **Data:** 2026-11-02
> **Fonte única:** `.workspace/plans/MCPOllam2026-11-01-213500-fase12-erros-treino.plan.md` (decisões D12-1 a D12-17, §5, §8, §10, §13, §14, §15).
> Este documento só fixa regras; o raciocínio e o histórico estão no plano.

## 0. Objetivo, escopo e precedência

- **C-0.1** Este contrato define, para a Fase 12, os dados que o gateway 0.13.0 e o mcpollama 0.10.0 guardam, exportam, particionam e medem sobre os erros do modelo local.
- **C-0.2** O contrato vale para: captura do pedido (`content.db`), `treino.db`, `POST /api/feedback`, exportação JSONL, partição treino/teste, protocolo de avaliação (`bakeoff -suites erros`) e melhora sem treino (D12-15). Fica fora o treino LoRA (plano §10).
- **C-0.3** Se o código divergir deste contrato, o contrato vence, ou ele é alterado com aprovação explícita do owner; o código nunca altera o contrato por conta própria.
- **C-0.4** Toda etapa a partir da 12.2 tem de testar contra estas regras; cada regra `C-<cap>.<n>` tem de ter ao menos um teste que falha se ela for violada.
- **C-0.5** Enquanto o estado for "proposta", nenhum código das etapas 12.2 em diante é iniciado. (Aprovado em 2026-11-02 20:05: a 12.2 pode começar.)
- **C-0.6** Mudança de regra incrementa a versão deste documento (semver) e é registrada no changelog do plano.

## Glossário

| Termo | Significado |
| --- | --- |
| ocorrência | o relato lógico de um resultado errado; unidade do erro |
| job | uma chamada ao gateway, com ID próprio (32 hex minúsculos, `X-Request-Id`) |
| papel | função de um job na ocorrência: `map1..mapN`, `reduce1..reduceN`, `final`, `unico` |
| `job_do_erro` | o job que produziu o erro; é a base da amostra exportável |
| amostra | cópia, em `treino.db`, do pedido canônico e da resposta de um job |
| pedido canônico | o corpo JSON que o Ollama recebe, depois da política (`prompt_json`) |
| grupo | componente conectado de ocorrências ligadas por `doc_hash` ou por job em comum |
| partição | `treino`, `teste` ou `quarentena` (coluna `particao`, não é estado) |
| retrato | lista congelada de casos de teste usada numa avaliação |
| linha de base | primeira avaliação do `qwen3-ia` atual, sem D12-15, num retrato |
| admin | usuário `admin` do `/monitor`; única fronteira de confiança |

## 1. Unidade do erro

- **C-1.1** A unidade do erro é a **ocorrência**: um resultado errado visto pelo Claude ou pelo usuário; `id` = 32 hex minúsculos.
- **C-1.2** Uma ocorrência tem um ou mais jobs em `ocorrencia_jobs` (PK `ocorrencia_id`+`job_id`), cada um com `papel` ∈ {`map1..`, `reduce1..`, `final`, `unico`}, `backend` ∈ {`primario`, `fallback`} e `ordem`.
- **C-1.3** Cada ocorrência tem exatamente um `job_do_erro`, que pertence à lista de jobs dela; quando não informado, o padrão é o job de papel `final`; com um único job, é o `unico`. Em `translate` (sem `final`) e em qualquer lista com mais de um job e sem `final`, o `job_do_erro` é **obrigatório** e o relato sem ele é recusado.
- **C-1.4** O gateway que recebe o relato recusa com 404, sem gravar nada, um `job_do_erro` que não exista em `requests` dele.
- **C-1.4a** (1.1.2) Se o `job_do_erro` ou qualquer job local da lista ainda está `na_fila` ou `em_execucao` em `requests`, o relato é recusado com **409** e `Retry-After`, sem gravar nada (a amostra não pode ser congelada incompleta).
- **C-1.5** O relato grava a ocorrência, os jobs e as amostras de **todos** os jobs numa única transação, copiando-as do `content.db`; as exceções são as ocorrências `distribuida` (C-1.10) e `fallback` (C-1.21), que não copiam amostras.
- **C-1.6** `amostras` tem uma linha por `job_id` (PK); um job pode pertencer a mais de uma ocorrência, e a amostra é então compartilhada.
- **C-1.7** A amostra guarda: `job_id`, `copiada_em`, `endpoint`, `modelo`, `think`, `prompt_json`, `prompt_texto`, `resposta`, `prompt_hmac`, `conteudo_hmac`, `key_id`, `prompt_truncado`, `prompt_json_truncado`, `tem_imagem`, `doc_hash`, `resp_final`, `resp_truncada`, `status`, `http_status`.
- **C-1.8** O `estado` da ocorrência é um de: `nova`, `confirmada`, `descartada`, `sem_pedido`, `distribuida`, `fallback`, `apagada`; a `particao` é um de: `treino`, `teste`, `quarentena`, ou **vazia** (pendente) enquanto o grupo não tiver amostra exportável (C-4.6).
- **C-1.9** Toda ocorrência nasce `nova`, exceto: `sem_pedido` quando o conteúdo do **`job_do_erro`** já expirou do `content.db` ou não tem `prompt_json` (job anterior à 0.13.0); e `distribuida` quando os jobs da lista têm backends diferentes. `fallback` quando o gateway que recebe é de fallback (C-1.21). Precedência, se mais de um valer: `distribuida`, depois `fallback`, depois `sem_pedido`.
- **C-1.9a** Se só jobs que **não** são o `job_do_erro` expiraram, a ocorrência nasce `nova`: as amostras existentes são copiadas e as dos expirados ficam só como referência em `ocorrencia_jobs`.
- **C-1.10** Numa ocorrência `distribuida` o gateway guarda só a ocorrência e as referências dos jobs (com o backend de cada um), **nenhuma amostra**, nem as locais; a resposta traz `estado=distribuida`, `amostras_copiadas=0` e `jobs_externos` = número de jobs de outro backend.
- **C-1.11** Os jobs de outro backend, numa ocorrência distribuída, são aceitos só como referência, sem validação local; o relato vai ao gateway do `job_do_erro`.
- **C-1.11a** (1.1.2) **Mesma origem:** a amostra de um job local só é copiada se a origem do relato bate com a do job em `requests`: a estação declarada (máquina **e** usuário) igual **e** o mesmo IP de origem verificado pelo gateway (o mesmo cálculo que ele grava no job); se o job não tem estação, só o IP (1.1.3: a estação sozinha é declarada e pode ser falsificada). Um job de outra origem fica só como referência em `ocorrencia_jobs` e é contado em `jobs_outra_origem` na resposta. Se for o `job_do_erro`, a ocorrência nasce `sem_pedido`. A ação "Marcar erro" do admin no monitor (12.6) não tem essa restrição.
- **C-1.12** Transições permitidas, e só elas: `nova`→`confirmada` (confirmar), `nova`, `confirmada`, `sem_pedido`, `distribuida` ou `fallback`→`descartada` (descartar: o admin julgou que não foi erro do modelo, e o caso sai também da estatística de erros), `confirmada`→`nova` (alterar a correção, C-6.5) e qualquer estado→`apagada` (apagar). `descartada` não volta a `nova`; `sem_pedido`, `distribuida` e `fallback` nunca são confirmadas.
- **C-1.12a** Confirmar exige `resposta_correta` não vazia; sem ela, a ação é recusada.
- **C-1.12b** Descartar uma ocorrência `confirmada` (1.1.0) apaga o embedding, zera `exemplo_publico`, invalida (`valido=0`) todo retrato que a lista e **não** libera a vaga dela na cota (C-4.11a). O histórico grava `descartou`. Uma exportação já baixada não é desfeita pelo servidor; as linhas `exportou` anteriores ficam no histórico como registro.
- **C-1.12c** (1.1.2) Só ocorrências `nova` ou `confirmada` podem ter campos editados (`resposta_correta`, `descricao`, `correcao_aplicada`). Nos estados de estatística (`sem_pedido`, `distribuida`, `fallback`) e em `descartada`, a edição é recusada.
- **C-1.12d** (1.1.2) A substituição do candidato automático pelo relato humano (C-1.19) recalcula o estado pela precedência da C-1.9 sobre a lista nova: se der `distribuida` ou `fallback`, o estado muda, as amostras copiadas pela substituída que ninguém mais usa saem (C-6.4) e nada novo é copiado. A lista de jobs é **trocada** pela nova (não somada), e cada campo alterado, inclusive os da estação e da origem, grava `alterou`.
- **C-1.13** Confirmar, descartar, completar a correção, marcar exemplo público, apagar e liberar quarentena são ações exclusivas do admin, com sessão, CSRF e same-site; cada uma grava no histórico.
- **C-1.14** `sem_pedido`, `distribuida`, `fallback` e `nova` contam só na estatística de erros: nunca entram na exportação, nos exemplos nem na avaliação. `descartada` e `apagada` não contam na estatística de erros; `descartada` conta só na taxa de descarte por regra automática (D12-8).
- **C-1.15** Uma ocorrência em partição `quarentena` fica fora da exportação de treino e de teste, do índice de exemplos e da avaliação, e conta só na estatística.
- **C-1.16** `categoria` ∈ {`numeracao_errada`, `contagem_errada`, `calculo_errado`, `inventou`, `omitiu`, `formato_invalido`, `resumo_infiel`, `traducao_errada`, `idioma_errado`, `truncado`, `ignorou_instrucao`, `outro`}; `origem` ∈ {`claude`, `usuario`, `automatico`}; `severidade` ∈ {`baixa`, `media`, `alta`}; valor fora do enum é recusado.
- **C-1.17** Limites do relato: corpo ≤ 256 KB, `descricao` (obrigatória) ≤ 4 KB, `resposta_correta` ≤ 128 KB, `correcao_aplicada` ≤ 4 KB, `projeto` ≤ 128 bytes e no máximo **64 jobs** na lista; campo JSON desconhecido também é recusado (1.1.2); acima disso o relato é recusado com mensagem clara. O `--max-chunks` da estação não é limitado: um pedido com mais de 64 chamadas simplesmente não pode ser relatado (1.1.1, documentado no `RELATAR-ERROS-IA.md`).
- **C-1.18** Os dados de estação, projeto e origem do relato são declarados e não autenticados; nada entra no treino, nos exemplos ou na avaliação sem confirmação do admin.
- **C-1.19** Candidato automático nasce `nova`, com `origem=automatico` e `resposta_correta` vazia; um relato humano da mesma ocorrência o substitui, e o histórico grava `alterou` com `campo=origem`.
- **C-1.20** Os candidatos automáticos têm só as regras: JSON inválido (pedido com `format` = `"json"` ou schema, e texto reprovado em `json.Valid`), resposta só de espaços, e `fim=length` com `num_predict` ≥ 256 (ou ausente).
- **C-1.20a** (1.1.4) A regra do JSON inválido fica **reservada**: só vale quando alguma tool expuser o `format` do Ollama (hoje nenhuma expõe; o `format` texto livre do `extract`/`summarize_file` não conta). Os candidatos automáticos vêm **desligados por padrão** (`--auto-report=false`), porque copiam para o `treino.db`, sem expiração, o pedido e a resposta, inclusive de documentos sensíveis; quem liga é a estação, de propósito.
- **C-1.21** Nesta fase, só o gateway **primário** guarda ocorrências elegíveis, porque cada gateway tem o seu `treino.db` e a partição não é coordenada entre eles. O papel do gateway vem da flag `--role` (`primario`, o padrão, ou `fallback`). Num gateway `fallback`, toda ocorrência com jobs num só backend nasce no estado próprio **`fallback`**: guarda só a ocorrência e as referências dos jobs, **nenhuma amostra**, e conta só na estatística. A resposta traz `estado=fallback` e `amostras_copiadas=0`. O estado `distribuida` continua reservado para jobs em backends diferentes (C-1.9).

## 2. Pedido canônico

- **C-2.1** O gateway grava `prompt_json` no `content.db` na **entrada** da fila (`SavePrompt`), a partir do corpo **depois** da política (`applyPolicy`), nunca do texto rotulado de `extractPrompt`.
- **C-2.2** O `prompt_json` contém `system`, `prompt` ou `messages` completos (todo o histórico do `chat`), `options`, `format`, `think` e `model`, como o Ollama os recebe.
- **C-2.3** O texto achatado e o `prompt_json` têm tetos de 16 MiB **separados**, cada um com a sua marca (`prompt_truncated`, `prompt_json_truncated`); truncar um não marca o outro.
- **C-2.4** A resposta tem teto de 16 MiB, marca `resp_truncated`, e `resp_final=1` só quando o streaming terminou.
- **C-2.5** Imagem no pedido é substituída, no `prompt_json`, pelo marcador `{"imagem_hmac":"<hex>"}` (HMAC-SHA256 da imagem com a chave da C-4.3), e o job recebe `tem_imagem=1`.
- **C-2.6** Um job com `tem_imagem=1` é não exportável e fica fora da avaliação; conta só na estatística.
- **C-2.7** As colunas novas do `content.db` (`prompt_json`, `prompt_json_truncated`, `tem_imagem`) entram por migração idempotente: um banco antigo abre sem perder dados.
- **C-2.8** O teto total do `content.db` é 4 GiB, com retenção de 7 dias; acima do teto apaga os mais antigos.
- **C-2.9** Ao relatar, as marcas `prompt_truncated`, `prompt_json_truncated`, `tem_imagem`, `resp_final`, `resp_truncated`, `status` e `http_status` são copiadas para a amostra, e a exportação lê só a amostra, nunca o `content.db`.
- **C-2.10** O `prompt_json` gravado para um job é igual, byte a byte, ao corpo que chegou ao Ollama, exceto pelas imagens trocadas pelo marcador (C-2.5) (teste com Ollama falso, inclusive `chat` com histórico).
- **C-2.11** O cabeçalho `X-Mcpollama-Doc` é removido antes de o pedido chegar ao Ollama e não aparece no `prompt_json`.
- **C-2.12** `prompt_hmac` (na amostra e no retrato) = HMAC-SHA256(chave, sha256 do `prompt_json` gravado). `conteudo_hmac` = HMAC-SHA256(chave, sha256 do JSON canônico de `system`, `prompt` e `messages`, sem `model`, `options`, `format` e `think`; chaves ordenadas, sem espaços). Nenhum dos dois é guardado como sha256 puro.

## 3. Regra única de exportação

- **C-3.1** Uma amostra entra no JSONL se e somente se **todas** as condições C-3.2 a C-3.10 forem verdadeiras; não existe outro caminho de exportação.
- **C-3.2** (item 1) A ocorrência está `confirmada` pelo admin.
- **C-3.3** (item 2) A amostra é a do `job_do_erro` da ocorrência.
- **C-3.4** (item 3) `resposta_correta` não está vazia.
- **C-3.5** (item 4) O job é de geração (`/api/generate`, `/api/chat`, `/v1/chat/completions` ou `/v1/completions`; nunca embeddings), com `status=ok` e `http_status=200`.
- **C-3.6** (item 5) Na amostra, `prompt_truncado=0`, `prompt_json_truncado=0`, `resp_final=1` e `resp_truncada=0`.
- **C-3.7** (item 6) A tool é `generate`, `chat`, `summarize_file`, `extract` ou `translate`.
- **C-3.8** (item 7) `tem_imagem=0`.
- **C-3.9** (item 8) A ocorrência não é `distribuida` nem `fallback`, e a partição não é `quarentena`.
- **C-3.10** (item 9) No `translate`, a ocorrência só entra se o `job_do_erro` for uma parte (`mapN`) ou a chamada única (`unico`, arquivo de uma parte, sem texto montado; 1.1.1) e a `resposta_correta` for a tradução certa **daquela chamada**; erro no texto montado do `translate` não entra.
- **C-3.11** A exportação aceita `parte=treino` ou `parte=teste`, exporta só amostras da partição pedida, e nenhuma amostra de `quarentena`.
- **C-3.12** A exportação é restrita ao admin (sessão + same-site), gerada na hora em streaming, não fica guardada no servidor, e grava `exportou` no histórico.
- **C-3.13** Cada linha do JSONL é `{"messages":[system, user, assistant],"meta":{...}}`; o `assistant` é a `resposta_correta`, e `system`/`user` vêm do `prompt_json`.
- **C-3.14** Para `chat`, as `messages` são as completas do `prompt_json`, com todo o histórico, e a última mensagem do assistente é a `resposta_correta`.
- **C-3.15** O `meta` traz `ocorrencia_id`, `job_id`, `papel`, `tool`, `categoria` e `particao`, e nenhum texto livre além desses.
- **C-3.16** Uma amostra que viola qualquer condição nunca aparece na saída (um teste por condição, com um caso que falha só nela).

## 4. Partição treino/teste

- **C-4.1** Nas tools de arquivo, o mcpollama calcula uma vez o sha256 do texto extraído do documento inteiro e envia, em **cada** chamada do pedido (map, reduce e partes), o cabeçalho `X-Mcpollama-Doc` com esse valor.
- **C-4.2** O gateway grava em `requests.doc_hash` somente `HMAC-SHA256(chave, valor do X-Mcpollama-Doc)`; o sha256 puro nunca é gravado (nem em `requests`, amostras, ocorrências, histórico ou logs).
- **C-4.3** A chave do HMAC fica num arquivo 0600 gerado pelo instalador (`--doc-key-file`), usa só a biblioteca padrão e fica fora do backup comum.
- **C-4.3a** Todos os **gateways** que calculam e guardam HMACs (primário e fallback) usam a **mesma chave**, copiada por canal protegido. Os servidores Ollama do pool, que ficam atrás de um gateway, não recebem a chave, porque não calculam nem guardam HMAC (1.1.0). Cada hash gravado leva `key_id` (os 16 primeiros hex do sha256 da chave), e o agrupamento só compara hashes com o mesmo `key_id`. Sem `--doc-key-file`, o gateway não aceita relatos (o `/api/feedback` responde 503 com mensagem explícita).
- **C-4.4** Trocar a chave só é feito com o owner, e quebra os grupos antigos.
- **C-4.5** Um **grupo** é um componente conectado de ocorrências ligadas pelo mesmo `doc_hash`, pelo mesmo `conteudo_hmac` (C-2.12) ou por um job em comum; o grupo inteiro cai do mesmo lado. Entram nos componentes **todas as ocorrências não apagadas**, inclusive `nova` e `descartada`, para não haver ligação oculta.
- **C-4.6** (1.1.0) A cota conta só grupos **com ao menos uma amostra exportável**, isto é, que passe em **todo** o predicado do cap. 3 (C-3.1). Os 30 primeiros desses grupos de cada categoria vão para o **teste**. "Primeiros" segue o momento em que o grupo passa a ter a primeira amostra exportável, e a categoria do grupo é a da ocorrência dessa primeira amostra. A partição é atribuída nesse momento; antes dele, o grupo fica com a partição **pendente** (vazia, C-1.8).
- **C-4.7** Depois da cota, um grupo (no momento da C-4.6) vai para o teste se `sha256(id do grupo) mod 100 < 15`, senão vai para o treino.
- **C-4.8** A partição, depois de atribuída, é estável: o mesmo grupo mantém o lado em reprocessamentos e novos relatos. Quando grupos do **mesmo** lado se fundem, o resultado usa o menor `grupo_id` anterior, mantém o lado e não é reavaliado pela cota nem pelos 15%.
- **C-4.9** (1.1.0) Quando uma ocorrência nova liga grupos: (a) se entre eles há grupos já atribuídos a **lados opostos** (treino e teste), o componente inteiro (todos os grupos unidos mais a nova) vai para `quarentena`; (b) se só um lado aparece entre os grupos atribuídos, o componente inteiro herda esse lado; (c) se nenhum tem lado, o componente fica pendente até a C-4.6.
- **C-4.10** Ao entrar em quarentena, os retratos que tinham casos desse componente passam a `valido=0`, e o índice de exemplos congelado que usava algum deles também é invalidado.
- **C-4.11** O admin libera uma quarentena escolhendo um lado para o componente inteiro, pela rota `POST /monitor/api/treino/{id}/liberar-quarentena`; o histórico grava `alterou` com `campo=particao`.
- **C-4.11a** Desconfirmar, descartar (C-1.12b) ou apagar uma ocorrência de teste **não libera** a vaga dela na cota, e invalida (`valido=0`) todo retrato que a lista.
- **C-4.11b** (1.1.2) A cota é guardada numa tabela própria, `cota_grupos` (`grupo_id`, `categoria`, `particao`, `atribuida_em`), sem nenhum conteúdo. Desconfirmar, descartar ou apagar ocorrências não mexe nela, e é ela que conta as vagas da C-4.6.
- **C-4.12** Nenhum `job_id`, `doc_hash` ou `grupo_id` aparece ao mesmo tempo em amostras exportáveis de treino e de teste.
- **C-4.13** Há um retrato por avaliação. Ele é uma lista congelada de (`ocorrencia_id`, `job_id`, `prompt_hmac`, `categoria`) mais o hash do índice de exemplos congelado, o ID/hash da versão das instruções e `valido`; depois de criado não muda, exceto `valido`→0.
- **C-4.14** Linha de base e candidato são sempre comparados no mesmo retrato; um retrato novo exige uma linha de base nova.
- **C-4.15** Tools sem documento (`generate`, `chat`) têm `doc_hash` vazio e agrupam-se por `conteudo_hmac` igual ou por job em comum (C-4.5).

## 5. Protocolo de avaliação

- **C-5.1** A entrada da avaliação são as amostras da partição `teste` que passam no predicado do cap. 3, reenviadas ao modelo candidato, pelo gateway, com o `prompt_json` **igual**, exceto `options.seed` (C-5.15), e o cabeçalho `X-Mcpollama-Tool: bakeoff-erros`.
- **C-5.2** `numeracao_errada` e `omitiu` são pontuadas pela lista de itens comparada com a resposta certa: acerto só com precisão **e** cobertura de 100%.
- **C-5.3** `formato_invalido` passa só se o texto passar em `json.Valid` **e** os valores forem iguais aos da resposta certa, depois de normalizar espaços e ordem de chaves; comparar só as chaves não basta.
- **C-5.4** `contagem_errada` e `calculo_errado` passam se o número final for igual; `truncado` passa se `fim=stop`, `resp_final=1` e `resp_truncated=0`.
- **C-5.5** `resumo_infiel`, `inventou`, `traducao_errada` e `ignorou_instrucao` são avaliadas por revisão humana às cegas (modo `blind`, saída atual e candidata embaralhadas).
- **C-5.6** Casos com conteúdo de cliente são avaliados só pelo owner; o Claude avalia apenas casos anonimizados e nunca recebe documento de cliente.
- **C-5.7** `outro` e `idioma_errado` ficam fora da avaliação e contam só na estatística.
- **C-5.8** Uma categoria só é **reportada** com ≥ 20 casos de teste e só **decide** com ≥ 30. Os limiares das C-5.10 e C-5.11 valem só para categorias com ≥ 30; as demais só são reportadas.
- **C-5.9** O relatório traz o ID do retrato, N, o acerto por categoria e o intervalo de Wilson de 95%.
- **C-5.10** Uma mudança (instrução nova, exemplos da D12-15 ou modelo v2) é aceita como **melhora** se a categoria-alvo ganhar ≥ 10 pontos percentuais com o limite inferior do intervalo acima do **ponto** (acerto) do modelo atual medido no mesmo retrato e na mesma sessão (C-5.15).
- **C-5.11** Nenhuma outra categoria pode cair mais de 3 pontos percentuais.
- **C-5.12** O S2 e o S3 da Fase 8 não podem cair mais de 2 pontos percentuais.
- **C-5.13** O gate **falha** se a suíte não rodar, se avaliar 0 casos, se nenhuma categoria atingir o mínimo de **30** ou se o retrato estiver com `valido=0`; o relatório diz então "dados insuficientes" e nunca "sem regressão".
- **C-5.14** A linha de base é a primeira rodada, com o `qwen3-ia` atual e sem a D12-15, registrada no relatório da etapa, e é a referência de tudo que vier depois.
- **C-5.15** **Controle de variação:** cada caso automático roda em **3 rodadas pareadas**, com `options.seed` = 1, 2 e 3 e as demais opções iguais ao `prompt_json`. O modelo atual e o candidato rodam com as mesmas sementes, intercalados na mesma sessão. O caso conta como acerto com **2 de 3** rodadas certas, e o relatório traz quantos casos mudaram de resultado entre rodadas. A avaliação humana às cegas usa 1 rodada (`seed`=1).
- **C-5.16** A cada comparação, o modelo atual é **medido de novo** no mesmo retrato e na mesma sessão (C-5.15). A linha de base da C-5.14 fica como registro histórico; se ela e a nova medição do modelo atual diferirem em mais de 3 pontos numa categoria, o relatório avisa (mudança no servidor).

## 6. Retenção e descarte

- **C-6.1** O `treino.db` é um arquivo 0600, sem expiração automática.
- **C-6.2** O teto do `treino.db` é 2 GiB de **dados vivos**, medido **depois** de o relato obter a vez de escrever e antes da transação (sem janela entre medir e gravar; 1.1.3), (páginas usadas, sem as livres e sem contar o `-wal` em dobro; 1.1.2); acima dele o gateway recusa novos relatos com **HTTP 507** e corpo de erro explícito, grava um evento e mostra aviso no monitor, e **não apaga nada sozinho**.
- **C-6.2a** (1.1.2) O `treino.db` usa `auto_vacuum` incremental e `journal_size_limit`: depois de apagar, o espaço volta ao orçamento, e o relato que antes recebia 507 volta a ser aceito.
- **C-6.3** Apagar é ação do admin, com histórico. Remove todo o conteúdo da ocorrência, os jobs dela e as amostras que ninguém mais usa, e deixa só uma linha mínima (`id`, `estado=apagada`, datas), sem nenhum campo de conteúdo.
- **C-6.4** Uma amostra (e o embedding) só é removida quando nenhuma ocorrência a referencia mais (contagem de referências).
- **C-6.5** Alterar `resposta_correta`, `descricao` ou `correcao_aplicada` de uma ocorrência `confirmada` desfaz a confirmação (volta a `nova`), apaga o embedding anterior e grava `desconfirmou` no histórico.
- **C-6.6** O embedding (`nomic-embed-text`, 768 floats) é calculado só quando o admin marca `exemplo_publico=1` numa ocorrência confirmada, e é apagado quando a marca sai ou a confirmação é desfeita.
- **C-6.7** A exportação é gerada na hora e não é guardada no servidor; o arquivo baixado é responsabilidade de quem baixou, e isso fica no histórico.
- **C-6.8** O `treino.db` e o `content.db` ficam fora do backup comum, ou só entram cifrados.
- **C-6.9** Treinar fora da rede exige dados anonimizados.
- **C-6.10** Consultas de banco atendidas a um visitante do monitor (aba Erros, exportação) usam contexto próprio (`context.Background()`) e cache em segundo plano, nunca o contexto do visitante sob mutex compartilhado.

## 7. Histórico

- **C-7.1** `historico` tem: `id`, `quando`, `ocorrencia_id`, `acao`, `campo`, `antes`, `depois`, `quem`, `ip`.
- **C-7.2** `acao` ∈ {`criou`, `alterou`, `confirmou`, `desconfirmou`, `descartou`, `apagou`, `exportou`}.
- **C-7.3** Criação, cada alteração de campo, revisão, apagamento e exportação gravam uma linha de histórico, na mesma transação da mudança.
- **C-7.4** Nos campos de conteúdo (`resposta_correta`, `descricao`, `correcao_aplicada`, pedidos e respostas), `antes` e `depois` guardam **só** `hmac=<hex> key_id=<id> bytes=<n>`: HMAC-SHA256 do texto com a chave da C-4.3, nunca o texto nem o sha256 puro (1.1.2).
- **C-7.5** `quem` é a estação declarada (relato) ou `admin` (ação no monitor); `ip` é o IP de origem; `quando` é o horário do gateway.
- **C-7.6** Nenhuma linha de `historico` contém texto de documento, pedido, resposta ou correção (teste que varre a tabela depois de um ciclo completo).
- **C-7.7** A exportação grava uma linha `exportou` **por ocorrência exportada**, com a parte (`treino` ou `teste`) em `campo` e quem baixou.
- **C-7.8** Apagar grava `apagou` com hash e tamanho dos campos de conteúdo.
- **C-7.9** O `historico` não expira e é mantido depois de `apagou` (sem texto sensível, C-7.4); ele conta no teto da C-6.2.

## 8. Privacidade dos exemplos e melhora sem treino

- **C-8.1** Só entram como exemplo no pedido as ocorrências `confirmadas` que o admin marcou como `exemplo_publico=1` (sintéticas ou anonimizadas).
- **C-8.2** Nenhum documento de cliente é injetado no pedido de outro; o filtro "mesmo projeto" fica para uma fase futura.
- **C-8.3** A busca por embedding usa **só** ocorrências da partição `treino`.
- **C-8.4** A busca exclui a ocorrência, o grupo e o documento em avaliação, e todo caso em `quarentena`.
- **C-8.5** O índice de exemplos é congelado, junto com o retrato, antes de cada avaliação, e o hash dele é registrado no retrato.
- **C-8.6** A análise que reescreve as instruções por tool usa **só** a partição de treino, nunca a de teste.
- **C-8.7** Cada versão das instruções tem ID e hash e é congelada antes da avaliação; o retrato registra a versão usada.
- **C-8.8** Os exemplos (até k pares "erro → certo") e as instruções novas ficam ligáveis por tool e por flag, com padrão **desligado**, e só ficam ligados quando a avaliação cumprir C-5.10 a C-5.12.
- **C-8.9** Uma ocorrência desconfirmada, descartada, apagada, `sem_pedido`, `distribuida` ou `fallback` não é usada como exemplo.
- **C-8.10** O valor de k e o limiar de similaridade são fixados na etapa 12.8, medidos pela 12.7, e registrados no retrato junto com o índice.

## Rastreabilidade (regra → decisão do plano)

| Regras | Decisão / seção do plano |
| --- | --- |
| C-0.1 a C-0.6 | §8.1 |
| C-1.1 a C-1.3, C-1.16, C-1.17 | D12-2, D12-3, D12-4, §8.2, `docs/RELATAR-ERROS-IA.md` |
| C-1.4, C-1.5, C-1.9 a C-1.11 | D12-4, D12-5, §8.3 |
| C-1.6, C-1.7 | D12-4, §8.2 |
| C-1.8, C-1.12 a C-1.15, C-1.18 | D12-4, D12-7, D12-7a, D12-16, §8.2 |
| C-1.19, C-1.20 | D12-8 |
| C-2.1 a C-2.8, C-2.10 | D12-13, §5.1, §8.2 |
| C-2.9, C-2.11 | D12-7 (item 5), §8.2, D12-7a(a) |
| C-3.1 a C-3.10, C-3.16 | D12-7 (itens 1 a 9) |
| C-3.11 a C-3.15 | D12-7, D12-10, §8.3, §8.6 |
| C-4.1 a C-4.4, C-4.15 | D12-7a (a), §8.2, §8.4, risco 11 |
| C-4.5 a C-4.14 | D12-7a (b) a (e), §14 itens 7 a 9 |
| C-5.1 a C-5.14 | D12-17, §8.5, §14 itens 4 e 7, §15.2 (B5) |
| C-6.1 a C-6.9 | D12-10, §8.2, §13 riscos 3 e 11 |
| C-6.10 | §2.4, §8.2, risco 8 |
| C-7.1 a C-7.9 | D12-16, D12-10, §8.2 |
| C-8.1 a C-8.10 | D12-15, §13 riscos 4 e 5, §14 item 3, §15.2 (B1, B2), §15.3 (C4) |
| C-1.9a, C-1.12a, C-1.21, C-2.12, C-4.3a, C-4.11a, C-5.15, C-5.16, C-7.9 e os acréscimos marcados nas demais | revisão do contrato pelo owner (2026-11-02 19:5x): as 21 lacunas + chave compartilhada, pedidos sem documento e variação do bakeoff |

## Lacunas fechadas (owner, 2026-11-02)

As 21 lacunas do rascunho foram fechadas pela sugestão conservadora de cada uma (regras C-1.3, C-1.9, C-1.9a, C-1.12, C-1.12a,
C-1.17, C-1.19, C-2.5, C-2.12, C-4.5, C-4.6, C-4.8, C-4.11, C-4.11a, C-4.13, C-5.2, C-5.4, C-5.8, C-5.10, C-5.13, C-6.2, C-6.3, C-6.6,
C-7.7, C-7.9 e C-8.10). Na mesma revisão, o owner acrescentou três pontos: a chave HMAC compartilhada entre gateways (C-4.3a e C-1.21),
o agrupamento de pedidos sem documento pelo conteúdo canônico (C-2.12, C-4.5 e C-4.15) e o controle de variação do bakeoff (C-5.15 e
C-5.16). Por coerência com a C-4.2, os hashes do pedido e da imagem também passaram a HMAC (`prompt_hmac`, `conteudo_hmac`, `imagem_hmac`),
no lugar do `prompt_sha256` do plano (§8.2).

Ajuste de 2026-11-02 20:00 (revisão externa): o gateway de fallback ganhou o estado próprio `fallback` (C-1.21). Antes, a C-1.21 reaproveitava `distribuida`, que pela C-1.9 significa jobs em backends diferentes.

Ajuste de 2026-11-02 20:02 (revisão externa): a C-1.12 passou a permitir descartar também `sem_pedido`, `distribuida` e `fallback`, para que relatos falsos ou equivocados nesses estados não fiquem na estatística.

## Versão 1.1.0 (owner, 2026-11-02 20:33)

Seis correções de lacunas achadas na sincronização do plano, aprovadas pelo owner com as precisões da revisão externa:
1. a cota conta só grupos com amostra exportável; a categoria e a ordem vêm da primeira amostra exportável; antes disso, a partição fica pendente (C-4.6, C-4.7, C-1.8);
2. `key_id` na amostra (C-1.7);
3. quarentena só quando grupos já atribuídos a lados opostos se ligam; um lado só é herdado; nenhum lado fica pendente (C-4.9);
4. a chave vai só aos gateways que calculam e guardam HMACs, não aos Ollamas do pool (C-4.3a);
5. `confirmada`→`descartada` direto, com efeitos explícitos (C-1.12, C-1.12b, C-4.11a);
6. rastreabilidade C-7.1 a C-7.9.

## Versão 1.1.1 (owner, 2026-11-02 20:40)

Achados M-1 e M-2 da revisão do código da 12.2: a C-3.10 aceita `unico` no `translate` (arquivo de uma parte não tem texto montado), e a C-1.17 documenta que pedido com mais de 64 chamadas não pode ser relatado, sem teto na flag `--max-chunks`.

## Versão 1.1.2 (owner, 2026-11-02 23:40)

Achados da revisão independente da 12.4 e decisões do owner:
- **C-7.4:** o histórico usa HMAC com a chave, mais o tamanho; nunca o sha256 puro.
- **C-4.11b:** a cota fica numa tabela própria (`cota_grupos`), que apagar não altera.
- **C-1.11a:** a amostra só é copiada de job da mesma origem do relato (estação, ou IP quando o job não tem estação); os outros jobs ficam como referência.
- **C-1.12c:** só `nova` e `confirmada` podem ser editadas.

Correções sem decisão nova:
- **C-1.4a:** job ainda na fila ou em execução recebe 409.
- **C-1.12d:** a substituição do automático respeita a precedência e troca a lista de jobs.
- **C-6.2 e C-6.2a:** o teto conta só os dados vivos, com `auto_vacuum`.
- **C-1.17:** os limites que o gateway já aplicava foram documentados.

## Versão 1.1.3 (owner, 2026-11-03 09:59)

Achados da revisão automática de segurança do commit da 12.4. A **C-1.11a** passa a exigir que a estação declarada **e** o IP verificado batam com os do job, porque a estação sozinha pode ser falsificada; quando o job não tem estação, vale só o IP (decisão do owner). A **C-6.2** passa a exigir que o teto seja medido depois de o relato obter a vez de escrever, o que fecha a janela de corrida entre medir e gravar.

## Versão 1.1.4 (owner, 2026-11-03 10:21)

Revisão da 12.5: a regra automática do JSON inválido fica reservada até alguma tool expor o `format`, e os automáticos passam a vir desligados por padrão (C-1.20a). O plano dizia "ligados" (D12-8, §14 item 1); prevalece esta versão.
