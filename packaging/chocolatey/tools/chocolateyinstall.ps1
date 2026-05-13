$toolsDir = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"
$url = "https://github.com/snapetech/iptvtunerr/releases/download/v0.1.63/iptv-tunerr-v0.1.63-windows-amd64.zip"
$checksum = "SKIP"

Install-ChocolateyZipPackage -PackageName 'iptvtunerr' -Url $url -UnzipLocation $toolsDir -Checksum $checksum -ChecksumType 'sha256'

$exe = Join-Path $toolsDir "iptv-tunerr-v0.1.63-windows-amd64.exe"
if (Test-Path $exe) {
    Install-BinFile -Name "iptv-tunerr" -Path $exe
}
