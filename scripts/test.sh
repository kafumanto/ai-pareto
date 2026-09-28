#!/bin/sh
# Purpose
# Verify the ai-pareto Go module with quick or full checks and report failures with their commands.
#
# Usage
# Run "scripts/test.sh --quick" for fast work-in-progress checks.
# Run "scripts/test.sh --full" or "scripts/test.sh" before a commit.
# Add "--verbose" to display intermediate output from every selected check.
#
# Arguments
# Accept --quick, --full, and --verbose. The default profile is --full and verbosity is disabled.
#
# Requirements
# Require POSIX sh, Go 1.27.x, and Git for full checks.
#
# Working directory
# The script accepts any current directory and resolves the repository root from its own path.
#
# Exit status
# Exit with status 0 when every selected check passes. Exit with status 1 when a check fails.
# Exit with status 2 for invalid arguments or status 127 when a required command is unavailable.

# Stop on unset variables while allowing the runner to continue after individual check failures.
set -u

# Resolve the directory containing this script; the command substitution returns its absolute path.
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd) || {
	printf '%s\n' 'error: cannot resolve the script directory.' >&2
	printf '%s\n' 'fix: run the script from an existing path and verify directory permissions.' >&2
	exit 1
}
# Resolve the module root from the script directory; the command substitution returns its absolute path.
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd) || {
	printf '%s\n' 'error: cannot resolve the repository root.' >&2
	printf '%s\n' 'fix: verify that the script is inside the repository and that its parent directory is accessible.' >&2
	exit 1
}

# Store customizable project expectations and target definitions in one configuration section.
FULL_PROFILE=full
QUICK_PROFILE=quick
EXPECTED_MODULE=github.com/kafumanto/ai-pareto
EXPECTED_GO_VERSION=1.27
# Keep package paths relative so the same list can drive expected-package checks and package-specific checks.
PACKAGE_PATHS='./internal/domain ./internal/optimize ./internal/source'
QUICK_TARGETS='linux/amd64'
FULL_TARGETS='linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64'

# Store the selected profile, verbosity mode, and accumulated runner status.
PROFILE=$FULL_PROFILE
PROFILE_SET=0
VERBOSE=0
CHECKS_RUN=0
FAILED_CHECKS=0
# Store parsed coverage lines until verbose output reaches the final summary.
COVERAGE_OUTPUT=

# Print the command-line usage text without changing repository state.
usage() {
	cat <<'EOF'
Usage: scripts/test.sh [--full|--quick]

Profiles:
  --full   Run all checks, including uncached tests, race and coverage checks,
           API documentation checks, Git whitespace checks, and six target builds.
  --quick  Run fast module, package, formatting, vet, normal test, and linux/amd64 build checks.

Options:
  --verbose  Display intermediate output from every selected check.
EOF
}

# Parse profiles and verbosity, and reject ambiguous or unknown arguments.
while [ "$#" -gt 0 ]; do
	case "$1" in
		--$FULL_PROFILE|--$QUICK_PROFILE)
			if [ "$PROFILE_SET" -eq 1 ]; then
				printf '%s\n' 'error: choose only one of --full and --quick' >&2
				printf '%s\n' 'fix: choose exactly one testing profile.' >&2
				exit 2
			fi
			PROFILE=${1#--}
			PROFILE_SET=1
			;;
		--verbose)
			VERBOSE=1
			;;
		--help|-h)
			usage
			exit 0
			;;
		*)
			printf 'error: unknown argument: %s\n' "$1" >&2
			printf '%s\n' 'fix: rerun the script with --help and choose a supported option.' >&2
			usage >&2
			exit 2
			;;
	esac
	shift
done

# Change to the module root before running Go, formatting, documentation, and Git checks.
cd "$REPO_ROOT" || {
	printf 'error: cannot change to repository root: %s\n' "$REPO_ROOT" >&2
	printf '%s\n' 'fix: verify that the repository root exists and that the directory is accessible.' >&2
	exit 1
}

# Fail early with an actionable status when a required executable is unavailable.
for required_command in go gofmt; do
	if ! command -v "$required_command" >/dev/null 2>&1; then
		printf 'error: required command is unavailable: %s\n' "$required_command" >&2
		printf 'fix: install Go %s or add the Go binary directory to PATH.\n' "$EXPECTED_GO_VERSION" >&2
		exit 127
	fi
done
if [ "$PROFILE" = "$FULL_PROFILE" ] && ! command -v git >/dev/null 2>&1; then
	printf '%s\n' 'error: required command is unavailable for the full profile: git' >&2
	printf '%s\n' 'fix: install Git or run the quick profile.' >&2
	exit 127
fi

# Run one named check, retain its output, and display it only when verbose or when the check fails.
# The fourth argument identifies the most likely correction when the check fails.
run_check() {
	check_name=$1
	check_command=$2
	check_function=$3
	check_fix=$4
	shift 4
	CHECKS_RUN=$((CHECKS_RUN + 1))

	# Execute the check with combined output so failures can report all diagnostics while passes stay quiet.
	# The command substitution returns the check function's combined standard output and standard error.
	check_output=$("$check_function" "$@" 2>&1)
	check_status=$?
	if [ "$VERBOSE" -eq 1 ]; then
		# Extract coverage lines from captured check output so the final summary can report them together.
		# The command substitution returns every coverage line emitted by the enhanced test check.
		deferred_coverage=$(printf '%s\n' "$check_output" | sed -n 's/^\(coverage: .*$\)$/\1/p')
		if [ -n "$deferred_coverage" ]; then
			COVERAGE_OUTPUT=$deferred_coverage
			# Remove deferred coverage lines from the check output so they are not displayed twice.
			check_output=$(printf '%s\n' "$check_output" | sed '/^coverage: /d')
		fi
	fi
	if [ "$VERBOSE" -eq 1 ] || [ "$check_status" -ne 0 ]; then
		printf '\n[%02d] %s\n' "$CHECKS_RUN" "$check_name"
		printf 'command: %s\n' "$check_command"
		if [ -n "$check_output" ]; then
			printf 'output:\n%s\n' "$check_output"
		fi
		if [ "$check_status" -eq 0 ]; then
			printf 'result: PASS\n'
		else
			printf 'result: FAIL (exit %s)\n' "$check_status"
			printf 'action: inspect the output above and correct the reported problem.\n'
			printf 'fix: %s\n' "$check_fix"
		fi
	fi

	# Record a failure while allowing later checks to report independent problems.
	if [ "$check_status" -ne 0 ]; then
		FAILED_CHECKS=$((FAILED_CHECKS + 1))
	fi
}

# Verify the installed toolchain belongs to the required Go 1.27 series.
check_go_version() {
	# Capture the toolchain version string so the configured Go release controls validation.
	go_version=$(go env GOVERSION) || return 1
	printf 'Go version: %s\n' "$go_version"
	case "$go_version" in
		go${EXPECTED_GO_VERSION}|go${EXPECTED_GO_VERSION}.*) return 0 ;;
		*)
			printf 'expected Go %s.x, got %s\n' "$EXPECTED_GO_VERSION" "$go_version" >&2
			return 1
			;;
	esac
}

# Verify the module path and declared Go version from the module metadata.
check_module_metadata() {
	# Capture JSON module metadata so the configured module and Go versions can be checked together.
	module_json=$(go list -m -json) || return 1
	printf '%s\n' "$module_json"
	# Extract the module path; printf supplies the JSON and sed returns its Path field.
	module_path=$(printf '%s\n' "$module_json" | sed -n 's/.*"Path": "\([^"]*\)".*/\1/p')
	# Extract the declared Go version; printf supplies the JSON and sed returns its GoVersion field.
	module_go=$(printf '%s\n' "$module_json" | sed -n 's/.*"GoVersion": "\([^"]*\)".*/\1/p')
	if [ "$module_path" != "$EXPECTED_MODULE" ] || [ "$module_go" != "$EXPECTED_GO_VERSION" ]; then
		printf 'expected module %s with Go %s, got %s with Go %s\n' \
			"$EXPECTED_MODULE" "$EXPECTED_GO_VERSION" "$module_path" "$module_go" >&2
		return 1
	fi
}

# Verify that the module has no dependency beyond its own main module.
check_module_dependencies() {
	# Capture every module path; the command substitution returns one path per line for exact comparison.
	module_list=$(go list -m all) || return 1
	printf '%s\n' "$module_list"
	if [ "$module_list" != "$EXPECTED_MODULE" ]; then
		printf 'expected only the main module, got additional module entries\n' >&2
		return 1
	fi
}

# Verify that package discovery contains exactly the foundation packages owned by this change.
check_packages() {
	# Capture every package import path; the command substitution returns one path per line for exact comparison.
	packages=$(go list ./...) || return 1
	printf '%s\n' "$packages"
	# Build expected import paths from the relative package-path configuration so one list remains authoritative.
	# The command substitution returns the configured package paths with the module path prefixed.
	expected_packages=$(for package_path in $PACKAGE_PATHS; do
		printf '%s/%s\n' "$EXPECTED_MODULE" "${package_path#./}"
	done)
	if [ "$packages" != "$expected_packages" ]; then
		printf 'expected package list:\n%s\nactual package list:\n%s\n' \
			"$expected_packages" "$packages"
		return 1
	fi
}

# Verify every Go source file is already formatted without modifying it.
check_format() {
	# Capture paths reported by gofmt; the command substitution returns one unformatted path per line.
	unformatted=$(gofmt -l .)
	if [ -n "$unformatted" ]; then
		printf 'unformatted Go files:\n%s\n' "$unformatted" >&2
		return 1
	fi
}

# Verify that package documentation commands can load every foundation package.
check_documentation() {
	# Discover current module packages so documentation coverage follows the module without another hardcoded list.
	# The command substitution returns one package import path per line for the documentation loop.
	packages=$(go list ./...) || return 1
	documentation_failed=0
	for package in $packages; do
		printf '\nDocumentation: %s\n' "$package"
		if ! go doc "$package"; then
			documentation_failed=1
		fi
	done
	return "$documentation_failed"
}

# Verify static analysis for every package.
check_vet() {
	go vet ./...
}

# Verify all package tests under the normal cached test configuration.
check_tests() {
	go test ./...
}

# Verify uncached race detection and statement coverage in one enhanced test run.
check_enhanced_tests() {
	# Enable the race detector with -race to report data races during tests.
	# Enable statement coverage reporting with -cover to report executed code coverage.
	# Set -count=1 to bypass cached test results and force a fresh test run.
	# Capture the complete test output and status so verbose mode can separate coverage lines without running tests twice.
	enhanced_output=$(go test -race -cover -count=1 ./... 2>&1)
	enhanced_status=$?
	if [ "$VERBOSE" -eq 1 ]; then
		# Remove the coverage suffix from test-result lines so coverage can be reported separately.
		# The command substitution returns test output without coverage details.
		test_output=$(printf '%s\n' "$enhanced_output" | sed 's/[[:space:]]coverage:.*$//')
		# Extract package names and coverage values from test-result lines.
		# The command substitution returns one coverage line for each package that reports coverage.
		coverage_output=$(printf '%s\n' "$enhanced_output" | sed -n 's/^ok[[:space:]][[:space:]]*\([^[:space:]]*\).*coverage:[[:space:]]*\(.*\)$/coverage: \1 \2/p')
		if [ -n "$test_output" ]; then
			printf '%s\n' "$test_output"
		fi
		# Keep coverage lines separate from test-result lines so run_check can defer them until the final summary.
		if [ -n "$coverage_output" ]; then
			printf '%s\n' "$coverage_output"
		fi
	elif [ -n "$enhanced_output" ]; then
		printf '%s\n' "$enhanced_output"
	fi
	return "$enhanced_status"
}

# Verify every required operating-system and architecture target with CGO disabled.
check_target_builds() {
	build_failed=0
	# Select the target set configured for the active profile so quick checks stay fast and full checks cover every target.
	if [ "$PROFILE" = "$FULL_PROFILE" ]; then
		targets=$FULL_TARGETS
	else
		targets=$QUICK_TARGETS
	fi
	for target in $targets; do
		# Split each configured target into operating-system and architecture values for the build environment.
		target_os=${target%/*}
		target_arch=${target#*/}
		printf '\nTarget: GOOS=%s GOARCH=%s CGO_ENABLED=0\n' "$target_os" "$target_arch"
		if ! CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build ./...; then
			build_failed=1
			printf 'target failed: GOOS=%s GOARCH=%s\n' "$target_os" "$target_arch" >&2
		fi
	done
	return "$build_failed"
}

# Verify Git reports no whitespace errors across staged and unstaged changes.
check_git_diff() {
	git diff --check HEAD
}

# Run the fast checks needed during normal development.
run_check 'Go toolchain version' 'go env GOVERSION' check_go_version \
	"Install Go ${EXPECTED_GO_VERSION}.x or select that toolchain in PATH."
run_check 'Module metadata' 'go list -m -json' check_module_metadata \
	'Fix go.mod, or update EXPECTED_MODULE and EXPECTED_GO_VERSION if the project metadata changed intentionally.'
run_check 'Module dependencies' 'go list -m all' check_module_dependencies \
	'Review go.mod and go.sum, then remove unintended dependencies or run go mod tidy when dependency changes are intentional.'
run_check 'Package list' 'go list ./...' check_packages \
	'Fix the reported module, import, or source error, then update PACKAGE_PATHS if the package set changed intentionally.'
run_check 'Go formatting' 'gofmt -l .' check_format \
	'Run gofmt -w . and fix any syntax errors it reports.'
run_check 'Go vet' 'go vet ./...' check_vet \
	'Fix every diagnostic reported by go vet.'
run_check 'Normal package tests' 'go test ./...' check_tests \
	'Fix the failing test or implementation, then rerun the affected package test.'
run_check 'CGO-disabled target builds' 'CGO_ENABLED=0 go build ./... for selected profile targets' check_target_builds \
	'Fix the reported platform-specific or CGO-disabled build error, or update the target list only if support intentionally changed.'

# Run expensive checks only in the full profile before a commit.
if [ "$PROFILE" = "$FULL_PROFILE" ]; then
	run_check 'Package documentation' 'go list ./... and go doc each package' check_documentation \
		'Fix the reported package-loading or documentation error, then rerun go doc.'
	run_check 'Uncached race and coverage tests' 'go test -race -cover -count=1 ./...' check_enhanced_tests \
		'Fix the reported test failure or race, then rerun the enhanced test command.'
	run_check 'Git whitespace check' 'git diff --check HEAD' check_git_diff \
		'Remove the whitespace errors reported by git diff --check HEAD.'
fi

# Report coverage immediately before the verbose summary so the final metrics stay together.
if [ "$VERBOSE" -eq 1 ] && [ -n "$COVERAGE_OUTPUT" ]; then
	printf '\nCoverage:\n%s\n' "$COVERAGE_OUTPUT"
fi

# Report the complete run only when verbose output was requested.
if [ "$VERBOSE" -eq 1 ]; then
	printf '\nSummary: profile=%s verbose=%s checks=%s failed=%s\n' \
		"$PROFILE" "$VERBOSE" "$CHECKS_RUN" "$FAILED_CHECKS"
fi
if [ "$FAILED_CHECKS" -eq 0 ]; then
	exit 0
fi
printf 'Result: FAIL\n' >&2
exit 1
