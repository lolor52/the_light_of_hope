# invest_intraday

## Технические требования
- Go 1.24.5
- PostgreSQL 17.5

## Структура модулей
- auth: каркас слоя аутентификации.
- auth/alor: заготовка интеграции с Alor для авторизации.
- market_prelaunch_selection: подготовительный этап отбора инструментов перед стартом торгов.
- order: заготовка работы с жизненным циклом заявок.
- a_submodule/variable: базовая площадка для расчётов переменных.
- a_submodule/variable/VWAP_VA_5: заготовка алгоритмов VWAP VA за 5 периодов.
- a_submodule/variable/VWAP_VA_today: заготовка расчётов VWAP VA за текущий день.
- a_submodule/variable/VWAP_VA_res: заготовка обработки результатов VWAP VA.
- a_submodule/session: заготовка управления торговыми сессиями.
- a_submodule/vah_gt_vwap_gt_val: заготовка контроля условия VAH > VWAP > VAL.
