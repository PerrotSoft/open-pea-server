# Open Pea Server — Креатив-сервер (Vercel + Постоянный оркестратор)

## Исправление 404 на Vercel
В этом архиве файлы для Vercel (`vercel.json`, `go.mod`, `api/`, `public/`) находятся **строго в корне архива**.
Просто распакуйте архив и загрузите его содержимое в репозиторий `PerrotSoft/open-pea-server` (в корень ветки `main` / `главный`), после чего Vercel автоматически пересоберёт проект и откроет страницу без ошибки 404.

## Переменные окружения в Vercel (Settings -> Environment Variables)
- `ORCHESTRATOR_URL` — адрес вашего постоянного сервера (например, `http://185.x.x.x:8080`)
- `API_SECRET` — секретный токен для авторизации запросов (должен совпадать с токеном оркестратора)

## Запуск постоянного оркестратора (`backend-vps/`)
На вашем VPS / выделенном сервере:
```bash
cd backend-vps/game-server && go build -o game-server .
cd ../orchestrator && go build -o orchestrator .
PUBLIC_HOST="ВАШ_БЕЛЫЙ_IP" API_SECRET="ваш-секретный-ключ" ./orchestrator
```
Креатив-сервер автоматически выключится, если на нём никого не будет более 30 минут.
