import Request from "../../Request";
import type { SaveSignupForm, SignupForm } from "@/lib/api/models/app/contacts/SignupForm";

export async function listSignupForms(): Promise<{ data: SignupForm[] }> {
    return await Request<{ data: SignupForm[] }>({ method: "GET", url: "/signup-forms", authorization: true });
}

export async function saveSignupForm(id: string | undefined, data: SaveSignupForm): Promise<SignupForm> {
    return await Request<SignupForm>({ method: id ? "PATCH" : "POST", url: id ? `/signup-forms/${id}` : "/signup-forms", data, authorization: true });
}

export async function deleteSignupForm(id: string): Promise<void> {
    await Request<void>({ method: "DELETE", url: `/signup-forms/${id}`, authorization: true });
}
