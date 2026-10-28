export type DeliverabilityEventType =
    | "delivered"
    | "deferred"
    | "bounce"
    | "complaint"
    | "unsubscribe"
    | "open"
    | "click"
    | "reply";

export interface DeliverabilityEventRow {
    id: string;
    campaign_id?: string;
    task_id?: string;
    contact_id?: string;
    event_type: DeliverabilityEventType;
    provider: string;
    recipient_email: string;
    reason: string;
    metadata?: Record<string, unknown>;
    created_at: string;
    campaign_name?: string;
    email_account_id?: string;
    mailbox_email?: string;
}

export interface DeliverabilityEventListResult {
    data: DeliverabilityEventRow[];
    pagination: { next_cursor: string | null; has_more: boolean };
}
