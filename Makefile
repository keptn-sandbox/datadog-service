build:
	go build -ldflags '-linkmode=external' -v -o datadog-service

test:
	go test -race -v ./...

docker-build:
	docker build . -t ghcr.io/vadasambar/datadog-service:dev

docker-run:
	docker run --rm -it -p 8080:8080  ghcr.io/vadasambar/datadog-service:dev

docker-push:
	docker push ghcr.io/vadasambar/datadog-service:dev