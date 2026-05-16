# Express Rate Limiter

A middleware-based rate limiter for Express.js using the token bucket algorithm.

## Requirements

- Token bucket algorithm with configurable capacity and refill rate
- Pluggable storage backends (in-memory default, Redis optional)
- Standard HTTP rate limit headers (`X-RateLimit-*`, `Retry-After`)
- Configurable per-route and global limits
- TypeScript with full type safety

## Implementation Plan

### Package Structure
```
tests/express-rate-limiter/
  package.json
  tsconfig.json
  src/
    index.ts          -- main export, createRateLimiter factory
    types.ts           -- TypeScript interfaces and types
    token-bucket.ts    -- TokenBucket class (core algorithm)
    store.ts           -- Store interface + MemoryStore implementation
  __tests__/
    rate-limiter.test.ts
```

### Core Algorithm
Token bucket with:
- `capacity` — max tokens (burst allowance)
- `refillRate` — tokens per second
- `refillInterval` — refill tick interval in ms (default 1000ms)

### API
```ts
import { createRateLimiter } from 'express-rate-limiter';

app.use(createRateLimiter({
  capacity: 100,
  refillRate: 10,
  message: 'Too many requests',
  statusCode: 429,
}));
```
