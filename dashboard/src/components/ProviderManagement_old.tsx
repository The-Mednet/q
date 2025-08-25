'use client';

import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Tabs,
  Tab,
  Card,
  CardContent,
  Alert,
  CircularProgress,
  Button,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  FormControlLabel,
  Switch,
  IconButton,
  Tooltip,
  Chip,
  Stack,
} from '@mui/material';
import {
  Add as AddIcon,
  Edit as EditIcon,
  Delete as DeleteIcon,
  Settings as SettingsIcon,
  CheckCircle as CheckCircleIcon,
  Error as ErrorIcon,
} from '@mui/icons-material';
import { WorkspaceProvider } from '../types/relay';
import { ProviderManagementService } from '../services/providerManagement';
import { ProviderConfigForm } from './ProviderConfigForm';
import { RateLimitsConfig } from './RateLimitsConfig';
import { UserRateLimitsConfig } from './UserRateLimitsConfig';
import { HeaderRewriteRules } from './HeaderRewriteRules';

interface TabPanelProps {
  children?: React.ReactNode;
  index: number;
  value: number;
}

function TabPanel({ children, value, index }: TabPanelProps) {
  return (
    <div
      role="tabpanel"
      hidden={value !== index}
      id={`provider-tabpanel-${index}`}
      aria-labelledby={`provider-tab-${index}`}
    >
      {value === index && <Box sx={{ p: 3 }}>{children}</Box>}
    </div>
  );
}

interface ProviderManagementProps {
  workspaceId: string;
  workspaceName?: string;
}

export function ProviderManagement({ workspaceId, workspaceName }: ProviderManagementProps) {
  const [tabValue, setTabValue] = useState(0);
  const [providers, setProviders] = useState<WorkspaceProvider[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [editingProvider, setEditingProvider] = useState<WorkspaceProvider | null>(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [providerToDelete, setProviderToDelete] = useState<WorkspaceProvider | null>(null);

  const fetchProviders = async () => {
    try {
      setLoading(true);
      setError(null);
      const fetchedProviders = await ProviderManagementService.getWorkspaceProviders(workspaceId);
      setProviders(fetchedProviders || []);
    } catch (err) {
      console.error('Error fetching providers:', err);
      setError('Failed to fetch providers');
      setProviders([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchProviders();
  }, [workspaceId]);

  const handleTabChange = (event: React.SyntheticEvent, newValue: number) => {
    setTabValue(newValue);
  };

  const handleCreateProvider = () => {
    setCreateDialogOpen(true);
  };

  const handleEditProvider = (provider: WorkspaceProvider) => {
    setEditingProvider(provider);
  };

  const handleDeleteProvider = (provider: WorkspaceProvider) => {
    setProviderToDelete(provider);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!providerToDelete) return;

    try {
      await ProviderManagementService.deleteProvider(providerToDelete.id);
      await fetchProviders();
      setDeleteDialogOpen(false);
      setProviderToDelete(null);
    } catch (err) {
      console.error('Error deleting provider:', err);
      setError('Failed to delete provider');
    }
  };

  const handleProviderUpdated = async () => {
    await fetchProviders();
    setCreateDialogOpen(false);
    setEditingProvider(null);
  };

  const getProviderIcon = (type: string) => {
    switch (type) {
      case 'gmail':
        return '📬';
      case 'mailgun':
        return '📮';
      case 'mandrill':
        return '🐵';
      default:
        return '📧';
    }
  };

  const getProviderStatusColor = (enabled: boolean) => {
    return enabled ? 'success' : 'default';
  };

  if (loading && providers.length === 0) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight={400}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
        <Typography variant="h4" component="h1">
          {workspaceName || 'Provider'} Settings
        </Typography>
        <Button
          variant="contained"
          startIcon={<AddIcon />}
          onClick={handleCreateProvider}
        >
          Add Provider
        </Button>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      <Box sx={{ borderBottom: 1, borderColor: 'divider', mb: 2 }}>
        <Tabs value={tabValue} onChange={handleTabChange} aria-label="provider configuration tabs">
          <Tab label="Configuration" />
          <Tab label="Rate Limits" />
          <Tab label="User Limits" />
          <Tab label="Header Rules" />
        </Tabs>
      </Box>

      <TabPanel value={tabValue} index={0}>
        <Box>
          {!providers || providers.length === 0 ? (
            <Card>
              <CardContent sx={{ textAlign: 'center', py: 4 }}>
                <Typography variant="h6" color="text.secondary">
                  No providers configured
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
                  Add your first email provider to get started
                </Typography>
                <Button
                  variant="contained"
                  startIcon={<AddIcon />}
                  sx={{ mt: 2 }}
                  onClick={handleCreateProvider}
                >
                  Add Provider
                </Button>
              </CardContent>
            </Card>
          ) : (
            <Stack spacing={2}>
              {providers.map((provider) => (
                <Card key={provider.id}>
                  <CardContent>
                    <Box display="flex" justifyContent="space-between" alignItems="start">
                      <Box display="flex" alignItems="start" gap={2}>
                        <Box fontSize="2rem">{getProviderIcon(provider.type)}</Box>
                        <Box>
                          <Typography variant="h6" component="div">
                            {provider.name}
                          </Typography>
                          <Typography variant="body2" color="text.secondary">
                            Type: {provider.type}
                          </Typography>
                          <Typography variant="body2" color="text.secondary">
                            Priority: {provider.priority}
                          </Typography>
                          <Box mt={1}>
                            <Chip
                              label={provider.enabled ? 'Enabled' : 'Disabled'}
                              color={getProviderStatusColor(provider.enabled)}
                              size="small"
                            />
                          </Box>
                        </Box>
                      </Box>
                      <Box display="flex" gap={1}>
                        <Button
                          size="small"
                          variant="outlined"
                          onClick={() => handleEditProvider(provider)}
                        >
                          Edit
                        </Button>
                        <Button
                          size="small"
                          variant="outlined"
                          color="error"
                          onClick={() => handleDeleteProvider(provider)}
                        >
                          Delete
                        </Button>
                      </Box>
                    </Box>
                    
                    {provider.config && (
                      <Box mt={2} p={2} bgcolor="grey.50" borderRadius={1}>
                        <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                          Configuration
                        </Typography>
                        <pre style={{ fontSize: '0.75rem', margin: 0, whiteSpace: 'pre-wrap' }}>
                          {JSON.stringify(provider.config, null, 2)}
                        </pre>
                      </Box>
                    )}
                  </CardContent>
                </Card>
              ))}
            </Stack>
          )}
        </Box>
      </TabPanel>

      <TabPanel value={tabValue} index={1}>
        <RateLimitsConfig workspaceId={workspaceId} />
      </TabPanel>

      <TabPanel value={tabValue} index={2}>
        <UserRateLimitsConfig workspaceId={workspaceId} />
      </TabPanel>

      <TabPanel value={tabValue} index={3}>
        <HeaderRewriteRules 
          providers={providers}
          onProviderUpdated={fetchProviders}
          workspaceId={workspaceId}
        />
      </TabPanel>

      {/* Create/Edit Provider Dialog */}
      <Dialog
        open={createDialogOpen || editingProvider !== null}
        onClose={() => {
          setCreateDialogOpen(false);
          setEditingProvider(null);
        }}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>
          {editingProvider ? 'Edit Provider' : 'Create Provider'}
        </DialogTitle>
        <DialogContent>
          <ProviderConfigForm
            workspaceId={workspaceId}
            provider={editingProvider}
            onSaved={handleProviderUpdated}
            onCancel={() => {
              setCreateDialogOpen(false);
              setEditingProvider(null);
            }}
          />
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <Dialog
        open={deleteDialogOpen}
        onClose={() => setDeleteDialogOpen(false)}
      >
        <DialogTitle>Confirm Delete</DialogTitle>
        <DialogContent>
          <Typography>
            Are you sure you want to delete the provider "{providerToDelete?.name}"?
            This action cannot be undone.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteDialogOpen(false)}>Cancel</Button>
          <Button onClick={handleDeleteConfirm} color="error" variant="contained">
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}