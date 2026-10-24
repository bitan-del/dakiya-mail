// The launch dialog's deliverability gate: a failed check must be named and
// confirmed before launching, a warning must not interrupt, and a gate that
// could not run must never leave the owner unable to launch.

import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import type Campaign from "@/lib/api/models/app/campaigns/Campaign";
import type { DeliverabilityPreflight, PreflightStatus } from "@/lib/api/models/app/campaigns/DeliverabilityPreflight";

const confirmShow = vi.fn();
let gate: { data?: DeliverabilityPreflight; isLoading: boolean; isError: boolean; refetch: () => void };

vi.mock("@/hooks/context/confirm", () => ({ useConfirm: () => ({ show: confirmShow }) }));
vi.mock("@/lib/api/hooks/app/campaigns/useCampaign", () => ({ default: () => ({ data: undefined, isLoading: false }) }));
vi.mock("@/lib/api/hooks/app/campaigns/usePreflight", () => ({ default: () => ({ data: undefined, isFetching: false }) }));
vi.mock("@/lib/api/hooks/app/campaigns/useDeliverabilityPreflight", () => ({ default: () => gate }));

const LaunchCampaignDialog = (await import("./LaunchCampaignDialog")).default;

const campaign = {
    id: "c1",
    name: "Spring outreach",
    daily_limit: 50,
    days: 31,
    open_tracking: true,
    link_tracking: true,
    sequences: [{}],
} as unknown as Campaign;

function report(...statuses: PreflightStatus[]): DeliverabilityPreflight {
    const checks = statuses.map((status, i) => ({
        key: `check_${i}`,
        label: `Check ${i}`,
        status,
        detail: `detail ${i}`,
        fix_hint: status === "pass" ? "" : `fix ${i}`,
    }));
    return {
        campaign_id: "c1",
        verdict: statuses.includes("fail") ? "fail" : statuses.includes("warn") ? "warn" : "pass",
        summary: { pass: 0, warn: 0, fail: 0 },
        checks,
        generated_at: "2026-01-01T00:00:00Z",
    };
}

function open(onConfirm = vi.fn().mockResolvedValue(undefined)) {
    render(<LaunchCampaignDialog campaign={campaign} onClose={vi.fn()} onConfirm={onConfirm} />);
    return onConfirm;
}

describe("LaunchCampaignDialog deliverability gate", () => {
    beforeEach(() => {
        confirmShow.mockReset();
        gate = { isLoading: false, isError: false, refetch: vi.fn() };
    });

    it("asks for confirmation naming the failed check, and launches only once confirmed", async () => {
        gate.data = report("pass", "fail");
        const onConfirm = open();

        fireEvent.click(screen.getByRole("button", { name: /launch anyway/i }));

        expect(onConfirm).not.toHaveBeenCalled();
        expect(confirmShow).toHaveBeenCalledTimes(1);
        const [text, proceed] = confirmShow.mock.calls[0];
        expect(text).toContain("Check 1");

        await proceed();
        await waitFor(() => expect(onConfirm).toHaveBeenCalledWith("c1"));
    });

    it("shows the fix hint of a failing check", () => {
        gate.data = report("fail");
        open();
        expect(screen.getByText("fix 0")).toBeTruthy();
    });

    it("launches straight away when there are only warnings", async () => {
        gate.data = report("pass", "warn");
        const onConfirm = open();

        fireEvent.click(screen.getByRole("button", { name: /launch now/i }));

        expect(confirmShow).not.toHaveBeenCalled();
        await waitFor(() => expect(onConfirm).toHaveBeenCalledWith("c1"));
    });

    it("explains a gate that could not run and still lets the owner launch", async () => {
        gate.isError = true;
        const onConfirm = open();
        expect(screen.getByText(/could not run the deliverability checks/i)).toBeTruthy();

        fireEvent.click(screen.getByRole("button", { name: /launch now/i }));
        await waitFor(() => expect(onConfirm).toHaveBeenCalledWith("c1"));
        expect(confirmShow).not.toHaveBeenCalled();
    });

    it("holds the launch button while the checks first run, and says why", () => {
        gate.isLoading = true;
        open();
        expect(screen.getByText(/running deliverability checks/i)).toBeTruthy();
        expect((screen.getByRole("button", { name: /checking/i }) as HTMLButtonElement).disabled).toBe(true);
    });
});
