ALTER TABLE ticker
    ADD CONSTRAINT ticker_pkey PRIMARY KEY (ticker_name, trading_session_date);
