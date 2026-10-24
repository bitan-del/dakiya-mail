import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { connectPostmaster, disconnectPostmaster, getPostmasterReport } from "@/lib/api/client/app/analytics/postmaster";

// The audit spine invalidates ["postmaster"] when a teammate connects or disconnects.
export const POSTMASTER_KEY = ["postmaster"];

export function usePostmaster(domain: string, days: number) {
    return useQuery({
        queryKey: [...POSTMASTER_KEY, domain, days],
        queryFn: () => getPostmasterReport(domain, days),
        placeholderData: (prev) => prev,
    });
}

export function useConnectPostmaster() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: connectPostmaster,
        onSuccess: () => void qc.invalidateQueries({ queryKey: POSTMASTER_KEY }),
    });
}

export function useDisconnectPostmaster() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: disconnectPostmaster,
        onSuccess: () => void qc.invalidateQueries({ queryKey: POSTMASTER_KEY }),
    });
}
