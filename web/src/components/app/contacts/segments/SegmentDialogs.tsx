// Save-as-segment and enroll-segment dialogs, shared by the contacts table and
// the segments page.

import React from "react";
import { BookmarkPlusIcon, MegaphoneIcon } from "lucide-react";
import toast from "react-hot-toast";

import { DialogFooter, DialogShell } from "@/components/ui/form-dialog";
import { Label, SearchInput, TextInput } from "@/components/ui/field";
import { cn } from "@/lib/utils";
import buildError from "@/lib/helper/buildError";
import type { AppError } from "@/lib/api/client/normalizeError";
import type SearchContacts from "@/lib/api/models/app/contacts/SearchContacts";
import type { Segment } from "@/lib/api/models/app/contacts/Segment";
import useCampaigns from "@/lib/api/hooks/app/campaigns/useCampaigns";
import { useCreateSegment, useEnrollSegment, useUpdateSegment } from "@/lib/api/hooks/app/segments/useSegments";

export function SaveSegmentDialog({
    open,
    onClose,
    filter,
    segment,
}: {
    open: boolean;
    onClose: () => void;
    filter: SearchContacts;
    // When set, edits this segment's name and description instead of creating one.
    segment?: Segment;
}) {
    const [name, setName] = React.useState("");
    const [description, setDescription] = React.useState("");
    const create = useCreateSegment();
    const update = useUpdateSegment();

    React.useEffect(() => {
        if (open) {
            setName(segment?.name ?? "");
            setDescription(segment?.description ?? "");
        }
    }, [open, segment]);

    async function submit() {
        const data = { name: name.trim(), description: description.trim(), filter };
        try {
            await toast.promise(segment ? update.mutateAsync({ id: segment.id, data }) : create.mutateAsync(data), {
                loading: "Saving segment…",
                success: (s) => `${s.name} saved${typeof s.contact_count === "number" ? `, ${s.contact_count.toLocaleString()} contacts match` : ""}`,
                error: (err: AppError) => buildError(err),
            });
            onClose();
        } catch {
            /* toast.promise already surfaced */
        }
    }

    return (
        <DialogShell open={open} onClose={onClose} eyebrow={segment ? "Edit" : "Save"} title="Segment" icon={<BookmarkPlusIcon className="w-3 h-3" />}>
            <div className="px-4 py-4 space-y-3 flex-1 min-h-0 overflow-y-auto">
                <div>
                    <Label>Name</Label>
                    <TextInput value={name} onChange={setName} placeholder="Finance leads, opened last 30 days" autoFocus className="w-full" />
                </div>
                <div>
                    <Label>Description</Label>
                    <TextInput value={description} onChange={setDescription} placeholder="Optional" className="w-full" />
                </div>
                {!segment && (
                    <p className="text-[11.5px] text-slate-500">
                        Saves the current filters. The segment is re-evaluated each time you open or enroll it, so new contacts that match are included.
                    </p>
                )}
            </div>
            <DialogFooter
                note={segment ? "Filters stay as saved." : "Sort order is not saved."}
                label="Save"
                ready={name.trim().length > 0 && name.trim().length <= 80}
                busy={create.isPending || update.isPending}
                onSubmit={submit}
            />
        </DialogShell>
    );
}

export function EnrollSegmentDialog({ open, onClose, segment }: { open: boolean; onClose: () => void; segment: Segment | null }) {
    const [q, setQ] = React.useState("");
    const [campaignId, setCampaignId] = React.useState("");
    const { campaigns, isPending } = useCampaigns({ query: q, folder: "", limit: 50, enabled: open });
    const enroll = useEnrollSegment();

    React.useEffect(() => {
        if (!open) {
            setQ("");
            setCampaignId("");
        }
    }, [open]);

    async function submit() {
        if (!segment || !campaignId) return;
        try {
            await toast.promise(enroll.mutateAsync({ id: segment.id, campaignId }), {
                loading: "Enrolling…",
                success: (r) =>
                    `${r.enrolled.toLocaleString()} added to the campaign${r.matched > r.enrolled ? `, ${(r.matched - r.enrolled).toLocaleString()} were already in it` : ""}${r.capped ? ". Run it again for the rest." : ""}`,
                error: (err: AppError) => buildError(err),
            });
            onClose();
        } catch {
            /* toast.promise already surfaced */
        }
    }

    return (
        <DialogShell open={open} onClose={onClose} eyebrow="Enroll" title={segment?.name ?? "Segment"} icon={<MegaphoneIcon className="w-3 h-3" />}>
            <div className="px-4 py-4 space-y-2 flex-1 min-h-0 overflow-y-auto">
                <Label>Campaign</Label>
                <SearchInput value={q} onChange={setQ} placeholder="Search campaigns" autoFocus className="w-full" />
                <div className="max-h-64 overflow-y-auto rounded-md border border-slate-200 divide-y divide-slate-100">
                    {isPending ? (
                        <div className="h-9 m-2 rounded bg-slate-50 animate-pulse" />
                    ) : campaigns.length === 0 ? (
                        <p className="px-3 py-3 text-[12px] text-slate-500">No campaigns match.</p>
                    ) : (
                        campaigns.map((c) => (
                            <button
                                key={c.id}
                                type="button"
                                onClick={() => setCampaignId(c.id)}
                                className={cn(
                                    "w-full h-9 px-3 flex items-center gap-2 text-left text-[12.5px] transition-colors",
                                    campaignId === c.id ? "bg-sky-50 text-sky-800" : "text-slate-700 hover:bg-slate-50",
                                )}
                            >
                                <span className="truncate flex-1">{c.name}</span>
                                <span className="text-[10.5px] text-slate-400 capitalize">{c.status}</span>
                            </button>
                        ))
                    )}
                </div>
            </div>
            <DialogFooter
                note={typeof segment?.contact_count === "number" ? `${segment.contact_count.toLocaleString()} contacts match now` : "Matches are counted when you enroll"}
                label="Enroll"
                ready={!!campaignId}
                busy={enroll.isPending}
                onSubmit={submit}
            />
        </DialogShell>
    );
}
