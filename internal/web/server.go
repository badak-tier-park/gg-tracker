package web

import (
	"crypto/rand"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
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

const discordRedirectURI = "http://localhost:8080/auth/discord/callback"

type Server struct {
	mu                  sync.RWMutex
	database            store.Store
	cfg                 *config.Config
	cfgPath             string
	restartCh           chan<- struct{}
	discordClientID     string
	discordClientSecret string
	// 단일 사용자 세션 (in-memory)
	discordID       int64
	discordUsername string
	pendingState    string
}

func NewServer(database store.Store, cfg *config.Config, cfgPath string, restartCh chan<- struct{}, discordClientID, discordClientSecret string) *Server {
	return &Server{
		database:            database,
		cfg:                 cfg,
		cfgPath:             cfgPath,
		restartCh:           restartCh,
		discordClientID:     discordClientID,
		discordClientSecret: discordClientSecret,
	}
}

// GetDiscordID returns the currently logged-in Discord user's ID (0 if not logged in).
// 스레드 안전 — watcher에서 func() int64 클로저로 호출됨.
func (s *Server) GetDiscordID() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.discordID
}

func (s *Server) Start(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/setup", s.handleSetup)
	mux.HandleFunc("/api/games", s.handleAPIGames)
	mux.HandleFunc("/api/stats", s.handleAPIStats)
	mux.HandleFunc("/auth/discord", s.handleDiscordLogin)
	mux.HandleFunc("/auth/discord/callback", s.handleDiscordCallback)
	mux.HandleFunc("/auth/discord/logout", s.handleDiscordLogout)

	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Printf("[오류] 웹 서버 시작 실패: %v\n", err)
	}
}

// ── Discord OAuth ──────────────────────────────────────────────

func (s *Server) handleDiscordLogin(w http.ResponseWriter, r *http.Request) {
	if s.discordClientID == "" {
		http.Error(w, "Discord 클라이언트 ID가 설정되지 않았습니다. .env에 DISCORD_CLIENT_ID를 추가해주세요.", http.StatusServiceUnavailable)
		return
	}

	b := make([]byte, 16)
	rand.Read(b)
	state := fmt.Sprintf("%x", b)

	s.mu.Lock()
	s.pendingState = state
	s.mu.Unlock()

	authURL := "https://discord.com/oauth2/authorize?" +
		"client_id=" + url.QueryEscape(s.discordClientID) +
		"&redirect_uri=" + url.QueryEscape(discordRedirectURI) +
		"&response_type=code" +
		"&scope=identify" +
		"&state=" + state

	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) handleDiscordCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	s.mu.Lock()
	expected := s.pendingState
	s.pendingState = ""
	s.mu.Unlock()

	if state == "" || state != expected {
		http.Error(w, "잘못된 요청입니다 (state 불일치)", http.StatusBadRequest)
		return
	}

	token, err := s.discordExchangeCode(code)
	if err != nil {
		http.Error(w, "Discord 인증 실패: "+err.Error(), http.StatusInternalServerError)
		return
	}

	discordID, username, err := s.discordGetUser(token)
	if err != nil {
		http.Error(w, "사용자 정보 조회 실패: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	s.discordID = discordID
	s.discordUsername = username
	s.mu.Unlock()

	fmt.Printf("[Discord] 로그인: %s (%d)\n", username, discordID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleDiscordLogout(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.discordID = 0
	s.discordUsername = ""
	s.mu.Unlock()
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) discordExchangeCode(code string) (string, error) {
	vals := url.Values{
		"client_id":     {s.discordClientID},
		"client_secret": {s.discordClientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {discordRedirectURI},
	}
	resp, err := http.PostForm("https://discord.com/api/oauth2/token", vals)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", fmt.Errorf("%s", result.Error)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("액세스 토큰 없음")
	}
	return result.AccessToken, nil
}

func (s *Server) discordGetUser(token string) (int64, string, error) {
	req, _ := http.NewRequest("GET", "https://discord.com/api/users/@me", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	var user struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return 0, "", err
	}

	id, err := strconv.ParseInt(user.ID, 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("Discord ID 파싱 실패: %v", err)
	}
	return id, user.Username, nil
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

	s.mu.RLock()
	username := s.discordUsername
	discordID := s.discordID
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	dashboardTmpl.Execute(w, map[string]any{
		"DiscordUsername": username,
		"DiscordID":       discordID,
		"HasDiscordApp":   s.discordClientID != "",
	})
}

// ── Helpers ───────────────────────────────────────────────────

func formatDuration(seconds int) string {
	m := seconds / 60
	s := seconds % 60
	return fmt.Sprintf("%d:%02d", m, s)
}
