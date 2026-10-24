// POST /emails/verify — check one address without storing it.
//
// The server runs syntax -> MX -> an SMTP RCPT probe, plus a probe of a random
// localpart on the same domain to spot a catch-all. It is a live network call
// per address, so expect seconds, not milliseconds.

import Request from "../../Request";

export type VerificationStatus = "valid" | "risky" | "invalid" | "unknown";

export interface VerifyAddressResult {
    email: string;
    status: VerificationStatus;
    reason: string;
    is_catch_all: boolean;
    has_mx: boolean;
    checked_at: string;
}

export default async function verifyAddress(email: string): Promise<VerifyAddressResult> {
    return await Request<VerifyAddressResult>({
        method: "POST",
        url: `/emails/verify`,
        data: { email },
        authorization: true,
    });
}
