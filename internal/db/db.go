package db

import (
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"

	"gg-tracker/internal/store"
)

type DB struct {
	conn *sql.DB
}

func New(connStr string) (*DB, error) {
	conn, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, err
	}
	return &DB{conn: conn}, nil
}

func (d *DB) InsertGame(g *store.Game) error {
	_, err := d.conn.Exec(`
		INSERT INTO games
			(discord_id, played_at, map_name, game_duration_seconds, winner_name, winner_race,
			 loser_name, loser_race, winner_apm, loser_apm, replay_file)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (replay_file) DO NOTHING`,
		g.DiscordID, g.PlayedAt, g.MapName, g.GameDurationSeconds,
		g.WinnerName, g.WinnerRace,
		g.LoserName, g.LoserRace,
		g.WinnerAPM, g.LoserAPM,
		g.ReplayFile,
	)
	return err
}

func (d *DB) ListGames(limit int) ([]*store.Game, error) {
	rows, err := d.conn.Query(`
		SELECT id, played_at, map_name, game_duration_seconds,
		       winner_name, winner_race, loser_name, loser_race,
		       winner_apm, loser_apm, replay_file
		FROM games
		ORDER BY played_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []*store.Game
	for rows.Next() {
		g := &store.Game{}
		if err := rows.Scan(
			&g.ID, &g.PlayedAt, &g.MapName, &g.GameDurationSeconds,
			&g.WinnerName, &g.WinnerRace, &g.LoserName, &g.LoserRace,
			&g.WinnerAPM, &g.LoserAPM, &g.ReplayFile,
		); err != nil {
			return nil, err
		}
		games = append(games, g)
	}
	return games, rows.Err()
}

func (d *DB) GetStats() (wins map[string]int, losses map[string]int, total int, err error) {
	wins = make(map[string]int)
	losses = make(map[string]int)

	d.conn.QueryRow(`SELECT COUNT(*) FROM games`).Scan(&total)

	rows, err := d.conn.Query(`SELECT winner_name, COUNT(*) FROM games GROUP BY winner_name`)
	if err != nil {
		return nil, nil, 0, err
	}
	for rows.Next() {
		var name string
		var count int
		rows.Scan(&name, &count)
		wins[name] = count
	}
	rows.Close()

	rows, err = d.conn.Query(`SELECT loser_name, COUNT(*) FROM games GROUP BY loser_name`)
	if err != nil {
		return nil, nil, 0, err
	}
	for rows.Next() {
		var name string
		var count int
		rows.Scan(&name, &count)
		losses[name] = count
	}
	rows.Close()

	return wins, losses, total, nil
}

func (d *DB) IsAlreadyProcessed(replayFile string) bool {
	var count int
	d.conn.QueryRow(`SELECT COUNT(*) FROM games WHERE replay_file = $1`, replayFile).Scan(&count)
	return count > 0
}

func (d *DB) Close() {
	if d != nil && d.conn != nil {
		d.conn.Close()
	}
}

