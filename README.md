# Open Pea Server — Креатив-сервер

## Що виправлено для Vercel CLI 65+
Vercel визначив проєкт як Go Backend Framework, який обов'язково вимагає точку входу `main.go` у корені проєкту (`Error: [BUILD_FAILED] No Go entrypoint found. Expected one of: main.go, cmd/api/main.go, cmd/server/main.go`).

У цьому архіві:
- У корені розташований `main.go` та `go.mod`.
- Статична сторінка `public/index.html` вбудована прямо в Go-бінарник через `//go:embed public/*`, що повністю вирішує і помилку збірки, і помилку `404 NOT_FOUND` на головній сторінці.
- `vercel.json` видалено, щоб уникнути конфліктів маршрутизації в Vercel CLI 65.15.0.

## Змінні середовища в Vercel (Settings -> Environment Variables)
- `ORCHESTRATOR_URL` — адреса вашого постійного сервера (наприклад, `http://185.x.x.x:8080`)
- `API_SECRET` — секретний токен (має збігатися з токеном оркестратора)
