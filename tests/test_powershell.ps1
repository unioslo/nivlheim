echo "------------- Testing the Powershell client ------------"

# cd to script's directory
$scriptpath = $MyInvocation.MyCommand.Path
$dir = Split-Path $scriptpath
cd $dir

# Put the client configuration in registry
$Content = Get-Content -path "../client/windows/client.conf"
$Bytes = [System.Text.Encoding]::UTF8.GetBytes($Content)
$Encoded = [convert]::ToBase64String($Bytes)
reg add HKEY_LOCAL_MACHINE\\SOFTWARE\\Nivlheim /f /v config /t REG_SZ /d $Encoded

# Run the client
../client/windows/nivlheim_client.ps1 -certfile nivlheim.p12 -logfile nivlheim.log -server bilderbygger.uio.no -trustallcerts:1 -nosleep:1
