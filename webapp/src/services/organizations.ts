import {
    listOrganizations,
    createOrganization,
    getOrganization,
    updateOrganization,
    deleteOrganization,
    setOrganizationMember,
    removeOrganizationMember,
    setOrganizationLogo,
    deleteOrganizationLogo,
} from '@/apiclient';
import type {
    Organization,
    OrganizationDetail,
    OrganizationCreateRequest,
    OrganizationUpdateRequest,
    OrganizationMemberRequest,
} from '@/apiclient';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { customApiClient, handleResponse } from './client';

export const organizationKeys = {
    all: () => ['organizations'] as const,
    detail: (id: string) => [...organizationKeys.all(), id] as const,
};

export function useOrganizations() {
    return useQuery({
        queryKey: organizationKeys.all(),
        queryFn: async () => {
            const response = await listOrganizations({ client: customApiClient });
            return handleResponse<Organization[]>(response);
        },
    });
}

export function useOrganization(id: string) {
    return useQuery({
        queryKey: organizationKeys.detail(id),
        queryFn: async () => {
            const response = await getOrganization({ path: { orgId: id }, client: customApiClient });
            return handleResponse<OrganizationDetail>(response);
        },
    });
}

export function useCreateOrganization() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async (body: OrganizationCreateRequest) => {
            const response = await createOrganization({ body, client: customApiClient });
            return handleResponse<OrganizationDetail>(response);
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: organizationKeys.all() });
        },
    });
}

export function useUpdateOrganization() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async ({ id, body }: { id: string; body: OrganizationUpdateRequest }) => {
            const response = await updateOrganization({
                path: { orgId: id },
                body,
                client: customApiClient,
            });
            return handleResponse<OrganizationDetail>(response);
        },
        onSuccess: (data) => {
            queryClient.invalidateQueries({ queryKey: organizationKeys.all() });
            queryClient.invalidateQueries({ queryKey: organizationKeys.detail(data.id) });
        },
    });
}

export function useDeleteOrganization() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async (id: string) => {
            const response = await deleteOrganization({ path: { orgId: id }, client: customApiClient });
            handleResponse(response);
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: organizationKeys.all() });
        },
    });
}

export function useSetOrganizationMember() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async ({
            orgId,
            userId,
            body,
        }: {
            orgId: string;
            userId: string;
            body: OrganizationMemberRequest;
        }) => {
            const response = await setOrganizationMember({
                path: { orgId, userId },
                body,
                client: customApiClient,
            });
            handleResponse(response);
        },
        onSuccess: (_, { orgId }) => {
            queryClient.invalidateQueries({ queryKey: organizationKeys.detail(orgId) });
        },
    });
}

export function useRemoveOrganizationMember() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async ({ orgId, userId }: { orgId: string; userId: string }) => {
            const response = await removeOrganizationMember({
                path: { orgId, userId },
                client: customApiClient,
            });
            handleResponse(response);
        },
        onSuccess: (_, { orgId }) => {
            queryClient.invalidateQueries({ queryKey: organizationKeys.detail(orgId) });
        },
    });
}

export function useSetOrganizationLogo() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async ({ orgId, file }: { orgId: string; file: File }) => {
            const response = await setOrganizationLogo({
                path: { orgId },
                body: file,
                headers: { 'Content-Type': 'image/*' },
                client: customApiClient,
            });
            return handleResponse<OrganizationDetail>(response);
        },
        onSuccess: (_, { orgId }) => {
            queryClient.invalidateQueries({ queryKey: organizationKeys.all() });
            queryClient.invalidateQueries({ queryKey: organizationKeys.detail(orgId) });
        },
    });
}

export function useDeleteOrganizationLogo() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async (orgId: string) => {
            const response = await deleteOrganizationLogo({ path: { orgId }, client: customApiClient });
            handleResponse(response);
        },
        onSuccess: (_, orgId) => {
            queryClient.invalidateQueries({ queryKey: organizationKeys.all() });
            queryClient.invalidateQueries({ queryKey: organizationKeys.detail(orgId) });
        },
    });
}
