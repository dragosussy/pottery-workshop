# Glaze notebook

Keeps the record of what went on a piece, how it was fired and how it came out,
so a surface that worked can be made again. Tiles are found by the number
written on them, and pots by their photo.

It's a Go backend (one binary, SQLite) with the Vue app embedded in it, at
`http://atelier/glaze/`. The code is in [apps/glaze/](../../../apps/glaze).

## Docs

- [Design](design.md): the problem, how it's used, data and API.
- [Deployment](../../deployment.md): setting up the Pi and shipping changes.

## Develop

`make dev` from the repo root runs everything behind the gateway on
`http://localhost:8000`. To work on this app alone, run these from
`apps/glaze/`:

```sh
make dev     # backend on :8081 (data in ./data), plus the Vite dev server
             # on http://localhost:5174/glaze/
make test    # Go: gofmt, vet (Mac and Pi), tests; web: types, lint, tests
```

Delete `./data` to start over. There's no hardware to simulate.
