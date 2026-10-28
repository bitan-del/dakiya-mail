import Request from "../../Request";
import type { DeliverabilityEventListResult } from "@/lib/api/models/app/suppression/DeliverabilityEvent";

export interface DeliverabilityEventQuery {
    types: string[];
    q: string;
    from?: string;
    campaignId?: string;
    cursor?: string | null;
}

export default async function listDeliverabilityEvents(p: DeliverabilityEventQuery): Promise<DeliverabilityEventListResult> {
    const qs = new URLSearchParams({ limit: "50" });
    if (p.types.length) qs.set("type", p.types.join(","));
    if (p.q) qs.set("q", p.q);
    if (p.from) qs.set("from", p.from);
    if (p.campaignId) qs.set("campaign_id", p.campaignId);
    if (p.cursor) qs.set("cursor", p.cursor);
    return await Request<DeliverabilityEventListResult>({
        method: "GET",
        url: `/deliverability/events?${qs.toString()}`,
        authorization: true,
    });
}
