// Shared foundation every domain file under services/ builds on: the
// configured API client, the shared QueryClient instance, and the
// response-handling helper every hook funnels through. Kept separate from
// the domain files (users.ts, statistics.ts, tokens.ts, counters.ts,
// rateLimiter.ts, middleware.ts) so none of them need to import from each
// other just to get at these.
import { createClient } from '@/apiclient/client';
import { QueryClient } from '@tanstack/react-query';

export const customApiClient = createClient({
  baseUrl: '/_',
});

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

// The shape every generated sdk.gen.ts call resolves to, loosely — a real
// call's response/error types are a much more specific discriminated union
// (see apiclient/client/types.gen.ts's RequestResult), but every one of
// them structurally satisfies this, and this is all handleResponse actually
// touches.
export interface ApiResult<T> {
  data?: T;
  error?: { message?: string };
}

// Helper to handle API responses: throws on error, returns data
export function handleResponse<T>(response: ApiResult<T>): T {
  if (response && response.error) {
    // Throw a real Error (not the raw { code, message } API error object)
    // so `error instanceof Error` checks and String(error) work consistently
    // wherever a query/mutation's error is rendered, not just the call
    // sites that know to reach for `.message` explicitly.
    throw new Error(response.error.message || 'Request failed');
  }
  return response.data as T;
}
