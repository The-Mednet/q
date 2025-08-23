'use client';

import React from 'react';
import {
  Box,
  Button,
  Card,
  CardContent,
  CardActions,
  Chip,
  Typography,
} from '@mui/material';
import {
  CheckCircle,
  Cancel,
} from '@mui/icons-material';
import { WorkspaceConfig } from '@/types/relay';

interface WorkspaceCardProps {
  workspace: WorkspaceConfig;
  onEdit: (workspace: WorkspaceConfig) => void;
  onDelete: (workspace: WorkspaceConfig) => void;
}

export const WorkspaceCard = React.memo(({ workspace, onEdit, onDelete }: WorkspaceCardProps) => {
  const getProviderType = () => {
    if (workspace.gmail) return 'Gmail';
    if (workspace.mailgun) return 'Mailgun';
    if (workspace.mandrill) return 'Mandrill';
    if (workspace.provider_type) {
      return workspace.provider_type.charAt(0).toUpperCase() + workspace.provider_type.slice(1);
    }
    return 'No Provider';
  };

  const hasCredentials = () => {
    if (workspace.gmail) {
      return !!(workspace.gmail.service_account_file || workspace.gmail.has_credentials);
    }
    return true; // Other providers don't need credential files
  };

  return (
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
            label={getProviderType()}
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

        {/* Gmail specific config */}
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
                icon={hasCredentials() ? <CheckCircle /> : <Cancel />}
                label={hasCredentials() ? 'Credentials Configured' : 'No Credentials'}
                size="small"
                color={hasCredentials() ? 'success' : 'warning'}
                variant="outlined"
              />
            </Box>
          </Box>
        )}
        
        {/* Mailgun specific config */}
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
        
        {/* Mandrill specific config */}
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
          onClick={() => onEdit(workspace)} 
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
          onClick={() => onDelete(workspace)}
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
  );
});