import Request from "../../Request";
import Client from "../../Client";
import getToken from "@/lib/helper/getToken";
import type {
    SuppressionFormat,
    SuppressionHashAlgo,
    SuppressionHashList,
    SuppressionImportResult,
    SuppressionListResult,
} from "@/lib/api/models/app/suppression/Suppression";

export async function listSuppressions(params: { q?: string; source?: string; cursor?: string | null; limit?: number }): Promise<SuppressionListResult> {
    const qs = new URLSearchParams();
    if (params.q) qs.set("q", params.q);
    if (params.source) qs.set("source", params.source);
    if (params.cursor) qs.set("cursor", params.cursor);
    if (params.limit) qs.set("limit", String(params.limit));
    const suffix = qs.toString() ? `?${qs.toString()}` : "";
    return await Request<SuppressionListResult>({ method: "GET", url: `/suppression${suffix}`, authorization: true });
}

export async function addSuppressions(emails: string[], reason: string): Promise<SuppressionImportResult> {
    return await Request<SuppressionImportResult>({
        method: "POST",
        url: "/suppression",
        data: { emails, reason },
        authorization: true,
    });
}

export async function removeSuppression(id: string): Promise<void> {
    await Request<void>({ method: "DELETE", url: `/suppression/${id}`, authorization: true });
}

export async function listSuppressionHashLists(): Promise<{ data: SuppressionHashList[] }> {
    return await Request<{ data: SuppressionHashList[] }>({ method: "GET", url: "/suppression/hash-lists", authorization: true });
}

export async function deleteSuppressionHashList(algo: SuppressionHashAlgo, listName: string): Promise<{ removed: number }> {
    return await Request<{ removed: number }>({
        method: "POST",
        url: "/suppression/hash-lists/delete",
        data: { algo, list_name: listName },
        authorization: true,
    });
}

export async function importSuppressions(file: File, format: SuppressionFormat, listName: string, reason: string): Promise<SuppressionImportResult> {
    const form = new FormData();
    form.append("file", file);
    form.append("format", format);
    if (listName) form.append("list_name", listName);
    if (reason) form.append("reason", reason);
    const token = getToken();
    const res = await Client.request<SuppressionImportResult>({
        method: "POST",
        url: "/suppression/import",
        data: form,
        headers: token?.access_token ? { Authorization: `Bearer ${token.access_token}` } : {},
    });
    return res.data;
}

export async function exportSuppressions(format: SuppressionFormat): Promise<Blob> {
    return await Request<Blob>({
        method: "GET",
        url: `/suppression/export?format=${format}`,
        authorization: true,
        responseType: "blob",
    });
}
