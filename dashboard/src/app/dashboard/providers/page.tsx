'use client';

import React, { useState, useEffect } from 'react';
import {
  Box,
  Button,
  Card,
  CardContent,
  CardActions,
  Chip,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Typography,
  Alert,
  CircularProgress,
  Tabs,
  Tab,
} from '@mui/material';
import GridLegacy from '@mui/material/GridLegacy';
import {
  Add as AddIcon,
  CheckCircle,
  Cancel,
  Email as EmailIcon,
  CloudQueue as CloudIcon,
  Mail as MailIcon,
} from '@mui/icons-material';
import { useWorkspaces } from '../../../services/providers';
import { ProviderManagementService } from '../../../services/providerManagement';
import { ProviderConfigForm } from '../../../components/ProviderConfigForm';
import { RateLimitsConfig } from '../../../components/RateLimitsConfig';
import { UserRateLimitsConfig } from '../../../components/UserRateLimitsConfig';
import { HeaderRewriteRules } from '../../../components/HeaderRewriteRules';
import { WorkspaceProvider } from '../../../types/relay';

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

export default function ProvidersPage() {
  const { data: workspaces, error: workspacesError, isLoading, mutate } = useWorkspaces();
  const [providers, setProviders] = useState<WorkspaceProvider[]>([]);
  const [loadingProviders, setLoadingProviders] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [selectedProvider, setSelectedProvider] = useState<WorkspaceProvider | null>(null);
  const [selectedWorkspaceId, setSelectedWorkspaceId] = useState<string>('');
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [providerToDelete, setProviderToDelete] = useState<WorkspaceProvider | null>(null);
  const [settingsDialogOpen, setSettingsDialogOpen] = useState(false);
  const [settingsWorkspaceId, setSettingsWorkspaceId] = useState<string>('');
  const [tabValue, setTabValue] = useState(0);

  // Fetch all providers for all workspaces
  useEffect(() => {
    const fetchAllProviders = async () => {
      if (!workspaces || workspaces.length === 0) {
        setProviders([]);
        setLoadingProviders(false);
        return;
      }

      try {
        setLoadingProviders(true);
        const allProviders: WorkspaceProvider[] = [];
        
        for (const workspace of workspaces) {
          const workspaceProviders = await ProviderManagementService.getWorkspaceProviders(workspace.id);
          if (workspaceProviders && workspaceProviders.length > 0) {
            allProviders.push(...workspaceProviders);
          }
        }
        
        setProviders(allProviders);
      } catch (err) {
        console.error('Error fetching providers:', err);
      } finally {
        setLoadingProviders(false);
      }
    };

    fetchAllProviders();
  }, [workspaces]);

  const handleOpenDialog = (provider?: WorkspaceProvider, workspaceId?: string) => {
    if (provider) {
      setSelectedProvider(provider);
      setSelectedWorkspaceId(provider.workspace_id || '');
    } else {
      setSelectedProvider(null);
      setSelectedWorkspaceId(workspaceId || workspaces?.[0]?.id || '');
    }
    setDialogOpen(true);
  };

  const handleCloseDialog = () => {
    setDialogOpen(false);
    setSelectedProvider(null);
    setSelectedWorkspaceId('');
  };

  const handleProviderUpdated = async () => {
    // Refresh providers list
    const allProviders: WorkspaceProvider[] = [];
    if (workspaces) {
      for (const workspace of workspaces) {
        const workspaceProviders = await ProviderManagementService.getWorkspaceProviders(workspace.id);
        if (workspaceProviders && workspaceProviders.length > 0) {
          allProviders.push(...workspaceProviders);
        }
      }
    }
    setProviders(allProviders);
    handleCloseDialog();
  };

  const handleDeleteProvider = (provider: WorkspaceProvider) => {
    setProviderToDelete(provider);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!providerToDelete) return;

    try {
      await ProviderManagementService.deleteProvider(providerToDelete.id);
      await handleProviderUpdated();
      setDeleteDialogOpen(false);
      setProviderToDelete(null);
    } catch (err) {
      console.error('Error deleting provider:', err);
    }
  };

  const handleOpenSettings = (workspaceId: string) => {
    setSettingsWorkspaceId(workspaceId);
    setSettingsDialogOpen(true);
    setTabValue(0);
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

  const getWorkspaceName = (workspaceId: string) => {
    const workspace = workspaces?.find(w => w.id === workspaceId);
    return workspace?.display_name || workspace?.id || workspaceId;
  };

  if (isLoading || loadingProviders) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '400px' }}>
        <CircularProgress />
      </Box>
    );
  }

  if (workspacesError) {
    return (
      <Alert severity="error">
        Failed to load providers: {workspacesError.message || 'Unknown error'}
      </Alert>
    );
  }

  return (
    <Box>
      <Box sx={{ mb: 3, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Typography variant="h5">Email Providers</Typography>
        <Button
          variant="contained"
          startIcon={<AddIcon />}
          onClick={() => handleOpenDialog()}
        >
          Add Provider
        </Button>
      </Box>

      <GridLegacy container spacing={3}>
        {providers.map((provider) => {
          const workspace = workspaces?.find(w => w.id === provider.workspace_id);
          return (
            <GridLegacy item xs={12} md={6} lg={4} key={provider.id}>
              <Card>
                <CardContent>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'start' }}>
                    <Box>
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                        <Box sx={{ fontSize: '1.5rem' }}>{getProviderIcon(provider.type)}</Box>
                        <Typography variant="h6">
                          {provider.type.charAt(0).toUpperCase() + provider.type.slice(1)}
                        </Typography>
                      </Box>
                      <Typography variant="body2" color="textSecondary" gutterBottom>
                        {workspace?.domain || 'No domain'}
                      </Typography>
                      <Box sx={{ display: 'flex', gap: 1, mt: 1 }}>
                        <Chip
                          icon={provider.enabled ? <CheckCircle /> : <Cancel />}
                          label={provider.enabled ? 'Active' : 'Inactive'}
                          color={provider.enabled ? 'success' : 'default'}
                          size="small"
                        />
                        <Chip
                          label={`Priority ${provider.priority}`}
                          variant="outlined"
                          size="small"
                        />
                      </Box>
                    </Box>
                  </Box>

                  <Box sx={{ mt: 2 }}>
                    <Typography variant="caption" color="textSecondary">
                      {getWorkspaceName(provider.workspace_id || '')}
                    </Typography>
                  </Box>
                </CardContent>
                <CardActions sx={{ px: 2, pb: 2 }}>
                  <Button
                    onClick={() => handleOpenDialog(provider)}
                    size="small"
                    variant="outlined"
                    sx={{ 
                      textTransform: 'none',
                      fontSize: '0.75rem',
                      py: 0.5,
                      px: 1.5,
                      minWidth: 'auto'
                    }}
                  >
                    Configure
                  </Button>
                  <Button
                    onClick={() => handleOpenSettings(provider.workspace_id || '')}
                    size="small"
                    variant="outlined"
                    sx={{ 
                      textTransform: 'none',
                      fontSize: '0.75rem',
                      py: 0.5,
                      px: 1.5,
                      minWidth: 'auto'
                    }}
                  >
                    Limits
                  </Button>
                  <Button
                    onClick={() => handleDeleteProvider(provider)}
                    size="small"
                    variant="outlined"
                    color="error"
                    sx={{ 
                      textTransform: 'none',
                      fontSize: '0.75rem',
                      py: 0.5,
                      px: 1.5,
                      minWidth: 'auto'
                    }}
                  >
                    Delete
                  </Button>
                </CardActions>
              </Card>
            </GridLegacy>
          );
        })}
        
        {providers.length === 0 && (
          <GridLegacy item xs={12}>
            <Card>
              <CardContent sx={{ textAlign: 'center', py: 4 }}>
                <Typography variant="h6" color="text.secondary">
                  No providers configured
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
                  Add a provider to start sending emails
                </Typography>
                <Button
                  variant="contained"
                  startIcon={<AddIcon />}
                  sx={{ mt: 2 }}
                  onClick={() => handleOpenDialog()}
                >
                  Add Provider
                </Button>
              </CardContent>
            </Card>
          </GridLegacy>
        )}
      </GridLegacy>

      {/* Add/Edit Provider Dialog */}
      <Dialog
        open={dialogOpen}
        onClose={handleCloseDialog}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>
          {selectedProvider ? 'Edit Provider Configuration' : 'Configure Provider'}
        </DialogTitle>
        <DialogContent>
          <ProviderConfigForm
            workspaceId={selectedWorkspaceId}
            provider={selectedProvider}
            onSaved={handleProviderUpdated}
            onCancel={handleCloseDialog}
          />
        </DialogContent>
      </Dialog>

      {/* Settings Dialog */}
      <Dialog
        open={settingsDialogOpen}
        onClose={() => setSettingsDialogOpen(false)}
        maxWidth="lg"
        fullWidth
      >
        <DialogTitle>
          {getWorkspaceName(settingsWorkspaceId)} Limits & Rules
        </DialogTitle>
        <DialogContent>
          <Box sx={{ borderBottom: 1, borderColor: 'divider', mb: 2 }}>
            <Tabs value={tabValue} onChange={(e, v) => setTabValue(v)}>
              <Tab label="Rate Limits" />
              <Tab label="User Limits" />
              <Tab label="Header Rules" />
            </Tabs>
          </Box>
          
          <TabPanel value={tabValue} index={0}>
            <RateLimitsConfig workspaceId={settingsWorkspaceId} />
          </TabPanel>

          <TabPanel value={tabValue} index={1}>
            <UserRateLimitsConfig workspaceId={settingsWorkspaceId} />
          </TabPanel>

          <TabPanel value={tabValue} index={2}>
            <HeaderRewriteRules 
              providers={providers.filter(p => p.workspace_id === settingsWorkspaceId)}
              onProviderUpdated={handleProviderUpdated}
              workspaceId={settingsWorkspaceId}
            />
          </TabPanel>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setSettingsDialogOpen(false)}>Close</Button>
        </DialogActions>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <Dialog
        open={deleteDialogOpen}
        onClose={() => setDeleteDialogOpen(false)}
      >
        <DialogTitle>Confirm Delete</DialogTitle>
        <DialogContent>
          <Typography>
            Are you sure you want to delete this provider configuration?
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