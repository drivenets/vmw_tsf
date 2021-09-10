.PHONY: all
all: hal-client halo twamp hal-stats

.PHONY: hal-client
hal-client:
	go build -o bin/hal-client cmd/client/main.go

.PHONY: hal-client-in-docker
hal-client-in-docker:
	@DOCKER_BUILDKIT=1 \
	docker build . \
		--target bin \
		--output bin/ \
		--build-arg TOOL_NAME=hal-client \
		--build-arg TOOL_MAIN=cmd/client/main.go

.PHONY: halo
halo:
	go build -o bin/halo cmd/nop/main.go

.PHONY: twamp
twamp:
	go build -o bin/twamp cmd/twamp/main.go

.PHONY:

.PHONY: proto
proto:
	cd ./pkg/hal/proto \
	&& protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		stats.proto

.PHONY: hal-stats
hal-stats:
	go build -o bin/hal-stats cmd/stats/main.go
