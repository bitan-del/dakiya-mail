// Brands: sender identities inside one workspace. Each brand owns a set of
// mailboxes, and their campaign mail ends with the brand's footer (name,
// postal address, an optional line and an unsubscribe link).

import React from "react";
import { Link } from "react-router-dom";
import { ArrowLeftIcon, BadgeCheckIcon, PencilIcon, PlusIcon, Trash2Icon } from "lucide-react";
import toast from "react-hot-toast";

import { NoAccess } from "@/components/layout/NoAccess";
import { EmptyBlock, Page, PageBody, PageTopbar, SectionBar, TopbarAction } from "@/components/layout/Page";
import { DialogFooter, DialogShell } from "@/components/ui/form-dialog";
import { Label, SearchInput, TextInput } from "@/components/ui/field";
import { Toggle } from "@/app/app/settings/_components/SectionShell";
import { useConfirm } from "@/hooks/context/confirm";
import { usePermission } from "@/hooks/usePermission";
import { cn } from "@/lib/utils";
import buildError from "@/lib/helper/buildError";
import type { AppError } from "@/lib/api/client/normalizeError";
import useEmails from "@/lib/api/hooks/app/emails/useEmails";
import { useBrands, useDeleteBrand, useSaveBrand } from "@/lib/api/hooks/app/brands/useBrands";
import type { Brand } from "@/lib/api/models/app/emails/Brand";

export default function BrandsPage() {
    const canView = usePermission("VIEW_CAMPAIGNS");
    const canManage = usePermission("MANAGE_EMAILS");
    const confirm = useConfirm();
    const brands = useBrands();
    const del = useDeleteBrand();
    const [editing, setEditing] = React.useState<Brand | "new" | null>(null);
    const rows = brands.data?.data ?? [];

    if (!canView) return <NoAccess feature="Brands" permissionLabel="View campaigns" />;

    return (
        <Page>
            <PageTopbar eyebrow="Brands" subtitle="Sender identities and the footer their campaign mail carries">
                {canManage && (
                    <TopbarAction icon={<PlusIcon className="w-3 h-3" />} onClick={() => setEditing("new")}>
                        <span className="hidden sm:inline">New brand</span>
                    </TopbarAction>
                )}
            </PageTopbar>
            <div className="px-5 pt-2">
                <Link to="/app/emails" className="inline-flex items-center gap-1 text-[11.5px] text-slate-500 hover:text-slate-900">
                    <ArrowLeftIcon className="w-3 h-3" />
                    Accounts
                </Link>
            </div>
            <SectionBar label="Brands" count={rows.length} />
            <PageBody>
                {brands.isPending ? (
                    <div className="px-5 py-4 space-y-2">
                        {[0, 1].map((i) => (
                            <div key={i} className="h-12 rounded bg-slate-50 animate-pulse" />
                        ))}
                    </div>
                ) : rows.length === 0 ? (
                    <div className="py-10">
                        <EmptyBlock
                            title="No brands yet"
                            body="Create a brand per client or product line, give it a postal address, and assign its mailboxes. Their campaign mail then ends with that brand's footer."
                        />
                    </div>
                ) : (
                    <div className="divide-y divide-slate-200/60">
                        {rows.map((b) => (
                            <div key={b.id} className="group min-h-12 px-5 py-2 flex items-center gap-3 text-[12.5px]">
                                <BadgeCheckIcon className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                                <button type="button" onClick={() => canManage && setEditing(b)} className="min-w-0 flex-1 text-left">
                                    <div className="text-slate-900 font-medium truncate">{b.name}</div>
                                    <div className="text-[11.5px] text-slate-500 truncate">
                                        {b.physical_address ? b.physical_address.replace(/\n/g, ", ") : "No postal address yet"}
                                    </div>
                                </button>
                                <span className={cn("text-[10.5px] px-1.5 py-0.5 rounded shrink-0", b.footer_enabled ? "bg-emerald-50 text-emerald-700" : "bg-slate-100 text-slate-600")}>
                                    {b.footer_enabled ? "Footer on" : "Footer off"}
                                </span>
                                <span className="font-mono text-[11.5px] text-slate-500 tabular-nums shrink-0">
                                    {b.mailbox_ids.length} mailbox{b.mailbox_ids.length === 1 ? "" : "es"}
                                </span>
                                {canManage && (
                                    <div className="flex items-center gap-1 opacity-100 md:opacity-0 md:group-hover:opacity-100 transition-opacity">
                                        <button
                                            type="button"
                                            aria-label={`Edit ${b.name}`}
                                            onClick={() => setEditing(b)}
                                            className="size-7 rounded-md text-slate-400 hover:text-slate-900 hover:bg-slate-100 inline-flex items-center justify-center"
                                        >
                                            <PencilIcon className="w-3.5 h-3.5" />
                                        </button>
                                        <button
                                            type="button"
                                            aria-label={`Delete ${b.name}`}
                                            onClick={() =>
                                                confirm.show(`Delete the brand "${b.name}"? Its mailboxes keep sending, without a brand footer.`, async () => {
                                                    await del.mutateAsync(b.id);
                                                    toast.success("Brand deleted");
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
            <BrandDialog
                open={editing !== null}
                onClose={() => setEditing(null)}
                brand={editing && editing !== "new" ? editing : undefined}
                otherBrands={rows}
            />
        </Page>
    );
}

function BrandDialog({ open, onClose, brand, otherBrands }: { open: boolean; onClose: () => void; brand?: Brand; otherBrands: Brand[] }) {
    const [name, setName] = React.useState("");
    const [website, setWebsite] = React.useState("");
    const [address, setAddress] = React.useState("");
    const [footerText, setFooterText] = React.useState("");
    const [footerEnabled, setFooterEnabled] = React.useState(true);
    const [unsubLink, setUnsubLink] = React.useState(true);
    const [mailboxes, setMailboxes] = React.useState<string[]>([]);
    const [q, setQ] = React.useState("");
    const { emails } = useEmails({ query: q, tag: "", limit: 100, enabled: open });
    const save = useSaveBrand();

    React.useEffect(() => {
        if (open) {
            setName(brand?.name ?? "");
            setWebsite(brand?.website ?? "");
            setAddress(brand?.physical_address ?? "");
            setFooterText(brand?.footer_text ?? "");
            setFooterEnabled(brand?.footer_enabled ?? true);
            setUnsubLink(brand?.unsubscribe_link ?? true);
            setMailboxes(brand?.mailbox_ids ?? []);
            setQ("");
        }
    }, [open, brand]);

    const ownerOf = React.useMemo(() => {
        const m = new Map<string, string>();
        for (const b of otherBrands) if (b.id !== brand?.id) for (const id of b.mailbox_ids) m.set(id, b.name);
        return m;
    }, [otherBrands, brand]);

    async function submit() {
        try {
            await toast.promise(
                save.mutateAsync({
                    id: brand?.id,
                    data: { name, website, physical_address: address, footer_text: footerText, footer_enabled: footerEnabled, unsubscribe_link: unsubLink },
                    mailboxIds: mailboxes,
                }),
                { loading: "Saving brand…", success: (b) => `${b.name} saved`, error: (err: AppError) => buildError(err) },
            );
            onClose();
        } catch {
            /* toast.promise already surfaced */
        }
    }

    const preview = [name.trim(), address.trim(), footerText.trim()].filter(Boolean);
    return (
        <DialogShell open={open} onClose={onClose} eyebrow={brand ? "Edit" : "New"} title="Brand" icon={<BadgeCheckIcon className="w-3 h-3" />}>
            <div className="px-4 py-4 space-y-3 flex-1 min-h-0 overflow-y-auto">
                <div className="grid grid-cols-2 gap-2">
                    <div>
                        <Label>Name</Label>
                        <TextInput value={name} onChange={setName} placeholder="Acme Lending" autoFocus={!brand} className="w-full" />
                    </div>
                    <div>
                        <Label>Website</Label>
                        <TextInput value={website} onChange={setWebsite} placeholder="https://acme.com" className="w-full" />
                    </div>
                </div>
                <div>
                    <Label>Postal address</Label>
                    <textarea
                        value={address}
                        onChange={(e) => setAddress(e.target.value)}
                        rows={2}
                        placeholder={"123 Main St, Suite 4\nAustin, TX 78701"}
                        className="w-full rounded-md border border-slate-200 px-2.5 py-2 text-[16px] md:text-[12.5px] text-slate-900 placeholder:text-slate-400 outline-none focus:border-sky-400 focus:ring-2 focus:ring-sky-100"
                    />
                    <p className="mt-1 text-[11px] text-slate-500">CAN-SPAM requires a valid postal address in commercial email.</p>
                </div>
                <div>
                    <Label>Extra footer line</Label>
                    <TextInput value={footerText} onChange={setFooterText} placeholder="You are receiving this because…" className="w-full" />
                </div>
                <div className="flex items-center justify-between text-[12.5px]">
                    <span className="text-slate-700">Add the footer to campaign mail</span>
                    <Toggle on={footerEnabled} onChange={setFooterEnabled} />
                </div>
                <div className="flex items-center justify-between text-[12.5px]">
                    <span className="text-slate-700">Include an unsubscribe link</span>
                    <Toggle on={unsubLink} onChange={setUnsubLink} />
                </div>
                {footerEnabled && preview.length > 0 && (
                    <div className="rounded-md border border-dashed border-slate-200 px-3 py-2 text-[11px] leading-relaxed text-slate-500 whitespace-pre-line">
                        {preview.join("\n")}
                        {unsubLink && <span className="block underline">Unsubscribe</span>}
                    </div>
                )}
                <div>
                    <Label>Mailboxes</Label>
                    <SearchInput value={q} onChange={setQ} placeholder="Search mailboxes" className="w-full mb-1.5" />
                    <div className="max-h-48 overflow-y-auto rounded-md border border-slate-200 divide-y divide-slate-100">
                        {emails.length === 0 ? (
                            <p className="px-3 py-2 text-[12px] text-slate-500">No mailboxes match.</p>
                        ) : (
                            emails.map((e) => {
                                const on = mailboxes.includes(e.id);
                                const other = ownerOf.get(e.id);
                                return (
                                    <button
                                        key={e.id}
                                        type="button"
                                        onClick={() => setMailboxes((m) => (on ? m.filter((x) => x !== e.id) : [...m, e.id]))}
                                        className={cn("w-full h-8 px-3 flex items-center gap-2 text-left text-[12px]", on ? "bg-sky-50 text-sky-800" : "text-slate-700 hover:bg-slate-50")}
                                    >
                                        <span className={cn("size-3.5 rounded border shrink-0", on ? "bg-sky-600 border-sky-600" : "border-slate-300")} />
                                        <span className="truncate flex-1">{e.email}</span>
                                        {other && !on && <span className="text-[10.5px] text-slate-400 truncate">in {other}</span>}
                                    </button>
                                );
                            })
                        )}
                    </div>
                    <p className="mt-1 text-[11px] text-slate-500">A mailbox belongs to one brand; picking it here moves it from any other.</p>
                </div>
            </div>
            <DialogFooter note={`${mailboxes.length} mailbox${mailboxes.length === 1 ? "" : "es"}`} label="Save" ready={name.trim().length > 0} busy={save.isPending} onSubmit={submit} />
        </DialogShell>
    );
}
