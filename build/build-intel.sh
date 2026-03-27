#!/bin/bash

SUPABASE_URL="https://vydawdpzfpmwqmvymwsi.supabase.co"
SUPABASE_KEY="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6InZ5ZGF3ZHB6ZnBtd3Ftdnltd3NpIiwicm9sZSI6ImFub24iLCJpYXQiOjE3NzQ0MjYzNDYsImV4cCI6MjA5MDAwMjM0Nn0.WEE-sHB5woplMM3URHIs3cX0mUV_MdvETsU-_v40XQs"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "[CHECK] SUPABASE_URL  = $SUPABASE_URL"
echo "[CHECK] SUPABASE_KEY  = ${SUPABASE_KEY:0:30}..."

if ! command -v go &> /dev/null; then
    echo "[ERROR] Go is not installed. Download: https://go.dev/dl/"
    exit 1
fi

echo "[1/3] Go version:"
go version

echo ""
echo "[2/3] go mod tidy..."
go mod tidy -C "$ROOT" || { echo "[ERROR] go mod tidy failed"; exit 1; }

echo ""
echo "[3/3] Building for Intel Mac (darwin/amd64)..."
LDFLAGS="-s -w -X main.supabaseURL=$SUPABASE_URL -X main.supabaseAnonKey=$SUPABASE_KEY"
GOOS=darwin GOARCH=amd64 go build -C "$ROOT" -ldflags "$LDFLAGS" -o "$ROOT/gg-tracker-intel" . || { echo "[ERROR] Build failed"; exit 1; }

echo ""
echo "Build successful! gg-tracker-intel is ready."

echo ""
echo "[VERIFY] Checking embedded values in binary..."
if strings "$ROOT/gg-tracker-intel" | grep -q "vydawdpzfpmwqmvymwsi"; then
    echo "[OK] SUPABASE_URL is embedded"
else
    echo "[FAIL] SUPABASE_URL NOT found in binary!"
fi
if strings "$ROOT/gg-tracker-intel" | grep -q "${SUPABASE_KEY:0:30}"; then
    echo "[OK] SUPABASE_KEY is embedded"
else
    echo "[FAIL] SUPABASE_KEY NOT found in binary!"
fi
