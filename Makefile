# Which implementation to run: go or python.
APP_LANG ?= go
CLUSTER  ?= txn-sandbox
NS       ?= txn-platform
IMAGE    := txn-api-$(APP_LANG):dev

.PHONY: help doctor cluster build load infra deploy loadgen clean

help: ## Show targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

doctor: ## Check that the tools you need are installed and working
	@./scripts/doctor.sh

cluster: ## Create the kind cluster and seed it with the objects that already exist in prod
	kind create cluster --config kind-config.yaml
	kubectl apply -f kind-seed.yaml

build: ## Build the app image (APP_LANG=go|python)
	docker build -t $(IMAGE) app/$(APP_LANG)

load: ## Copy the app image into the kind cluster
	kind load docker-image $(IMAGE) --name $(CLUSTER)

infra: ## Apply the Terraform (namespace, ConfigMap, Secret)
	terraform -chdir=terraform init -input=false
	terraform -chdir=terraform apply -input=false -auto-approve

deploy: ## Install or upgrade the Helm release
	helm upgrade --install txn-api deploy/helm/txn-api -n $(NS) --set image.repository=txn-api-$(APP_LANG)

loadgen: ## Send traffic to the app (expects a port-forward on localhost:8080)
	./scripts/loadgen.sh

clean: ## Delete the kind cluster and local Terraform state
	kind delete cluster --name $(CLUSTER)
	rm -rf terraform/.terraform terraform/terraform.tfstate*
