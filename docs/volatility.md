# Волатильность — формулы

## Данные (MOEX ISS)
- **candles 1m/5m**: `open`, `high`, `low`, `close`, `volume`
- **candles 1d**: `close` (для нормализации при необходимости)

## Метрики
```
TR_t = max( high_t - low_t,
            |high_t - close_{t-1}|,
            |low_t  - close_{t-1}| )

ATR_n   = SMA_n(TR_t)
ATR%_n  = ATR_n / close_t * 100

CumVol_t = Σ_{j=1..t} volume_j
RVOL_t   = CumVol_t / avg_{d=1..N}( CumVol_{d,t} )
```

## Нормировка и индекс волатильности
```
norm_up(x_i) = (x_i - min_S x) / (max_S x - min_S x + ε)

V_i = 100 * [ 0.6*norm_up(ATR%_i) + 0.4*norm_up(RVOL_i) ]
```

## Параметры
```
ε = 1e-9
n = 14..20  (период ATR)
N = 20      (история для RVOL)
Таймфрейм: 1m или 5m
```
