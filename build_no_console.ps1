# Build Windows GUI binaries without console windows.
New-Item -ItemType Directory -Force -Path bin | Out-Null

go build -ldflags="-H=windowsgui" -o bin/RaysAndMagic.exe .
go build -ldflags="-H=windowsgui" -o bin/RaysAndMagicMapViewer.exe ./assets/map_viewer

Write-Host "Built bin/RaysAndMagic.exe without console window."
Write-Host "Built bin/RaysAndMagicMapViewer.exe without console window."
