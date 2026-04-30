import type { CurrentUser } from './types';

export function getUserDisplayName(user: CurrentUser | null): string {
    if (!user) {
        return 'Guest';
    }

    if (user.name) {
        return user.name;
    }

    if (user.givenName || user.familyName) {
        return [user.givenName, user.familyName].filter(Boolean).join(' ').trim();
    }

    return user.username ?? 'User';
}

export function getUserAvatar(user: CurrentUser | null): string | null {
    return user?.picture ?? null;
}

export function getUserInitials(user: CurrentUser | null): string {
    if (!user) {
        return 'G';
    }

    const displayName = getUserDisplayName(user);
    const words = displayName.split(' ').filter(Boolean);

    if (words.length >= 2) {
        return `${words[0][0]}${words[words.length - 1][0]}`.toUpperCase();
    }

    if (words.length === 1) {
        return words[0].slice(0, 2).toUpperCase();
    }

    return (user.username ?? 'User').slice(0, 2).toUpperCase();
}
