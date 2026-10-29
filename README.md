# Council Signal

A public, read-only council vacancy dashboard with a Go checker, scheduled GitHub Actions, and optional TiDB Cloud persistence. This project is prepared for **your GitHub Pages hosting**, not hosted automatically by this download.

## What is included

- Searchable public dashboard; council and status filters; first-found/deadline sorting; CSV export.
- The existing 206-entry council sponsor list and all 11 supplied careers links.
- Broad title-and-description matching: **IT, ICT, digital, technical, analyst, web**. No job-category filter. `IT` is uppercase, whole-word to avoid the everyday pronoun "it"; the other terms are case-insensitive whole words, and analyst also matches analysts. Broad matches such as scientific technical roles are intentionally included.
- Go HTTP connectors for **MyJobScotland** and **Talentsoft**, with structured JobPosting data and a Talentsoft page-field fallback.
- Pagination and missing-description checks. If the advertised vacancy count exceeds discovered URLs, coverage is marked partial. Limits are 20 listing pages and 300 detail pages per council per run.
- robots.txt checks, request spacing, HTTP timeouts and visible failure reporting. No bypasses, logins or CAPTCHA handling.
- TiDB-backed deduplication and history; optional new-job email digest using Resend.
- Import review for future sponsor-register CSVs, preserving existing links.
- Automated Go tests and a GitHub Actions deployment workflow.

## Important current coverage limits

This is a first release, **not full coverage of all 206 councils**. Only 11 have careers links. The other 195 show "No link yet".

| Council/platform | Current connector |
| --- | --- |
| Aberdeen, Angus, Argyll and Bute | MyJobScotland HTTP connector |
| Aberdeenshire | MyJobScotland connector, but supplied link remains disabled pending identity verification |
| Bedford | Talentsoft HTTP connector |
| Ashford | Link association unverified and platform connector not implemented |
| Basildon, Bassetlaw | iTrent connector not implemented |
| Basingstoke and Deane | Connector not implemented |
| Bath and North East Somerset | Jobtrain connector not implemented |
| Birmingham | WMJobs connector not implemented; supplied URL also has filters |

Unsupported sources show "Manual check"; they are never represented as successfully searched. These platforms need separate tested connectors, possibly browser rendering or an available feed. A blocked/changed page can also prevent a supported connector from completing.

Two council/link associations were inferred from the original document order, not independently confirmed. Do not enable them until verified. Council sponsor identities and older registered names may need further review; a licence does not establish vacancy-level sponsorship.

## 1. Create the GitHub repository

1. Sign in at https://github.com/new and create a **public** repository named `council-job-tracker` (or your preferred name). GitHub Free Pages requires a public repository.
2. Unzip this package. Upload the **contents** of the `council-job-tracker` folder to the repository root, including `.github` and `go.sum`. The GitHub workflow must be at `.github/workflows/check-and-publish.yml`, not inside an extra folder. For reliable upload, use Git:

```bash
cd council-job-tracker
git init -b main
git add .
git commit -m "Add council vacancy tracker"
git remote add origin https://github.com/YOUR-USERNAME/council-job-tracker.git
git push -u origin main
```

3. Repository **Settings → Pages → Build and deployment → Source → GitHub Actions**.
4. **Actions → Check council vacancies and publish → Run workflow**. Without TiDB secrets this only publishes the bundled report; scheduled checks are skipped with a warning. Add TiDB next to enable persistent checks.

Use GitHub's returned Pages URL; do not assume publication succeeded until the deployment shows success. No custom domain is required. `.openai/hosting.json` exists only for the development preview and can be removed; GitHub Pages does not use it.

## 2. Create the free TiDB database

1. Create a **MySQL-compatible TiDB Cloud Starter** instance. Keep its monthly spending limit at **0** if you want the free-only setup. Observe quota usage; exceeding limits can stop new connections.
2. In TiDB's SQL Editor, create a database:

```sql
CREATE DATABASE IF NOT EXISTS council_tracker;
```

3. Use a database user with SELECT, INSERT, UPDATE and CREATE on `council_tracker` (CREATE is needed for initial table creation; it can be removed if you create the table separately).
4. Copy the connection fields from TiDB's Connect dialog into **GitHub repository Settings → Secrets and variables → Actions → New repository secret**:

| Secret | Value |
| --- | --- |
| `TIDB_HOST` | TiDB hostname, without a protocol or port |
| `TIDB_PORT` | Port from Connect dialog, usually `4000` |
| `TIDB_USER` | Full database username from Connect dialog |
| `TIDB_PASSWORD` | Database password |
| `TIDB_DATABASE` | `council_tracker` |

The Go driver verifies TLS certificates and hostname. The program creates one `tracker_state` JSON table row holding jobs and notified IDs. This compact snapshot approach is appropriate for the initial small personal tracker; an indexed relational job table would be a sensible later step if history grows large. Only one scheduled workflow may update it at a time; the supplied workflow concurrency group enforces this.

Never place credentials in `config/councils.csv`, source code, `site/`, or a public GitHub issue. `.env.example` contains names only and is not automatically loaded. No database credentials are sent to website visitors.

## 3. Optional email alerts

Add these GitHub Actions secrets only if you want email delivery:

- `RESEND_API_KEY`
- `EMAIL_FROM`: a sender your email provider permits, with required verification completed.
- `EMAIL_TO`: your recipient address.

No email is sent unless the key is configured. The first successful email includes all currently listed matching vacancies; later emails include newly discovered IDs. A job is marked notified only after the provider accepts the send. Failed email sends leave it pending, but do not prevent the dashboard report from being published. A crash between provider acceptance and state persistence can still result in a duplicate digest.

No digest is sent when there are no new listed matches. Failure statuses remain visible on the dashboard and workflow logs; this version does not send separate failure-only alerts.

## 4. Schedule and operating it

The workflow requests a run daily at **07:23 UTC** (07:23 UK winter time / 08:23 UK summer time). GitHub scheduled jobs can be delayed or missed, and public repository schedules can be disabled after repository inactivity. Check workflow health occasionally. The public dashboard shows the report timestamp and per-council outcomes.

- Run manually at any time with **Actions → Check council vacancies and publish → Run workflow**.
- Change the cron expression in `.github/workflows/check-and-publish.yml` to adjust the schedule.
- The site is a snapshot, not a live connection to TiDB; changes appear after a successful workflow and Pages deployment.
- Publication fails safely when TiDB persistence fails. The previous published site remains in place, so check its timestamp.
- All website content, council URLs and job descriptions in `site/jobs.json` are public. Private email settings and sent IDs remain in GitHub Secrets / TiDB.
- Job status `listed` means seen on the latest scan, not a guarantee applications are still accepted. Always read the current advert. Date-only deadlines remain current through the stated date in UTC; exact closing times are not currently modelled.

## 5. Add or update council careers links

Edit `config/councils.csv` in GitHub or Excel (save as UTF-8 CSV). Columns:

| Column | Meaning |
| --- | --- |
| `name` | Registered council name; keep stable to preserve history |
| `url` | HTTPS careers listing URL |
| `adapter` | `myjobscotland`, `talentsoft`, or `manual` |
| `confirmed` | `true` only after verifying the council/link association |
| `notes` | Internal source-review notes; not published in dashboard output |
| `routes` | Routes recorded in the sponsor register |

The next push to `main` triggers a check and publication. A new URL for an unsupported platform does not create a connector automatically.

## 6. Import a newer sponsor register

This public-dashboard option manages imports through GitHub/CSV, **not a public upload form**.

1. Upload the new public register CSV to a path such as `imports/sponsors.csv` in the repository.
2. **Actions → Review an updated sponsor register → Run workflow**. Enter that file path.
3. Download the `sponsor-register-review` artifact from the run. It contains `import-review.csv`.
4. Review `import_status`: `present`, `not_found_review`, or `review_addition`.
5. New name-based candidates may include churches and other non-local-authority organisations. Review manually; the tool intentionally never auto-enables a new candidate. Existing links are preserved.
6. Keep valid local-authority rows, remove invalid candidates and the extra `import_status` column if desired, and replace `config/councils.csv` with the reviewed file. Existing council names not found in the new register stay in the review for rename checks; do not blindly delete them.

Local alternative:

```bash
go run ./cmd/tracker import-register -file path/to/new-register.csv -out import-review.csv
```

## Local development

Install Go 1.24 or newer, then:

```bash
go test ./...
go vet ./...
go run ./cmd/tracker
```

Without `TIDB_HOST`, history is stored in ignored `.state/state.json`. That is suitable on your computer, **not ephemeral GitHub runners**; the supplied workflow deliberately skips scans if TiDB is absent to prevent duplicate alerts/history loss.

Preview the public site with an HTTP server, for example:

```bash
python3 -m http.server 8000 --directory site
```

Open http://localhost:8000. Do not open index.html directly with file://, because the JSON fetch needs HTTP.

## Tests and release checks

Tests cover keyword false matches, structured vacancy parsing, pagination deduplication, date-only closing dates, preserving jobs after failed scans, local state persistence and sponsor-register import review. Run `go test ./...` and `go vet ./...` before deploying connector changes. Live site coverage must be checked separately because recruitment sites can change or block access.

See `VALIDATION.md` for the checks completed while preparing this release.

## Grouped dashboard filters
The scanner retains all collected vacancies, including those without keyword matches. The dashboard starts with no keyword selection and shows all current openings from supported sources. Group checkboxes select/clear their children; partial selections are shown as indeterminate. Selections across both groups use OR matching. Keyword tags are computed from full titles and descriptions before public excerpts are shortened. Initial groups: IT & Technology (IT, ICT, Digital, Tech, Technical, Analyst, Web); Business & Administration (Business, Admin, Project Officer, Business Support). Keyword editing is deferred.
