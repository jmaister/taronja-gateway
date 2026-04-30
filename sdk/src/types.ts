export interface ErrorResponse {
    code: number;
    message: string;
}

export interface HealthResponse {
    status: string;
    timestamp: string;
    uptime: string;
    database: {
        status: string;
        open_connections?: number;
    };
}

export interface CurrentUser {
    authenticated?: boolean;
    username?: string;
    email?: string;
    name?: string | null;
    picture?: string | null;
    givenName?: string | null;
    familyName?: string | null;
    provider?: string;
    timestamp?: string;
    isAdmin?: boolean;
}

export interface UserCreateRequest {
    username: string;
    email: string;
    password: string;
}

export interface UserResponse {
    id: string;
    username: string;
    email?: string;
    name?: string | null;
    picture?: string | null;
    provider?: string | null;
}

export interface RateLimiterStat {
    ip: string;
    requests: number;
    errors: number;
    scan404: number;
    blockedUntil: string;
}

export type RateLimiterStats = RateLimiterStat[];

export interface RateLimiterConfigResponse {
    requestsPerMinute?: number;
    maxErrors?: number;
    blockMinutes?: number;
    vulnerabilityScan?: {
        urls: string[];
        max404: number;
        blockMinutes: number;
    };
}

export interface RequestStatistics {
    totalRequests: number;
    requestsByStatus: Record<string, number>;
    averageResponseTime: number;
    averageResponseSize: number;
    requestsByCountry: Record<string, number>;
    requestsByDeviceType: Record<string, number>;
    requestsByPlatform: Record<string, number>;
    requestsByBrowser: Record<string, number>;
    requestsByUser: Record<string, number>;
    requestsByJA4Fingerprint: Record<string, number>;
}

export interface RequestDetail {
    id: string;
    timestamp: string;
    path: string;
    user_id: string | null;
    username?: string | null;
    status_code: number;
    response_time: number;
    response_size: number;
    country: string;
    city: string;
    latitude?: number | null;
    longitude?: number | null;
    device_type: string;
    platform: string;
    platform_version: string;
    browser: string;
    browser_version: string;
}

export interface RequestDetailsResponse {
    requests: RequestDetail[];
}

export interface TokenResponse {
    id: string;
    name: string;
    is_active: boolean;
    created_at: string;
    expires_at?: string | null;
    last_used_at?: string | null;
    usage_count: number;
    scopes: string[];
    revoked_at?: string | null;
}

export interface TokenCreateRequest {
    name: string;
    expires_at?: string | null;
    scopes?: string[];
}

export interface TokenCreateResponse {
    token: string;
    token_info: TokenResponse;
}

export interface UserCountersResponse {
    user_id: string;
    username?: string;
    email?: string;
    counter_id: string;
    balance: number;
    last_updated: string;
    has_history: boolean;
}

export interface CounterAdjustmentRequest {
    amount: number;
    description: string;
}

export interface CounterTransactionResponse {
    id: string;
    user_id: string;
    counter_id: string;
    amount: number;
    balance_after: number;
    description: string;
    created_at: string;
}

export interface CounterHistoryResponse {
    user_id: string;
    counter_id: string;
    current_balance: number;
    has_history: boolean;
    transactions: CounterTransactionResponse[];
    total_count: number;
    limit: number;
    offset: number;
}

export interface AllUserCountersResponse {
    counter_id: string;
    users: UserCountersResponse[];
    total_count: number;
    limit: number;
    offset: number;
}

export interface AvailableCountersResponse {
    counters: string[];
}

export interface DateRangeQuery {
    startDate?: string;
    endDate?: string;
}

export interface PaginationQuery {
    limit?: number;
    offset?: number;
}

export interface LogoutResult {
    redirected: boolean;
    status: number;
    url: string;
}
