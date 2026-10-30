<#
.SYNOPSIS
  Instalador do mcpollama-gateway (fila + /monitor, Fase 9.6) no servidor de IA, a partir deste PC.

.DESCRIPTION
  Modos (-Modo):
    Completo   (padrao) gera o binario Linux, monta o pacote com manifesto SHA-256, envia para
               /root/deploy96 (root 0700), roda a etapa 1, pede a senha do admin do /monitor se ela
               ainda nao existir (voce digita no seu terminal, sem eco) e roda a etapa 2 (ativacao).
    Preparar   so a etapa 1 (usuario, pastas, binario); nao mexe no trafego.
    Atualizar  troca so o binario de um gateway ja instalado: etapa 1 + restart + conferencia.
    Senha      so (re)define a senha do admin do /monitor.
    Reverter   roda o rollback de uma ativacao: -Backup /root/srv-backup-20261028/fase96-ativacao-AAAAMMDD-HHMMSS

  Seguranca:
    - o pacote vai para uma pasta so do root (0700);
    - o hash do manifesto e calculado aqui e passado como argumento, e as etapas conferem todos os arquivos contra ele;
    - nenhum token ou senha passa por este script.

.EXAMPLE
  powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1
.EXAMPLE
  powershell -NoProfile -ExecutionPolicy Bypass -File .\Instalar-Gateway.ps1 -Modo Atualizar
#>
[CmdletBinding()]
param(
    [ValidateSet('Completo', 'Preparar', 'Atualizar', 'Senha', 'Reverter')]
    [string]$Modo = 'Completo',
    [string]$Servidor = 'lx-ai01-root',      # alias do ~/.ssh/config (root@10.100.2.2)
    [string]$Versao = '',                    # padrao: 0.9.0-<commit>
    [switch]$SemBuild,                       # usa o D:\MCP\mcpollama-gateway existente
    [string]$Backup = ''                     # para -Modo Reverter
)
$ErrorActionPreference = 'Stop'
$Aqui = Split-Path -Parent $MyInvocation.MyCommand.Path
$Repo = (Resolve-Path (Join-Path $Aqui '..\..')).Path
$Bin = 'D:\MCP\mcpollama-gateway'
$Remoto = '/root/deploy96'
$Arquivos = 'mcpollama-gateway.service', 'ollama.conf', 'stage1.sh', 'stage2.sh', 'rollback.sh', 'mcpgw-definir-senha.sh'

function Passo($t) { Write-Host "`n== $t" -ForegroundColor Cyan }
function Invoke-Remoto([string]$cmd) {
    & ssh.exe -o LogLevel=ERROR -o ConnectTimeout=10 $Servidor $cmd
    if ($LASTEXITCODE -ne 0) { throw "falhou no servidor (exit $LASTEXITCODE): $cmd" }
}

if ($Modo -eq 'Reverter') {
    if (-not $Backup) { throw 'informe -Backup <pasta fase96-ativacao-*> (a etapa 2 imprime o caminho)' }
    Passo "Rollback a partir de $Backup"
    Invoke-Remoto "bash '$Backup/rollback.sh' '$Backup'"
    return
}
if ($Modo -eq 'Senha') {
    Passo 'Senha do admin do /monitor (digite no terminal; nada passa por este script)'
    & ssh.exe -o LogLevel=ERROR -t $Servidor 'bash /root/mcpgw-definir-senha.sh'
    return
}

# 1) binario
if (-not $SemBuild) {
    if (-not $Versao) { $Versao = '0.9.0-' + (& git -C $Repo rev-parse --short HEAD).Trim() }
    Passo "Gerando o binario Linux $Versao em $Bin"
    Push-Location (Join-Path $Repo 'src')
    try {
        $env:GOOS = 'linux'; $env:CGO_ENABLED = '0'
        & go build -ldflags "-X main.Version=$Versao" -o $Bin ./cmd/mcpollama-gateway
        if ($LASTEXITCODE -ne 0) { throw 'go build falhou' }
    } finally { Remove-Item Env:GOOS, Env:CGO_ENABLED -ErrorAction SilentlyContinue; Pop-Location }
}
if (-not (Test-Path $Bin)) { throw "binario ausente: $Bin" }

# 2) pacote local com finais de linha LF e manifesto SHA-256
Passo 'Montando o pacote e o manifesto SHA-256'
$Pacote = Join-Path ([IO.Path]::GetTempPath()) ('deploy96-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $Pacote | Out-Null
try {
    $utf8 = New-Object Text.UTF8Encoding $false
    foreach ($f in $Arquivos) {
        $txt = [IO.File]::ReadAllText((Join-Path $Aqui $f)) -replace "`r`n", "`n"
        [IO.File]::WriteAllText((Join-Path $Pacote $f), $txt, $utf8)
    }
    Copy-Item $Bin (Join-Path $Pacote 'mcpollama-gateway')
    $linhas = foreach ($f in ($Arquivos + 'mcpollama-gateway')) {
        (Get-FileHash -Algorithm SHA256 (Join-Path $Pacote $f)).Hash.ToLower() + '  ' + $f
    }
    [IO.File]::WriteAllText((Join-Path $Pacote 'SHA256SUMS'), (($linhas -join "`n") + "`n"), $utf8)
    $SumsHash = (Get-FileHash -Algorithm SHA256 (Join-Path $Pacote 'SHA256SUMS')).Hash.ToLower()
    Write-Host "SHA256SUMS = $SumsHash"

    # 3) envio para pasta so do root
    Passo "Enviando para ${Servidor}:$Remoto (root 0700)"
    Invoke-Remoto "rm -rf $Remoto && install -d -m 0700 -o root -g root $Remoto"
    & scp.exe -q -o LogLevel=ERROR -o ConnectTimeout=10 (Get-ChildItem $Pacote | ForEach-Object FullName) "${Servidor}:$Remoto/"
    if ($LASTEXITCODE -ne 0) { throw 'scp falhou' }
    Invoke-Remoto "chown root:root $Remoto/* && chmod 0600 $Remoto/* && chmod 0700 $Remoto/*.sh $Remoto/mcpollama-gateway && install -m 0700 -o root -g root $Remoto/mcpgw-definir-senha.sh /root/mcpgw-definir-senha.sh"
} finally { Remove-Item -Recurse -Force $Pacote -ErrorAction SilentlyContinue }

# 4) etapa 1
Passo 'Etapa 1 (preparo; nao mexe no trafego)'
Invoke-Remoto "bash $Remoto/stage1.sh $SumsHash"
if ($Modo -eq 'Preparar') { Write-Host "`nPronto (so preparo)." -ForegroundColor Green; return }

if ($Modo -eq 'Atualizar') {
    Passo 'Reiniciando o gateway com o binario novo'
    # sem aspas duplas no comando remoto: o PowerShell 5.1 as remove ao chamar o ssh.exe
    Invoke-Remoto 'systemctl restart mcpollama-gateway && sleep 2 && systemctl is-active mcpollama-gateway && /usr/local/bin/mcpollama-gateway --version && echo gateway_api_version=$(curl -s -m 5 -o /dev/null -w %{http_code} http://127.0.0.1:11500/api/version) && echo gateway_monitor=$(curl -s -m 5 -o /dev/null -w %{http_code} http://127.0.0.1:11500/monitor)'
    Write-Host "`nBinario atualizado." -ForegroundColor Green
    return
}

# 5) senha (so se ainda nao existir)
& ssh.exe -o LogLevel=ERROR -o ConnectTimeout=10 $Servidor 'test -s /etc/mcpollama-gateway/admin'
if ($LASTEXITCODE -ne 0) {
    Passo 'Senha do admin do /monitor ainda nao definida: digite agora (sem eco)'
    & ssh.exe -o LogLevel=ERROR -t $Servidor 'bash /root/mcpgw-definir-senha.sh'
    & ssh.exe -o LogLevel=ERROR -o ConnectTimeout=10 $Servidor 'test -s /etc/mcpollama-gateway/admin'
    if ($LASTEXITCODE -ne 0) { throw 'senha nao definida; rode de novo com -Modo Senha e depois -Modo Completo -SemBuild' }
}

# 6) etapa 2
Passo 'Etapa 2 (ativacao: gateway, Ollama so no loopback, nginx -> gateway; rollback automatico se falhar)'
Invoke-Remoto "bash $Remoto/stage2.sh $SumsHash"
Write-Host "`nInstalado. Monitor: https://ia.cslsolucoes.com.br:8443/monitor (so da LAN)." -ForegroundColor Green
