ROOT_DIR:=$(dir $(realpath $(lastword $(MAKEFILE_LIST))))

mediacheck: *.go
	go build .

build:
	docker run --rm -v $(ROOT_DIR):/src -v /var/run/docker.sock:/var/run/docker.sock centurylink/golang-builder thraxil/mediacheck

push: build
	docker push thraxil/mediacheck
