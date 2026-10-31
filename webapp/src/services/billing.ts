import {
    listBillingPlans,
    createBillingPlan,
    getBillingPlan,
    updateBillingPlan,
    listBillingSubscriptions,
    activateBillingSubscription,
    cancelBillingSubscription,
    listBillingPurchases,
    refundBillingPurchase,
    listBillingDocuments,
    retryBillingDocument,
    getBillingSetup,
    runBillingSetupAction,
} from '@/apiclient';
import type {
    BillingPlan,
    BillingPlanInput,
    BillingPlanUpdate,
    BillingSubscription,
    BillingCancelRequest,
    BillingPurchase,
    BillingRefundRequest,
    BillingDocument,
    BillingSetup,
    BillingSetupActionRequest,
} from '@/apiclient';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { customApiClient, handleResponse } from './client';

export const billingKeys = {
    plans: () => ['billingPlans'] as const,
    planDetail: (key: string) => [...billingKeys.plans(), key] as const,
    subscriptions: (filters?: string) => ['billingSubscriptions', filters || ''] as const,
    purchases: (filters?: string) => ['billingPurchases', filters || ''] as const,
    documents: (filters?: string) => ['billingDocuments', filters || ''] as const,
    setup: () => ['billingSetup'] as const,
};

// Plans
export function useBillingPlans(query?: { archived?: boolean }) {
    return useQuery({
        queryKey: billingKeys.plans(),
        queryFn: async () => {
            const response = await listBillingPlans({
                query: query ? { include_archived: query.archived } : undefined,
                client: customApiClient,
            });
            return handleResponse<BillingPlan[]>(response);
        },
    });
}

export function useBillingPlan(key: string) {
    return useQuery({
        queryKey: billingKeys.planDetail(key),
        queryFn: async () => {
            const response = await getBillingPlan({ path: { key }, client: customApiClient });
            return handleResponse<BillingPlan>(response);
        },
    });
}

export function useCreateBillingPlan() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async (body: BillingPlanInput) => {
            const response = await createBillingPlan({ body, client: customApiClient });
            return handleResponse<BillingPlan>(response);
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: billingKeys.plans() });
        },
    });
}

export function useUpdateBillingPlan() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async ({ key, body }: { key: string; body: BillingPlanUpdate }) => {
            const response = await updateBillingPlan({
                path: { key },
                body,
                client: customApiClient,
            });
            return handleResponse<BillingPlan>(response);
        },
        onSuccess: (data) => {
            queryClient.invalidateQueries({ queryKey: billingKeys.plans() });
            queryClient.invalidateQueries({ queryKey: billingKeys.planDetail(data.key) });
        },
    });
}

// Subscriptions
export function useBillingSubscriptions(query?: { subject?: string; status?: string; plan?: string }) {
    const filterKey = query ? JSON.stringify(query) : '';
    return useQuery({
        queryKey: billingKeys.subscriptions(filterKey),
        queryFn: async () => {
            const response = await listBillingSubscriptions({
                query: query || {},
                client: customApiClient,
            });
            return handleResponse<BillingSubscription[]>(response);
        },
    });
}

export function useActivateBillingSubscription() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async (id: string) => {
            const response = await activateBillingSubscription({
                path: { id },
                client: customApiClient,
            });
            return handleResponse<BillingSubscription>(response);
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: billingKeys.subscriptions() });
        },
    });
}

export function useCancelBillingSubscription() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async ({ id, body }: { id: string; body: BillingCancelRequest }) => {
            const response = await cancelBillingSubscription({
                path: { id },
                body,
                client: customApiClient,
            });
            return handleResponse<BillingSubscription>(response);
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: billingKeys.subscriptions() });
        },
    });
}

// Purchases
export function useBillingPurchases(query?: { plan?: string; status?: string; since?: string }) {
    const filterKey = query ? JSON.stringify(query) : '';
    return useQuery({
        queryKey: billingKeys.purchases(filterKey),
        queryFn: async () => {
            const response = await listBillingPurchases({
                query: query || {},
                client: customApiClient,
            });
            return handleResponse<BillingPurchase[]>(response);
        },
    });
}

export function useRefundBillingPurchase() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async ({ id, body }: { id: string; body: BillingRefundRequest }) => {
            const response = await refundBillingPurchase({
                path: { id },
                body,
                client: customApiClient,
            });
            handleResponse(response);
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: billingKeys.purchases() });
        },
    });
}

// Documents (Invoices)
export function useBillingDocuments(query?: { subject?: string; state?: string; kind?: string }) {
    const filterKey = query ? JSON.stringify(query) : '';
    return useQuery({
        queryKey: billingKeys.documents(filterKey),
        queryFn: async () => {
            const response = await listBillingDocuments({
                query: query || {},
                client: customApiClient,
            });
            return handleResponse<BillingDocument[]>(response);
        },
    });
}

export function useRetryBillingDocument() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async (id: string) => {
            const response = await retryBillingDocument({
                path: { id },
                client: customApiClient,
            });
            handleResponse(response);
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: billingKeys.documents() });
        },
    });
}

// Setup
export function useBillingSetup() {
    return useQuery({
        queryKey: billingKeys.setup(),
        queryFn: async () => {
            const response = await getBillingSetup({ client: customApiClient });
            return handleResponse<BillingSetup>(response);
        },
    });
}

export function useRunBillingSetupAction() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async (body: BillingSetupActionRequest) => {
            const response = await runBillingSetupAction({
                body,
                client: customApiClient,
            });
            handleResponse(response);
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: billingKeys.setup() });
        },
    });
}
