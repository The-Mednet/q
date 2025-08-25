import { api } from './network';
import {
  WorkspaceProvider,
  WorkspaceRateLimitConfig,
  WorkspaceUserRateLimit,
  ProviderHeaderRewriteRule,
  CreateProviderRequest,
  UpdateProviderRequest,
  UpdateRateLimitsRequest,
  CreateUserRateLimitRequest,
  CreateHeaderRuleRequest,
} from '../types/relay';

export class ProviderManagementService {
  // Provider CRUD operations
  static async getWorkspaceProviders(workspaceId: string): Promise<WorkspaceProvider[]> {
    return await api.get<WorkspaceProvider[]>(`/api/workspaces/${workspaceId}/providers`);
  }

  static async getProvider(providerId: number): Promise<WorkspaceProvider> {
    return await api.get<WorkspaceProvider>(`/api/providers/${providerId}`);
  }

  static async createProvider(workspaceId: string, providerData: Omit<CreateProviderRequest, 'workspace_id'>): Promise<WorkspaceProvider> {
    return await api.post<WorkspaceProvider>(`/api/workspaces/${workspaceId}/providers`, {
      ...providerData,
      workspace_id: workspaceId,
    });
  }

  static async updateProvider(providerId: number, providerData: UpdateProviderRequest): Promise<WorkspaceProvider> {
    return await api.put<WorkspaceProvider>(`/api/providers/${providerId}`, providerData);
  }

  static async deleteProvider(providerId: number): Promise<void> {
    await api.delete(`/api/providers/${providerId}`);
  }

  // Rate limits operations
  static async getWorkspaceRateLimits(workspaceId: string): Promise<WorkspaceRateLimitConfig> {
    return await api.get<WorkspaceRateLimitConfig>(`/api/workspaces/${workspaceId}/rate-limits`);
  }

  static async updateWorkspaceRateLimits(workspaceId: string, rateLimits: UpdateRateLimitsRequest): Promise<WorkspaceRateLimitConfig> {
    return await api.put<WorkspaceRateLimitConfig>(`/api/workspaces/${workspaceId}/rate-limits`, rateLimits);
  }

  // User rate limits operations
  static async getWorkspaceUserRateLimits(workspaceId: string): Promise<WorkspaceUserRateLimit[]> {
    return await api.get<WorkspaceUserRateLimit[]>(`/api/workspaces/${workspaceId}/user-rate-limits`);
  }

  static async createUserRateLimit(workspaceId: string, userRateLimit: Omit<CreateUserRateLimitRequest, 'workspace_id'>): Promise<WorkspaceUserRateLimit> {
    return await api.post<WorkspaceUserRateLimit>(`/api/workspaces/${workspaceId}/user-rate-limits`, {
      ...userRateLimit,
      workspace_id: workspaceId,
    });
  }

  static async deleteUserRateLimit(userRateLimitId: number): Promise<void> {
    await api.delete(`/api/user-rate-limits/${userRateLimitId}`);
  }

  // Header rewrite rules operations
  static async getProviderHeaderRules(providerId: number): Promise<ProviderHeaderRewriteRule[]> {
    return await api.get<ProviderHeaderRewriteRule[]>(`/api/providers/${providerId}/header-rules`);
  }

  static async createHeaderRule(providerId: number, headerRule: Omit<CreateHeaderRuleRequest, 'provider_id'>): Promise<ProviderHeaderRewriteRule> {
    return await api.post<ProviderHeaderRewriteRule>(`/api/providers/${providerId}/header-rules`, {
      ...headerRule,
      provider_id: providerId,
    });
  }

  static async deleteHeaderRule(headerRuleId: number): Promise<void> {
    await api.delete(`/api/header-rules/${headerRuleId}`);
  }
}