import { useInfiniteQuery, type InfiniteData } from "@tanstack/react-query";
import listContactTimeline from "@/lib/api/client/app/contacts/listContactTimeline";
import type { ContactTimelineResult } from "@/lib/api/models/app/contacts/ContactTimelineEvent";

// 50 keeps each page light while still filling the visible scroller in
// one fetch. Backend caps `limit` at 200.
const PAGE_LIMIT = 50;

export default function useContactTimeline(contactId: string, enabled = true) {
    const q = useInfiniteQuery<
        ContactTimelineResult,
        Error,
        InfiniteData<ContactTimelineResult, string | undefined>,
        [string, string, string],
        string | undefined
    >({
        queryKey: ["contacts", contactId, "timeline"],
        queryFn: ({ pageParam }) =>
            listContactTimeline(contactId, {
                limit: PAGE_LIMIT,
                before: pageParam,
            }),
        initialPageParam: undefined,
        getNextPageParam: (lastPage) => {
            if (!lastPage.has_more) return undefined;
            const items = lastPage.data ?? [];
            if (items.length === 0) return undefined;
            // Cursor = the oldest item we have so far; backend returns
            // newest-first so the tail of the last page is the cursor.
            return items[items.length - 1]?.at;
        },
        enabled: enabled && !!contactId,
        staleTime: 15_000,
    });

    const events =
        q.data?.pages
            .flatMap((p) => p.data ?? [])
            .filter((e): e is NonNullable<typeof e> => e != null) ?? [];

    return { ...q, events };
}
