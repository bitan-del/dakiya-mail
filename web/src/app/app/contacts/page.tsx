import { useSearchParams } from "react-router-dom";
import ContactsTable from "@/components/app/contacts/ContactsTable";
import AdvisorSummaryBar from "@/components/app/advisor/AdvisorSummaryBar";
import { NoAccess } from "@/components/layout/NoAccess";
import { usePermission } from "@/hooks/usePermission";
import { useSegment } from "@/lib/api/hooks/app/segments/useSegments";

export default function ContactsPage() {
    const canView = usePermission("VIEW_CONTACTS");
    const [params] = useSearchParams();
    const segmentId = params.get("segment");
    const segment = useSegment(segmentId);
    if (!canView) return <NoAccess feature="contacts" permissionLabel="View contacts" />;
    // Wait for the segment so the table starts on its filters instead of flashing everyone.
    if (segmentId && segment.isPending) return null;
    return (
        <>
            {/* List-hygiene findings live where the list does. */}
            <AdvisorSummaryBar surface="contacts" noun="list" nounPlural="lists" className="mx-5 mt-3" />
            <ContactsTable
                key={segment.data?.id ?? "all"}
                initialFilter={segment.data?.filter}
                segment={segment.data}
            />
        </>
    );
}
