import { describe, expect, it } from 'vitest';
import { getUserAvatar, getUserDisplayName, getUserInitials } from './utils';

describe('user helpers', () => {
    it('prefers name over other display fields', () => {
        expect(getUserDisplayName({
            authenticated: true,
            name: 'Ada Lovelace',
            username: 'ada',
        })).toBe('Ada Lovelace');
    });

    it('builds initials from the display name', () => {
        expect(getUserInitials({
            authenticated: true,
            givenName: 'Ada',
            familyName: 'Lovelace',
        })).toBe('AL');
    });

    it('returns a nullable avatar url', () => {
        expect(getUserAvatar({
            authenticated: true,
            picture: 'https://example.com/avatar.png',
        })).toBe('https://example.com/avatar.png');
        expect(getUserAvatar(null)).toBeNull();
    });
});
