export type SignupFormField = "first_name" | "last_name" | "company" | "phone";

export interface SignupForm {
    id: string;
    name: string;
    slug: string;
    title: string;
    description: string;
    fields: SignupFormField[];
    category_ids: string[];
    double_opt_in: boolean;
    email_account_id?: string;
    success_message: string;
    redirect_url: string;
    enabled: boolean;
    submissions: number;
    confirmations: number;
    created_at: string;
}

export interface SaveSignupForm {
    name: string;
    slug: string;
    title: string;
    description: string;
    fields: SignupFormField[];
    category_ids: string[];
    double_opt_in: boolean;
    email_account_id: string;
    success_message: string;
    redirect_url: string;
    enabled: boolean;
}
