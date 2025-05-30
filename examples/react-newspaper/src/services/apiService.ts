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

/**
 * Logs out the current user by calling the backend logout endpoint.
 * For this example, it's a placeholder as no specific backend logout URL is given.
 * If a backend logout endpoint exists (e.g., POST to /api/logout), implement it here.
 * @returns {Promise<void>}
 */
export const logoutUser = async (): Promise<void> => {
  try {
    // Example: if there was a logout endpoint
    // const response = await fetch('/api/logout', { method: 'POST' });
    // if (!response.ok) {
    //   throw new ApiError('Logout failed', response.status);
    // }
    // console.log('Backend logout successful');
    return Promise.resolve(); // Placeholder implementation
  } catch (error) {
    console.error('Error during backend logout:', error);
    // Depending on requirements, might re-throw or handle silently
    throw error;
  }
};
