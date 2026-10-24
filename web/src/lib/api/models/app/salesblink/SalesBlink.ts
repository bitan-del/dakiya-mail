// Settings → SalesBlink. Warmup for connected mailboxes is run by a SalesBlink
// account instead of the local pool.

export interface SalesBlinkLink {
    base_url: string;
    auto_enroll: boolean;
    warmup_per_day: number;
    max_daily_frequency: number;
    ramp_up_frequency: number;
    auto_ramp_up: boolean;
    connected_by: string | null;
    connected_at: string;
    last_synced_at: string | null;
    last_error: string;
    // The API key is never returned by the server.
}

export interface SalesBlinkStatus {
    connected: boolean;
    link?: SalesBlinkLink;
    /** Mailboxes currently warming in SalesBlink. */
    enrolled: number;
    /** Eligible but not yet enrolled. */
    eligible: number;
    /** Cannot be enrolled (OAuth, or no IMAP). */
    ineligible: number;
}

export interface SalesBlinkMailboxRow {
    email_account_id: string;
    email: string;
    provider: string;
    status: string;
    enrolled: boolean;
    eligible: boolean;
    ineligible_reason?: string;
    sender_id?: string;
    health_score?: number | null;
    enrolled_at?: string;
    last_error?: string;
}

export interface SalesBlinkSyncResult {
    enrolled: number;
    skipped: number;
    failed: number;
    errors?: string[];
}

export interface SalesBlinkSettingsInput {
    auto_enroll: boolean;
    warmup_per_day: number;
    max_daily_frequency: number;
    ramp_up_frequency: number;
    auto_ramp_up: boolean;
}

/** SalesBlink caps warmup volume per sender; mirror it so the UI can too. */
export const SALESBLINK_MIN_PER_DAY = 1;
export const SALESBLINK_MAX_PER_DAY = 50;
