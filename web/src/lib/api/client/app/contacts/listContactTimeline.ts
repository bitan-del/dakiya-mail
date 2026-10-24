import type { ContactTimelineResult } from "@/lib/api/models/app/contacts/ContactTimelineEvent";
import Request from "../../Request";

export default async function listContactTimeline(
    contactId: string,
    opts?: { limit?: number; before?: string },
): Promise<ContactTimelineResult> {
    const params = new URLSearchParams();
    if (opts?.limit) params.set("limit", String(opts.limit));
    if (opts?.before) params.set("before", opts.before);
    const qs = params.toString();
    return await Request<ContactTimelineResult>({
        method: "GET",
        url: `/contacts/${contactId}/timeline${qs ? `?${qs}` : ""}`,
        authorization: true,
    });
}
