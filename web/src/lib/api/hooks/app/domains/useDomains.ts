import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { addDomain, deleteDomain, listDomains, updateDomain, verifyDomain } from "@/lib/api/client/app/domains/domains";
import type { SaveSendingDomain } from "@/lib/api/models/app/emails/SendingDomain";

export function useDomains() {
    return useQuery({ queryKey: ["domains", "list"], queryFn: listDomains, staleTime: 10_000 });
}

function useInvalidate() {
    const qc = useQueryClient();
    return () => {
        qc.invalidateQueries({ queryKey: ["domains"] });
        qc.invalidateQueries({ queryKey: ["emails"] });
    };
}

export function useAddDomain() {
    const invalidate = useInvalidate();
    return useMutation({ mutationFn: addDomain, onSuccess: invalidate });
}

export function useUpdateDomain() {
    const invalidate = useInvalidate();
    return useMutation({ mutationFn: (v: { id: string; data: SaveSendingDomain }) => updateDomain(v.id, v.data), onSuccess: invalidate });
}

export function useVerifyDomain() {
    const invalidate = useInvalidate();
    return useMutation({ mutationFn: verifyDomain, onSuccess: invalidate });
}

export function useDeleteDomain() {
    const invalidate = useInvalidate();
    return useMutation({ mutationFn: deleteDomain, onSuccess: invalidate });
}
