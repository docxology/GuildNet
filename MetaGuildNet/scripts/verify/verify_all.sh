#!/bin/bash
# Comprehensive verification of all layers

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/../lib/common.sh"

# Configuration
TIMEOUT="${METAGN_VERIFY_TIMEOUT:-300}"
JSON_OUTPUT=false
STEP_BY_STEP=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --json) JSON_OUTPUT=true; shift ;;
        --step-by-step) STEP_BY_STEP=true; shift ;;
        *) shift ;;
    esac
done

# Verification state
declare -a RESULTS=()
OVERALL_STATUS="healthy"
START_TIME=$(date +%s)

main() {
    if [[ "$JSON_OUTPUT" == "false" ]]; then
        log_section "MetaGuildNet Verification Suite"
    fi
    
    # Run verifications
    verify_layer "Network" "$SCRIPT_DIR/verify_network.sh"
    maybe_pause "Network"
    
    verify_layer "Cluster" "$SCRIPT_DIR/verify_cluster.sh"
    maybe_pause "Cluster"
    
    verify_layer "Database" "$SCRIPT_DIR/verify_database.sh"
    maybe_pause "Database"
    
    verify_layer "Application" "$SCRIPT_DIR/verify_application.sh"
    maybe_pause "Application"
    
    # Show results
    show_results
    
    # Exit with appropriate code
    [[ "$OVERALL_STATUS" == "healthy" ]] && return 0 || return 1
}

verify_layer() {
    local layer="$1"
    local script="$2"
    
    if [[ "$JSON_OUTPUT" == "false" ]]; then
        log_subsection "L${#RESULTS[@]}: $layer Layer"
    fi
    
    local layer_start
    layer_start=$(date +%s)
    
    if [[ -f "$script" ]]; then
        if bash "$script" 2>&1 | tee /tmp/verify_${layer,,}.log; then
            local layer_end
            layer_end=$(date +%s)
            local duration=$((layer_end - layer_start))
            
            RESULTS+=("$layer:healthy:$duration")
            
            if [[ "$JSON_OUTPUT" == "false" ]]; then
                log_success "$layer Layer: HEALTHY (${duration}s)"
            fi
        else
            OVERALL_STATUS="unhealthy"
            RESULTS+=("$layer:unhealthy:0")
            
            if [[ "$JSON_OUTPUT" == "false" ]]; then
                log_error "$layer Layer: UNHEALTHY"
            fi
        fi
    else
        RESULTS+=("$layer:skipped:0")
        if [[ "$JSON_OUTPUT" == "false" ]]; then
            log_warn "$layer Layer: SKIPPED (verification script not found)"
        fi
    fi
}

maybe_pause() {
    if [[ "$STEP_BY_STEP" == "true" ]] && [[ "$JSON_OUTPUT" == "false" ]]; then
        echo ""
        read -r -p "Press Enter to continue to next layer..."
        echo ""
    fi
}

show_results() {
    local end_time
    end_time=$(date +%s)
    local total_duration=$((end_time - START_TIME))
    
    if [[ "$JSON_OUTPUT" == "true" ]]; then
        show_json_results "$total_duration"
    else
        show_text_results "$total_duration"
    fi
}

show_json_results() {
    local duration="$1"
    
    cat << EOF
{
  "timestamp": "$(timestamp_iso)",
  "overall_status": "$OVERALL_STATUS",
  "duration_seconds": $duration,
  "layers": [
EOF
    
    local first=true
    for result in "${RESULTS[@]}"; do
        IFS=: read -r layer status layer_duration <<< "$result"
        
        if [[ "$first" == "true" ]]; then
            first=false
        else
            echo ","
        fi
        
        cat << EOF
    {
      "name": "$layer",
      "status": "$status",
      "duration_seconds": $layer_duration
    }
EOF
    done
    
    cat << EOF

  ]
}
EOF
}

show_text_results() {
    local duration="$1"
    
    echo ""
    log_section "Verification Results"
    
    local passed=0
    local failed=0
    local skipped=0
    
    for result in "${RESULTS[@]}"; do
        IFS=: read -r layer status layer_duration <<< "$result"
        
        case "$status" in
            healthy)
                log_success "$layer: HEALTHY (${layer_duration}s)"
                passed=$((passed + 1))
                ;;
            unhealthy)
                log_error "$layer: UNHEALTHY"
                failed=$((failed + 1))
                ;;
            skipped)
                log_warn "$layer: SKIPPED"
                skipped=$((skipped + 1))
                ;;
        esac
    done
    
    echo ""
    echo "╔════════════════════════════════════════╗"
    printf "║ Overall Status: %-22s ║\n" "$OVERALL_STATUS"
    printf "║ Total Duration: %-22s ║\n" "$(human_duration "$duration")"
    printf "║ Passed: %-30s ║\n" "$passed/${#RESULTS[@]}"
    if ((failed > 0)); then
        printf "║ Failed: %-30s ║\n" "$failed"
    fi
    if ((skipped > 0)); then
        printf "║ Skipped: %-29s ║\n" "$skipped"
    fi
    echo "╚════════════════════════════════════════╝"
    echo ""
    
    if [[ "$OVERALL_STATUS" != "healthy" ]]; then
        log_info "For troubleshooting, run: make meta-diagnose"
    fi
}

main "$@"

