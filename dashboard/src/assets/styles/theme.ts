import { createTheme, ThemeOptions } from '@mui/material/styles';

// Mednet color constants
export const mednet_blue = '#303B8B';
export const color_blue = '#3F49A7';
export const color_blue_bright = '#2979FF';
export const color_blue_bright_2 = '#1565C0';
export const color_blue_gray = '#465774';

export const color_gray_border = '#E5E5E5';
export const color_gray_background = '#F5F7F8';
export const color_gray_text = '#727C8D';
export const dark_blue_text = '#3D4D69';

export const color_black_main = '#1A1A1A';
export const color_black_secondary = '#404040';

export const color_white_background = '#FAFAFA';
export const color_light_blue_background = '#E6F5FC';

export const color_red_warning = '#DC4A3E';
export const color_yellow_warning = '#FFCC00';
export const color_orange_warning = '#FF7900';
export const color_green_success = '#4BB543';

// Chart colors
export const chart_primary_color = '#0088FE';
export const chart_secondary_color = '#00C49F';
export const chart_tertiary_color = '#FFBB28';
export const chart_quaternary_color = '#FF8042';

const themeOptions: ThemeOptions = {
  palette: {
    primary: {
      main: mednet_blue,
      light: color_blue,
      dark: '#1F2147',
    },
    secondary: {
      main: color_blue_bright,
      light: color_light_blue_background,
      dark: color_blue_bright_2,
    },
    error: {
      main: color_red_warning,
    },
    warning: {
      main: color_yellow_warning,
    },
    success: {
      main: color_green_success,
    },
    info: {
      main: color_blue_bright_2,
    },
    grey: {
      50: color_white_background,
      100: color_gray_background,
      200: color_gray_border,
      300: color_gray_text,
      700: color_black_secondary,
      900: color_black_main,
    },
    background: {
      default: color_gray_background,
      paper: '#FFFFFF',
    },
    text: {
      primary: color_black_main,
      secondary: color_gray_text,
    },
  },
  typography: {
    fontFamily: [
      '-apple-system',
      'BlinkMacSystemFont',
      '"Segoe UI"',
      'Roboto',
      '"Helvetica Neue"',
      'Arial',
      'sans-serif',
    ].join(','),
    h1: {
      fontSize: '2.5rem',
      fontWeight: 600,
      color: dark_blue_text,
    },
    h2: {
      fontSize: '2rem',
      fontWeight: 600,
      color: dark_blue_text,
    },
    h3: {
      fontSize: '1.75rem',
      fontWeight: 600,
      color: dark_blue_text,
    },
    h4: {
      fontSize: '1.5rem',
      fontWeight: 600,
      color: dark_blue_text,
    },
    h5: {
      fontSize: '1.25rem',
      fontWeight: 600,
      color: dark_blue_text,
    },
    h6: {
      fontSize: '1rem',
      fontWeight: 600,
      color: dark_blue_text,
    },
    body1: {
      fontSize: '1rem',
    },
    body2: {
      fontSize: '0.875rem',
    },
  },
  shape: {
    borderRadius: 8,
  },
  components: {
    MuiCard: {
      styleOverrides: {
        root: {
          boxShadow: '0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06)',
          '&:hover': {
            boxShadow: '0 10px 15px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -2px rgba(0, 0, 0, 0.05)',
          },
        },
      },
    },
    MuiButton: {
      styleOverrides: {
        root: {
          textTransform: 'none',
          fontWeight: 600,
        },
      },
    },
    MuiTab: {
      styleOverrides: {
        root: {
          textTransform: 'none',
          fontWeight: 600,
        },
      },
    },
    MuiPaper: {
      styleOverrides: {
        root: {
          borderRadius: 8,
        },
      },
    },
  },
};

export const theme = createTheme(themeOptions);

// Chart color palette
export const ChartPalette = {
  colors: [
    chart_primary_color,
    chart_secondary_color,
    chart_tertiary_color,
    chart_quaternary_color,
    '#8884d8',
    '#82ca9d',
    '#ffc658',
  ],
  status: {
    queued: color_yellow_warning,
    processing: color_blue_bright,
    sent: color_green_success,
    failed: color_red_warning,
    auth_error: color_orange_warning,
  },
  providers: {
    gmail: '#4285F4',
    mailgun: '#FF5A00',
    mandrill: '#3EBCE6',
  },
};