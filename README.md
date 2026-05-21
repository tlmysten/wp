# worktree-tools

`wp` registers worktree-local dev servers and points a localias domain at the active instance.

## Install

```sh
go install -buildvcs=false ./cmd/wp
```

## Configure a service

```sh
wp proxy service add slush --alias dev.slush.app
```

## Run a worktree instance

```sh
wp proxy run --service slush --id tlmysten--some-feature --port-env PORT -- pnpm dev
```

`wp proxy run` picks an available localhost port, sets `PORT`, registers the instance, switches the service alias to it, and unregisters the instance when the child process exits.

Use a fixed port when needed:

```sh
wp proxy run --service slush --id tlmysten--some-feature --port 5173 -- pnpm dev
```

Start without switching immediately:

```sh
wp proxy run --service slush --id tlmysten--some-feature --switch=false -- pnpm dev
```

## Switch instances

```sh
wp proxy switch --service slush --id tlmysten--some-feature
```

## Inspect state

```sh
wp proxy service list
wp proxy list
```

By default, state is stored under the user config directory in `wp/proxy-state.json`. Set `WP_STATE_DIR` or pass `--state-dir` to use another location.

## Test server

The repo includes a tiny HTTP server for manual validation:

```sh
go run ./cmd/testserver -id one
```
