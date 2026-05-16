import type { Request, Response } from 'express';
import type { RateLimiterOptions, RateLimiterMiddleware, RateLimitInfo } from './types';
import { TokenBucket } from './token-bucket';
import { MemoryStore } from './store';

const DEFAULT_OPTIONS: Partial<RateLimiterOptions> = {
  capacity: 100,
  refillRate: 10,
  refillInterval: 1000,
  statusCode: 429,
  message: 'Too many requests, please try again later.',
  headers: true,
};

function defaultKeyGenerator(req: Request): string {
  return req.ip || req.socket.remoteAddress || 'unknown';
}

function setRateLimitHeaders(res: Response, info: RateLimitInfo): void {
  res.setHeader('X-RateLimit-Limit', info.limit);
  res.setHeader('X-RateLimit-Remaining', info.remaining);
  res.setHeader('X-RateLimit-Reset', info.reset);
  if (info.retryAfter > 0) {
    res.setHeader('Retry-After', Math.ceil(info.retryAfter / 1000));
  }
}

export function createRateLimiter(userOptions: Partial<RateLimiterOptions> = {}): RateLimiterMiddleware {
  const options: RateLimiterOptions = {
    ...DEFAULT_OPTIONS,
    ...userOptions,
  } as RateLimiterOptions;

  const keyGenerator = options.keyGenerator || defaultKeyGenerator;
  const { capacity, refillRate, refillInterval = 1000, skip, message, statusCode, headers } = options;

  const buckets = new Map<string, TokenBucket>();
  const store = new MemoryStore();

  return function rateLimiter(req: Request, res: Response, next: () => void): void {
    if (skip && skip(req)) {
      next();
      return;
    }

    const key = keyGenerator(req);

    let bucket = buckets.get(key);
    if (!bucket) {
      bucket = new TokenBucket(capacity, refillRate, refillInterval);
      buckets.set(key, bucket);
    }

    const allowed = bucket.tryConsume(1);
    const remaining = Math.max(0, Math.floor(bucket.tokens));

    if (headers) {
      const resetTime = Math.ceil((bucket.lastRefill + refillInterval) / 1000);
      const info: RateLimitInfo = {
        limit: capacity,
        remaining,
        reset: resetTime,
        retryAfter: allowed ? 0 : bucket.timeToNextToken(),
      };
      setRateLimitHeaders(res, info);
    }

    if (!allowed) {
      res.status(statusCode).json({ error: message });
      return;
    }

    next();
  };
}
