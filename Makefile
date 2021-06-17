.PHONY: all
all: hal-client halo twamp

.PHONY: hal-client
hal-client:
	go build -o bin/hal-client cmd/client/main.go

.PHONY: halo
halo:
	go build -o bin/halo cmd/nop/main.go

.PHONY: twamp
twamp:
	go build -o bin/twamp cmd/twamp/main.go

.PHONY: clean
clean:
	rm -f bin/*
