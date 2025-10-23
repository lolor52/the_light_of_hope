# invest_intraday

## Технические требования
- Go 1.24.5
- PostgreSQL 17.5

## Миграции
- Применение: `go run ./cmd/migrate -dir migrations`
- Восстановление dirty-версии: `go run ./cmd/migrate -dir migrations -force <версия>`

## Ключевая логика модулей
- `cmd/migrate`: CLI-обёртка над `internal/a_submodule/migrate`, задаёт часовой пояс Europe/Moscow и умеет выполнять `force` для очистки dirty-состояний перед `Up`.
- `internal/auth`: слой абстракций для аутентификации.
- `internal/a_submodule/migrate`: обёртка над golang-migrate, проверяет парность файлов `*.up.sql`/`*.down.sql` и поддерживает ручной `force` версии.
- `internal/a_submodule/alor`: клиент ALOR OpenAPI для получения рыночных данных.
- `internal/a_submodule/indicators`: расчёт VAH/VAL/VWAP по данным основной торговой сессии через ALOR.
- `internal/a_technical/config`: функции загрузки JSON-конфигурации со строкой подключения к БД.
- `internal/a_technical/db`: репозиторий `TickerRepository` для поиска и вставки записей о тикерах в `ticker_history`.
- `models`: структуры предметной области; `models.TickerHistory` представляет расчётные метрики торговых сессий, `models.TickerInfo` хранит справочные данные тикеров, `models.OrderPrice` описывает уровни входа заявок.

