'use client';

import React, { useState } from 'react';
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
  TextField,
  Typography,
  Switch,
  FormControlLabel,
  Alert,
} from '@mui/material';
import GridLegacy from '@mui/material/GridLegacy';
import {
  Add as AddIcon,
  CheckCircle,
  Cancel,
  CloudUpload as UploadIcon,
} from '@mui/icons-material';
import { useWorkspaces, createWorkspace, updateWorkspace, deleteWorkspace } from '@/services/providers';
import { WorkspaceConfig } from '@/types/relay';

export default function ProvidersPage() {
  const { data, error, mutate } = useWorkspaces();
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [uploading, setUploading] = useState(false);
  const [uploadMessage, setUploadMessage] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [selectedWorkspace, setSelectedWorkspace] = useState<WorkspaceConfig | null>(null);
  const [hasUploadedCredentials, setHasUploadedCredentials] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [formData, setFormData] = useState<Partial<WorkspaceConfig>>({
    display_name: '',
    domains: [],
    workspace_daily_limit: 2000,
    per_user_daily_limit: 100,
    enabled: true,
  });

  const handleOpenDialog = async (workspace?: WorkspaceConfig) => {
    if (workspace) {
      setSelectedWorkspace(workspace);
      
      // For Gmail workspaces, check if credentials exist by trying to fetch workspace details
      if (workspace.gmail) {
        try {
          // Check if the workspace has credentials by looking at the service_account_file
          // or by checking if we've uploaded credentials this session
          const hasFile = workspace.gmail.service_account_file && workspace.gmail.service_account_file !== '';
          const hasDbCredentials = workspace.gmail.has_credentials;
          
          // Since the API doesn't properly return credential status, 
          // we'll track uploaded credentials in session
          const credentialKey = `creds_uploaded_${workspace.id}`;
          const uploadedInSession = sessionStorage.getItem(credentialKey) === 'true';
          
          setHasUploadedCredentials(hasFile || hasDbCredentials || uploadedInSession);
        } catch (err) {
          console.error('Error checking credentials:', err);
          setHasUploadedCredentials(false);
        }
      } else {
        setHasUploadedCredentials(false);
      }
      
      // Map API response to form data structure
      setFormData({
        display_name: workspace.display_name || '',
        domains: workspace.domain ? [workspace.domain] : workspace.domains || [],
        workspace_daily_limit: workspace.rate_limits?.workspace_daily || workspace.workspace_daily_limit || 2000,
        per_user_daily_limit: workspace.rate_limits?.per_user_daily || workspace.per_user_daily_limit || 100,
        enabled: workspace.enabled !== undefined ? workspace.enabled : true,
        provider_type: workspace.gmail ? 'gmail' : workspace.mailgun ? 'mailgun' : workspace.mandrill ? 'mandrill' : undefined,
      });
    } else {
      setSelectedWorkspace(null);
      setHasUploadedCredentials(false);
      setFormData({
        display_name: '',
        domains: [],
        workspace_daily_limit: 2000,
        per_user_daily_limit: 100,
        enabled: true,
      });
    }
    setDialogOpen(true);
    setSelectedFile(null);
    setUploadMessage(null);
  };

  const handleCloseDialog = () => {
    setDialogOpen(false);
    setSelectedWorkspace(null);
    setSaveError(null);
  };

  const handleSave = async () => {
    setSaving(true);
    setSaveError(null);
    try {
      // Transform formData to match API structure
      const payload: any = {
        display_name: formData.display_name,
        domain: formData.domains?.[0] || '', // API expects single domain
        rate_limits: {
          workspace_daily: formData.workspace_daily_limit,
          per_user_daily: formData.per_user_daily_limit,
        },
        enabled: formData.enabled,
      };

      // Add provider config based on type
      if (selectedWorkspace?.gmail || formData.provider_type === 'gmail') {
        payload.gmail = {
          enabled: formData.enabled,
          default_sender: selectedWorkspace?.gmail?.default_sender || '',
          service_account_file: selectedWorkspace?.gmail?.service_account_file || '',
        };
      } else if (selectedWorkspace?.mailgun) {
        payload.mailgun = selectedWorkspace.mailgun;
      } else if (selectedWorkspace?.mandrill) {
        payload.mandrill = selectedWorkspace.mandrill;
      }

      if (selectedWorkspace) {
        console.log('Updating workspace:', selectedWorkspace.id, payload);
        await updateWorkspace(selectedWorkspace.id, payload);
      } else {
        console.log('Creating workspace:', payload);
        await createWorkspace(payload);
      }
      mutate();
      handleCloseDialog();
    } catch (err: any) {
      console.error('Failed to save workspace:', err);
      setSaveError(err?.message || 'Failed to save workspace');
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (selectedWorkspace) {
      try {
        await deleteWorkspace(selectedWorkspace.id);
        mutate();
        setDeleteDialogOpen(false);
        setSelectedWorkspace(null);
      } catch (err) {
        console.error('Failed to delete workspace:', err);
      }
    }
  };


  if (error) {
    return <Alert severity="error">Failed to load providers: {error.message}</Alert>;
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
        {data?.map((workspace) => (
          <GridLegacy item xs={12} md={6} lg={4} key={workspace.id}>
            <Card>
              <CardContent>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'start', mb: 2 }}>
                  <Box>
                    <Typography variant="h6" gutterBottom>
                      {workspace.display_name}
                    </Typography>
                    <Typography variant="body2" color="textSecondary">
                      {workspace.domain || 'No domain'}
                    </Typography>
                  </Box>
                  <Chip
                    icon={workspace.enabled ? <CheckCircle /> : <Cancel />}
                    label={
                      workspace.gmail ? 'Gmail' : 
                      workspace.mailgun ? 'Mailgun' : 
                      workspace.mandrill ? 'Mandrill' :
                      workspace.provider_type ? workspace.provider_type.charAt(0).toUpperCase() + workspace.provider_type.slice(1) :
                      'No Provider'
                    }
                    color={workspace.enabled ? 'default' : 'error'}
                    size="small"
                    sx={{ 
                      backgroundColor: workspace.enabled ? '#f5f5f5' : undefined,
                      color: workspace.enabled ? '#666' : undefined 
                    }}
                  />
                </Box>

                <Box sx={{ mt: 2 }}>
                  <Typography variant="body2" gutterBottom>
                    Rate Limits:
                  </Typography>
                  <Typography variant="caption" display="block">
                    Workspace: {workspace.rate_limits?.workspace_daily || workspace.workspace_daily_limit || 0}/day
                  </Typography>
                  <Typography variant="caption" display="block">
                    Per User: {workspace.rate_limits?.per_user_daily || workspace.per_user_daily_limit || 0}/day
                  </Typography>
                </Box>

                {/* Show Gmail config if present */}
                {workspace.gmail && (
                  <Box sx={{ mt: 2 }}>
                    <Typography variant="body2" gutterBottom>
                      Gmail Configuration:
                    </Typography>
                    <Typography variant="caption" display="block">
                      Default Sender: {workspace.gmail.default_sender || 'Not set'}
                    </Typography>
                    <Typography variant="caption" display="block">
                      Status: {workspace.gmail.enabled ? 'Enabled' : 'Disabled'}
                    </Typography>
                    <Box sx={{ mt: 1 }}>
                      <Chip 
                        icon={workspace.gmail.service_account_file || workspace.gmail.has_credentials ? <CheckCircle /> : <Cancel />}
                        label={workspace.gmail.service_account_file || workspace.gmail.has_credentials ? 'Credentials Configured' : 'No Credentials'}
                        size="small"
                        color={workspace.gmail.service_account_file || workspace.gmail.has_credentials ? 'success' : 'warning'}
                        variant="outlined"
                      />
                    </Box>
                  </Box>
                )}
                
                {/* Show Mailgun config if present */}
                {workspace.mailgun && (
                  <Box sx={{ mt: 2 }}>
                    <Typography variant="body2" gutterBottom>
                      Mailgun Configuration:
                    </Typography>
                    <Typography variant="caption" display="block">
                      Domain: {workspace.mailgun.domain || workspace.domain}
                    </Typography>
                    <Typography variant="caption" display="block">
                      Status: {workspace.mailgun.enabled ? 'Enabled' : 'Disabled'}
                    </Typography>
                  </Box>
                )}
                
                {/* Show Mandrill config if present */}
                {workspace.mandrill && (
                  <Box sx={{ mt: 2 }}>
                    <Typography variant="body2" gutterBottom>
                      Mandrill Configuration:
                    </Typography>
                    <Typography variant="caption" display="block">
                      Status: {workspace.mandrill.enabled ? 'Enabled' : 'Disabled'}
                    </Typography>
                  </Box>
                )}
              </CardContent>
              <CardActions sx={{ px: 2, pb: 2 }}>
                <Button 
                  onClick={() => handleOpenDialog(workspace)} 
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
                  Edit
                </Button>
                <Button
                  onClick={() => {
                    setSelectedWorkspace(workspace);
                    setDeleteDialogOpen(true);
                  }}
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
        ))}
      </GridLegacy>

      {/* Add/Edit Dialog */}
      <Dialog open={dialogOpen} onClose={handleCloseDialog} maxWidth="md" fullWidth>
        <DialogTitle>
          {selectedWorkspace ? 'Edit Provider' : 'Add New Provider'}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ pt: 2 }}>
            <GridLegacy container spacing={2}>
              <GridLegacy item xs={12}>
                <TextField
                  label="Display Name"
                  fullWidth
                  value={formData.display_name}
                  onChange={(e) => setFormData({ ...formData, display_name: e.target.value })}
                />
              </GridLegacy>
              <GridLegacy item xs={12}>
                <TextField
                  label="Domains"
                  fullWidth
                  value={formData.domains?.join(', ') || ''}
                  onChange={(e) => setFormData({ 
                    ...formData, 
                    domains: e.target.value.split(',').map(s => s.trim()).filter(s => s) 
                  })}
                  helperText="Comma-separated email domains (e.g., yourdomain.com, mail.example.org)"
                />
              </GridLegacy>
              <GridLegacy item xs={6}>
                <TextField
                  label="Workspace Daily Limit"
                  type="number"
                  fullWidth
                  value={formData.workspace_daily_limit}
                  onChange={(e) => setFormData({
                    ...formData,
                    workspace_daily_limit: parseInt(e.target.value),
                  })}
                />
              </GridLegacy>
              <GridLegacy item xs={6}>
                <TextField
                  label="Per User Daily Limit"
                  type="number"
                  fullWidth
                  value={formData.per_user_daily_limit}
                  onChange={(e) => setFormData({
                    ...formData,
                    per_user_daily_limit: parseInt(e.target.value),
                  })}
                />
              </GridLegacy>

              {/* Gmail Credentials Upload - only show for Gmail providers */}
              {(selectedWorkspace?.gmail || formData.provider_type === 'gmail') && (
                <GridLegacy item xs={12}>
                  <Box sx={{ border: '1px dashed #ccc', borderRadius: 2, p: 2 }}>
                    <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
                      <Typography variant="subtitle2">
                        Gmail Service Account Credentials
                      </Typography>
                      {hasUploadedCredentials && (
                        <Chip 
                          icon={<CheckCircle />} 
                          label="Credentials Uploaded" 
                          size="small" 
                          color="success"
                          variant="outlined"
                        />
                      )}
                    </Box>
                    <Box sx={{ display: 'flex', gap: 2, alignItems: 'center', mt: 1 }}>
                      <Button
                        variant="outlined"
                        component="label"
                        startIcon={<UploadIcon />}
                        size="small"
                      >
                        {hasUploadedCredentials ? 'Replace JSON File' : 'Select JSON File'}
                        <input
                          type="file"
                          hidden
                          accept=".json"
                          onChange={(e) => {
                            const file = e.target.files?.[0];
                            if (file) {
                              setSelectedFile(file);
                              setUploadMessage(null);
                            }
                          }}
                        />
                      </Button>
                      {selectedFile && (
                        <Typography variant="body2" color="textSecondary">
                          {selectedFile.name}
                        </Typography>
                      )}
                    </Box>
                    {selectedFile && selectedWorkspace && (
                      <Button
                        variant="contained"
                        size="small"
                        sx={{ mt: 2 }}
                        onClick={async () => {
                          setUploading(true);
                          setUploadMessage(null);
                          try {
                            const formData = new FormData();
                            formData.append('credentials', selectedFile);
                            
                            const response = await fetch(`/api/workspaces/${selectedWorkspace.id}/credentials`, {
                              method: 'POST',
                              body: formData,
                            });
                            
                            if (response.ok) {
                              const result = await response.json();
                              setUploadMessage('Credentials uploaded successfully!');
                              setSelectedFile(null);
                              setHasUploadedCredentials(true);
                              // Force immediate refresh of the workspace data
                              await mutate();
                              // Also clear SWR cache to force fresh data
                              await mutate('/api/workspaces', undefined, { revalidate: true });
                            } else {
                              let errorMessage = 'Upload failed';
                              try {
                                const result = await response.json();
                                errorMessage = result.message || errorMessage;
                              } catch {
                                if (response.status === 404) {
                                  errorMessage = 'Credential upload requires MySQL database (currently using in-memory storage)';
                                } else {
                                  errorMessage = `Upload failed: ${response.statusText}`;
                                }
                              }
                              setUploadMessage(`Error: ${errorMessage}`);
                            }
                          } catch (err) {
                            setUploadMessage('Failed to upload credentials');
                          } finally {
                            setUploading(false);
                          }
                        }}
                        disabled={uploading}
                      >
                        {uploading ? 'Uploading...' : 'Upload Credentials'}
                      </Button>
                    )}
                    {uploadMessage && (
                      <Alert 
                        severity={uploadMessage.includes('Error') ? 'error' : 'success'} 
                        sx={{ mt: 2 }}
                      >
                        {uploadMessage}
                      </Alert>
                    )}
                    <Typography variant="caption" display="block" sx={{ mt: 1, color: 'text.secondary' }}>
                      Upload your Google Cloud service account JSON file. This will securely store your credentials in the database.
                    </Typography>
                  </Box>
                </GridLegacy>
              )}

              <GridLegacy item xs={12}>
                <FormControlLabel
                  control={
                    <Switch
                      checked={formData.enabled ?? true}
                      onChange={(e) => setFormData({
                        ...formData,
                        enabled: e.target.checked,
                      })}
                    />
                  }
                  label="Enabled"
                />
              </GridLegacy>
            </GridLegacy>
          </Box>
        </DialogContent>
        <DialogActions>
          {saveError && (
            <Alert severity="error" sx={{ mr: 2 }}>
              {saveError}
            </Alert>
          )}
          <Button onClick={handleCloseDialog} disabled={saving}>Cancel</Button>
          <Button 
            onClick={handleSave} 
            variant="contained"
            disabled={saving}
          >
            {saving ? 'Saving...' : (selectedWorkspace ? 'Update' : 'Create')}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <Dialog open={deleteDialogOpen} onClose={() => setDeleteDialogOpen(false)}>
        <DialogTitle>Confirm Delete</DialogTitle>
        <DialogContent>
          <Typography>
            Are you sure you want to delete the provider &quot;{selectedWorkspace?.display_name}&quot;?
            This action cannot be undone.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteDialogOpen(false)}>Cancel</Button>
          <Button onClick={handleDelete} color="error" variant="contained">
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}