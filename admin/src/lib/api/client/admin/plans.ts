// /admin/plans — plan catalog.

import { Request } from "@/lib/api/client";
import { buildSearchQuery } from "@/lib/api/client/admin/query";
import type {
    AdminPlanSearch,
    AdminPlansResult,
    Plan,
    UpdatePlanRequest,
} from "@/lib/api/models/admin";

// listPlans is the no-arg variant used as a facet-dropdown source (it is passed
// directly as a react-query queryFn, so it must NOT accept the query context as
// params). It returns the first page; callers read `.data`.
export function listPlans(): Promise<AdminPlansResult> {
    return Request({
        method: "GET",
        url: "/admin/plans",
        authorization: true,
    });
}

// searchPlans is the faceted variant for the Plans explorer page.
export function searchPlans(
    params: AdminPlanSearch = {},
): Promise<AdminPlansResult> {
    return Request({
        method: "GET",
        url: `/admin/plans${buildSearchQuery(params as Record<string, unknown>)}`,
        authorization: true,
    });
}

export function updatePlan(
    id: string,
    body: UpdatePlanRequest,
): Promise<Plan> {
    return Request({
        method: "PATCH",
        url: `/admin/plans/${id}`,
        authorization: true,
        data: body,
    });
}
