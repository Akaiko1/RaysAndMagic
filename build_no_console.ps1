# Build without console window (Windows GUI app)
go build -ldflags="-H=windowsgui" -o RaysAndMagic.exe
Write-Host "Built RaysAndMagic.exe without console window."
