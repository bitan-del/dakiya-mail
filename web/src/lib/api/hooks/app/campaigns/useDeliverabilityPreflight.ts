import { useQuery } from "@tanstack/react-query";
import getDeliverabilityPreflight from "@/lib/api/client/app/campaigns/getDeliverabilityPreflight";

// Fresh on every open: a user who fixes a DNS record or a list and reopens the
// launch dialog must see the fix, never a cached verdict.
export default function useDeliverabilityPreflight(campaignId: string, enabled: boolean) {
    return useQuery({
        queryKey: ["app", "campaigns", campaignId, "deliverability-preflight"],
        queryFn: () => getDeliverabilityPreflight(campaignId),
        enabled: enabled && !!campaignId,
        staleTime: 0,
        gcTime: 0,
        retry: false,
    });
}
