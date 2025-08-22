'use client';

import React from 'react';
import { usePathname, useRouter } from 'next/navigation';
import { 
  Box, 
  Tabs, 
  Tab, 
  Paper, 
  Container,
  Typography,
  AppBar,
  Toolbar,
  Avatar,
  IconButton,
  Badge,
  Fade,
  useTheme
} from '@mui/material';
import {
  BarChart as MetricsIcon,
  CloudQueue as ProvidersIcon,
  Hub as PoolsIcon,
  Email as MessagesIcon,
  Notifications as NotificationsIcon,
  Settings as SettingsIcon,
  Speed as SpeedIcon
} from '@mui/icons-material';

interface TabItem {
  label: string;
  path: string;
  icon: React.ReactElement;
}

const tabs: TabItem[] = [
  { label: 'Metrics', path: '/dashboard/metrics', icon: <MetricsIcon /> },
  { label: 'Providers', path: '/dashboard/providers', icon: <ProvidersIcon /> },
  { label: 'Pools', path: '/dashboard/pools', icon: <PoolsIcon /> },
  { label: 'Messages', path: '/dashboard/messages', icon: <MessagesIcon /> },
];

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const theme = useTheme();

  const currentTab = tabs.findIndex(tab => pathname === tab.path);

  const handleTabChange = (_event: React.SyntheticEvent, newValue: number) => {
    router.push(tabs[newValue].path);
  };

  return (
    <Box sx={{ 
      flexGrow: 1, 
      bgcolor: 'background.default', 
      minHeight: '100vh',
      background: 'linear-gradient(180deg, #F8FAFB 0%, #F1F5F9 100%)'
    }}>
      <AppBar 
        position="sticky" 
        elevation={0}
        sx={{
          backgroundColor: 'rgba(255, 255, 255, 0.95)',
          backdropFilter: 'blur(20px)',
          borderBottom: '1px solid',
          borderColor: 'divider',
        }}
      >
        <Toolbar sx={{ height: 70 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', flexGrow: 1 }}>
            <Box
              sx={{
                width: 40,
                height: 40,
                borderRadius: 2,
                background: 'linear-gradient(135deg, #303B8B 0%, #5969C5 100%)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                mr: 2,
                boxShadow: '0 4px 12px rgba(48, 59, 139, 0.2)',
              }}
            >
              <SpeedIcon sx={{ color: 'white', fontSize: 24 }} />
            </Box>
            <Box>
              <Typography 
                variant="h5" 
                component="div" 
                sx={{ 
                  fontWeight: 700,
                  color: theme.palette.grey[900],
                  letterSpacing: '-0.02em',
                }}
              >
                Relay Dashboard
              </Typography>
              <Typography 
                variant="caption" 
                sx={{ 
                  color: theme.palette.grey[600],
                  fontWeight: 500,
                }}
              >
                Email Infrastructure Management
              </Typography>
            </Box>
          </Box>
          
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <IconButton
              size="large"
              sx={{
                backgroundColor: theme.palette.grey[100],
                '&:hover': {
                  backgroundColor: theme.palette.grey[200],
                },
              }}
            >
              <Badge badgeContent={3} color="error">
                <NotificationsIcon />
              </Badge>
            </IconButton>
            
            <IconButton
              size="large"
              sx={{
                backgroundColor: theme.palette.grey[100],
                '&:hover': {
                  backgroundColor: theme.palette.grey[200],
                },
              }}
            >
              <SettingsIcon />
            </IconButton>
            
            <Avatar 
              sx={{ 
                ml: 2,
                width: 42,
                height: 42,
                bgcolor: theme.palette.primary.main,
                fontWeight: 600,
                boxShadow: '0 4px 12px rgba(48, 59, 139, 0.2)',
              }}
            >
              M
            </Avatar>
          </Box>
        </Toolbar>
      </AppBar>
      
      <Container maxWidth="xl" sx={{ mt: 4, px: { xs: 2, sm: 3, md: 4 } }}>
        <Fade in timeout={500}>
          <Paper 
            sx={{ 
              mb: 4,
              overflow: 'hidden',
              backgroundColor: 'white',
              boxShadow: '0px 4px 24px rgba(0, 0, 0, 0.06)',
              border: 'none',
            }}
          >
            <Tabs
              value={currentTab >= 0 ? currentTab : 0}
              onChange={handleTabChange}
              indicatorColor="primary"
              textColor="primary"
              variant="fullWidth"
              sx={{
                '& .MuiTabs-flexContainer': {
                  backgroundColor: 'white',
                },
                '& .MuiTab-root': {
                  py: 2.5,
                  fontSize: '0.95rem',
                  fontWeight: 600,
                  color: theme.palette.grey[600],
                  transition: 'all 0.2s ease',
                  '&:hover': {
                    backgroundColor: theme.palette.grey[50],
                    color: theme.palette.primary.main,
                  },
                  '&.Mui-selected': {
                    color: theme.palette.primary.main,
                    backgroundColor: 'rgba(48, 59, 139, 0.04)',
                  },
                },
                '& .MuiTabs-indicator': {
                  height: 3,
                  backgroundColor: theme.palette.primary.main,
                },
              }}
            >
              {tabs.map((tab) => (
                <Tab
                  key={tab.path}
                  label={
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                      {React.cloneElement(tab.icon, { 
                        sx: { fontSize: 20 } 
                      })}
                      <span>{tab.label}</span>
                    </Box>
                  }
                />
              ))}
            </Tabs>
          </Paper>
        </Fade>

        <Fade in timeout={700}>
          <Box sx={{ 
            animation: 'fadeInUp 0.5s ease-out',
            '@keyframes fadeInUp': {
              from: {
                opacity: 0,
                transform: 'translateY(20px)',
              },
              to: {
                opacity: 1,
                transform: 'translateY(0)',
              },
            },
          }}>
            {children}
          </Box>
        </Fade>
      </Container>
    </Box>
  );
}