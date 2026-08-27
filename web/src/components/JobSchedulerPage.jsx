import { useState } from "react";
import { useQueuePicker } from "../useQueuePicker";
import QueuePickerBar from "./QueuePickerBar";
import Jobs from "./Jobs";
import JobScheduling from "./JobScheduling";
import ScheduledJobs from "./ScheduledJobs";
import Dlq from "./Dlq";

const TABS = ["Jobs", "Batch scheduling", "Cron jobs", "Dead letter queue"];

// A flat, top-level home for everything job-lifecycle: create
// immediate/delayed/scheduled jobs and cancel them (Jobs), create several
// jobs at once under one shared schedule (Batch scheduling), manage
// recurring cron definitions (Cron jobs), and replay/discard dead-lettered
// jobs (Dead letter queue) - all via the same flat queue picker
// Customization uses, instead of the Organizations drill-down.
export default function JobSchedulerPage() {
  const picker = useQueuePicker();
  const { queue, retryPolicies, scripts, error } = picker;
  const [tab, setTab] = useState("Jobs");

  return (
    <div>
      <div className="content-header"><h2>Job Scheduler</h2></div>
      <p className="muted" style={{ marginBottom: 16 }}>
        Pick a queue to create and cancel jobs, batch-schedule several at once, manage recurring cron definitions, or
        replay/discard dead-lettered jobs.
      </p>

      <QueuePickerBar picker={picker} />

      {error && <div className="error">{error}</div>}

      {queue ? (
        <>
          <div className="tabs" style={{ marginTop: 24 }}>
            {TABS.map((t) => (
              <button key={t} className={`tab ${tab === t ? "tab-active" : ""}`} onClick={() => setTab(t)}>{t}</button>
            ))}
          </div>

          {tab === "Jobs" && <Jobs queueId={queue.id} retryPolicies={retryPolicies} scripts={scripts} />}
          {tab === "Batch scheduling" && (
            <JobScheduling queue={queue} retryPolicies={retryPolicies} scripts={scripts} />
          )}
          {tab === "Cron jobs" && (
            <ScheduledJobs queueId={queue.id} retryPolicies={retryPolicies} scripts={scripts} />
          )}
          {tab === "Dead letter queue" && <Dlq queueId={queue.id} />}
        </>
      ) : (
        <p className="muted" style={{ marginTop: 24 }}>Select a queue above to schedule or manage jobs in it.</p>
      )}
    </div>
  );
}
