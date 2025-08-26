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
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  IconButton,
  Tooltip,
  Stack,
  InputAdornment,
} from '@mui/material';
import {
  Add as AddIcon,
  Delete as DeleteIcon,
  Refresh as RefreshIcon,
  Person as PersonIcon,
} from '@mui/icons-material';
import { WorkspaceUserRateLimit } from '../types/relay';
import { ProviderManagementService } from '../services/providerManagement';

interface UserRateLimitsConfigProps {
  workspaceId: string;
}

export function UserRateLimitsConfig({ workspaceId }: UserRateLimitsConfigProps) {
  const [userRateLimits, setUserRateLimits] = useState<WorkspaceUserRateLimit[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [userRateLimitToDelete, setUserRateLimitToDelete] = useState<WorkspaceUserRateLimit | null>(null);
  const [formData, setFormData] = useState({
    user_email: '',
    daily: 100,
    hourly: 10,
  });
  const [creating, setCreating] = useState(false);

  const fetchUserRateLimits = async () => {
    try {
      setLoading(true);
      setError(null);
      const limits = await ProviderManagementService.getWorkspaceUserRateLimits(workspaceId);
      setUserRateLimits(limits);
    } catch (err) {
      console.error('Error fetching user rate limits:', err);
      setError('Failed to fetch user rate limits');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchUserRateLimits();
  }, [workspaceId]);

  const handleCreateDialogOpen = () => {
    setFormData({
      user_email: '',
      daily: 100,
      hourly: 10,
    });
    setCreateDialogOpen(true);
  };

  const handleCreateDialogClose = () => {
    setCreateDialogOpen(false);
    setFormData({
      user_email: '',
      daily: 100,
      hourly: 10,
    });
  };

  const handleInputChange = (field: string, value: string | number) => {
    setFormData(prev => ({
      ...prev,
      [field]: value,
    }));
  };

  const validateForm = (): string | null => {
    if (!formData.user_email.trim()) {
      return 'User email is required';
    }
    
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    if (!emailRegex.test(formData.user_email)) {
      return 'Please enter a valid email address';
    }

    if (formData.daily <= 0) {
      return 'Daily limit must be greater than 0';
    }
    
    if (formData.hourly <= 0) {
      return 'Hourly limit must be greater than 0';
    }
    
    if (formData.hourly > formData.daily) {
      return 'Hourly limit cannot exceed daily limit';
    }

    // Check if user already has a custom rate limit
    if (userRateLimits.some(limit => limit.user_email === formData.user_email)) {
      return 'This user already has a custom rate limit configured';
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
      await ProviderManagementService.createUserRateLimit(workspaceId, formData);
      await fetchUserRateLimits();
      handleCreateDialogClose();
    } catch (err) {
      console.error('Error creating user rate limit:', err);
      setError('Failed to create user rate limit');
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = (userRateLimit: WorkspaceUserRateLimit) => {
    setUserRateLimitToDelete(userRateLimit);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!userRateLimitToDelete) return;

    try {
      await ProviderManagementService.deleteUserRateLimit(userRateLimitToDelete.id);
      await fetchUserRateLimits();
      setDeleteDialogOpen(false);
      setUserRateLimitToDelete(null);
    } catch (err) {
      console.error('Error deleting user rate limit:', err);
      setError('Failed to delete user rate limit');
    }
  };

  if (loading && userRateLimits.length === 0) {
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
          Custom User Rate Limits
        </Typography>
        <Stack direction="row" spacing={1}>
          <Tooltip title="Refresh">
            <IconButton onClick={fetchUserRateLimits} disabled={loading}>
              <RefreshIcon />
            </IconButton>
          </Tooltip>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleCreateDialogOpen}
          >
            Add User Limit
          </Button>
        </Stack>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      <Card>
        <CardContent>
          {userRateLimits.length === 0 ? (
            <Box textAlign="center" py={4}>
              <PersonIcon sx={{ fontSize: 48, color: 'text.secondary', mb: 2 }} />
              <Typography variant="h6" color="text.secondary" gutterBottom>
                No custom user rate limits configured
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                Create custom rate limits for specific users to override the default provider limits
              </Typography>
              <Button
                variant="contained"
                startIcon={<AddIcon />}
                onClick={handleCreateDialogOpen}
              >
                Add User Limit
              </Button>
            </Box>
          ) : (
            <TableContainer>
              <Table>
                <TableHead>
                  <TableRow>
                    <TableCell>User Email</TableCell>
                    <TableCell align="right">Daily Limit</TableCell>
                    <TableCell align="right">Hourly Limit</TableCell>
                    <TableCell align="center">Created</TableCell>
                    <TableCell align="center">Actions</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {userRateLimits.map((userLimit) => (
                    <TableRow key={userLimit.id}>
                      <TableCell>
                        <Typography variant="body2" fontWeight="medium">
                          {userLimit.user_email}
                        </Typography>
                      </TableCell>
                      <TableCell align="right">
                        {userLimit.daily.toLocaleString()}
                      </TableCell>
                      <TableCell align="right">
                        {userLimit.hourly.toLocaleString()}
                      </TableCell>
                      <TableCell align="center">
                        <Typography variant="body2" color="text.secondary">
                          {new Date(userLimit.created_at).toLocaleDateString()}
                        </Typography>
                      </TableCell>
                      <TableCell align="center">
                        <Button
                          onClick={() => handleDelete(userLimit)}
                          color="error"
                          size="small"
                          variant="outlined"
                          sx={{ 
                            fontSize: '0.75rem',
                            py: 0.25,
                            px: 1,
                            minWidth: 'auto'
                          }}
                        >
                          Remove
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </CardContent>
      </Card>

      {/* Create User Rate Limit Dialog */}
      <Dialog
        open={createDialogOpen}
        onClose={handleCreateDialogClose}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Create Custom User Rate Limit</DialogTitle>
        <DialogContent>
          <Stack spacing={3} sx={{ mt: 1 }}>
            <TextField
              label="User Email"
              type="email"
              value={formData.user_email}
              onChange={(e) => handleInputChange('user_email', e.target.value)}
              fullWidth
              required
              helperText="Email address of the user"
            />
            
            <TextField
              label="Daily Limit"
              type="number"
              value={formData.daily}
              onChange={(e) => handleInputChange('daily', parseInt(e.target.value) || 0)}
              inputProps={{ min: 1 }}
              fullWidth
              required
              helperText="Maximum emails per day for this user"
              InputProps={{
                endAdornment: <InputAdornment position="end">emails/day</InputAdornment>,
              }}
            />
            
            <TextField
              label="Hourly Limit"
              type="number"
              value={formData.hourly}
              onChange={(e) => handleInputChange('hourly', parseInt(e.target.value) || 0)}
              inputProps={{ min: 1 }}
              fullWidth
              required
              helperText="Maximum emails per hour for this user"
              InputProps={{
                endAdornment: <InputAdornment position="end">emails/hour</InputAdornment>,
              }}
            />
          </Stack>
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
            {creating ? 'Creating...' : 'Create'}
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
            Are you sure you want to delete the custom rate limit for &ldquo;{userRateLimitToDelete?.user_email}&rdquo;?
            This user will fall back to the default provider rate limits.
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