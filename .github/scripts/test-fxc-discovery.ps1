$ErrorActionPreference = 'Stop'
$cases = @(
  @{ Name = 'versioned'; Paths = @('10.0.9.0/x64/fxc.exe', '10.0.10.0/x64/fxc.exe'); Want = '10.0.10.0/x64/fxc.exe' },
  @{ Name = 'legacy'; Paths = @('x64/fxc.exe'); Want = 'x64/fxc.exe' },
  @{ Name = 'mixed'; Paths = @('x64/fxc.exe', '10.0.10.0/x64/fxc.exe', '10.0.99.0/x86/fxc.exe'); Want = '10.0.10.0/x64/fxc.exe' },
  @{ Name = 'missing'; Paths = @('10.0.99.0/x86/fxc.exe'); Want = $null }
)
foreach ($case in $cases) {
  $root = Join-Path ([IO.Path]::GetTempPath()) ([guid]::NewGuid().ToString())
  try {
    foreach ($relative in $case.Paths) {
      $path = Join-Path $root $relative
      New-Item -ItemType Directory -Path (Split-Path $path) -Force | Out-Null
      New-Item -ItemType File -Path $path | Out-Null
    }
    $found = $null
    $failure = $null
    try { $found = & (Join-Path $PSScriptRoot 'find-fxc.ps1') -SdkBin $root }
    catch { $failure = $_ }
    if ($null -eq $case.Want) {
      if ($null -eq $failure -or $failure.ToString() -notlike '*fxc.exe is required*') {
        throw "$($case.Name): missing compiler did not produce the expected error"
      }
    } elseif ($failure -or $found.FullName -ne (Join-Path $root $case.Want)) {
      throw "$($case.Name): wrong compiler or discovery failed: $failure"
    }
    Write-Output "$($case.Name): passed"
  } finally {
    Remove-Item $root -Recurse -Force -ErrorAction SilentlyContinue
  }
}
