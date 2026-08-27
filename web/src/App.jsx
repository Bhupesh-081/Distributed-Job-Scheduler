import { useEffect, useState } from "react";
import * as api from "./api";
import { getSettings, applyTheme } from "./settings";
import Queues from "./components/Queues";
import QueueDetail from "./components/QueueDetail";
import RetryPolicies from "./components/RetryPolicies";
import Scripts from "./components/Scripts";
import Workers from "./components/Workers";
import Overview from "./components/Overview";
import Settings from "./components/Settings";
import AuthScreen from "./components/AuthScreen";
import EmptyState from "./components/EmptyState";
import Account from "./components/Account";
import Breadcrumbs from "./components/Breadcrumbs";
import Customization from "./components/Customization";
import JobSchedulerPage from "./components/JobSchedulerPage";
import { IconBolt, IconChart, IconFolder, IconServer, IconSettings, IconUser, IconLayers, IconClock } from "./components/Icons";

// Org -> project -> (queues | retry policies) -> queue detail (jobs / scheduled jobs / DLQ).
function OrgsBrowser() {
  const [orgs, setOrgs] = useState([]);
  const [orgName, setOrgName] = useState("");
  const [org, setOrg] = useState(null);

  const [projects, setProjects] = useState([]);
  const [projectName, setProjectName] = useState("");
  const [project, setProject] = useState(null);
  const [projectTab, setProjectTab] = useState("Queues");

  const [queues, setQueues] = useState([]);
  const [retryPolicies, setRetryPolicies] = useState([]);
  const [scripts, setScripts] = useState([]);
  const [openQueue, setOpenQueue] = useState(null);

  const [error, setError] = useState("");

  async function refreshOrgs() {
    setError("");
    try {
      setOrgs(await api.listOrganizations());
    } catch (err) {
      setError(err.message);
    }
  }

  useEffect(() => {
    refreshOrgs();
  }, []);

  async function createOrg(e) {
    e.preventDefault();
    if (!orgName.trim()) return;
    setError("");
    try {
      await api.createOrganization(orgName.trim());
      setOrgName("");
      refreshOrgs();
    } catch (err) {
      setError(err.message);
    }
  }

  async function selectOrg(o) {
    setOrg(o);
    setProject(null);
    setOpenQueue(null);
    setError("");
    try {
      setProjects(await api.listProjects(o.id));
    } catch (err) {
      setError(err.message);
    }
  }

  async function createProject(e) {
    e.preventDefault();
    if (!projectName.trim()) return;
    setError("");
    try {
      await api.createProject(org.id, projectName.trim());
      setProjectName("");
      setProjects(await api.listProjects(org.id));
    } catch (err) {
      setError(err.message);
    }
  }

  async function selectProject(p) {
    setProject(p);
    setProjectTab("Queues");
    setOpenQueue(null);
    setError("");
    try {
      const [qs, rps, scr] = await Promise.all([api.listQueues(p.id), api.listRetryPolicies(p.id), api.listScripts(p.id)]);
      setQueues(qs);
      setRetryPolicies(rps);
      setScripts(scr);
    } catch (err) {
      setError(err.message);
    }
  }

  async function refreshQueues() {
    setError("");
    try {
      setQueues(await api.listQueues(project.id));
    } catch (err) {
      setError(err.message);
    }
  }

  async function refreshRetryPolicies() {
    setError("");
    try {
      setRetryPolicies(await api.listRetryPolicies(project.id));
    } catch (err) {
      setError(err.message);
    }
  }

  async function refreshScripts() {
    setError("");
    try {
      setScripts(await api.listScripts(project.id));
    } catch (err) {
      setError(err.message);
    }
  }

  const crumbs = [{ label: "Organizations", onClick: org ? () => { setOrg(null); setProject(null); setOpenQueue(null); } : undefined }];
  if (org) crumbs.push({ label: org.name, onClick: project ? () => { setProject(null); setOpenQueue(null); } : undefined });
  if (project) crumbs.push({ label: project.name, onClick: openQueue ? () => setOpenQueue(null) : undefined });
  if (openQueue) crumbs.push({ label: openQueue.name });

  return (
    <div>
      <Breadcrumbs items={crumbs} />

      {!org && (
        <>
          <div className="content-header"><h2>Organizations</h2></div>
          <form className="card inline-form" onSubmit={createOrg}>
            <input placeholder="New organization name" value={orgName} onChange={(e) => setOrgName(e.target.value)} />
            <button className="btn-primary" type="submit">Create</button>
          </form>
          {error && <div className="error">{error}</div>}
          {orgs.length === 0 ? (
            <EmptyState
              icon={IconFolder}
              title="No organizations yet"
              description="Create one above to start adding projects, queues, and jobs."
            />
          ) : (
            <ul className="org-list">
              {orgs.map((o) => (
                <li key={o.id} className="card org-row">
                  <span className="org-name">{o.name}</span>
                  <button className="link" onClick={() => selectOrg(o)}>Open</button>
                </li>
              ))}
            </ul>
          )}
        </>
      )}

      {org && !project && (
        <>
          <div className="content-header"><h2>{org.name}</h2></div>
          <form className="card inline-form" onSubmit={createProject}>
            <input placeholder="New project name" value={projectName} onChange={(e) => setProjectName(e.target.value)} />
            <button className="btn-primary" type="submit">Create</button>
          </form>
          {error && <div className="error">{error}</div>}
          {projects.length === 0 ? (
            <EmptyState
              icon={IconFolder}
              title="No projects yet"
              description="Create one above - every project can own multiple queues."
            />
          ) : (
            <ul className="org-list">
              {projects.map((p) => (
                <li key={p.id} className="card org-row">
                  <span className="org-name">{p.name}</span>
                  <button className="link" onClick={() => selectProject(p)}>Open</button>
                </li>
              ))}
            </ul>
          )}
        </>
      )}

      {org && project && openQueue && (
        <QueueDetail
          queue={openQueue}
          retryPolicies={retryPolicies}
          scripts={scripts}
          onChanged={(updated) => {
            setOpenQueue(updated);
            refreshQueues();
          }}
        />
      )}

      {org && project && !openQueue && (
        <>
          <div className="content-header"><h2>{project.name}</h2></div>
          {error && <div className="error">{error}</div>}
          <div className="tabs">
            {["Queues", "Retry policies", "Scripts"].map((t) => (
              <button key={t} className={`tab ${projectTab === t ? "tab-active" : ""}`} onClick={() => setProjectTab(t)}>{t}</button>
            ))}
          </div>
          {projectTab === "Queues" && (
            <Queues
              projectId={project.id}
              queues={queues}
              retryPolicies={retryPolicies}
              refresh={refreshQueues}
              onOpen={setOpenQueue}
            />
          )}
          {projectTab === "Retry policies" && (
            <RetryPolicies projectId={project.id} policies={retryPolicies} refresh={refreshRetryPolicies} />
          )}
          {projectTab === "Scripts" && (
            <Scripts projectId={project.id} scripts={scripts} refresh={refreshScripts} />
          )}
        </>
      )}
    </div>
  );
}

const NAV = [
  { id: "overview", label: "Overview", icon: IconChart },
  { id: "organizations", label: "Organizations", icon: IconFolder },
  { id: "job-scheduler", label: "Job Scheduler", icon: IconClock },
  { id: "customization", label: "Customization", icon: IconLayers },
  { id: "workers", label: "Workers", icon: IconServer },
  { id: "account", label: "Account", icon: IconUser },
  { id: "settings", label: "Settings", icon: IconSettings },
];

function Dashboard({ onLogout }) {
  const [tab, setTab] = useState("overview");
  const [me, setMe] = useState(null);

  useEffect(() => {
    api.getMe().then(setMe).catch(() => {});
  }, []);

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="sidebar-brand">
          <span className="brand-mark"><IconBolt size={18} /></span>
          <span className="brand">Job Scheduler</span>
        </div>
        {me && (
          <button type="button" className="sidebar-whoami" onClick={() => setTab("account")}>
            {me.display_name || me.email}
          </button>
        )}
        <nav className="sidenav">
          {NAV.map((n) => (
            <button
              key={n.id}
              className={`sidenav-item ${tab === n.id ? "sidenav-active" : ""}`}
              onClick={() => setTab(n.id)}
            >
              <span className="sidenav-icon"><n.icon size={16} /></span>{n.label}
            </button>
          ))}
        </nav>
        <button
          type="button"
          className="link sidebar-signout"
          onClick={() => window.confirm("Sign out?") && onLogout()}
        >
          Sign out
        </button>
        <div className="sidebar-footer muted">Distributed Job Scheduler</div>
      </aside>

      <main className="content">
        {tab === "overview" && <Overview onNavigate={setTab} />}
        {tab === "organizations" && <OrgsBrowser />}
        {tab === "job-scheduler" && <JobSchedulerPage />}
        {tab === "customization" && <Customization />}
        {tab === "workers" && <Workers />}
        {tab === "account" && <Account onLogout={onLogout} />}
        {tab === "settings" && <Settings />}
      </main>
    </div>
  );
}

export default function App() {
  const [loggedIn, setLoggedIn] = useState(api.isLoggedIn());

  useEffect(() => {
    applyTheme(getSettings().theme);
  }, []);

  function handleLogout() {
    // Flip immediately (unmounts Dashboard and every child's state right
    // away) instead of waiting on the revoke call - no window where a
    // stale job/log view stays on screen while the request is in flight.
    setLoggedIn(false);
    api.logout();
  }

  return loggedIn ? (
    <Dashboard onLogout={handleLogout} />
  ) : (
    <AuthScreen onAuthed={() => setLoggedIn(true)} />
  );
}
