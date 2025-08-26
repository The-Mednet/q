'use client';

import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  FormControlLabel,
  Switch,
  Button,
  Alert,
  Paper,
  Divider,
  Stack,
  Chip,
} from '@mui/material';
import { WorkspaceProvider } from '../types/relay';
import { ProviderManagementService } from '../services/providerManagement';

interface ProviderConfigFormProps {
  workspaceId: string;
  provider?: WorkspaceProvider | null;
  onSaved: () => void;
  onCancel: () => void;
}

interface FormData {
  name: string;
  domain: string;
  type: 'gmail' | 'mailgun' | 'mandrill';
  enabled: boolean;
  priority: number;
  config: {
    // Gmail config
    service_account_file?: string;
    service_account_json?: string;
    default_sender?: string;
    has_uploaded_credentials?: boolean;
    
    // Mailgun config
    api_key?: string;
    domain?: string;
    base_url?: string;
    track_opens?: boolean;
    track_clicks?: boolean;
    
    // Mandrill config (reuses api_key and base_url)
  };
}

const DEFAULT_GMAIL_SCOPES = [
  'https://www.googleapis.com/auth/gmail.send',
  'https://www.googleapis.com/auth/gmail.readonly'
];

const DEFAULT_MAILGUN_BASE_URL = 'https://api.mailgun.net/v3';
const DEFAULT_MANDRILL_BASE_URL = 'https://mandrillapp.com/api/1.0';

export function ProviderConfigForm({ workspaceId, provider, onSaved, onCancel }: ProviderConfigFormProps) {
  const [formData, setFormData] = useState<FormData>({
    name: '',
    domain: '',
    type: 'gmail',
    enabled: true,
    priority: 1,
    config: {},
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [scopeInput, setScopeInput] = useState('');

  useEffect(() => {
    if (provider) {
      // Populate form with existing provider data
      setFormData({
        name: provider.name || provider.display_name || '',
        domain: provider.domain || '',
        type: provider.type,
        enabled: provider.enabled,
        priority: provider.priority,
        config: provider.config ? extractConfigData(provider.config, provider.type) : {},
      });
    }
  }, [provider]);

  const extractConfigData = (config: any, type: string) => {
    switch (type) {
      case 'gmail':
        return {
          service_account_file: config.service_account_file || '',
          default_sender: config.default_sender || '',
          delegated_user: config.delegated_user || '',
          scopes: config.scopes || DEFAULT_GMAIL_SCOPES,
        };
      case 'mailgun':
        return {
          api_key: config.api_key || '',
          domain: config.domain || '',
          base_url: config.base_url || DEFAULT_MAILGUN_BASE_URL,
          track_opens: config.track_opens || false,
          track_clicks: config.track_clicks || false,
        };
      case 'mandrill':
        return {
          api_key: config.api_key || '',
          base_url: config.base_url || DEFAULT_MANDRILL_BASE_URL,
        };
      default:
        return {};
    }
  };

  const handleInputChange = (field: string, value: any) => {
    setFormData(prev => ({
      ...prev,
      [field]: value,
    }));
  };

  const handleConfigChange = (field: string, value: any) => {
    setFormData(prev => ({
      ...prev,
      config: {
        ...prev.config,
        [field]: value,
      },
    }));
  };

  const handleTypeChange = (newType: 'gmail' | 'mailgun' | 'mandrill') => {
    // Reset config when changing provider type
    let defaultConfig = {};
    
    switch (newType) {
      case 'gmail':
        defaultConfig = {
          service_account_file: '',
          default_sender: '',
          delegated_user: '',
          scopes: DEFAULT_GMAIL_SCOPES,
        };
        break;
      case 'mailgun':
        defaultConfig = {
          api_key: '',
          domain: '',
          base_url: DEFAULT_MAILGUN_BASE_URL,
          track_opens: false,
          track_clicks: false,
        };
        break;
      case 'mandrill':
        defaultConfig = {
          api_key: '',
          base_url: DEFAULT_MANDRILL_BASE_URL,
        };
        break;
    }
    
    setFormData(prev => ({
      ...prev,
      type: newType,
      config: defaultConfig,
    }));
  };

  const handleServiceAccountUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    try {
      const text = await file.text();
      const json = JSON.parse(text); // Validate it's valid JSON
      
      // Store the JSON content
      setFormData(prev => ({
        ...prev,
        config: {
          ...prev.config,
          service_account_json: text,
          has_uploaded_credentials: true,
        },
      }));
      
      setError(null);
      setSuccessMessage(`Service account JSON uploaded successfully (${file.name})`);
    } catch (err) {
      setError('Invalid JSON file. Please upload a valid service account JSON file.');
    }
  };

  const handleAddScope = () => {
    if (scopeInput.trim() && formData.config.scopes) {
      const newScopes = [...formData.config.scopes, scopeInput.trim()];
      handleConfigChange('scopes', newScopes);
      setScopeInput('');
    }
  };

  const handleRemoveScope = (scopeToRemove: string) => {
    if (formData.config.scopes) {
      const newScopes = formData.config.scopes.filter(scope => scope !== scopeToRemove);
      handleConfigChange('scopes', newScopes);
    }
  };

  const validateForm = (): string | null => {
    if (!formData.name.trim()) {
      return 'Provider name is required';
    }

    if (formData.priority < 1) {
      return 'Priority must be at least 1';
    }

    switch (formData.type) {
      case 'gmail':
        if (!formData.config.default_sender?.trim()) {
          return 'Default sender email is required for Gmail';
        }
        break;
      case 'mailgun':
        if (!formData.config.api_key?.trim()) {
          return 'API key is required for Mailgun';
        }
        break;
      case 'mandrill':
        if (!formData.config.api_key?.trim()) {
          return 'API key is required for Mandrill';
        }
        break;
    }

    return null;
  };

  const handleSubmit = async () => {
    const validationError = validateForm();
    if (validationError) {
      setError(validationError);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      // Prepare config without sensitive data for provider_config column
      const configForStorage = { ...formData.config };
      const serviceAccountJson = configForStorage.service_account_json;
      delete configForStorage.service_account_json; // Don't store in provider_config
      delete configForStorage.has_uploaded_credentials; // This is computed
      
      const requestData = {
        name: formData.name,
        domain: formData.domain,
        type: formData.type,
        enabled: formData.enabled,
        priority: formData.priority,
        config: configForStorage,
        service_account_json: serviceAccountJson, // Send separately for service_account_json column
      };

      if (provider) {
        // Update existing provider
        await ProviderManagementService.updateProvider(provider.id, requestData);
      } else {
        // Create new provider
        await ProviderManagementService.createProvider(workspaceId, requestData);
      }

      onSaved();
    } catch (err) {
      console.error('Error saving provider:', err);
      setError('Failed to save provider configuration');
    } finally {
      setLoading(false);
    }
  };

  const renderGmailConfig = () => (
    <Stack spacing={3}>
      <Box>
        <Typography variant="subtitle2" gutterBottom>
          Service Account Credentials
        </Typography>
        {successMessage ? (
          <Alert severity="success" sx={{ mb: 2 }} onClose={() => setSuccessMessage(null)}>
            {successMessage}
          </Alert>
        ) : formData.config.has_uploaded_credentials ? (
          <Alert severity="success" sx={{ mb: 2 }}>
            Service account credentials ready to be saved
          </Alert>
        ) : provider?.config?.has_uploaded_credentials ? (
          <Alert severity="success" sx={{ mb: 2 }}>
            Service account credentials are stored in the database
          </Alert>
        ) : (
          <Alert severity="info" sx={{ mb: 2 }}>
            Upload a service account JSON file or specify a file path
          </Alert>
        )}
        
        <Stack spacing={2}>
          <Button
            variant="outlined"
            component="label"
            fullWidth
          >
            Upload Service Account JSON
            <input
              type="file"
              hidden
              accept=".json,application/json"
              onChange={handleServiceAccountUpload}
            />
          </Button>
          
          <Divider>OR</Divider>
          
          <TextField
            label="Service Account File Path"
            value={formData.config.service_account_file || ''}
            onChange={(e) => handleConfigChange('service_account_file', e.target.value)}
            fullWidth
            helperText="Path to the service account JSON file on server (e.g., credentials/service-account.json)"
          />
        </Stack>
      </Box>
      
      <TextField
        label="Default Sender Email"
        value={formData.config.default_sender || ''}
        onChange={(e) => handleConfigChange('default_sender', e.target.value)}
        fullWidth
        required
        type="email"
        helperText="Default email address for sending emails (e.g., noreply@yourdomain.com)"
      />
    </Stack>
  );

  const renderMailgunConfig = () => (
    <Stack spacing={3}>
      <TextField
        label="API Key"
        value={formData.config.api_key || ''}
        onChange={(e) => handleConfigChange('api_key', e.target.value)}
        fullWidth
        required
        type="password"
        helperText="Your Mailgun API key"
      />
      
      <TextField
        label="Base URL"
        value={formData.config.base_url || ''}
        onChange={(e) => handleConfigChange('base_url', e.target.value)}
        fullWidth
        helperText="Mailgun API base URL (e.g., https://api.mailgun.net/v3)"
      />
    </Stack>
  );

  const renderMandrillConfig = () => (
    <Stack spacing={3}>
      <TextField
        label="API Key"
        value={formData.config.api_key || ''}
        onChange={(e) => handleConfigChange('api_key', e.target.value)}
        fullWidth
        required
        type="password"
        helperText="Your Mandrill API key"
      />
      
      <TextField
        label="Base URL"
        value={formData.config.base_url || ''}
        onChange={(e) => handleConfigChange('base_url', e.target.value)}
        fullWidth
        helperText="Mandrill API base URL"
      />
    </Stack>
  );

  const renderProviderConfig = () => {
    switch (formData.type) {
      case 'gmail':
        return renderGmailConfig();
      case 'mailgun':
        return renderMailgunConfig();
      case 'mandrill':
        return renderMandrillConfig();
      default:
        return null;
    }
  };

  return (
    <Box sx={{ width: '100%' }}>
      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      <Stack spacing={2.5} sx={{ width: '100%' }}>
        <TextField
          label="Provider Name"
          value={formData.name}
          onChange={(e) => handleInputChange('name', e.target.value)}
          fullWidth
          required
          helperText="A descriptive name for this provider configuration"
        />

        <TextField
          label="Domain"
          value={formData.domain}
          onChange={(e) => handleInputChange('domain', e.target.value)}
          fullWidth
          required
          helperText="The domain this provider will handle (e.g., yourdomain.com)"
        />

        <FormControl fullWidth>
          <InputLabel id="provider-type-label">Provider Type</InputLabel>
          <Select
            labelId="provider-type-label"
            label="Provider Type"
            value={formData.type}
            onChange={(e) => handleTypeChange(e.target.value as 'gmail' | 'mailgun' | 'mandrill')}
            disabled={!!provider} // Don't allow changing type for existing providers
          >
            <MenuItem value="gmail">Gmail</MenuItem>
            <MenuItem value="mailgun">Mailgun</MenuItem>
            <MenuItem value="mandrill">Mandrill</MenuItem>
          </Select>
        </FormControl>

        <TextField
          label="Priority"
          type="number"
          value={formData.priority}
          onChange={(e) => handleInputChange('priority', parseInt(e.target.value) || 1)}
          fullWidth
          inputProps={{ min: 1 }}
          helperText="Lower numbers have higher priority (1 = highest)"
        />

        <FormControlLabel
          control={
            <Switch
              checked={formData.enabled}
              onChange={(e) => handleInputChange('enabled', e.target.checked)}
            />
          }
          label="Enabled"
        />

        <Divider />

        <Typography variant="h6">
          {formData.type.charAt(0).toUpperCase() + formData.type.slice(1)} Configuration
        </Typography>

        {renderProviderConfig()}
      </Stack>

      <Box display="flex" justifyContent="flex-end" gap={2} mt={3}>
        <Button onClick={onCancel} disabled={loading}>
          Cancel
        </Button>
        <Button
          onClick={handleSubmit}
          variant="contained"
          disabled={loading}
        >
          {loading ? 'Saving...' : provider ? 'Update Provider' : 'Create Provider'}
        </Button>
      </Box>
    </Box>
  );
}