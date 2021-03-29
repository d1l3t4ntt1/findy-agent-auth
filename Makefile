S3_TOOL_ASSET_PATH := https://$(HTTPS_PREFIX)api.github.com/repos/findy-network/findy-common-go/releases/assets/34026533


build:
	go build ./...

vet:
	go vet ./...

shadow:
	@echo Running govet
	go vet -vettool=$(GOPATH)/bin/shadow ./...
	@echo Govet success

check_fmt:
	$(eval GOFILES = $(shell find . -name '*.go'))
	@gofmt -l $(GOFILES)

lint:
	@golangci-lint run

lint_e:
	@$(GOPATH)/bin/golint ./... | grep -v export | cat

test:
	go test -v -p 1 -failfast ./...

logged_test:
	go test -v -p 1 -failfast ./... -args -logtostderr=true -v=10

test_cov:
	go test -v -p 1 -failfast -coverprofile=c.out ./... && go tool cover -html=c.out

check: check_fmt vet shadow

dclean:
	-docker rmi findy-agent-auth

dbuild:
	@[ "${HTTPS_PREFIX}" ] || ( echo "ERROR: HTTPS_PREFIX <{githubUser}:{githubToken}@> is not set"; exit 1 )
	mkdir -p .docker
	curl -L -H "Accept:application/octet-stream" "$(S3_TOOL_ASSET_PATH)" > .docker/s3-copy
	docker build \
		--build-arg HTTPS_PREFIX=$(HTTPS_PREFIX) \
		-t findy-agent-auth \
		.

drun:
	docker run -it --rm -p 8888:8888 findy-agent-auth
