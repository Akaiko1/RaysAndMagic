param([Parameter(Mandatory = $true)][string]$SdkBin)

# Versioned SDKs rank above the legacy bin/x64 layout. A non-version parent
# still contains a usable compiler; it must not throw during sorting.
$fxc = Get-ChildItem $SdkBin -Filter fxc.exe -Recurse -File -ErrorAction Stop |
  Where-Object { $_.Directory.Name -eq 'x64' } |
  Sort-Object {
    $sdkVersion = $null
    if ([version]::TryParse($_.Directory.Parent.Name, [ref]$sdkVersion)) {
      $sdkVersion
    } else {
      [version]'0.0'
    }
  } -Descending |
  Select-Object -First 1
if (-not $fxc) { throw 'Windows SDK fxc.exe is required for release shaders' }
$fxc
