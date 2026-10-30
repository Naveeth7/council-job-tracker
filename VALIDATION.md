# Release validation — 28 October 2026

## Completed

- `go test ./...`: passed.
- `go vet ./...`: passed.
- `node --check site/app.js`: passed.
- Sponsor-register import review generated successfully from the supplied 143,175-row register without overwriting the council configuration.
- Live HTTP scan: Aberdeen 49 vacancy descriptions, Angus 22, Argyll and Bute 31, Bedford 10. Total **112 descriptions reviewed**, **16 broad keyword matches**. This is a point-in-time observation, not a guarantee of future coverage or vacancy availability.
- Two uncertain link associations were not enabled. Five linked platforms without a connector were explicitly marked manual check. The other 195 councils have no careers links configured.
- Public dashboard opened successfully in the browser with real scan results; job counts, council coverage, search, pagination and source links were inspected. The dashboard is a public-read-only design, not an administrator application.
- Public job text is limited to short matching excerpts; full descriptions stay in private/local state for matching. No secret values are included in the website or package.

## Not verified or activated

- Live TiDB connection and database operations: requires the owner's database secrets. Local persistence was tested.
- Email delivery: requires the owner's provider credentials and permitted sender.
- GitHub Actions execution, Pages deployment and scheduled operation: requires a selected repository and Pages/secrets configuration. The source workflow is supplied, but no GitHub deployment has been made.
- Unsupported recruitment platforms: no complete cross-council monitoring is claimed.
- Mobile browser rendering: responsive styles are included; a full device matrix was not tested.

First-found dates belong to this scan. Missing posting dates remain empty. Broad keyword matches include non-IT roles that mention IT/ICT skills or technical work, as requested.

### Additional connectors (29 October 2026)
Added deterministic scanner tests covering Jobtrain pagination, retention of non-keyword roles, missing listing pages marked partial, Hireserve feed discovery and detail fallback, WMJobs whitespace links and location arrays, and security challenge detection without retries. Smoke-tested parsing against freshly fetched Bath, Birmingham and Basingstoke adverts. Full live scans run separately; dashboard coverage remains authoritative for current success/failure counts.

### Oracle careers connector (30 October 2026)
Added tests for full-description keyword matching, retaining non-keyword vacancies, pagination, advertised-count mismatches, missing pages, explicit dates/sponsorship, wrong detail IDs and job limits. Live local and scheduled-runner checks are reported separately; current dashboard coverage is authoritative.
