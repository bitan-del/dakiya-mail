// /admin/instance/* : the operator's view of this deployment.
//
// The environment is authoritative, so configuration and limits are read
// only. Instance settings is the one editable document and it holds only
// keys that no environment variable owns, except the AI provider, which wins
// over AI_PROVIDER when saved.

import { Request } from "@/lib/api/client";

export type ConfigSource = "env" | "default" | "derived" | "unset";
export type RuntimeChangeable = "boot-only" | "per-request";

export interface InstanceConfigEntry {
    key: string;
    // Always empty for a sensitive key: the fingerprint is the only disclosure.
    value: string;
    source: ConfigSource;
    sensitive: boolean;
    set: boolean;
    // First 4 hex chars of SHA-256 of the value, so two services can be
    // compared (same AUTH_SECRET?) without either secret being shown.
    fingerprint: string;
    group: string;
    effect: string;
    docs: string;
    runtime_changeable: RuntimeChangeable;
}

export interface InstanceConfigResult {
    entries: InstanceConfigEntry[];
}

export function getInstanceConfig(): Promise<InstanceConfigResult> {
    return Request({
        method: "GET",
        url: "/admin/instance/config",
        authorization: true,
    });
}

export type CheckSeverity = "error" | "warning" | "info";

export interface InstanceCheck {
    id: string;
    severity: CheckSeverity;
    title: string;
    message: string;
    // Site-relative docs path, e.g. "/development/configuration/#secrets".
    docs?: string;
    // Free-text subject (a mailbox address, a worker name); empty when instance-wide.
    target?: string;
}

export interface InstanceHealthSummary {
    error: number;
    warning: number;
    info: number;
}

export interface InstanceHealthResult {
    // Only non-ok checks are returned, so an empty list means all clear.
    checks: InstanceCheck[];
    summary: InstanceHealthSummary;
}

export function getInstanceHealth(): Promise<InstanceHealthResult> {
    return Request({
        method: "GET",
        url: "/admin/instance/health",
        authorization: true,
    });
}

export interface InstanceLimitEntry {
    name: string;
    value: string;
    unit?: string;
    description?: string;
}

export interface InstanceLimitGroup {
    title: string;
    entries: InstanceLimitEntry[];
}

export interface InstanceLimitsResult {
    groups: InstanceLimitGroup[];
}

export function getInstanceLimits(): Promise<InstanceLimitsResult> {
    return Request({
        method: "GET",
        url: "/admin/instance/limits",
        authorization: true,
    });
}

export interface InstanceSettings {
    invitations: {
        links_enabled: boolean;
        ttl_hours: number;
    };
    access: {
        allow_invited_signup: boolean;
    };
    // Mailbox sync fair use. Zero is never stored: the backend clamps every
    // value into its band and resolves an absent key to the compiled default.
    sync: {
        backfill_days: number;
        backfill_messages: number;
        daily_messages_per_mailbox: number;
        daily_messages_per_org: number;
    };
    // The sending-domain authentication gate. A mailbox whose domain has been
    // failing SPF or DMARC for longer than the grace window stops sending cold
    // mail and warmup mail until the records are fixed.
    deliverability: {
        enforce_domain_auth: boolean;
        auth_grace_hours: number;
    };
    // The saved LLM provider. An empty provider defers to the AI_* environment.
    // The key is write-only: the backend returns only whether one is set and its
    // last four characters.
    ai: {
        provider: string;
        model: string;
        base_url: string;
        api_key_set: boolean;
        api_key_hint: string;
    };
}

// What a save sends for the AI section. An empty or absent api_key keeps the
// stored key; clear_api_key removes it.
export interface InstanceSettingsUpdate extends Omit<InstanceSettings, "ai"> {
    ai: {
        provider: string;
        model: string;
        base_url: string;
        api_key?: string;
        clear_api_key?: boolean;
    };
}

export interface AITestResult {
    ok: boolean;
    error?: string;
    provider?: string;
    model?: string;
    latency_ms?: number;
}

export function getInstanceSettings(): Promise<InstanceSettings> {
    return Request({
        method: "GET",
        url: "/admin/instance/settings",
        authorization: true,
    });
}

export function putInstanceSettings(body: InstanceSettingsUpdate): Promise<InstanceSettings> {
    return Request({
        method: "PUT",
        url: "/admin/instance/settings",
        data: body,
        authorization: true,
    });
}

// Makes one tiny completion against the form's provider (an empty key reuses
// the saved one for the same provider) and answers ok or the provider's error.
export function testInstanceAI(body: {
    provider: string;
    model: string;
    base_url: string;
    api_key?: string;
}): Promise<AITestResult> {
    return Request({
        method: "POST",
        url: "/admin/instance/settings/ai/test",
        data: body,
        authorization: true,
    });
}
