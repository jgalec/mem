import type { Store, TokenBucketState } from './types';

export class MemoryStore implements Store {
  private buckets: Map<string, TokenBucketState> = new Map();
  private cleanupTimer: ReturnType<typeof setInterval>;

  constructor(cleanupIntervalMs: number = 60000) {
    this.cleanupTimer = setInterval(() => this.cleanup(), cleanupIntervalMs);
  }

  async get(key: string): Promise<TokenBucketState | null> {
    const state = this.buckets.get(key);
    return state ? { ...state } : null;
  }

  async set(key: string, state: TokenBucketState): Promise<void> {
    this.buckets.set(key, { ...state });
  }

  async delete(key: string): Promise<void> {
    this.buckets.delete(key);
  }

  async shutdown(): Promise<void> {
    clearInterval(this.cleanupTimer);
    this.buckets.clear();
  }

  private cleanup(): void {
    const now = Date.now();
    const staleThreshold = now - 300000; // 5 min
    for (const [key, state] of this.buckets) {
      if (state.lastRefill < staleThreshold) {
        this.buckets.delete(key);
      }
    }
  }
}
