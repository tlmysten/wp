# worktree-tools

`wp` registers worktree-local dev servers and points a localias domain at the active instance.

The model is:

```text
service -> instance id -> role
slush   -> feature-a   -> backend | frontend
```

## Install

```sh
go install ./cmd/wp
```

## Configure a service

```sh
wp service add slush --alias dev.slush.app --alias-role frontend
```

`--alias-role` is the default role localias should point at when you switch instances.

## Run a full stack

```sh
wp run slush/backend --id tlmysten--some-feature --port-env PORT -- pnpm -F backend dev
```

In another terminal for the same worktree:

```sh
wp run slush/frontend \
  --id tlmysten--some-feature \
  --port-env PORT \
  --env 'EXPO_PUBLIC_APPS_BACKEND_URL={{backend.url}}' \
  --switch \
  -- pnpm -F wallet dev:web
```

That produces:

```text
dev.slush.app -> frontend(tlmysten--some-feature) -> backend(tlmysten--some-feature)
```

`wp run` picks an available localhost port, sets `PORT`, registers the role, optionally switches the service alias, and unregisters the role when the child process exits.

Use a fixed port when needed:

```sh
wp run slush/frontend --id tlmysten--some-feature --port 5173 -- pnpm -F wallet dev:web
```

Start without switching immediately:

```sh
wp run slush/frontend --id tlmysten--some-feature --switch=false -- pnpm -F wallet dev:web
```

## Switch instances

```sh
wp switch slush tlmysten--some-feature
```

Use an explicit role when needed:

```sh
wp switch slush/frontend tlmysten--some-feature
```

## Inspect state

```sh
wp service list
wp list
```

By default, state is stored under the user config directory in `wp/proxy-state.json`. Set `WP_STATE_DIR` or pass `--state-dir` to use another location.

The older `wp proxy ...` commands still exist. They are wrappers around the same state and localias backend.

## Test server

The repo includes a tiny HTTP server for manual validation:

```sh
go run ./cmd/testserver -id one
```
