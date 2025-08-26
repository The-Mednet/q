'use client';

import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Card,
  CardContent,
  Button,
  Alert,
  CircularProgress,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  IconButton,
  Tooltip,
  Stack,
  Chip,
  FormControlLabel,
  Switch,
  Accordion,
  AccordionSummary,
  AccordionDetails,
} from '@mui/material';
import {
  Add as AddIcon,
  Delete as DeleteIcon,
  Refresh as RefreshIcon,
  Rule as RuleIcon,
  ExpandMore as ExpandMoreIcon,
} from '@mui/icons-material';
import { WorkspaceProvider, ProviderHeaderRewriteRule } from '../types/relay';
import { ProviderManagementService } from '../services/providerManagement';

interface HeaderRewriteRulesProps {
  providers: WorkspaceProvider[];
  onProviderUpdated: () => void;
  workspaceId?: string;  // If provided, show only for this workspace's providers
  onClose?: () => void;
}

export function HeaderRewriteRules({ providers, onProviderUpdated, workspaceId, onClose }: HeaderRewriteRulesProps) {
  const [headerRulesByProvider, setHeaderRulesByProvider] = useState<Record<number, ProviderHeaderRewriteRule[]>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [ruleToDelete, setRuleToDelete] = useState<ProviderHeaderRewriteRule | null>(null);
  const [selectedProviderId, setSelectedProviderId] = useState<number | null>(null);
  const [formData, setFormData] = useState({
    provider_id: 0,
    header_name: '',
    action: 'add' as 'add' | 'replace' | 'remove',
    value: '',
    condition: '',
    enabled: true,
  });
  const [creating, setCreating] = useState(false);

  const fetchHeaderRules = async () => {
    try {
      setLoading(true);
      setError(null);
      
      const rulesByProvider: Record<number, ProviderHeaderRewriteRule[]> = {};
      
      // Fetch header rules for each provider
      await Promise.all(
        providers.map(async (provider) => {
          try {
            const rules = await ProviderManagementService.getProviderHeaderRules(provider.id);
            rulesByProvider[provider.id] = rules;
          } catch (err) {
            console.error(`Error fetching rules for provider ${provider.id}:`, err);
            rulesByProvider[provider.id] = [];
          }
        })
      );
      
      setHeaderRulesByProvider(rulesByProvider);
    } catch (err) {
      console.error('Error fetching header rules:', err);
      setError('Failed to fetch header rules');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (providers.length > 0) {
      fetchHeaderRules();
    }
  }, [providers]);

  const handleCreateDialogOpen = (providerId?: number) => {
    setFormData({
      provider_id: providerId || (providers.length > 0 ? providers[0].id : 0),
      header_name: '',
      action: 'add',
      value: '',
      condition: '',
      enabled: true,
    });
    setCreateDialogOpen(true);
  };

  const handleCreateDialogClose = () => {
    setCreateDialogOpen(false);
    setFormData({
      provider_id: 0,
      header_name: '',
      action: 'add',
      value: '',
      condition: '',
      enabled: true,
    });
  };

  const handleInputChange = (field: string, value: any) => {
    setFormData(prev => ({
      ...prev,
      [field]: value,
    }));
  };

  const validateForm = (): string | null => {
    if (formData.provider_id === 0) {
      return 'Please select a provider';
    }
    
    if (!formData.header_name.trim()) {
      return 'Header name is required';
    }

    if ((formData.action === 'add' || formData.action === 'replace') && !formData.value.trim()) {
      return 'Value is required for add and replace actions';
    }

    return null;
  };

  const handleCreate = async () => {
    const validationError = validateForm();
    if (validationError) {
      setError(validationError);
      return;
    }

    setCreating(true);
    setError(null);

    try {
      await ProviderManagementService.createHeaderRule(formData.provider_id, {
        header_name: formData.header_name,
        action: formData.action,
        value: formData.value || undefined,
        condition: formData.condition || undefined,
        enabled: formData.enabled,
      });
      await fetchHeaderRules();
      handleCreateDialogClose();
    } catch (err) {
      console.error('Error creating header rule:', err);
      setError('Failed to create header rule');
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = (rule: ProviderHeaderRewriteRule) => {
    setRuleToDelete(rule);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!ruleToDelete) return;

    try {
      await ProviderManagementService.deleteHeaderRule(ruleToDelete.id);
      await fetchHeaderRules();
      setDeleteDialogOpen(false);
      setRuleToDelete(null);
    } catch (err) {
      console.error('Error deleting header rule:', err);
      setError('Failed to delete header rule');
    }
  };

  const getActionColor = (action: string) => {
    switch (action) {
      case 'add':
        return 'success';
      case 'replace':
        return 'warning';
      case 'remove':
        return 'error';
      default:
        return 'default';
    }
  };

  const getProviderName = (providerId: number) => {
    const provider = providers.find(p => p.id === providerId);
    return provider ? provider.name : `Provider ${providerId}`;
  };

  const getProviderIcon = (providerId: number) => {
    const provider = providers.find(p => p.id === providerId);
    if (!provider) return '📧';
    
    switch (provider.type) {
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

  const totalRules = Object.values(headerRulesByProvider).reduce((total, rules) => total + rules.length, 0);

  if (providers.length === 0) {
    return (
      <Card>
        <CardContent sx={{ textAlign: 'center', py: 4 }}>
          <RuleIcon sx={{ fontSize: 48, color: 'text.secondary', mb: 2 }} />
          <Typography variant="h6" color="text.secondary" gutterBottom>
            No providers available
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Create providers first before configuring header rewrite rules
          </Typography>
        </CardContent>
      </Card>
    );
  }

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight={300}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
        <Typography variant="h5" component="h2">
          Header Rewrite Rules
          {totalRules > 0 && (
            <Chip 
              label={`${totalRules} rules`} 
              size="small" 
              sx={{ ml: 2 }} 
            />
          )}
        </Typography>
        <Stack direction="row" spacing={1}>
          <Button
            onClick={fetchHeaderRules}
            disabled={loading}
            variant="outlined"
            size="small"
          >
            Refresh
          </Button>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => handleCreateDialogOpen()}
          >
            Add Rule
          </Button>
        </Stack>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      {totalRules === 0 ? (
        <Card>
          <CardContent sx={{ textAlign: 'center', py: 4 }}>
            <RuleIcon sx={{ fontSize: 48, color: 'text.secondary', mb: 2 }} />
            <Typography variant="h6" color="text.secondary" gutterBottom>
              No header rewrite rules configured
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Create rules to modify email headers before sending
            </Typography>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              onClick={() => handleCreateDialogOpen()}
            >
              Add Rule
            </Button>
          </CardContent>
        </Card>
      ) : (
        <Stack spacing={2}>
          {/* If workspaceId is provided, don't show provider accordions since we're already in context */}
          {workspaceId ? (
            // Show rules directly without provider grouping
            providers.map((provider) => {
              const rules = headerRulesByProvider[provider.id] || [];
              return (
                <Box key={provider.id}>
                  {rules.length === 0 ? (
                    <Card>
                      <CardContent sx={{ textAlign: 'center', py: 3 }}>
                        <Typography variant="body2" color="text.secondary" gutterBottom>
                          No header rules configured for this provider
                        </Typography>
                        <Button
                          size="small"
                          startIcon={<AddIcon />}
                          onClick={() => handleCreateDialogOpen(provider.id)}
                          sx={{ mt: 1 }}
                        >
                          Add Rule
                        </Button>
                      </CardContent>
                    </Card>
                  ) : (
                    <TableContainer component={Paper} variant="outlined">
                      <Table size="small">
                        <TableHead>
                          <TableRow>
                            <TableCell>Header Name</TableCell>
                            <TableCell>Action</TableCell>
                            <TableCell>Value</TableCell>
                            <TableCell>Condition</TableCell>
                            <TableCell>Status</TableCell>
                            <TableCell align="center">Actions</TableCell>
                          </TableRow>
                        </TableHead>
                        <TableBody>
                          {rules.map((rule) => (
                            <TableRow key={rule.id}>
                              <TableCell>
                                <Typography variant="body2" fontFamily="monospace">
                                  {rule.header_name}
                                </Typography>
                              </TableCell>
                              <TableCell>
                                <Chip
                                  label={rule.action}
                                  color={getActionColor(rule.action) as any}
                                  size="small"
                                />
                              </TableCell>
                              <TableCell>
                                {rule.value ? (
                                  <Typography variant="body2" fontFamily="monospace">
                                    {rule.value}
                                  </Typography>
                                ) : (
                                  <Typography variant="body2" color="text.secondary">
                                    —
                                  </Typography>
                                )}
                              </TableCell>
                              <TableCell>
                                {rule.condition ? (
                                  <Typography variant="body2" fontFamily="monospace">
                                    {rule.condition}
                                  </Typography>
                                ) : (
                                  <Typography variant="body2" color="text.secondary">
                                    —
                                  </Typography>
                                )}
                              </TableCell>
                              <TableCell>
                                <Chip
                                  label={rule.enabled ? 'Enabled' : 'Disabled'}
                                  color={rule.enabled ? 'success' : 'default'}
                                  size="small"
                                />
                              </TableCell>
                              <TableCell align="center">
                                <Button
                                  onClick={() => handleDelete(rule)}
                                  color="error"
                                  size="small"
                                  variant="outlined"
                                >
                                  Delete
                                </Button>
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </TableContainer>
                  )}
                </Box>
              );
            })
          ) : (
            // Show with provider accordions when not in workspace context
            providers.map((provider) => {
              const rules = headerRulesByProvider[provider.id] || [];
              
              return (
                <Accordion key={provider.id} defaultExpanded={rules.length > 0}>
                  <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                    <Box display="flex" alignItems="center" gap={2}>
                      <Box fontSize="1.5rem">{getProviderIcon(provider.id)}</Box>
                      <Typography variant="h6">
                        {provider.name}
                      </Typography>
                      <Chip 
                        label={`${rules.length} rules`} 
                        size="small" 
                        color={rules.length > 0 ? 'primary' : 'default'}
                      />
                    </Box>
                  </AccordionSummary>
                  <AccordionDetails>
                  {rules.length === 0 ? (
                    <Box textAlign="center" py={2}>
                      <Typography variant="body2" color="text.secondary" gutterBottom>
                        No header rules configured for this provider
                      </Typography>
                      <Button
                        size="small"
                        startIcon={<AddIcon />}
                        onClick={() => handleCreateDialogOpen(provider.id)}
                      >
                        Add Rule
                      </Button>
                    </Box>
                  ) : (
                    <>
                      <Box display="flex" justifyContent="flex-end" mb={2}>
                        <Button
                          size="small"
                          startIcon={<AddIcon />}
                          onClick={() => handleCreateDialogOpen(provider.id)}
                        >
                          Add Rule
                        </Button>
                      </Box>
                      <TableContainer component={Paper} variant="outlined">
                        <Table size="small">
                          <TableHead>
                            <TableRow>
                              <TableCell>Header Name</TableCell>
                              <TableCell>Action</TableCell>
                              <TableCell>Value</TableCell>
                              <TableCell>Condition</TableCell>
                              <TableCell>Status</TableCell>
                              <TableCell align="center">Actions</TableCell>
                            </TableRow>
                          </TableHead>
                          <TableBody>
                            {rules.map((rule) => (
                              <TableRow key={rule.id}>
                                <TableCell>
                                  <Typography variant="body2" fontFamily="monospace">
                                    {rule.header_name}
                                  </Typography>
                                </TableCell>
                                <TableCell>
                                  <Chip
                                    label={rule.action}
                                    color={getActionColor(rule.action) as any}
                                    size="small"
                                  />
                                </TableCell>
                                <TableCell>
                                  {rule.value ? (
                                    <Typography variant="body2" fontFamily="monospace">
                                      {rule.value}
                                    </Typography>
                                  ) : (
                                    <Typography variant="body2" color="text.secondary">
                                      —
                                    </Typography>
                                  )}
                                </TableCell>
                                <TableCell>
                                  {rule.condition ? (
                                    <Typography variant="body2" fontFamily="monospace">
                                      {rule.condition}
                                    </Typography>
                                  ) : (
                                    <Typography variant="body2" color="text.secondary">
                                      —
                                    </Typography>
                                  )}
                                </TableCell>
                                <TableCell>
                                  <Chip
                                    label={rule.enabled ? 'Enabled' : 'Disabled'}
                                    color={rule.enabled ? 'success' : 'default'}
                                    size="small"
                                  />
                                </TableCell>
                                <TableCell align="center">
                                  <Button
                                    onClick={() => handleDelete(rule)}
                                    color="error"
                                    size="small"
                                    variant="outlined"
                                  >
                                    Delete
                                  </Button>
                                </TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      </TableContainer>
                    </>
                  )}
                </AccordionDetails>
              </Accordion>
              );
            })
          )}
        </Stack>
      )}

      {/* Close button at bottom if onClose is provided */}
      {onClose && (
        <Box display="flex" justifyContent="flex-end" mt={3}>
          <Button onClick={onClose}>
            Close
          </Button>
        </Box>
      )}

      {/* Create Header Rule Dialog */}
      <Dialog
        open={createDialogOpen}
        onClose={handleCreateDialogClose}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Create Header Rewrite Rule</DialogTitle>
        <DialogContent sx={{ mt: 1 }}>
          <Box sx={{ pt: 1 }}>
            <Stack spacing={2.5} sx={{ width: '100%' }}>
            {/* Only show provider selection if not in workspace context */}
            {!workspaceId && (
              <FormControl fullWidth required>
                <InputLabel id="provider-select-label">Provider</InputLabel>
                <Select
                  labelId="provider-select-label"
                  label="Provider"
                  value={formData.provider_id}
                  onChange={(e) => handleInputChange('provider_id', e.target.value)}
                >
                  {providers.map((provider) => (
                    <MenuItem key={provider.id} value={provider.id}>
                      {getProviderIcon(provider.id)} {provider.name}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            )}
            
            <TextField
              label="Header Name"
              value={formData.header_name}
              onChange={(e) => handleInputChange('header_name', e.target.value)}
              fullWidth
              required
              helperText="Name of the email header to modify (e.g., X-Custom-Header)"
            />
            
            <FormControl fullWidth required>
              <InputLabel id="action-select-label">Action</InputLabel>
              <Select
                labelId="action-select-label"
                label="Action"
                value={formData.action}
                onChange={(e) => handleInputChange('action', e.target.value)}
              >
                <MenuItem value="add">Add - Add a new header</MenuItem>
                <MenuItem value="replace">Replace - Replace header value if exists</MenuItem>
                <MenuItem value="remove">Remove - Remove header entirely</MenuItem>
              </Select>
            </FormControl>
            
            {(formData.action === 'add' || formData.action === 'replace') && (
              <TextField
                label="Header Value"
                value={formData.value}
                onChange={(e) => handleInputChange('value', e.target.value)}
                fullWidth
                required
                multiline
                rows={2}
                helperText="Value to set for the header"
              />
            )}
            
            <TextField
              label="Condition (Optional)"
              value={formData.condition}
              onChange={(e) => handleInputChange('condition', e.target.value)}
              fullWidth
              multiline
              rows={2}
              helperText="Optional condition for when this rule should apply (advanced)"
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
            </Stack>
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCreateDialogClose} disabled={creating}>
            Cancel
          </Button>
          <Button
            onClick={handleCreate}
            variant="contained"
            disabled={creating}
          >
            {creating ? 'Creating...' : 'Create Rule'}
          </Button>
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
            Are you sure you want to delete the header rewrite rule for "{ruleToDelete?.header_name}"?
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