import { beforeEach, describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router-dom';
import { OrganizationsPage } from './OrganizationsPage';

const calls = vi.hoisted(() => ({ mutations: [] as unknown[] }));

vi.mock('../services/organizations', () => ({
    useOrganizations: () => ({
        data: [
            { id: 'org-1', name: 'Acme Inc', members: [{ userId: 'user-1', role: 'owner' }], createdAt: '2026-01-01T00:00:00Z', logoUrl: '/_/api/organizations/org-1/logo?v=1' },
            { id: 'org-2', name: 'Tech Corp', members: [], createdAt: '2026-02-01T00:00:00Z' },
        ],
        isLoading: false,
        error: null,
        refetch: () => {},
    }),
    useCreateOrganization: () => ({
        mutate: (data: unknown) => calls.mutations.push(data),
        isPending: false,
        error: null,
    }),
    useDeleteOrganization: () => ({
        mutate: (data: unknown) => calls.mutations.push(data),
        isPending: false,
        error: null,
    }),
    useOrganization: () => ({
        data: { id: 'org-1', name: 'Acme Inc', members: [{ userId: 'user-1', role: 'owner' }] },
        isLoading: false,
        error: null,
        refetch: () => {},
    }),
    useSetOrganizationMember: () => ({
        mutate: (data: unknown) => calls.mutations.push(data),
        isPending: false,
        error: null,
    }),
    useSetOrganizationLogo: () => ({
        mutate: (data: unknown) => calls.mutations.push(data),
        isPending: false,
        error: null,
    }),
    useDeleteOrganizationLogo: () => ({
        mutate: (data: unknown) => calls.mutations.push(data),
        isPending: false,
        error: null,
    }),
    useRemoveOrganizationMember: () => ({
        mutate: (data: unknown) => calls.mutations.push(data),
        isPending: false,
        error: null,
    }),
}));

const render = (url: string = '/') =>
    renderToStaticMarkup(
        <MemoryRouter initialEntries={[url]}>
            <OrganizationsPage />
        </MemoryRouter>,
    );

describe('OrganizationsPage', () => {
    beforeEach(() => {
        calls.mutations = [];
    });

    it('renders and shows organizations list', () => {
        const html = render();
        expect(html).toContain('Organizations');
        expect(html).toContain('Acme Inc');
        expect(html).toContain('Tech Corp');
    });

    it('shows member count', () => {
        const html = render();
        expect(html).toContain('Members');
        expect(html).toContain('1');
    });

    it('shows view and delete buttons', () => {
        const html = render();
        expect(html).toContain('View');
        expect(html).toContain('Delete');
    });

    it('shows the logo of an organization in the list', () => {
        const html = render();
        expect(html).toContain('/_/api/organizations/org-1/logo?v=1');
    });

    it('has create form', () => {
        const html = render();
        expect(html).toContain('New organization');
        expect(html).toContain('Name');
    });
});
