const express = require('express');
const { Pool } = require('pg');
const Redis = require('ioredis');
const cors = require('cors');

const app = express();
app.use(cors());

const pgPool = new Pool({
  user: 'jonny',
  host: 'localhost',
  database: 'saas_analytics',
  password: '',
  port: 5434,
});

const redis = new Redis({ host: 'localhost', port: 6379 });

app.get('/metrics', async (req, res) => {
  const pgResult = await pgPool.query('SELECT COUNT(*) FROM events');
  const redisCount = await redis.get('counter:page_view');
  res.json({
    postgres_events: pgResult.rows[0].count,
    redis_pageviews: redisCount || 0,
  });
});

app.listen(3001, () => {
  console.log('Node backend listening on port 3001');
});