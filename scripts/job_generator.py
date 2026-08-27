#!/usr/bin/env python3
"""Repeatedly submits jobs to the Distributed Job Scheduler over its REST
API, so you can watch the dashboard (Jobs tab -> run-history grid, live
WebSocket status updates) fill up with a realistic, continuously changing
mix of queued/running/success/failed/dead jobs instead of a handful of
one-off test jobs.

This is a standalone client, run from your own machine - it is NOT meant
to be pasted into the dashboard's Script Library and scheduled as a job
inside the scheduler itself. Two reasons:

  1. It needs your login credentials. A job's payload is stored in
     Postgres and visible to anyone with access to that job's queue
     (GET /jobs/{id}) - embedding a real password in it is a credential
     leak waiting to happen.
  2. It loops forever (or for a long time). A job's execution is capped
     at 5 minutes (internal/executor/executor.go's maxTimeout) - the
     scheduler would just kill it partway through.

If you want this running unattended/on a schedule, use your OS's own
cron/systemd timer to run this script periodically (or with --forever
under a process supervisor) - that keeps the scheduler itself simple and
your credentials out of its database. You're welcome to also paste this
file's contents into the Script Library (Scripts tab) purely as a stored
copy/reference; just don't wire it up as a scheduled job as-is.

Usage:
    export JOB_GEN_EMAIL=you@example.com
    export JOB_GEN_PASSWORD='your password'
    python3 scripts/job_generator.py --queue-id <uuid> --count 20 --interval 2

    # or run indefinitely until Ctrl+C:
    python3 scripts/job_generator.py --queue-id <uuid> --forever

Find a queue's ID in the dashboard's Customization tab (select
Organization -> Project -> Queue - open a queue there or via the URL/API;
GET /projects/{projectId}/queues also lists them), or create one there.

No third-party dependencies - stdlib only (urllib), same as every job
this scheduler executes.
"""

import argparse
import json
import os
import random
import sys
import time
import urllib.error
import urllib.request

# A rotating mix of payloads designed to exercise the whole job lifecycle:
# quick successes, a slow-but-successful one (so you can actually see
# "running" in the live status before it flips), and one that always fails
# with a low retries_max so it dead-letters quickly for the DLQ tab.
JOB_TEMPLATES = [
    {
        "label": "quick-success",
        "payload": {"cmd": "python3", "args": ["-c", "print('job ok')"]},
        "retries_max": 3,
    },
    {
        "label": "slow-success",
        "payload": {"cmd": "bash", "args": ["-c", "sleep 3 && echo done", "bash"]},
        "retries_max": 3,
    },
    {
        "label": "always-fails",
        "payload": {"cmd": "bash", "args": ["-c", "echo boom >&2; exit 1", "bash"]},
        "retries_max": 1,
    },
]


def api_request(base_url, path, token=None, body=None, method=None):
    url = base_url.rstrip("/") + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method or ("POST" if data else "GET"))
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    try:
        with urllib.request.urlopen(req) as resp:
            raw = resp.read()
            return json.loads(raw) if raw else None
    except urllib.error.HTTPError as e:
        detail = e.read().decode(errors="replace")
        raise SystemExit(f"{method or 'POST'} {path} -> {e.code}: {detail}")


def login_or_register(api_url, email, password):
    try:
        tokens = api_request(api_url, "/auth/login", body={"email": email, "password": password})
    except SystemExit:
        print(f"login failed, registering {email} instead...", file=sys.stderr)
        tokens = api_request(api_url, "/auth/register", body={"email": email, "password": password})
    return tokens["access_token"]


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--queue-id", required=True, help="Target queue's UUID")
    parser.add_argument("--api-url", default=os.environ.get("JOB_GEN_API_URL", "http://localhost:8080"))
    parser.add_argument("--job-api-url", default=os.environ.get("JOB_GEN_JOB_API_URL", "http://localhost:8081"))
    parser.add_argument("--email", default=os.environ.get("JOB_GEN_EMAIL"))
    parser.add_argument("--password", default=os.environ.get("JOB_GEN_PASSWORD"))
    parser.add_argument("--interval", type=float, default=2.0, help="Seconds between job submissions")
    parser.add_argument("--count", type=int, default=20, help="How many jobs to submit (ignored with --forever)")
    parser.add_argument("--forever", action="store_true", help="Keep submitting until Ctrl+C")
    args = parser.parse_args()

    if not args.email or not args.password:
        parser.error("--email/--password required (or set JOB_GEN_EMAIL / JOB_GEN_PASSWORD)")

    token = login_or_register(args.api_url, args.email, args.password)
    print(f"authenticated as {args.email}")

    submitted = 0
    try:
        while args.forever or submitted < args.count:
            template = random.choice(JOB_TEMPLATES)
            submitted += 1
            job = api_request(
                args.job_api_url,
                "/jobs",
                token=token,
                body={
                    "name": f"gen-{submitted}-{template['label']}",
                    "scheduled_type": "immediate",
                    "queue_id": args.queue_id,
                    "payload": template["payload"],
                    "retries_max": template["retries_max"],
                },
            )
            print(f"[{submitted}] created {job['name']} ({job['id']}) status={job['status']}")
            time.sleep(args.interval)
    except KeyboardInterrupt:
        pass
    print(f"\nsubmitted {submitted} job(s)")


if __name__ == "__main__":
    main()
