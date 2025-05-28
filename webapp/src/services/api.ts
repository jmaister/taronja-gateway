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
  const response = await fetch('/api/users', {
    method: 'GET',
    headers: {
      'Accept': 'application/json',
    },
  });
  return handleResponse<User[]>(response);
}

// Create a new user
export async function createUser(userData: UserCreateRequest): Promise<User> {
  const response = await fetch('/api/users', {
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
  const response = await fetch(`/api/users/${userId}`, {
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
  const response = await fetch('/me', {
    method: 'GET',
    headers: {
      'Accept': 'application/json',
    },
  });
  return handleResponse<CurrentUser>(response);
}
