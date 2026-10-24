// /salesblink/* — hand mailbox warmup to a SalesBlink account.

import Request from "@/lib/api/client/Request";
import type {
    SalesBlinkLink,
    SalesBlinkMailboxRow,
    SalesBlinkSettingsInput,
    SalesBlinkStatus,
    SalesBlinkSyncResult,
} from "@/lib/api/models/app/salesblink/SalesBlink";

export async function getSalesBlinkStatus(): Promise<SalesBlinkStatus> {
    return await Request<SalesBlinkStatus>({ method: "GET", url: "/salesblink", authorization: true });
}

export async function listSalesBlinkMailboxes(): Promise<SalesBlinkMailboxRow[]> {
    const res = await Request<{ data: SalesBlinkMailboxRow[] }>({
        method: "GET",
        url: "/salesblink/mailboxes",
        authorization: true,
    });
    return res.data ?? [];
}

export async function connectSalesBlink(apiKey: string, autoEnroll: boolean): Promise<SalesBlinkLink> {
    return await Request<SalesBlinkLink>({
        method: "POST",
        url: "/salesblink/connect",
        data: { api_key: apiKey, auto_enroll: autoEnroll },
        authorization: true,
    });
}

export async function updateSalesBlinkSettings(input: SalesBlinkSettingsInput): Promise<SalesBlinkLink> {
    return await Request<SalesBlinkLink>({ method: "PATCH", url: "/salesblink", data: input, authorization: true });
}

export async function disconnectSalesBlink(): Promise<void> {
    await Request<void>({ method: "DELETE", url: "/salesblink", authorization: true });
}

/** Enrol every eligible mailbox that is not already warming. */
export async function syncSalesBlink(): Promise<SalesBlinkSyncResult> {
    return await Request<SalesBlinkSyncResult>({ method: "POST", url: "/salesblink/sync", authorization: true });
}

export async function enrollSalesBlinkMailbox(id: string): Promise<SalesBlinkMailboxRow> {
    return await Request<SalesBlinkMailboxRow>({
        method: "POST",
        url: `/salesblink/mailboxes/${id}/enroll`,
        authorization: true,
    });
}

export async function unenrollSalesBlinkMailbox(id: string): Promise<void> {
    await Request<void>({ method: "DELETE", url: `/salesblink/mailboxes/${id}/enroll`, authorization: true });
}
