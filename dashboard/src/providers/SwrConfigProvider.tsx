'use client';

import React from 'react';
import { SWRConfig } from 'swr';

export default function SwrConfigProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <SWRConfig
      value={{
        revalidateOnFocus: false,
        shouldRetryOnError: true,
        errorRetryCount: 3,
        errorRetryInterval: 5000,
        onError: (error) => {
          if (error.status === 401) {
            // Handle unauthorized
            console.error('Unauthorized access');
          }
        },
      }}
    >
      {children}
    </SWRConfig>
  );
}