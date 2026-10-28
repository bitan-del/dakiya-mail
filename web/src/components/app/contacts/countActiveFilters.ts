import type SearchContacts from "@/lib/api/models/app/contacts/SearchContacts";

// How many filters are set, for badges and "save as segment".
export function countActiveFilters(f: SearchContacts, hasCampaignContext: boolean): number {
    let n = 0;
    if (f.query) n++;
    n += f.custom_field_filters.length;
    if (f.subscribed !== undefined) n++;
    if (f.lead_status) n++;
    if (f.engagement) n++;
    if (f.min_campaigns !== undefined) n++;
    if (f.max_campaigns !== undefined) n++;
    if (f.created_after) n++;
    if (f.created_before) n++;
    if (f.updated_after) n++;
    if (f.updated_before) n++;
    // Don't count campaign scoping if it's coming from an outer page context.
    if (!hasCampaignContext && f.campaign_ids.length > 0) n++;
    if (f.category_ids && f.category_ids.length > 0) n++;
    return n;
}
