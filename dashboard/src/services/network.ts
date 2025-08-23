// Network service following Mednet pattern

export class FetchError extends Error {
  constructor(
    public status: number,
    public message: string,
    public response?: Response
  ) {
    super(message);
    this.name = 'FetchError';
    this.status = status;
    this.message = message;
    this.response = response;
  }
}

const BASE_URL = process.env.NEXT_PUBLIC_API_URL || '';
const API_PREFIX = 'api/';

export interface FetchRequestInit extends RequestInit {
  json?: object;
}

function handleRequestOptions(
  options?: FetchRequestInit
): FetchRequestInit {
  if (!options) {
    options = {};
  }

  if (options.json) {
    options.headers = new Headers(options.headers ?? {});
    options.headers.set('Content-Type', 'application/json');
    options.body = JSON.stringify(options.json);
    delete options.json;
  }

  return options;
}

export async function fetcher<T>(
  path: string,
  options?: FetchRequestInit
): Promise<T> {
  const url = path.startsWith('http') 
    ? path 
    : path.startsWith('/api/')
    ? `${BASE_URL}${path}`
    : `${BASE_URL}/${API_PREFIX}${path}`;

  const processedOptions = handleRequestOptions(options);

  const response = await fetch(url, processedOptions);

  if (!response.ok) {
    let errorMessage = response.statusText;
    
    try {
      const errorData = await response.json();
      errorMessage = errorData.message || errorData.error || errorMessage;
    } catch {
      // If JSON parsing fails, try text
      try {
        const errorText = await response.text();
        errorMessage = errorText || errorMessage;
      } catch {
        // Use status text as fallback
      }
    }
    
    throw new FetchError(response.status, errorMessage, response);
  }

  // Handle empty responses
  const contentType = response.headers.get('content-type');
  if (!contentType || !contentType.includes('application/json')) {
    return {} as T;
  }

  return response.json();
}

// Convenience methods
export const api = {
  get: <T>(path: string) => fetcher<T>(path),
  
  post: <T>(path: string, data?: object) => 
    fetcher<T>(path, { method: 'POST', json: data }),
  
  put: <T>(path: string, data?: object) => 
    fetcher<T>(path, { method: 'PUT', json: data }),
  
  delete: <T>(path: string) => 
    fetcher<T>(path, { method: 'DELETE' }),
};