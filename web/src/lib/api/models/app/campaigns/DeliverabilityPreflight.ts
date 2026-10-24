// Result of GET /campaigns/{id}/preflight: the pre-send deliverability gate.
// Read-only and recomputed on every call, so it always reflects the campaign now.

export type PreflightStatus = "pass" | "warn" | "fail";

export interface DeliverabilityCheck {
    key: string;
    label: string;
    status: PreflightStatus;
    detail: string;
    // Empty on a pass.
    fix_hint: string;
}

export interface DeliverabilityPreflight {
    campaign_id: string;
    verdict: PreflightStatus;
    summary: { pass: number; warn: number; fail: number };
    checks: DeliverabilityCheck[];
    generated_at: string;
}

export function failedChecks(r: DeliverabilityPreflight | undefined): DeliverabilityCheck[] {
    return (r?.checks ?? []).filter((c) => c.status === "fail");
}

// Checks the gate covers more thoroughly than the platform checks the launch
// dialog already lists, so the dialog shows each of them once.
export const COVERED_BY_DELIVERABILITY_GATE = ["unsubscribe_header", "list_quality"];
