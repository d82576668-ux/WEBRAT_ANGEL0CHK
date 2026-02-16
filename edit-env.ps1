$ErrorActionPreference = "Stop"
$p = ".env"
if (-not (Test-Path $p)) {
  "" | Out-File -FilePath $p -Encoding UTF8
}
$kv = @{}
Get-Content $p | ForEach-Object {
  if ($_ -match '^\s*#' -or $_ -match '^\s*$') { }
  else {
    $pair = $_ -split '=', 2
    if ($pair.Length -ge 2) { $kv[$pair[0]] = $pair[1] }
  }
}
$curDb = if ($kv.ContainsKey("DATABASE_URL")) { $kv["DATABASE_URL"] } else { "" }
$curSecret = if ($kv.ContainsKey("STREAM_SECRET")) { $kv["STREAM_SECRET"] } else { "webrat-secret" }
$curPort = if ($kv.ContainsKey("PORT")) { $kv["PORT"] } else { "8080" }
Write-Host "DATABASE_URL [$curDb]:" -NoNewline
$db = Read-Host
if ([string]::IsNullOrWhiteSpace($db)) { $db = $curDb }
Write-Host "STREAM_SECRET [$curSecret]:" -NoNewline
$secret = Read-Host
if ([string]::IsNullOrWhiteSpace($secret)) { $secret = $curSecret }
Write-Host "PORT [$curPort]:" -NoNewline
$port = Read-Host
if ([string]::IsNullOrWhiteSpace($port)) { $port = $curPort }
$lookup = @{ "DATABASE_URL" = $db; "STREAM_SECRET" = $secret; "PORT" = $port }
$lines = Get-Content $p
$newLines = New-Object System.Collections.Generic.List[string]
$seen = @{}
foreach ($line in $lines) {
  $matched = $false
  foreach ($name in $lookup.Keys) {
    if ($line -match ('^' + [regex]::Escape($name) + '\s*=')) {
      $newLines.Add($name + '=' + $lookup[$name])
      $seen[$name] = $true
      $matched = $true
      break
    }
  }
  if (-not $matched) { $newLines.Add($line) }
}
foreach ($name in $lookup.Keys) {
  if (-not $seen.ContainsKey($name)) {
    $newLines.Add($name + '=' + $lookup[$name])
  }
}
$newLines | Out-File $p -Encoding UTF8
Write-Host "Updated .env"
