import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

// Custom metrics for detailed analysis
const cacheHits = new Counter('cache_hits');
const cacheMisses = new Counter('cache_misses');
const writeLatency = new Trend('write_latency_ms');
const readLatency = new Trend('read_latency_ms');
const errorRate = new Rate('error_rate');
const writeCount = new Counter('write_operations');
const readCount = new Counter('read_operations');

export const options = {
    stages: [
        { duration: '30s', target: 100 }, // Ramp up to 100 users
        { duration: '1m', target: 100 },  // Stay at 100 users
        { duration: '30s', target: 0 },   // Ramp down
    ],
    thresholds: {
        http_req_duration: ['p(99)<100'],      // 99% of requests < 100ms
        'read_latency_ms': ['p(95)<10'],       // 95% of reads < 10ms (cache hits)
        'write_latency_ms': ['p(99)<100'],     // 99% of writes < 100ms
        'error_rate': ['rate<0.01'],           // Error rate < 1%
        'http_req_failed': ['rate<0.01'],      // Failed requests < 1%
    },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

// Setup function: Pre-populate database with test URLs
export function setup() {
    console.log('Setting up test data...');
    const codes = [];
    const params = {
        headers: { 'Content-Type': 'application/json' },
    };

    // Create 100 initial URLs for cache testing
    for (let i = 0; i < 100; i++) {
        const payload = JSON.stringify({
            url: `https://example.com/setup/${i}`,
        });

        const res = http.post(`${BASE_URL}/api/shorten`, payload, params);

        if (res.status === 200) {
            try {
                const data = res.json();
                codes.push(data.short_code);
            } catch (e) {
                console.error(`Failed to parse response: ${e}`);
            }
        } else {
            console.warn(`Setup request failed: ${res.status}`);
        }

        // Small delay to avoid overwhelming the service during setup
        sleep(0.01);
    }

    console.log(`Setup complete: ${codes.length} URLs created`);
    return { shortCodes: codes };
}

export default function (data) {
    const params = {
        headers: { 'Content-Type': 'application/json' },
    };

    // Read-Heavy workload: 90% reads, 10% writes
    const isWrite = Math.random() < 0.1;

    if (isWrite) {
        // Write operation: Create new short URL
        writeCount.add(1);

        const payload = JSON.stringify({
            url: `https://example.com/test/${__VU}-${__ITER}`,
        });

        const startTime = new Date().getTime();
        const shortenRes = http.post(`${BASE_URL}/api/shorten`, payload, params);
        const duration = new Date().getTime() - startTime;

        writeLatency.add(duration);

        const success = check(shortenRes, {
            'shorten status is 200': (r) => r.status === 200,
            'response has short_code': (r) => {
                try {
                    return r.json('short_code') !== undefined;
                } catch (e) {
                    return false;
                }
            },
        });

        errorRate.add(!success);

        if (!success) {
            console.error(`Write failed: ${shortenRes.status} - ${shortenRes.body}`);
        } else {
            // Add newly created code to local pool for potential future reads
            try {
                const newCode = shortenRes.json('short_code');
                if (data.shortCodes) {
                    data.shortCodes.push(newCode);
                }
            } catch (e) {
                console.error(`Failed to parse short_code: ${e}`);
            }
        }

    } else {
        // Read operation: Access existing short URL
        readCount.add(1);

        if (!data.shortCodes || data.shortCodes.length === 0) {
            console.warn('No short codes available for read operations');
            sleep(0.1);
            return;
        }

        // Pick random short code from existing pool
        const randomIndex = Math.floor(Math.random() * data.shortCodes.length);
        const shortCode = data.shortCodes[randomIndex];

        const startTime = new Date().getTime();
        const redirectRes = http.get(`${BASE_URL}/${shortCode}`, {
            redirects: 0,  // Don't follow redirects to measure 302 response time
        });
        const duration = new Date().getTime() - startTime;

        readLatency.add(duration);

        const success = check(redirectRes, {
            'redirect status is 302': (r) => r.status === 302,
            'location header present': (r) => r.headers['Location'] !== undefined,
        });

        errorRate.add(!success);

        // Estimate cache hit/miss based on latency
        // Cache hits (Redis) should be <5ms, DB queries are typically >10ms
        if (duration < 5) {
            cacheHits.add(1);
        } else {
            cacheMisses.add(1);
        }

        if (!success) {
            console.error(`Read failed: ${redirectRes.status} - ${redirectRes.body}`);
        }
    }

    // Realistic user behavior: 0-300ms think time
    sleep(Math.random() * 0.3);
}

export function teardown(data) {
    console.log('Test completed');
    console.log(`Total short codes created: ${data.shortCodes ? data.shortCodes.length : 0}`);
}
