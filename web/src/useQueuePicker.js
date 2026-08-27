import { useEffect, useState } from "react";
import * as api from "./api";

// Shared org -> project -> queue cascading picker (Customization,
// JobScheduling both need "pick a queue" without the full Organizations
// drill-down UI) - one fetch chain instead of two copies of it.
export function useQueuePicker() {
  const [orgs, setOrgs] = useState([]);
  const [orgId, setOrgId] = useState("");
  const [projects, setProjects] = useState([]);
  const [projectId, setProjectId] = useState("");
  const [queues, setQueues] = useState([]);
  const [queueId, setQueueId] = useState("");
  const [retryPolicies, setRetryPolicies] = useState([]);
  const [scripts, setScripts] = useState([]);
  const [error, setError] = useState("");

  useEffect(() => {
    api.listOrganizations().then(setOrgs).catch((err) => setError(err.message));
  }, []);

  useEffect(() => {
    setProjectId("");
    setProjects([]);
    setQueueId("");
    setQueues([]);
    if (!orgId) return;
    api.listProjects(orgId).then(setProjects).catch((err) => setError(err.message));
  }, [orgId]);

  useEffect(() => {
    setQueueId("");
    setQueues([]);
    if (!projectId) return;
    Promise.all([api.listQueues(projectId), api.listRetryPolicies(projectId), api.listScripts(projectId)])
      .then(([qs, rps, scr]) => {
        setQueues(qs);
        setRetryPolicies(rps);
        setScripts(scr);
      })
      .catch((err) => setError(err.message));
  }, [projectId]);

  return {
    orgs, orgId, setOrgId,
    projects, projectId, setProjectId, refreshProjects: () => api.listProjects(orgId).then(setProjects),
    queues, setQueues, queueId, setQueueId, queue: queues.find((q) => q.id === queueId) || null,
    refreshQueues: () => api.listQueues(projectId).then(setQueues),
    retryPolicies, scripts,
    error, setError,
  };
}
