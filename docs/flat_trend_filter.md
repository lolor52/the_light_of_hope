# «Плоский» тренд-фильтр — формулы

## Данные (MOEX ISS)
- **candles 1m**: `value`, `volume`
- **candles 1d**: `high`, `low`
- **history (daily)**: `VALUE_d`, `VOLUME_d`

## VWAP и наклон (интрадей)
```
VWAP_t       = Σ_{j=1..t} value_j / Σ_{j=1..t} volume_j
SlopeVWAP_k% = (VWAP_t - VWAP_{t-k}) / VWAP_{t-k} * 100
```

## Перекрытие и смещение (по дневным свечам)
```
OverlapWidth_d = max( 0, min(high_d, high_{d-1}) - max(low_d, low_{d-1}) )
UnionWidth_d   = max( high_d, high_{d-1} ) - min( low_d, low_{d-1} )
Overlap%_d     = OverlapWidth_d / (UnionWidth_d + ε) * 100

Mid_d     = (high_d + low_d) / 2
VWAP_day  = VALUE_d / VOLUME_d
Skew%_d   = |VWAP_day - Mid_d| / (high_d - low_d + ε) * 100
```

## Нормировка и индекс «плоскости»
```
norm_up(x_i)   = (x_i - min_S x) / (max_S x - min_S x + ε)
norm_down(x_i) = 1 - norm_up(x_i)

F_i = 100 * [ 0.5*norm_up(Overlap%_i)
            + 0.3*norm_down(|SlopeVWAP_k%|_i)
            + 0.2*norm_down(Skew%_i) ]
```

## Параметры
```
ε = 1e-9
k = 10     (баров для SlopeVWAP)
```
