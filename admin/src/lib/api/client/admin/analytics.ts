// /admin/analytics/*

import { Request } from "@/lib/api/client";
import type {
    AnalyticsTrends,
    DailyEmailStat,
    HourlyEmailStat,
    PlatformOverview,
    UserGrowthStat,
    WorkerLoadStat,
} from "@/lib/api/models/admin";

export function getPlatformOverview(): Promise<PlatformOverview> {
    return Request({
        method: "GET",
        url: "/admin/analytics/overview",
        authorization: true,
    });
}

export function getAnalyticsTrends(): Promise<AnalyticsTrends> {
    return Request({
        method: "GET",
        url: "/admin/analytics/trends",
        authorization: true,
    });
}

export function getDailyEmailStats(days = 30): Promise<{ data: DailyEmailStat[] }> {
    return Request({
        method: "GET",
        url: `/admin/analytics/emails/daily?days=${days}`,
        authorization: true,
    });
}

export function getHourlyEmailStats(): Promise<{ data: HourlyEmailStat[] }> {
    return Request({
        method: "GET",
        url: "/admin/analytics/emails/hourly",
        authorization: true,
    });
}

export function getWorkerLoadStats(): Promise<{ data: WorkerLoadStat[] }> {
    return Request({
        method: "GET",
        url: "/admin/analytics/workers/load",
        authorization: true,
    });
}

export function getUserGrowthStats(days = 30): Promise<{ data: UserGrowthStat[] }> {
    return Request({
        method: "GET",
        url: `/admin/analytics/users/growth?days=${days}`,
        authorization: true,
    });
}
