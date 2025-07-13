CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(255) UNIQUE NOT NULL,
    registered_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS events (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(255) NOT NULL,
    occurred_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE if not exists api_keys (
    id SERIAL PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(user_id),
    api_key TEXT UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE if not exists purchases (
    id SERIAL PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(user_id),
    product_id TEXT NOT NULL,
    quantity INT DEFAULT 1,
    purchased_at TIMESTAMP DEFAULT NOW()
);

