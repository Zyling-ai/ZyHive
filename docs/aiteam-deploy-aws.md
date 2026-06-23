# aiteam AWS Staging Deploy Guide

> ZyHive aiteam direction runs on a dedicated AWS staging instance.
> The production `hive.lilianbot.com` deploy is unaffected.

---

## 1. Instance facts

| Property | Value |
|----------|-------|
| Region | `ap-east-1` (Hong Kong) |
| Instance ID | `i-04405815de67eda10` |
| Public IPv4 | `18.162.161.138` |
| Instance type | `t4g.small` (Graviton ARM64, 2 vCPU / 2 GiB RAM) |
| AMI | Ubuntu 22.04 LTS arm64 (`ami-02693908364da798f`) |
| SSH user | `ubuntu` (passwordless sudo) |
| Public ports | 22, 8080 |
| Disk | 29 GiB root, 1.9 GiB used at first boot |
| Binary path | `/usr/local/bin/zyhive` |
| Config path | `/etc/zyhive/zyhive.json` (mode 0600 after 26.5.10v7) |
| Service | systemd unit `zyhive.service` |

## 2. Required environment

Three values are needed by the deploy pipeline. Store them in GitHub
Secrets (or a local `.env` for hand deploys).

| Name | What | Where |
|------|------|-------|
| `AWS_ACCESS_KEY_ID` | IAM access key | GitHub Secret + `~/.aws/credentials` |
| `AWS_SECRET_ACCESS_KEY` | matching secret | GitHub Secret + `~/.aws/credentials` |
| `ZYHIVE_STAGING_TOKEN` | bearer token from `zyhive.json` `auth.token` | GitHub Secret only |

The IAM key needs `ec2-instance-connect:SendSSHPublicKey` plus
`ec2:DescribeInstances`. Nothing else.

## 3. Deploy via `scripts/deploy-aws.sh`

```bash
export AWS_ACCESS_KEY_ID=AKIA...
export AWS_SECRET_ACCESS_KEY=...
./scripts/deploy-aws.sh 26.5.10v16
```

The script does, in order:

1. `cd ui && npx vite build` — produces `ui/dist/`
2. `cp -r ui/dist cmd/aipanel/ui_dist` — keep `go:embed` in sync
3. `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags '-X main.Version=...'`
4. `ssh-keygen -t ed25519 -f ~/.ssh/zyhive_aws` if missing
5. `aws ec2-instance-connect send-ssh-public-key` — pushes the public
   key into the instance's authorized_keys for 60 seconds
6. `scp` the binary to `/tmp/zyhive-deploy.new`
7. SSH and run the swap script: backup → `mv /tmp/zyhive-deploy.new
   /usr/local/bin/zyhive` → `sudo systemctl restart zyhive`
8. `curl http://localhost:8080/api/version` self-check

Total elapsed: ~35 seconds.

## 4. Smoke test

```bash
./scripts/test/smoke-aiteam.sh \
    http://18.162.161.138:8080 \
    "$ZYHIVE_STAGING_TOKEN" \
    26.5.10v16
```

Verifies 20 assertions:

- `/api/version` reachable + correct
- `/healthz` 200 / `/readyz` 200 or 503
- `/api/aiteam/flags` available even unauthenticated → 401 (auth) is
  expected; with token → JSON list of 8 flags
- 6× gated endpoints return `404 {error:"not enabled", subsystem:...}`
- `/api/agents` 200 — proves main-line is unaffected by aiteam

## 5. GitHub Actions

`.github/workflows/deploy-staging.yml` mirrors the script with cloud
provenance:

| Trigger | Effect |
|---------|--------|
| `workflow_dispatch` (manual button) | Build + deploy + smoke |
| `push tags v*-staging` | Same, auto on tag push |

Concurrency group `aws-staging` serialises deploys so a slow workflow
never starts an overlap that would leave the binary half-swapped.

## 6. Logs & troubleshooting

```bash
# Tail systemd journal
ssh -i ~/.ssh/zyhive_aws ubuntu@18.162.161.138 \
    'sudo journalctl -u zyhive -f --since "5 min ago"'

# Inspect aiteam audit log directly (when subsystems enabled)
ssh ... 'sudo tail -f /var/lib/zyhive/agents/aiteam/audit.log'

# Roll back to previous binary if something is wrong
ssh ... 'sudo ls -lt /usr/local/bin/zyhive.bak-*'
ssh ... 'sudo cp /usr/local/bin/zyhive.bak-YYYYMMDD-HHMMSS /usr/local/bin/zyhive && sudo systemctl restart zyhive'
```

Every deploy snapshots the prior binary to
`/usr/local/bin/zyhive.bak-<timestamp>`. Manual cleanup eventually
needed if you deploy hundreds of times (~24 MiB per backup).

## 7. Enabling aiteam experimental subsystems

The systemd unit reads env from `/etc/zyhive/aiteam.env` if present.
The deploy script can upload one if you pass `ZYHIVE_FLAGS_ENV`:

```bash
cat > /tmp/aiteam.env <<EOF
ZYHIVE_EXPERIMENTAL_WALLET=1
ZYHIVE_EXPERIMENTAL_BUDGETGUARD=1
ZYHIVE_EXPERIMENTAL_JUDGE=1
ZYHIVE_EXPERIMENTAL_PAYROLL=1
ZYHIVE_EXPERIMENTAL_PROMPTDEF=1
ZYHIVE_EXPERIMENTAL_SANDBOX=1
ZYHIVE_EXPERIMENTAL_AITEAM_DASHBOARD=1

# Revenue needs the secret too
ZYHIVE_EXPERIMENTAL_REVENUE=1
ZYHIVE_AITEAM_REVENUE_SECRET=$(openssl rand -hex 32)
EOF

ZYHIVE_FLAGS_ENV=/tmp/aiteam.env ./scripts/deploy-aws.sh 26.5.10v16
```

Then update the systemd unit (one-time) so it sources the env file —
add `EnvironmentFile=/etc/zyhive/aiteam.env` under `[Service]` and
`sudo systemctl daemon-reload && sudo systemctl restart zyhive`.

## 8. Security notes

- The IAM key has no production access; do **not** reuse for the
  hive.lilianbot.com production server.
- The bearer token (`zyhive.json` `auth.token`) is rotation-eligible
  via the planned `zyhive token --rotate` CLI subcommand (P1 follow-up).
- Audit log lives at `/var/lib/zyhive/agents/aiteam/audit.log` (mode
  0600). Operators should `chown` it to root in production.

## 9. Tear-down

If staging needs to be reset:

```bash
ssh ... 'sudo systemctl stop zyhive && sudo rm -rf /var/lib/zyhive/agents/aiteam'
./scripts/deploy-aws.sh 26.5.10v6   # baseline before any aiteam state
```

Deleting only the `aiteam/` subdir resets every aiteam subsystem
without touching agents / sessions / network / memory data.

---

*deploy path · Phase 1+2 已部署至 26.5.10v24 · 19 successful deploys logged（详见 aiteam-architecture.md §8）*
