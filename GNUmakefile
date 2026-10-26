default: test

# Build the provider into ./dist-dev and print the dev_overrides block that
# makes `terraform plan` use it without a registry.
.PHONY: build
build:
	go build -o dist-dev/terraform-provider-forumulate .
	@echo
	@echo 'Add to ~/.terraformrc:'
	@echo '  provider_installation {'
	@echo '    dev_overrides { "forumulate/forumulate" = "'$$(pwd)'/dist-dev" }'
	@echo '    direct {}'
	@echo '  }'

.PHONY: test
test:
	go vet ./...
	go test ./...

# The same run under a real Terraform CLI (set TF to its path, or have it on PATH).
.PHONY: e2e
e2e:
	FORUMULATE_E2E_TERRAFORM=$${TF:-$$(command -v terraform)} go test -count=1 -v -run TestProviderUnderRealTerraform ./internal/e2e

# Regenerate docs/ and format examples/.
.PHONY: docs
docs:
	go generate ./...

.PHONY: fmt
fmt:
	gofmt -w .
