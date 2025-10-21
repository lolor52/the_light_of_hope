GRANT UPDATE, DELETE ON TABLE ticker TO invest_intraday_app;

GRANT UPDATE (
    trading_session_date,
    trading_session_active,
    ticker_name,
    secid,
    boardid,
    vwap,
    val,
    vah,
    liquidity,
    volatility,
    flat_trend_filter
) ON TABLE ticker TO invest_intraday_app;
