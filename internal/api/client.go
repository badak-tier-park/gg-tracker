package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"gg-tracker/internal/store"
)

type Client struct {
	supabaseURL string
	anonKey     string
	http        *http.Client
}

func New(supabaseURL, anonKey string) *Client {
	return &Client{
		supabaseURL: supabaseURL,
		anonKey:     anonKey,
		http:        &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) InsertGame(g *store.Game) error {
	body, _ := json.Marshal(map[string]any{
		"played_at":             g.PlayedAt.Format(time.RFC3339),
		"map_name":              g.MapName,
		"game_duration_seconds": g.GameDurationSeconds,
		"winner_name":           g.WinnerName,
		"winner_race":           g.WinnerRace,
		"loser_name":            g.LoserName,
		"loser_race":            g.LoserRace,
		"winner_apm":            g.WinnerAPM,
		"loser_apm":             g.LoserAPM,
		"replay_file":           g.ReplayFile,
	})

	req, err := http.NewRequest("POST", c.supabaseURL+"/functions/v1/record-game", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.anonKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Edge Function 오류: %s", resp.Status)
	}
	return nil
}

func (c *Client) getGames(limit int, replayFile string) ([]*store.Game, error) {
	endpoint := fmt.Sprintf("%s/functions/v1/get-games?limit=%d", c.supabaseURL, limit)
	if replayFile != "" {
		endpoint += "&replay_file=" + url.QueryEscape(replayFile)
	}

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.anonKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get-games 오류: %s | %s", resp.Status, string(body))
	}

	var rows []struct {
		ID                  int64  `json:"id"`
		PlayedAt            string `json:"played_at"`
		MapName             string `json:"map_name"`
		GameDurationSeconds int    `json:"game_duration_seconds"`
		WinnerName          string `json:"winner_name"`
		WinnerRace          string `json:"winner_race"`
		LoserName           string `json:"loser_name"`
		LoserRace           string `json:"loser_race"`
		WinnerAPM           int    `json:"winner_apm"`
		LoserAPM            int    `json:"loser_apm"`
		ReplayFile          string `json:"replay_file"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, err
	}

	games := make([]*store.Game, 0, len(rows))
	for _, r := range rows {
		t, _ := time.Parse(time.RFC3339, r.PlayedAt)
		games = append(games, &store.Game{
			ID:                  r.ID,
			PlayedAt:            t,
			MapName:             r.MapName,
			GameDurationSeconds: r.GameDurationSeconds,
			WinnerName:          r.WinnerName,
			WinnerRace:          r.WinnerRace,
			LoserName:           r.LoserName,
			LoserRace:           r.LoserRace,
			WinnerAPM:           r.WinnerAPM,
			LoserAPM:            r.LoserAPM,
			ReplayFile:          r.ReplayFile,
		})
	}
	return games, nil
}

func (c *Client) ListGames(limit int) ([]*store.Game, error) {
	return c.getGames(limit, "")
}

func (c *Client) GetStats() (wins map[string]int, losses map[string]int, total int, err error) {
	games, err := c.ListGames(10000)
	if err != nil {
		return nil, nil, 0, err
	}
	wins = make(map[string]int)
	losses = make(map[string]int)
	for _, g := range games {
		wins[g.WinnerName]++
		losses[g.LoserName]++
	}
	return wins, losses, len(games), nil
}

func (c *Client) IsAlreadyProcessed(replayFile string) bool {
	games, err := c.getGames(1, replayFile)
	if err != nil {
		return false
	}
	return len(games) > 0
}

func (c *Client) Close() {}
