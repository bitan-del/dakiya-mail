import type BulkEditContactsField from "./BulkEditContactsField";

export default interface BulkEditContacts {
    contacts: string[];

    add_campaigns: string[];
    remove_campaigns: string[];
    add_categories?: string[];
    remove_categories?: string[];
    fields: BulkEditContactsField[];
    subscribe?: boolean;
}
