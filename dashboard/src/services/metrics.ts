import useSWR from 'swr';
import { api } from './network';

// Response types for metrics API
export interface StatsResponse {
  total_messages: number;
  messages_queued: number;
  messages_processing: number;
  messages_sent: number;
  messages_failed: number;
  messages_today: number;
  success_rate: number;
  hourly_stats: HourlyStat[];
  provider_stats: ProviderStat[];
}

export interface HourlyStat {
  hour: string;
  sent: number;
  failed: number;
  queued: number;
  avg_processing_time: number;
}

export interface ProviderStat {
  provider: string;
  sent: number;
  failed: number;
}

export interface RateLimitsResponse {
  workspace_limits: WorkspaceLimit[];
  user_limits: UserLimit[];
}

export interface WorkspaceLimit {
  workspace_id: string;
  used: number;
  limit: number;
  reset_at: string;
}

export interface UserLimit {
  email: string;
  used: number;
  limit: number;
  reset_at: string;
}

export interface HealthResponse {
  healthy: boolean;
  provider_status: ProviderHealth[];
  errors?: string[];
}

export interface ProviderHealth {
  name: string;
  healthy: boolean;
  error?: string;
}

// Metrics API hooks
export function useStats() {
  return useSWR<StatsResponse>('/stats', api.get, {
    refreshInterval: 10000,
    revalidateOnFocus: true,
  });
}

export function useRateLimits() {
  return useSWR<RateLimitsResponse>('/rate-limits', api.get, {
    refreshInterval: 60000,
    revalidateOnFocus: true,
  });
}

export function useHealth() {
  return useSWR<HealthResponse>('/health', api.get, {
    refreshInterval: 30000,
    revalidateOnFocus: true,
  });
}

// Legacy hooks for compatibility
export function useRateLimit() {
  return useSWR('/rate-limit', api.get, {
    refreshInterval: 30000,
    revalidateOnFocus: true,
  });
}

// Manual queue processing
export async function processQueue() {
  return api.post('/process-queue');
}