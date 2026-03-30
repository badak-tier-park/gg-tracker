package watcher

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"gg-tracker/internal/parser"
	"gg-tracker/internal/store"
)

type Watcher struct {
	replayPath   string
	db           store.Store
	getDiscordID func() int64
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

func New(replayPath string, database store.Store, getDiscordID func() int64) *Watcher {
	return &Watcher{
		replayPath:   replayPath,
		db:           database,
		getDiscordID: getDiscordID,
		stopCh:       make(chan struct{}),
	}
}

func (w *Watcher) Stop() {
	close(w.stopCh)
	w.wg.Wait()
}

func (w *Watcher) Watch() {
	w.wg.Add(1)
	defer w.wg.Done()

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Printf("[오류] 파일 감시자 생성 실패: %v\n", err)
		return
	}
	defer fsw.Close()

	// 디렉토리가 없으면 생성
	if err := os.MkdirAll(w.replayPath, 0755); err != nil {
		fmt.Printf("[오류] 디렉토리 생성 실패: %v\n", err)
		return
	}

	if err := fsw.Add(w.replayPath); err != nil {
		fmt.Printf("[오류] 경로 감시 실패 (%s): %v\n", w.replayPath, err)
		return
	}

	// 중복 처리 방지용 debounce 맵
	pending := make(map[string]*time.Timer)
	var mu sync.Mutex

	for {
		select {
		case <-w.stopCh:
			return

		case event, ok := <-fsw.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Create|fsnotify.Write) == 0 {
				continue
			}

			// 하위 디렉토리 생성 시 감시 추가 (날짜별 폴더, AutoSave 등)
			info, err := os.Stat(event.Name)
			if err == nil && info.IsDir() {
				fsw.Add(event.Name)
				continue
			}

			if !isReplayFile(event.Name) {
				continue
			}

			mu.Lock()
			if t, exists := pending[event.Name]; exists {
				t.Reset(3 * time.Second)
			} else {
				name := event.Name
				pending[name] = time.AfterFunc(3*time.Second, func() {
					mu.Lock()
					delete(pending, name)
					mu.Unlock()
					w.processReplay(name)
				})
			}
			mu.Unlock()

		case err, ok := <-fsw.Errors:
			if !ok {
				return
			}
			fmt.Printf("[오류] 파일 감시 오류: %v\n", err)
		}
	}
}

func (w *Watcher) processReplay(filePath string) {
	// 파일 크기가 안정될 때까지 대기 (최대 10초)
	if !waitForStableFile(filePath, 2*time.Second, 10*time.Second) {
		fmt.Printf("[경고] 파일이 아직 쓰여지고 있습니다: %s\n", filepath.Base(filePath))
		return
	}

	// 동일 파일명(예: LastReplay.rep)에 덮어쓰는 경우를 위해 해시로 중복 검사
	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("[%s] 파일 읽기 실패: %v\n", timestamp(), err)
		return
	}
	hashStr := fmt.Sprintf("%x", sha256.Sum256(data))

	if w.db.IsAlreadyProcessed(hashStr) {
		return
	}

	fmt.Printf("[%s] 리플레이 감지: %s\n", timestamp(), filepath.Base(filePath))

	discordID := w.getDiscordID()
	if discordID == 0 {
		fmt.Printf("[%s] Discord 로그인 필요 — http://localhost:8080 에서 로그인 후 게임이 기록됩니다\n", timestamp())
		return
	}

	game, err := parser.ParseReplay(filePath)
	if err != nil {
		fmt.Printf("[%s] 파싱 실패: %v\n", timestamp(), err)
		return
	}

	// DB의 UNIQUE 제약조건 통과 및 고유성 보장을 위해 ReplayFile 필드를 해시로 덮어씀
	game.ReplayFile = hashStr
	game.DiscordID = discordID

	if err := w.db.InsertGame(game); err != nil {
		fmt.Printf("[%s] DB 저장 실패: %v\n", timestamp(), err)
		return
	}

	fmt.Printf("[%s] ✓ 기록 완료: %s(%s) 승 vs %s(%s) 패 | 맵: %s | %s\n",
		timestamp(),
		game.WinnerName, game.WinnerRace,
		game.LoserName, game.LoserRace,
		game.MapName,
		formatDuration(game.GameDurationSeconds),
	)
}

// waitForStableFile polls the file until its size doesn't change for stableFor duration.
func waitForStableFile(path string, stableFor, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	var lastSize int64 = -1
	stableSince := time.Time{}

	for time.Now().Before(deadline) {
		info, err := os.Stat(path)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		if info.Size() == lastSize {
			if stableSince.IsZero() {
				stableSince = time.Now()
			} else if time.Since(stableSince) >= stableFor {
				return true
			}
		} else {
			lastSize = info.Size()
			stableSince = time.Time{}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func isReplayFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".rep")
}

func formatDuration(seconds int) string {
	m := seconds / 60
	s := seconds % 60
	return fmt.Sprintf("%d:%02d", m, s)
}

func timestamp() string {
	return time.Now().Format("15:04:05")
}
