# Security Scan Results

Date: 2026-04-13

## Scope

Reviewed the security-relevant code and deployment assets in this repository, focusing on:

- Go application code under `main.go`, `collector/`, `alertlog/`, `azvault/`, `ocivault/`, and `hashivault/`
- Container and deployment assets under `Dockerfile`, `kubernetes/`, and `docker-compose/`
- Example Java code under `docker-compose/txeventq-load/`

Excluded from direct review:

- Generated documentation under `docs/`
- Website source under `site/`, except where it would affect runtime security of the shipped exporter

## Method

- Traced configuration loading, secret retrieval, database connection setup, HTTP exposure, metrics reload, and alert log export paths.
- Reviewed file I/O, SQL execution, external service clients, and deployment defaults.
- Re-checked candidate findings for exploitability and false positives before finalizing this report.

## Findings

### 1. SQL injection through alert log state file

- Severity: Medium
- Affected code: `alertlog/alertlog.go:157-169`

The exporter reads the last processed alert-log record from a local JSON-lines file, trusts the `timestamp` field from that file, and interpolates it directly into a SQL statement with `fmt.Sprintf`:

```go
stmt := fmt.Sprintf(`select originating_timestamp, module_id, execution_context_id, message_text
    from v$diag_alert_ext
    where originating_timestamp > to_utc_timestamp_tz('%s')`, lastLogRecord.Timestamp)
rows, err := d.Session.Query(stmt)
```

If an attacker can modify or replace the alert log output file, they can inject arbitrary SQL into the next alert-log query. That turns local file-write access into database query execution with the exporter’s database privileges.

Why this is real:

- The input is not exclusively sourced from Oracle; it is re-read from the local file on every update cycle.
- `readLastMatchingLogRecord` only validates that each line is JSON, not that the timestamp is a safe timestamp literal.
- The resulting SQL is executed without bind parameters.

Recommended remediation:

- Parse the stored timestamp into a typed value before use.
- Use a bind variable instead of string interpolation.
- Treat malformed timestamps as invalid state and fall back to `defaultLastLogRecord` or fail closed.

Example fix direction:

```go
stmt := `select originating_timestamp, module_id, execution_context_id, message_text
    from v$diag_alert_ext
    where originating_timestamp > to_utc_timestamp_tz(:1)`
rows, err := d.Session.QueryContext(ctx, stmt, lastLogRecord.Timestamp)
```

### 2. Unverified Go toolchain download in the Docker build

- Severity: Low
- Affected code: `Dockerfile:19-23`

The build stage downloads a Go tarball from the network and extracts it immediately:

```dockerfile
RUN microdnf install wget gzip gcc && \
    wget -q https://go.dev/dl/go${GO_VERSION}.${GOOS}-${GOARCH}.tar.gz && \
    rm -rf /usr/local/go && \
    tar -C /usr/local -xzf go${GO_VERSION}.${GOOS}-${GOARCH}.tar.gz
```

This is a supply-chain weakness. The build does not verify a checksum or signature before executing a compiler toolchain that will build the shipped binary.

Why this is real:

- HTTPS reduces risk but does not replace artifact verification.
- A compromised mirror, CA trust path, or build environment could inject a malicious toolchain.
- The toolchain becomes part of the trusted build chain for release artifacts.

Recommended remediation:

- Verify the tarball against an expected SHA-256 checksum before extraction.
- Prefer a pinned official Go base image or another reproducible build source instead of bootstrapping Go with `wget`.

### 3. Alert log files are created with broader permissions than intended

- Severity: Low
- Affected code: `alertlog/alertlog.go:146-154`, `alertlog/alertlog.go:177-178`

When the alert log file does not exist, the exporter creates it with `os.Create`:

```go
f, e := os.Create(logDestination)
```

`os.Create` creates files with mode `0666` before the process umask is applied. In common environments with umask `022`, that yields a readable file such as `0644`. The later `os.OpenFile(..., 0600)` call does not correct the file mode because the permission argument is only used when a file is created.

Impact:

- Alert log exports can become readable by unintended local users or by other containers sharing the same mounted volume.
- The file contains database names, module IDs, ECIDs, and alert log messages, which may expose operationally sensitive information.

Recommended remediation:

- Replace the create path with a single `os.OpenFile(logDestination, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)` call.
- If the file may already exist, enforce mode `0600` with `os.Chmod` after opening or during startup validation.

## Reviewed And Excluded

The following candidates were reviewed and intentionally not counted as product vulnerabilities after the false-positive pass:

### Demo credentials in `docker-compose/`

- Files reviewed:
  - `docker-compose/compose.yaml:21-23`
  - `docker-compose/compose.yaml:34-35`
  - `docker-compose/compose.yaml:50-51`
  - `docker-compose/txeventq-load/src/main/resources/application.yaml:8-12`
- Reason excluded:
  These files are demo and local-development assets, not production defaults for the exporter binary. They are still unsafe to reuse outside local testing, but they do not indicate a vulnerability in the exporter code path itself.

### Unauthenticated metrics endpoint by default

- Files reviewed:
  - `main.go:128-202`
  - `collector/config.go:280-309`
- Reason excluded:
  The exporter uses Prometheus `exporter-toolkit` support and can be fronted with TLS/basic auth via the web config file. Exposing metrics without auth is an operational deployment choice, not a code defect on its own.

### Vault-secret parsing panic risk

- File reviewed:
  - `hashivault/hashivault.go:121-123`
- Reason excluded:
  The unchecked `val.(string)` assertion can crash on malformed Vault data, but the data source is an administrator-controlled secret backend and this is better classified as robustness hardening than a meaningful security vulnerability in the current threat model.

## Overall Assessment

Three issues survived the false-positive review:

- A real SQL injection path in alert log synchronization state handling
- A low-severity alert log file-permission disclosure issue
- A low-severity build supply-chain weakness in the Dockerfile

The SQL issue is the only finding that directly enables runtime query manipulation.
