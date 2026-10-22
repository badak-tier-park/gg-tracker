# GG Tracker — Claude 작업 지침

SC1 Remastered `.rep` 리플레이 파일을 감시해 승/패 기록을 자동으로 DB에 저장하는 데스크톱 프로그램(Go).
**비개발자 사용자**가 실행 파일 하나(Windows `.exe` / macOS)로 받아 쓴다.
**이 문서에는 코드에서 확인할 수 없는 내용만 적는다.** 파일 구조·의존성은 코드와 `go.mod`를 볼 것.

## 놓치면 버그가 되는 것
- DB에 직접 쓰지 않고 Edge Function(`badak-tier-park-api`의 `record-game`, `get-games`)을 거친다.
  `internal/db`(로컬 PostgreSQL 직접 연결)는 쓰지 않을 예정인 코드다
- 기록되는 `games`는 웹(badak-web)의 전적·대시보드가 읽는다. `map_name`은 웹에서 `maps.name`/`aliases`와
  **텍스트로** 매칭되므로 리플레이의 맵 이름을 가공해 넣으면 웹에서 맵이 연결되지 않는다
- 기록에는 디스코드 로그인(`discord_id`)이 필요하다. 미로그인 상태에서는 감지해도 저장하지 않고 넘어간다.
  세션은 메모리에만 있어 프로그램을 다시 켜면 재로그인해야 한다

## 조직 저장소 지도
바닥티어는 저장소 여러 개가 **같은 Supabase 프로젝트를 함께 쓴다**(운영 `badak`, 개발 `badak-dev`).
한 저장소에서 테이블·정책을 바꾸면 다른 저장소가 깨질 수 있으니, DB를 건드릴 땐 아래 사용처를 먼저 본다.
공개 저장소이므로 project ID는 문서에 쓰지 않고 Supabase MCP의 `list_projects`로 조회한다.

| 저장소 | 역할 | 배포 | DB와의 관계 |
|--------|------|------|-------------|
| `badak-web` | 웹앱(리그·팀배틀·토너먼트·대시보드·관리) | Vercel (`main`=운영) | 대부분의 테이블을 읽고 쓴다 |
| `badak-bot` | 디스코드 봇(`/유저등록` 등) | EC2 (`main` 푸시 시) | `users` 등록 — 웹은 여기 등록된 사용자만 로그인 가능 |
| `gg-tracker` | 리플레이 감시 데스크톱 앱(Go) | GitHub Release (`main` 푸시 시) | `record-game` 함수로 `games`에 전적 기록 |
| `badak-tier-park-api` | Supabase Edge Functions | `supabase functions deploy` | `record-game`·`get-games`(gg-tracker), `notify-entry`(웹의 엔트리 제출 → 디스코드) |
| `.github` | 조직 프로필·공통 이슈/PR 템플릿 | — | — |

로컬에서는 `~/dev/badak-tier-park/<저장소>`에 나란히 둔다.

## 환경변수와 배포
- 로컬 실행(`go run .`)은 `.env`로 **개발 DB(`badak-dev`)**에 적재한다. 키 목록은 `.env.example`
- 배포는 `main` 푸시 시 GitHub Actions(`release.yml`)가 빌드해 GitHub Release로 올린다.
  운영 값은 GitHub Secrets에서 빌드 시점에 ldflags로 주입되며 로컬 `.env`와 완전히 분리돼 있다
  (`SUPABASE_URL`, `SUPABASE_KEY`, `APP_SECRET`, `DISCORD_CLIENT_ID`, `DISCORD_CLIENT_SECRET`)
- **릴리스는 `version.txt`의 값이 태그가 된다.** 이미 있는 태그면 빌드를 건너뛰므로, 배포하려면
  `version.txt`를 올린 뒤 `main`에 병합한다
- Supabase 프로젝트는 이름으로 지칭한다(운영 `badak`, 개발 `badak-dev`). 공개 저장소이므로 project ID를
  문서에 쓰지 않는다. 스키마 변경은 Supabase MCP로 직접 적용하고 로컬 스키마 파일은 두지 않는다

## Discord OAuth
- Discord Developer Portal의 앱에 Redirect `http://localhost:8080/auth/discord/callback`이 등록돼 있어야 한다
- 대시보드 헤더의 "Discord로 로그인" → OAuth → 세션 저장(메모리)

## 알려진 주의사항
- Go 백틱 문자열 안에 JS 템플릿 리터럴(`` `${...}` ``) 사용 불가 → 문자열 연결로 우회
- `repcore.GameType1on1` 상수는 `screp v1.5.1`에 없음 → 사용하지 않음

## 커밋 전 검증
테스트는 없다. 빌드와 vet이 통과해야 커밋한다:
```
go build ./... && go vet ./...
```
리플레이 감지·기록을 바꿨다면 실제 `.rep` 파일로 개발 DB에 들어가는지 확인한다.

## GitHub 작업 규칙
조직 공통 규칙의 원본은 `badak-web/CLAUDE.md`의 "Git" 섹션이다. 이 저장소에 해당하는 것만 추린다.

- 브랜치: `feat/`·`fix/`·`refactor/`·`style/`·`docs/`·`chore/` → `dev`로 PR → 모아서 `dev` → `main` 배포 PR.
  **`main` 병합 = 운영 배포**이므로 작업 PR을 `main`으로 열지 않는다
- PR 제목은 커밋과 같은 형식(`feat: …`). 템플릿은 조직 공통(`badak-tier-park/.github`)
- Assignees = 작성자, Reviewers = 작성자 외 쓰기 권한자 전원
  (`gh api repos/badak-tier-park/<저장소>/collaborators --jq '.[].login'`), 종류 Labels,
  관련 이슈는 본문 `Closes #n`. 이 저장소에는 badak-web 같은 PR 자동 설정 워크플로가 없어 직접 채운다
- 보호 규칙: `dev`·`main` 모두 승인 1명 필요, 삭제·강제 푸시 금지

- Projects: 조직 프로젝트 `gg-tracker`(#4)
  (`gh project item-add 4 --owner badak-tier-park --url <PR/이슈 URL>`)

---

## 전역 개발 규칙 (사용자 전역 CLAUDE.md 복사본)
`~/.claude/CLAUDE.md`는 특정 PC에만 있어 다른 기기에서는 보이지 않으므로 여기에 복사해 둔다.
프로젝트에 국한되지 않는 새 규칙이 생기면 여기와 전역 파일 **양쪽에** 반영한다.
위 프로젝트 규칙과 겹치면 위(프로젝트) 서술이 우선한다.

### 커밋 컨벤션
**Conventional Commits**: `<type>(<scope>): <subject>` — scope는 선택.
- type: `feat` · `fix` · `docs` · `test` · `refactor` · `chore` · `style` · `perf`
- subject는 명령형 한 줄, 끝에 마침표 없음
- 본문은 "무엇"이 아니라 **"왜"**를 설명할 때만 쓴다

### 커밋 전 검증
typecheck·lint·test가 통과하는 상태에서 커밋한다. 명령은 프로젝트마다 다르니
CLAUDE.md·package.json scripts·CI에서 확인한다(이 저장소의 명령은 위 "커밋 전 검증").

### 이슈 정리
이슈를 닫기 전에 하위 이슈(sub-issue) 상태를 확인한다. 하위가 하나라도 열려 있으면
상위는 닫지 않는다. 해결 근거를 코드·커밋에서 확인하지 못한 이슈는 닫지 말고 사용자에게 묻는다.

### PR 작성
- Assignees에 작성자, Reviewers에 작성자를 뺀 팀원 전원, 종류·영역 Labels, 관련 이슈는 본문 `Closes #n`으로
  Development에 연결, 프로젝트를 쓰면 Projects에도 넣는다. 저장소에 자동화가 있어도 `gh pr create`로
  직접 올릴 때는 `--assignee @me --reviewer … --label …`로 채운다(자동화가 안 도는 경우가 있다)
- PR 템플릿이 있으면 그 섹션 구조를 따른다. 하지 않은 테스트는 체크하지 않고, 확인하지 못한 것은 적는다

### 문서 계층
- 루트 CLAUDE.md는 매 세션 로드되므로 전역 규칙과 "놓치면 버그가 되는" 요약만 둔다
- 기능별 상세는 그 디렉토리의 CLAUDE.md에 두고, 루트에 **문서 지도**로 위치를 적는다
- 하위 문서는 그 디렉토리 파일을 열 때만 로드된다 — 다른 디렉토리에서 그 기능을 건드릴 땐
  직접 읽어야 하므로 루트에 요약과 경로를 남긴다
- 코드·git 이력으로 알 수 있는 것은 적지 않는다. 코드로 알 수 없는 "왜"와 도메인 룰만 적는다
