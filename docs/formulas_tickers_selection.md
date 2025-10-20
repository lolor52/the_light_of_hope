# Индексы из MOEX ISS: Ликвидность, Волатильность, «Плоский» тренд

## Данные (MOEX ISS)

* **marketdata**: `BID`, `OFFER`
* **orderbook**: `bids[i].price`, `bids[i].quantity`, `asks[i].price`, `asks[i].quantity`
* **candles 1m/5m**: `open`, `high`, `low`, `close`, `volume`, `value`
* **candles 1d**: `high`, `low`, `open`, `close`
* **history (daily)**: `VALUE_d`, `VOLUME_d`
* **securities**: `LOTSIZE`, `MINSTEP`

---

## 1) Ликвидность → индекс `L_i ∈ [0,100]`

Смысл: низкий спред, высокий оборот, глубокий стакан, мелкий тик.

**Метрики**

```
Mid = (OFFER + BID) / 2
Spread_abs = OFFER − BID
Spread_rel% = (OFFER − BID) / Mid × 100
Depth_N_bid = Σ_{k=1..N} bids[k].quantity × LOTSIZE
Depth_N_ask = Σ_{k=1..N} asks[k].quantity × LOTSIZE
Depth_TOP5 = Depth_5_bid + Depth_5_ask
Turnover_per_min = value_{1m}
Tick% = MINSTEP / Mid × 100
```

> Если `quantity` уже в штуках, берите `LOTSIZE = 1`.

**Нормировка по множеству тикеров S в момент t**

```
norm↑(x_i) = (x_i − min_S x) / (max_S x − min_S x + ε)
norm↓(x_i) = 1 − norm↑(x_i)
```

**Индекс**

```
L_i = 100 × [ 0.35·norm↓(Spread_rel%_i)
            + 0.35·norm↑(Turnover_per_min_i)
            + 0.25·norm↑(Depth_TOP5_i)
            + 0.05·norm↓(Tick%_i) ]
```

---

## 2) Волатильность → индекс `V_i ∈ [0,100]`

Смысл: больше типичный ход и активнее объём относительно истории.

**Метрики (из 1m/5m свечей)**

```
TR_t = max( H_t − L_t, |H_t − C_{t−1}|, |L_t − C_{t−1}| )
ATR_n = SMA_n(TR_t)
ATR%_n = ATR_n / C_t × 100

CumVol_t = Σ_{j=1..t} volume_j
RVOL_t = CumVol_t / avg_{d=1..N}( CumVol_{d,t} )
```

**Нормировка и индекс**

```
V_i = 100 × [ 0.6·norm↑(ATR%_i) + 0.4·norm↑(RVOL_i) ]
```

---

## 3) «Плоский» тренд-фильтр → индекс `F_i ∈ [0,100]`  (больше = «плоско»)

Смысл: дни перекрываются, VWAP меняется мало, VWAP близок к середине диапазона.

**VWAP и наклон (интрадей)**

```
VWAP_t = Σ_{j=1..t} value_j / Σ_{j=1..t} volume_j
SlopeVWAP_k% = (VWAP_t − VWAP_{t−k}) / VWAP_{t−k} × 100
```

**Перекрытие и смещение (по 1d)**

```
OverlapWidth_d = max(0, min(H_d, H_{d−1}) − max(L_d, L_{d−1}))
UnionWidth_d   = max(H_d, H_{d−1}) − min(L_d, L_{d−1})
Overlap%_d     = OverlapWidth_d / (UnionWidth_d + ε) × 100

Mid_d = (H_d + L_d) / 2
VWAP_day = VALUE_d / VOLUME_d
Skew%_d = |VWAP_day − Mid_d| / (H_d − L_d + ε) × 100
```

**Нормировка и индекс**

```
F_i = 100 × [ 0.5·norm↑(Overlap%_i)
            + 0.3·norm↓(|SlopeVWAP_k%|_i)
            + 0.2·norm↓(Skew%_i) ]
```

---

## Параметры

```
ε = 1e−9  (защита от деления на ноль)
N = 20    (история для RVOL)
n = 14–20 (период ATR)
k = 10    (баров для SlopeVWAP)
N уровней стакана для глубины: 5
```
