# Intraday Readiness — Spec (.md)

Цель: оценить, насколько тикер оптимален для интрадейной торговли **на предстоящей сессии**.

**Источник данных:** брать значения из таблицы `Ticker` за 5 последних торговых сессий: поля `VAH`, `VAL`, `VWAP`, `Liquidity`, `Volatility`, `FlatTrendFilter`. Обозначим их: `VAH[d]`, `VAL[d]`, `VWAP[d]`, `L[d]`, `V[d]`, `F[d]`, где `d=1..5`, `5` — последняя сессия.

Параметры по умолчанию:
```
w = [0.03, 0.07, 0.20, 0.30, 0.40]      # веса свежести для d=1..5
ε = 1e-9
TH_TREND = 60                           # порог выбора режима
```

---

## 1) Прогноз «режима» следующей сессии (тренд или диапазон)

**Расчёты (используем сессии d=4 и d=5):**
```
ΔVWAP% = (VWAP[5] - VWAP[4]) / VWAP[4] * 100

Breakout = 1, если (VWAP[5] > VAH[4]) или (VWAP[5] < VAL[4]), иначе 0

OverlapWidth = max(0, min(VAH[5], VAH[4]) - max(VAL[5], VAL[4]))
UnionWidth   = max(VAH[5], VAH[4]) - min(VAL[5], VAL[4])
Overlap%     = 100 * OverlapWidth / (UnionWidth + ε)

TrendScore = 0.4*(100 - F[5]) + 0.3*(100 - Overlap%) + 0.2*abs(ΔVWAP%) + 0.1*(100*Breakout)

Regime = "Trend"    , если TrendScore ≥ TH_TREND
Regime = "Range"    , если TrendScore < TH_TREND
```
Смысл: мало перекрытия + сдвиг VWAP + пробой + низкая «плоскость» (F) ⇒ тренд.

---

## 2) Индексы пригодности по стилям (взвешенные 5-дневные)

**Взвешенные средние:**
```
L_w = Σ_{d=1..5} w[d]*L[d]
V_w = Σ_{d=1..5} w[d]*V[d]
F_w = Σ_{d=1..5} w[d]*F[d]
```

**Штраф за нестабильность (разброс за 5 дней):**
```
R_L = max(L[d]) - min(L[d])
R_V = max(V[d]) - min(V[d])
R_F = max(F[d]) - min(F[d])
Penalty = 0.3 * (R_L + R_V + R_F) / 3
```

### 2.1) Диапазон (mean‑reversion вокруг VWAP)
Нужно: ликвидно, плоско, волатильность средняя.
```
V_mid = max(0, 100 - 2*abs(V_w - 50))
Score_MR_next = clamp( 0.5*L_w + 0.3*F_w + 0.2*V_mid - Penalty , 0, 100 )
```

### 2.2) Тренд (движение «в одну сторону»)
Нужно: ликвидно, волатильно, не плоско.
```
Score_MO_next = clamp( 0.5*L_w + 0.35*V_w + 0.15*(100 - F_w) - Penalty , 0, 100 )
```

**Функция:**
```
clamp(x, a, b) = min( max(x, a), b )
```

---

## 3) Итог на предстоящую сессию
```
если Regime == "Trend":   FinalScore = Score_MO_next
если Regime == "Range":   FinalScore = Score_MR_next
```
Интерпретация:
```
FinalScore ≥ 80  → отлично
70 ≤ FinalScore < 80 → подходит
< 70 → осторожно/ниже приоритета
```

---

## Примечания по данным
- Для всех расчётов брать **только** значения из таблицы `Ticker`:
  - `VAH, VAL, VWAP, Liquidity, Volatility, FlatTrendFilter` за 5 сессий.
- Никакие другие источники не требуются.
- Масштабы `Liquidity / Volatility / FlatTrendFilter` предполагаются в диапазоне 0–100.
- Порог `TH_TREND`, веса `w` и коэффициенты можно калибровать на истории.
- Все деления защищать `ε` от нуля.
