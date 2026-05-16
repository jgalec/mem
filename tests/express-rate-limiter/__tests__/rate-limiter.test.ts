import { createRateLimiter } from '../src/index';
import { TokenBucket } from '../src/token-bucket';
import { MemoryStore } from '../src/store';
import type { Request, Response } from 'express';

function mockReq(ip: string = '127.0.0.1'): Partial<Request> {
  return { ip, socket: { remoteAddress: ip } } as Partial<Request>;
}

function mockRes(): Partial<Response> {
  const headers: Record<string, string | number> = {};
  let status = 200;
  const json = jest.fn();
  return {
    status: jest.fn((code: number) => {
      status = code;
      return { json };
    }),
    setHeader: jest.fn((name: string, value: string | number) => {
      headers[name] = value;
    }),
    getHeader: jest.fn((name: string) => headers[name]),
    headersSent: false,
  } as unknown as Partial<Response>;
}

describe('TokenBucket', () => {
  test('initializes with full capacity', () => {
    const bucket = new TokenBucket(10, 2, 1000);
    expect(bucket.tokens).toBe(10);
  });

  test('tryConsume reduces tokens when available', () => {
    const bucket = new TokenBucket(5, 1, 1000);
    expect(bucket.tryConsume(1)).toBe(true);
    expect(bucket.tokens).toBeCloseTo(4, 0);
  });

  test('tryConsume fails when tokens exhausted', () => {
    const bucket = new TokenBucket(1, 1, 1000);
    expect(bucket.tryConsume(1)).toBe(true);
    expect(bucket.tryConsume(1)).toBe(false);
  });

  test('refills tokens over time', () => {
    const bucket = new TokenBucket(10, 10, 1000);
    // consume all
    for (let i = 0; i < 10; i++) bucket.tryConsume(1);
    expect(bucket.tokens).toBeCloseTo(0, 0);

    // simulate time passing
    const state = bucket.getState();
    state.lastRefill = Date.now() - 2000;
    bucket.loadState(state);
    expect(bucket.tokens).toBeGreaterThanOrEqual(9);
  });

  test('never exceeds capacity', () => {
    const bucket = new TokenBucket(5, 10, 1000);
    const state = bucket.getState();
    state.lastRefill = Date.now() - 10000;
    bucket.loadState(state);
    expect(bucket.tokens).toBeLessThanOrEqual(5);
  });

  test('timeToNextToken returns 0 when tokens available', () => {
    const bucket = new TokenBucket(10, 1, 1000);
    expect(bucket.timeToNextToken()).toBe(0);
  });

  test('timeToNextToken returns ms until refill when empty', () => {
    const bucket = new TokenBucket(1, 1, 1000);
    bucket.tryConsume(1);
    const wait = bucket.timeToNextToken();
    expect(wait).toBeGreaterThan(0);
    expect(wait).toBeLessThanOrEqual(1000);
  });
});

describe('MemoryStore', () => {
  test('get returns null for missing key', async () => {
    const store = new MemoryStore();
    expect(await store.get('missing')).toBeNull();
  });

  test('set and get roundtrip', async () => {
    const store = new MemoryStore();
    const state = { tokens: 5, lastRefill: Date.now() };
    await store.set('key1', state);
    const retrieved = await store.get('key1');
    expect(retrieved).toEqual(state);
  });

  test('delete removes key', async () => {
    const store = new MemoryStore();
    await store.set('key1', { tokens: 5, lastRefill: Date.now() });
    await store.delete('key1');
    expect(await store.get('key1')).toBeNull();
  });
});

describe('createRateLimiter middleware', () => {
  test('calls next when tokens available', () => {
    const limiter = createRateLimiter({ capacity: 100, refillRate: 10 });
    const req = mockReq();
    const res = mockRes();
    const next = jest.fn();

    limiter(req as Request, res as Response, next);
    expect(next).toHaveBeenCalled();
  });

  test('returns 429 when tokens exhausted', () => {
    const limiter = createRateLimiter({ capacity: 1, refillRate: 0.001 });
    const req = mockReq();
    const res = mockRes();
    const next = jest.fn();

    limiter(req as Request, res as Response, next);
    expect(next).toHaveBeenCalled();

    const res2 = mockRes();
    limiter(req as Request, res2 as Response, jest.fn());
    expect(res2.status).toHaveBeenCalledWith(429);
  });

  test('sets rate limit headers', () => {
    const limiter = createRateLimiter({ capacity: 50, refillRate: 5, headers: true });
    const req = mockReq();
    const res = mockRes();
    const next = jest.fn();

    limiter(req as Request, res as Response, next);
    expect(res.setHeader).toHaveBeenCalledWith('X-RateLimit-Limit', 50);
    expect(res.setHeader).toHaveBeenCalledWith('X-RateLimit-Remaining', expect.any(Number));
  });

  test('skips when skip function returns true', () => {
    const limiter = createRateLimiter({
      capacity: 1,
      refillRate: 0,
      skip: (req) => req.ip === '10.0.0.1',
    });
    const req = mockReq('10.0.0.1');
    const res = mockRes();
    const next = jest.fn();

    limiter(req as Request, res as Response, next);
    // should pass even with zero refill rate
    expect(next).toHaveBeenCalled();
  });

  test('uses custom key generator', () => {
    const limiter = createRateLimiter({
      capacity: 2,
      refillRate: 1,
      keyGenerator: (req) => req.headers?.['x-api-key'] as string || 'anon',
    });
    const req = { ...mockReq(), headers: { 'x-api-key': 'abc' } };
    const res = mockRes();
    const next = jest.fn();

    limiter(req as unknown as Request, res as Response, next);
    expect(next).toHaveBeenCalled();
  });

  test('custom message and statusCode', () => {
    const limiter = createRateLimiter({
      capacity: 1,
      refillRate: 0.001,
      message: 'Custom block',
      statusCode: 503,
    });
    const req = mockReq();
    const res = mockRes();

    limiter(req as Request, res as Response, jest.fn());
    const res2 = mockRes();
    limiter(req as Request, res2 as Response, jest.fn());

    expect(res2.status).toHaveBeenCalledWith(503);
  });
});
