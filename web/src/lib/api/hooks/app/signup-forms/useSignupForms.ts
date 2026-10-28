import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { deleteSignupForm, listSignupForms, saveSignupForm } from "@/lib/api/client/app/signup-forms/signupForms";
import type { SaveSignupForm } from "@/lib/api/models/app/contacts/SignupForm";

export function useSignupForms() {
    return useQuery({ queryKey: ["signup-forms", "list"], queryFn: listSignupForms, staleTime: 10_000 });
}

export function useSaveSignupForm() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (v: { id?: string; data: SaveSignupForm }) => saveSignupForm(v.id, v.data),
        onSuccess: () => qc.invalidateQueries({ queryKey: ["signup-forms"] }),
    });
}

export function useDeleteSignupForm() {
    const qc = useQueryClient();
    return useMutation({ mutationFn: deleteSignupForm, onSuccess: () => qc.invalidateQueries({ queryKey: ["signup-forms"] }) });
}
