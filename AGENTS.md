# AGENTS.md

How AI agents work in this repo. Read it before changing anything.

`atelier` is a set of small apps for a pottery studio, running on a Raspberry
Pi 5 and used over Tailscale. One human develops it and owns every decision.
Start with [docs/README.md](docs/README.md).

## Guiding principles

### 1. Simplicity comes first

- Build the smallest thing that solves a problem someone actually has.
- Don't add features, options, abstractions or layers for a future that may
  not come. Three similar lines are better than an early abstraction.
- Every new dependency, service and config setting has to earn its place.
- Move code into shared packages only when a second app really needs it.
- When a change makes something unnecessary, remove it.

### 2. No compromises on readability and good practices

Simple never means careless.

- Write code the next reader understands first time: clear names, small
  focused functions, and comments that explain why rather than what.
- Handle errors, validate input at the boundaries, and fail safely. Anything
  that drives hardware must end up in a safe state when something goes wrong.
- Test the behaviour that matters, and follow the patterns already in the app
  you are working on.
- Leave formatting, linting and types clean. `make test` must pass.
- Keep secrets out of the repo.

### 3. Think through the implications of every change

Before adding or changing a feature, work out what it touches:

- **Data:** does it change a schema or files that already exist on the Pi?
  How do they migrate, and what about backups?
- **Hardware:** can it affect the pump, sensors or screen? What happens on a
  crash, restart or power loss?
- **Other apps:** does it change the [rules every app follows](docs/architecture.md#rules-every-app-follows),
  the gateway, or anything shared?
- **Use:** does it still work on a phone, in the studio, on the tailnet?
- **Deploy:** does it need a manual step on the Pi? Can it be undone?
- **Docs:** which pages stop being true?

Tell the human about the implications that matter, before the work if they
could change the decision.

### 4. Validate unknowns with the human developer

- Check what you can yourself by reading the code and running the tests.
- Ask about what you can't: requirements, studio practice, the wiring, the
  state of the Pi, and any choice with real trade-offs.
- Don't guess and carry on. Ask a short, specific question and recommend an
  answer. Say which assumptions you made.
- Report results honestly, including what failed and what you couldn't check.

### 5. Agents don't push changes. The human does

- Never run `git push`, `docker push` or `./deploy.sh`.
- Never change the Pi, the GitHub repository or the Docker Hub images.
- Leave finished work ready for the human to review and push, and say what
  you verified.

## Working in this repo

| What | Where |
|---|---|
| Apps | `apps/<name>/` |
| Documentation, for every app | `docs/`, with each app in `docs/apps/<name>/` |
| What runs on the Pi | `docker-compose.yml` |
| Everything for development, at `http://localhost:8000` | `make dev` |
| One app on its own | `make -C apps/<name> dev` |
| All checks | `make test` |

- Docs live at the root in `docs/`, never inside an app.
- Each app has its own design doc. Read it before working on that app.

## Testing

Validate as much as makes sense before calling something done.

- A change in behaviour comes with tests for the normal path, the failures
  people can really hit (bad input, a conflict, a sensor or app not
  answering), and anything that could quietly damage data or misdrive
  hardware.
- Don't test, or write code for, edge cases that will never happen in this
  studio. If you can't describe how it would come about, leave it out.
- Test where it can break. Logic gets unit tests and the API gets the app's
  HTTP test. Unit tests can't see URLs, the gateway, Docker builds or the
  browser, so when a change crosses one of those, run the real thing with
  simulated hardware and check it.
- When people will see a change, open it and use it at phone width.
  `make test` passing doesn't show that it works.
- Say what you tested and how. Name what you couldn't test, such as anything
  that only exists on the Pi, and give the human the exact steps to check it.

## The dev environment

Starting an app for development has to stay simple, and it has to be testable
from more than the machine running it.

- One command starts everything: `make dev`, from a fresh clone. If setup
  needs another step, make the command do it rather than documenting it.
- Development needs Go, Node and Docker, and no Pi, hardware, accounts or
  secrets. Hardware and outside services are simulated or optional.
- Phones and other computers on the tailnet must be able to use the dev
  server. Keep it listening on all interfaces and accepting tailnet names, and
  don't hard-code `localhost` into anything the browser uses.
- The studio uses plain HTTP, in development and on the Pi. Check that a
  browser feature works without HTTPS before relying on it.
- Development data stays in the app's own git-ignored `./data`, and deleting
  it starts the app over.
- Before finishing a change, check that it hasn't made any of this harder. A
  new app has to meet all of it from the start.

## Keeping the docs true

Stale docs are worse than none, so they change along with the code.

- When a change makes a page wrong or leaves out something a reader needs,
  update that page in the same change. Don't wait to be asked.
- Docs stay high level: what the system does, how it is organised, why it was
  decided that way, and how to run it.
- Docs are not a changelog. Don't record individual changes, fixes or
  refactors there. Git history does that.
- Mark anything not built yet as planned.
