'use client';

import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Card,
  CardContent,
  TextField,
  Button,
  Alert,
  CircularProgress,
  Stack,
  Divider,
  Grid,
  InputAdornment,
  Tooltip,
  IconButton,
} from '@mui/material';
import {
  Save as SaveIcon,
  Refresh as RefreshIcon,
  Info as InfoIcon,
} from '@mui/icons-material';
import { WorkspaceRateLimitConfig } from '../types/relay';
import { ProviderManagementService } from '../services/providerManagement';

interface RateLimitsConfigProps {
  workspaceId: string;
}

export function RateLimitsConfig({ workspaceId }: RateLimitsConfigProps) {
  const [rateLimits, setRateLimits] = useState<WorkspaceRateLimitConfig | null>(null);
  const [formData, setFormData] = useState({
    daily: 1000,
    hourly: 100,
    per_user_daily: 100,
    per_user_hourly: 10,
  });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const fetchRateLimits = async () => {
    try {
      setLoading(true);
      setError(null);
      const limits = await ProviderManagementService.getWorkspaceRateLimits(workspaceId);
      setRateLimits(limits);
      setFormData({
        daily: limits.daily,
        hourly: limits.hourly,
        per_user_daily: limits.per_user_daily,
        per_user_hourly: limits.per_user_hourly,
      });
    } catch (err) {
      console.error('Error fetching rate limits:', err);
      setError('Failed to fetch rate limits');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchRateLimits();
  }, [workspaceId]);

  const handleInputChange = (field: string, value: string) => {
    const numValue = parseInt(value) || 0;
    setFormData(prev => ({
      ...prev,
      [field]: numValue,
    }));
    
    // Clear success message when making changes
    if (success) {
      setSuccess(null);
    }
  };

  const validateForm = (): string | null => {
    if (formData.daily <= 0) {
      return 'Daily limit must be greater than 0';
    }
    if (formData.hourly <= 0) {
      return 'Hourly limit must be greater than 0';
    }
    if (formData.per_user_daily <= 0) {
      return 'Per-user daily limit must be greater than 0';
    }
    if (formData.per_user_hourly <= 0) {
      return 'Per-user hourly limit must be greater than 0';
    }
    if (formData.hourly > formData.daily) {
      return 'Hourly limit cannot exceed daily limit';
    }
    if (formData.per_user_hourly > formData.per_user_daily) {
      return 'Per-user hourly limit cannot exceed per-user daily limit';
    }
    return null;
  };

  const handleSave = async () => {
    const validationError = validateForm();
    if (validationError) {
      setError(validationError);
      return;
    }

    setSaving(true);
    setError(null);

    try {
      const updatedLimits = await ProviderManagementService.updateWorkspaceRateLimits(workspaceId, formData);
      setRateLimits(updatedLimits);
      setSuccess('Rate limits updated successfully');
    } catch (err) {
      console.error('Error updating rate limits:', err);
      setError('Failed to update rate limits');
    } finally {
      setSaving(false);
    }
  };

  const hasChanges = () => {
    if (!rateLimits) return false;
    return (
      formData.daily !== rateLimits.daily ||
      formData.hourly !== rateLimits.hourly ||
      formData.per_user_daily !== rateLimits.per_user_daily ||
      formData.per_user_hourly !== rateLimits.per_user_hourly
    );
  };

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
          Rate Limits Configuration
        </Typography>
        <Tooltip title="Refresh">
          <IconButton onClick={fetchRateLimits} disabled={loading}>
            <RefreshIcon />
          </IconButton>
        </Tooltip>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      {success && (
        <Alert severity="success" sx={{ mb: 2 }}>
          {success}
        </Alert>
      )}

      <Card>
        <CardContent>
          <Typography variant="h6" gutterBottom>
            Provider Rate Limits
            <Tooltip title="These limits apply to all email sending for this provider">
              <IconButton size="small" sx={{ ml: 1 }}>
                <InfoIcon fontSize="small" />
              </IconButton>
            </Tooltip>
          </Typography>
          
          <Grid container spacing={3} sx={{ mt: 1 }}>
            <Grid item xs={12} md={6}>
              <TextField
                label="Daily Limit"
                type="number"
                value={formData.daily}
                onChange={(e) => handleInputChange('daily', e.target.value)}
                inputProps={{ min: 1 }}
                fullWidth
                helperText="Maximum emails per day for this provider"
                InputProps={{
                  endAdornment: <InputAdornment position="end">emails/day</InputAdornment>,
                }}
              />
            </Grid>
            
            <Grid item xs={12} md={6}>
              <TextField
                label="Hourly Limit"
                type="number"
                value={formData.hourly}
                onChange={(e) => handleInputChange('hourly', e.target.value)}
                inputProps={{ min: 1 }}
                fullWidth
                helperText="Maximum emails per hour for this provider"
                InputProps={{
                  endAdornment: <InputAdornment position="end">emails/hour</InputAdornment>,
                }}
              />
            </Grid>
          </Grid>

          <Divider sx={{ my: 3 }} />

          <Typography variant="h6" gutterBottom>
            Per-User Rate Limits
            <Tooltip title="These limits apply to individual users within the provider">
              <IconButton size="small" sx={{ ml: 1 }}>
                <InfoIcon fontSize="small" />
              </IconButton>
            </Tooltip>
          </Typography>
          
          <Grid container spacing={3} sx={{ mt: 1 }}>
            <Grid item xs={12} md={6}>
              <TextField
                label="Per-User Daily Limit"
                type="number"
                value={formData.per_user_daily}
                onChange={(e) => handleInputChange('per_user_daily', e.target.value)}
                inputProps={{ min: 1 }}
                fullWidth
                helperText="Default daily limit per user"
                InputProps={{
                  endAdornment: <InputAdornment position="end">emails/day</InputAdornment>,
                }}
              />
            </Grid>
            
            <Grid item xs={12} md={6}>
              <TextField
                label="Per-User Hourly Limit"
                type="number"
                value={formData.per_user_hourly}
                onChange={(e) => handleInputChange('per_user_hourly', e.target.value)}
                inputProps={{ min: 1 }}
                fullWidth
                helperText="Default hourly limit per user"
                InputProps={{
                  endAdornment: <InputAdornment position="end">emails/hour</InputAdornment>,
                }}
              />
            </Grid>
          </Grid>

          <Box display="flex" justifyContent="flex-end" mt={3}>
            <Button
              variant="contained"
              startIcon={<SaveIcon />}
              onClick={handleSave}
              disabled={saving || !hasChanges()}
            >
              {saving ? 'Saving...' : 'Save Rate Limits'}
            </Button>
          </Box>

          {rateLimits && (
            <Box mt={3} p={2} bgcolor="grey.50" borderRadius={1}>
              <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                Last Updated: {new Date(rateLimits.updated_at).toLocaleString()}
              </Typography>
            </Box>
          )}
        </CardContent>
      </Card>
    </Box>
  );
}