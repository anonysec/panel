# Shell Scripts Reference

This document describes the shell scripts in the repository and their purpose.

## Active Scripts (Required)

| Script | Purpose |
|--------|---------|
| `install.sh` | Main panel installer. Sets up Go, builds panel binary, configures systemd service. |
| `node-install.sh` | Node agent installer. Downloads node binary, registers with panel, starts service. |
| `koris.sh` | CLI management tool. Start/stop/restart panel, check status, update, backup/restore. |
| `node/node.sh` | Node-side management tool. Start/stop/restart node agent, check status, update. |
| `panel/panel.sh` | Panel-side service management helper (used internally by koris.sh). |
| `deploy.sh` | CI/CD deployment script. Used by GitHub Actions for automated releases. |

## Deprecated Scripts (Not Needed)

| Script | Reason |
|--------|--------|
| `scripts/install-node.sh` | Superseded by root-level `node-install.sh` which has better validation and health checks. |
| `scripts/install-panel.sh` | Superseded by root-level `install.sh` with Go version management and connectivity checks. |
| `deploy-report.sh` | Reporting utility for legacy deploy flow. Not needed for binary distribution model. |

## Notes

- All active scripts are designed to be update-friendly (no breaking changes on re-run).
- The source is structured to be packageable as a single binary distribution later.
- `node-install.sh` includes connectivity checks, Go version detection, and health verification.
- `install.sh` validates input, generates secure tokens, and supports idempotent re-installation.
