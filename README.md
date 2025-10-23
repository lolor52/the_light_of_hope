# invest_intraday

## Технические требования
- Go 1.24.5
- PostgreSQL 17.5

## Интеграции
- При обращении к "MOEX ISS" используйте авторизацию через "MOEX Passport", чтобы избежать ограничений на запросы.

## Миграции
- Применение: `go run ./cmd/migrate -dir migrations`
- Восстановление dirty-версии: `go run ./cmd/migrate -dir migrations -force <версия>`

## Ключевая логика модулей
- `cmd/migrate`: CLI-обёртка над `internal/a_submodule/migrate`, задаёт часовой пояс Europe/Moscow и умеет выполнять `force` для очистки dirty-состояний перед `Up`.
- `internal/auth`: слой абстракций для аутентификации; на данный момент используется подмодуль `auth/moex_passport`.
- `internal/a_submodule/migrate`: обёртка над golang-migrate, проверяет парность файлов `*.up.sql`/`*.down.sql` и поддерживает ручной `force` версии.
- `internal/a_submodule/moex`: HTTP-клиент MOEX ISS с авторизацией через Passport и методами для загрузки истории, стакана, свечей и справочника инструмента.
- `internal/a_technical/config`: функции загрузки JSON-конфигурации со строкой подключения к БД.
- `internal/a_technical/db`: репозиторий `TickerRepository` для поиска и вставки записей о тикерах в `ticker_history`.
- `models`: структуры предметной области; `models.TickerHistory` представляет расчётные метрики торговых сессий, `models.TickerInfo` хранит справочные данные тикеров, `models.OrderPrice` описывает уровни входа заявок.

