export type SendingDomainProvider = "google_workspace" | "microsoft_365" | "amazon_ses" | "other";

export interface DNSRecord {
    purpose: "spf" | "dkim" | "dmarc";
    type: "TXT" | "CNAME";
    host: string;
    value: string;
    found: boolean | null;
    note?: string;
}

export interface SendingDomain {
    id: string;
    domain: string;
    provider: SendingDomainProvider;
    dkim_selectors: string[];
    state: "pending" | "passing" | "failing" | "unknown";
    spf_found: boolean;
    dkim_found: boolean;
    dmarc_found: boolean;
    dmarc_policy: string;
    check_summary: string;
    last_checked_at?: string;
    mailbox_count: number;
    records: DNSRecord[];
}

export interface SaveSendingDomain {
    domain: string;
    provider: SendingDomainProvider;
    dkim_selectors: string[];
}
