import Request from "../../Request";
import type SearchContacts from "@/lib/api/models/app/contacts/SearchContacts";
import type { EnrollSegmentResult, Segment } from "@/lib/api/models/app/contacts/Segment";

export interface SaveSegment {
    name: string;
    description: string;
    filter: SearchContacts;
}

export async function listSegments(): Promise<{ data: Segment[] }> {
    return await Request<{ data: Segment[] }>({ method: "GET", url: "/segments", authorization: true });
}

export async function getSegment(id: string): Promise<Segment> {
    return await Request<Segment>({ method: "GET", url: `/segments/${id}`, authorization: true });
}

export async function createSegment(data: SaveSegment): Promise<Segment> {
    return await Request<Segment>({ method: "POST", url: "/segments", data, authorization: true });
}

export async function updateSegment(id: string, data: SaveSegment): Promise<Segment> {
    return await Request<Segment>({ method: "PATCH", url: `/segments/${id}`, data, authorization: true });
}

export async function deleteSegment(id: string): Promise<void> {
    await Request<void>({ method: "DELETE", url: `/segments/${id}`, authorization: true });
}

export async function enrollSegment(id: string, campaignId: string): Promise<EnrollSegmentResult> {
    return await Request<EnrollSegmentResult>({
        method: "POST",
        url: `/segments/${id}/enroll`,
        data: { campaign_id: campaignId },
        authorization: true,
    });
}
