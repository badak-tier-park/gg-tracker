$supabaseUrl = "https://vydawdpzfpmwqmvymwsi.supabase.co"
$supabaseKey = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6InZ5ZGF3ZHB6ZnBtd3Ftdnltd3NpIiwicm9sZSI6ImFub24iLCJpYXQiOjE3NzQ0MjYzNDYsImV4cCI6MjA5MDAwMjM0Nn0.WEE-sHB5woplMM3URHIs3cX0mUV_MdvETsU-_v40XQs"

Write-Host "[CHECK] SUPABASE_URL  = $supabaseUrl"
Write-Host "[CHECK] SUPABASE_KEY  = $($supabaseKey.Substring(0, 30))..."

if (!(Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host "[ERROR] Go is not installed. Download: https://go.dev/dl/"
    exit 1
}

Write-Host "[1/3] Go version:"
go version

Write-Host ""
Write-Host "[2/3] go mod tidy..."
go mod tidy
if ($LASTEXITCODE -ne 0) { Write-Host "[ERROR] go mod tidy failed"; exit 1 }

Write-Host ""
Write-Host "[3/3] Building..."
$ldflags = "-s -w -X main.supabaseURL=$supabaseUrl -X main.supabaseAnonKey=$supabaseKey"
go build -ldflags $ldflags -o gg-tracker.exe .
if ($LASTEXITCODE -ne 0) { Write-Host "[ERROR] Build failed"; exit 1 }

Write-Host ""
Write-Host "Build successful! gg-tracker.exe is ready."

Write-Host ""
Write-Host "[VERIFY] Checking embedded values in exe..."
$urlFound = Select-String -Path "gg-tracker.exe" -Pattern ([regex]::Escape("vydawdpzfpmwqmvymwsi")) -Quiet
$keyFound = Select-String -Path "gg-tracker.exe" -Pattern ([regex]::Escape($supabaseKey.Substring(0, 30))) -Quiet
Write-Host $(if ($urlFound) { "[OK] SUPABASE_URL is embedded" } else { "[FAIL] SUPABASE_URL NOT found in exe!" })
Write-Host $(if ($keyFound) { "[OK] SUPABASE_KEY is embedded" } else { "[FAIL] SUPABASE_KEY NOT found in exe!" })
