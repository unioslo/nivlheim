############################################################################
#
#  nivlheim_client.ps1
#
#  This script is supposed to be run once an hour. It gathers configuration
#  information from the host, and sends it to the Nivlheim server.
#  For more information, see: http://nivlheim.uio.no/
#
############################################################################

<#
.Synopsis
This script is supposed to be run once an hour. It gathers configuration
information from the host, and sends it to the Nivlheim server.
.Description
For more information, see: http://nivlheim.uio.no/
.Link
nivlheim_client.ps1
.Inputs
None
.Notes
Authors: Øyvind Hagberg, Mustafa Ocak
#>

param(
#	[string]$config = "C:\Program Files (x86)\Nivlheim\etc\nivlheim.conf",
	[string]$certfile = "",
	[string]$logfile = "C:\Program Files (x86)\Nivlheim\logs\nivlheim.log",
	[string]$server = "",
	[bool]$testmode = $false,
	[bool]$nosleep = $false
)

Set-Variable version -option Constant -value "2.7.17"
Set-Variable useragent -option Constant -value "NivlheimPowershellClient/$version"
Set-PSDebug -strict
Set-StrictMode -version "Latest"	# http://technet.microsoft.com/en-us/library/hh849692.aspx
Add-Type -Assembly System.Web   # we need System.Web.HttpUtility

# syntax for putting functions in a separate file:
# $functions = "$($MyInvocation.MyCommand.path | split-path)\functions.ps1"
# . $functions

function Parse-Ini
{
	$ini = @{}
	$section = "_"
	$ini[$section] = @{}
	foreach ($s in $input) {
		switch -regex ($s)
		{
			"^\[(.+)\]$" # Section
			{
				$section = $matches[1]
				$ini[$section] = @{}
				#Write-Host "Section: $section"
				break
			}
			"^\s*([;#].*)\s*$" # Comment
			{
				break
			}
			"^(.+?)\s*=(.*)$" # Key and value pair
			{
				$name,$value = $matches[1..2]
				if ($section -eq "commands") {
					# commands may include equal signs, so
					# the usual key=value interpretation will not work.
					$ini[$section]["$name=$value"] = ""
					#Write-Host "Command: $name=$value"
				} else {
					$ini[$section][$name] = $value
					#Write-Host "Key-value: $name = $value"
				}
				break
			}
			"^(.+)\s*$" # Key without value
			{
				$name = $matches[1]
				$ini[$section][$name] = ""
				#Write-Host "Key without value: $name"
				break
			}
		}
	}
	return $ini
}

function IsNull($objectToCheck) {
	if (!$objectToCheck) {
		return $true
	}

	if ($objectToCheck -is [String] -and $objectToCheck -eq [String]::Empty) {
		return $true
	}

	if ($objectToCheck -is [DBNull] -or $objectToCheck -is [System.Management.Automation.Language.NullString]) {
		return $true
	}

	return $false
}

function Bind-IPEndPointCallback([System.Net.IPAddress]$ip,$port) {
	New-Object -TypeName System.Net.IPEndPoint $ip, $port
}

function http($uri, $method = "get", $timeoutSeconds = 60, $clientCert = $null, $params = $null) {
	Write-Host $method.ToUpper() $uri

	if ($method -ne "get" -and $method -ne "post") {
		throw "Unknown HTTP method: $method"
	}

	# By default, Powershell uses TLS 1.0. The server security requires TLS 1.2 or 1.1.
	# Also, TLSv1.0 has been deprecated due to POODLE, so let's not use that.
	# https://stackoverflow.com/questions/41618766/
	[Net.ServicePointManager]::SecurityProtocol = "tls12, tls11"

	$WebRequest = [System.Net.HttpWebRequest]::Create($uri);
	$WebRequest.UserAgent = $useragent
	$WebRequest.Timeout = $timeoutSeconds * 1000
	$WebRequest.Method = $method

	# Deprecated: Bind to a certain address when contacting the server.
	# if (-Not (IsNull $Script:myaddr)) {
	#	$WebRequest.ServicePoint.BindIPEndPointDelegate = { (Bind-IPEndPointCallback -ip $Script:myaddr -port 0 ) }
	# }

	if ($clientCert) {
		$WebRequest.ClientCertificates.Add($clientCert) | Out-Null
	}
	if ($params -and ($method -eq "post")) {
		$WebRequest.ContentType = "application/x-www-form-urlencoded"
		# convert dictionary to query string
		# Add-Type -Assembly System.Web  # was done at the start of the script, no need to do it here
		$str = ""
		$params.Keys | ForEach-Object {
			$str += [System.Web.HttpUtility]::UrlEncode($_)
			$str += "="
			$str += [System.Web.HttpUtility]::UrlEncode($params[$_])
			$str += "&"
		}
		# convert query string to bytes, assume utf-8
		$enc = [system.Text.Encoding]::UTF8
		$bodyBytes = $enc.getBytes($str)
		# send request body
		$WebRequest.ContentLength = $bodyBytes.Length
		$dataStream = $WebRequest.GetRequestStream()
		$dataStream.Write($bodyBytes, 0, $bodyBytes.Length)
		$dataStream.Close()
	}
	$Response = $null
	try {
		$Response = $WebRequest.GetResponse()
		Write-Host "Status:" ($Response.StatusCode -as [int]) $Response.StatusCode
		$sr = new-object IO.StreamReader($Response.GetResponseStream())
		$result = $sr.ReadToEnd()
		return $result
	}
	catch [System.Net.WebException] {
		if ($_.Exception.Status -eq [System.Net.WebExceptionStatus]::ProtocolError) {
			$Response = $_.Exception.Response
			Write-Host "HTTP Status:" ($Response.StatusCode -as [int]) $Response.StatusCode
			$sr = new-object IO.StreamReader($Response.GetResponseStream())
			Write-Host $sr.ReadToEnd()
		}
		throw $_.Exception
	}
	finally {
		try { $Response.Close() } catch {}
	}
}

function CountZipItemsRecursive( [__ComObject] $parent ) {
	[int] $count = 0
	$parent.Items() | ForEach-Object {
		$count += 1
		If ($_.IsFolder -eq $true) {
			$count += CountZipItemsRecursive($_.GetFolder)
		}
	}
	return $count
}

function zipfolder($foldername, $zipfilename) {
	$r = Test-Path $foldername
	if (-not $r) {
		throw "$foldername does not exist!"
	}
	try {
		[System.Reflection.Assembly]::LoadWithPartialName("System.IO.Compression.FileSystem") | Out-Null
		[System.IO.Compression.ZipFile]
		# If the previous 2 lines didn't throw an exception, we have the ZipFile class.
		Write-Host "zipfolder: Using System.IO.Compression.ZipFile"
		$compressionLevel = [System.IO.Compression.CompressionLevel]::Fastest
		$r = [System.IO.Compression.ZipFile]::CreateFromDirectory($foldername, $zipfilename, $compressionLevel, $false)
	}
	catch {
		Write-Host "zipfolder: Using a Shell.Application COM object"

		# Manually create an empty zip file
		$bytes = [Byte[]] (80,75,5,6,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0)
		[IO.File]::WriteAllBytes($zipfilename, $bytes)

		# Create a com object
		$shellApp = New-Object -ComObject Shell.Application
		if ($shellApp -eq $null) {
			throw "Failed to create a Shell.Application COM object"
		}
		$zipFile = $shellApp.NameSpace($zipfilename)
		if ($zipFile -eq $null) {
			throw "Failed to get the zip file namespace from the COM object"
		}

		# Add stuff to the zip file
		$total = 0
		Get-ChildItem $foldername | ?{ $_.PSIsContainer } | ForEach-Object {  # list only folders
			$child = "$foldername\$_"
			if (Get-ChildItem $child) {  # if it isn't empty
				$zipFile.CopyHere($child, 4 + 16 + 512 + 1024)
				$total += @(Get-ChildItem -Path $child -Force -Recurse).Count
				$total++ # the folder itself
			}
		}

		# Wait for the zipping to complete. Count files and dirs in the zip archive.
		$zipitems = 0
		$iterations = 0
		while ($zipitems -lt $total -and $iterations -lt 10) {
			Start-Sleep -Milliseconds 500
			$zipitems = CountZipItemsRecursive($zipFile)
			# Write-Host "I count" $zipitems "of" $total "expected items in the zip file"
			$iterations++
		}
	}
}

# https://gallery.technet.microsoft.com/scriptcenter/PowerShell-Script-to-Roll-a96ec7d4
function Reset-Log
{
	# The function checks to see if file in question is larger than the parameter specified.
	# If it is, it will roll a log and delete the oldes log if there are more than x logs.
	param([string]$fileName, [int64]$filesize = 1mb , [int] $logcount = 5)

	$logRollStatus = $true
	if(test-path $filename)
	{
		$file = Get-ChildItem $filename
		if((($file).length) -ige $filesize) #this starts the log roll
		{
			$fileDir = $file.Directory
			#this gets the name of the file we started with
			$fn = $file.name
			$files = Get-ChildItem $filedir | ?{$_.name -like "$fn*"} | Sort-Object lastwritetime
			#this gets the fullname of the file we started with
			$filefullname = $file.fullname
			for ($i = (@($files).count); $i -gt 0; $i--)
			{
				$files = Get-ChildItem $filedir | ?{$_.name -like "$fn*"} | Sort-Object lastwritetime
				$operatingFile = $files | ?{($_.name).trim($fn) -eq $i}
				if ($operatingfile)
				 {$operatingFilenumber = ($files | ?{($_.name).trim($fn) -eq $i}).name.trim($fn)}
				else
				{$operatingFilenumber = $null}

				if(($operatingFilenumber -eq $null) -and ($i -ne 1) -and ($i -lt $logcount))
				{
					$operatingFilenumber = $i
					$newfilename = "$filefullname.$operatingFilenumber"
					$operatingFile = $files | ?{($_.name).trim($fn) -eq ($i-1)}
					write-host "moving to $newfilename"
					move-item ($operatingFile.FullName) -Destination $newfilename -Force
				}
				elseif($i -ge $logcount)
				{
					if($operatingFilenumber -eq $null)
					{
						$operatingFilenumber = $i - 1
						$operatingFile = $files | ?{($_.name).trim($fn) -eq $operatingFilenumber}

					}
					write-host "deleting " ($operatingFile.FullName)
					remove-item ($operatingFile.FullName) -Force
				}
				elseif($i -eq 1)
				{
					$operatingFilenumber = 1
					$newfilename = "$filefullname.$operatingFilenumber"
					write-host "moving to $newfilename"
					move-item $filefullname -Destination $newfilename -Force
				}
				else
				{
					$operatingFilenumber = $i +1
					$newfilename = "$filefullname.$operatingFilenumber"
					$operatingFile = $files | ?{($_.name).trim($fn) -eq ($i-1)}
					write-host "moving to $newfilename"
					move-item ($operatingFile.FullName) -Destination $newfilename -Force
				}
			}
		  }
		 else
		 { $logRollStatus = $false}
	}
	else
	{
		$logrollStatus = $false
	}
	$LogRollStatus
}

# create a shortened version of a command line, usable as a file name
function shortencmd($orig) {
	$s = "";
	$i = 0;
	$orig = $orig -replace "\S+\\", "";
	while ($s.Length -lt 30 -and $i -lt $orig.Length) {
		$c = $orig.Substring($i++, 1)
		if ($c -match '[a-zA-Z0-9-]') {
			$s = $s + $c
		}
		else {
			$s = $s + '_'
		}
	}
	# make sure it doesn't look like a hex string, this is necessary
	# because of backward compatibility on the server side.
	if ($s -match '^[a-fA-F0-9]+$') {
		$s = $s + '_'
	}
	return $s
}

function dotNetVersion() {
	try {
		return (Get-ItemProperty "HKLM:\SOFTWARE\Microsoft\NET Framework Setup\NDP\v4\Full").Version
	} catch {
		return "unknown"
	}
}

function CanAccessPath($path) {
	if ((Test-Path $path) -eq $true) {
		$x = gci $path -ErrorAction SilentlyContinue
		if ($Error[0].Exception.GetType().Name -eq 'UnauthorizedAccessException') {
			return $false
		}
		return $true
	}
	return $false
}

function ParseAndSaveCertificateFromResult($r) {
	if (IsNull $r) {
		return $false
	}
	# Try and parse and write the P12 file
	if ($r -match "(?s)-----BEGIN P12-----(.*)-----END P12-----") {
		try {
			$p12bytes = [System.Convert]::FromBase64String($matches[1])
			[IO.File]::WriteAllBytes($certpath, $p12bytes)
		} catch {
			Write-Host "Unable to write to the certificate file $certpath"
			Write-Host $error[0]
			return $false
		}
	}
	else {
		Write-Warning "The server didn't give me a certificate:"
		Write-Host $r
		return $false
	}
	# Receive the PEM cert and key too
	if ($r -match "(?s)(-----BEGIN CERTIFICATE-----.*-----END CERTIFICATE-----)") {
		$p = Split-Path -Parent $certpath
		[IO.File]::WriteAllText($p + "/nivlheim.crt", $matches[1])
	} else {
		Write-Host "Failed to obtain a PEM certificate file"
		return $false
	}
	if ($r -match "(?s)(-----BEGIN (?:RSA )?PRIVATE KEY-----.*-----END (?:RSA )?PRIVATE KEY-----)") {
		$p = Split-Path -Parent $certpath
		[IO.File]::WriteAllText($p + "/nivlheim.key", $matches[1])
	} else {
		Write-Host "Failed to obtain a PEM key file"
		return $false
	}
	return $true
}

# Sleep a random interval, so not all the machines try to contact the server
# at the same time every hour.
if (-Not $nosleep -And -not $testmode) {
	$delay = Get-Random -Minimum 1 -Maximum 3300
	Write-Host "Sleeping for $delay seconds..."
	Start-Sleep -Seconds $delay
}

if (-Not $testmode) {
	# show an error message if I can't write to the log file
	try { [io.file]::OpenWrite($logfile).close() }
	catch {
		Write-Warning "Unable to write to the log file: $logfile"
		exit 1
	}
}

try {
	if (-Not $testmode) {
		Reset-Log -fileName $logfile -filesize 100000 -logcount 5 | Out-Null
		try {
			# On PowerShell versions before 6, Start-Transcript doesn't support
			# the -UseMinimalHeader argument, so the command will fail.
			Start-Transcript -Path $logfile -Append -Force -UseMinimalHeader -ErrorAction Stop
		} catch {
			Start-Transcript -Path $logfile -Append -Force -ErrorAction Continue
		}
	}

# We don't want PowerShell to display exceptions that we catch ourselves
$ErrorActionPreference = "Stop"

Write-Host "Nivlheim client version: $version"
Set-Variable psver -option Constant -value "$($PSVersionTable.PSVersion.Major).$($PSVersionTable.PSVersion.Minor)"
Write-Host "Powershell version: $psver"
$x = dotNetVersion
Write-Host ".NET version:" $x

$invocation = (Get-Variable MyInvocation).Value
$dirpath = Split-Path $invocation.MyCommand.Path

if ($testmode) {
	# Read the configuration from a local file
	$conf = Get-Content "client.conf" | Parse-Ini
} else {
	# Read the configuration from registry
	try {
		$base64 = (Get-ItemProperty -Path Registry::HKEY_LOCAL_MACHINE\SOFTWARE\Nivlheim).config
		$text = [Text.Encoding]::UTF8.GetString( [Convert]::FromBase64String($base64) )
		$backToBase64 = [Convert]::ToBase64String( [Text.Encoding]::UTF8.GetBytes($text) )
		if ($base64 -ne $backToBase64) {
			throw "config is not properly base64 encoded"
		}
		$conf = $text.Split([environment]::NewLine) | Parse-Ini
	}
	catch {
		Write-Host "Error while reading config registry value HKEY_LOCAL_MACHINE\SOFTWARE\Nivlheim\config:"
		Write-Host $error[0]
		exit 1
	}
}

# Compute the server url.
$actualserver = "nivlheim.uio.no" # start with a default value
if ($conf.ContainsKey("settings") -And $conf["settings"].ContainsKey("server")) {
	# Got a value from config file/regkey
	$actualserver = $conf["settings"]["server"]
}
if ($server -ne "") {
	# Got a command line argument, that overrides config
	$actualserver = $server
}
$serverbaseurl = "https://$actualserver/cgi-bin/" # must have trailing slash

# Default certificate file path
$certpath = "C:\Program Files (x86)\Nivlheim\var\nivlheim.p12"
# Certificate file path given in config
if ($conf.Containskey("ssl") -and $conf["ssl"].ContainsKey("certfile")) {
	$certpath = $conf["ssl"]["certfile"]
}
# Certificate file path given on command line
if ($certfile -ne "") {
	$certpath = $certfile
}
# Ensure it is a rooted path
if (-Not [System.IO.Path]::IsPathRooted($certpath)) {
	$certpath = $dirpath + "\" + $certpath
}

# Do I have a client certificate?
$haveCert = $false
try {
	[System.IO.File]::OpenRead($certpath).Close()
	if ((Get-Item $certpath).length -gt 0kb) {
		$haveCert = $true
	}
}
catch {}

$cert = $null
if ($haveCert) {
	# I have a certificate, but does it work?
	$flags = [System.Security.Cryptography.X509Certificates.X509KeyStorageFlags]"MachineKeySet"
	$certWorks = $true;
	try {
		$cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($certpath, "", $flags)
		$r = http ($serverbaseurl + "secure/ping") "get" 60 $cert
	} catch {
		$certWorks = $false;
		# Can I connect to the server at all?
		try {
			$r = http ($serverbaseurl + "ping") "get" 60
		}
		catch {
			# No? In that case, quit
			Write-Host "Unable to connect to the server, giving up."
			Write-Host $error[0]
			Write-Host $Error[0].Exception.InnerException
			exit 1
		}
	}
}

if ($haveCert -and -not $certWorks) {
	Write-Host "My certificate doesn't work. Trying to renew it..."
	$r = $null
	try {
		$url = $serverbaseurl + "secure/renewcert"
		$r = http $url "get" 60 $cert
	} catch {
		Write-Host "Renewing didn't work, trying to request a new one"
		$hostname = [System.Web.HttpUtility]::UrlEncode([System.Net.Dns]::GetHostByName(($env:computerName)).Hostname)
		$url = $serverbaseurl + "reqcert?hostname=$hostname"
		try {
			$r = http $url "get" 60
		} catch {
			Write-Host $error[0]
			Write-Host $Error[0].Exception.InnerException
		}
	}
	$ok = ParseAndSaveCertificateFromResult $r
	if (-not $ok) {
		Write-Host "Failed to obtain a valid client certificate."
		exit 1
	}
}
elseif (-not $haveCert) {
	Write-Host "I don't have a certificate, requesting one now..."
	$hostname = [System.Web.HttpUtility]::UrlEncode([System.Net.Dns]::GetHostByName(($env:computerName)).Hostname)
	$url = $serverbaseurl + "reqcert?hostname=$hostname"
	$r = $null
	$ok = $false
	try {
		$r = http $url "get" 60
		$ok = ParseAndSaveCertificateFromResult $r
	} catch {
		Write-Host $error[0]
		Write-Host $Error[0].Exception.InnerException
	}
	if (-not $ok) {
		Write-Host "Failed to obtain a valid client certificate."
		exit 1
	}
}

# Load the certificate
if (-not (CanAccessPath $certpath)) {
	Write-Host "Unable to read $certpath"
	exit 1
}
$flags = [System.Security.Cryptography.X509Certificates.X509KeyStorageFlags]"MachineKeySet,Exportable"
$cert = $null
try {
	$cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($certpath, "", $flags)
} catch {
	Write-Host $error[0]
}
if (IsNull $cert) {
	Write-Host "Unable to load $certpath"
	exit 1
}
if (-not $cert.Issuer.Contains("Nivlheim")) {
	Write-Host "The client certificate isn't issued by Nivlheim."
	exit 1
}

#================ End of certificate processing =========

# Create a temporary directory structure for storing files.
$tmpdir = $env:TEMP + "/nivlheim_tmp"
try {
	$r = Remove-Item -Path $tmpdir -Recurse -Force -ErrorAction:SilentlyContinue
} catch {}
$r = New-Item -Path $tmpdir -ItemType directory -Force
$r = New-Item -Path "$tmpdir/files" -ItemType directory -Force
$r = New-Item -Path "$tmpdir/commands" -ItemType directory -Force

# Collect files
if ($conf.ContainsKey("files")) {
	if ($testmode) {
		Write-Host "`nProcessing [files]"
	}
	foreach ($filename in $conf["files"].Keys) {
		if ($testmode) {
			Write-Host $filename
		}
		$withoutDrive = Split-Path -Path $filename -NoQualifier
		$parent = Split-Path -Path $withoutDrive -Parent
		if (-not (CanAccessPath $filename)) {
			if ($testmode) { Write-Host "Not found: $filename" }
			continue
		}
		try {
			if (-Not (Test-Path "$tmpdir/files$parent")) {
				$r = New-Item "$tmpdir/files$parent" -ItemType directory
			}
			Copy-Item -Path $filename -Destination "$tmpdir/files$withoutDrive"
		}
		catch {
			Write-Host $error[0]
		}
	}
}

# Run all the commands, and save the output to files
$enc = [System.Text.Encoding]::UTF8
if ($conf.ContainsKey("commands")) {
	if ($testmode) {
		Write-Host "`nProcessing [commands]"
	}
	foreach ($cmd in $conf["commands"].Keys) {
		if ($testmode) {
			Write-Host $cmd
		}
		$short = shortencmd($cmd)
		$filename = "$tmpdir/commands/" + $short
		echo $cmd > $filename
		try {
			Invoke-Expression -Command "$cmd" >> $filename 2>&1
		}
		catch {
			Write-Output $error[0] | Add-Content $filename
		}
	}
}

# Run all the aliased commands
if ($conf.ContainsKey("commandalias")) {
	if ($testmode) {
		Write-Host "`nProcessing [commandalias]"
	}
	foreach ($alias in $conf["commandalias"].Keys) {
		if ($testmode) {
			Write-Host $alias
		}
		$cmd = $conf["commandalias"][$alias]
		$filename = "$tmpdir/commands/" + $alias
		echo $alias > $filename
		try {
			Invoke-Expression -Command "$cmd" >> $filename 2>&1
		}
		catch {
			Write-Output $error[0] | Add-Content $filename
		}
	}
}
if ($testmode) {
	Write-Host ""
}

# Create a zip file
$zipname = "$env:TEMP/nivlheim-archive.zip"
try {
	Remove-Item -Path $zipname -Force -ErrorAction:SilentlyContinue
} catch {}
$r = zipfolder $tmpdir $zipname

# create a signature for the zip file
$bytes = [IO.File]::ReadAllBytes($zipname)
$oid = [System.Security.Cryptography.CryptoConfig]::MapNameToOID("SHA1")
$sha = New-Object System.Security.Cryptography.SHA1CryptoServiceProvider
$hash = $sha.ComputeHash($bytes)
# PrivateKey supports only RSA or DSA keys, so it returns either an RSA or a DSA object in .NET Core or an RSACryptoServiceProvider or a DSACryptoServiceProvider object in .NET Framework.
$cryptoServiceProvider = $cert.PrivateKey
try {
	$sig = $cryptoServiceProvider.SignHash($hash, $oid)
} catch {
	# Works in .NET Core
	$pk1 = New-Object System.Security.Cryptography.RSACryptoServiceProvider
 	$pk1.ImportParameters($cryptoServiceProvider.ExportParameters($True))
	$sig = $pk1.SignHash($hash, $oid)
}
#[IO.File]::WriteAllBytes($dirpath + "\signature", $sig)

# read nonce if it exists
try {
	$p = Split-Path -Parent $certpath
	$nonce = [IO.File]::ReadAllLines($p + "/nonce.txt")[0]
} catch {
	$nonce = 0
}

# http post the stuff
try {
	$url = $serverbaseurl + "secure/post"
	$params = @{
		"hostname" = [System.Net.Dns]::GetHostByName(($env:computerName)).Hostname;
		"version" = $version;
		"signature_base64" = [System.Convert]::ToBase64String($sig);
		"archive_base64" = [System.Convert]::ToBase64String([IO.File]::ReadAllBytes($zipname));
		"nonce" = $nonce;
	}
	$r = http $url "post" 60 $cert $params
	Write-Output "Response from server:" $r
	if ( [string]($r) -match 'nonce=(\d+)' ) {
		$p = Split-Path -Parent $certpath
		[IO.File]::WriteAllText($p + "/nonce.txt", $matches[1])
	}
}
catch {
	Write-Output $error[0]
	exit 1
}

# Cleanup. Remove the zip file and temporary directory
$r = Remove-Item -path $tmpdir -recurse -force
$r = Remove-Item -path $zipname -force

}
catch {
	Write-Output $error[0]
	exit 1
}
finally {
	if (-Not $testmode) {
		Stop-Transcript
	}
}
