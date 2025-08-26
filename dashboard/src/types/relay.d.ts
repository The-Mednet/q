// API Response Types

export interface ApiStats {
  total: number;
  statusCounts: Array<{
    Status: 'queued' | 'processing' | 'sent' | 'failed' | 'auth_error';
    Count: number;
  }>;
}

export interface Message {
  id: string;
  from_email: string;
  from_name?: string;
  to_emails: string[];
  cc_emails?: string[];
  bcc_emails?: string[];
  subject?: string;
  status: 'queued' | 'processing' | 'sent' | 'failed' | 'auth_error';
  provider?: string;
  created_at: string;
  sent_at?: string;
  error?: string;
  html_body?: string;
  text_body?: string;
  headers?: Record<string, string>;
  workspace_id?: string;
  campaign_id?: string;
  user_id?: string;
  retry_count?: number;
}

export interface WorkspaceRateLimit {
  workspace_id: string;
  display_name: string;
  domain?: string;
  domains?: string[];
  workspace_limit: number;
  workspace_sent: number;
  workspace_remaining: number;
  workspace_reset_time?: string;
  provider_type?: 'gmail' | 'mailgun' | 'mandrill';
  users?: Record<string, {
    email: string;
    sent: number;
    limit: number;
    remaining: number;
  }>;
}

export interface RateLimitResponse {
  total_sent: number;
  workspace_count: number;
  workspaces: WorkspaceRateLimit[];
}

export interface LoadBalancingPool {
  id: string;
  name: string;
  algorithm?: 'capacity_weighted' | 'round_robin' | 'least_used' | 'random_weighted';
  strategy?: 'capacity_weighted' | 'round_robin' | 'least_used' | 'random_weighted';
  providers?: string[];
  enabled: boolean;
  domain_patterns?: string[];
  workspace_count?: number;
  selection_count?: number;
  stats?: {
    total_requests: number;
    successful_requests: number;
    failed_requests: number;
  };
  created_at?: string;
  updated_at?: string;
}

export interface LoadBalancingSelection {
  pool_id: string;
  pool_name: string;
  workspace_id: string;
  sender_email: string;
  selected_at: string;
  capacity_score: string;
}

export interface HealthCheck {
  status: 'healthy' | 'degraded' | 'unhealthy';
  timestamp: number;
  service: string;
  queue?: {
    status: string;
    error?: string;
  };
  processor?: {
    running: boolean;
    last_processed: number;
  };
}

// Provider Configuration Types

export interface WorkspaceConfig {
  id: string;
  display_name: string;
  domains: string[];
  workspace_daily_limit: number;
  per_user_daily_limit: number;
  enabled: boolean;
  providers?: ProviderConfig[];
  custom_user_limits?: Record<string, number>;
}

export interface ProviderConfig {
  id?: number;
  workspace_id: string;
  provider_type: 'gmail' | 'mailgun' | 'mandrill';
  enabled: boolean;
  config: GmailConfig | MailgunConfig | MandrillConfig;
}

export interface GmailConfig {
  service_account_file?: string;
  service_account_json?: Record<string, unknown>;
  has_credentials?: boolean;
  default_sender: string;
  enable_webhooks?: boolean;
  header_rewrite?: HeaderRewriteConfig;
}

export interface MailgunConfig {
  api_key: string;
  domain?: string;
  base_url?: string;
  region?: 'us' | 'eu';
  enabled: boolean;
  enable_webhooks?: boolean;
  tracking?: {
    opens: boolean;
    clicks: boolean;
    unsubscribe: boolean;
  };
  header_rewrite?: HeaderRewriteConfig;
}

export interface MandrillConfig {
  api_key: string;
  base_url?: string;
  enabled: boolean;
  enable_webhooks?: boolean;
  subaccount?: string;
  tracking?: {
    opens: boolean;
    clicks: boolean;
    auto_text: boolean;
    auto_html: boolean;
    inline_css: boolean;
    url_strip_qs: boolean;
  };
}

export interface HeaderRewriteConfig {
  enabled: boolean;
  rules: Array<{
    header_name: string;
    action?: 'remove' | 'replace';
    new_value?: string;
  }>;
}

// New Provider Management API Types

export interface WorkspaceProvider {
  id: number;
  provider_id: string;
  display_name: string;
  domain: string;
  type: 'gmail' | 'mailgun' | 'mandrill';
  name: string;
  enabled: boolean;
  priority: number;
  created_at: string;
  updated_at: string;
  config?: GmailProviderConfig | MailgunProviderConfig | MandrillProviderConfig;
}

export interface GmailProviderConfig {
  id: number;
  provider_id: number;
  service_account_file: string;
  default_sender: string;
  delegated_user?: string;
  scopes: string[];
  created_at: string;
  updated_at: string;
}

export interface MailgunProviderConfig {
  id: number;
  provider_id: number;
  api_key: string;
  domain: string;
  base_url: string;
  track_opens: boolean;
  track_clicks: boolean;
  created_at: string;
  updated_at: string;
}

export interface MandrillProviderConfig {
  id: number;
  provider_id: number;
  api_key: string;
  base_url: string;
  created_at: string;
  updated_at: string;
}

export interface WorkspaceRateLimitConfig {
  workspace_id: string;
  daily: number;
  hourly: number;
  per_user_daily: number;
  per_user_hourly: number;
  created_at: string;
  updated_at: string;
}

export interface WorkspaceUserRateLimit {
  id: number;
  workspace_id: string;
  user_email: string;
  daily: number;
  hourly: number;
  created_at: string;
  updated_at: string;
}

export interface ProviderHeaderRewriteRule {
  id: number;
  provider_id: number;
  header_name: string;
  action: 'add' | 'replace' | 'remove';
  value?: string;
  condition?: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateProviderRequest {
  workspace_id: string;
  type: 'gmail' | 'mailgun' | 'mandrill';
  name: string;
  enabled: boolean;
  priority: number;
  config: object;
}

export interface UpdateProviderRequest {
  name: string;
  enabled: boolean;
  priority: number;
  config?: object;
}

export interface UpdateRateLimitsRequest {
  daily: number;
  hourly: number;
  per_user_daily: number;
  per_user_hourly: number;
}

export interface CreateUserRateLimitRequest {
  workspace_id: string;
  user_email: string;
  daily: number;
  hourly: number;
}

export interface CreateHeaderRuleRequest {
  provider_id: number;
  header_name: string;
  action: 'add' | 'replace' | 'remove';
  value?: string;
  condition?: string;
  enabled: boolean;
}