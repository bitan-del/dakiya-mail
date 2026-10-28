import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
    createSegment,
    deleteSegment,
    enrollSegment,
    getSegment,
    listSegments,
    updateSegment,
    type SaveSegment,
} from "@/lib/api/client/app/segments/segments";

export function useSegments() {
    return useQuery({ queryKey: ["segments", "list"], queryFn: listSegments, staleTime: 10_000 });
}

export function useSegment(id: string | null) {
    return useQuery({
        queryKey: ["segments", "detail", id],
        queryFn: () => getSegment(id as string),
        enabled: !!id,
        staleTime: 10_000,
    });
}

function useInvalidate() {
    const qc = useQueryClient();
    return () => qc.invalidateQueries({ queryKey: ["segments"] });
}

export function useCreateSegment() {
    const invalidate = useInvalidate();
    return useMutation({ mutationFn: createSegment, onSuccess: invalidate });
}

export function useUpdateSegment() {
    const invalidate = useInvalidate();
    return useMutation({ mutationFn: (v: { id: string; data: SaveSegment }) => updateSegment(v.id, v.data), onSuccess: invalidate });
}

export function useDeleteSegment() {
    const invalidate = useInvalidate();
    return useMutation({ mutationFn: deleteSegment, onSuccess: invalidate });
}

export function useEnrollSegment() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (v: { id: string; campaignId: string }) => enrollSegment(v.id, v.campaignId),
        onSuccess: () => {
            qc.invalidateQueries({ queryKey: ["campaigns"] });
            qc.invalidateQueries({ queryKey: ["contacts"] });
        },
    });
}
