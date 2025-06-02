import type { User } from '../contexts/AuthContext'; // Assuming User is exported from AuthContext or a shared types file

export class ApiError extends Error {
  public status?: number;
  
  constructor(message: string, status?: number) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

/**
 * Fetches the current user's data from the backend.
 * @returns {Promise<User | null>} The user data if authenticated, otherwise null.
 * @throws {ApiError} If there's a network error or unexpected server response.
 */
export const fetchMe = async (): Promise<User | null> => {
  try {
    const response = await fetch('/_/me');

    if (response.ok) { // Status 200-299
      return await response.json() as User;
    }

    if (response.status === 401) { // Not authenticated
      return null;
    }

    // Other non-ok responses
    throw new ApiError(`Failed to fetch user data. Status: ${response.status}`, response.status);

  } catch (error) {
    if (error instanceof ApiError) {
      throw error; // Re-throw ApiError
    }
    // Network errors or other unexpected errors during fetch/JSON parsing
    console.error('Network or parsing error in fetchMe:', error);
    throw new ApiError('Network error or failed to parse user data.', error instanceof Error ? undefined : undefined);
  }
};

// Note: Logout is now handled by direct redirect to /_/logout URL
// No need for an API service function since the gateway handles it
