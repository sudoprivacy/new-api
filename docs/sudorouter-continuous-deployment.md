<!-- sudoapi: Guarded single-VM SudoRouter continuous deployment. -->
# SudoRouter production deployment

The production branch is `sudo/rc24`. Successful CI on its exact current head
triggers an immutable ACR image build and the installed Ladder release worker
on `sudorouter_jp`. The rollout is serialized and continues rollback locally if
the Actions connection disappears. The temporary slave runs the **new image**.
An authenticated, eight-output-token Sonnet request must pass directly against
that candidate before Caddy sends public traffic to it. The primary then drains,
updates and passes its own business probe before Caddy switches back and the
temporary slave drains and is removed. Datastores are not recreated.

This keeps one permanent application replica. It does not provide host/database
failover or unlimited stream lifetime. Review schema compatibility for old/new
overlap and rollback before merging migration changes. Never automatically
restore a database to roll back an application image.

## Controls and credentials

- `SUDOROUTER_DEPLOY_ENABLED=true` enables automatic/manual production dispatch.
  Keep false while installing or accepting the first rollout.
- `SUDOROUTER_RELEASE_BUNDLE` pins the canonical manifest hash of the reviewed
  `/opt/sudorouter-deploy` bundle from Ladder. Changing Python files requires
  installing the reviewed bundle and updating this variable together.
- Azure client/tenant/subscription IDs are repository variables. GitHub OIDC
  authenticates a dedicated identity for the `production` environment, restricted
  to `sudo/rc24`. Its role can read and invoke commands on this one existing VM;
  it cannot create VMs or administer the subscription. Run Command executes as
  root, so production branch and environment writers are trusted operators.
- Existing ACR build credentials stay in Actions secrets. The dedicated gateway
  probe key stays in password-agent (**SudoRouter - production deployment probe**)
  and `/etc/sudorouter-deploy/probe.json`, mode 0600. It allows one Sonnet model,
  Anthropic only, finite quota and an expiry. A depleted/expired key aborts before
  cutover. No scheduled paid inference is required.
- The dispatcher requires fresh off-host PostgreSQL protection, spare memory and
  disk. It preserves source/configuration checks and Caddy ETag compare-and-set.

The canonical operational code is in Ladder's `deploy/rollout_compose.py`,
`deploy/rollout_caddy_bridge.py` and `deploy/start_sudorouter_release.py`.
Private release logs, Compose copies and recovery dumps live under
`/data/new-api/releases/gha-<run>-<attempt>`. Do not paste them into CI output.
The safe terminal receipt includes cleanup and availability observations;
Azure command success alone does not establish release success.

## Billing protection and remaining limits

Production deployments explicitly disable `BATCH_UPDATE_ENABLED`. Quota changes
then use database writes rather than the five-second process-local batch.
Failed-request refunds complete in the HTTP handler, allowing normal HTTP drain
to wait for them. These changes reduce specific shutdown/loss windows and keep
existing request-local refund guards. They do not make every wallet/token write
one atomic ledger transaction. A forced kill during an upstream request or an
ambiguous database commit still needs request/usage reconciliation; never blindly
retry a non-idempotent wallet credit. Existing pre-upgrade processes retain their
old behavior until drained. Continuous PostgreSQL backup protects committed data.

The future permanent HA design and its costs remain in
[the staged reliability plan](sudorouter-reliability-plan.md).
