import { createTheme } from '@mui/material/styles';

const theme = createTheme({
  palette: {
    mode: 'light',
    primary: {
      main: '#303B8B', // Mednet blue
      light: '#5969C5',
      dark: '#1E2659',
    },
    secondary: {
      main: '#00BFA5',
      light: '#5EFDD7',
      dark: '#008E76',
    },
    background: {
      default: '#F8FAFB',
      paper: '#FFFFFF',
    },
    grey: {
      50: '#FAFBFC',
      100: '#F4F6F8',
      200: '#E9ECF0',
      300: '#D8DCE2',
      400: '#B8BFC9',
      500: '#8B95A3',
      600: '#6B7785',
      700: '#4A5568',
      800: '#2D3748',
      900: '#1A202C',
    },
    success: {
      main: '#10B981',
      light: '#34D399',
      dark: '#059669',
    },
    error: {
      main: '#EF4444',
      light: '#F87171',
      dark: '#DC2626',
    },
    warning: {
      main: '#F59E0B',
      light: '#FCD34D',
      dark: '#D97706',
    },
  },
  typography: {
    fontFamily: [
      'Inter',
      '-apple-system',
      'BlinkMacSystemFont',
      '"Segoe UI"',
      'Roboto',
      '"Helvetica Neue"',
      'Arial',
      'sans-serif',
    ].join(','),
    fontWeightLight: 400,
    fontWeightRegular: 500,
    fontWeightMedium: 600,
    fontWeightBold: 700,
    h1: {
      fontSize: '2.5rem',
      fontWeight: 700,
      lineHeight: 1.2,
      letterSpacing: '-0.02em',
    },
    h2: {
      fontSize: '2rem',
      fontWeight: 700,
      lineHeight: 1.3,
      letterSpacing: '-0.01em',
    },
    h3: {
      fontSize: '1.75rem',
      fontWeight: 600,
      lineHeight: 1.4,
      letterSpacing: '-0.01em',
    },
    h4: {
      fontSize: '1.5rem',
      fontWeight: 600,
      lineHeight: 1.4,
    },
    h5: {
      fontSize: '1.25rem',
      fontWeight: 600,
      lineHeight: 1.5,
    },
    h6: {
      fontSize: '1.125rem',
      fontWeight: 600,
      lineHeight: 1.5,
    },
    body1: {
      fontSize: '1rem',
      fontWeight: 500,
      lineHeight: 1.6,
    },
    body2: {
      fontSize: '0.875rem',
      fontWeight: 500,
      lineHeight: 1.5,
    },
    button: {
      fontWeight: 600,
      textTransform: 'none',
      letterSpacing: '0.02em',
    },
  },
  shape: {
    borderRadius: 12,
  },
  shadows: [
    'none',
    '0px 2px 4px rgba(0,0,0,0.04)',
    '0px 4px 8px rgba(0,0,0,0.06)',
    '0px 8px 16px rgba(0,0,0,0.08)',
    '0px 12px 24px rgba(0,0,0,0.10)',
    '0px 16px 32px rgba(0,0,0,0.12)',
    '0px 20px 40px rgba(0,0,0,0.14)',
    '0px 24px 48px rgba(0,0,0,0.16)',
    '0px 28px 56px rgba(0,0,0,0.18)',
    '0px 32px 64px rgba(0,0,0,0.20)',
    '0px 36px 72px rgba(0,0,0,0.22)',
    '0px 40px 80px rgba(0,0,0,0.24)',
    '0px 44px 88px rgba(0,0,0,0.26)',
    '0px 48px 96px rgba(0,0,0,0.28)',
    '0px 52px 104px rgba(0,0,0,0.30)',
    '0px 56px 112px rgba(0,0,0,0.32)',
    '0px 60px 120px rgba(0,0,0,0.34)',
    '0px 64px 128px rgba(0,0,0,0.36)',
    '0px 68px 136px rgba(0,0,0,0.38)',
    '0px 72px 144px rgba(0,0,0,0.40)',
    '0px 76px 152px rgba(0,0,0,0.42)',
    '0px 80px 160px rgba(0,0,0,0.44)',
    '0px 84px 168px rgba(0,0,0,0.46)',
    '0px 88px 176px rgba(0,0,0,0.48)',
    '0px 92px 184px rgba(0,0,0,0.50)',
  ],
  components: {
    MuiButton: {
      styleOverrides: {
        root: {
          borderRadius: 10,
          padding: '10px 20px',
          fontSize: '0.95rem',
          boxShadow: 'none',
          transition: 'all 0.2s ease-in-out',
          '&:hover': {
            transform: 'translateY(-2px)',
            boxShadow: '0px 8px 24px rgba(0,0,0,0.15)',
          },
        },
        contained: {
          boxShadow: '0px 4px 12px rgba(48, 59, 139, 0.15)',
          '&:hover': {
            boxShadow: '0px 8px 24px rgba(48, 59, 139, 0.25)',
          },
        },
      },
    },
    MuiCard: {
      styleOverrides: {
        root: {
          borderRadius: 16,
          boxShadow: '0px 4px 20px rgba(0,0,0,0.08)',
          border: '1px solid rgba(0,0,0,0.06)',
          transition: 'all 0.3s ease-in-out',
          '&:hover': {
            boxShadow: '0px 8px 32px rgba(0,0,0,0.12)',
            transform: 'translateY(-4px)',
          },
        },
      },
    },
    MuiPaper: {
      styleOverrides: {
        root: {
          borderRadius: 16,
          boxShadow: '0px 4px 20px rgba(0,0,0,0.08)',
          border: '1px solid rgba(0,0,0,0.06)',
        },
      },
    },
    MuiAppBar: {
      styleOverrides: {
        root: {
          backgroundColor: '#FFFFFF',
          color: '#1A202C',
          boxShadow: '0px 1px 3px rgba(0,0,0,0.08)',
          borderBottom: '1px solid rgba(0,0,0,0.06)',
        },
      },
    },
    MuiChip: {
      styleOverrides: {
        root: {
          borderRadius: 8,
          fontWeight: 600,
          fontSize: '0.85rem',
        },
      },
    },
    MuiTextField: {
      styleOverrides: {
        root: {
          '& .MuiOutlinedInput-root': {
            borderRadius: 10,
            '& fieldset': {
              borderColor: 'rgba(0,0,0,0.12)',
            },
            '&:hover fieldset': {
              borderColor: 'rgba(48, 59, 139, 0.3)',
            },
            '&.Mui-focused fieldset': {
              borderColor: '#303B8B',
              borderWidth: 2,
            },
          },
        },
      },
    },
    MuiTab: {
      styleOverrides: {
        root: {
          fontWeight: 600,
          fontSize: '0.95rem',
          textTransform: 'none',
          minHeight: 48,
          padding: '12px 24px',
          '&.Mui-selected': {
            color: '#303B8B',
          },
        },
      },
    },
    MuiTabs: {
      styleOverrides: {
        indicator: {
          height: 3,
          borderRadius: '3px 3px 0 0',
          backgroundColor: '#303B8B',
        },
      },
    },
  },
});

export default theme;