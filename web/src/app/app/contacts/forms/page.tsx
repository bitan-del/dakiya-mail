// Signup forms: hosted pages at <api>/f/<slug> that add the people who fill
// them in as contacts, with optional double opt-in and an embed snippet.

import React from "react";
import { Link } from "react-router-dom";
import { ArrowLeftIcon, CopyIcon, ExternalLinkIcon, FileInputIcon, PencilIcon, PlusIcon, Trash2Icon } from "lucide-react";
import toast from "react-hot-toast";

import { NoAccess } from "@/components/layout/NoAccess";
import { EmptyBlock, Page, PageBody, PageTopbar, SectionBar, TopbarAction } from "@/components/layout/Page";
import { DialogFooter, DialogShell } from "@/components/ui/form-dialog";
import { Label, TextInput } from "@/components/ui/field";
import { SelectMenu } from "@/components/ui/select-menu";
import { Toggle } from "@/app/app/settings/_components/SectionShell";
import CategoryPicker from "@/components/app/contacts/CategoryPicker";
import { useConfirm } from "@/hooks/context/confirm";
import { usePermission } from "@/hooks/usePermission";
import { cn } from "@/lib/utils";
import { API_URL } from "@/lib/information";
import buildError from "@/lib/helper/buildError";
import type { AppError } from "@/lib/api/client/normalizeError";
import useEmails from "@/lib/api/hooks/app/emails/useEmails";
import { useDeleteSignupForm, useSaveSignupForm, useSignupForms } from "@/lib/api/hooks/app/signup-forms/useSignupForms";
import type { SignupForm, SignupFormField } from "@/lib/api/models/app/contacts/SignupForm";

const FIELDS: { value: SignupFormField; label: string }[] = [
    { value: "first_name", label: "First name" },
    { value: "last_name", label: "Last name" },
    { value: "company", label: "Company" },
    { value: "phone", label: "Phone" },
];

const formURL = (slug: string) => `${API_URL.replace(/\/$/, "")}/f/${slug}`;

function copy(text: string, what: string) {
    navigator.clipboard.writeText(text).then(() => toast.success(`${what} copied`), () => toast.error("Could not copy"));
}

export default function SignupFormsPage() {
    const canView = usePermission("VIEW_CONTACTS");
    const canManage = usePermission("MANAGE_CONTACTS");
    const confirm = useConfirm();
    const forms = useSignupForms();
    const del = useDeleteSignupForm();
    const [editing, setEditing] = React.useState<SignupForm | "new" | null>(null);
    const rows = forms.data?.data ?? [];

    if (!canView) return <NoAccess feature="Signup forms" permissionLabel="View contacts" />;

    return (
        <Page>
            <PageTopbar eyebrow="Forms" subtitle="Hosted signup pages that add contacts">
                {canManage && (
                    <TopbarAction icon={<PlusIcon className="w-3 h-3" />} onClick={() => setEditing("new")}>
                        <span className="hidden sm:inline">New form</span>
                    </TopbarAction>
                )}
            </PageTopbar>
            <div className="px-5 pt-2">
                <Link to="/app/contacts" className="inline-flex items-center gap-1 text-[11.5px] text-slate-500 hover:text-slate-900">
                    <ArrowLeftIcon className="w-3 h-3" />
                    Contacts
                </Link>
            </div>
            <SectionBar label="Forms" count={rows.length} />
            <PageBody>
                {forms.isPending ? (
                    <div className="px-5 py-4 space-y-2">
                        {[0, 1].map((i) => (
                            <div key={i} className="h-12 rounded bg-slate-50 animate-pulse" />
                        ))}
                    </div>
                ) : rows.length === 0 ? (
                    <div className="py-10">
                        <EmptyBlock title="No forms yet" body="Create a form, share its link or embed it on your site, and everyone who signs up becomes a contact." />
                    </div>
                ) : (
                    <div className="divide-y divide-slate-200/60">
                        {rows.map((f) => (
                            <div key={f.id} className="group min-h-12 px-5 py-2 flex items-center gap-3 text-[12.5px]">
                                <FileInputIcon className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                                <div className="min-w-0 flex-1">
                                    <div className="text-slate-900 font-medium truncate">{f.name}</div>
                                    <div className="text-[11.5px] text-slate-500 truncate font-mono">{formURL(f.slug)}</div>
                                </div>
                                <span className={cn("text-[10.5px] px-1.5 py-0.5 rounded shrink-0", f.enabled ? "bg-emerald-50 text-emerald-700" : "bg-slate-100 text-slate-600")}>
                                    {f.enabled ? (f.double_opt_in ? "Live · double opt-in" : "Live") : "Off"}
                                </span>
                                <span className="hidden md:inline font-mono text-[11.5px] text-slate-500 tabular-nums shrink-0">
                                    {f.double_opt_in ? `${f.confirmations}/${f.submissions} confirmed` : `${f.submissions} signups`}
                                </span>
                                <div className="flex items-center gap-1">
                                    <a href={formURL(f.slug)} target="_blank" rel="noreferrer" aria-label="Open form" className="size-7 rounded-md text-slate-400 hover:text-slate-900 hover:bg-slate-100 inline-flex items-center justify-center">
                                        <ExternalLinkIcon className="w-3.5 h-3.5" />
                                    </a>
                                    <button
                                        type="button"
                                        onClick={() => copy(`<iframe src="${formURL(f.slug)}" style="width:100%;max-width:480px;height:520px;border:0" title="${f.name.replace(/"/g, "")}"></iframe>`, "Embed code")}
                                        className="h-7 px-2 rounded-md border border-slate-200 hover:border-slate-300 text-[11.5px] text-slate-700 inline-flex items-center gap-1"
                                    >
                                        <CopyIcon className="w-3 h-3" />
                                        Embed
                                    </button>
                                    {canManage && (
                                        <>
                                            <button type="button" aria-label={`Edit ${f.name}`} onClick={() => setEditing(f)} className="size-7 rounded-md text-slate-400 hover:text-slate-900 hover:bg-slate-100 inline-flex items-center justify-center">
                                                <PencilIcon className="w-3.5 h-3.5" />
                                            </button>
                                            <button
                                                type="button"
                                                aria-label={`Delete ${f.name}`}
                                                onClick={() =>
                                                    confirm.show(`Delete the form "${f.name}"? Its link stops working. Contacts it added stay.`, async () => {
                                                        await del.mutateAsync(f.id);
                                                        toast.success("Form deleted");
                                                    })
                                                }
                                                className="size-7 rounded-md text-slate-400 hover:text-rose-600 hover:bg-rose-50 inline-flex items-center justify-center"
                                            >
                                                <Trash2Icon className="w-3.5 h-3.5" />
                                            </button>
                                        </>
                                    )}
                                </div>
                            </div>
                        ))}
                    </div>
                )}
            </PageBody>
            <FormDialog open={editing !== null} onClose={() => setEditing(null)} form={editing && editing !== "new" ? editing : undefined} />
        </Page>
    );
}

function slugify(s: string) {
    return s
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, "-")
        .replace(/^-+|-+$/g, "")
        .slice(0, 63);
}

function FormDialog({ open, onClose, form }: { open: boolean; onClose: () => void; form?: SignupForm }) {
    const [name, setName] = React.useState("");
    const [slug, setSlug] = React.useState("");
    const [slugTouched, setSlugTouched] = React.useState(false);
    const [title, setTitle] = React.useState("");
    const [description, setDescription] = React.useState("");
    const [fields, setFields] = React.useState<SignupFormField[]>([]);
    const [categories, setCategories] = React.useState<string[]>([]);
    const [doubleOptIn, setDoubleOptIn] = React.useState(false);
    const [mailbox, setMailbox] = React.useState("");
    const [successMessage, setSuccessMessage] = React.useState("");
    const [redirectURL, setRedirectURL] = React.useState("");
    const [enabled, setEnabled] = React.useState(true);
    const { emails } = useEmails({ query: "", tag: "", limit: 100, enabled: open });
    const save = useSaveSignupForm();

    React.useEffect(() => {
        if (!open) return;
        setName(form?.name ?? "");
        setSlug(form?.slug ?? "");
        setSlugTouched(!!form);
        setTitle(form?.title ?? "");
        setDescription(form?.description ?? "");
        setFields(form?.fields ?? ["first_name"]);
        setCategories(form?.category_ids ?? []);
        setDoubleOptIn(form?.double_opt_in ?? false);
        setMailbox(form?.email_account_id ?? "");
        setSuccessMessage(form?.success_message ?? "");
        setRedirectURL(form?.redirect_url ?? "");
        setEnabled(form?.enabled ?? true);
    }, [open, form]);

    async function submit() {
        try {
            await toast.promise(
                save.mutateAsync({
                    id: form?.id,
                    data: {
                        name, slug, title, description, fields, category_ids: categories, double_opt_in: doubleOptIn,
                        email_account_id: doubleOptIn ? mailbox : mailbox || "", success_message: successMessage, redirect_url: redirectURL, enabled,
                    },
                }),
                { loading: "Saving form…", success: (f) => `${f.name} saved`, error: (err: AppError) => buildError(err) },
            );
            onClose();
        } catch {
            /* toast.promise already surfaced */
        }
    }

    const ready = name.trim().length > 0 && slug.length >= 3 && (!doubleOptIn || !!mailbox);
    return (
        <DialogShell open={open} onClose={onClose} eyebrow={form ? "Edit" : "New"} title="Signup form" icon={<FileInputIcon className="w-3 h-3" />}>
            <div className="px-4 py-4 space-y-3 flex-1 min-h-0 overflow-y-auto">
                <div className="grid grid-cols-2 gap-2">
                    <div>
                        <Label>Name</Label>
                        <TextInput
                            value={name}
                            onChange={(v) => {
                                setName(v);
                                if (!slugTouched) setSlug(slugify(v));
                            }}
                            placeholder="Newsletter"
                            autoFocus={!form}
                            className="w-full"
                        />
                    </div>
                    <div>
                        <Label>Address</Label>
                        <TextInput
                            value={slug}
                            onChange={(v) => {
                                setSlugTouched(true);
                                setSlug(slugify(v));
                            }}
                            placeholder="newsletter"
                            className="w-full font-mono"
                        />
                    </div>
                </div>
                <p className="-mt-1 text-[11px] text-slate-500 font-mono truncate">{formURL(slug || "…")}</p>
                <div>
                    <Label>Heading</Label>
                    <TextInput value={title} onChange={setTitle} placeholder="Get our weekly rates update" className="w-full" />
                </div>
                <div>
                    <Label>Description</Label>
                    <TextInput value={description} onChange={setDescription} placeholder="Optional" className="w-full" />
                </div>
                <div>
                    <Label>Fields besides email</Label>
                    <div className="flex flex-wrap gap-1.5">
                        {FIELDS.map((f) => (
                            <button
                                key={f.value}
                                type="button"
                                onClick={() => setFields((cur) => (cur.includes(f.value) ? cur.filter((x) => x !== f.value) : [...cur, f.value]))}
                                className={cn("h-7 px-2.5 rounded-md border text-[12px]", fields.includes(f.value) ? "border-sky-400 bg-sky-50 text-sky-700" : "border-slate-200 text-slate-600 hover:border-slate-300")}
                            >
                                {f.label}
                            </button>
                        ))}
                    </div>
                </div>
                <div>
                    <Label>Add to categories</Label>
                    <CategoryPicker value={categories} onChange={setCategories} />
                </div>
                <div className="flex items-center justify-between text-[12.5px]">
                    <span className="text-slate-700">Double opt-in (confirm by email first)</span>
                    <Toggle on={doubleOptIn} onChange={setDoubleOptIn} />
                </div>
                {doubleOptIn && (
                    <div>
                        <Label>Send confirmations from</Label>
                        <SelectMenu
                            value={mailbox}
                            onChange={setMailbox}
                            options={emails.map((e) => ({ value: e.id, label: e.email }))}
                            placeholder="Pick a mailbox"
                            fullWidth
                            aria-label="Confirmation mailbox"
                        />
                    </div>
                )}
                <div>
                    <Label>Thank-you message</Label>
                    <TextInput value={successMessage} onChange={setSuccessMessage} placeholder="Thanks, you're signed up." className="w-full" />
                </div>
                <div>
                    <Label>Redirect after signup</Label>
                    <TextInput value={redirectURL} onChange={setRedirectURL} placeholder="https://yoursite.com/thanks (optional)" className="w-full" />
                </div>
                <div className="flex items-center justify-between text-[12.5px]">
                    <span className="text-slate-700">Form is live</span>
                    <Toggle on={enabled} onChange={setEnabled} />
                </div>
            </div>
            <DialogFooter
                note={doubleOptIn && !mailbox ? "Pick a mailbox for confirmations." : "Signups become subscribed contacts."}
                label="Save"
                ready={ready}
                busy={save.isPending}
                onSubmit={submit}
            />
        </DialogShell>
    );
}
