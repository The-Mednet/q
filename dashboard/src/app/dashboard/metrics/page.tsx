'use client';

import React from 'react';
import {
  Paper,
  Typography,
  Box,
  Card,
  CardContent,
  Chip,
  LinearProgress,
  Alert,
} from '@mui/material';
import GridLegacy from '@mui/material/GridLegacy';
import {
  AreaChart,
  Area,
  BarChart,
  Bar,
  LineChart,
  Line,
  PieChart,
  Pie,
  Cell,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from 'recharts';
import { useStats, useRateLimits, useHealth } from '@/services/metrics';
import { ChartPalette } from '@/assets/styles/theme';

const statusColors = ChartPalette.status;

function MetricsPage() {
  const { data: stats, isLoading: statsLoading } = useStats();
  const { data: rateLimits, isLoading: rateLimitsLoading } = useRateLimits();
  const { data: health, isLoading: healthLoading } = useHealth();

  if (statsLoading || rateLimitsLoading || healthLoading) {
    return <LinearProgress />;
  }

  const hourlyData = stats?.hourly_stats || [];
  const statusData = stats ? [
    { name: 'Queued', value: stats.messages_queued, color: statusColors.queued },
    { name: 'Processing', value: stats.messages_processing, color: statusColors.processing },
    { name: 'Sent', value: stats.messages_sent, color: statusColors.sent },
    { name: 'Failed', value: stats.messages_failed, color: statusColors.failed },
  ] : [];

  const providerData = stats?.provider_stats || [];

  return (
    <GridLegacy container spacing={3}>
      {/* Health Status Alert */}
      {health && !health.healthy && (
        <GridLegacy item xs={12}>
          <Alert severity="error">
            System is unhealthy: {health.errors?.join(', ')}
          </Alert>
        </GridLegacy>
      )}

      {/* Summary Cards */}
      <GridLegacy item xs={12} md={3}>
        <Card sx={{ 
          background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
          color: 'white',
          position: 'relative',
          overflow: 'visible',
          '&:hover': {
            transform: 'translateY(-8px)',
            boxShadow: '0px 16px 48px rgba(102, 126, 234, 0.3)',
          }
        }}>
          <CardContent>
            <Typography sx={{ fontSize: '0.875rem', opacity: 0.9, fontWeight: 600 }} gutterBottom>
              Total Messages
            </Typography>
            <Typography variant="h3" sx={{ fontWeight: 700, mb: 1 }}>
              {stats?.total_messages?.toLocaleString() || '0'}
            </Typography>
            <Box sx={{ mt: 1 }}>
              <Chip
                label={`+${stats?.messages_today || 0} today`}
                size="small"
                sx={{ 
                  backgroundColor: 'rgba(255,255,255,0.2)', 
                  color: 'white',
                  fontWeight: 600,
                }}
              />
            </Box>
          </CardContent>
        </Card>
      </GridLegacy>

      <GridLegacy item xs={12} md={3}>
        <Card sx={{ 
          background: 'linear-gradient(135deg, #10B981 0%, #059669 100%)',
          color: 'white',
          position: 'relative',
          overflow: 'visible',
          '&:hover': {
            transform: 'translateY(-8px)',
            boxShadow: '0px 16px 48px rgba(16, 185, 129, 0.3)',
          }
        }}>
          <CardContent>
            <Typography sx={{ fontSize: '0.875rem', opacity: 0.9, fontWeight: 600 }} gutterBottom>
              Success Rate
            </Typography>
            <Typography variant="h3" sx={{ fontWeight: 700, mb: 1 }}>
              {stats?.success_rate ? `${(stats.success_rate * 100).toFixed(1)}%` : '0%'}
            </Typography>
            <Box sx={{ mt: 1 }}>
              <LinearProgress
                variant="determinate"
                value={(stats?.success_rate || 0) * 100}
                sx={{ 
                  backgroundColor: 'rgba(255,255,255,0.2)',
                  '& .MuiLinearProgress-bar': {
                    backgroundColor: 'rgba(255,255,255,0.8)',
                  },
                  height: 6,
                  borderRadius: 3,
                }}
              />
            </Box>
          </CardContent>
        </Card>
      </GridLegacy>

      <GridLegacy item xs={12} md={3}>
        <Card sx={{ 
          background: 'linear-gradient(135deg, #F59E0B 0%, #D97706 100%)',
          color: 'white',
          position: 'relative',
          overflow: 'visible',
          '&:hover': {
            transform: 'translateY(-8px)',
            boxShadow: '0px 16px 48px rgba(245, 158, 11, 0.3)',
          }
        }}>
          <CardContent>
            <Typography sx={{ fontSize: '0.875rem', opacity: 0.9, fontWeight: 600 }} gutterBottom>
              Queue Size
            </Typography>
            <Typography variant="h3" sx={{ fontWeight: 700, mb: 1 }}>
              {(stats?.messages_queued || 0) + (stats?.messages_processing || 0)}
            </Typography>
            <Box sx={{ mt: 1 }}>
              <Chip
                label={stats?.messages_processing ? `${stats.messages_processing} processing` : 'Idle'}
                size="small"
                sx={{ 
                  backgroundColor: 'rgba(255,255,255,0.2)', 
                  color: 'white',
                  fontWeight: 600,
                }}
              />
            </Box>
          </CardContent>
        </Card>
      </GridLegacy>

      <GridLegacy item xs={12} md={3}>
        <Card sx={{ 
          background: 'linear-gradient(135deg, #303B8B 0%, #5969C5 100%)',
          color: 'white',
          position: 'relative',
          overflow: 'visible',
          '&:hover': {
            transform: 'translateY(-8px)',
            boxShadow: '0px 16px 48px rgba(48, 59, 139, 0.3)',
          }
        }}>
          <CardContent>
            <Typography sx={{ fontSize: '0.875rem', opacity: 0.9, fontWeight: 600 }} gutterBottom>
              Active Providers
            </Typography>
            <Typography variant="h3" sx={{ fontWeight: 700, mb: 1 }}>
              {health?.provider_status?.filter(p => p.healthy).length || 0} / {health?.provider_status?.length || 0}
            </Typography>
            <Box sx={{ mt: 1, display: 'flex', gap: 0.5, flexWrap: 'wrap' }}>
              {health?.provider_status?.map(provider => (
                <Chip
                  key={provider.name}
                  label={provider.name}
                  size="small"
                  sx={{ 
                    backgroundColor: provider.healthy ? 'rgba(255,255,255,0.2)' : 'rgba(239, 68, 68, 0.3)',
                    color: 'white',
                    fontWeight: 600,
                    fontSize: '0.75rem',
                  }}
                />
              ))}
            </Box>
          </CardContent>
        </Card>
      </GridLegacy>

      {/* Message Volume Chart */}
      <GridLegacy item xs={12} lg={8}>
        <Paper sx={{ p: 2 }}>
          <Typography variant="h6" gutterBottom>
            Message Volume (24 Hours)
          </Typography>
          <ResponsiveContainer width="100%" height={300}>
            <AreaChart data={hourlyData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="hour" />
              <YAxis />
              <Tooltip />
              <Legend />
              <Area
                type="monotone"
                dataKey="sent"
                stackId="1"
                stroke={statusColors.sent}
                fill={statusColors.sent}
                name="Sent"
              />
              <Area
                type="monotone"
                dataKey="failed"
                stackId="1"
                stroke={statusColors.failed}
                fill={statusColors.failed}
                name="Failed"
              />
              <Area
                type="monotone"
                dataKey="queued"
                stackId="1"
                stroke={statusColors.queued}
                fill={statusColors.queued}
                name="Queued"
              />
            </AreaChart>
          </ResponsiveContainer>
        </Paper>
      </GridLegacy>

      {/* Status Distribution */}
      <GridLegacy item xs={12} lg={4}>
        <Paper sx={{ p: 2 }}>
          <Typography variant="h6" gutterBottom>
            Status Distribution
          </Typography>
          <ResponsiveContainer width="100%" height={300}>
            <PieChart>
              <Pie
                data={statusData}
                cx="50%"
                cy="50%"
                labelLine={false}
                label={({ name, percent }) => `${name} ${((percent ?? 0) * 100).toFixed(0)}%`}
                outerRadius={80}
                fill="#8884d8"
                dataKey="value"
              >
                {statusData.map((entry, index) => (
                  <Cell key={`cell-${index}`} fill={entry.color} />
                ))}
              </Pie>
              <Tooltip />
            </PieChart>
          </ResponsiveContainer>
        </Paper>
      </GridLegacy>

      {/* Provider Performance */}
      <GridLegacy item xs={12} lg={6}>
        <Paper sx={{ p: 2 }}>
          <Typography variant="h6" gutterBottom>
            Provider Performance
          </Typography>
          <ResponsiveContainer width="100%" height={300}>
            <BarChart data={providerData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="provider" />
              <YAxis />
              <Tooltip />
              <Legend />
              <Bar dataKey="sent" fill={statusColors.sent} name="Sent" />
              <Bar dataKey="failed" fill={statusColors.failed} name="Failed" />
            </BarChart>
          </ResponsiveContainer>
        </Paper>
      </GridLegacy>

      {/* Rate Limits */}
      <GridLegacy item xs={12} lg={6}>
        <Paper sx={{ p: 2 }}>
          <Typography variant="h6" gutterBottom>
            Rate Limit Usage
          </Typography>
          <Box sx={{ mt: 2 }}>
            {rateLimits?.workspace_limits?.map((limit) => (
              <Box key={limit.workspace_id} sx={{ mb: 2 }}>
                <Typography variant="body2" gutterBottom>
                  {limit.workspace_id}
                </Typography>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                  <LinearProgress
                    variant="determinate"
                    value={(limit.used / limit.limit) * 100}
                    sx={{ flexGrow: 1, height: 8 }}
                    color={limit.used / limit.limit > 0.8 ? 'warning' : 'primary'}
                  />
                  <Typography variant="body2" sx={{ minWidth: 100 }}>
                    {limit.used} / {limit.limit}
                  </Typography>
                </Box>
              </Box>
            ))}
          </Box>
        </Paper>
      </GridLegacy>

      {/* Average Processing Time */}
      <GridLegacy item xs={12}>
        <Paper sx={{ p: 2 }}>
          <Typography variant="h6" gutterBottom>
            Processing Time Trends
          </Typography>
          <ResponsiveContainer width="100%" height={250}>
            <LineChart data={hourlyData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="hour" />
              <YAxis />
              <Tooltip />
              <Legend />
              <Line
                type="monotone"
                dataKey="avg_processing_time"
                stroke="#8884d8"
                name="Avg Processing Time (ms)"
              />
            </LineChart>
          </ResponsiveContainer>
        </Paper>
      </GridLegacy>
    </GridLegacy>
  );
}

export default React.memo(MetricsPage);