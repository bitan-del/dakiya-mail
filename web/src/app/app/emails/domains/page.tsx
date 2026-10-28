// Sender domains: the SPF, DKIM and DMARC records each sending domain should
// publish for its provider, checked against live DNS. Verifying also refreshes
// the authentication state of every mailbox on the domain.

import React from "react";
import { Link } from "react-router-dom";
import { ArrowLeftIcon, CheckIcon, ChevronRightIcon, CopyIcon, GlobeIcon, Loader2Icon, PlusIcon, RefreshCwIcon, Trash2Icon, XIcon } from "lucide-react";
import toast from "react-hot-toast";

import { NoAccess } from "@/components/layout/NoAccess";
import { EmptyBlock, Page, PageBody, PageTopbar, SectionBar, TopbarAction } from "@/components/layout/Page";
import { DialogFooter, DialogShell } from "@/components/ui/form-dialog";
import { Label, TextInput } from "@/components/ui/field";
import { useConfirm } from "@/hooks/context/confirm";
import { usePermission } from "@/hooks/usePermission";
import { cn } from "@/lib/utils";
import buildError from "@/lib/helper/buildError";
import type { AppError } from "@/lib/api/client/normalizeError";
import { useAddDomain, useDeleteDomain, useDomains, useUpdateDomain, useVerifyDomain } from "@/lib/api/hooks/app/domains/useDomains";
import type { DNSRecord, SendingDomain, SendingDomainProvider } from "@/lib/api/models/app/emails/SendingDomain";

const PROVIDERS: { value: SendingDomainProvider; label: string; selectorHint: string }[] = [
    { value: "amazon_ses", label: "Amazon SES", selectorHint: "The three Easy DKIM tokens from the SES console, one per line" },
    { value: "google_workspace", label: "Google Workspace", selectorHint: "Leave empty to check the google selector" },
    { value: "microsoft_365", label: "Microsoft 365", selectorHint: "Leave empty to check selector1 and selector2" },
    { value: "other", label: "Other", selectorHint: "Your provider's DKIM selector, one per line" },
];
const PROVIDER_LABEL = Object.fromEntries(PROVIDERS.map((p) => [p.value, p.label])) as Record<SendingDomainProvider, string>;

const STATE: Record<SendingDomain["state"], { label: string; tone: string }> = {
    passing: { label: "Verified", tone: "bg-emerald-50 text-emerald-700" },
    failing: { label: "Needs records", tone: "bg-rose-50 text-rose-700" },
    unknown: { label: "Could not check", tone: "bg-amber-50 text-amber-700" },
    pending: { label: "Not checked", tone: "bg-slate-100 text-slate-600" },
};

export default function DomainsPage() {
    const canView = usePermission("VIEW_CAMPAIGNS");
    const canManage = usePermission("MANAGE_EMAILS");
    const domains = useDomains();
    const [open, setOpen] = React.useState<string | null>(null);
    const [adding, setAdding] = React.useState(false);
    const [editing, setEditing] = React.useState<SendingDomain | null>(null);
    const rows = domains.data?.data ?? [];

    if (!canView) return <NoAccess feature="Sender domains" permissionLabel="View campaigns" />;

    return (
        <Page>
            <PageTopbar eyebrow="Domains" subtitle="SPF, DKIM and DMARC for every domain you send from">
                {canManage && (
                    <TopbarAction icon={<PlusIcon className="w-3 h-3" />} onClick={() => setAdding(true)}>
                        <span className="hidden sm:inline">Add domain</span>
                    </TopbarAction>
                )}
            </PageTopbar>
            <div className="px-5 pt-2">
                <Link to="/app/emails" className="inline-flex items-center gap-1 text-[11.5px] text-slate-500 hover:text-slate-900">
                    <ArrowLeftIcon className="w-3 h-3" />
                    Accounts
                </Link>
            </div>
            <SectionBar label="Domains" count={rows.length} />
            <PageBody>
                {domains.isPending ? (
                    <div className="px-5 py-4 space-y-2">
                        {[0, 1].map((i) => (
                            <div key={i} className="h-12 rounded bg-slate-50 animate-pulse" />
                        ))}
                    </div>
                ) : rows.length === 0 ? (
                    <div className="py-10">
                        <EmptyBlock
                            title="No sender domains yet"
                            body="Add the domain your mailboxes send from to get the exact DNS records to publish, then verify them."
                        />
                    </div>
                ) : (
                    <div className="divide-y divide-slate-200/60">
                        {rows.map((d) => (
                            <DomainRow
                                key={d.id}
                                domain={d}
                                open={open === d.id}
                                canManage={canManage}
                                onToggle={() => setOpen(open === d.id ? null : d.id)}
                                onEdit={() => setEditing(d)}
                            />
                        ))}
                    </div>
                )}
            </PageBody>
            <DomainDialog open={adding} onClose={() => setAdding(false)} />
            <DomainDialog open={!!editing} onClose={() => setEditing(null)} domain={editing ?? undefined} />
        </Page>
    );
}

function DomainRow({ domain: d, open, canManage, onToggle, onEdit }: { domain: SendingDomain; open: boolean; canManage: boolean; onToggle: () => void; onEdit: () => void }) {
    const verify = useVerifyDomain();
    const del = useDeleteDomain();
    const confirm = useConfirm();
    const st = STATE[d.state];
    return (
        <div>
            <div className="min-h-12 px-5 py-2 flex items-center gap-3 text-[12.5px]">
                <button type="button" onClick={onToggle} className="flex items-center gap-3 min-w-0 flex-1 text-left">
                    <ChevronRightIcon className={cn("w-3 h-3 text-slate-400 shrink-0 transition-transform", open && "rotate-90")} />
                    <GlobeIcon className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                    <div className="min-w-0">
                        <div className="text-slate-900 font-medium truncate">{d.domain}</div>
                        <div className="text-[11.5px] text-slate-500 truncate">
                            {PROVIDER_LABEL[d.provider]} · {d.mailbox_count} mailbox{d.mailbox_count === 1 ? "" : "es"}
                            {d.last_checked_at && ` · checked ${new Date(d.last_checked_at).toLocaleString()}`}
                        </div>
                    </div>
                </button>
                <span className={cn("text-[10.5px] px-1.5 py-0.5 rounded shrink-0", st.tone)}>{st.label}</span>
                {canManage && (
                    <>
                        <button
                            type="button"
                            onClick={() =>
                                toast.promise(verify.mutateAsync(d.id), {
                                    loading: `Checking ${d.domain}…`,
                                    success: (r) => `${r.domain}: ${STATE[r.state].label}`,
                                    error: (err: AppError) => buildError(err),
                                })
                            }
                            disabled={verify.isPending}
                            className="h-7 px-2 rounded-md border border-slate-200 hover:border-slate-300 text-[11.5px] text-slate-700 inline-flex items-center gap-1"
                        >
                            {verify.isPending ? <Loader2Icon className="w-3 h-3 animate-spin" /> : <RefreshCwIcon className="w-3 h-3" />}
                            Verify
                        </button>
                        <button
                            type="button"
                            aria-label={`Remove ${d.domain}`}
                            onClick={() =>
                                confirm.show(`Remove ${d.domain} from sender domains? Mailboxes on it keep working.`, async () => {
                                    await del.mutateAsync(d.id);
                                    toast.success("Domain removed");
                                })
                            }
                            className="size-7 rounded-md text-slate-400 hover:text-rose-600 hover:bg-rose-50 inline-flex items-center justify-center"
                        >
                            <Trash2Icon className="w-3.5 h-3.5" />
                        </button>
                    </>
                )}
            </div>
            {open && (
                <div className="px-5 pb-4 pl-[52px] space-y-2">
                    <div className="rounded-md border border-slate-200 divide-y divide-slate-100 overflow-hidden">
                        {d.records.map((r, i) => (
                            <RecordRow key={i} record={r} />
                        ))}
                    </div>
                    <div className="flex items-center gap-3 text-[11.5px] text-slate-500">
                        <span className="truncate">{d.check_summary || "Not checked yet."}</span>
                        {canManage && (
                            <button type="button" onClick={onEdit} className="ml-auto text-sky-700 hover:underline shrink-0">
                                Change provider or DKIM selectors
                            </button>
                        )}
                    </div>
                </div>
            )}
        </div>
    );
}

function RecordRow({ record: r }: { record: DNSRecord }) {
    return (
        <div className="px-3 py-2 grid grid-cols-[64px_56px_1fr_auto] gap-x-3 gap-y-1 items-start text-[12px]">
            <span className="uppercase text-[10.5px] tracking-[0.1em] text-slate-400 pt-0.5">{r.purpose}</span>
            <span className="font-mono text-[11px] text-slate-500 pt-0.5">{r.type}</span>
            <div className="min-w-0 space-y-1">
                <CopyLine label="Host" value={r.host} />
                <CopyLine label="Value" value={r.value} />
                {r.note && <p className="text-[11px] text-slate-500">{r.note}</p>}
            </div>
            <span className="pt-0.5">
                {r.found === null ? (
                    <span className="text-[10.5px] text-slate-400">unchecked</span>
                ) : r.found ? (
                    <span className="inline-flex items-center gap-1 text-[10.5px] text-emerald-700"><CheckIcon className="w-3 h-3" />found</span>
                ) : (
                    <span className="inline-flex items-center gap-1 text-[10.5px] text-rose-700"><XIcon className="w-3 h-3" />missing</span>
                )}
            </span>
        </div>
    );
}

function CopyLine({ label, value }: { label: string; value: string }) {
    const copyable = !value.includes("<") && !/^(The key|Your provider)/.test(value);
    return (
        <div className="flex items-center gap-2 min-w-0">
            <span className="w-10 shrink-0 text-[10.5px] text-slate-400">{label}</span>
            <span className="font-mono text-[11.5px] text-slate-800 truncate" title={value}>{value}</span>
            {copyable && (
                <button
                    type="button"
                    aria-label={`Copy ${label.toLowerCase()}`}
                    onClick={() => navigator.clipboard.writeText(value).then(() => toast.success("Copied"), () => toast.error("Could not copy"))}
                    className="size-5 shrink-0 rounded text-slate-400 hover:text-slate-900 hover:bg-slate-100 inline-flex items-center justify-center"
                >
                    <CopyIcon className="w-3 h-3" />
                </button>
            )}
        </div>
    );
}

function DomainDialog({ open, onClose, domain }: { open: boolean; onClose: () => void; domain?: SendingDomain }) {
    const [name, setName] = React.useState("");
    const [provider, setProvider] = React.useState<SendingDomainProvider>("amazon_ses");
    const [selectors, setSelectors] = React.useState("");
    const add = useAddDomain();
    const update = useUpdateDomain();

    React.useEffect(() => {
        if (open) {
            setName(domain?.domain ?? "");
            setProvider(domain?.provider ?? "amazon_ses");
            setSelectors((domain?.dkim_selectors ?? []).join("\n"));
        }
    }, [open, domain]);

    async function submit() {
        const data = {
            domain: name.trim(),
            provider,
            dkim_selectors: selectors.split(/[\s,]+/).map((s) => s.trim()).filter(Boolean),
        };
        try {
            await toast.promise(domain ? update.mutateAsync({ id: domain.id, data }) : add.mutateAsync(data), {
                loading: "Checking DNS…",
                success: (d) => `${d.domain}: ${STATE[d.state].label}`,
                error: (err: AppError) => buildError(err),
            });
            onClose();
        } catch {
            /* toast.promise already surfaced */
        }
    }

    const hint = PROVIDERS.find((p) => p.value === provider)?.selectorHint;
    return (
        <DialogShell open={open} onClose={onClose} eyebrow={domain ? "Edit" : "Add"} title="Sender domain" icon={<GlobeIcon className="w-3 h-3" />}>
            <div className="px-4 py-4 space-y-3 flex-1 min-h-0 overflow-y-auto">
                <div>
                    <Label>Domain</Label>
                    <TextInput value={name} onChange={setName} placeholder="mail.example.com" autoFocus={!domain} disabled={!!domain} className="w-full" />
                </div>
                <div>
                    <Label>Sends through</Label>
                    <div className="grid grid-cols-2 gap-1.5">
                        {PROVIDERS.map((p) => (
                            <button
                                key={p.value}
                                type="button"
                                onClick={() => setProvider(p.value)}
                                className={cn(
                                    "h-8 rounded-md border text-[12px] transition-colors",
                                    provider === p.value ? "border-sky-400 bg-sky-50 text-sky-700 font-medium" : "border-slate-200 text-slate-600 hover:border-slate-300",
                                )}
                            >
                                {p.label}
                            </button>
                        ))}
                    </div>
                </div>
                <div>
                    <Label>DKIM selectors</Label>
                    <textarea
                        value={selectors}
                        onChange={(e) => setSelectors(e.target.value)}
                        rows={3}
                        placeholder={provider === "amazon_ses" ? "abcdef...\nghijkl...\nmnopqr..." : "selector"}
                        className="w-full rounded-md border border-slate-200 px-2.5 py-2 text-[16px] md:text-[12.5px] font-mono text-slate-900 placeholder:text-slate-400 outline-none focus:border-sky-400 focus:ring-2 focus:ring-sky-100"
                    />
                    <p className="mt-1 text-[11px] text-slate-500">{hint}</p>
                </div>
            </div>
            <DialogFooter
                note="DNS is checked as soon as you save."
                label={domain ? "Save" : "Add domain"}
                ready={name.trim().length > 3}
                busy={add.isPending || update.isPending}
                onSubmit={submit}
            />
        </DialogShell>
    );
}
