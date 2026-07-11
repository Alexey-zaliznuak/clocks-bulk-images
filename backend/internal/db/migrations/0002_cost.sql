-- Per-task cost of the OpenRouter video generation, in USD.
-- RUB is derived on the fly from a live exchange rate, so it is not stored.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0;
