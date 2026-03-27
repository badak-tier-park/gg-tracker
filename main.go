package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/joho/godotenv"

	"gg-tracker/internal/api"
	"gg-tracker/internal/config"
	"gg-tracker/internal/db"
	"gg-tracker/internal/store"
	"gg-tracker/internal/watcher"
	"gg-tracker/internal/web"
)

// 빌드 시 ldflags로 주입: -X main.supabaseURL=... -X main.supabaseAnonKey=...
var (
	supabaseURL     string
	supabaseAnonKey string
)

func main() {
	printBanner()

	exeDir, err := getExeDir()
	if err != nil {
		log.Fatalf("실행 파일 경로 오류: %v", err)
	}

	// .env 파일 로드 (없어도 무시) — 개발 환경용 fallback
	// go run 시 현재 디렉토리, 빌드된 exe 실행 시 exe 위치 순으로 탐색
	godotenv.Load(".env")
	godotenv.Load(filepath.Join(exeDir, ".env"))

	// ldflags 미주입 시 환경변수 fallback
	if supabaseURL == "" {
		supabaseURL = os.Getenv("SUPABASE_URL")
	}
	if supabaseAnonKey == "" {
		supabaseAnonKey = os.Getenv("SUPABASE_ANON_KEY")
	}

	cfgPath := filepath.Join(exeDir, "config.json")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("설정 로드 실패: %v", err)
	}

	// 설정 완료 신호 채널 (web server → main)
	restartCh := make(chan struct{}, 1)

	// Supabase 키가 있으면 운영(Edge Function), 없으면 로컬 DB
	var st store.Store
	if supabaseURL != "" && supabaseAnonKey != "" {
		st = api.New(supabaseURL, supabaseAnonKey)
	} else {
		dbConnStr := buildDBConnStr()
		if dbConnStr == "" {
			log.Fatal("로컬 실행 시 .env 파일에 DB_USER, DB_PASSWORD, DB_HOST, DB_PORT, DB_NAME 을 설정해주세요.")
		}
		localDB, err := db.New(dbConnStr)
		if err != nil {
			log.Fatalf("DB 연결 실패: %v", err)
		}
		defer localDB.Close()
		st = localDB
	}

	var w *watcher.Watcher

	server := web.NewServer(st, cfg, cfgPath, restartCh)
	go server.Start(":8080")

	time.Sleep(300 * time.Millisecond)

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  대시보드: http://localhost:8080")

	if !cfg.SetupComplete {
		fmt.Println("  상태: 초기 설정이 필요합니다")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()
		fmt.Println("브라우저에서 리플레이 경로를 설정해주세요.")
		openBrowser("http://localhost:8080/setup")
	} else {
		fmt.Printf("  리플레이 경로: %s\n", cfg.ReplayPath)
		fmt.Println("  상태: 감시 중 ●")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()
		w = startWatcher(cfg.ReplayPath, st)
		openBrowser("http://localhost:8080")
	}

	fmt.Println("이 창을 닫으면 프로그램이 종료됩니다.")
	fmt.Println()

	// 설정 변경 신호 대기 (리플레이 경로 변경 시)
	for range restartCh {
		if w != nil {
			w.Stop()
		}
		w = startWatcher(cfg.ReplayPath, st)
	}
}

func buildDBConnStr() string {
	// DB_CONN_STRING 우선, 없으면 개별 항목 조합
	if v := os.Getenv("DB_CONN_STRING"); v != "" {
		return v
	}
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	name := os.Getenv("DB_NAME")
	if user == "" || host == "" || name == "" {
		return ""
	}
	if port == "" {
		port = "5432"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s", user, password, host, port, name)
}

func startWatcher(path string, database store.Store) *watcher.Watcher {
	w := watcher.New(path, database)
	go w.Watch()
	fmt.Printf("[%s] 감시 시작: %s\n", timestamp(), path)
	return w
}

func printBanner() {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║  GG Tracker - SC1 Remastered 전적 자동 기록기  ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()
}

func getExeDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return filepath.Dir(os.Args[0]), nil
	}
	return filepath.Dir(exe), nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}

func timestamp() string {
	return time.Now().Format("15:04:05")
}
