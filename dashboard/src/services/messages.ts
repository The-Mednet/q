import useSWR from 'swr';
import { api } from './network';
import { Message } from '@/types/relay';

export interface MessagesResponse {
  messages: Message[];
  total: number;
  offset: number;
  limit: number;
}

export interface MessageParams {
  limit?: number;
  offset?: number;
  status?: string;
  search?: string;
}

// Messages API hooks - overloaded for compatibility
export function useMessages(offset?: number, limit?: number, search?: string): ReturnType<typeof useSWR<MessagesResponse>>;
export function useMessages(params: MessageParams): ReturnType<typeof useSWR<MessagesResponse>>;
export function useMessages(
  offsetOrParams?: number | MessageParams,
  limit?: number,
  search?: string
) {
  let queryString: string;
  
  if (typeof offsetOrParams === 'object') {
    // New style with params object
    queryString = new URLSearchParams({
      limit: String(offsetOrParams.limit || 25),
      offset: String(offsetOrParams.offset || 0),
      ...(offsetOrParams.status && offsetOrParams.status !== 'all' && { status: offsetOrParams.status }),
      ...(offsetOrParams.search && { search: offsetOrParams.search }),
    }).toString();
  } else {
    // Legacy style with positional parameters
    queryString = new URLSearchParams({
      limit: String(limit || 25),
      offset: String(offsetOrParams || 0),
      ...(search && { search }),
    }).toString();
  }

  return useSWR<MessagesResponse>(
    `/messages?${queryString}`,
    api.get,
    {
      refreshInterval: 10000,
      revalidateOnFocus: true,
    }
  );
}

export function useMessage(id: string | null) {
  return useSWR<Message>(
    id ? `/messages/${id}` : null,
    api.get
  );
}

// Message operations
export async function deleteMessage(id: string) {
  return api.delete(`/messages/${id}`);
}

export async function retryMessage(id: string) {
  return api.post(`/messages/${id}/retry`);
}

export async function resendMessage(id: string) {
  return api.post(`/messages/${id}/resend`, {});
}

export async function getMessageDetails(id: string): Promise<Message> {
  return api.get(`/messages/${id}`);
}