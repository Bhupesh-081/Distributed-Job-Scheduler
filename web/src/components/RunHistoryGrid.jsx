import { fmtDate } from "../format";

// Airflow "grid view"-style run history: one square per job, oldest to
// newest left-to-right, colored by status - a run history at a glance
// instead of just the current job's status badge. Reuses the same
// --status-* tokens the status badges/donut use, so colors stay in sync.
export default function RunHistoryGrid({ jobs, max = 30 }) {
  if (!jobs || jobs.length === 0) return <p className="muted">No runs yet.</p>;

  const recent = jobs.slice(0, max).slice().reverse();
  return (
    <div className="run-grid">
      {recent.map((j) => (
        <span
          key={j.id}
          className="run-cell"
          style={{ background: `var(--status-${j.status}, var(--border))` }}
          title={`${j.name}\n${j.status} - ${fmtDate(j.created_at)}`}
        />
      ))}
    </div>
  );
}
