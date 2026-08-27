function splitArgs(s) {
  return (s || "").trim().split(/\s+/).filter(Boolean);
}

// Every type ultimately builds the same {cmd, args} payload the executor
// already runs via exec.CommandContext (internal/executor) - this is a
// frontend-only convenience layer, no backend change needed.
export const BUILTIN_TYPES = [
  {
    id: "python",
    label: "Python script",
    iconKey: "code",
    // Matches store.Script's script_type - lets PayloadPicker offer a
    // "Load from library" dropdown for this type's code field.
    scriptType: "python",
    fields: [
      {
        key: "code",
        label: "Python code",
        type: "code",
        placeholder: 'print("hello from the scheduler")',
        required: true,
      },
      { key: "args", label: "sys.argv arguments", placeholder: "--flag value" },
    ],
    // Passed as a single argv element via exec.CommandContext (no shell),
    // same as every other job type - `python3 -c "<code>"` needs nothing
    // new from the executor (internal/executor/executor.go).
    build: (v) => ({ cmd: "python3", args: ["-c", v.code, ...splitArgs(v.args)] }),
  },
  {
    id: "bash",
    label: "Bash script",
    iconKey: "terminal",
    scriptType: "bash",
    fields: [
      {
        key: "code",
        label: "Bash code",
        type: "code",
        placeholder: 'echo "hello from the scheduler"',
        required: true,
      },
      { key: "args", label: "Positional arguments ($1, $2, ...)", placeholder: "" },
    ],
    // "bash" as argv[0] after -c becomes $0 inside the script, so
    // splitArgs(v.args) lands as $1, $2, ... - same no-shell-interpolation
    // guarantee as every other type.
    build: (v) => ({ cmd: "bash", args: ["-c", v.code, "bash", ...splitArgs(v.args)] }),
  },
  {
    id: "shell",
    label: "Shell command",
    iconKey: "terminal",
    fields: [
      { key: "cmd", label: "Command", placeholder: "echo", required: true },
      { key: "args", label: "Arguments", placeholder: "hello world" },
    ],
    build: (v) => ({ cmd: v.cmd, args: splitArgs(v.args) }),
  },
  {
    id: "node",
    label: "Node.js script",
    iconKey: "code",
    fields: [
      { key: "script", label: "Script path", placeholder: "/scripts/task.js", required: true },
      { key: "args", label: "Arguments", placeholder: "" },
    ],
    build: (v) => ({ cmd: "node", args: [v.script, ...splitArgs(v.args)] }),
  },
  {
    id: "docker",
    label: "Docker container",
    iconKey: "box",
    fields: [
      { key: "image", label: "Image", placeholder: "alpine:latest", required: true },
      { key: "args", label: "Container command", placeholder: "echo hi" },
    ],
    build: (v) => ({ cmd: "docker", args: ["run", "--rm", v.image, ...splitArgs(v.args)] }),
  },
  {
    id: "http",
    label: "HTTP request",
    iconKey: "globe",
    fields: [
      { key: "method", label: "Method (GET/POST/...)", placeholder: "GET" },
      { key: "url", label: "URL", placeholder: "https://example.com/webhook", required: true },
    ],
    build: (v) => ({ cmd: "curl", args: ["-s", "-X", (v.method || "GET").toUpperCase(), v.url] }),
  },
  {
    id: "raw",
    label: "Custom (raw JSON)",
    iconKey: "wrench",
    fields: [],
    build: null,
  },
];

const CUSTOM_KEY = "scheduler_custom_job_types_v1";

export function loadCustomTypes() {
  try {
    return JSON.parse(localStorage.getItem(CUSTOM_KEY) || "[]");
  } catch {
    return [];
  }
}

export function saveCustomTypes(types) {
  localStorage.setItem(CUSTOM_KEY, JSON.stringify(types));
}

function customTypeToRuntime(t) {
  return {
    id: t.id,
    label: t.label,
    iconKey: "wrench",
    fields: [{ key: "args", label: t.argsHint || "Arguments", placeholder: "" }],
    build: (v) => ({ cmd: t.cmd, args: splitArgs(v.args) }),
  };
}

// Built-ins, then user-defined types from Settings, raw JSON always last as
// the escape hatch.
export function allTypes() {
  const raw = BUILTIN_TYPES.find((t) => t.id === "raw");
  const rest = BUILTIN_TYPES.filter((t) => t.id !== "raw");
  return [...rest, ...loadCustomTypes().map(customTypeToRuntime), raw];
}

// Shared by every payload-picking form (JobForm, ScheduledJobs) so the
// python-code / raw-JSON / validation logic lives in one place.
export function buildPayload(typeId, fieldValues, rawPayload) {
  const types = allTypes();
  const type = types.find((t) => t.id === typeId) || types[0];

  if (type.id === "raw") {
    try {
      return { payload: JSON.parse(rawPayload) };
    } catch {
      return { error: "Payload must be valid JSON" };
    }
  }

  for (const f of type.fields) {
    if (f.required && !(fieldValues[f.key] || "").trim()) {
      return { error: `${f.label} is required` };
    }
  }
  return { payload: type.build(fieldValues) };
}
