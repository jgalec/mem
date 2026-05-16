import type { TokenBucketState } from './types';

export class TokenBucket {
  public readonly capacity: number;
  public readonly refillRate: number;
  public readonly refillInterval: number;

  private state: TokenBucketState;

  constructor(capacity: number, refillRate: number, refillInterval: number = 1000) {
    this.capacity = capacity;
    this.refillRate = refillRate;
    this.refillInterval = refillInterval;
    this.state = { tokens: capacity, lastRefill: Date.now() };
  }

  get tokens(): number {
    this.refill();
    return this.state.tokens;
  }

  get lastRefill(): number {
    return this.state.lastRefill;
  }

  loadState(state: TokenBucketState): void {
    this.state = { ...state };
  }

  getState(): TokenBucketState {
    return { ...this.state };
  }

  tryConsume(count: number = 1): boolean {
    this.refill();
    if (this.state.tokens >= count) {
      this.state.tokens -= count;
      return true;
    }
    return false;
  }

  timeToNextToken(): number {
    if (this.state.tokens >= 0.5) return 0;
    const tokensPerMs = this.refillRate / this.refillInterval;
    if (tokensPerMs <= 0) return Infinity;
    const needed = 1 - this.state.tokens;
    return Math.ceil(needed / tokensPerMs);
  }

  private refill(): void {
    const now = Date.now();
    const elapsed = now - this.state.lastRefill;
    if (elapsed <= 0) return;

    const tokensToAdd = (elapsed / this.refillInterval) * this.refillRate;
    const newTokens = Math.min(this.capacity, this.state.tokens + tokensToAdd);

    if (newTokens > this.state.tokens) {
      this.state.tokens = newTokens;
      this.state.lastRefill = now;
    }
  }
}
