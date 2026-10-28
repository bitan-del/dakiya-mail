export type SuppressionSource = "bounce" | "complaint" | "unsubscribe" | "manual" | "import" | "hashed_list" | string;
export type SuppressionFormat = "plain" | "md5" | "sha256";
export type SuppressionHashAlgo = "md5" | "sha256";

export interface SuppressedRecipient {
    id: string;
    email: string;
    reason: string;
    source: SuppressionSource;
    campaign_id?: string;
    expires_at?: string;
    created_at: string;
    updated_at: string;
}

export interface SuppressionListResult {
    data: SuppressedRecipient[];
    pagination: { total: number | null; next_cursor: string | null; has_more: boolean };
}

export interface SuppressionImportResult {
    added: number;
    skipped: number;
    invalid: number;
}

export interface SuppressionHashList {
    list_name: string;
    algo: SuppressionHashAlgo;
    count: number;
    created_at: string;
}
