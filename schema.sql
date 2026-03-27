CREATE TABLE IF NOT EXISTS games (
    id                    BIGSERIAL PRIMARY KEY,
    played_at             TIMESTAMPTZ NOT NULL,
    map_name              TEXT NOT NULL,
    game_duration_seconds INTEGER NOT NULL,
    winner_name           TEXT NOT NULL,
    winner_race           TEXT NOT NULL,
    loser_name            TEXT NOT NULL,
    loser_race            TEXT NOT NULL,
    winner_apm            INTEGER NOT NULL DEFAULT 0,
    loser_apm             INTEGER NOT NULL DEFAULT 0,
    replay_file           TEXT NOT NULL UNIQUE
);
