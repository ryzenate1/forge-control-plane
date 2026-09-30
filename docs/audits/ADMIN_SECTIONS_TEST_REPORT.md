# Admin Sections Test Report

- Date: 2026-08-23T11:25:12
- Base URL: http://127.0.0.1:8080/api/v1
- Sections run: 61
- Checks: 196 passed, 0 failed

| # | Section | Passed | Failed | Verdict |
|---|---------|--------|--------|---------|
| 1 | Overview | 4 | 0 | PASS |
| 2 | Regions | 6 | 0 | PASS |
| 3 | Locations | 4 | 0 | PASS |
| 4 | Nodes | 11 | 0 | PASS |
| 5 | Allocations | 5 | 0 | PASS |
| 6 | Database Hosts | 5 | 0 | PASS |
| 7 | Mounts | 4 | 0 | PASS |
| 8 | Nests | 3 | 0 | PASS |
| 9 | Eggs | 6 | 0 | PASS |
| 10 | App Templates | 2 | 0 | PASS |
| 11 | Webhooks | 4 | 0 | PASS |
| 12 | API Keys | 3 | 0 | PASS |
| 13 | Users | 5 | 0 | PASS |
| 14 | Roles | 4 | 0 | PASS |
| 15 | OAuth Clients | 5 | 0 | PASS |
| 16 | Organizations | 4 | 0 | PASS |
| 17 | Projects | 7 | 0 | PASS |
| 18 | Environments | 7 | 0 | PASS |
| 19 | Activity | 3 | 0 | PASS |
| 20 | Audit | 1 | 0 | PASS |
| 21 | Monitoring | 4 | 0 | PASS |
| 22 | Health | 2 | 0 | PASS |
| 23 | Host | 5 | 0 | PASS |
| 24 | Docker | 4 | 0 | PASS |
| 25 | Files | 3 | 0 | PASS |
| 26 | Settings | 4 | 0 | PASS |
| 27 | Cron Jobs | 5 | 0 | PASS |
| 28 | Migrations | 2 | 0 | PASS |
| 29 | Evacuations | 1 | 0 | PASS |
| 30 | Recovery Plans | 2 | 0 | PASS |
| 31 | Reservations | 2 | 0 | PASS |
| 32 | Scheduler | 5 | 0 | PASS |
| 33 | Auto-Scaler | 2 | 0 | PASS |
| 34 | Failover | 2 | 0 | PASS |
| 35 | Firewall | 3 | 0 | PASS |
| 36 | Load Balancer | 4 | 0 | PASS |
| 37 | Ingress | 3 | 0 | PASS |
| 38 | Deployments | 2 | 0 | PASS |
| 39 | Certificates | 2 | 0 | PASS |
| 40 | Cloud | 3 | 0 | PASS |
| 41 | Compose | 1 | 0 | PASS |
| 42 | Backups | 2 | 0 | PASS |
| 43 | Database Services | 4 | 0 | PASS |
| 44 | Catalog | 2 | 0 | PASS |
| 45 | Pipelines | 2 | 0 | PASS |
| 46 | Billing | 2 | 0 | PASS |
| 47 | Endpoints | 4 | 0 | PASS |
| 48 | Git Connections | 3 | 0 | PASS |
| 49 | Source Deployments | 4 | 0 | PASS |
| 50 | Plugins | 2 | 0 | PASS |
| 51 | Notifications | 2 | 0 | PASS |
| 52 | Social Login | 1 | 0 | PASS |
| 53 | Storage Providers | 1 | 0 | PASS |
| 54 | SFTP | 2 | 0 | PASS |
| 55 | Preview Deployments | 2 | 0 | PASS |
| 56 | Reconciler | 2 | 0 | PASS |
| 57 | Reaper | 1 | 0 | PASS |
| 58 | Jobs | 2 | 0 | PASS |
| 59 | Cache | 2 | 0 | PASS |
| 60 | Cleanup | 1 | 0 | PASS |
| 61 | SSH Keys | 1 | 0 | PASS |


Overall verdict: PASS

## Known API issues

All previously identified bugs are fixed and verified. Remaining failures, if any, are environment limitations:

| Endpoint | HTTP | Cause |
|----------|------|-------|
| POST /admin/database-services | 500/504 | provision depends on the local beacon pulling the postgres image via Docker; may time out in constrained environments |
