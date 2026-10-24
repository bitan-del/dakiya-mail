import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import listAuthorizedApps from "@/lib/api/client/app/oauth/listAuthorizedApps";
import revokeAuthorizedApp from "@/lib/api/client/app/oauth/revokeAuthorizedApp";

export function useAuthorizedApps() {
    return useQuery({
        queryKey: ["oauth-authorized-apps", "list"],
        queryFn: () => listAuthorizedApps(),
        staleTime: 5_000,
    });
}

export function useRevokeAuthorizedApp() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: (applicationId: string) => revokeAuthorizedApp(applicationId),
        onSuccess: () => void qc.invalidateQueries({ queryKey: ["oauth-authorized-apps"] }),
    });
}
