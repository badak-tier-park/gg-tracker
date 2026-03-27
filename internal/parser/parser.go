package parser

import (
	"fmt"
	"unicode/utf8"

	"github.com/icza/screp/rep"
	"github.com/icza/screp/rep/repcore"
	"github.com/icza/screp/repparser"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"

	"gg-tracker/internal/store"
)

// ParseReplay parses a .rep file and returns structured game data.
func ParseReplay(filePath string) (*store.Game, error) {
	r, err := repparser.ParseFileConfig(filePath, repparser.Config{
		Commands: true,
		MapData:  false,
	})
	if err != nil {
		return nil, fmt.Errorf("파싱 실패: %w", err)
	}
	if r == nil || r.Header == nil {
		return nil, fmt.Errorf("리플레이 헤더를 읽을 수 없습니다")
	}
	r.Compute()

	// 인간 플레이어만 수집
	var players []*rep.Player
	for _, p := range r.Header.Players {
		if p != nil && p.Type == repcore.PlayerTypeHuman {
			players = append(players, p)
		}
	}
	if len(players) < 2 {
		return nil, fmt.Errorf("인식 가능한 플레이어가 2명 미만입니다 (감지: %d명)", len(players))
	}

	winner, loser, err := determineWinnerLoser(r, players)
	if err != nil {
		return nil, err
	}

	mapName := cleanMapName(r.Header.Map)
	if mapName == "" {
		mapName = cleanMapName(r.Header.Title)
	}
	return &store.Game{
		PlayedAt:            r.Header.StartTime,
		MapName:             mapName,
		GameDurationSeconds: framesToSeconds(int32(r.Header.Frames), r.Header.Speed.Name),
		WinnerName:          winner.Name,
		WinnerRace:          raceString(winner.Race),
		LoserName:           loser.Name,
		LoserRace:           raceString(loser.Race),
		WinnerAPM:           getAPM(r, winner),
		LoserAPM:            getAPM(r, loser),
		ReplayFile:          filePath,
	}, nil
}

// determineWinnerLoser uses WinnerTeam if available, otherwise uses
// leave-game command ordering: earlier leaver = loser.
func determineWinnerLoser(r *rep.Replay, players []*rep.Player) (*rep.Player, *rep.Player, error) {
	// 1순위: screp이 계산한 WinnerTeam 사용
	if r.Computed != nil && r.Computed.WinnerTeam > 0 {
		var winner, loser *rep.Player
		for _, p := range players {
			if p.Team == r.Computed.WinnerTeam {
				winner = p
			} else {
				loser = p
			}
		}
		if winner != nil && loser != nil {
			return winner, loser, nil
		}
	}

	if r.Computed == nil {
		return nil, nil, fmt.Errorf("computed 데이터 없음: 승자 결정 불가")
	}

	// Leave 커맨드를 PlayerID로 매핑
	leaveFrame := make(map[byte]repcore.Frame)
	for _, cmd := range r.Computed.LeaveGameCmds {
		if cmd == nil {
			continue
		}
		base := cmd.BaseCmd()
		leaveFrame[base.PlayerID] = base.Frame
	}

	// 플레이어별 leave 여부 분류
	var withLeave, withoutLeave []*rep.Player
	for _, p := range players {
		if _, ok := leaveFrame[p.ID]; ok {
			withLeave = append(withLeave, p)
		} else {
			withoutLeave = append(withoutLeave, p)
		}
	}

	switch {
	case len(withoutLeave) == 1 && len(withLeave) >= 1:
		// Leave 커맨드 없는 플레이어 = 승자 (상대가 먼저 나감)
		return withoutLeave[0], withLeave[0], nil

	case len(withLeave) >= 2:
		// 둘 다 leave: 가장 먼저 나간 사람이 패자
		loser := withLeave[0]
		for _, p := range withLeave[1:] {
			if leaveFrame[p.ID] < leaveFrame[loser.ID] {
				loser = p
			}
		}
		var winner *rep.Player
		for _, p := range players {
			if p != loser {
				winner = p
				break
			}
		}
		if winner == nil {
			return nil, nil, fmt.Errorf("승자를 특정할 수 없습니다")
		}
		return winner, loser, nil

	default:
		return nil, nil, fmt.Errorf("게임 종료 정보 부족으로 승자 결정 불가")
	}
}

func getAPM(r *rep.Replay, p *rep.Player) int {
	if r.Computed == nil {
		return 0
	}
	// PIDPlayerDescs 맵으로 O(1) 조회
	if r.Computed.PIDPlayerDescs != nil {
		if pd, ok := r.Computed.PIDPlayerDescs[p.ID]; ok && pd != nil {
			return int(pd.APM)
		}
	}
	// 폴백: 슬라이스 순회
	for _, pd := range r.Computed.PlayerDescs {
		if pd != nil && pd.PlayerID == p.ID {
			return int(pd.APM)
		}
	}
	return 0
}

var fpsMap = map[string]float64{
	"Slowest": 6.0,
	"Slower":  9.0,
	"Slow":    12.0,
	"Normal":  15.0,
	"Fast":    18.0,
	"Faster":  21.0,
	"Fastest": 23.81,
}

// toUTF8 converts EUC-KR encoded strings to UTF-8.
// SC1 replay map names are often stored in EUC-KR (CP949).
// If the string is already valid UTF-8, returns it as-is.
// cleanMapName strips SC1 color codes and handles encoding.
func cleanMapName(s string) string {
	// 1단계: SC1 컬러 코드(제어문자 0x01~0x1F) 제거
	var raw []byte
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x20 {
			raw = append(raw, s[i])
		}
	}
	cleaned := string(raw)

	// 2단계: 유효한 UTF-8이면 그대로 사용
	if utf8.ValidString(cleaned) {
		return cleaned
	}

	// 3단계: EUC-KR로 디코딩 시도
	result, _, err := transform.String(korean.EUCKR.NewDecoder(), cleaned)
	if err != nil {
		return cleaned
	}
	return result
}


func framesToSeconds(frames int32, speedName string) int {
	fps, ok := fpsMap[speedName]
	if !ok {
		fps = 23.81
	}
	return int(float64(frames) / fps)
}

func raceString(race *repcore.Race) string {
	if race == nil {
		return "알 수 없음"
	}
	switch race {
	case repcore.RaceTerran:
		return "테란"
	case repcore.RaceZerg:
		return "저그"
	case repcore.RaceProtoss:
		return "프로토스"
	default:
		return race.Name
	}
}

