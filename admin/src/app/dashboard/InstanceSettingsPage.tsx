// Instance settings: the only writable configuration in the product. These
// keys are disjoint from the environment, so nothing here is overwritten at the
// next boot, except the AI provider, which wins over the AI_* variables when saved.

import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { CheckCircle2, Plug, Save, SlidersHorizontal, XCircle } from "lucide-react";
import { PageHeader } from "@/components/layout/PageHeader";
import { ErrorState } from "@/components/ErrorState";
import { Button } from "@/components/ui/button";
import {
    Card,
    CardContent,
    CardDescription,
    CardHeader,
    CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import {
    getInstanceSettings,
    putInstanceSettings,
    testInstanceAI,
    type InstanceSettings,
    type InstanceSettingsUpdate,
} from "@/lib/api/client/admin/instance";

const SETTINGS_KEY = ["admin", "instance", "settings"];

// The backend clamps to the same band; matching it here keeps the operator
// from spending a round-trip to learn the range.
const TTL_MIN_HOURS = 1;
const TTL_MAX_HOURS = 720;

// Sending-domain authentication grace, mirroring internal/app/instancesettings.
const AUTH_GRACE_MIN_HOURS = 1;
const AUTH_GRACE_MAX_HOURS = 720;

// Mailbox sync fair-use bands, mirroring internal/config/constants.go.
const SYNC_FIELDS = [
    {
        key: "backfillDays",
        setting: "backfill_days",
        label: "Import window (days)",
        min: 1,
        max: 730,
        help: "How far back the initial import reaches when a mailbox is connected. Newest mail first.",
    },
    {
        key: "backfillMessages",
        setting: "backfill_messages",
        label: "Import cap (messages per mailbox)",
        min: 1,
        max: 100000,
        help: "The most messages the initial import stores for one mailbox, whatever the window holds.",
    },
    {
        key: "dailyPerMailbox",
        setting: "daily_messages_per_mailbox",
        label: "Daily budget (messages per mailbox)",
        min: 1,
        max: 100000,
        help: "New mail one mailbox may store per UTC day. Over it, mail waits for the next day; replies to the mailbox's own sends have a separate budget of the same size and keep landing.",
    },
    {
        key: "dailyPerOrg",
        setting: "daily_messages_per_org",
        label: "Daily budget (messages per organization)",
        min: 1,
        max: 2000000,
        help: "New plus imported mail across one organization per UTC day.",
    },
] as const;

type SyncFieldKey = (typeof SYNC_FIELDS)[number]["key"];

// AI provider presets, mirroring internal/pkg/generation/resolve.go. The empty
// provider is "env" here because a Select item cannot carry an empty value.
const AI_ENV = "env";

interface AIPreset {
    value: string;
    label: string;
    baseUrl: string;
    model: string;
    modelHint?: string;
}

const AI_PRESETS: AIPreset[] = [
    { value: AI_ENV, label: "Server environment (AI_* variables)", baseUrl: "", model: "" },
    {
        value: "openai",
        label: "OpenAI",
        baseUrl: "https://api.openai.com/v1",
        model: "",
        modelHint: "Blank uses gpt-4o-mini, and gpt-4o on paid plans.",
    },
    {
        value: "openrouter",
        label: "OpenRouter",
        baseUrl: "https://openrouter.ai/api/v1",
        model: "meta-llama/llama-3.1-8b-instruct:free",
    },
    {
        value: "groq",
        label: "Groq",
        baseUrl: "https://api.groq.com/openai/v1",
        model: "openai/gpt-oss-20b",
    },
    { value: "anthropic", label: "Anthropic", baseUrl: "", model: "" },
    {
        value: "ollama",
        label: "Ollama (local)",
        baseUrl: "http://localhost:11434/v1",
        model: "llama3.1",
    },
    { value: "custom", label: "Custom OpenAI-compatible", baseUrl: "", model: "" },
];

function aiPreset(value: string): AIPreset {
    return AI_PRESETS.find((p) => p.value === value) ?? AI_PRESETS[0];
}

interface AIFormState {
    provider: string;
    model: string;
    baseUrl: string;
    // Write-only: never seeded from the server, which returns no key.
    apiKey: string;
    clearKey: boolean;
}

interface FormState {
    linksEnabled: boolean;
    ttlHours: string;
    allowInvitedSignup: boolean;
    sync: Record<SyncFieldKey, string>;
    enforceDomainAuth: boolean;
    authGraceHours: string;
    ai: AIFormState;
}

function toForm(s: InstanceSettings): FormState {
    return {
        linksEnabled: s.invitations.links_enabled,
        ttlHours: String(s.invitations.ttl_hours),
        allowInvitedSignup: s.access.allow_invited_signup,
        sync: {
            backfillDays: String(s.sync.backfill_days),
            backfillMessages: String(s.sync.backfill_messages),
            dailyPerMailbox: String(s.sync.daily_messages_per_mailbox),
            dailyPerOrg: String(s.sync.daily_messages_per_org),
        },
        enforceDomainAuth: s.deliverability.enforce_domain_auth,
        authGraceHours: String(s.deliverability.auth_grace_hours),
        ai: {
            provider: s.ai.provider || AI_ENV,
            model: s.ai.model,
            baseUrl: s.ai.base_url,
            apiKey: "",
            clearKey: false,
        },
    };
}

function aiDirtyAgainst(s: InstanceSettings, f: AIFormState): boolean {
    return (
        f.provider !== (s.ai.provider || AI_ENV) ||
        f.model !== s.ai.model ||
        f.baseUrl !== s.ai.base_url ||
        f.apiKey !== "" ||
        f.clearKey
    );
}

// The first problem with the AI form, or null. A provider switch drops the
// saved key server-side, so a key must be typed unless the provider is unchanged.
function aiProblem(s: InstanceSettings, f: AIFormState): string | null {
    if (f.provider === AI_ENV) return null;
    if (f.baseUrl.trim() !== "" && !/^https?:\/\/\S+$/i.test(f.baseUrl.trim())) {
        return "The base URL must start with http:// or https://.";
    }
    if (f.provider === "custom" && (f.baseUrl.trim() === "" || f.model.trim() === "")) {
        return "A custom provider needs a base URL and a model.";
    }
    const keptKey = s.ai.api_key_set && f.provider === s.ai.provider && !f.clearKey;
    if (f.provider !== "ollama" && !keptKey && f.apiKey.trim() === "") {
        return "Enter an API key for this provider.";
    }
    return null;
}

function syncFieldValid(raw: string, min: number, max: number): boolean {
    const n = Number(raw);
    return raw.trim() !== "" && Number.isInteger(n) && n >= min && n <= max;
}

export default function InstanceSettingsPage() {
    const qc = useQueryClient();
    const [form, setForm] = useState<FormState | null>(null);

    const settingsQ = useQuery({
        queryKey: SETTINGS_KEY,
        queryFn: getInstanceSettings,
        retry: false,
    });

    // Reseed whenever a fresh document arrives so a background refetch does
    // not silently keep an operator editing a stale form.
    useEffect(() => {
        if (settingsQ.data) setForm(toForm(settingsQ.data));
    }, [settingsQ.data]);

    const saveMut = useMutation({
        mutationFn: (body: InstanceSettingsUpdate) => putInstanceSettings(body),
        onSuccess: (saved) => {
            qc.setQueryData(SETTINGS_KEY, saved);
            setForm(toForm(saved));
            testMut.reset();
            toast.success("Instance settings saved");
        },
        onError: (err: Error) => toast.error(err.message || "Could not save settings"),
    });

    const testMut = useMutation({
        mutationFn: (ai: AIFormState) =>
            testInstanceAI({
                provider: ai.provider === AI_ENV ? "" : ai.provider,
                model: ai.model.trim(),
                base_url: ai.baseUrl.trim(),
                api_key: ai.apiKey.trim() || undefined,
            }),
        onError: (err: Error) => toast.error(err.message || "Could not run the test"),
    });

    const server = settingsQ.data;
    const syncDirty =
        !!server &&
        !!form &&
        SYNC_FIELDS.some((f) => form.sync[f.key] !== String(server.sync[f.setting]));
    const aiDirty = !!server && !!form && aiDirtyAgainst(server, form.ai);
    const aiError = server && form && aiDirty ? aiProblem(server, form.ai) : null;
    const dirty =
        !!server &&
        !!form &&
        (form.linksEnabled !== server.invitations.links_enabled ||
            form.ttlHours !== String(server.invitations.ttl_hours) ||
            form.allowInvitedSignup !== server.access.allow_invited_signup ||
            form.enforceDomainAuth !== server.deliverability.enforce_domain_auth ||
            form.authGraceHours !== String(server.deliverability.auth_grace_hours) ||
            syncDirty ||
            aiDirty);
    const syncValid =
        form !== null && SYNC_FIELDS.every((f) => syncFieldValid(form.sync[f.key], f.min, f.max));

    const authGrace = form ? Number(form.authGraceHours) : NaN;
    const authGraceValid =
        form !== null &&
        form.authGraceHours.trim() !== "" &&
        Number.isInteger(authGrace) &&
        authGrace >= AUTH_GRACE_MIN_HOURS &&
        authGrace <= AUTH_GRACE_MAX_HOURS;

    const ttl = form ? Number(form.ttlHours) : NaN;
    const ttlValid =
        form !== null &&
        form.ttlHours.trim() !== "" &&
        Number.isInteger(ttl) &&
        ttl >= TTL_MIN_HOURS &&
        ttl <= TTL_MAX_HOURS;

    function save() {
        if (!form) return;
        if (!ttlValid) {
            toast.error(
                `Invitation validity must be a whole number of hours between ${TTL_MIN_HOURS} and ${TTL_MAX_HOURS}`,
            );
            return;
        }
        if (!syncValid) {
            toast.error("Every sync budget must be a whole number inside its range");
            return;
        }
        if (!authGraceValid) {
            toast.error(
                `The authentication grace period must be a whole number of hours between ${AUTH_GRACE_MIN_HOURS} and ${AUTH_GRACE_MAX_HOURS}`,
            );
            return;
        }
        if (aiError) {
            toast.error(aiError);
            return;
        }
        const apiKey = form.ai.apiKey.trim();
        saveMut.mutate(
            {
                invitations: { links_enabled: form.linksEnabled, ttl_hours: ttl },
                access: { allow_invited_signup: form.allowInvitedSignup },
                sync: {
                    backfill_days: Number(form.sync.backfillDays),
                    backfill_messages: Number(form.sync.backfillMessages),
                    daily_messages_per_mailbox: Number(form.sync.dailyPerMailbox),
                    daily_messages_per_org: Number(form.sync.dailyPerOrg),
                },
                deliverability: {
                    enforce_domain_auth: form.enforceDomainAuth,
                    auth_grace_hours: authGrace,
                },
                ai: {
                    provider: form.ai.provider === AI_ENV ? "" : form.ai.provider,
                    model: form.ai.model.trim(),
                    base_url: form.ai.baseUrl.trim(),
                    ...(apiKey ? { api_key: apiKey } : {}),
                    ...(form.ai.clearKey ? { clear_api_key: true } : {}),
                },
            },
            {
                onSuccess: () => {
                    if (aiDirty) {
                        toast.info("AI provider saved. Restart the backend and consumer to apply it.");
                    }
                },
            },
        );
    }

    function setAI(patch: Partial<AIFormState>) {
        if (!form) return;
        testMut.reset();
        setForm({ ...form, ai: { ...form.ai, ...patch } });
    }

    // Picking a provider fills its endpoint and model. Switching back to the saved
    // provider restores the saved values, so the form is clean again.
    function pickProvider(value: string) {
        if (!server) return;
        const preset = aiPreset(value);
        const back = value === (server.ai.provider || AI_ENV);
        setAI({
            provider: value,
            model: back ? server.ai.model : preset.model,
            baseUrl: back ? server.ai.base_url : preset.baseUrl,
            apiKey: "",
            clearKey: false,
        });
    }

    return (
        <div>
            <PageHeader
                title="Instance settings"
                description="These settings are stored in the database. Only the AI provider overlaps the environment, and a saved one wins."
            >
                <Button size="sm" variant="outline" asChild>
                    <Link to="/configuration">
                        <SlidersHorizontal className="size-4" />
                        Configuration
                    </Link>
                </Button>
                <Button
                    size="sm"
                    onClick={save}
                    disabled={!dirty || saveMut.isPending || !form}
                >
                    <Save className="size-4" />
                    {saveMut.isPending ? "Saving..." : "Save changes"}
                </Button>
            </PageHeader>

            {settingsQ.isLoading && (
                <div className="space-y-3">
                    <Skeleton className="h-40 w-full" />
                    <Skeleton className="h-28 w-full" />
                </div>
            )}

            {settingsQ.isError && (
                <ErrorState
                    error={settingsQ.error}
                    title="Could not load instance settings"
                    onRetry={() => settingsQ.refetch()}
                />
            )}

            {form && (
                <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
                    <Card>
                        <CardHeader>
                            <CardTitle>Invitations</CardTitle>
                            <CardDescription>
                                How people are brought into a workspace from Settings, Members in
                                the dashboard.
                            </CardDescription>
                        </CardHeader>
                        <CardContent className="space-y-3 pt-0">
                            <div className="flex items-center justify-between gap-3 rounded-md border border-border p-3">
                                <div className="min-w-0">
                                    <p className="text-sm font-medium">Invitation links</p>
                                    <p className="text-xs text-muted-foreground">
                                        Show a copyable link next to each pending invitation. Leave
                                        this on when the platform mail transport does not deliver,
                                        otherwise an invited person never receives anything.
                                    </p>
                                </div>
                                <Switch
                                    checked={form.linksEnabled}
                                    onCheckedChange={(v) =>
                                        setForm({ ...form, linksEnabled: v })
                                    }
                                />
                            </div>

                            <div>
                                <Label htmlFor="ttl-hours">Invitation validity (hours)</Label>
                                {/* Text, not number: the native spinner is not ours, and the value is already validated as a string. */}
                                <Input
                                    id="ttl-hours"
                                    type="text"
                                    inputMode="numeric"
                                    autoComplete="off"
                                    value={form.ttlHours}
                                    onChange={(e) =>
                                        setForm({ ...form, ttlHours: e.target.value })
                                    }
                                    aria-invalid={!ttlValid}
                                    className="mt-1"
                                />
                                <p className="mt-1 text-xs text-muted-foreground">
                                    Between {TTL_MIN_HOURS} and {TTL_MAX_HOURS} hours (30 days).
                                    Existing invitations keep the expiry they were issued with.
                                </p>
                                {!ttlValid && (
                                    <p className="mt-1 text-xs text-red-600">
                                        Enter a whole number of hours between {TTL_MIN_HOURS} and{" "}
                                        {TTL_MAX_HOURS}.
                                    </p>
                                )}
                            </div>
                        </CardContent>
                    </Card>

                    <Card>
                        <CardHeader>
                            <CardTitle>Access</CardTitle>
                            <CardDescription>
                                Who may create an account on this instance. The registration mode
                                itself is owned by the environment and is listed on Configuration.
                            </CardDescription>
                        </CardHeader>
                        <CardContent className="pt-0">
                            <div className="flex items-center justify-between gap-3 rounded-md border border-border p-3">
                                <div className="min-w-0">
                                    <p className="text-sm font-medium">Allow invited sign-up</p>
                                    <p className="text-xs text-muted-foreground">
                                        Someone holding a valid invitation can create an account
                                        even though open sign-ups are closed. Turning this off
                                        means only existing accounts can sign in.
                                    </p>
                                </div>
                                <Switch
                                    checked={form.allowInvitedSignup}
                                    onCheckedChange={(v) =>
                                        setForm({ ...form, allowInvitedSignup: v })
                                    }
                                />
                            </div>
                        </CardContent>
                    </Card>

                    <Card className="lg:col-span-2">
                        <CardHeader>
                            <CardTitle>Mailbox sync fair use</CardTitle>
                            <CardDescription>
                                What a connected mailbox imports and how much new mail it may
                                store. Mail over a budget waits and is picked up when the window
                                rolls; nothing is dropped, and replies to the mailbox&apos;s own
                                outreach are never held. Changes apply the next time a mailbox is
                                loaded onto a worker (within a few minutes). The fixed pacing
                                numbers are listed under Limits.
                            </CardDescription>
                        </CardHeader>
                        <CardContent className="grid grid-cols-1 gap-3 pt-0 md:grid-cols-2">
                            {SYNC_FIELDS.map((f) => {
                                const valid = syncFieldValid(form.sync[f.key], f.min, f.max);
                                return (
                                    <div key={f.key}>
                                        <Label htmlFor={`sync-${f.key}`}>{f.label}</Label>
                                        <Input
                                            id={`sync-${f.key}`}
                                            type="text"
                                            inputMode="numeric"
                                            autoComplete="off"
                                            value={form.sync[f.key]}
                                            onChange={(e) =>
                                                setForm({
                                                    ...form,
                                                    sync: { ...form.sync, [f.key]: e.target.value },
                                                })
                                            }
                                            aria-invalid={!valid}
                                            className="mt-1"
                                        />
                                        <p className="mt-1 text-xs text-muted-foreground">
                                            {f.help} Between {f.min.toLocaleString()} and{" "}
                                            {f.max.toLocaleString()}.
                                        </p>
                                        {!valid && (
                                            <p className="mt-1 text-xs text-red-600">
                                                Enter a whole number between {f.min.toLocaleString()}{" "}
                                                and {f.max.toLocaleString()}.
                                            </p>
                                        )}
                                    </div>
                                );
                            })}
                        </CardContent>
                    </Card>

                    <Card className="lg:col-span-2">
                        <CardHeader>
                            <CardTitle>Sending-domain authentication</CardTitle>
                            <CardDescription>
                                Gmail, Yahoo, and Outlook reject or spam-filter mail from a domain
                                without SPF and DMARC, and one unauthenticated sender damages the
                                reputation of every mailbox in the shared warmup pool. Dakiya Mail
                                checks each sending domain daily and can stop cold sending and
                                warmup from a domain that keeps failing. The grace period is how
                                long a domain may keep failing first, so a DNS outage cannot stop
                                a customer&apos;s campaigns and the owner is warned throughout it.
                            </CardDescription>
                        </CardHeader>
                        <CardContent className="space-y-4 pt-0">
                            <div className="flex items-start justify-between gap-4">
                                <div className="space-y-0.5">
                                    <Label>Stop sending from unauthenticated domains</Label>
                                    <p className="text-xs text-muted-foreground">
                                        Off keeps the check informational: domains are still
                                        checked, shown on the mailbox, and raised by the advisor,
                                        but nothing is ever blocked.
                                    </p>
                                </div>
                                <Switch
                                    checked={form.enforceDomainAuth}
                                    onCheckedChange={(v) =>
                                        setForm({ ...form, enforceDomainAuth: v })
                                    }
                                />
                            </div>
                            <div className="md:max-w-sm">
                                <Label htmlFor="auth-grace-hours">Grace period (hours)</Label>
                                <Input
                                    id="auth-grace-hours"
                                    type="text"
                                    inputMode="numeric"
                                    autoComplete="off"
                                    value={form.authGraceHours}
                                    onChange={(e) =>
                                        setForm({ ...form, authGraceHours: e.target.value })
                                    }
                                    aria-invalid={!authGraceValid}
                                    disabled={!form.enforceDomainAuth}
                                    className="mt-1"
                                />
                                <p className="mt-1 text-xs text-muted-foreground">
                                    How long a domain must stay failing before its mailboxes stop
                                    sending. Between {AUTH_GRACE_MIN_HOURS} and{" "}
                                    {AUTH_GRACE_MAX_HOURS.toLocaleString()}.
                                </p>
                                {!authGraceValid && (
                                    <p className="mt-1 text-xs text-red-600">
                                        Enter a whole number between {AUTH_GRACE_MIN_HOURS} and{" "}
                                        {AUTH_GRACE_MAX_HOURS.toLocaleString()}.
                                    </p>
                                )}
                            </div>
                        </CardContent>
                    </Card>

                    <Card className="lg:col-span-2">
                        <CardHeader>
                            <CardTitle>AI provider</CardTitle>
                            <CardDescription>
                                The model behind the writing assistant, the dashboard agent,
                                reply drafts, contact research, automation AI steps and warmup
                                content. A provider saved here replaces the AI_* environment
                                variables as a whole (the key, endpoint and models never mix with
                                the environment&apos;s). It is read when the backend and consumer
                                start, so restart both after saving. Warmup content bank generation
                                uses the OpenAI Batch API, so it needs OpenAI; every other AI
                                feature works with any provider.
                            </CardDescription>
                        </CardHeader>
                        <CardContent className="space-y-4 pt-0">
                            <div className="md:max-w-sm">
                                <Label htmlFor="ai-provider">Provider</Label>
                                <Select value={form.ai.provider} onValueChange={pickProvider}>
                                    <SelectTrigger id="ai-provider" className="mt-1">
                                        <SelectValue />
                                    </SelectTrigger>
                                    <SelectContent>
                                        {AI_PRESETS.map((p) => (
                                            <SelectItem key={p.value} value={p.value}>
                                                {p.label}
                                            </SelectItem>
                                        ))}
                                    </SelectContent>
                                </Select>
                                {form.ai.provider === AI_ENV && (
                                    <p className="mt-1 text-xs text-muted-foreground">
                                        Nothing is saved here, so the AI_* variables decide (AI is off
                                        when they are unset). Pick a provider to save one instead.
                                    </p>
                                )}
                            </div>

                            {form.ai.provider !== AI_ENV && (
                                <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                                    {form.ai.provider !== "anthropic" && (
                                        <>
                                            <div>
                                                <Label htmlFor="ai-base-url">Base URL</Label>
                                                <Input
                                                    id="ai-base-url"
                                                    type="text"
                                                    autoComplete="off"
                                                    value={form.ai.baseUrl}
                                                    onChange={(e) => setAI({ baseUrl: e.target.value })}
                                                    className="mt-1"
                                                />
                                                <p className="mt-1 text-xs text-muted-foreground">
                                                    {form.ai.provider === "custom"
                                                        ? "Required. Any OpenAI-compatible endpoint, such as vLLM or LM Studio."
                                                        : "Filled from the provider. Change it only to reach a proxy."}
                                                </p>
                                            </div>
                                            <div>
                                                <Label htmlFor="ai-model">Model</Label>
                                                <Input
                                                    id="ai-model"
                                                    type="text"
                                                    autoComplete="off"
                                                    value={form.ai.model}
                                                    onChange={(e) => setAI({ model: e.target.value })}
                                                    className="mt-1"
                                                />
                                                <p className="mt-1 text-xs text-muted-foreground">
                                                    {aiPreset(form.ai.provider).modelHint ??
                                                        "The model id this provider serves."}
                                                </p>
                                            </div>
                                        </>
                                    )}
                                    <div className="md:col-span-2 md:max-w-sm">
                                        <Label htmlFor="ai-api-key">API key</Label>
                                        {/* new-password keeps the browser from autofilling the admin's own login here. */}
                                        <Input
                                            id="ai-api-key"
                                            type="password"
                                            autoComplete="new-password"
                                            value={form.ai.apiKey}
                                            onChange={(e) => setAI({ apiKey: e.target.value })}
                                            placeholder={
                                                server?.ai.api_key_set &&
                                                form.ai.provider === server.ai.provider &&
                                                !form.ai.clearKey
                                                    ? `Saved, ending in ${server.ai.api_key_hint || "••••"}. Leave blank to keep it`
                                                    : form.ai.provider === "ollama"
                                                      ? "Optional"
                                                      : "Paste the key"
                                            }
                                            disabled={form.ai.clearKey}
                                            className="mt-1"
                                        />
                                        <p className="mt-1 text-xs text-muted-foreground">
                                            Sealed with CREDENTIALS_ENCRYPTION_KEY and never shown again.
                                            Switching provider drops the saved key, so enter one for the new
                                            provider.
                                        </p>
                                        {server?.ai.api_key_set &&
                                            form.ai.provider === server.ai.provider &&
                                            (form.ai.clearKey ? (
                                                <p className="mt-1 text-xs text-amber-700">
                                                    The saved key is removed when you save.{" "}
                                                    <button
                                                        type="button"
                                                        className="underline"
                                                        onClick={() => setAI({ clearKey: false })}
                                                    >
                                                        Undo
                                                    </button>
                                                </p>
                                            ) : (
                                                <Button
                                                    type="button"
                                                    size="sm"
                                                    variant="ghost"
                                                    className="mt-1"
                                                    onClick={() => setAI({ clearKey: true, apiKey: "" })}
                                                >
                                                    Remove saved key
                                                </Button>
                                            ))}
                                    </div>
                                </div>
                            )}

                            {aiError && <p className="text-xs text-red-600">{aiError}</p>}

                            <div className="flex flex-wrap items-center gap-3">
                                <Button
                                    type="button"
                                    size="sm"
                                    variant="outline"
                                    onClick={() => testMut.mutate(form.ai)}
                                    disabled={testMut.isPending || (form.ai.clearKey && !form.ai.apiKey)}
                                >
                                    <Plug className="size-4" />
                                    {testMut.isPending ? "Testing..." : "Test connection"}
                                </Button>
                                {testMut.data?.ok && (
                                    <span className="flex items-center gap-1.5 text-xs text-emerald-700">
                                        <CheckCircle2 className="size-3.5" />
                                        Connected to {testMut.data.provider}
                                        {testMut.data.model ? ` (${testMut.data.model})` : ""} in{" "}
                                        {testMut.data.latency_ms} ms
                                    </span>
                                )}
                                {testMut.data && !testMut.data.ok && (
                                    <span className="flex items-start gap-1.5 text-xs text-red-600">
                                        <XCircle className="mt-0.5 size-3.5 shrink-0" />
                                        {testMut.data.error}
                                    </span>
                                )}
                            </div>
                            <p className="text-xs text-muted-foreground">
                                Tests the form as it stands, with no save or restart needed. With
                                the environment selected it tests the saved provider, or the
                                environment when none is saved.
                            </p>
                        </CardContent>
                    </Card>
                </div>
            )}

            {form && dirty && (
                <div className="mt-4 flex items-center gap-2">
                    <Button size="sm" onClick={save} disabled={saveMut.isPending}>
                        <Save className="size-4" />
                        {saveMut.isPending ? "Saving..." : "Save changes"}
                    </Button>
                    <Button
                        size="sm"
                        variant="outline"
                        onClick={() => server && setForm(toForm(server))}
                        disabled={saveMut.isPending}
                    >
                        Discard
                    </Button>
                    <span className="text-xs text-muted-foreground">
                        Saving records the change in the admin audit log.
                    </span>
                </div>
            )}
        </div>
    );
}
