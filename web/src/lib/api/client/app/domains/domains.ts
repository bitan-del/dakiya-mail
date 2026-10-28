import Request from "../../Request";
import type { SaveSendingDomain, SendingDomain } from "@/lib/api/models/app/emails/SendingDomain";

export async function listDomains(): Promise<{ data: SendingDomain[] }> {
    return await Request<{ data: SendingDomain[] }>({ method: "GET", url: "/domains", authorization: true });
}

export async function addDomain(data: SaveSendingDomain): Promise<SendingDomain> {
    return await Request<SendingDomain>({ method: "POST", url: "/domains", data, authorization: true });
}

export async function updateDomain(id: string, data: SaveSendingDomain): Promise<SendingDomain> {
    return await Request<SendingDomain>({ method: "PATCH", url: `/domains/${id}`, data, authorization: true });
}

export async function verifyDomain(id: string): Promise<SendingDomain> {
    return await Request<SendingDomain>({ method: "POST", url: `/domains/${id}/verify`, authorization: true });
}

export async function deleteDomain(id: string): Promise<void> {
    await Request<void>({ method: "DELETE", url: `/domains/${id}`, authorization: true });
}
