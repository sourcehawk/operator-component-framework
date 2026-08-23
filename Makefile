.PHONY: all
all: fmt lint test test-scaffold test-examples build-examples

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
ai-instructions: ai-instructions-gen fmt-md

.PHONY: ai-instructions-gen
ai-instructions-gen: ## Generate all AI instruction files from source templates in .ai/
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

PLUGIN_SKILLS := plugin/skills

.PHONY: sync-plugin
sync-plugin: ## Sync framework docs into the Claude plugin skill references.
	rm -rf $(PLUGIN_SKILLS)/building-components/references \
		$(PLUGIN_SKILLS)/using-primitives/references \
		$(PLUGIN_SKILLS)/custom-resource-wrappers/references \
		$(PLUGIN_SKILLS)/structuring-operators/references \
		$(PLUGIN_SKILLS)/testing-operators/references
	mkdir -p $(PLUGIN_SKILLS)/building-components/references \
		$(PLUGIN_SKILLS)/using-primitives/references/primitives \
		$(PLUGIN_SKILLS)/custom-resource-wrappers/references \
		$(PLUGIN_SKILLS)/structuring-operators/references \
		$(PLUGIN_SKILLS)/testing-operators/references
	cp docs/component.md $(PLUGIN_SKILLS)/building-components/references/component.md
	cp docs/primitives.md $(PLUGIN_SKILLS)/using-primitives/references/primitives.md
	cp docs/primitives/*.md $(PLUGIN_SKILLS)/using-primitives/references/primitives/
	cp docs/custom-resource.md $(PLUGIN_SKILLS)/custom-resource-wrappers/references/custom-resource.md
	cp docs/guidelines.md $(PLUGIN_SKILLS)/structuring-operators/references/guidelines.md
	cp docs/compatibility.md $(PLUGIN_SKILLS)/structuring-operators/references/compatibility.md
	cp docs/testing.md $(PLUGIN_SKILLS)/testing-operators/references/testing.md

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

.PHONY: docs-serve
docs-serve: ## Serve the documentation site locally with live reload.
	mkdocs serve

.PHONY: docs-build
docs-build: ## Build the documentation site in strict mode.
	mkdocs build --strict

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

.PHONY: test-scaffold
test-scaffold: ## Scaffold every wrapper variant into a temp module and run its tests.
	go test -tags scaffold -count=1 -run TestScaffoldedWrappers ./internal/scaffold/...

.PHONY: build-examples
build-examples: ## Build all example binaries.
	go build ./examples/...

.PHONY: test-examples
test-examples: ## Run example tests (golden files, mutation unit tests).
	go test ./examples/...

.PHONY: run-examples
run-examples: ## Run all examples to verify they execute without error.
	go run ./examples/mutations-and-gating/.
	go run ./examples/extraction-and-guards/.
	go run ./examples/component-prerequisites/.
	go run ./examples/custom-resource/.
	go run ./examples/grace-inconsistency/.

##@ E2E Testing

KIND_CLUSTER_NAME ?= ocf-e2e
KIND_IMAGE ?= kindest/node:v1.36.1@sha256:3489c7674813ba5d8b1a9977baea8a6e553784dab7b84759d1014dbd78f7ebd5

.PHONY: kind-create
kind-create: ## Create a kind cluster for E2E tests (skips if it already exists).
	@if kind get clusters 2>/dev/null | grep -q '^$(KIND_CLUSTER_NAME)$$'; then \
		echo "Kind cluster '$(KIND_CLUSTER_NAME)' already exists, skipping creation."; \
	else \
		kind create cluster --name $(KIND_CLUSTER_NAME) --image $(KIND_IMAGE) --wait 60s; \
	fi

.PHONY: kind-delete
kind-delete: ## Delete the kind E2E cluster.
	kind delete cluster --name $(KIND_CLUSTER_NAME)

.PHONY: kind-set-context
kind-set-context: ## Set kubectl context to the E2E kind cluster.
	kubectl config use-context kind-$(KIND_CLUSTER_NAME)

.PHONY: e2e
e2e: ginkgo kind-create kind-set-context ## Run all E2E tests (creates kind cluster if needed).
	$(GINKGO) -v --timeout 20m --tags e2e ./e2e/...

PRIMITIVE ?=

.PHONY: e2e-primitives
e2e-primitives: ginkgo kind-create kind-set-context ## Run primitive E2E tests only. Use PRIMITIVE=<name> to filter.
	$(GINKGO) -v --timeout 20m --tags e2e $(if $(PRIMITIVE),--label-filter "$(PRIMITIVE)") ./e2e/primitives/...

.PHONY: e2e-component
e2e-component: ginkgo kind-create kind-set-context ## Run component E2E tests only.
	$(GINKGO) -v --timeout 15m --tags e2e ./e2e/component/...

.PHONY: e2e-full
e2e-full: kind-create kind-set-context e2e kind-delete ## Full E2E lifecycle: create cluster, test, teardown.

##@ Observability

OBS_DIR := observability
# Render output directory. The dev stack overrides it to keep its render apart.
OBS_OUT ?= $(OBS_DIR)/generated
# Prometheus metric namespace the condition gauge was created with
# (ocm.NewOperatorConditionsGauge("<namespace>")). Required for rendering.
METRIC_NAMESPACE ?= unset
# Label carrying the namespace of the custom resource. The pod scraping the
# operator usually owns `namespace`, so the exported label arrives as
# `exported_namespace`; override with NAMESPACE_LABEL=namespace if yours does not.
NAMESPACE_LABEL ?= exported_namespace
# Shape of the rendered alert files: prometheusrule (one PrometheusRule object
# per rule file) or rules (plain files for prometheus' rule_files).
ALERT_FORMAT ?= prometheusrule
# Optional metadata for the PrometheusRule objects: the namespace to create them
# in, and comma-separated key=value labels, for example the release label a
# kube-prometheus-stack ruleSelector matches on (PROMETHEUSRULE_LABELS=release=kps).
PROMETHEUSRULE_NAMESPACE ?=
PROMETHEUSRULE_LABELS ?=
# Metric namespace the alert unit tests are written against.
ALERT_TEST_NAMESPACE := test_operator

# Fail unless METRIC_NAMESPACE was given. $(1) is the target name for the hint.
define require_metric_namespace
@[ "$(METRIC_NAMESPACE)" != "unset" ] && [ -n "$(METRIC_NAMESPACE)" ] || { \
	echo "Error: METRIC_NAMESPACE is required."; \
	echo "Usage: make $(1) METRIC_NAMESPACE=my_operator"; \
	exit 1; \
}
endef

# Render a template to stdout. $(1) template path, $(2) metric namespace,
# $(3) namespace label.
define render_template
sed -e 's/{{operator_namespace}}/$(2)_/g' -e 's/{{namespace_label}}/$(3)/g' $(1)
endef

.PHONY: dashboards
dashboards: ## Render the Grafana dashboards for METRIC_NAMESPACE into observability/generated/dashboards.
	$(call require_metric_namespace,dashboards)
	@echo "Rendering dashboards for $(METRIC_NAMESPACE) (namespace label: $(NAMESPACE_LABEL))..."
	@mkdir -p $(OBS_OUT)/dashboards
	@for file in $(OBS_DIR)/dashboards/*.tpl.json; do \
	  [ -e "$$file" ] || continue; \
	  name=$$(basename "$$file" .tpl.json); \
	  $(call render_template,"$$file",$(METRIC_NAMESPACE),$(NAMESPACE_LABEL)) > "$(OBS_OUT)/dashboards/$$name.json"; \
	done

.PHONY: alerts
alerts: ## Render the Prometheus alert rules for METRIC_NAMESPACE into observability/generated/alerts.
	$(call require_metric_namespace,alerts)
	@echo "Rendering alerts for $(METRIC_NAMESPACE) (namespace label: $(NAMESPACE_LABEL), format: $(ALERT_FORMAT))..."
	@mkdir -p $(OBS_OUT)/alerts
	@for file in $(OBS_DIR)/alerts/*.yaml; do \
	  [ -e "$$file" ] || continue; \
	  case "$$file" in \
	    *.tpl.yaml) \
	      name=$$(basename "$$file" .tpl.yaml); \
	      rule_name="$(METRIC_NAMESPACE)-$$name" ;; \
	    *) \
	      name=$$(basename "$$file" .yaml); \
	      rule_name="ocf-$$name" ;; \
	  esac; \
	  rule_name=$$(echo "$$rule_name" | tr '[:upper:]' '[:lower:]' | tr '_:' '--'); \
	  out="$(OBS_OUT)/alerts/$$name.yaml"; \
	  case "$(ALERT_FORMAT)" in \
	    rules) \
	      $(call render_template,"$$file",$(METRIC_NAMESPACE),$(NAMESPACE_LABEL)) > "$$out" ;; \
	    prometheusrule) \
	      { \
	        echo "apiVersion: monitoring.coreos.com/v1"; \
	        echo "kind: PrometheusRule"; \
	        echo "metadata:"; \
	        echo "  name: $$rule_name"; \
	        [ -z "$(PROMETHEUSRULE_NAMESPACE)" ] || echo "  namespace: $(PROMETHEUSRULE_NAMESPACE)"; \
	        if [ -n "$(PROMETHEUSRULE_LABELS)" ]; then \
	          echo "  labels:"; \
	          for kv in $$(echo "$(PROMETHEUSRULE_LABELS)" | tr ',' ' '); do \
	            echo "    $${kv%%=*}: \"$${kv#*=}\""; \
	          done; \
	        fi; \
	        echo "spec:"; \
	        $(call render_template,"$$file",$(METRIC_NAMESPACE),$(NAMESPACE_LABEL)) \
	          | sed -e 's/^/  /' -e 's/[[:space:]]*$$//'; \
	      } > "$$out" ;; \
	    *) \
	      echo "Error: ALERT_FORMAT must be prometheusrule or rules, got '$(ALERT_FORMAT)'."; exit 1 ;; \
	  esac; \
	done

.PHONY: test-alerts
test-alerts: ## Lint and unit test the alert rules with promtool.
	@command -v promtool >/dev/null 2>&1 || { \
		echo "Error: promtool is required to test the alert rules."; \
		echo "It ships with prometheus: https://prometheus.io/download/"; \
		exit 1; \
	}
	@set -e; \
	tmpdir=$$(mktemp -d "$${TMPDIR:-/tmp}/ocf-alerts.XXXXXX"); \
	trap 'rm -rf "$$tmpdir"' EXIT; \
	mkdir -p "$$tmpdir/tests" "$$tmpdir/namespace-label"; \
	for file in $(OBS_DIR)/alerts/*.yaml; do \
	  [ -e "$$file" ] || continue; \
	  case "$$file" in \
	    *.tpl.yaml) \
	      name=$$(basename "$$file" .tpl.yaml); \
	      $(call render_template,"$$file",$(ALERT_TEST_NAMESPACE),exported_namespace) > "$$tmpdir/$$name.yaml"; \
	      $(call render_template,"$$file",$(ALERT_TEST_NAMESPACE),namespace) > "$$tmpdir/namespace-label/$$name.yaml" ;; \
	    *) \
	      cp "$$file" "$$tmpdir/"; \
	      cp "$$file" "$$tmpdir/namespace-label/" ;; \
	  esac; \
	done; \
	cp $(OBS_DIR)/alerts/tests/*.yaml "$$tmpdir/tests/"; \
	echo "Linting rules..."; \
	promtool check rules --lint=all --lint-fatal "$$tmpdir"/*.yaml; \
	echo "Linting rules with NAMESPACE_LABEL=namespace..."; \
	promtool check rules --lint=all --lint-fatal "$$tmpdir"/namespace-label/*.yaml; \
	echo "Running unit tests..."; \
	promtool test rules --diff "$$tmpdir"/tests/*.yaml


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