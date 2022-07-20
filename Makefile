
# Image URL to use all building/pushing image targets
MANAGER_IMG ?= ghcr.io/weaveworks/tf-controller
RUNNER_IMG  ?= ghcr.io/weaveworks/tf-runner
TAG ?= latest
BUILD_SHA ?= $(shell git rev-parse --short HEAD)
BUILD_VERSION ?= $(shell git describe --tags $$(git rev-list --tags --max-count=1))

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.22
# source controller version
SOURCE_VER ?= v0.24.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# Allows for defining additional Docker buildx arguments, e.g. '--push'.
BUILD_ARGS ?=

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
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

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config="config/crd/bases"
	cd api; $(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config="../config/crd/bases"

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
	cd api; $(CONTROLLER_GEN) object:headerFile="../hack/boilerplate.go.txt" paths="./..."

# Generate API reference documentation
.PHONY: api-docs
api-docs: gen-crd-api-reference-docs
	$(GEN_CRD_API_REFERENCE_DOCS) -api-dir=./api/v1alpha1 -config=./hack/api-docs/config.json -template-dir=./hack/api-docs/template -out-file=./docs/References/terraform.md

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

download-crd-deps:
	curl -s https://raw.githubusercontent.com/fluxcd/source-controller/${SOURCE_VER}/config/crd/bases/source.toolkit.fluxcd.io_gitrepositories.yaml > config/crd/bases/gitrepositories.yaml
	curl -s https://raw.githubusercontent.com/fluxcd/source-controller/${SOURCE_VER}/config/crd/bases/source.toolkit.fluxcd.io_buckets.yaml > config/crd/bases/buckets.yaml

TEST_SETTINGS=INSECURE_LOCAL_RUNNER=1 DISABLE_K8S_LOGS=1 DISABLE_TF_LOGS=1 DISABLE_TF_K8S_BACKEND=1 KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)"

.PHONY: test
test: manifests generate download-crd-deps fmt vet envtest api-docs ## Run tests.
	$(TEST_SETTINGS) go test ./controllers -coverprofile cover.out -v

# usage: make TARGET=250 target-test
.PHONY: target-test
target-test: manifests generate download-crd-deps fmt vet envtest api-docs ## Run tests. e.g make TARGET=250 target-test
	$(TEST_SETTINGS) go test ./controllers -coverprofile cover.out -v -run $(TARGET)

gen-grpc:
	./tools/bin/protoc --go_out=. --go_opt=Mrunner/runner.proto=runner/ --go-grpc_out=. --go-grpc_opt=Mrunner/runner.proto=runner/ runner/runner.proto
##@ Build

.PHONY: build
build: gen-grpc generate fmt vet ## Build manager binary.
	go build -o bin/runner cmd/runner/main.go
	go build -o bin/manager cmd/manager/main.go
	go build -o bin/tfctl \
		-ldflags "-X main.BuildSHA=$(BUILD_SHA) -X main.BuildVersion=$(BUILD_VERSION)" \
		cmd/tfctl/main.go

.PHONY: install-cli
install-cli:
	go build -o ${GOPATH}/bin/tfctl \
		-ldflags "-X main.BuildSHA=$(BUILD_SHA) -X main.BuildVersion=$(BUILD_VERSION)" \
		cmd/tfctl/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run cmd/manager/main.go

.PHONY: docker-build
docker-build: ## Build docker
	docker build -t ${MANAGER_IMG}:${TAG} ${BUILD_ARGS} .
	docker build -t ${RUNNER_IMG}:${TAG} -f runner.Dockerfile ${BUILD_ARGS} .

.PHONY: docker-buildx
docker-buildx: ## Build docker
	docker buildx build --load -t ${MANAGER_IMG}:${TAG} ${BUILD_ARGS} .
	docker buildx build --load -t ${RUNNER_IMG}:${TAG} -f runner.Dockerfile ${BUILD_ARGS} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${MANAGER_IMG}:${TAG}
	docker push ${RUNNER_IMG}:${TAG}

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image weaveworks/tf-controller=${MANAGER_IMG}:${TAG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

# Deploy controller dev image in the configured Kubernetes cluster in ~/.kube/config
.PHONY: dev-deploy
dev-deploy: manifests kustomize
	mkdir -p config/dev && cp config/default/* config/dev
	cd config/dev && $(KUSTOMIZE) edit set image ghcr.io/weaveworks/tf-controller=${MANAGER_IMG}:${TAG}
	$(KUSTOMIZE) build config/dev | yq e "select(.kind == \"Deployment\" and .metadata.name == \"tf-controller\").spec.template.spec.containers[0].env[1].value = \"test/tf-runner:$${TAG}\"" - | kubectl apply -f -
	rm -rf config/dev

# Delete dev deployment and CRDs
.PHONY: dev-cleanup
dev-cleanup: manifests kustomize
	mkdir -p config/dev && cp config/default/* config/dev
	cd config/dev && $(KUSTOMIZE) edit set image ghcr.io/weaveworks/tf-controller=${MANAGER_IMG}:${TAG}
	$(KUSTOMIZE) build config/dev | kubectl delete -f -
	rm -rf config/dev

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.7.0)

KUSTOMIZE = $(shell pwd)/bin/kustomize
.PHONY: kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v3@v3.8.7)

ENVTEST = $(shell pwd)/bin/setup-envtest
.PHONY: envtest
envtest: ## Download envtest-setup locally if necessary.
	$(call go-get-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

# Find or download gen-crd-api-reference-docs
GEN_CRD_API_REFERENCE_DOCS = $(shell pwd)/bin/gen-crd-api-reference-docs
.PHONY: gen-crd-api-reference-docs
gen-crd-api-reference-docs:
	$(call go-get-tool,$(GEN_CRD_API_REFERENCE_DOCS),github.com/ahmetb/gen-crd-api-reference-docs@v0.3.0)

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

.PHONY: release-manifests
release-manifests:
	rm -rf ./config/release || true
	mkdir ./config/release
	kustomize build ./config/crd > ./config/release/tf-controller.crds.yaml
	kustomize build ./config/rbac > ./config/release/tf-controller.rbac.yaml
	kustomize build ./config/manager > ./config/release/tf-controller.deployment.yaml
	(cd ./config/release && yq e . tf-controller.crds.yaml tf-controller.deployment.yaml tf-controller.rbac.yaml > tf-controller.all.yaml)
