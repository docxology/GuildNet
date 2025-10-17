# MetaGuildNet Makefile
# Fork-specific automation and verification

.PHONY: help \
	setup-wizard check-prereqs setup-network setup-cluster setup-application \
	verify verify-network verify-cluster verify-database verify-application verify-all verify-json verify-watch verify-verbose verify-step-by-step \
	test-all test-integration test-e2e test-coverage benchmark load-test \
	diagnose diag-network diag-cluster diag-database diag-application export-diagnostics \
	health-check cleanup sync-upstream docs

# Configuration
SCRIPT_DIR := $(CURDIR)/scripts
TEST_DIR := $(CURDIR)/tests
DOCS_DIR := $(CURDIR)/docs
EXAMPLES_DIR := $(CURDIR)/examples

# MetaGuildNet settings
METAGN_SETUP_MODE ?= auto
METAGN_VERIFY_TIMEOUT ?= 300
METAGN_AUTO_APPROVE_ROUTES ?= true
METAGN_LOG_LEVEL ?= info

# Export for scripts
export METAGN_SETUP_MODE
export METAGN_VERIFY_TIMEOUT
export METAGN_AUTO_APPROVE_ROUTES
export METAGN_LOG_LEVEL

# ---------- Help ----------
help: ## Show this help
	@echo "MetaGuildNet - GuildNet Fork Enhancements"
	@echo ""
	@echo "Quick Start:"
	@echo "  make setup-wizard    # Automated full setup"
	@echo "  make verify-all      # Comprehensive verification"
	@echo "  make health-check    # Quick health check"
	@echo ""
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*##"; printf ""} /^[a-zA-Z0-9_-]+:.*##/ { printf "  %-24s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# ---------- Setup ----------
setup-wizard: ## Run automated setup wizard (all layers)
	@echo "Starting MetaGuildNet setup wizard..."
	bash $(SCRIPT_DIR)/setup/setup_wizard.sh

check-prereqs: ## Check prerequisites and dependencies
	bash $(SCRIPT_DIR)/setup/check_prerequisites.sh

setup-network: ## Setup network layer (Headscale + Tailscale)
	bash $(SCRIPT_DIR)/setup/setup_network.sh

setup-cluster: ## Setup cluster layer (Talos + K8s + addons)
	bash $(SCRIPT_DIR)/setup/setup_cluster.sh

setup-application: ## Setup application layer (Host App + Operator)
	bash $(SCRIPT_DIR)/setup/setup_application.sh

# ---------- Verification ----------
verify: verify-all ## Alias for verify-all

verify-all: ## Run comprehensive verification (all layers)
	bash $(SCRIPT_DIR)/verify/verify_all.sh

verify-network: ## Verify network layer only
	bash $(SCRIPT_DIR)/verify/verify_network.sh

verify-cluster: ## Verify cluster layer only
	bash $(SCRIPT_DIR)/verify/verify_cluster.sh

verify-database: ## Verify database layer only
	bash $(SCRIPT_DIR)/verify/verify_database.sh

verify-application: ## Verify application layer only
	bash $(SCRIPT_DIR)/verify/verify_application.sh

verify-json: ## Output verification results as JSON
	bash $(SCRIPT_DIR)/verify/verify_all.sh --json

verify-watch: ## Watch mode (re-run every 30s)
	watch -n 30 $(MAKE) verify-all

verify-verbose: ## Verbose verification output
	METAGN_LOG_LEVEL=debug bash $(SCRIPT_DIR)/verify/verify_all.sh

verify-step-by-step: ## Step-by-step verification with pauses
	bash $(SCRIPT_DIR)/verify/verify_all.sh --step-by-step

# ---------- Testing ----------
test-all: test-integration test-e2e ## Run all tests

test-integration: ## Run integration tests
	bash $(TEST_DIR)/run_integration_tests.sh

test-e2e: ## Run end-to-end tests
	bash $(TEST_DIR)/run_e2e_tests.sh

test-coverage: ## Run tests with coverage report
	bash $(TEST_DIR)/run_with_coverage.sh

benchmark: ## Run performance benchmarks
	bash $(TEST_DIR)/run_benchmarks.sh

load-test: ## Run load tests (CONCURRENT=10)
	bash $(TEST_DIR)/run_load_tests.sh

# ---------- Diagnostics ----------
diagnose: ## Run comprehensive diagnostics
	bash $(SCRIPT_DIR)/utils/diagnose.sh

diag-network: ## Diagnose network layer
	bash $(SCRIPT_DIR)/utils/diagnose_network.sh

diag-cluster: ## Diagnose cluster layer
	bash $(SCRIPT_DIR)/utils/diagnose_cluster.sh

diag-database: ## Diagnose database layer
	bash $(SCRIPT_DIR)/utils/diagnose_database.sh

diag-application: ## Diagnose application layer
	bash $(SCRIPT_DIR)/utils/diagnose_application.sh

export-diagnostics: ## Export diagnostic bundle
	bash $(SCRIPT_DIR)/utils/export_diagnostics.sh

# ---------- Health Check ----------
health-check: ## Quick health check (all layers)
	bash $(SCRIPT_DIR)/utils/health_check.sh

# ---------- Cleanup ----------
cleanup: ## Clean up MetaGuildNet temporary files
	@echo "Cleaning up MetaGuildNet temporary files..."
	rm -rf $(CURDIR)/tmp/*
	rm -rf $(CURDIR)/logs/*
	rm -rf $(CURDIR)/diagnostics-*.tar.gz
	@echo "Cleanup complete"

# ---------- Upstream Sync ----------
sync-upstream: ## Sync with upstream GuildNet
	bash $(SCRIPT_DIR)/utils/sync_upstream.sh

# ---------- Documentation ----------
docs: ## Generate/serve documentation
	@echo "MetaGuildNet documentation:"
	@echo "  README:        $(DOCS_DIR)/README.md"
	@echo "  Architecture:  $(DOCS_DIR)/ARCHITECTURE.md"
	@echo "  Setup:         $(DOCS_DIR)/SETUP.md"
	@echo "  Verification:  $(DOCS_DIR)/VERIFICATION.md"
	@echo "  Contributing:  $(DOCS_DIR)/CONTRIBUTING.md"
	@echo "  Upstream Sync: $(DOCS_DIR)/UPSTREAM_SYNC.md"

# ---------- Examples ----------
example-basic: ## Run basic example
	bash $(EXAMPLES_DIR)/basic/create-workspace.sh

example-advanced: ## Run advanced example
	bash $(EXAMPLES_DIR)/advanced/multi-user-setup.sh

# ---------- Development ----------
dev-watch: ## Watch for changes and re-verify
	bash $(SCRIPT_DIR)/utils/dev_watch.sh

dev-logs: ## Tail all logs
	bash $(SCRIPT_DIR)/utils/tail_logs.sh

# ---------- Info ----------
info: ## Show MetaGuildNet configuration
	@echo "MetaGuildNet Configuration:"
	@echo "  Setup Mode:           $(METAGN_SETUP_MODE)"
	@echo "  Verify Timeout:       $(METAGN_VERIFY_TIMEOUT)s"
	@echo "  Auto Approve Routes:  $(METAGN_AUTO_APPROVE_ROUTES)"
	@echo "  Log Level:            $(METAGN_LOG_LEVEL)"
	@echo ""
	@echo "Directories:"
	@echo "  Scripts:  $(SCRIPT_DIR)"
	@echo "  Tests:    $(TEST_DIR)"
	@echo "  Docs:     $(DOCS_DIR)"
	@echo "  Examples: $(EXAMPLES_DIR)"

version: ## Show MetaGuildNet version
	@echo "MetaGuildNet v1.0.0-meta"
	@echo "Fork of GuildNet with enhanced automation and verification"

