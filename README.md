# worktree-tools

`wp` registers worktree-local dev servers and points one stable endpoint at the active instance.

## What This Is

`wp` is a small local development router for multi-worktree projects. Each named service has one stable public endpoint, while each worktree can run its own private random-port process. `wp run` starts a process, records the chosen port under an instance id, and optionally switches the service to that instance. `wp serve` handles stable localhost port forwarding; localias-backed services are switched through the localias CLI.

This is intentionally not a process supervisor. It stores lightweight local state, starts commands when asked, removes registrations when those commands exit, and uses live checks for things like whether `wp serve` is listening.

The model is deliberately small:

```text
service -> instance id -> localhost port
```

A service has exactly one public endpoint:

- `--alias dev.slush.app` uses localias.
- `--listen 3003` uses `wp`'s built-in reverse proxy.

## Install

```sh
go install ./cmd/wp
```

## Configure Services

```sh
wp service add slush-web --alias dev.slush.app
wp service add slush-backend --listen 3003
```

## Reverse Proxy Port Mapping

Run the stable backend port in one terminal:

```sh
wp serve slush-backend
```

Run a backend instance from any worktree:

```sh
wp run slush-backend \
  --id tlmysten--some-feature \
  --port-env PORT \
  --extra-port prometheus:PROMETHEUS_PORT \
  --switch \
  -- pnpm -F backend dev
```

Now `http://localhost:3003` proxies to the active `slush-backend` instance. Switch later with:

```sh
wp switch slush-backend tlmysten--some-feature
```

## Localias Mapping

Run a frontend instance and point the localias domain at it:

```sh
wp run slush-web \
  --id tlmysten--some-feature \
  --port-env PORT \
  --env 'EXPO_PUBLIC_APPS_BACKEND_URL=http://localhost:3003' \
  --switch \
  -- sh -c 'EXPO_PUBLIC_WEBSITE=true pnpm -F wallet exec expo start --web --port "$PORT"'
```

Switch later with:

```sh
wp switch slush-web tlmysten--some-feature
```

## Environment Templates

`--env` can reference registered services:

```sh
wp run slush-web \
  --env 'EXPO_PUBLIC_APPS_BACKEND_URL={{slush-backend.url}}' \
  --env 'PROMETHEUS_URL={{slush-backend.prometheus.url}}' \
  -- your-command
```

For another service, `wp` prefers the instance with the same `--id`; if it is not registered, it uses that service's active instance.

## Inspect State

```sh
wp service list
wp list
wp serve status
wp current slush-backend
wp prune
wp doctor
wp unregister slush-backend tlmysten--some-feature
```

Useful workflow helpers:

- `wp list` shows configured services, whether the built-in `wp serve` port is listening, the active instance, and any registered instances.
- `wp serve status [service]` live-checks whether the fixed local port is listening.
- `wp current <service>` prints the active target URL for scripts.
- `wp switch --id <instance-id>` switches every service that has that instance id.
- `wp prune [service]` removes stale registrations whose process is gone.
- `wp doctor` checks state, localias availability, `wp serve` ports, and active targets.
- `wp service rename <old> <new>` renames a configured service.

By default, state is stored under the user config directory in `wp/proxy-state.json`. Set `WP_STATE_DIR` or pass `--state-dir` to use another location.

## Test Server

The repo includes a tiny HTTP server for manual validation:

```sh
go run ./cmd/testserver -id one
```
