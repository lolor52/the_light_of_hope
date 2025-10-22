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
- `cmd/tickers_selection`: загружает `config.json`, инициализирует сервис заполнения тикеров и завершает работу по сигналу ОС.
- `internal/auth`: слой абстракций для аутентификации; подмодуль `auth/alor` подготовлен для интеграции с Alor.
- `internal/order`: заготовка под управление жизненным циклом заявок.
- `internal/a_submodule/migrate`: обёртка над golang-migrate, проверяет парность файлов `*.up.sql`/`*.down.sql` и поддерживает ручной `force` версии.
- `internal/a_submodule/moex`: HTTP-клиент MOEX ISS с авторизацией через Passport и методами для загрузки истории, стакана, свечей и справочника инструмента.
- `internal/a_submodule/tickers_filling`: сервис `Service` выбирает последние активные торговые сессии, пересчитывает VWAP/VAL/VAH, фильтр бокового тренда, волатильность и ликвидность, а затем сохраняет результаты в таблицу `ticker_history`.
- `internal/a_submodule/session`, `internal/a_submodule/variable`, `internal/a_submodule/vah_gt_vwap_gt_val`: заготовки под дальнейшую логику торговых сессий и расчёты переменных/условия VAH > VWAP > VAL.
- `internal/a_technical/config`: функции загрузки JSON-конфигурации с параметрами MOEX Passport, списком тикеров и строкой подключения к БД.
- `internal/a_technical/db`: репозиторий `TickerRepository` для поиска и вставки записей о тикерах в `ticker_history`.
- `models`: структуры предметной области; `models.Ticker` хранит рассчитанные метрики торговой сессии для записи в БД.

## Дополнительная документация
- Формулы расчёта метрик отбора тикеров описаны в `docs/formulas_tickers_selection.md`.
