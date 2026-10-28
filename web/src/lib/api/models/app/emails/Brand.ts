export interface Brand {
    id: string;
    name: string;
    website: string;
    physical_address: string;
    footer_text: string;
    footer_enabled: boolean;
    unsubscribe_link: boolean;
    mailbox_ids: string[];
    created_at: string;
    updated_at: string;
}

export interface SaveBrand {
    name: string;
    website: string;
    physical_address: string;
    footer_text: string;
    footer_enabled: boolean;
    unsubscribe_link: boolean;
}
