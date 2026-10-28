import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
    addSuppressions,
    deleteSuppressionHashList,
    importSuppressions,
    listSuppressionHashLists,
    listSuppressions,
    removeSuppression,
} from "@/lib/api/client/app/suppression/suppression";
import type { SuppressionFormat, SuppressionHashAlgo } from "@/lib/api/models/app/suppression/Suppression";

export function useSuppressions(q: string, source: string) {
    return useInfiniteQuery({
        queryKey: ["suppression", "list", q, source],
        queryFn: ({ pageParam }) => listSuppressions({ q, source, cursor: pageParam, limit: 50 }),
        initialPageParam: null as string | null,
        getNextPageParam: (last) => (last.pagination.has_more ? last.pagination.next_cursor : null),
        staleTime: 5_000,
    });
}

export function useSuppressionHashLists() {
    return useQuery({
        queryKey: ["suppression", "hash-lists"],
        queryFn: listSuppressionHashLists,
        staleTime: 5_000,
    });
}

function useInvalidateSuppression() {
    const qc = useQueryClient();
    return () => qc.invalidateQueries({ queryKey: ["suppression"] });
}

export function useAddSuppressions() {
    const invalidate = useInvalidateSuppression();
    return useMutation({
        mutationFn: ({ emails, reason }: { emails: string[]; reason: string }) => addSuppressions(emails, reason),
        onSuccess: invalidate,
    });
}

export function useRemoveSuppression() {
    const invalidate = useInvalidateSuppression();
    return useMutation({ mutationFn: removeSuppression, onSuccess: invalidate });
}

export function useImportSuppressions() {
    const invalidate = useInvalidateSuppression();
    return useMutation({
        mutationFn: (v: { file: File; format: SuppressionFormat; listName: string; reason: string }) =>
            importSuppressions(v.file, v.format, v.listName, v.reason),
        onSuccess: invalidate,
    });
}

export function useDeleteSuppressionHashList() {
    const invalidate = useInvalidateSuppression();
    return useMutation({
        mutationFn: (v: { algo: SuppressionHashAlgo; listName: string }) => deleteSuppressionHashList(v.algo, v.listName),
        onSuccess: invalidate,
    });
}
