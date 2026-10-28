// Suppression list manager: every address the workspace will never mail
// (bounces, complaints, unsubscribes, manual adds, imports) plus hashed
// advertiser lists matched by MD5 or SHA-256 at send time.

import React from "react";
import { DownloadIcon, HashIcon, Loader2Icon, PlusIcon, ShieldOffIcon, Trash2Icon, UploadIcon } from "lucide-react";
import toast from "react-hot-toast";

import { NoAccess } from "@/components/layout/NoAccess";
import { EmptyBlock, Page, PageBody, PageTopbar, SectionBar, TopbarAction } from "@/components/layout/Page";
import { Label, SearchInput, TextInput } from "@/components/ui/field";
import { DialogFooter, DialogShell } from "@/components/ui/form-dialog";
import { useConfirm } from "@/hooks/context/confirm";
import { usePermission } from "@/hooks/usePermission";
import { cn } from "@/lib/utils";
import buildError from "@/lib/helper/buildError";
import type { AppError } from "@/lib/api/client/normalizeError";
import { exportSuppressions } from "@/lib/api/client/app/suppression/suppression";
import { downloadBlob } from "@/lib/api/client/app/contacts/exportContacts";
import {
    useAddSuppressions,
    useDeleteSuppressionHashList,
    useImportSuppressions,
    useRemoveSuppression,
    useSuppressionHashLists,
    useSuppressions,
} from "@/lib/api/hooks/app/suppression/useSuppression";
import type { SuppressedRecipient, SuppressionFormat } from "@/lib/api/models/app/suppression/Suppression";

const SOURCES: { value: string; label: string }[] = [
    { value: "", label: "All" },
    { value: "bounce", label: "Bounces" },
    { value: "complaint", label: "Complaints" },
    { value: "unsubscribe", label: "Unsubscribes" },
    { value: "manual", label: "Manual" },
    { value: "import", label: "Imported" },
];

const SOURCE_TONE: Record<string, string> = {
    bounce: "bg-rose-50 text-rose-700",
    complaint: "bg-amber-50 text-amber-700",
    unsubscribe: "bg-slate-100 text-slate-700",
    manual: "bg-sky-50 text-sky-700",
    import: "bg-violet-50 text-violet-700",
};

export default function SuppressionPage() {
    const canView = usePermission("VIEW_CONTACTS");
    const canManage = usePermission("MANAGE_CONTACTS");
    const confirm = useConfirm();

    const [q, setQ] = React.useState("");
    const [debounced, setDebounced] = React.useState("");
    const [source, setSource] = React.useState("");
    const [addOpen, setAddOpen] = React.useState(false);
    const [importOpen, setImportOpen] = React.useState(false);
    const [exporting, setExporting] = React.useState<SuppressionFormat | null>(null);

    React.useEffect(() => {
        const t = setTimeout(() => setDebounced(q.trim()), 250);
        return () => clearTimeout(t);
    }, [q]);

    const list = useSuppressions(debounced, source);
    const hashLists = useSuppressionHashLists();
    const remove = useRemoveSuppression();
    const deleteList = useDeleteSuppressionHashList();

    const rows = list.data?.pages.flatMap((p) => p.data) ?? [];
    const total = list.data?.pages[0]?.pagination.total ?? 0;

    async function doExport(format: SuppressionFormat) {
        setExporting(format);
        try {
            const blob = await exportSuppressions(format);
            downloadBlob(blob, `suppression-${format}-${new Date().toISOString().slice(0, 10)}.csv`);
        } catch (err) {
            toast.error(buildError(err as AppError));
        } finally {
            setExporting(null);
        }
    }

    if (!canView) return <NoAccess feature="Suppression" permissionLabel="View contacts" />;

    return (
        <Page>
            <PageTopbar eyebrow="Suppression" subtitle="Addresses this workspace never mails">
                <ExportButton label="CSV" busy={exporting === "plain"} onClick={() => doExport("plain")} />
                <ExportButton label="MD5" busy={exporting === "md5"} onClick={() => doExport("md5")} />
                <ExportButton label="SHA-256" busy={exporting === "sha256"} onClick={() => doExport("sha256")} />
                {canManage && (
                    <>
                        <TopbarAction variant="ghost" icon={<UploadIcon className="w-3 h-3" />} onClick={() => setImportOpen(true)}>
                            <span className="hidden sm:inline">Import</span>
                        </TopbarAction>
                        <TopbarAction icon={<PlusIcon className="w-3 h-3" />} onClick={() => setAddOpen(true)}>
                            <span className="hidden sm:inline">Add</span>
                        </TopbarAction>
                    </>
                )}
            </PageTopbar>

            <SectionBar label="Hashed lists" count={hashLists.data?.data.length ?? 0} />
            <div className="border-b border-slate-200/60">
                {(hashLists.data?.data.length ?? 0) === 0 ? (
                    <p className="px-5 py-3 text-[12px] text-slate-500">
                        No hashed lists. Import an advertiser's MD5 or SHA-256 file and every send checks recipients against it.
                    </p>
                ) : (
                    <div className="divide-y divide-slate-200/60">
                        {hashLists.data?.data.map((l) => (
                            <div key={`${l.algo}:${l.list_name}`} className="h-11 px-5 flex items-center gap-3 text-[12.5px]">
                                <HashIcon className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                                <span className="text-slate-900 font-medium truncate">{l.list_name || "Untitled list"}</span>
                                <span className="text-[10.5px] uppercase tracking-[0.1em] px-1.5 py-0.5 rounded bg-slate-100 text-slate-600">
                                    {l.algo === "sha256" ? "SHA-256" : "MD5"}
                                </span>
                                <span className="ml-auto font-mono text-[11.5px] text-slate-500 tabular-nums">
                                    {l.count.toLocaleString()} hashes
                                </span>
                                {canManage && (
                                    <button
                                        type="button"
                                        aria-label={`Remove ${l.list_name}`}
                                        onClick={() =>
                                            confirm.show(`Remove the hashed list "${l.list_name || "Untitled list"}"? Its addresses become mailable again.`, async () => {
                                                await deleteList.mutateAsync({ algo: l.algo, listName: l.list_name });
                                                toast.success("Hashed list removed");
                                            })
                                        }
                                        className="size-7 rounded-md text-slate-400 hover:text-rose-600 hover:bg-rose-50 inline-flex items-center justify-center transition-colors"
                                    >
                                        <Trash2Icon className="w-3.5 h-3.5" />
                                    </button>
                                )}
                            </div>
                        ))}
                    </div>
                )}
            </div>

            <SectionBar label="Addresses" count={total}>
                <div className="flex items-center gap-1">
                    {SOURCES.map((s) => (
                        <button
                            key={s.value}
                            type="button"
                            onClick={() => setSource(s.value)}
                            className={cn(
                                "h-6 px-2 rounded text-[11.5px] transition-colors",
                                source === s.value ? "bg-sky-50 text-sky-700 font-medium" : "text-slate-500 hover:text-slate-900 hover:bg-slate-50",
                            )}
                        >
                            {s.label}
                        </button>
                    ))}
                </div>
                <SearchInput value={q} onChange={setQ} placeholder="Search address or reason" className="w-full md:w-64" />
            </SectionBar>
            <PageBody>
                {list.isPending ? (
                    <div className="px-5 py-4 space-y-2">
                        {[0, 1, 2].map((i) => (
                            <div key={i} className="h-11 rounded bg-slate-50 animate-pulse" />
                        ))}
                    </div>
                ) : rows.length === 0 ? (
                    <div className="py-10">
                        <EmptyBlock
                            title={debounced || source ? "No matches" : "Nothing suppressed yet"}
                            body={
                                debounced || source
                                    ? "Try a shorter search or another filter."
                                    : "Bounces, complaints and unsubscribes land here automatically. Add or import addresses to block them by hand."
                            }
                        />
                    </div>
                ) : (
                    <div className="divide-y divide-slate-200/60">
                        {rows.map((r) => (
                            <SuppressionRow
                                key={r.id}
                                row={r}
                                canManage={canManage}
                                onRemove={() =>
                                    confirm.show(`Remove ${r.email} from the suppression list? It can be mailed again.`, async () => {
                                        await remove.mutateAsync(r.id);
                                        toast.success("Removed from suppression");
                                    })
                                }
                            />
                        ))}
                        {list.hasNextPage && (
                            <div className="px-5 py-3">
                                <button
                                    type="button"
                                    onClick={() => list.fetchNextPage()}
                                    disabled={list.isFetchingNextPage}
                                    className="h-7 px-3 rounded-md border border-slate-200 hover:border-slate-300 text-[12px] text-slate-700 inline-flex items-center gap-1.5 transition-colors"
                                >
                                    {list.isFetchingNextPage && <Loader2Icon className="w-3 h-3 animate-spin" />}
                                    Load more
                                </button>
                            </div>
                        )}
                    </div>
                )}
            </PageBody>

            <AddDialog open={addOpen} onClose={() => setAddOpen(false)} />
            <ImportDialog open={importOpen} onClose={() => setImportOpen(false)} />
        </Page>
    );
}

function ExportButton({ label, busy, onClick }: { label: string; busy: boolean; onClick: () => void }) {
    return (
        <button
            type="button"
            onClick={onClick}
            disabled={busy}
            title={`Export as ${label}`}
            className="h-7 px-2 rounded-md border border-slate-200 hover:border-slate-300 text-[11.5px] text-slate-600 hover:text-slate-900 inline-flex items-center gap-1 transition-colors"
        >
            {busy ? <Loader2Icon className="w-3 h-3 animate-spin" /> : <DownloadIcon className="w-3 h-3" />}
            {label}
        </button>
    );
}

function SuppressionRow({ row, canManage, onRemove }: { row: SuppressedRecipient; canManage: boolean; onRemove: () => void }) {
    return (
        <div className="group min-h-11 px-5 py-2 flex items-center gap-3 text-[12.5px]">
            <ShieldOffIcon className="w-3.5 h-3.5 text-slate-400 shrink-0" />
            <div className="min-w-0 flex-1">
                <div className="text-slate-900 truncate">{row.email}</div>
                {row.reason && <div className="text-[11.5px] text-slate-500 truncate">{row.reason}</div>}
            </div>
            <span className={cn("text-[10.5px] px-1.5 py-0.5 rounded capitalize shrink-0", SOURCE_TONE[row.source] ?? "bg-slate-100 text-slate-600")}>
                {row.source}
            </span>
            <span className="hidden md:inline w-28 text-right text-[11.5px] text-slate-400 tabular-nums shrink-0">
                {new Date(row.created_at).toLocaleDateString()}
            </span>
            {canManage && (
                <button
                    type="button"
                    aria-label={`Remove ${row.email}`}
                    onClick={onRemove}
                    className="size-7 rounded-md text-slate-400 hover:text-rose-600 hover:bg-rose-50 inline-flex items-center justify-center transition-colors opacity-100 md:opacity-0 md:group-hover:opacity-100"
                >
                    <Trash2Icon className="w-3.5 h-3.5" />
                </button>
            )}
        </div>
    );
}

function parseEmails(text: string): string[] {
    return text
        .split(/[\s,;]+/)
        .map((s) => s.trim())
        .filter(Boolean);
}

function AddDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
    const [text, setText] = React.useState("");
    const [reason, setReason] = React.useState("");
    const add = useAddSuppressions();
    const emails = parseEmails(text);

    React.useEffect(() => {
        if (!open) {
            setText("");
            setReason("");
        }
    }, [open]);

    async function submit() {
        try {
            const res = await toast.promise(add.mutateAsync({ emails, reason }), {
                loading: "Adding…",
                success: (r) => `${r.added} added${r.skipped ? `, ${r.skipped} already suppressed` : ""}${r.invalid ? `, ${r.invalid} invalid` : ""}`,
                error: (err: AppError) => buildError(err),
            });
            if (res) onClose();
        } catch {
            /* toast.promise already surfaced */
        }
    }

    return (
        <DialogShell open={open} onClose={onClose} eyebrow="Add" title="Suppressed addresses" icon={<PlusIcon className="w-3 h-3" />}>
            <div className="px-4 py-4 space-y-3 flex-1 min-h-0 overflow-y-auto">
                <div>
                    <Label>Addresses</Label>
                    <textarea
                        value={text}
                        onChange={(e) => setText(e.target.value)}
                        placeholder={"one@company.com\ntwo@company.com"}
                        rows={6}
                        autoFocus
                        className="w-full rounded-md border border-slate-200 px-2.5 py-2 text-[16px] md:text-[12.5px] text-slate-900 placeholder:text-slate-400 outline-none focus:border-sky-400 focus:ring-2 focus:ring-sky-100 font-mono"
                    />
                    <p className="mt-1 text-[11px] text-slate-500">One per line, or separated by commas. Up to 1,000; import a file for more.</p>
                </div>
                <div>
                    <Label>Reason</Label>
                    <TextInput value={reason} onChange={setReason} placeholder="Requested removal" className="w-full" />
                </div>
            </div>
            <DialogFooter
                note={emails.length ? `${emails.length} address${emails.length === 1 ? "" : "es"}` : "Nothing entered yet"}
                label="Suppress"
                ready={emails.length > 0 && emails.length <= 1000}
                busy={add.isPending}
                onSubmit={submit}
            />
        </DialogShell>
    );
}

const FORMATS: { value: SuppressionFormat; label: string; hint: string }[] = [
    { value: "plain", label: "Email addresses", hint: "One address per row; the first column is read." },
    { value: "md5", label: "MD5 hashes", hint: "32-character hex digests of the lowercased address." },
    { value: "sha256", label: "SHA-256 hashes", hint: "64-character hex digests of the lowercased address." },
];

function ImportDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
    const [file, setFile] = React.useState<File | null>(null);
    const [format, setFormat] = React.useState<SuppressionFormat>("plain");
    const [listName, setListName] = React.useState("");
    const [reason, setReason] = React.useState("");
    const imp = useImportSuppressions();
    const hashed = format !== "plain";

    React.useEffect(() => {
        if (!open) {
            setFile(null);
            setFormat("plain");
            setListName("");
            setReason("");
        }
    }, [open]);

    async function submit() {
        if (!file) return;
        try {
            const res = await toast.promise(imp.mutateAsync({ file, format, listName, reason }), {
                loading: "Importing…",
                success: (r) => `${r.added.toLocaleString()} added${r.skipped ? `, ${r.skipped.toLocaleString()} already listed` : ""}${r.invalid ? `, ${r.invalid.toLocaleString()} invalid rows` : ""}`,
                error: (err: AppError) => buildError(err),
            });
            if (res) onClose();
        } catch {
            /* toast.promise already surfaced */
        }
    }

    return (
        <DialogShell open={open} onClose={onClose} eyebrow="Import" title="Suppression file" icon={<UploadIcon className="w-3 h-3" />}>
            <div className="px-4 py-4 space-y-3 flex-1 min-h-0 overflow-y-auto">
                <div>
                    <Label>Format</Label>
                    <div className="grid grid-cols-3 gap-1.5">
                        {FORMATS.map((f) => (
                            <button
                                key={f.value}
                                type="button"
                                onClick={() => setFormat(f.value)}
                                className={cn(
                                    "h-8 rounded-md border text-[12px] transition-colors",
                                    format === f.value ? "border-sky-400 bg-sky-50 text-sky-700 font-medium" : "border-slate-200 text-slate-600 hover:border-slate-300",
                                )}
                            >
                                {f.label}
                            </button>
                        ))}
                    </div>
                    <p className="mt-1 text-[11px] text-slate-500">{FORMATS.find((f) => f.value === format)?.hint} A header row is skipped.</p>
                </div>
                <div>
                    <Label>File</Label>
                    <label className="flex items-center gap-2 h-9 px-2.5 rounded-md border border-dashed border-slate-300 hover:border-sky-400 cursor-pointer text-[12.5px] text-slate-600">
                        <UploadIcon className="w-3.5 h-3.5 text-slate-400" />
                        <span className="truncate">{file ? file.name : "Choose a .csv or .txt file (max 100 MB)"}</span>
                        <input
                            type="file"
                            accept=".csv,.txt,text/csv,text/plain"
                            className="hidden"
                            onChange={(e) => {
                                const f = e.target.files?.[0] ?? null;
                                setFile(f);
                                if (f && !listName) setListName(f.name.replace(/\.(csv|txt)$/i, ""));
                            }}
                        />
                    </label>
                </div>
                <div>
                    <Label>{hashed ? "List name" : "List name (optional)"}</Label>
                    <TextInput value={listName} onChange={setListName} placeholder="Advertiser or offer" className="w-full" />
                    {hashed && <p className="mt-1 text-[11px] text-slate-500">Used to find and remove this list later.</p>}
                </div>
                {!hashed && (
                    <div>
                        <Label>Reason</Label>
                        <TextInput value={reason} onChange={setReason} placeholder="Imported list" className="w-full" />
                    </div>
                )}
            </div>
            <DialogFooter
                note="Matched on every send, including the unibox."
                label="Import"
                ready={!!file && (!hashed || listName.trim().length > 0)}
                busy={imp.isPending}
                onSubmit={submit}
            />
        </DialogShell>
    );
}
