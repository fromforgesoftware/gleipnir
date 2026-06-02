CONDUIT_DIR ?= services/conduit

.PHONY: conduit-proto conduit-build conduit-test conduit-vet conduit-lint conduit-migrate conduit-run

conduit-proto:          ## Regenerate Go code from the Conduit protos
	cd $(CONDUIT_DIR) && buf generate

conduit-lint:           ## Lint the Conduit protos
	cd $(CONDUIT_DIR) && buf lint

conduit-build:          ## Build all Conduit packages
	cd $(CONDUIT_DIR) && go build ./...

conduit-vet:            ## Vet the Conduit module
	cd $(CONDUIT_DIR) && go vet ./...

conduit-test:           ## Run Conduit unit tests
	cd $(CONDUIT_DIR) && go test ./...

CONDUIT_LOCAL_ENV := SVC_NAME=conduit REST_ADDRESS=:8080 HTTP_ADDRESS=:8080 GRPC_ADDRESS=:9090

conduit-migrate:        ## Apply Conduit migrations (reads DB_* env)
	cd $(CONDUIT_DIR) && go run ./cmd/migrator

conduit-run:            ## Run the Conduit server locally
	cd $(CONDUIT_DIR) && $(CONDUIT_LOCAL_ENV) go run ./cmd/server
