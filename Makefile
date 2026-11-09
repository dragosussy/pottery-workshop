# Commands for the whole repo. Each app has its own Makefile for dev and tests.

.PHONY: test

# Everything that should pass before a deploy, for every app.
test:
	$(MAKE) -C apps/humidity test
