import Request from "../../Request";
import type { Brand, SaveBrand } from "@/lib/api/models/app/emails/Brand";

export async function listBrands(): Promise<{ data: Brand[] }> {
    return await Request<{ data: Brand[] }>({ method: "GET", url: "/brands", authorization: true });
}

export async function createBrand(data: SaveBrand): Promise<Brand> {
    return await Request<Brand>({ method: "POST", url: "/brands", data, authorization: true });
}

export async function updateBrand(id: string, data: SaveBrand): Promise<Brand> {
    return await Request<Brand>({ method: "PATCH", url: `/brands/${id}`, data, authorization: true });
}

export async function setBrandMailboxes(id: string, emailAccountIds: string[]): Promise<Brand> {
    return await Request<Brand>({ method: "PUT", url: `/brands/${id}/mailboxes`, data: { email_account_ids: emailAccountIds }, authorization: true });
}

export async function deleteBrand(id: string): Promise<void> {
    await Request<void>({ method: "DELETE", url: `/brands/${id}`, authorization: true });
}
