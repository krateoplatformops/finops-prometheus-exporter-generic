ARCH?=amd64
REPO?=#your repository here 
VERSION?=0.1

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build -o ./bin/prometheus-exporter-generic main.go

container:
	docker build -t $(REPO)finops-prometheus-exporter-generic:$(VERSION) .
	docker push $(REPO)finops-prometheus-exporter-generic:$(VERSION)

container-multi:
	docker buildx build --tag $(REPO)finops-prometheus-exporter-generic:$(VERSION) --push --platform linux/amd64,linux/arm64 .