// GET /deliverability/postmaster (models.PostmasterReport): ratios are 0..1, null when Gmail reported none.

export type PostmasterReputation = "high" | "medium" | "low" | "bad";

export interface PostmasterDay {
    date: string;
    domain_reputation: PostmasterReputation | null;
    spam_rate: number | null;
    spf_ratio: number | null;
    dkim_ratio: number | null;
    dmarc_ratio: number | null;
}

export interface PostmasterSummary {
    days_with_data: number;
    latest_date: string | null;
    latest_reputation: PostmasterReputation | null;
    spam_rate_avg: number | null;
    spam_rate_max: number | null;
    days_over_warn: number;
    days_over_critical: number;
    spf_avg: number | null;
    dkim_avg: number | null;
    dmarc_avg: number | null;
}

export interface PostmasterDomain {
    domain: string;
    mailboxes: number;
    latest_date: string | null;
}

export interface PostmasterReport {
    connected: boolean;
    can_connect: boolean;
    last_synced_at: Date | null;
    last_error: string;
    domains: PostmasterDomain[];
    domain: string;
    days: number;
    thresholds: { spam_warn: number; spam_critical: number };
    summary: PostmasterSummary;
    data: PostmasterDay[];
}

export interface PostmasterCredentials {
    client_id: string;
    client_secret: string;
    refresh_token: string;
}
