// Network service following Mednet pattern

export class FetchError extends Error {
  constructor(
    public status: number,
    public message: string
  ) {
    super(message);
    this.status = status;
    this.message = message;
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
    const errorText = await response.text();
    throw new FetchError(response.status, errorText || response.statusText);
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