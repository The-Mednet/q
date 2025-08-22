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
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
} from '@mui/material';
import GridLegacy from '@mui/material/GridLegacy';
import {
  Add as AddIcon,
  History as HistoryIcon,
  CheckCircle,
  Cancel,
} from '@mui/icons-material';
import { usePools, useSelections, createPool, updatePool, deletePool, togglePool } from '@/services/pools';
import { LoadBalancingPool } from '@/types/relay';

export default function PoolsPage() {
  const { data: poolsData, error: poolsError, mutate: mutatePools } = usePools();
  const { data: selectionsData } = useSelections(50);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [selectedPool, setSelectedPool] = useState<LoadBalancingPool | null>(null);
  const [formData, setFormData] = useState<Partial<LoadBalancingPool>>({
    name: '',
    algorithm: 'round_robin',
    providers: [],
    enabled: true,
  });

  const handleOpenDialog = (pool?: LoadBalancingPool) => {
    if (pool) {
      setSelectedPool(pool);
      setFormData(pool);
    } else {
      setSelectedPool(null);
      setFormData({
        name: '',
        algorithm: 'round_robin',
        providers: [],
        enabled: true,
      });
    }
    setDialogOpen(true);
  };

  const handleCloseDialog = () => {
    setDialogOpen(false);
    setSelectedPool(null);
  };

  const handleSave = async () => {
    try {
      if (selectedPool) {
        await updatePool(selectedPool.id, formData);
      } else {
        await createPool(formData);
      }
      mutatePools();
      handleCloseDialog();
    } catch (err) {
      console.error('Failed to save pool:', err);
    }
  };

  const handleDelete = async () => {
    if (selectedPool) {
      try {
        await deletePool(selectedPool.id);
        mutatePools();
        setDeleteDialogOpen(false);
        setSelectedPool(null);
      } catch (err) {
        console.error('Failed to delete pool:', err);
      }
    }
  };

  const handleToggle = async (pool: LoadBalancingPool) => {
    try {
      await togglePool(pool.id, !pool.enabled);
      mutatePools();
    } catch (err) {
      console.error('Failed to toggle pool:', err);
    }
  };


  if (poolsError) {
    return <Alert severity="error">Failed to load pools: {poolsError.message}</Alert>;
  }

  const strategyDescriptions = {
    round_robin: 'Distributes requests evenly across all providers in sequence',
    capacity_weighted: 'Routes based on provider capacity (higher capacity = more traffic)',
    least_used: 'Routes to the provider with the fewest recent selections',
    random_weighted: 'Randomly selects a provider weighted by capacity',
  };

  return (
    <Box>
      <Box sx={{ mb: 3, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Typography variant="h5">Load Balancing Pools</Typography>
        <Button
          variant="contained"
          startIcon={<AddIcon />}
          onClick={() => handleOpenDialog()}
        >
          Add Pool
        </Button>
      </Box>

      <GridLegacy container spacing={3}>
        {/* Pools List */}
        <GridLegacy item xs={12} lg={7}>
          <GridLegacy container spacing={2}>
            {poolsData?.pools?.map((pool) => (
              <GridLegacy item xs={12} key={pool.id}>
                <Card>
                  <CardContent>
                    <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'start' }}>
                      <Box>
                        <Typography variant="h6" gutterBottom>
                          {pool.name}
                        </Typography>
                        <Box sx={{ display: 'flex', gap: 1, mb: 1 }}>
                          <Chip
                            icon={pool.enabled ? <CheckCircle /> : <Cancel />}
                            label={pool.enabled ? 'Active' : 'Inactive'}
                            color={pool.enabled ? 'default' : 'error'}
                            size="small"
                            sx={{ 
                              backgroundColor: pool.enabled ? '#f5f5f5' : undefined,
                              color: pool.enabled ? '#666' : undefined 
                            }}
                          />
                          <Chip
                            label={(pool.algorithm || pool.strategy || 'unknown').replace('_', ' ').toUpperCase()}
                            variant="outlined"
                            size="small"
                          />
                        </Box>
                      </Box>
                    </Box>

                    <Typography variant="body2" color="textSecondary" gutterBottom>
                      Domain Patterns: {pool.domain_patterns?.join(', ') || 'None'}
                    </Typography>

                    <Box sx={{ mt: 2, display: 'flex', gap: 2 }}>
                      <Typography variant="caption">
                        Workspaces: {pool.workspace_count}
                      </Typography>
                      <Typography variant="caption">
                        Selections: {pool.selection_count}
                      </Typography>
                    </Box>
                  </CardContent>
                  <CardActions sx={{ px: 2, pb: 2 }}>
                    <Button
                      onClick={() => handleToggle(pool)}
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
                      {pool.enabled ? 'Disable' : 'Enable'}
                    </Button>
                    <Button 
                      onClick={() => handleOpenDialog(pool)}
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
                        setSelectedPool(pool);
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
        </GridLegacy>

        {/* Recent Selections */}
        <GridLegacy item xs={12} lg={5}>
          <Paper sx={{ p: 2 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
              <HistoryIcon sx={{ mr: 1 }} />
              <Typography variant="h6">Recent Provider Selections</Typography>
            </Box>
            <TableContainer>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>Pool</TableCell>
                    <TableCell>Workspace</TableCell>
                    <TableCell>Time</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {selectionsData?.selections?.map((selection, index) => (
                    <TableRow key={index}>
                      <TableCell>{selection.pool_id}</TableCell>
                      <TableCell>
                        <Chip
                          label={selection.workspace_id}
                          size="small"
                          color="primary"
                        />
                      </TableCell>
                      <TableCell>
                        {new Date(selection.selected_at).toLocaleTimeString()}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          </Paper>
        </GridLegacy>
      </GridLegacy>

      {/* Add/Edit Dialog */}
      <Dialog open={dialogOpen} onClose={handleCloseDialog} maxWidth="sm" fullWidth>
        <DialogTitle>
          {selectedPool ? 'Edit Pool' : 'Add New Pool'}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ pt: 2 }}>
            <GridLegacy container spacing={2}>
              <GridLegacy item xs={12}>
                <TextField
                  label="Pool Name"
                  fullWidth
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                />
              </GridLegacy>
              
              <GridLegacy item xs={12}>
                <FormControl fullWidth>
                  <InputLabel>Strategy</InputLabel>
                  <Select
                    value={formData.algorithm || 'round_robin'}
                    onChange={(e) => setFormData({ ...formData, algorithm: e.target.value as LoadBalancingPool['algorithm'] })}
                    label="Strategy"
                  >
                    <MenuItem value="round_robin">Round Robin</MenuItem>
                    <MenuItem value="capacity_weighted">Capacity Weighted</MenuItem>
                    <MenuItem value="least_used">Least Used</MenuItem>
                    <MenuItem value="random_weighted">Random Weighted</MenuItem>
                  </Select>
                </FormControl>
                <Typography variant="caption" color="textSecondary" sx={{ mt: 1 }}>
                  {strategyDescriptions[(formData.algorithm || 'round_robin') as keyof typeof strategyDescriptions]}
                </Typography>
              </GridLegacy>

              <GridLegacy item xs={12}>
                <TextField
                  label="Domain Patterns"
                  fullWidth
                  value={formData.domain_patterns?.join(', ') || ''}
                  onChange={(e) => setFormData({ 
                    ...formData, 
                    domain_patterns: e.target.value.split(',').map(s => s.trim()).filter(s => s) 
                  })}
                  helperText="Comma-separated domain patterns (e.g., *.example.com, mail.domain.org)"
                />
              </GridLegacy>

              <GridLegacy item xs={12}>
                <FormControlLabel
                  control={
                    <Switch
                      checked={formData.enabled ?? true}
                      onChange={(e) => setFormData({ ...formData, enabled: e.target.checked })}
                    />
                  }
                  label="Enabled"
                />
              </GridLegacy>
            </GridLegacy>
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseDialog}>Cancel</Button>
          <Button onClick={handleSave} variant="contained">
            {selectedPool ? 'Update' : 'Create'}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <Dialog open={deleteDialogOpen} onClose={() => setDeleteDialogOpen(false)}>
        <DialogTitle>Confirm Delete</DialogTitle>
        <DialogContent>
          <Typography>
            Are you sure you want to delete the pool &quot;{selectedPool?.name}&quot;?
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