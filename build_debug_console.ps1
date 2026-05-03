# Build Windows console binaries for local debugging.
New-Item -ItemType Directory -Force -Path bin | Out-Null

go build -o bin/RaysAndMagic_debug.exe .
go build -o bin/RaysAndMagicMapViewer_debug.exe ./assets/map_viewer

Write-Host "Built bin/RaysAndMagic_debug.exe with debug console."
Write-Host "Built bin/RaysAndMagicMapViewer_debug.exe with debug console."
