// Define types based on OpenAPI spec
// Re-using User from UsersListPage.tsx for consistency, or define a shared types file.
// For now, let's assume User type is available or re-defined here if needed.
// import { User } from '../components/UsersListPage'; // Or from a central types location

// UserResponse from OpenAPI spec (matches User interface in components)
export interface User {
  id: string;
  username: string;
  email: string;
  provider: string;
  name?: string | null;
  picture?: string | null;
  createdAt: string;
  updatedAt: string;
}

// UserCreateRequest from OpenAPI spec
export interface UserCreateRequest {
  username: string;
  email: string;
  password?: string; // Password might be optional if using OAuth/external providers primarily
  provider?: string; // Usually 'local' for password-based, or could be set by backend
  name?: string;
}

// Assuming Session might be part of a User's details or a separate fetch
// For now, not explicitly used in these core API functions but good to have if needed.
export interface Session {
  provider: string;
  validUntil: string;
  active: boolean;
  closedOn?: string | null;
}

// CurrentUserResponse for /me endpoint (simplified, adjust based on actual /me response)
// Often similar to User, but might have more/less fields or specific roles/permissions.
export interface CurrentUser extends User {
  // Add any /me specific fields, e.g., roles, permissions
  // For now, assuming it's the same as User for simplicity
}

// RequestStatistics from OpenAPI spec
export interface RequestStatistics {
  totalRequests: number;
  requestsByStatus: Record<string, number>;
  averageResponseTime: number;
  averageResponseSize: number;
  requestsByCountry: Record<string, number>;
  requestsByMethod: Record<string, number>;
  requestsByPath: Record<string, number>;
  requestsByDeviceType: Record<string, number>;
  requestsByPlatform: Record<string, number>;
  requestsByBrowser: Record<string, number>;
  requestsByUser?: Record<string, number>;
  requestsByJA4Fingerprint: Record<string, number>;
}

// Token interfaces from OpenAPI spec
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

// Helper function to process API responses with generics for return type
async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    let errorMessage = response.statusText || `Request failed with status ${response.status}`;
    try {
      const errorData = await response.json();
      // Prefer more specific error messages from API if available
      errorMessage = errorData.message || errorData.error || errorData.detail || errorMessage;
    } catch (e) {
      // If parsing JSON fails, log it but stick with the HTTP error message
      console.error("Failed to parse error response as JSON:", e);
    }
    throw new Error(errorMessage);
  }

  const contentType = response.headers.get('content-type');
  if (response.status === 204) { // No Content
    return null as T; // Or handle as appropriate for your app (e.g., return undefined)
  }
  
  if (contentType && contentType.includes('application/json')) {
    return response.json() as Promise<T>;
  }
  
  // For non-JSON responses, if any are expected (e.g., plain text)
  // This part might need adjustment based on what non-JSON responses are valid for your API
  // For now, assuming that if it's not JSON and not 204, it's an issue or needs specific handling
  // Or, if you expect text for some OK responses:
  // return response.text() as unknown as Promise<T>;
  
  // Defaulting to throwing an error if an OK response is not JSON and not 204,
  // as most modern APIs will use JSON.
  throw new Error(`Unexpected content type: ${contentType} for status ${response.status}`);
}

// Fetch all users
export async function fetchUsers(): Promise<User[]> {
  const response = await fetch('/_/api/users', {
    method: 'GET',
    headers: {
      'Accept': 'application/json',
    },
  });
  return handleResponse<User[]>(response);
}

// Create a new user
export async function createUser(userData: UserCreateRequest): Promise<User> {
  const response = await fetch('/_/api/users', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Accept': 'application/json',
    },
    body: JSON.stringify(userData),
  });
  // API returns UserResponse (which is our User type) with 201 status
  return handleResponse<User>(response);
}

// Fetch a single user by ID
export async function fetchUser(userId: string): Promise<User> {
  const response = await fetch(`/_/api/users/${userId}`, {
    method: 'GET',
    headers: {
      'Accept': 'application/json',
    },
  });
  return handleResponse<User>(response);
}

// Fetch the current user's details
// Assuming /me returns a CurrentUser (or User) object
export async function fetchCurrentUser(): Promise<CurrentUser> { 
  const response = await fetch('/_/me', {
    method: 'GET',
    headers: {
      'Accept': 'application/json',
    },
  });
  return handleResponse<CurrentUser>(response);
}

// Fetch request statistics
export async function fetchRequestStatistics(startDate?: string, endDate?: string): Promise<RequestStatistics> {
  const url = new URL('/_/api/statistics/requests', window.location.origin);
  if (startDate) {
    url.searchParams.append('start_date', startDate);
  }
  if (endDate) {
    url.searchParams.append('end_date', endDate);
  }

  const response = await fetch(url.toString(), {
    method: 'GET',
    headers: {
      'Accept': 'application/json',
    },
  });
  return handleResponse<RequestStatistics>(response);
}

// Token Management API Functions (Admin only)

// Fetch all tokens for a specific user (admin only)
export async function fetchUserTokens(userId: string): Promise<TokenResponse[]> {
  const response = await fetch(`/_/api/users/${userId}/tokens`, {
    method: 'GET',
    headers: {
      'Accept': 'application/json',
    },
  });
  return handleResponse<TokenResponse[]>(response);
}

// Create a new token for a specific user (admin only)
export async function createToken(userId: string, tokenData: TokenCreateRequest): Promise<TokenCreateResponse> {
  const response = await fetch(`/_/api/users/${userId}/tokens`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Accept': 'application/json',
    },
    body: JSON.stringify(tokenData),
  });
  return handleResponse<TokenCreateResponse>(response);
}

// Get token details by ID (admin only)
export async function fetchToken(tokenId: string): Promise<TokenResponse> {
  const response = await fetch(`/_/api/tokens/${tokenId}`, {
    method: 'GET',
    headers: {
      'Accept': 'application/json',
    },
  });
  return handleResponse<TokenResponse>(response);
}

// Revoke a token (admin only)
export async function revokeToken(tokenId: string): Promise<void> {
  const response = await fetch(`/_/api/tokens/${tokenId}`, {
    method: 'DELETE',
    headers: {
      'Accept': 'application/json',
    },
  });
  
  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(errorData.message || `Failed to revoke token: ${response.status} ${response.statusText}`);
  }
}

// Update token (e.g., activate/deactivate)
export async function updateToken(tokenId: string, updates: Partial<Pick<TokenResponse, 'name' | 'is_active'>>): Promise<TokenResponse> {
  const response = await fetch(`/_/api/tokens/${tokenId}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      'Accept': 'application/json',
    },
    body: JSON.stringify(updates),
  });
  return handleResponse<TokenResponse>(response);
}
