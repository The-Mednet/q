import useSWR from 'swr';
import { api } from './network';
import { LoadBalancingPool, LoadBalancingSelection } from '@/types/relay';

export interface PoolsResponse {
  pools: LoadBalancingPool[];
}

export interface SelectionsResponse {
  selections: LoadBalancingSelection[];
}

// Load Balancing Pools API
export function usePools() {
  return useSWR<PoolsResponse>('/loadbalancing/pools', api.get, {
    refreshInterval: 30000,
    revalidateOnFocus: true,
  });
}

export function useSelections(limit: number = 20) {
  return useSWR<SelectionsResponse>(
    `/loadbalancing/selections?limit=${limit}`,
    api.get,
    {
      refreshInterval: 10000,
      revalidateOnFocus: true,
    }
  );
}

export async function createPool(pool: Partial<LoadBalancingPool>) {
  return api.post<LoadBalancingPool>('/loadbalancing/pools', pool);
}

export async function updatePool(id: string, pool: Partial<LoadBalancingPool>) {
  return api.put<LoadBalancingPool>(`/loadbalancing/pools/${id}`, pool);
}

export async function deletePool(id: string) {
  return api.delete(`/loadbalancing/pools/${id}`);
}

export async function togglePool(id: string, enabled: boolean) {
  return api.put<LoadBalancingPool>(`/loadbalancing/pools/${id}`, { enabled });
}