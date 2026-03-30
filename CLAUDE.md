# GG Tracker — AI 컨텍스트

> 이 파일은 AI 코딩 도구(Claude, Cursor, Copilot 등)와 개발자를 위한 프로젝트 컨텍스트입니다.

## 프로젝트 개요

SC1 Remastered `.rep` 리플레이 파일을 감시하여 승/패 기록을 자동으로 DB에 저장하는 프로그램.
비개발자 사용자가 사용하는 단일 `.exe`로 배포됩니다.

## 기술 스택

| 역할 | 라이브러리 |
|------|-----------|
| 언어 | Go |
| 리플레이 파서 | `github.com/icza/screp` |
| 파일 감시 | `github.com/fsnotify/fsnotify` |
| DB 드라이버 | `github.com/jackc/pgx/v5` |
| 환경변수 | `github.com/joho/godotenv` |

## 파일 구조

```
gg-tracker/
├── main.go
├── go.mod / go.sum
├── config.json          # 사용자 설정 (ReplayPath 등)
├── .env                 # 로컬 개발용 (gitignore)
└── internal/
    ├── store/store.go   # Game 구조체 + Store 인터페이스 (공통)
    ├── api/client.go    # Supabase Edge Function HTTP 클라이언트
    ├── db/db.go         # 로컬 PostgreSQL 직접 연결
    ├── config/config.go # ReplayPath, SetupComplete
    ├── parser/parser.go # screp 파서, 승자/패자 감지
    ├── watcher/watcher.go # fsnotify, debounce 3초
    └── web/server.go    # 로컬 웹 대시보드 (port 8080)
```

## Supabase 프로젝트

- Project ID: `vydawdpzfpmwqmvymwsi`
- Edge Function: `record-game` (배포 완료)
- Edge Function 레포: `badak-tier-park-api` → `supabase/functions/record-game/index.ts`

## DB 스키마

스키마는 별도 레포에서 관리됩니다:

> https://github.com/badak-tier-park/badak-schema/blob/main/schema.sql

### 주요 테이블 요약

```sql
-- 사용자
users (id, discord_id, nickname, race CHECK('T','Z','P'), tier, is_admin, ...)

-- 닉네임/종족 변경 요청 (승인 워크플로)
change_requests (id, type CHECK('nickname','race'), discord_id, old_value, new_value,
                 status CHECK('pending','approved','rejected'), message_id, channel_id, ...)

-- 게임 기록
games (id, discord_id, played_at, map_name, game_duration_seconds,
       winner_name, winner_race, loser_name, loser_race,
       winner_apm, loser_apm, replay_file UNIQUE, ...)
```

## 알려진 주의사항

- Go 백틱 문자열 안에 JS 템플릿 리터럴(`` `${...}` ``) 사용 불가 → 문자열 연결로 우회
- `repcore.GameType1on1` 상수는 `screp v1.5.1`에 없음 → 사용하지 않음

## Discord OAuth 설정

게임 기록 시 `discord_id`를 저장하기 위해 Discord 로그인이 필요합니다.

### 1. Discord Application 등록

1. [Discord Developer Portal](https://discord.com/developers/applications) → New Application
2. OAuth2 → Redirects → `http://localhost:8080/auth/discord/callback` 추가
3. CLIENT ID, CLIENT SECRET 복사

### 2. 환경변수 설정

```env
# .env (로컬 개발)
DISCORD_CLIENT_ID=your_client_id
DISCORD_CLIENT_SECRET=your_client_secret
```

운영 빌드 시 GitHub Actions secret으로 ldflags 주입:
```
-X main.discordClientID=${{ secrets.DISCORD_CLIENT_ID }}
-X main.discordClientSecret=${{ secrets.DISCORD_CLIENT_SECRET }}
```

### 동작 방식

- 대시보드 헤더의 "Discord로 로그인" 버튼 → OAuth 인증 → 세션 저장 (in-memory)
- 로그인 상태에서만 리플레이 감지 시 DB 저장 (미로그인 시 콘솔 경고 출력 후 스킵)
- 세션은 프로그램 재시작 시 초기화 → 재로그인 필요
