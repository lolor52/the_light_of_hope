# invest_intraday

## Технические требования
- Go 1.24.5
- PostgreSQL 17.5

## Миграции
- Применение: `go run ./cmd/migrate -dir migrations`
- Восстановление dirty-версии: `go run ./cmd/migrate -dir migrations -force <версия>`

## Ключевая логика модулей
- `main.go`: точка входа, которая только инициализирует базовую инфраструктуру (конфигурацию, подключение модулей); HTTP-эндпоинты и их логика реализуются внутри соответствующих модулей.
- `cmd/migrate`: CLI-обёртка над `internal/a_submodule/migrate`, задаёт часовой пояс Europe/Moscow и умеет выполнять `force` для очистки dirty-состояний перед `Up`.
- `internal/a_submodule/migrate`: обёртка над golang-migrate, проверяет парность файлов `*.up.sql`/`*.down.sql` и поддерживает ручной `force` версии.
- `internal/a_technical/config`: функции загрузки JSON-конфигурации со строкой подключения к БД.
- `models`: структуры предметной области; `models.TickerHistory` представляет расчётные метрики торговых сессий, `models.TickerInfo` хранит справочные данные тикеров, `models.OrderPrice` описывает уровни входа заявок.
