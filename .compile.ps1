Set-Location -LiteralPath $PSScriptRoot

$env:CGO_ENABLED = "1"
$env:CC = "C:\msys64\ucrt64\bin\gcc.exe"
$env:PATH = "C:\msys64\ucrt64\bin;" + $env:PATH

go build -trimpath -buildvcs=false -ldflags "-H=windowsgui -s -w" -o StayInCalc.exe .
