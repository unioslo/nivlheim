# cd to script's directory
$scriptpath = $MyInvocation.MyCommand.Path
$dir = Split-Path $scriptpath
cd $dir

echo "------ Add a registry key with the client configuration ------"
$Content = Get-Content -path "../client/windows/client.conf"
$Bytes = [System.Text.Encoding]::UTF8.GetBytes($Content)
$Encoded = [convert]::ToBase64String($Bytes)
reg add HKEY_LOCAL_MACHINE\SOFTWARE\Nivlheim /f /v config /t REG_SZ /d $Encoded

echo "------ Run the client ------------"
../client/windows/nivlheim_client.ps1 -certfile nivlheim.p12 -logfile nivlheim.log -server bilderbygger.uio.no -trustallcerts:1 -nosleep:1
