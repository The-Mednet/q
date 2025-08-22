import useSWR from 'swr';
import { api } from './network';
import { WorkspaceConfig, ProviderConfig } from '@/types/relay';

// Workspace/Provider CRUD operations

export function useWorkspaces() {
  return useSWR<WorkspaceConfig[]>('/api/workspaces', api.get, {
    revalidateOnFocus: true,
    revalidateOnMount: true,
    refreshInterval: 5000, // Refresh every 5 seconds
  });
}

export function useWorkspace(id: string | null) {
  return useSWR<WorkspaceConfig>(
    id ? `/api/workspaces/${id}` : null,
    api.get
  );
}

export async function createWorkspace(workspace: Partial<WorkspaceConfig>) {
  return api.post<WorkspaceConfig>('/api/workspaces', workspace);
}

export async function updateWorkspace(id: string, workspace: Partial<WorkspaceConfig>) {
  return api.put<WorkspaceConfig>(`/api/workspaces/${id}`, workspace);
}

export async function deleteWorkspace(id: string) {
  return api.delete(`/api/workspaces/${id}`);
}

export async function testWorkspace(id: string) {
  return api.post<{ success: boolean; message: string }>(`/api/workspaces/${id}/test`);
}

// Provider operations
export function useProviders(workspaceId: string | null) {
  return useSWR<ProviderConfig[]>(
    workspaceId ? `/api/workspaces/${workspaceId}/providers` : null,
    api.get
  );
}

export async function addProvider(workspaceId: string, provider: Partial<ProviderConfig>) {
  return api.post<ProviderConfig>(`/api/workspaces/${workspaceId}/providers`, provider);
}

export async function updateProvider(id: number, provider: Partial<ProviderConfig>) {
  return api.put<ProviderConfig>(`/api/providers/${id}`, provider);
}

export async function deleteProvider(id: number) {
  return api.delete(`/api/providers/${id}`);
}

export async function testProvider(id: number) {
  return api.post<{ success: boolean; message: string }>(`/api/providers/${id}/test`);
}

export async function rotateProviderKey(id: number) {
  return api.post<{ success: boolean; new_key?: string }>(`/api/providers/${id}/rotate-key`);
}

// Helper to validate provider configuration
export function validateProviderConfig(
  type: 'gmail' | 'mailgun' | 'mandrill',
  config: Record<string, unknown>
): { valid: boolean; errors: string[] } {
  const errors: string[] = [];

  switch (type) {
    case 'gmail':
      if (!config.default_sender) {
        errors.push('Default sender email is required');
      }
      if (!config.service_account_json && !config.service_account_file) {
        errors.push('Service account credentials are required');
      }
      break;

    case 'mailgun':
      if (!config.api_key) {
        errors.push('API key is required');
      }
      if (!config.domain && !config.base_url) {
        errors.push('Domain or base URL is required');
      }
      break;

    case 'mandrill':
      if (!config.api_key) {
        errors.push('API key is required');
      }
      break;
  }

  return {
    valid: errors.length === 0,
    errors,
  };
}