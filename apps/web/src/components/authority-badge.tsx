export type AuthorityLabel = "CANONICAL" | "OFF-CHAIN SIGNED" | "LOCAL DERIVED" | "LOCAL ONLY" | "FINALIZED" | "SUBMITTED" | "HISTORICAL / UNCONFIRMED" | "MISSING LOCALLY" | "CONTENT-ADDRESSED" | "AVAILABLE LOCALLY";

export function AuthorityBadge({ label }: { label: AuthorityLabel }) {
  return <span className={`badge badge-${label.toLowerCase().replaceAll(" ", "-").replaceAll("/", "-")}`}>{label}</span>;
}
