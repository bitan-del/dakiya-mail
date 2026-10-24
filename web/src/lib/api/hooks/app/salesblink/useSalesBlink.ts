import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
    connectSalesBlink,
    disconnectSalesBlink,
    enrollSalesBlinkMailbox,
    getSalesBlinkStatus,
    listSalesBlinkMailboxes,
    syncSalesBlink,
    unenrollSalesBlinkMailbox,
    updateSalesBlinkSettings,
} from "@/lib/api/client/app/salesblink/salesBlink";
import type { SalesBlinkSettingsInput } from "@/lib/api/models/app/salesblink/SalesBlink";

export const SALESBLINK_KEY = ["salesblink"];

export function useSalesBlinkStatus(enabled = true) {
    return useQuery({
        queryKey: [...SALESBLINK_KEY, "status"],
        queryFn: getSalesBlinkStatus,
        enabled,
        staleTime: 10_000,
    });
}

export function useSalesBlinkMailboxes(enabled = true) {
    return useQuery({
        queryKey: [...SALESBLINK_KEY, "mailboxes"],
        queryFn: listSalesBlinkMailboxes,
        enabled,
        staleTime: 10_000,
    });
}

export function useConnectSalesBlink() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: ({ apiKey, autoEnroll }: { apiKey: string; autoEnroll: boolean }) =>
            connectSalesBlink(apiKey, autoEnroll),
        onSuccess: () => void qc.invalidateQueries({ queryKey: SALESBLINK_KEY }),
    });
}

export function useUpdateSalesBlinkSettings() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (input: SalesBlinkSettingsInput) => updateSalesBlinkSettings(input),
        onSuccess: () => void qc.invalidateQueries({ queryKey: SALESBLINK_KEY }),
    });
}

export function useDisconnectSalesBlink() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: () => disconnectSalesBlink(),
        onSuccess: () => void qc.invalidateQueries({ queryKey: SALESBLINK_KEY }),
    });
}

/** Bulk enrol: the "warm all of them" button. */
export function useSyncSalesBlink() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: () => syncSalesBlink(),
        onSuccess: () => void qc.invalidateQueries({ queryKey: SALESBLINK_KEY }),
    });
}

export function useEnrollSalesBlinkMailbox() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (id: string) => enrollSalesBlinkMailbox(id),
        onSuccess: () => void qc.invalidateQueries({ queryKey: SALESBLINK_KEY }),
    });
}

export function useUnenrollSalesBlinkMailbox() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (id: string) => unenrollSalesBlinkMailbox(id),
        onSuccess: () => void qc.invalidateQueries({ queryKey: SALESBLINK_KEY }),
    });
}
