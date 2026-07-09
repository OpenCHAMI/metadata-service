# Makefile for running k6 load tests

.PHONY: help install smoke staged storm endurance all clean

help:
	@echo "Metadata Service Load Tests"
	@echo ""
	@echo "Available targets:"
	@echo "  install   - Install k6 (macOS only)"
	@echo "  smoke     - Quick smoke test (10 VUs, 30s)"
	@echo "  staged    - Staged boot test (1K→10K, ~5min)"
	@echo "  storm     - Cold boot storm (10K concurrent, ~3min)"
	@echo "  endurance - Sustained load (2K VUs, 10min)"
	@echo "  all       - Run all tests in sequence"
	@echo "  clean     - Remove test results"

install:
	@which k6 > /dev/null || (echo "Installing k6..." && brew install k6)

smoke: install
	@echo "Running smoke test..."
	k6 run smoke.js

staged: install
	@echo "Running staged boot test..."
	k6 run staged-boot.js

storm: install
	@echo "Running cold boot storm test..."
	k6 run boot-storm.js

endurance: install
	@echo "Running endurance test..."
	k6 run endurance.js

all: smoke staged storm
	@echo ""
	@echo "All tests complete!"
	@echo "Review results in load-tests/results/"

clean:
	rm -rf load-tests/results/*.json
	rm -rf load-tests/results/*.html
