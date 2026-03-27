package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"sync"

	"gg-tracker/internal/config"
	"gg-tracker/internal/store"
)

//go:embed templates/*
var templateFS embed.FS

var (
	dashboardTmpl = template.Must(template.ParseFS(templateFS, "templates/dashboard.html"))
	setupTmpl     = template.Must(template.ParseFS(templateFS, "templates/setup.html"))
)

type Server struct {
	mu        sync.RWMutex
	database  store.Store
	cfg       *config.Config
	cfgPath   string
	restartCh chan<- struct{}
}

func NewServer(database store.Store, cfg *config.Config, cfgPath string, restartCh chan<- struct{}) *Server {
	return &Server{
		database:  database,
		cfg:       cfg,
		cfgPath:   cfgPath,
		restartCh: restartCh,
	}
}

func (s *Server) Start(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/setup", s.handleSetup)
	mux.HandleFunc("/api/games", s.handleAPIGames)
	mux.HandleFunc("/api/stats", s.handleAPIStats)

	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Printf("[오류] 웹 서버 시작 실패: %v\n", err)
	}
}

// ── API handlers ──────────────────────────────────────────────

func (s *Server) handleAPIGames(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	database := s.database
	s.mu.RUnlock()

	if database == nil {
		http.Error(w, "DB가 연결되지 않았습니다", http.StatusServiceUnavailable)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 200
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
	}

	games, err := database.ListGames(limit)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	type gameJSON struct {
		ID         int64  `json:"id"`
		PlayedAt   string `json:"played_at"`
		MapName    string `json:"map_name"`
		Duration   string `json:"duration"`
		WinnerName string `json:"winner_name"`
		WinnerRace string `json:"winner_race"`
		LoserName  string `json:"loser_name"`
		LoserRace  string `json:"loser_race"`
		WinnerAPM  int    `json:"winner_apm"`
		LoserAPM   int    `json:"loser_apm"`
	}

	result := make([]gameJSON, 0, len(games))
	for _, g := range games {
		result = append(result, gameJSON{
			ID:         g.ID,
			PlayedAt:   g.PlayedAt.Format("2006-01-02 15:04"),
			MapName:    g.MapName,
			Duration:   formatDuration(g.GameDurationSeconds),
			WinnerName: g.WinnerName,
			WinnerRace: g.WinnerRace,
			LoserName:  g.LoserName,
			LoserRace:  g.LoserRace,
			WinnerAPM:  g.WinnerAPM,
			LoserAPM:   g.LoserAPM,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleAPIStats(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	database := s.database
	s.mu.RUnlock()

	if database == nil {
		http.Error(w, "DB가 연결되지 않았습니다", http.StatusServiceUnavailable)
		return
	}

	wins, losses, total, err := database.GetStats()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	type playerStat struct {
		Name   string `json:"name"`
		Wins   int    `json:"wins"`
		Losses int    `json:"losses"`
		Total  int    `json:"total"`
	}

	allNames := make(map[string]struct{})
	for name := range wins {
		allNames[name] = struct{}{}
	}
	for name := range losses {
		allNames[name] = struct{}{}
	}

	stats := make([]playerStat, 0, len(allNames))
	for name := range allNames {
		stats = append(stats, playerStat{
			Name:   name,
			Wins:   wins[name],
			Losses: losses[name],
			Total:  wins[name] + losses[name],
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"total":   total,
		"players": stats,
	})
}

// ── Page handlers ─────────────────────────────────────────────

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		replayPath := r.FormValue("replay_path")
		if replayPath == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			setupTmpl.Execute(w, map[string]string{
				"Error":      "리플레이 경로를 입력해주세요.",
				"ReplayPath": s.cfg.ReplayPath,
			})
			return
		}

		s.cfg.ReplayPath = replayPath
		s.cfg.SetupComplete = true
		if err := s.cfg.Save(); err != nil {
			http.Error(w, "설정 저장 실패: "+err.Error(), 500)
			return
		}

		select {
		case s.restartCh <- struct{}{}:
		default:
		}

		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	setupTmpl.Execute(w, map[string]string{
		"Error":      "",
		"ReplayPath": s.cfg.ReplayPath,
	})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.SetupComplete {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	dashboardTmpl.Execute(w, nil)
}

// ── Helpers ───────────────────────────────────────────────────

func formatDuration(seconds int) string {
	m := seconds / 60
	s := seconds % 60
	return fmt.Sprintf("%d:%02d", m, s)
}
