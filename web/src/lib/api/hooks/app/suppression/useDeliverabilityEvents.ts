import { useInfiniteQuery } from "@tanstack/react-query";
import listDeliverabilityEvents, { type DeliverabilityEventQuery } from "@/lib/api/client/app/deliverability-events/listDeliverabilityEvents";

// Keyed under ["analytics", ...] so the realtime events that refresh the
// deliverability dashboard refresh the log too.
export default function useDeliverabilityEvents(q: Omit<DeliverabilityEventQuery, "cursor">) {
    return useInfiniteQuery({
        queryKey: ["analytics", "deliverability", "events", q],
        queryFn: ({ pageParam }) => listDeliverabilityEvents({ ...q, cursor: pageParam }),
        initialPageParam: null as string | null,
        getNextPageParam: (last) => (last.pagination.has_more ? last.pagination.next_cursor : null),
        staleTime: 5_000,
    });
}
