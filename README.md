# Vauln Address API

API для проверки безопасности криптокошельков в разных блокчейнах (EVM, Bitcoin, Solana, Sui, Tron).

## Архитектура

```
┌─────────────────────────────────────────────────────────────┐
│                      Nginx (port 80/443)                     │
│   ┌─────────────────────────────────────────────────────┐   │
│   │  Frontend (SPA) - статические файлы                │   │
│   │  /api/* → проксирует на API                        │   │
│   └─────────────────────────────────────────────────────┘   │
└─────────────────────┬───────────────────────────────────────┘
                      │
                      │ proxy_pass
                      ▼
┌─────────────────────────────────────────────────────────────┐
│                 Go API Server (port 9111)                   │
│   Только API endpoints /api/*                                │
└─────────────────────────────────────────────────────────────┘
```

## Технологии

- **Язык**: Go 1.21+
- **База данных**: PostgreSQL
- **Фреймворк**: Gin
- **Frontend**: Vue.js (собирается отдельно)
- **Веб-сервер**: Nginx
- **Конфигурация**: .env файл

## Структура проекта

```
vauln-address/
├── cmd/server/          # Точка входа API
├── internal/
│   ├── config/          # Конфигурация
│   ├── handlers/        # HTTP обработчики
│   ├── middleware/      # Rate limiting, CORS
│   ├── models/          # Модели данных
│   ├── repository/     # Работа с БД
│   └── validators/      # Валидация адресов
├── frontend/            # Vue.js приложение
│   ├── src/            # Исходный код
│   └── dist/           # Собранные файлы
├── migrations/          # SQL миграции
├── nginx.conf.example   # Пример конфигурации Nginx
├── .env.example         # Пример конфигурации API
└── go.mod
```

## Установка

```bash
# Клонирование и переход в директорию
cd vauln-address

# Установка зависимостей
go mod download

# Создание .env файла
cp .env.example .env
# Отредактируйте .env под вашу конфигурацию
```

## Настройка базы данных

Создайте базу данных PostgreSQL:

```sql
CREATE DATABASE vauln_address;
```

## Запуск

```bash
go run cmd/server/main.go
```

## API Endpoints

### GET /
Информация о сервисе

### GET /health
Проверка здоровья сервиса

**Response:**
```json
{
  "status": "ok",
  "service": "vauln-address-api",
  "time": "2024-01-01T00:00:00Z"
}
```

### POST /check
Проверка кошелька (требует rate limit: 10 запросов/24 часа)

**Request:**
```json
{
  "address": "0x742d35Cc6634C0532925a3b844Bc9e7595f5B2a1",
  "chain": "evm"
}
```

**Response:**
```json
{
  "address": "0x742d35Cc6634C0532925a3b844Bc9e7595f5B2a1",
  "chain": "evm",
  "status": "hacked",
  "has_pk": true,
  "has_seed": false,
  "found": true
}
```

**Статусы:**
- `hacked` - кошелек взломан
- `vulnerable` - уязвим
- `safe` - безопасен
- `hacker` - принадлежит хакеру
- `drained` - средства выведены
- `not_found` - не найден в базе

### GET /recent
Последние проверки (для live-алертов)

**Response:**
```json
{
  "checks": [
    {
      "id": 1,
      "address": "...",
      "chain": "evm",
      "status": "hacked",
      "checked_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1
}
```

### POST /contact
Контактная форма

**Request:**
```json
{
  "name": "John Doe",
  "email": "john@example.com",
  "message": "Your message here..."
}
```

### GET /chains
Список поддерживаемых сетей с примерами адресов

## Валидация адресов

| Сеть     | Формат                                      | Пример                                      |
|----------|---------------------------------------------|---------------------------------------------|
| EVM      | 0x + 40 hex символов                       | 0x742d35Cc6634C0532925a3b844Bc9e7595f5B2a1 |
| Bitcoin  | 26-35 символов, начинается с 1, 3 или bc1  | bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlh |
| Solana   | 32-44 символа base58                        | 7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV |
| Sui      | 0x + 64 hex символа                        | 0x8a...c4e6                                 |
| Tron     | 34 символа, начинается с T                 | TJK5M5kKxP8xF9cGvN2pL6rU4sW7xA3bCd          |

## Rate Limiting

- **Лимит**: 10 проверок кошельков за 24 часа на один IP
- **Headers**: `X-RateLimit-Limit`, `X-RateLimit-Remaining`

## Развертывание

### 1. Сборка Frontend

```bash
cd frontend
npm install
npm run build
# Файлы будут в frontend/dist/
```

### 2. Настройка API

```bash
cp .env.example .env
# Отредактируйте .env (SERVER_PORT=9111 уже установлен по умолчанию)
```

### 3. Настройка Nginx

```bash
# Скопируйте конфигурацию
cp nginx.conf.example /etc/nginx/sites-available/vauln-address

# Отредактируйте конфигурацию:
# - set $API_ADDRESS - адрес вашего API сервера
# - set $FRONTEND_PATH - путь к собранным файлам frontend
# - server_name - ваш домен

# Активируйте сайт
ln -s /etc/nginx/sites-available/vauln-address /etc/nginx/sites-enabled/
nginx -t && systemctl reload nginx
```

### 4. Запуск API

```bash
# Запуск API сервера (порт 9111)
go run cmd/server/main.go
```

### Docker

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o server ./cmd/server

FROM alpine
COPY --from=builder /app/server /server
COPY --from=builder /app/.env.example /.env
CMD ["/server"]
```

### Systemd Service (пример)

```ini
[Unit]
Description=Vauln Address API
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/vauln-address
ExecStart=/opt/vauln-address/server
EnvironmentFile=/opt/vauln-address/.env
Restart=always

[Install]
WantedBy=multi-user.target
```

## Разработка

```bash
# Форматирование
go fmt ./...

# Линтинг
go vet ./...

# Тесты
go test ./...
```

## Лицензия

MIT
