import { TaronjaGatewayError } from './errors';
import type {
    AllUserCountersResponse,
    AvailableCountersResponse,
    CounterAdjustmentRequest,
    CounterHistoryResponse,
    CounterTransactionResponse,
    CurrentUser,
    DateRangeQuery,
    ErrorResponse,
    HealthResponse,
    LogoutResult,
    PaginationQuery,
    RateLimiterConfigResponse,
    RateLimiterStats,
    RequestDetailsResponse,
    RequestStatistics,
    TokenCreateRequest,
    TokenCreateResponse,
    TokenResponse,
    UserCountersResponse,
    UserCreateRequest,
    UserResponse,
} from './types';

type Primitive = string | number | boolean | null | undefined;
type QueryValue = Primitive | Primitive[];
type QueryParams = Record<string, QueryValue>;

export interface TaronjaClientOptions {
    baseUrl?: string;
    credentials?: RequestCredentials;
    headers?: HeadersInit;
    fetch?: typeof fetch;
    loginPath?: string;
    logoutPath?: string;
    mePath?: string;
    openApiPath?: string;
}

export interface RequestOptions {
    signal?: AbortSignal;
}

export interface LogoutOptions extends RequestOptions {
    redirectTo?: string;
}

export interface CounterLookupOptions extends RequestOptions, PaginationQuery {
}

export interface TaronjaClient {
    getLoginUrl(options?: { redirectTo?: string }): string;
    getHealth(options?: RequestOptions): Promise<HealthResponse>;
    getCurrentUser(options?: RequestOptions): Promise<CurrentUser | null>;
    logout(options?: LogoutOptions): Promise<LogoutResult>;
    getOpenApiYaml(options?: RequestOptions): Promise<string>;
    listUsers(options?: RequestOptions): Promise<UserResponse[]>;
    getUserById(userId: string, options?: RequestOptions): Promise<UserResponse>;
    createUser(payload: UserCreateRequest, options?: RequestOptions): Promise<UserResponse>;
    listTokens(userId: string, options?: RequestOptions): Promise<TokenResponse[]>;
    getToken(tokenId: string, options?: RequestOptions): Promise<TokenResponse>;
    createToken(userId: string, payload: TokenCreateRequest, options?: RequestOptions): Promise<TokenCreateResponse>;
    deleteToken(tokenId: string, options?: RequestOptions): Promise<void>;
    getRequestStatistics(query?: DateRangeQuery & RequestOptions): Promise<RequestStatistics>;
    getRequestDetails(query?: DateRangeQuery & RequestOptions): Promise<RequestDetailsResponse>;
    getRateLimiterStats(options?: RequestOptions): Promise<RateLimiterStats>;
    getRateLimiterConfig(options?: RequestOptions): Promise<RateLimiterConfigResponse>;
    getAvailableCounters(options?: RequestOptions): Promise<AvailableCountersResponse>;
    getAllUserCounters(counterId: string, options?: CounterLookupOptions): Promise<AllUserCountersResponse>;
    getUserCounters(counterId: string, userId: string, options?: RequestOptions): Promise<UserCountersResponse>;
    getUserCounterHistory(counterId: string, userId: string, options?: CounterLookupOptions): Promise<CounterHistoryResponse>;
    adjustUserCounters(counterId: string, userId: string, payload: CounterAdjustmentRequest, options?: RequestOptions): Promise<CounterTransactionResponse>;
}

const DEFAULT_BASE_URL = '/_';
const DEFAULT_LOGIN_PATH = '/login';
const DEFAULT_LOGOUT_PATH = '/logout';
const DEFAULT_ME_PATH = '/me';
const DEFAULT_OPEN_API_PATH = '/openapi.yaml';

function getFetchImplementation(fetchImplementation?: typeof fetch): typeof fetch {
    if (fetchImplementation) {
        return fetchImplementation;
    }

    if (typeof globalThis.fetch === 'function') {
        return globalThis.fetch.bind(globalThis);
    }

    throw new TaronjaGatewayError('No fetch implementation available.');
}

function joinUrl(baseUrl: string, path: string): string {
    const normalizedBaseUrl = baseUrl.endsWith('/') ? baseUrl.slice(0, -1) : baseUrl;
    const normalizedPath = path.startsWith('/') ? path : `/${path}`;
    return `${normalizedBaseUrl}${normalizedPath}`;
}

function buildQueryString(query?: QueryParams): string {
    if (!query) {
        return '';
    }

    const searchParams = new URLSearchParams();

    for (const [key, value] of Object.entries(query)) {
        if (value === undefined || value === null) {
            continue;
        }

        if (Array.isArray(value)) {
            for (const item of value) {
                if (item !== undefined && item !== null) {
                    searchParams.append(key, String(item));
                }
            }
            continue;
        }

        searchParams.set(key, String(value));
    }

    const serializedQuery = searchParams.toString();
    return serializedQuery ? `?${serializedQuery}` : '';
}

async function parseResponseBody(response: Response): Promise<unknown> {
    if (response.status === 204) {
        return undefined;
    }

    const contentType = response.headers.get('content-type') ?? '';
    if (contentType.includes('application/json')) {
        return response.json();
    }

    return response.text();
}

function getErrorMessage(status: number, body: unknown): string {
    if (body && typeof body === 'object' && 'message' in body) {
        const message = (body as ErrorResponse).message;
        if (typeof message === 'string' && message.length > 0) {
            return message;
        }
    }

    return `Taronja Gateway request failed with status ${status}.`;
}

export function isAuthenticatedUser(user: CurrentUser | null | undefined): user is CurrentUser {
    return Boolean(user?.authenticated);
}

export function createTaronjaClient(options: TaronjaClientOptions = {}): TaronjaClient {
    const baseUrl = options.baseUrl ?? DEFAULT_BASE_URL;
    const credentials = options.credentials ?? 'same-origin';
    const defaultHeaders = options.headers;
    const fetchImplementation = getFetchImplementation(options.fetch);
    const loginPath = options.loginPath ?? DEFAULT_LOGIN_PATH;
    const logoutPath = options.logoutPath ?? DEFAULT_LOGOUT_PATH;
    const mePath = options.mePath ?? DEFAULT_ME_PATH;
    const openApiPath = options.openApiPath ?? DEFAULT_OPEN_API_PATH;

    async function request<T>(
        path: string,
        init: RequestInit = {},
        query?: QueryParams,
        responseType: 'json' | 'text' | 'none' = 'json',
    ): Promise<T> {
        const response = await fetchImplementation(`${joinUrl(baseUrl, path)}${buildQueryString(query)}`, {
            ...init,
            credentials,
            headers: {
                Accept: responseType === 'json' ? 'application/json' : '*/*',
                ...defaultHeaders,
                ...init.headers,
            },
        });

        if (!response.ok) {
            const responseBody = await parseResponseBody(response);
            throw new TaronjaGatewayError(getErrorMessage(response.status, responseBody), response.status, responseBody);
        }

        if (responseType === 'none') {
            return undefined as T;
        }

        if (responseType === 'text') {
            return response.text() as Promise<T>;
        }

        return response.json() as Promise<T>;
    }

    return {
        getLoginUrl(loginOptions = {}) {
            return `${joinUrl(baseUrl, loginPath)}${buildQueryString({ redirect: loginOptions.redirectTo })}`;
        },

        getHealth(options) {
            return request<HealthResponse>('/health', { method: 'GET', signal: options?.signal });
        },

        async getCurrentUser(options) {
            const response = await fetchImplementation(joinUrl(baseUrl, mePath), {
                method: 'GET',
                credentials,
                signal: options?.signal,
                headers: {
                    Accept: 'application/json',
                    ...defaultHeaders,
                },
            });

            if (response.status === 401) {
                return null;
            }

            if (!response.ok) {
                const responseBody = await parseResponseBody(response);
                throw new TaronjaGatewayError(getErrorMessage(response.status, responseBody), response.status, responseBody);
            }

            const user = await response.json() as CurrentUser;
            if (typeof user.authenticated !== 'boolean') {
                throw new TaronjaGatewayError('Current user response is missing the authenticated flag.', response.status, user);
            }

            return user;
        },

        async logout(logoutOptions) {
            const response = await fetchImplementation(`${joinUrl(baseUrl, logoutPath)}${buildQueryString({ redirect: logoutOptions?.redirectTo })}`, {
                method: 'GET',
                credentials,
                signal: logoutOptions?.signal,
                headers: {
                    Accept: '*/*',
                    ...defaultHeaders,
                },
                redirect: 'follow',
            });

            if (response.status >= 400) {
                const responseBody = await parseResponseBody(response);
                throw new TaronjaGatewayError(getErrorMessage(response.status, responseBody), response.status, responseBody);
            }

            return {
                redirected: response.redirected,
                status: response.status,
                url: response.url,
            };
        },

        getOpenApiYaml(options) {
            return request<string>(openApiPath, { method: 'GET', signal: options?.signal }, undefined, 'text');
        },

        listUsers(options) {
            return request<UserResponse[]>('/api/users', { method: 'GET', signal: options?.signal });
        },

        getUserById(userId, options) {
            return request<UserResponse>(`/api/users/${encodeURIComponent(userId)}`, { method: 'GET', signal: options?.signal });
        },

        createUser(payload, options) {
            return request<UserResponse>('/api/users', {
                method: 'POST',
                signal: options?.signal,
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify(payload),
            });
        },

        listTokens(userId, options) {
            return request<TokenResponse[]>(`/api/users/${encodeURIComponent(userId)}/tokens`, {
                method: 'GET',
                signal: options?.signal,
            });
        },

        getToken(tokenId, options) {
            return request<TokenResponse>(`/api/tokens/${encodeURIComponent(tokenId)}`, {
                method: 'GET',
                signal: options?.signal,
            });
        },

        createToken(userId, payload, options) {
            return request<TokenCreateResponse>(`/api/users/${encodeURIComponent(userId)}/tokens`, {
                method: 'POST',
                signal: options?.signal,
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify(payload),
            });
        },

        deleteToken(tokenId, options) {
            return request<void>(`/api/tokens/${encodeURIComponent(tokenId)}`, {
                method: 'DELETE',
                signal: options?.signal,
            }, undefined, 'none');
        },

        getRequestStatistics(query) {
            return request<RequestStatistics>('/api/statistics/requests', {
                method: 'GET',
                signal: query?.signal,
            }, {
                start_date: query?.startDate,
                end_date: query?.endDate,
            });
        },

        getRequestDetails(query) {
            return request<RequestDetailsResponse>('/api/statistics/requests/details', {
                method: 'GET',
                signal: query?.signal,
            }, {
                start_date: query?.startDate,
                end_date: query?.endDate,
            });
        },

        getRateLimiterStats(options) {
            return request<RateLimiterStats>('/api/statistics/rate-limiter', {
                method: 'GET',
                signal: options?.signal,
            });
        },

        getRateLimiterConfig(options) {
            return request<RateLimiterConfigResponse>('/api/config/rate-limiter', {
                method: 'GET',
                signal: options?.signal,
            });
        },

        getAvailableCounters(options) {
            return request<AvailableCountersResponse>('/api/admin/counters', {
                method: 'GET',
                signal: options?.signal,
            });
        },

        getAllUserCounters(counterId, options) {
            return request<AllUserCountersResponse>(`/api/admin/counters/${encodeURIComponent(counterId)}`, {
                method: 'GET',
                signal: options?.signal,
            }, {
                limit: options?.limit,
                offset: options?.offset,
            });
        },

        getUserCounters(counterId, userId, options) {
            return request<UserCountersResponse>(`/api/counters/${encodeURIComponent(counterId)}/${encodeURIComponent(userId)}`, {
                method: 'GET',
                signal: options?.signal,
            });
        },

        getUserCounterHistory(counterId, userId, options) {
            return request<CounterHistoryResponse>(`/api/counters/${encodeURIComponent(counterId)}/${encodeURIComponent(userId)}/history`, {
                method: 'GET',
                signal: options?.signal,
            }, {
                limit: options?.limit,
                offset: options?.offset,
            });
        },

        adjustUserCounters(counterId, userId, payload, options) {
            return request<CounterTransactionResponse>(`/api/counters/${encodeURIComponent(counterId)}/${encodeURIComponent(userId)}`, {
                method: 'POST',
                signal: options?.signal,
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify(payload),
            });
        },
    };
}
