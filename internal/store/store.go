package store

import "time"

type Game struct {
	ID                  int64
	DiscordID           int64
	PlayedAt            time.Time
	MapName             string
	GameDurationSeconds int
	WinnerName          string
	WinnerRace          string
	LoserName           string
	LoserRace           string
	WinnerAPM           int
	LoserAPM            int
	ReplayFile          string
}

type Store interface {
	InsertGame(g *Game) error
	ListGames(limit int) ([]*Game, error)
	GetStats() (wins map[string]int, losses map[string]int, total int, err error)
	IsAlreadyProcessed(replayFile string) bool
	Close()
}
