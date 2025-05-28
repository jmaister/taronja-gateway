import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { fetchUsers, User } from './api'; // handleResponse is not exported, tested implicitly.

// Mock global fetch
global.fetch = vi.fn();

describe('API Service: fetchUsers', () => {
  beforeEach(() => {
    // Reset mocks before each test
    vi.resetAllMocks(); // Vitest's way to reset mocks, equivalent to jest.resetAllMocks()
  });

  // afterEach is not strictly necessary with vi.resetAllMocks() in beforeEach for vi.fn(),
  // but can be useful for more complex global state cleanup if needed.
  // For vi.spyOn, direct restoration in afterEach might be more common.
  // For vi.fn() on global.fetch, resetAllMocks usually covers it.
  afterEach(() => {
    // vi.restoreAllMocks(); // Use if you were using vi.spyOn and wanted to restore original implementations
  });

  it('should fetch users successfully', async () => {
    const mockUsers: User[] = [
      { id: '1', username: 'testuser1', email: 'test1@example.com', provider: 'local', name: 'Test User 1', picture: '', createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() },
      { id: '2', username: 'testuser2', email: 'test2@example.com', provider: 'local', name: 'Test User 2', picture: '', createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() },
    ];

    (fetch as vi.Mock).mockResolvedValue({
      ok: true,
      json: async () => mockUsers,
      headers: new Headers({ 'Content-Type': 'application/json' }),
      // status: 200, // Not strictly needed for ok: true, but good for completeness
    } as Response);

    const users = await fetchUsers();
    expect(fetch).toHaveBeenCalledTimes(1);
    // Check the URL and options passed to fetch
    expect(fetch).toHaveBeenCalledWith('/api/users', {
      method: 'GET',
      headers: { 'Accept': 'application/json' },
    });
    expect(users).toEqual(mockUsers);
  });

  it('should throw an error if fetch fails and response is json', async () => {
    (fetch as vi.Mock).mockResolvedValue({
      ok: false,
      statusText: 'API Error', // This will be used if json parsing fails or message is not in json
      headers: new Headers({ 'Content-Type': 'application/json' }),
      // status: 500, // Example status
      json: async () => ({ message: 'Failed to fetch from JSON' }) 
    } as Response);

    // The handleResponse function tries to parse JSON error first
    await expect(fetchUsers()).rejects.toThrow('Failed to fetch from JSON');
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenCalledWith('/api/users', {
      method: 'GET',
      headers: { 'Accept': 'application/json' },
    });
  });

  it('should throw an error using statusText if fetch fails and response is not json', async () => {
    (fetch as vi.Mock).mockResolvedValue({
      ok: false,
      statusText: 'Server Error Not JSON',
      headers: new Headers({'Content-Type': 'text/plain'}), // Indicate non-JSON response
      // status: 500,
      // No json method or json method throws error
      json: async () => { throw new Error("Cannot parse as JSON"); } 
    } as Response);

    await expect(fetchUsers()).rejects.toThrow('Server Error Not JSON');
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenCalledWith('/api/users', {
      method: 'GET',
      headers: { 'Accept': 'application/json' },
    });
  });
});

// Tests for handleResponse are not included here as it's not exported from api.ts.
// Its behavior is implicitly tested through fetchUsers. If handleResponse
// were exported and more complex, it would warrant its own describe block.
// describe('API Service: handleResponse', () => {
//   // ... tests for handleResponse ...
// });
