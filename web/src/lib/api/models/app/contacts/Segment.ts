import type SearchContacts from "./SearchContacts";

export interface Segment {
    id: string;
    name: string;
    description: string;
    filter: SearchContacts;
    created_at: string;
    updated_at: string;
    contact_count?: number | null;
}

export interface EnrollSegmentResult {
    matched: number;
    enrolled: number;
    capped: boolean;
}
