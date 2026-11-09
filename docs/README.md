# Documentation

## The whole system

- [Architecture](architecture.md): how the Pi and this repo are organised, the
  rules every app follows, and what isn't built yet.
- [Deployment](deployment.md): setting up the Pi and shipping changes.

## Apps

**humidity**, the drying-shelf controller:

- [Overview and local development](apps/humidity/README.md)
- [Hardware](apps/humidity/hardware.md): enabling the interfaces, wiring the
  pump and the screen.
- [Design](apps/humidity/design.md): features, estimate maths, pump loop, data
  and API.
- [Ideas](apps/humidity/ideas.md): next steps and things deliberately left out.

**glaze**, the notebook of recipes, tests and results:

- [Overview and local development](apps/glaze/README.md)
- [Design](apps/glaze/design.md): the problem, how it's used, data and API.

## Contributing

- [AGENTS.md](../AGENTS.md): the principles for developing in this repo.

## Adding a page

System-wide pages go directly in `docs/`. Anything about one app goes in
`docs/apps/<name>/`, starting with a `README.md`, and gets a link here.
