
search:
	OPENOBSERVE_AUTH_TOKEN="$(OPENOBSERVE_AUTH_TOKEN)" \
		go run ./cmd/cli search minecraft mod creation

sync:
	OPENOBSERVE_AUTH_TOKEN="$(OPENOBSERVE_AUTH_TOKEN)" \
		go run ./cmd/cli sync
