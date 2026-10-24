// Per-mailbox warmup analytics from GET /analytics/warmup?email_id=&from=&to=
// (backend models.WarmupAnalytics).

export interface WarmupSummary {
    total_sent: number;
    total_replied: number;
    average_daily: number;
    reply_rate: number; // percentage
    target_progress: number; // percentage to max
    days_active: number;
}

export interface WarmupDailyStat {
    date: string; // YYYY-MM-DD
    emails_sent: number;
    emails_replied: number;
    target_volume: number;
}

export default interface WarmupAnalytics {
    email_account_id: string;
    email: string;
    date_range: { from: string; to: string };
    summary: WarmupSummary;
    daily_stats: WarmupDailyStat[];
}
