# «Плоский» тренд-фильтр — формулы (только «Основная» сессия)

## Данные
- **candles 1m**: `open`, `close`, `high`, `low`, `value`, `volume`, `begin`, `end`
- **candles 1d**: не используются для high/low и суточных агрегатов
- **history (daily)**: не используются для VWAP; суточные агрегаты считаются из 1m по «Основной» сессии

## Сессионный фильтр
Для дня *d* определим множество минутных баров «Основной» сессии (MSK):
```
T_open(d)  = начало Основной сессии по биржевому расписанию для BOARDID тикера на дату d (MSK)
T_close(d) = окончание Основной сессии по биржевому расписанию для BOARDID тикера на дату d (MSK)

TA_open_start(d), TA_open_end(d) = интервалы аукциона открытия по биржевому расписанию для тикера/BOARDID на дату d (MSK)
TA_close_start(d), TA_close_end(d) = интервалы аукциона закрытия по биржевому расписанию для тикера/BOARDID на дату d (MSK)

S_d = { j : begin_j ∈ [ T_open(d), T_close(d) )
           и begin_j ∉ [ TA_open_start(d),  TA_open_end(d) )
           и begin_j ∉ [ TA_close_start(d), TA_close_end(d) ) }
```
Все суммы, средние и экстремумы ниже считаются только по множеству **S_d**.

### Предыдущая торговая сессия
```
d_prev = предыдущая дата «Основной» торговой сессии по торговому календарю биржи для тикера/BOARDID
```

Индекс *t* — это порядковый номер бара внутри **S_d** по возрастанию времени.

## VWAP и наклон (интрадей)
```
VWAP_t       = ( Σ_{j∈S_d, j≤t} value_j ) / ( Σ_{j∈S_d, j≤t} volume_j + ε )
SlopeVWAP_k% = ( VWAP_t - VWAP_{t-k} ) / ( VWAP_{t-k} + ε ) * 100
```
*Примечание:* если t < k, то SlopeVWAP_k% не определяется или пропускается.

## Дневные экстремумы и суточные агрегаты по «Основной»
Дневные high/low берутся как экстремумы 1m-баров внутри **S_d**:
```
high_d^MS = max_{j∈S_d} high_1m_j
low_d^MS  = min_{j∈S_d} low_1m_j
```
Суточные суммы и дневной VWAP по «Основной»:
```
VALUE_d^MS  = Σ_{j∈S_d} value_j
VOLUME_d^MS = Σ_{j∈S_d} volume_j
VWAP_day^MS = VALUE_d^MS / ( VOLUME_d^MS + ε )
```

## Перекрытие диапазонов (дни d и d_prev, только «Основная»)
```
OverlapWidth_d = max( 0,
                      min( high_d^MS,  high_{d_prev}^MS ) -
                      max( low_d^MS,   low_{d_prev}^MS  ) )

UnionWidth_d   = max( high_d^MS,  high_{d_prev}^MS ) -
                 min( low_d^MS,   low_{d_prev}^MS  )

Overlap%_d     = OverlapWidth_d / ( UnionWidth_d + ε ) * 100
```

## Смещение относительно середины диапазона «Основной»
```
Mid_d   = ( high_d^MS + low_d^MS ) / 2
Skew%_d = | VWAP_day^MS - Mid_d | / ( high_d^MS - low_d^MS + ε ) * 100
```

## Нормировка и индекс «плоскости»
```
norm_up(x_i)   = ( x_i - min_S x ) / ( max_S x - min_S x + ε )
norm_down(x_i) = 1 - norm_up(x_i)

F_i = 100 * [ 0.5 * norm_up( Overlap%_i )
            + 0.3 * norm_down( |SlopeVWAP_k%|_i )
            + 0.2 * norm_down( Skew%_i ) ]
```

## Параметры
```
ε = 1e-9
k = 10  // баров 1m внутри S_d для SlopeVWAP
S = множество тикеров для кросс-нормировки на дату d
Временная зона: MSK
```


### Уточнение VWAP и LOTSIZE
Используйте количество бумаг Q. Если `volume` в лотах, `Q = volume * LOTSIZE`; иначе `Q = volume`. Тогда дневной VWAP вычисляется как `VWAP = (∑ VALUE_i)/(∑ Q_i)` по минутам основной сессии.