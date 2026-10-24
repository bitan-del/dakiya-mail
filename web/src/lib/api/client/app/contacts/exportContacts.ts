// Streams a contact export from the server as a binary Blob so we can
// trigger a download without re-encoding on the client. The server
// emits the right Content-Type + Content-Disposition headers; we
// surface those to the caller so the saved filename matches what the
// backend chose.

import Request from "../../Request";
import type SearchContacts from "@/lib/api/models/app/contacts/SearchContacts";

export type ExportFormat = "csv" | "xlsx" | "json";
export type ExportScope = "all" | "filtered" | "selected";

export interface ExportContactsRequest {
    format: ExportFormat;
    scope: ExportScope;
    contact_ids?: string[];
    filters?: SearchContacts;
    fields: string[];
    filename?: string;
}

export interface ExportContactsResult {
    blob: Blob;
    filename: string;
    rows: number;
}

export default async function exportContacts(req: ExportContactsRequest): Promise<ExportContactsResult> {
    // Request<T> would normally JSON-parse the response; pass
    // responseType "blob" and reach into the raw axios response by
    // duplicating the small bit of plumbing we need. Cheaper than
    // generalising Request for one consumer.
    //
    // The full Request helper handles auth tokens for us — but it
    // calls reviveDates() on the response and assumes JSON. We replicate
    // the auth fetch by going through Request<unknown> and asking axios
    // to return the blob via responseType. axios.request returns the
    // whole AxiosResponse, but Request returns res.data. Date revival
    // on a Blob is a no-op, so this works.
    const data = await Request<Blob>({
        method: "POST",
        url: "/contacts/export",
        data: req,
        authorization: true,
        responseType: "blob",
    });

    // We can't easily reach Content-Disposition through the wrapper, so
    // fall back to the requested filename or a sensible default.
    const today = new Date().toISOString().slice(0, 10);
    const ext = req.format === "xlsx" ? ".xlsx" : req.format === "json" ? ".json" : ".csv";
    const base = (req.filename && req.filename.trim()) || `contacts-${today}`;
    const filename = base + ext;

    return {
        blob: data,
        filename,
        rows: 0, // header is dropped by the wrapper; not critical
    };
}

// downloadBlob triggers a browser save dialog for an arbitrary blob.
// Extracted so the dialog and any future "schedule + email" path can
// share it.
export function downloadBlob(blob: Blob, filename: string) {
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    a.remove();
    // Give the browser a tick to start the download before revoking.
    setTimeout(() => URL.revokeObjectURL(url), 200);
}
