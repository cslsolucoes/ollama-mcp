# MCPVMWare — configuração e segurança

| Campo | Valor |
| --- | --- |
| **FileVersion** | 1.0.0 |
| **Data** | 2026-10-23 |
| **Origem** | onda B2 do plano `.workspace/plans/MCPOllam2026-10-23-153853-brief-referencias.plan.md` |

Citações relativas a `referencias/`. Valores de credencial **não** aparecem; campos e variáveis são citados pelo nome. Os testes empíricos usaram valores fictícios inventados nesta onda, numa cópia do código fora de `referencias/`.

## 1. Flags e variáveis de ambiente (tabela completa)

Declaração: `MCPVMWare/src/mcpvmware-mcp/main.go:44-61`; fallback para env: `:133-165`.

| Flag | Tipo / default | Env de fallback | Papel |
| --- | --- | --- | --- |
| `--vcenter-url` | string, `""` | `VCENTER_URL` (`:133-135`) | modo vCenter |
| `--vmware-url` | string, `""` | **nenhuma** | modo ESXi standalone |
| `--workstation-url` | string, `""` | **nenhuma** | modo Workstation Pro (`vmrest`) |
| `--vmware-all-url` | string, `""` | **nenhuma** | vCenter/ESXi, tools de vSphere |
| `--all-url` | string, `""` | **nenhuma** | modo "100%" (todas as tools) |
| `--cloud-aws-url` | string, `""` | **nenhuma** | modo VMC |
| `--username` | string, `""` | `VCENTER_USERNAME` (`:137-139`) | usuário (vCenter/ESXi ou Basic Auth do `vmrest`) |
| `--password` | string, `""` | `VCENTER_PASSWORD` (`:140-142`) | senha (idem) |
| `--refresh-token` | string, `""` | `VMC_REFRESH_TOKEN` (`:163-165`) | token CSP do VMC |
| `--insecure` | bool, `false` | `VCENTER_INSECURE` (`:143-149`): qualquer valor exceto `""`, `0`, `false`, `no`, `off` liga | desliga a verificação de certificado |
| `--version` | bool, `false` | — | imprime versão e sai |
| `--debug` | bool, `false` | `MCPVMWARE_DEBUG`: só o valor `"1"` liga (`:150-152`) | log verboso em stderr |
| `--allow-destructive` | bool, `false` | `VCENTER_ALLOW_DESTRUCTIVE` (mesma regra do `--insecure`, `:153-159`) | abre o gate destrutivo |
| `--tools-verbosity` | string, `"full"` | **nenhuma** | `full` ou `compact`; valor inválido → `log.Fatalf` (`:271-277`) |
| `--profile` | string, `""` | **nenhuma** | lista separada por vírgula de famílias |
| `--audit-log-path` | string, `""` | `MCPVMWARE_AUDIT_LOG` (`:160-162`) | arquivo JSONL de auditoria |

Observações:

- **Assimetria flags→env:** só `--vcenter-url` e os campos de credencial têm env; os 4 outros modos de conexão, `--tools-verbosity` e `--profile` não têm. A mensagem de erro de `main.go:174` fala em "equivalent env vars" mas só 3 existem para a conexão. Para o mcpollama (que declara `OLLAMA_*` para tudo): simetria total, incluindo `--tools-verbosity` e `--debug`, com a **mesma** regra de booleano em todas (aqui `MCPVMWARE_DEBUG` só aceita `1`, o resto aceita vários valores).
- **Exatamente uma flag de conexão** (`:70-123`), com exceção de `--all-url` + secundárias (`:75-86`, `:215-256`).
- **Segredo em argv:** `--password` e `--refresh-token` são argumentos de linha de comando, visíveis na lista de processos do SO. A env evita isso. O mcpollama deve preferir a env `OLLAMA_API_KEY` e documentar o risco de `--api-key` na linha de comando (**(inferência)** sobre o SO; o código não trata). `--api-key-file` foi descartado pelo owner (P-1 = não, 2026-10-23 15:50).
- Validações no boot: `--cloud-aws-url` exige `--refresh-token` (`:169-172`); os outros modos exigem URL, usuário e senha (`:173-175`).

## 2. TLS, CA e `insecure`

- **Só existe `--insecure`.** Não há `--tls-ca`, nem *pinning*. O vSphere usa `govmomi.NewClient(ctx, u, cfg.Insecure)` (`MCPVMWare/src/vmware/client.go:84`).
- Aviso em stderr quando ligado: `WARNING: TLS certificate verification disabled ...` (`main.go:176-178`).
- `workstation.Client`: só troca o transporte para `InsecureSkipVerify: true` quando a URL é `https` **e** `Insecure` (`workstation/client.go:73-76`); caso contrário `http.DefaultTransport` (raízes do SO). `//nolint:gosec` documentando que é opt-in explícito.
- `cloudaws.Client`: idem, mas o comentário da `Config` diz que `Insecure` serve a testes contra `httptest`; contra os hosts reais "nunca é necessário" (`cloudaws/client.go:55`). Hosts do CSP/VMC são constantes (`:31-34`); só testes sobrescrevem (`Config.AuthURL/BaseURL`).
- **Consequência para o mcpollama:** a CA Cloudflare Origin não é confiável no SO (CLAUDE.md do projeto). Precisa de `--tls-ca` (que MCPVMWare **não** tem; o molde para isso é o `MCPNas`, outra onda). O padrão de transporte customizado aqui (`&http.Transport{TLSClientConfig: ...}`) é o ponto de extensão; mas atenção: criar `&http.Transport{}` do zero **perde os defaults** de `http.DefaultTransport` (proxy do ambiente, `ForceAttemptHTTP2`, timeouts de dial e de handshake) — **(inferência)** pela documentação do `net/http`; o código dos clientes usa exatamente esse padrão e não tem esses defaults.
- **Timeouts de cliente:** `http.Client{Timeout: 30s}` no `workstation` (`workstation/client.go:82`) e no `cloudaws` (`cloudaws/client.go:127`). O cliente SOAP do govmomi não recebe timeout explícito aqui; o `ctx` global é `Background` (`main.go:184`).

## 3. Autenticação por produto

| Produto | Mecanismo | Onde |
| --- | --- | --- |
| vCenter/ESXi | SOAP `Login` com usuário/senha **depois** de envolver o transporte com *keepalive* (10 min de ociosidade; sessão padrão = 30 min) | `vmware/client.go:36, 84-93` |
| REST/VAPI (VAMI, tags, library) | login **lazy** na 1ª chamada, sessão reaproveitada; erro nomeia a causa provável | `vmware/client.go:113-128` |
| SSO admin | idem, lazy | `vmware/client.go:139-156` |
| Workstation `vmrest` | Basic Auth em **toda** requisição (sem sessão) | `workstation/client.go:44-49, 108` |
| VMC | troca de `refresh_token` por `access_token` (CSP), cache com margem de 60 s, 1 retentativa em `401` | `cloudaws/client.go:133-202` |

Nota de desenho que interessa ao mcpollama: o login e o keepalive são **dentro do cliente**, a camada `tools` não sabe de credencial; e a URL passada ao cliente **não pode** levar *userinfo* (`vmware/client.go:51-64`, bug-001), senão o auto-login do govmomi duplica o login.

## 4. Gate destrutivo (`destructive.go`)

Três camadas, em ordem, no wrapper `wrapDestructive` (`MCPVMWare/src/tools/destructive.go:177-215`; cópias para Workstation `:72-110` e VMC `:133-171`):

1. **Gate do servidor:** `r.allowDestructive` (flag `--allow-destructive`/`VCENTER_ALLOW_DESTRUCTIVE`), fechado por padrão. Nega **antes de tocar no backend**, independentemente do que o chamador passa (`:198-202`).
2. **`confirm` estrito:** `args["confirm"].(bool)`; só `true` booleano passa. `"true"` (string) e `1` (int) não contam (`:180, 203-205`; teste `destructive_test.go:51-62`).
3. **Audit** de **toda** chamada, permitida ou negada (`:182-196, 207-213`).

Tiers (`:23-28`): `tier1` irreversível, `tier2` disruptivo reversível. Decididos com o owner por `AskUserQuestion` em 2026-08-10 (comentário `:15-22`). O tier vem da **classificação por nome do método** no `gen` (`gen/main.go:194-205, 523-542`), com `fail-safe` = tier2 para o que não casa; foi corrigido à mão em casos de falso positivo (`Unmount*` → tier2) e de falso negativo (`authentication.Manager.Issue`, de sem-tier para tier2, `cerebrum.md:384-392`). Convenção de custo: no VMC qualquer escrita em SDDC é tier1 por causa do custo financeiro (`destructive.go:117-125`, `cerebrum.md:454-463`).

Medido (em cópia, `--workstation-url` + `vmware_workstation_vm_delete`, tier 1):

| Cenário | Resultado |
| --- | --- |
| gate fechado, `confirm:true` | `isError`, texto "...destructive operation (tier 1) and this server was started without --allow-destructive/VCENTER_ALLOW_DESTRUCTIVE — restart it with that flag/env var..." |
| gate aberto, sem `confirm` | `isError`, "requires confirm:true (exact boolean) to run" |
| gate aberto + `confirm:true` | executa o handler (no teste: erro de rede porque a porta estava morta; o gate passou) |

Fraqueza de desenho: a descrição do `confirm` repete "this argument alone is not enough" em **cada** uma das 607 tools (`vm.go:26-29`), o que pesa no `tools/list`; um texto único curto seria suficiente.

## 5. Audit log

- **Estrutura** (`auditEntry`, `destructive.go:38-47`): `time` (UTC, RFC3339), `tool`, `tier`, `args`, `gate_open`, `confirmed`, `allowed`, `error`.
- **Escrita** (`writeAudit`, `:222-243`): JSON Lines, `O_APPEND|O_CREATE|O_WRONLY`, permissão `0600`, mutex; **falha de escrita nunca vira erro de tool** (decisão explícita: perder uma linha de auditoria não pode bloquear/desfazer uma ação já aprovada, `:217-221`). Desligado sem `--audit-log-path`: "o servidor nunca grava em disco sem ser pedido" (`registry.go:59-65`).
- **Problema medido: `args` entra em texto puro.** Chamei uma tool destrutiva com um campo extra `password` (valor fictício) e o arquivo de auditoria registrou `"args":{"confirm":true,"id":"abc","password":"[valor fictício]"}` nas três linhas (negada por gate, negada por falta de `confirm`, e executada). Não há nenhuma função de *redaction* no código (`grep` por `redact|sanitize|mask` só acha `netmask`/`subnet_mask`). E existem tools destrutivas reais com argumento de senha: `vmware_crypto_register_kmip_server` e `_update_kmip_server` (`generated_crypto_kmip.go:198-225`, `tier2`), além de outras com `password`/`vm_password` (`grep` em `generated_*.go`). **Logo, o audit grava segredos reais em disco.**
- **Linha de auditoria também vaza o erro do backend**, que pode conter a URL (ver §6).
- **Para o mcpollama:** auditar metadados (ferramenta, caminho já normalizado, tamanho, hash, duração, contadores), nunca `args` crus, porque `generate`/`chat`/`summarize_file` carregam texto do usuário e conteúdo de arquivo possivelmente sensível. Lista de chaves a mascarar é a opção fraca; **allowlist de campos auditáveis** é a certa.

## 6. Segredos em log e em erro

| Vazamento | Evidência | Medido? |
| --- | --- | --- |
| `args` completos no audit log | §5 | **sim** |
| `args` completos no `--debug` (`[STDIN] <linha inteira>` em stderr) | `main.go:325-327` | por leitura de código |
| `vm_password` do Workstation entra na **URL** e a URL entra na mensagem de erro | `workstation/client.go:139, 151, 153` imprimem `u.String()`; `workstation_vm.go:78-87` monta `path + "?" + v.Encode()` | **sim**: com a porta morta o erro continha `.../vms/abc%3FvmPassword=[valor fictício]` |
| Defeito colateral do item acima | `u.Path = u.Path + path` (`workstation/client.go:120`) com `?` dentro do path → vira `%3F` no **caminho** em vez de query | **sim** (URL observada); que a feature falha contra um `vmrest` real é **(inferência)**: a URL enviada não tem query string |
| `refresh_token` do VMC na **query** do POST de troca | `cloudaws/client.go:141`; erros de transporte envolvem o `*url.Error` com a URL (`:149`) | **(inferência)** por analogia com o caso medido; não testado (chamaria o serviço real do VMC, que a própria lição `cerebrum.md:472-480` proíbe) |
| Erro de conexão do vSphere usa `u.Redacted()` (esconde senha em *userinfo*) | `vmware/client.go:86, 92` | por leitura de código; é a única redação do projeto |
| `.mcp.json` com credenciais reais fora do `.gitignore` | `STATUS.md:101` registra que o `.mcp.json` com credenciais reais "não estava no `.gitignore`" apesar do aviso no próprio arquivo; corrigido | por leitura de STATUS |

**Regras para o mcpollama (candidatas a requisito SEC):** (a) segredo nunca na URL (usar `Authorization: Bearer`); (b) toda mensagem de erro que inclua URL passa por uma função única que remove `userinfo` e query; (c) `--debug` redige `arguments`, ou loga só tamanho; (d) o token vem de arquivo/env e nunca é impresso (inclusive no `--version`).

## 7. Superfície de leitura de arquivos (a lacuna que o mcpollama precisa fechar)

MCPVMWare tem **uma** tool que lê um arquivo do disco do servidor: `vmware_datastore_upload_file`, com `local_path` livre (`tools/datastore.go:71-109`, lido por `ds.UploadFile`; descrição: "local_path is read from the filesystem of the machine running this MCP server", `:22`). **Não há allowlist, normalização, resolução de *symlink* nem limite de tamanho**: `grep` por `Clean|Abs|EvalSymlinks|allow-root` nas tools (excluídos testes) não acha nada. As tools de OVA/OVF leem do disco também (`generated_vcenter_template.go:59` menciona `os.ReadFile` do `.ovf`). Aqui o risco é aceito porque o chamador é o dono da máquina; no mcpollama as tools `summarize_file`/`extract`/`translate` leem **por pedido do modelo**, então a lista de raízes permitidas (`--allow-root`), a resolução de `..`/*symlink*/ADS/caminhos UNC do Windows e o teto de bytes precisam existir **desde a primeira versão**.
