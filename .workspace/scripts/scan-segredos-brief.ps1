<#
.SYNOPSIS
  Varredura de segredos do brief das referencias do mcpollama (gate de toda onda).

.DESCRIPTION
  Carrega EM MEMORIA os valores sensiveis conhecidos das fontes de referencias/ (e do
  csl-ollama.env) e procura cada um, literalmente, em referencias/Brief_Referencias/**.
  Depois procura padroes genericos (Bearer, api_key, password, senha, PRIVATE KEY).

  Nunca imprime um valor: so contagens e arquivo:linha.

  Saida:
    loaded=<n>          valores conhecidos carregados (tem de ser > 0, senao o gate e vazio)
    known_hits=<n>      ocorrencias de valores conhecidos no brief (gate duro: 0)
    generic_hits=<n>    ocorrencias de padroes genericos (triagem)
    generic_untriaged=<n>  genericos sem [omitido] na linha (revisar a mao)

  Exit: 0 = known_hits 0 e loaded > 0; 1 = known_hits > 0; 2 = loaded 0 (gate invalido).

.PARAMETER Root
  Raiz do projeto (default: dois niveis acima deste script).

.PARAMETER BriefDir
  Pasta varrida (default: referencias/Brief_Referencias). Usado no controle positivo:
  apontar para uma copia temporaria de uma fonte sensivel e esperar known_hits > 0.
#>
param(
    [string]$Root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path,
    [string]$BriefDir,
    [switch]$Diagnose
)

$ErrorActionPreference = 'Stop'
$refs  = Join-Path $Root 'referencias'
$brief = if ($BriefDir) { (Resolve-Path $BriefDir).Path } else { Join-Path $refs 'Brief_Referencias' }

$values = New-Object 'System.Collections.Generic.HashSet[string]'
$valueSrc = @{}          # valor -> arquivo de origem (so para -Diagnose; nunca impresso)
$script:curSrc = ''
$script:allowed = 0

# Allowlist de falsos positivos JA TRIADOS: um SHA-256 (hex) por linha, com comentario apos '#'.
# Nunca guarda o valor em claro. Cada entrada tem a justificativa registrada no plano do brief.
function Get-Sha256([string]$s) {
    $sha = [Security.Cryptography.SHA256]::Create()
    try { return ([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($s))) -replace '-', '').ToLower() }
    finally { $sha.Dispose() }
}
$allow = New-Object 'System.Collections.Generic.HashSet[string]'
$allowFile = Join-Path $PSScriptRoot 'scan-segredos-allowlist.txt'
if (Test-Path -LiteralPath $allowFile) {
    foreach ($l in [IO.File]::ReadAllLines($allowFile)) {
        $h = ($l -split '#')[0].Trim().ToLower()
        if ($h -match '^[0-9a-f]{64}$') { [void]$allow.Add($h) }
    }
}

function Get-Shape([string]$v) {
    # Descreve a FORMA do valor sem revela-lo. So imprime o texto quando e claramente um NOME
    # (UPPER_SNAKE de variavel de ambiente), que nao e segredo.
    if ($v -cmatch '^[A-Z][A-Z0-9_]{2,}$') { return "nome-de-variavel: $v" }
    if ($v -match '^\d{1,3}(\.\d{1,3}){3}(:\d+)?$') { return 'IP' }
    if ($v -match '^[A-Za-z]:\\|^/|\\') { return "caminho (len=$($v.Length))" }
    if ($v -match '^https?://') { return "URL (len=$($v.Length))" }
    if ($v -match '^[a-z0-9-]+(\.[a-z0-9-]+)+$') { return "host/dominio (len=$($v.Length))" }
    $kinds = @(); if ($v -cmatch '[a-z]') { $kinds += 'minusc' }; if ($v -cmatch '[A-Z]') { $kinds += 'maiusc' }
    if ($v -match '\d') { $kinds += 'digito' }; if ($v -match '[^A-Za-z0-9]') { $kinds += 'simbolo' }
    return "outro (len=$($v.Length), $($kinds -join '+'))"
}
$sensitiveKey = '(?i)(pass|pwd|senha|token|secret|api[-_]?key|apikey|auth|credential|bearer)'
$inlineRx = '(?i)(?:senha|password|passwd|pwd|token|api[-_]?key|secret|bearer)\s*[:=|]\s*[`"'']?([^\s`"''|,;)]+)|(?:^|\s)(?:-p|-pw|--password|--pass|--api-key|--token)[\s=]+[`"'']?([^\s`"'')]+)'

function Add-Value([string]$v) {
    if ([string]::IsNullOrWhiteSpace($v)) { return }
    $v = $v.Trim()
    if ($v.Length -lt 6) { return }                       # curto demais = ruido
    if ($v -match '^\[omitido\]$|^\$\{?env|^%[A-Z_]+%$|^<[^>]+>$') { return }   # placeholders
    if ($v -match '^(true|false|null|localhost|https?://[^@]*)$') { return }
    if ($v -match '^--?[A-Za-z][\w-]*$') { return }       # nome de flag (ex.: --insecure), nao valor
    if ($allow.Contains((Get-Sha256 $v))) { $script:allowed++; return }   # falso positivo ja triado
    if ($values.Add($v)) { $valueSrc[$v] = $script:curSrc }
}

function Add-FromText([string]$text) {
    foreach ($m in [regex]::Matches($text, $inlineRx, 'Multiline')) {
        if ($m.Groups[1].Success) { Add-Value $m.Groups[1].Value }
        if ($m.Groups[2].Success) { Add-Value $m.Groups[2].Value }
    }
}

function Walk-Json($node, [string]$key, [bool]$underEnvOrArgs) {
    if ($null -eq $node) { return }
    if ($node -is [string]) {
        if ($underEnvOrArgs -or ($key -match $sensitiveKey)) { Add-Value $node }
        Add-FromText $node
        return
    }
    if ($node -is [System.Collections.IEnumerable] -and -not ($node -is [string])) {
        foreach ($item in $node) { Walk-Json $item $key $underEnvOrArgs }
        return
    }
    if ($node -is [psobject]) {
        foreach ($p in $node.PSObject.Properties) {
            $flag = $underEnvOrArgs -or ($p.Name -in @('env', 'args', 'headers'))
            # Postman: { key: "password", value: "..." }
            if ($p.Name -eq 'value' -and $node.PSObject.Properties['key'] -and ([string]$node.key -match $sensitiveKey)) {
                Add-Value ([string]$p.Value)
            }
            Walk-Json $p.Value $p.Name $flag
        }
    }
}

# --- fontes JSON ---------------------------------------------------------------
$jsonSources = @()
$jsonSources += Get-ChildItem $refs -Recurse -Force -File -Filter '.mcp.json' -ErrorAction SilentlyContinue
$jsonSources += Get-ChildItem $refs -Recurse -Force -File -Filter 'settings.local.json' -ErrorAction SilentlyContinue |
    Where-Object { $_.DirectoryName -like '*\.claude' }
$jsonSources += Get-Item (Join-Path $refs 'MCPVMWare\.workspace\config_teste.json') -ErrorAction SilentlyContinue
$jsonSources += Get-ChildItem (Join-Path $refs 'MCPVMWare\.workspace') -File -Filter '*.postman_collection.json' -ErrorAction SilentlyContinue
$jsonSources = $jsonSources | Where-Object { $_ -and $_.FullName -notlike "$brief*" }

foreach ($f in $jsonSources) {
    $script:curSrc = $f.FullName.Substring($refs.Length + 1)
    $raw = [IO.File]::ReadAllText($f.FullName)
    try { Walk-Json ($raw | ConvertFrom-Json) '' $false } catch { Add-FromText $raw }
}

# --- fontes texto --------------------------------------------------------------
$textSources = @()
$textSources += Get-Item (Join-Path $refs 'MCPVMWare\.workspace\DADOS_CONEXAO_TESTE.md') -ErrorAction SilentlyContinue
$textSources += Get-ChildItem (Join-Path $refs 'MCPVMWare\.workspace\reports') -File -Filter '*configuracao-servidores-teste*' -ErrorAction SilentlyContinue
$textSources += Get-ChildItem (Join-Path $refs 'MCPNas\truenas-mcp\.workspace\reports') -File -Filter '*servidor-teste-config*' -ErrorAction SilentlyContinue
$textSources += Get-ChildItem $refs -Recurse -Force -File -Filter '*.env' -ErrorAction SilentlyContinue |
    Where-Object { $_.FullName -notlike "$brief*" }
foreach ($f in ($textSources | Where-Object { $_ })) { $script:curSrc = $f.FullName.Substring($refs.Length + 1); Add-FromText ([IO.File]::ReadAllText($f.FullName)) }

# --- token do Ollama da CSL ----------------------------------------------------
$envFile = Join-Path $env:USERPROFILE '.openclaude\csl-ollama.env'
if (Test-Path -LiteralPath $envFile) {
    $script:curSrc = 'csl-ollama.env'
    foreach ($line in [IO.File]::ReadAllLines($envFile)) {
        if ($line -match '^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*["'']?(.*?)["'']?\s*$' -and $Matches[1] -match $sensitiveKey) {
            Add-Value $Matches[2]
        }
    }
}

$sourcesCount = @($jsonSources).Count + @($textSources | Where-Object { $_ }).Count + [int](Test-Path -LiteralPath $envFile)
Write-Output "sources=$sourcesCount loaded=$($values.Count) allowlisted=$($script:allowed)"

# --- busca no brief ------------------------------------------------------------
$knownHits = 0; $genericHits = 0; $untriaged = 0
$genericRx = '(?i)Bearer\s+[A-Za-z0-9]|api[-_]?key|password|senha|BEGIN .*PRIVATE KEY'
$files = @(Get-ChildItem $brief -Recurse -File -ErrorAction SilentlyContinue)
foreach ($f in $files) {
    $rel = if ($f.FullName.StartsWith($Root)) { $f.FullName.Substring($Root.Length + 1) } else { $f.Name }
    $lines = [IO.File]::ReadAllLines($f.FullName)
    for ($i = 0; $i -lt $lines.Count; $i++) {
        $line = $lines[$i]
        foreach ($v in $values) {
            if ($line.Contains($v)) {
                $knownHits++
                if ($Diagnose) { Write-Output "KNOWN  ${rel}:$($i + 1)  <- $($valueSrc[$v]) | $(Get-Shape $v)" }
                else { Write-Output "KNOWN  ${rel}:$($i + 1)" }
                break
            }
        }
        if ($line -match $genericRx) {
            $genericHits++
            if ($line -notmatch '\[omitido\]') { $untriaged++; Write-Output "GENERIC ${rel}:$($i + 1)" }
        }
    }
}

Write-Output "files=$($files.Count) known_hits=$knownHits generic_hits=$genericHits generic_untriaged=$untriaged"
if ($values.Count -eq 0) { exit 2 }
if ($knownHits -gt 0) { exit 1 }
exit 0
