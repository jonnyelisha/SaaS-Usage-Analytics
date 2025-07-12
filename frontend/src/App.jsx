import { useEffect, useState } from 'react';

function App() {
  const [metrics, setMetrics] = useState({ postgres_events: 0, redis_pageviews: 0 });

  useEffect(() => {
    const fetchMetrics = () => {
      fetch('http://localhost:3001/metrics')
        .then(res => res.json())
        .then(data => setMetrics(data))
        .catch(err => console.error(err));
    };

    fetchMetrics();
    const interval = setInterval(fetchMetrics, 2000);
    return () => clearInterval(interval);
  }, []);

  return (
    <div style={{ padding: '2rem' }}>
      <h1>SaaS Usage Dashboard</h1>
      <p>📊 Total Events (Postgres): {metrics.postgres_events}</p>
      <p>⚡ Real-Time Page Views (Redis): {metrics.redis_pageviews}</p>
    </div>
  );
}

export default App;
