import type { Metadata } from 'next';
import { Inter } from 'next/font/google';
import MuiThemeProvider from '@/providers/MuiThemeProvider';
import SwrConfigProvider from '@/providers/SwrConfigProvider';
import './globals.css';

const inter = Inter({ subsets: ['latin'] });

export const metadata: Metadata = {
  title: 'SMTP Relay Dashboard',
  description: 'Mednet SMTP Relay Service Dashboard',
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body className={inter.className}>
        <MuiThemeProvider>
          <SwrConfigProvider>
            {children}
          </SwrConfigProvider>
        </MuiThemeProvider>
      </body>
    </html>
  );
}
