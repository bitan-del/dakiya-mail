import type { DeliverabilityPreflight } from "@/lib/api/models/app/campaigns/DeliverabilityPreflight";
import Request from "../../Request";

export default async function getDeliverabilityPreflight(campaignId: string): Promise<DeliverabilityPreflight> {
    return await Request<DeliverabilityPreflight>({
        method: "GET",
        url: `/campaigns/${campaignId}/preflight`,
        authorization: true,
    });
}
