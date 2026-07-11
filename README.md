# Named Clocks

Сервис-конвейер: по списку ФИО генерирует именные видео с часами.
Для каждого ФИО выполняются 3 этапа:

1. **Изображение** — заказ в [Imanator](https://imanator.pro) по `templateId` + настройкам (имя/фамилия подставляются в шаблон), поллинг статуса.
2. **Видео** — image-to-video в [OpenRouter](https://openrouter.ai) (готовая картинка идёт первым кадром, «оживляем» её), поллинг статуса.
3. **Сохранение** — готовое видео скачивается и кладётся в MinIO (S3), пользователю отдаётся ссылка.

Ссылки на результаты появляются на фронте по мере готовности. Есть история пачек.

## Стек

- **Frontend** — Vite + React + TypeScript + Tailwind (nginx в проде)
- **Backend** — Go (chi, воркер-пул, поллинг)
- **БД** — PostgreSQL (задачи и пачки)
- **Объектное хранилище** — MinIO (S3-совместимое)
- **Оркестрация** — Docker Compose (монорепа)

```
named_clocks/
├── docker-compose.yml
├── .env.example
├── backend/     # Go API + воркер
└── frontend/    # Vite + React
```

## Быстрый старт (Docker)

1. Скопируй переменные окружения и заполни ключи:

```bash
cp .env.example .env
```

Обязательно задай:
- `IMANATOR_API_KEY` — Bearer-ключ Иманатора;
- `OPENROUTER_API_KEY` — ключ OpenRouter;
- при желании поменяй `APP_LOGIN` / `APP_PASSWORD` и `JWT_SECRET`.

2. Подними всё:

```bash
docker compose up -d --build
```

3. Открой:
- Фронт: <http://localhost:8081>
- API: <http://localhost:8080/api/health>
- MinIO Console: <http://localhost:9001>

### Вход

Логин/пароль по умолчанию: `admin` / `clocks2026!` (меняются через `.env`).
Пользователь один — мультиаккаунт не предусмотрен.

## Как пользоваться

1. Зайди на вкладку **Создать**.
2. Вставь список ФИО построчно (например, `Иван Иванов`).
3. Укажи **ID шаблона Иманатора** и, при необходимости, доп. настройки шаблона (JSON).
4. Выбери модель видео (список подтягивается из OpenRouter), при желании поправь промпт/длительность/разрешение.
5. Нажми «Запустить N задач» — прогресс и ссылки появятся справа.
6. Вкладка **История** — старые пачки и их результаты.

Все настройки формы (список имён, шаблон, промпт и т.д.) хранятся в `localStorage` браузера.

### Подстановка имени в шаблон

Иманатор подставляет `settings` в шаблон как `{{ключ}}`. По умолчанию бэкенд кладёт:
- `firstName` — имя,
- `lastName` — фамилию,
- `name` — «Имя Фамилия».

Ключи можно переопределить на форме под конкретный шаблон.

## Локальная разработка (без Docker)

Backend (нужен Go 1.25+, локальные Postgres и MinIO):

```bash
cd backend
export $(grep -v '^#' ../.env | xargs)   # или задай переменные вручную
go run ./cmd/server
```

Frontend:

```bash
cd frontend
npm install
npm run dev   # http://localhost:5173, API проксируется на :8080
```

## Параметры воркера

| Переменная | По умолчанию | Описание |
|---|---|---|
| `WORKER_CONCURRENCY` | 4 | Сколько задач обрабатывается параллельно |
| `POLL_INTERVAL_SECONDS` | 2 | Интервал поллинга статусов |
| `STAGE_TIMEOUT_SECONDS` | 600 | Таймаут ожидания одного этапа |

## Заметки по интеграциям

- **Imanator**: `POST /api/image-generation-orders` → `GET /api/image-generation-orders/{id}`. Авторизация `Bearer <IMANATOR_API_KEY>`.
- **OpenRouter Video**: `POST /api/v1/videos` (`frame_images` = первый кадр для image-to-video) → поллинг `GET /api/v1/videos/{id}`, результат в `unsigned_urls`. Список моделей: `GET /api/v1/videos/models`.
- Presigned-ссылки MinIO подписываются под `MINIO_PUBLIC_ENDPOINT` (по умолчанию `localhost:9000`), чтобы их открывал браузер.
