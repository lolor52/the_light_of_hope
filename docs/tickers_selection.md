# Отбор тикеров для интрадей-торговли во флете: спецификация расчёта

Версия: 1.2 (замена бинарного фильтра на `FlatTrendFilter ∈ [0,100]`; остальное без изменений)

> **Сеансовый режим MOEX:** все входы и расчёты ниже применяются **только к Основной торговой сессии**. Утренняя и вечерняя сессии полностью исключены из выборки и агрегирования.

## Входы по каждому тикеру и дню *d* за последние *X* дней
_Все поля ниже уже рассчитаны по данным Основной торговой сессии MOEX._
- **Вселенная тикеров**: все тикеры борда TQBR (MOEX). Фильтр по борду TQBR обязателен на входе; бумаги вне TQBR исключаются.
- **VAH[d]**, **VAL[d]**, **VWAP[d]**
- **Liquidity[d]** ∈ [0,100]
- **Volatility[d]** ∈ [0,100]
- **FlatTrendFilter[d]** ∈ [0,100]  — чем выше, тем «флетовее» день

## Обозначения и константы
- `X` — число торговых дней **Основной** сессии в окне расчёта.
- ε = 1e−9 для защиты от деления на ноль.
- `clip(x, a, b)` — обрезка в [a,b].
- Перцентили `Pq(·)` считаются **кросс-секционно** по всем отобранным дням всех тикеров внутри окна *X*.
- Масштабируем V и L: `v = Volatility/100`, `l = Liquidity/100`.
- `τ_flat` — порог «флэта» по `FlatTrendFilter` на шкале 0–100. По умолчанию 70.

## Шаг 1. Отбор дней
_Рассматриваются только даты и метрики из Основной торговой сессии MOEX._
Используются только дни, где одновременно:
1) `FlatTrendFilter[d] ≥ τ_flat`  (порог «флэта»)  
2) `Liquidity[d] ≥ 30`  
3) `W[d] = VAH[d] − VAL[d] > 0` и `VWAP[d] > 0`

### Отсев экстремумов (robust)
После вычисления метрик ниже:
- Исключить дни с `v > P99(v)`.
- Исключить дни с `band_rel > P99(band_rel)`.
Если после отсевов у тикера <2 дней, тикер снимается с ранжирования.

## Шаг 2. Суточные метрики
```
W[d]        = max(VAH[d] - VAL[d], ε)
C[d]        = (VAH[d] + VAL[d]) / 2
band_rel[d] = W[d] / max(VWAP[d], ε)                 # относительная ширина коридора
centering[d]= 1 - min(1, abs(VWAP[d] - C[d]) / W[d]) # 1 если VWAP по центру VAH/VAL
v[d]        = Volatility[d] / 100
l[d]        = Liquidity[d]  / 100
```

### Нормализация ширины коридора
```
P10 = P10(band_rel[все отобранные дни])
P90 = P90(band_rel[все отобранные дни])
band_rel_norm[d] = clip((band_rel[d] - P10) / max(P90 - P10, ε), 0, 1)
```

### Предпочтительная волатильность для флэта
```
v_star = median( v[d] для всех d ∈ S )   # глобальный целевой уровень по всем отобранным дням и тикерам
h      = 0.40                                            # «полуширина» предпочтения
vol_pref[d] = clip(1 - abs(v[d] - v_star)/h, 0, 1)     # треугольный профиль
```

### Риск-штрафы
```
risk_pen[d]  = v[d] * (1 - l[d])
P90v         = P90(v[все отобранные дни])
P99v         = P99(v[все отобранные дни])
spike_pen[d] = clip((v[d] - P90v) / max(P99v - P90v, ε), 0, 1)
```

## Шаг 3. Суточные подбаллы и итог дня
```
Profit[d] = 0.55*band_rel_norm[d] + 0.25*l[d] + 0.20*vol_pref[d]
Safety[d] = 0.50*centering[d]     + 0.30*l[d] + 0.20*(1 - risk_pen[d]) - 0.20*spike_pen[d]
Score_day[d] = clip(0.60*Profit[d] + 0.40*Safety[d], 0, 1)
```

## Шаг 4. Агрегация по тикеру
Для тикера *T* с множеством его отобранных дней `D_T`:
```
score_avg[T]   = average( Score_day[d]           по d∈D_T )
center_w[T]    = average( centering[d]           по d∈D_T )
band_w[T]      = average( band_rel_norm[d]       по d∈D_T )
l_w[T]         = average( l[d]                   по d∈D_T )
v_w[T]         = average( v[d]                   по d∈D_T )
stability[T]   = |D_T| / X                       # доля «флэт-дней»
FinalScore[T]  = 100 * score_avg[T] * stability[T]
```

**Примечание о выборе на одну дату.** Если нужен отбор на конкретную дату `d0`, ранжируйте тикеры по `Score_day[d0]`. `FinalScore` используйте для оценки многодневной стабильности.

### Фильтры перед ранжированием тикеров
- Удалить тикеры с `stability[T] < 0.40`.
- Удалить тикеры с `l_w[T] < 0.30`.

### Ранжирование
Сортировать тикеры по `FinalScore[T]` по убыванию.

## Выходные поля по каждому тикеру
- `FinalScore`  
- `score_avg`, `stability`  
- `band_w`, `center_w`, `l_w`, `v_w`  
- `days_used = |D_T|`

## Псевдокод
```python
# inputs: table rows (ticker, day, VAH, VAL, VWAP, Liquidity, Volatility, FlatTrendFilter)
# Гарантия источника: ряды уже агрегированы ТОЛЬКО по Основной торговой сессии MOEX.
# Если поле session присутствует, оставляем только основную сессию:
tau_flat = 70  # порог "флэта" по шкале 0..100 (гиперпараметр)

# защитный фильтр по сессии (не меняет результат, если вход уже очищен)
rows = [r for r in rows if getattr(r, 'session', 'main') in ('main', 'Основная')]

S = []  # список отобранных дневных записей
for row in rows:
    if row.FlatTrendFilter < tau_flat: continue
    if row.Liquidity < 30:             continue
    W = max(row.VAH - row.VAL, 1e-9)
    if row.VWAP <= 0:                  continue
    band_rel = W / row.VWAP
    S.append({
        'ticker': row.ticker,
        'day': row.day,
        'W': W,
        'C': (row.VAH + row.VAL)/2,
        'VWAP': row.VWAP,
        'band_rel': band_rel,
        'v': row.Volatility/100,
        'l': row.Liquidity/100,
    })

# перцентили по S
P10  = pct(S.band_rel, 10)
P90  = pct(S.band_rel, 90)
P90v = pct(S.v, 90)
P99v = pct(S.v, 99)
v_star = median([s.v for s in S])

# дневные баллы
for s in S:
    centering = 1 - min(1, abs(s.VWAP - (s.C)) / s.W)
    band_rel_norm = clip((s.band_rel - P10)/max(P90-P10,1e-9), 0, 1)
    vol_pref = clip(1 - abs(s.v - v_star)/0.40, 0, 1)
    risk_pen  = s.v * (1 - s.l)
    spike_pen = clip((s.v - P90v)/max(P99v - P90v, 1e-9), 0, 1)
    Profit = 0.55*band_rel_norm + 0.25*s.l + 0.20*vol_pref
    Safety = 0.50*centering + 0.30*s.l + 0.20*(1 - risk_pen) - 0.20*spike_pen
    s.Score_day = clip(0.60*Profit + 0.40*Safety, 0, 1)

# отсев экстримов
S = [s for s in S if s.v <= P99v]
P99_band = pct([s.band_rel for s in S], 99)
S = [s for s in S if s.band_rel <= P99_band]

# агрегация по тикеру
by_ticker = groupby(S, key='ticker')
results = []
for T, items in by_ticker.items():
    X = total_days_in_window_for_ticker(T)  # число торговых дней Основной сессии в окне
    if X == 0: continue
    D_T = items
    if len(D_T) < 2: continue
    score_avg = mean(i.Score_day for i in D_T)
    stability = len(D_T) / X
    l_w = mean(i.l for i in D_T)
    if stability < 0.40 or l_w < 0.30: continue
    band_w = mean(i.band_rel_norm for i in D_T)
    center_w = mean(1 - min(1, abs(i.VWAP - (i.C)) / i.W) for i in D_T)
    v_w = mean(i.v for i in D_T)
    FinalScore = 100 * score_avg * stability
    results.append({
        'ticker': T,
        'FinalScore': FinalScore,
        'score_avg': score_avg,
        'stability': stability,
        'band_w': band_w,
        'center_w': center_w,
        'l_w': l_w,
        'v_w': v_w,
        'days_used': len(D_T),
    })

# ранжирование
results.sort(key=lambda r: r['FinalScore'], reverse=True)
```

## Гиперпараметры и тюнинг
- `τ_flat` — порог «флэта» по `FlatTrendFilter` (0–100). По умолчанию 70.
- `h` в `vol_pref`: 0.30–0.45. Уже = селективнее.
- Вес Profit/Safety: 0.60/0.40. Для сверхконсервативного подхода 0.50/0.50.
- Требование к центровке для «жёсткого флэта»: можно дополнительно требовать `centering[d] ≥ 0.7`.

## Проверка устойчивости
Рассчитывайте метрики на скользящем окне *X*. Отслеживайте дисперсию `Score_day` по тикеру. Если CV > 1.0, тикер нестабилен для флэта, даже при высоким `FinalScore`.

## Опционально: расширение Safety c MOEX ISS
_Все поля берутся за период Основной торговой сессии._
Если доступны поля:
- `best_ask - best_bid` (спред), `depth@5` (сумма объёма 5 уровней), `trades_per_min`.
Тогда добавьте:
```
spread_rel[d] = (best_ask - best_bid) / max(W[d], ε)
spread_pen[d] = clip((spread_rel[d] - P10(spread_rel)) / max(P90(spread_rel) - P10(spread_rel), ε), 0, 1)
thin_pen[d]   = clip(1 - depth@5[d] / P90(depth@5), 0, 1)
activity[d]   = clip(trades_per_min[d] / P90(trades_per_min), 0, 1)
Safety[d]    += 0.10*activity[d] - 0.10*spread_pen[d] - 0.10*thin_pen[d]
```
Блок опционален и не требуется для базового расчёта.

## Валидация
Бэктест на истории: доля дней, где фактическая внутридневная PnL-метрика «флэт-стратегии» > 0, должна монотонно возрастать по квинтилям `FinalScore`.