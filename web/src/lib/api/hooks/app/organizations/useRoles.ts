import { useQuery } from "@tanstack/react-query";
import getRoles from "@/lib/api/client/app/organizations/getRoles";

export default function useRoles() {
    return useQuery({
        queryKey: ["organizations", "roles"],
        queryFn: () => getRoles(),
        select: (res) => res.data ?? [],
    })
}
