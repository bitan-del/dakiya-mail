// /deliverability/postmaster: Gmail's own view of the workspace's sending domains.

import type { PostmasterCredentials, PostmasterReport } from "@/lib/api/models/app/analytics/Postmaster";
import Request from "../../Request";

export async function getPostmasterReport(domain: string, days: number): Promise<PostmasterReport> {
    const params = new URLSearchParams({ days: String(days) });
    if (domain) params.set("domain", domain);
    return await Request<PostmasterReport>({
        method: "GET",
        url: `/deliverability/postmaster?${params.toString()}`,
        authorization: true,
    });
}

export async function connectPostmaster(creds: PostmasterCredentials): Promise<void> {
    await Request<void>({ method: "PUT", url: "/deliverability/postmaster/credentials", data: creds, authorization: true });
}

export async function disconnectPostmaster(): Promise<void> {
    await Request<void>({ method: "DELETE", url: "/deliverability/postmaster/credentials", authorization: true });
}
