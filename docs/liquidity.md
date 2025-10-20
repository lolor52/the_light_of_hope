# Ликвидность — формулы

## Данные (MOEX ISS)
- **marketdata**: `BID`, `OFFER`
- **orderbook**: `bids[i].price`, `bids[i].quantity`, `asks[i].price`, `asks[i].quantity`
- **candles 1m**: `value` (денежный оборот за минуту), `volume`
- **securities**: `LOTSIZE`, `MINSTEP`

## Метрики
```
Mid = (OFFER + BID) / 2
Spread_abs = OFFER - BID
Spread_rel% = (OFFER - BID) / Mid * 100

Depth_N_bid = Σ_{k=1..N} bids[k].quantity * LOTSIZE
Depth_N_ask = Σ_{k=1..N} asks[k].quantity * LOTSIZE
Depth_TOP5 = Depth_5_bid + Depth_5_ask

Turnover_per_min = value_{1m}
Tick% = MINSTEP / Mid * 100
```

> Если `quantity` уже в штуках, берите `LOTSIZE = 1`.

## Нормировка по множеству тикеров S (момент t)
```
norm_up(x_i)   = (x_i - min_S x) / (max_S x - min_S x + ε)
norm_down(x_i) = 1 - norm_up(x_i)
```

## Индекс ликвидности
```
L_i = 100 * [ 0.35*norm_down(Spread_rel%_i)
            + 0.35*norm_up(Turnover_per_min_i)
            + 0.25*norm_up(Depth_TOP5_i)
            + 0.05*norm_down(Tick%_i) ]
```

## Параметры
```
ε = 1e-9
N уровней стакана для глубины: 5
```
