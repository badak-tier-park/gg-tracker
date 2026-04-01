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
├── .env.example         # 환경변수 템플릿
└── internal/
    ├── store/store.go   # Game 구조체 + Store 인터페이스 (공통)
    ├── api/client.go    # Supabase Edge Function HTTP 클라이언트
    ├── db/db.go         # 로컬 PostgreSQL 직접 연결 (미사용 예정)
    ├── config/config.go # ReplayPath, SetupComplete
    ├── parser/parser.go # screp 파서, 승자/패자 감지
    ├── watcher/watcher.go # fsnotify, debounce 3초
    └── web/server.go    # 로컬 웹 대시보드 (port 8080)
```

## 환경변수

### 로컬 개발 (.env)

로컬 실행 시 Supabase **개발 DB**(`badak-dev`)에 데이터를 적재합니다.
`.env.example`을 복사해서 `.env`를 만들고 값을 채워주세요.

```env
SUPABASE_URL=https://<dev-project-id>.supabase.co
SUPABASE_ANON_KEY=
APP_SECRET=
DISCORD_CLIENT_ID=
DISCORD_CLIENT_SECRET=
```

### 운영 배포 (GitHub Secrets)

`main` 브랜치 푸시 시 GitHub Actions가 빌드하면서 아래 시크릿을 ldflags로 주입합니다.
로컬 `.env`와 완전히 분리되어 있습니다.

| Secret 키 | 용도 |
|-----------|------|
| `SUPABASE_URL` | 운영 Supabase 프로젝트 URL |
| `SUPABASE_KEY` | 운영 anon key |
| `APP_SECRET` | Edge Function 인증 시크릿 |
| `DISCORD_CLIENT_ID` | Discord OAuth 클라이언트 ID |
| `DISCORD_CLIENT_SECRET` | Discord OAuth 클라이언트 시크릿 |

## Supabase 프로젝트

| 환경 | Project ID | 용도 |
|------|-----------|------|
| 운영 | `vydawdpzfpmwqmvymwsi` | 실제 유저 데이터 |
| 개발 | `wtzfekruohdxchjpefdj` | 로컬 개발 및 테스트 |

Edge Function: `record-game`, `get-games` (운영/개발 양쪽 모두 배포됨)

## DB 스키마

스키마 변경이 필요할 경우 Supabase MCP를 통해 직접 적용합니다 (`.mcp.json` 참고).

로컬에 별도 스키마 파일을 관리하지 않습니다.

## 알려진 주의사항

- Go 백틱 문자열 안에 JS 템플릿 리터럴(`` `${...}` ``) 사용 불가 → 문자열 연결로 우회
- `repcore.GameType1on1` 상수는 `screp v1.5.1`에 없음 → 사용하지 않음

## Discord OAuth 설정

게임 기록 시 `discord_id`를 저장하기 위해 Discord 로그인이 필요합니다.

### 1. Discord Application 등록

1. [Discord Developer Portal](https://discord.com/developers/applications) → New Application
2. OAuth2 → Redirects → `http://localhost:8080/auth/discord/callback` 추가
3. CLIENT ID, CLIENT SECRET 복사

### 2. 동작 방식

- 대시보드 헤더의 "Discord로 로그인" 버튼 → OAuth 인증 → 세션 저장 (in-memory)
- 로그인 상태에서만 리플레이 감지 시 DB 저장 (미로그인 시 콘솔 경고 출력 후 스킵)
- 세션은 프로그램 재시작 시 초기화 → 재로그인 필요
