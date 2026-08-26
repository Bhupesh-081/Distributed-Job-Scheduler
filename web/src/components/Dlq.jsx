import { useEffect, useState } from "react";
import * as api from "../api";
import { fmtDate } from "../format";

export default function Dlq({ queueId }) {
  const [entries, setEntries] = useState([]);
  const [error, setError] = useState("");

  async function refresh() {
    setError("");
    try {
      setEntries(await api.listDLQ(queueId));
    } catch (err) {
      setError(err.message);
    }
  }

  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [queueId]);

  async function replay(id) {
    setError("");
    try {
      await api.replayDLQEntry(id);
      refresh();
    } catch (err) {
      setError(err.message);
    }
  }

  async function discard(id) {
    setError("");
    try {
      await api.deleteDLQEntry(id);
      refresh();
    } catch (err) {
      setError(err.message);
    }
  }

  return (
    <div>
      {error && <div className="error">{error}</div>}
      <table className="table">
        <thead>
          <tr>
            <th>Job</th>
            <th>Retries</th>
            <th>Error</th>
            <th>Moved at</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {entries.map((e) => (
            <tr key={e.id}>
              <td className="mono">{e.job_id}</td>
              <td>{e.retries_count}</td>
              <td className="truncate" title={e.final_error}>{e.final_error || "-"}</td>
              <td>{fmtDate(e.moved_at)}</td>
              <td className="row-actions">
                <button className="link" onClick={() => replay(e.id)}>Replay</button>
                <button className="link danger" onClick={() => discard(e.id)}>Discard</button>
              </td>
            </tr>
          ))}
          {entries.length === 0 && (
            <tr><td colSpan={5} className="muted">No dead-lettered jobs for this queue.</td></tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
