import { useQuery } from "@tanstack/react-query";
import listTaskTypes from "@/lib/api/client/app/crm/taskTypes/listTaskTypes";

export default function useTaskTypes() {
    return useQuery({
        queryKey: ["crm", "task-types"],
        queryFn: () => listTaskTypes(),
        staleTime: 5 * 60 * 1000,
    });
}
