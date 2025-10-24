DOCKER_TAG ?= latest

docker:
	docker build . -t ci4rail/traceletsim:$(DOCKER_TAG) && docker push ci4rail/traceletsim:$(DOCKER_TAG)

PHONY: docker