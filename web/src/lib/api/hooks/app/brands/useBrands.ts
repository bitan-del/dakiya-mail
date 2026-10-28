import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createBrand, deleteBrand, listBrands, setBrandMailboxes, updateBrand } from "@/lib/api/client/app/brands/brands";
import type { SaveBrand } from "@/lib/api/models/app/emails/Brand";

export function useBrands() {
    return useQuery({ queryKey: ["brands", "list"], queryFn: listBrands, staleTime: 10_000 });
}

function useInvalidate() {
    const qc = useQueryClient();
    return () => qc.invalidateQueries({ queryKey: ["brands"] });
}

// Creates or updates a brand, then sets its mailboxes, as one save.
export function useSaveBrand() {
    const invalidate = useInvalidate();
    return useMutation({
        mutationFn: async (v: { id?: string; data: SaveBrand; mailboxIds: string[] }) => {
            const saved = v.id ? await updateBrand(v.id, v.data) : await createBrand(v.data);
            return await setBrandMailboxes(saved.id, v.mailboxIds);
        },
        onSuccess: invalidate,
    });
}

export function useDeleteBrand() {
    const invalidate = useInvalidate();
    return useMutation({ mutationFn: deleteBrand, onSuccess: invalidate });
}
