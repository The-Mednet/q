'use client';

import React, { useState } from 'react';
import {
  Box,
  Paper,
  Typography,
  Chip,
  Button,
  TextField,
  InputAdornment,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Alert,
  IconButton,
  Tooltip,
} from '@mui/material';
import Grid2 from '@mui/material/GridLegacy';
import { DataGrid, GridColDef, GridRenderCellParams } from '@mui/x-data-grid';
import {
  Search as SearchIcon,
  Refresh as RefreshIcon,
  Visibility as ViewIcon,
  Send as ResendIcon,
  CheckCircle,
  Error as ErrorIcon,
  Schedule,
  HourglassEmpty,
} from '@mui/icons-material';
import { useMessages, resendMessage } from '@/services/messages';
import { Message } from '@/types/relay';

const statusConfig = {
  queued: { color: 'warning' as const, icon: <Schedule /> },
  processing: { color: 'info' as const, icon: <HourglassEmpty /> },
  sent: { color: 'success' as const, icon: <CheckCircle /> },
  failed: { color: 'error' as const, icon: <ErrorIcon /> },
};

export default function MessagesPage() {
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(25);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedMessage, setSelectedMessage] = useState<Message | null>(null);
  const [detailsOpen, setDetailsOpen] = useState(false);

  const { data, error, isLoading, mutate } = useMessages(page * pageSize, pageSize, searchTerm);

  const handleResend = async (messageId: string) => {
    try {
      await resendMessage(messageId);
      mutate();
    } catch (err) {
      console.error('Failed to resend message:', err);
    }
  };

  const handleViewDetails = (message: Message) => {
    setSelectedMessage(message);
    setDetailsOpen(true);
  };

  const columns: GridColDef<Message>[] = [
    {
      field: 'status',
      headerName: 'Status',
      width: 120,
      renderCell: (params: GridRenderCellParams<Message>) => {
        const config = statusConfig[params.row.status as keyof typeof statusConfig];
        return (
          <Chip
            icon={config.icon}
            label={params.row.status.toUpperCase()}
            color={config.color}
            size="small"
          />
        );
      },
    },
    {
      field: 'from_email',
      headerName: 'From',
      width: 200,
    },
    {
      field: 'to_emails',
      headerName: 'To',
      width: 200,
      renderCell: (params: GridRenderCellParams<Message>) => params.row.to_emails?.join(', '),
    },
    {
      field: 'subject',
      headerName: 'Subject',
      flex: 1,
      minWidth: 250,
    },
    {
      field: 'provider',
      headerName: 'Provider',
      width: 120,
      renderCell: (params: GridRenderCellParams<Message>) => (
        <Chip
          label={params.row.provider || 'Pending'}
          size="small"
          variant="outlined"
        />
      ),
    },
    {
      field: 'created_at',
      headerName: 'Created',
      width: 180,
      renderCell: (params: GridRenderCellParams<Message>) => 
        params.row.created_at ? new Date(params.row.created_at).toLocaleString() : '',
    },
    {
      field: 'actions',
      headerName: 'Actions',
      width: 120,
      sortable: false,
      renderCell: (params: GridRenderCellParams<Message>) => (
        <Box>
          <Tooltip title="View Details">
            <IconButton
              size="small"
              onClick={() => handleViewDetails(params.row)}
            >
              <ViewIcon />
            </IconButton>
          </Tooltip>
          {params.row.status === 'failed' && (
            <Tooltip title="Resend">
              <IconButton
                size="small"
                onClick={() => handleResend(params.row.id)}
                color="primary"
              >
                <ResendIcon />
              </IconButton>
            </Tooltip>
          )}
        </Box>
      ),
    },
  ];

  if (error) {
    return <Alert severity="error">Failed to load messages: {error.message}</Alert>;
  }

  return (
    <Box>
      <Box sx={{ mb: 3, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Typography variant="h5">Messages</Typography>
        <Box sx={{ display: 'flex', gap: 2 }}>
          <TextField
            placeholder="Search messages..."
            size="small"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <SearchIcon />
                </InputAdornment>
              ),
            }}
          />
          <Button
            variant="outlined"
            startIcon={<RefreshIcon />}
            onClick={() => mutate()}
          >
            Refresh
          </Button>
        </Box>
      </Box>

      <Paper>
        <DataGrid
          rows={data?.messages || []}
          columns={columns}
          rowCount={data?.total || 0}
          loading={isLoading}
          pageSizeOptions={[10, 25, 50, 100]}
          paginationModel={{
            page,
            pageSize,
          }}
          onPaginationModelChange={(model) => {
            setPage(model.page);
            setPageSize(model.pageSize);
          }}
          paginationMode="server"
          autoHeight
          disableRowSelectionOnClick
          sx={{
            '& .MuiDataGrid-cell': {
              borderBottom: '1px solid rgba(224, 224, 224, 0.5)',
            },
          }}
        />
      </Paper>

      {/* Message Details Dialog */}
      <Dialog open={detailsOpen} onClose={() => setDetailsOpen(false)} maxWidth="md" fullWidth>
        <DialogTitle>
          Message Details
          <Chip
            icon={statusConfig[selectedMessage?.status as keyof typeof statusConfig]?.icon}
            label={selectedMessage?.status.toUpperCase()}
            color={statusConfig[selectedMessage?.status as keyof typeof statusConfig]?.color}
            size="small"
            sx={{ ml: 2 }}
          />
        </DialogTitle>
        <DialogContent>
          <Grid2 container spacing={2} sx={{ mt: 1 }}>
            <Grid2 item xs={12} md={6}>
              <Typography variant="subtitle2" color="textSecondary">
                Message ID
              </Typography>
              <Typography variant="body2" sx={{ mb: 2 }}>
                {selectedMessage?.id}
              </Typography>
            </Grid2>
            
            <Grid2 item xs={12} md={6}>
              <Typography variant="subtitle2" color="textSecondary">
                Provider
              </Typography>
              <Typography variant="body2" sx={{ mb: 2 }}>
                {selectedMessage?.provider || 'Not assigned'}
              </Typography>
            </Grid2>

            <Grid2 item xs={12} md={6}>
              <Typography variant="subtitle2" color="textSecondary">
                From
              </Typography>
              <Typography variant="body2" sx={{ mb: 2 }}>
                {selectedMessage?.from_email}
                {selectedMessage?.from_name && ` (${selectedMessage.from_name})`}
              </Typography>
            </Grid2>

            <Grid2 item xs={12} md={6}>
              <Typography variant="subtitle2" color="textSecondary">
                To
              </Typography>
              <Typography variant="body2" sx={{ mb: 2 }}>
                {selectedMessage?.to_emails?.join(', ')}
              </Typography>
            </Grid2>

            {selectedMessage?.cc_emails && selectedMessage.cc_emails.length > 0 && (
              <Grid2 item xs={12} md={6}>
                <Typography variant="subtitle2" color="textSecondary">
                  CC
                </Typography>
                <Typography variant="body2" sx={{ mb: 2 }}>
                  {selectedMessage.cc_emails.join(', ')}
                </Typography>
              </Grid2>
            )}

            {selectedMessage?.bcc_emails && selectedMessage.bcc_emails.length > 0 && (
              <Grid2 item xs={12} md={6}>
                <Typography variant="subtitle2" color="textSecondary">
                  BCC
                </Typography>
                <Typography variant="body2" sx={{ mb: 2 }}>
                  {selectedMessage.bcc_emails.join(', ')}
                </Typography>
              </Grid2>
            )}

            <Grid2 item xs={12}>
              <Typography variant="subtitle2" color="textSecondary">
                Subject
              </Typography>
              <Typography variant="body2" sx={{ mb: 2 }}>
                {selectedMessage?.subject}
              </Typography>
            </Grid2>

            <Grid2 item xs={12} md={6}>
              <Typography variant="subtitle2" color="textSecondary">
                Created At
              </Typography>
              <Typography variant="body2" sx={{ mb: 2 }}>
                {selectedMessage?.created_at && new Date(selectedMessage.created_at).toLocaleString()}
              </Typography>
            </Grid2>

            <Grid2 item xs={12} md={6}>
              <Typography variant="subtitle2" color="textSecondary">
                Sent At
              </Typography>
              <Typography variant="body2" sx={{ mb: 2 }}>
                {selectedMessage?.sent_at ? new Date(selectedMessage.sent_at).toLocaleString() : 'Not sent'}
              </Typography>
            </Grid2>

            {selectedMessage?.error && (
              <Grid2 item xs={12}>
                <Alert severity="error">
                  <Typography variant="subtitle2">Error Details</Typography>
                  <Typography variant="body2">
                    {selectedMessage.error}
                  </Typography>
                </Alert>
              </Grid2>
            )}

            <Grid2 item xs={12}>
              <Typography variant="subtitle2" color="textSecondary">
                Message Body
              </Typography>
              <Paper variant="outlined" sx={{ p: 2, mt: 1, maxHeight: 300, overflow: 'auto' }}>
                {selectedMessage?.html_body ? (
                  <div dangerouslySetInnerHTML={{ __html: selectedMessage.html_body }} />
                ) : (
                  <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap' }}>
                    {selectedMessage?.text_body}
                  </Typography>
                )}
              </Paper>
            </Grid2>

            {selectedMessage?.headers && Object.keys(selectedMessage.headers).length > 0 && (
              <Grid2 item xs={12}>
                <Typography variant="subtitle2" color="textSecondary">
                  Headers
                </Typography>
                <Paper variant="outlined" sx={{ p: 2, mt: 1 }}>
                  {Object.entries(selectedMessage.headers).map(([key, value]) => (
                    <Typography key={key} variant="body2">
                      <strong>{key}:</strong> {value}
                    </Typography>
                  ))}
                </Paper>
              </Grid2>
            )}
          </Grid2>
        </DialogContent>
        <DialogActions>
          {selectedMessage?.status === 'failed' && (
            <Button
              onClick={() => {
                handleResend(selectedMessage.id);
                setDetailsOpen(false);
              }}
              color="primary"
              startIcon={<ResendIcon />}
            >
              Resend
            </Button>
          )}
          <Button onClick={() => setDetailsOpen(false)}>Close</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}