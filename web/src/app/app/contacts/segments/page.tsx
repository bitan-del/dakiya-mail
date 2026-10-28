// Saved segments: named contacts filters, re-evaluated on every use. Open one
// in the contacts table, enroll it into a campaign, rename or delete it.

import React from "react";
import { Link } from "react-router-dom";
import { ArrowLeftIcon, BookmarkIcon, MegaphoneIcon, PencilIcon, Trash2Icon } from "lucide-react";
import toast from "react-hot-toast";

import { NoAccess } from "@/components/layout/NoAccess";
import { EmptyBlock, Page, PageBody, PageTopbar, SectionBar, TopbarAction } from "@/components/layout/Page";
import { EnrollSegmentDialog, SaveSegmentDialog } from "@/components/app/contacts/segments/SegmentDialogs";
import { useConfirm } from "@/hooks/context/confirm";
import { usePermission } from "@/hooks/usePermission";
import { useDeleteSegment, useSegments } from "@/lib/api/hooks/app/segments/useSegments";
import type { Segment } from "@/lib/api/models/app/contacts/Segment";

export default function SegmentsPage() {
    const canView = usePermission("VIEW_CONTACTS");
    const canManage = usePermission("MANAGE_CONTACTS");
    const confirm = useConfirm();
    const segments = useSegments();
    const del = useDeleteSegment();
    const [enrolling, setEnrolling] = React.useState<Segment | null>(null);
    const [editing, setEditing] = React.useState<Segment | null>(null);

    const rows = segments.data?.data ?? [];

    if (!canView) return <NoAccess feature="Segments" permissionLabel="View contacts" />;

    return (
        <Page>
            <PageTopbar eyebrow="Segments" subtitle="Saved contact filters, always current">
                <TopbarAction variant="ghost" href="/app/contacts">
                    New from Contacts filters
                </TopbarAction>
            </PageTopbar>
            <div className="px-5 pt-2">
                <Link to="/app/contacts" className="inline-flex items-center gap-1 text-[11.5px] text-slate-500 hover:text-slate-900">
                    <ArrowLeftIcon className="w-3 h-3" />
                    Contacts
                </Link>
            </div>
            <SectionBar label="Segments" count={rows.length} />
            <PageBody>
                {segments.isPending ? (
                    <div className="px-5 py-4 space-y-2">
                        {[0, 1, 2].map((i) => (
                            <div key={i} className="h-12 rounded bg-slate-50 animate-pulse" />
                        ))}
                    </div>
                ) : rows.length === 0 ? (
                    <div className="py-10">
                        <EmptyBlock
                            title="No segments yet"
                            body="Set filters on the Contacts page, then choose Save as segment. A segment updates itself as contacts change."
                        />
                    </div>
                ) : (
                    <div className="divide-y divide-slate-200/60">
                        {rows.map((s) => (
                            <div key={s.id} className="group min-h-12 px-5 py-2 flex items-center gap-3 text-[12.5px]">
                                <BookmarkIcon className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                                <Link to={`/app/contacts?segment=${s.id}`} className="min-w-0 flex-1">
                                    <div className="text-slate-900 font-medium truncate hover:text-sky-700">{s.name}</div>
                                    {s.description && <div className="text-[11.5px] text-slate-500 truncate">{s.description}</div>}
                                </Link>
                                <span className="font-mono text-[11.5px] text-slate-500 tabular-nums shrink-0">
                                    {typeof s.contact_count === "number" ? `${s.contact_count.toLocaleString()} contacts` : "—"}
                                </span>
                                {canManage && (
                                    <div className="flex items-center gap-1 opacity-100 md:opacity-0 md:group-hover:opacity-100 transition-opacity">
                                        <button
                                            type="button"
                                            onClick={() => setEnrolling(s)}
                                            className="h-7 px-2 rounded-md border border-slate-200 hover:border-slate-300 text-[11.5px] text-slate-700 inline-flex items-center gap-1"
                                        >
                                            <MegaphoneIcon className="w-3 h-3" />
                                            Enroll
                                        </button>
                                        <button
                                            type="button"
                                            aria-label={`Rename ${s.name}`}
                                            onClick={() => setEditing(s)}
                                            className="size-7 rounded-md text-slate-400 hover:text-slate-900 hover:bg-slate-100 inline-flex items-center justify-center"
                                        >
                                            <PencilIcon className="w-3.5 h-3.5" />
                                        </button>
                                        <button
                                            type="button"
                                            aria-label={`Delete ${s.name}`}
                                            onClick={() =>
                                                confirm.show(`Delete the segment "${s.name}"? Contacts are not affected.`, async () => {
                                                    await del.mutateAsync(s.id);
                                                    toast.success("Segment deleted");
                                                })
                                            }
                                            className="size-7 rounded-md text-slate-400 hover:text-rose-600 hover:bg-rose-50 inline-flex items-center justify-center"
                                        >
                                            <Trash2Icon className="w-3.5 h-3.5" />
                                        </button>
                                    </div>
                                )}
                            </div>
                        ))}
                    </div>
                )}
            </PageBody>
            <EnrollSegmentDialog open={!!enrolling} onClose={() => setEnrolling(null)} segment={enrolling} />
            <SaveSegmentDialog open={!!editing} onClose={() => setEditing(null)} filter={editing?.filter ?? emptyFilter} segment={editing ?? undefined} />
        </Page>
    );
}

const emptyFilter = { query: "", custom_field_filters: [], campaign_ids: [], sort_by: "created_at" as const, reverse: false };
