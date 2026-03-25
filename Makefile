.PHONY: all
all: fmt fmt-md lint test build-examples

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

PRETTIER ?= $(LOCALBIN)/prettier
ENVTEST ?= $(LOCALBIN)/setup-envtest
GINKGO ?= $(LOCALBIN)/ginkgo
PRETTIER_VERSION ?= 3.8.1
GINKGO_VERSION ?= $(shell go list -m -f "{{ .Version }}" github.com/onsi/ginkgo/v2)

#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')

#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: ginkgo
ginkgo: $(GINKGO) ## Download ginkgo CLI locally if necessary.
$(GINKGO): $(LOCALBIN)
	$(call go-install-tool,$(GINKGO),github.com/onsi/ginkgo/v2/ginkgo,$(GINKGO_VERSION))

##@ AI Instructions

AI_BASE  := .ai/base.md
AI_REVIEW := .ai/review.md

.PHONY: ai-instructions
ai-instructions: ## Generate all AI instruction files from source templates in .ai/
	cp $(AI_BASE) CLAUDE.md
	@mkdir -p .junie
	cp $(AI_BASE) .junie/guidelines.md
	@mkdir -p .github
	cp $(AI_REVIEW) .github/copilot-review-guidelines.md
	@{ \
		cat $(AI_BASE); \
		printf '\n\n---\n\n## Code Review\n\nWhen reviewing pull requests, apply the standards in [`.github/copilot-review-guidelines.md`](copilot-review-guidelines.md).\n'; \
	} > .github/copilot-instructions.md

##@ Development

.PHONY: fmt
fmt: fmt-go fmt-md ## Run all formatting in the project


.PHONY: fmt-go
fmt-go: ## Format Go source files.
	go fmt ./...


.PHONY: fmt-md
fmt-md: prettier ## Format Markdown files.
	$(PRETTIER) --write '**/*.md' --ignore-path .gitignore

.PHONY: prettier
prettier: $(PRETTIER) ## Download prettier locally if necessary.
$(PRETTIER): $(LOCALBIN)
	@[ -f $(PRETTIER) ] || { \
		set -e ; \
		echo "Installing prettier@$(PRETTIER_VERSION)..." ; \
		npm install --prefix $(LOCALBIN)/prettier-pkg prettier@$(PRETTIER_VERSION) && \
		printf '#!/bin/sh\nexec node "$(LOCALBIN)/prettier-pkg/node_modules/.bin/prettier" "$$@"\n' > $(PRETTIER) && \
		chmod +x $(PRETTIER) ; \
	}

.PHONY: lint ## Run all linters
lint: lint-go lint-md

.PHONY: lint-md
lint-md: prettier ## Check Markdown files are formatted.
	$(PRETTIER) --check '**/*.md' --ignore-path .gitignore


.PHONY: lint-go ## Lint go files.
lint-go:
	golangci-lint run

.PHONY: test
test: setup-envtest
	go test -v $(shell go list ./... | grep -v /examples/) -coverprofile cover.out

.PHONY: build-examples
build-examples: ## Build all example binaries.
	go build ./examples/...

.PHONY: run-examples
run-examples: ## Run all examples to verify they execute without error.
	go run ./examples/deployment-primitive/.
	go run ./examples/configmap-primitive/.
	go run ./examples/custom-resource-implementation/.
	go run ./examples/service-primitive/.

##@ E2E Testing

KIND_CLUSTER_NAME ?= ocf-e2e
KIND_IMAGE ?= kindest/node:v1.31.0

.PHONY: kind-create
kind-create: ## Create a kind cluster for E2E tests.
	kind create cluster --name $(KIND_CLUSTER_NAME) --image $(KIND_IMAGE) --wait 60s

.PHONY: kind-delete
kind-delete: ## Delete the kind E2E cluster.
	kind delete cluster --name $(KIND_CLUSTER_NAME)

.PHONY: kind-set-context
kind-set-context: ## Set kubectl context to the E2E kind cluster.
	kubectl config use-context kind-$(KIND_CLUSTER_NAME)

.PHONY: e2e
e2e: ginkgo ## Run E2E tests (requires active kind cluster).
	$(GINKGO) -v --timeout 10m --tags e2e ./e2e/...

.PHONY: e2e-primitives
e2e-primitives: ginkgo ## Run primitive E2E tests only.
	$(GINKGO) -v --timeout 10m --tags e2e ./e2e/primitives/...

.PHONY: e2e-component
e2e-component: ginkgo ## Run component E2E tests only.
	$(GINKGO) -v --timeout 10m --tags e2e ./e2e/component/...

.PHONY: e2e-full
e2e-full: kind-create kind-set-context e2e kind-delete ## Full E2E lifecycle: create cluster, test, teardown.


# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef