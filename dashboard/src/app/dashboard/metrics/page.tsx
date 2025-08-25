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

  // Ensure data is properly initialized and safe for Recharts
  const hourlyData = React.useMemo(() => {
    if (!stats?.hourly_stats || stats.hourly_stats.length === 0) {
      // Return empty dataset that won't crash Recharts
      return [{ hour: '00:00', sent: 0, failed: 0, queued: 0, avg_processing_time: 0 }];
    }
    
    // Aggregate duplicate hours and ensure all fields are numbers
    const hourMap = new Map();
    stats.hourly_stats.forEach(stat => {
      const hour = stat.hour || '00:00';
      if (hourMap.has(hour)) {
        const existing = hourMap.get(hour);
        hourMap.set(hour, {
          hour,
          sent: (existing.sent || 0) + (stat.sent || 0),
          failed: (existing.failed || 0) + (stat.failed || 0),
          queued: (existing.queued || 0) + (stat.queued || 0),
          avg_processing_time: Math.max(existing.avg_processing_time || 0, stat.avg_processing_time || 0),
        });
      } else {
        hourMap.set(hour, {
          hour,
          sent: stat.sent || 0,
          failed: stat.failed || 0,
          queued: stat.queued || 0,
          avg_processing_time: stat.avg_processing_time || 0,
        });
      }
    });
    
    // Convert to array and sort by hour
    const result = Array.from(hourMap.values()).sort((a, b) => {
      // Simple hour comparison (assumes HH:MM format)
      return a.hour.localeCompare(b.hour);
    });
    
    return result.length > 0 ? result : [{ hour: '00:00', sent: 0, failed: 0, queued: 0, avg_processing_time: 0 }];
  }, [stats?.hourly_stats]);

  const statusData = React.useMemo(() => {
    if (!stats) return [];
    const data = [
      { name: 'Queued', value: stats.messages_queued || 0, color: statusColors.queued },
      { name: 'Processing', value: stats.messages_processing || 0, color: statusColors.processing },
      { name: 'Sent', value: stats.messages_sent || 0, color: statusColors.sent },
      { name: 'Failed', value: stats.messages_failed || 0, color: statusColors.failed },
    ];
    // Filter out zero values for cleaner pie chart
    return data.filter(d => d.value > 0);
  }, [stats]);

  const providerData = React.useMemo(() => {
    if (!stats?.provider_stats || stats.provider_stats.length === 0) {
      return [];
    }
    return stats.provider_stats.map(stat => ({
      provider: stat.provider || 'Unknown',
      sent: stat.sent || 0,
      failed: stat.failed || 0,
    }));
  }, [stats?.provider_stats]);

  if (statsLoading || rateLimitsLoading || healthLoading) {
    return <LinearProgress />;
  }

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
          overflow: 'visible'
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
          overflow: 'visible'
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
          overflow: 'visible'
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
          overflow: 'visible'
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
          {hourlyData && hourlyData.length > 0 ? (
            <ResponsiveContainer width="100%" height={300}>
              <AreaChart data={hourlyData} isAnimationActive={false}>
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
                  animationDuration={0}
                  isAnimationActive={false}
                />
                <Area
                  type="monotone"
                  dataKey="failed"
                  stackId="1"
                  stroke={statusColors.failed}
                  fill={statusColors.failed}
                  name="Failed"
                  animationDuration={0}
                  isAnimationActive={false}
                />
                <Area
                  type="monotone"
                  dataKey="queued"
                  stackId="1"
                  stroke={statusColors.queued}
                  fill={statusColors.queued}
                  name="Queued"
                  animationDuration={0}
                  isAnimationActive={false}
                />
              </AreaChart>
            </ResponsiveContainer>
          ) : (
            <Box sx={{ height: 300, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <Typography color="text.secondary">No data available</Typography>
            </Box>
          )}
        </Paper>
      </GridLegacy>

      {/* Status Distribution */}
      <GridLegacy item xs={12} lg={4}>
        <Paper sx={{ p: 2 }}>
          <Typography variant="h6" gutterBottom>
            Status Distribution
          </Typography>
          {statusData && statusData.length > 0 ? (
            <ResponsiveContainer width="100%" height={300}>
              <PieChart isAnimationActive={false}>
                <Pie
                  data={statusData}
                  cx="50%"
                  cy="50%"
                  labelLine={false}
                  label={({ name, percent }) => `${name} ${((percent ?? 0) * 100).toFixed(0)}%`}
                  outerRadius={80}
                  fill="#8884d8"
                  dataKey="value"
                  isAnimationActive={false}
                >
                  {statusData.map((entry, index) => (
                    <Cell key={`cell-${index}`} fill={entry.color} />
                  ))}
                </Pie>
                <Tooltip />
              </PieChart>
            </ResponsiveContainer>
          ) : (
            <Box sx={{ height: 300, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <Typography color="text.secondary">No messages yet</Typography>
            </Box>
          )}
        </Paper>
      </GridLegacy>

      {/* Provider Performance */}
      <GridLegacy item xs={12} lg={6}>
        <Paper sx={{ p: 2 }}>
          <Typography variant="h6" gutterBottom>
            Provider Performance
          </Typography>
          {providerData && providerData.length > 0 ? (
            <ResponsiveContainer width="100%" height={300}>
              <BarChart data={providerData} isAnimationActive={false}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="provider" />
                <YAxis />
                <Tooltip />
                <Legend />
                <Bar dataKey="sent" fill={statusColors.sent} name="Sent" isAnimationActive={false} />
                <Bar dataKey="failed" fill={statusColors.failed} name="Failed" isAnimationActive={false} />
              </BarChart>
            </ResponsiveContainer>
          ) : (
            <Box sx={{ height: 300, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <Typography color="text.secondary">No provider data available</Typography>
            </Box>
          )}
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
          {hourlyData && hourlyData.length > 0 ? (
            <ResponsiveContainer width="100%" height={250}>
              <LineChart data={hourlyData} isAnimationActive={false}>
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
                  isAnimationActive={false}
                />
              </LineChart>
            </ResponsiveContainer>
          ) : (
            <Box sx={{ height: 250, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <Typography color="text.secondary">No timing data available</Typography>
            </Box>
          )}
        </Paper>
      </GridLegacy>
    </GridLegacy>
  );
}

export default React.memo(MetricsPage);