import type { Request, Response } from 'express';

export interface RateLimiterOptions {
  capacity: number;
  refillRate: number;
  refillInterval?: number;
  keyGenerator?: (req: Request) => string;
  skip?: (req: Request) => boolean;
  message?: string;
  statusCode?: number;
  headers?: boolean;
}

export interface TokenBucketState {
  tokens: number;
  lastRefill: number;
}

export interface RateLimitInfo {
  limit: number;
  remaining: number;
  reset: number;
  retryAfter: number;
}

export type RateLimiterMiddleware = (
  req: Request,
  res: Response,
  next: () => void
) => void;

export interface Store {
  get(key: string): Promise<TokenBucketState | null>;
  set(key: string, state: TokenBucketState): Promise<void>;
  delete(key: string): Promise<void>;
  shutdown(): Promise<void>;
}
