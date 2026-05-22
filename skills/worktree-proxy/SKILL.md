---
name: worktree-proxy
description: Use when working in a local multi-worktree development repo that has the `wp` worktree proxy CLI available, especially when an agent needs to run a dev server without hardcoding ports or needs to point a stable local endpoint at the active worktree instance.
---

# Worktree Proxy

Use `wp` when local dev servers may conflict on fixed ports across multiple worktrees.

At a high level:

- A `wp` service is one stable endpoint.
- `wp run` starts a command on an available port, records it under an instance id, and can switch the service to that instance.
- `wp serve` runs the built-in reverse proxy for services that use fixed localhost ports.
- Alias-backed services are switched through localias.

Before choosing commands, inspect the local setup:

```sh
wp --help
wp service list
wp list
wp serve status
```

Prefer the repo's existing service names and scripts. Do not assume fixed ports are free; let `wp run` allocate them unless the repo instructions say otherwise.
