#!/bin/bash

# Migration Validation Script
# Validates migration order, duplicates, foreign keys, and Batch 2 entities

set -e

echo "🔍 Starting migration validation..."

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MIGRATION_DIR="$SCRIPT_DIR/../../forge/api/migrations"

echo "Migration directory: $MIGRATION_DIR"

# How many files each migration identifier is allowed to have.
#
# Ten bare prefixes shipped with duplicates before the rule existed and are
# grandfathered at their shipped counts; everything else may appear once. This
# mirrors allowedHistoricalDuplicates/knownMax in validateNoDuplicatePrefixes
# (forge/api/internal/store/migration.go), which is the authority — change both
# together. A NEW file may not reuse these numbers bare: it needs a letter
# suffix (082_a_...) or a new number.
#
# A case statement rather than an associative array on purpose: macOS still
# ships bash 3.2, where `declare -A` is not supported and degrades into an
# indexed array whose "018" subscript is then parsed as an invalid octal
# literal. That made this check pass while validating nothing.
allowed_count_for() {
    case "$1" in
        015|018|020|044|054|080|083|087) echo 2 ;;
        057|082) echo 3 ;;
        *) echo 1 ;;
    esac
}

# Function to check for duplicate migration identifiers
check_duplicates() {
    echo -e "\n${BLUE}Checking for duplicate migration identifiers...${NC}"
    
    cd "$MIGRATION_DIR"
    
    # Match the runtime runner: numeric migrations use their number, while
    # letter-suffixed migrations use number+letter (for example, 103_a).
    DUPLICATES=$(
        for migration in *.sql; do
            stem=${migration%.sql}
            first=${stem%%_*}
            remainder=${stem#*_}
            letter=${remainder%%_*}
            if [[ $letter =~ ^[a-z]$ ]] && [[ $remainder == *_* ]]; then
                echo "${first}_${letter}"
            else
                echo "$first"
            fi
        done | sort | uniq -d
    )
    
    local failed=0
    while read -r count identifier; do
        [ -z "$identifier" ] && continue
        local allowed
        allowed=$(allowed_count_for "$identifier")
        if [ "$count" -gt "$allowed" ]; then
            if [ "$allowed" -eq 1 ]; then
                echo -e "${RED}❌ Duplicate migration identifier '$identifier' ($count files)${NC}"
                echo "   Rename one with a letter suffix (${identifier}_a_*) or use a new number."
            else
                echo -e "${RED}❌ Grandfathered identifier '$identifier' grew to $count files (max $allowed)${NC}"
                echo "   New files must not reuse this number bare; use ${identifier}_a_* or a new number."
            fi
            failed=1
        fi
    done < <(
        for migration in *.sql; do
            stem=${migration%.sql}
            first=${stem%%_*}
            remainder=${stem#*_}
            letter=${remainder%%_*}
            if [[ $letter =~ ^[a-z]$ ]] && [[ $remainder == *_* ]]; then
                echo "${first}_${letter}"
            else
                echo "$first"
            fi
        done | sort | uniq -c
    )

    if [ "$failed" -ne 0 ]; then
        return 1
    fi
    echo -e "${GREEN}✅ No unexpected duplicate migration identifiers${NC}"
    echo "   (10 historical duplicates grandfathered at their shipped counts)"
}

# Function to check migration filenames are parseable by the runner
#
# This replaces an earlier "check migration order" that sorted the filenames and
# then asserted the sorted list was sorted — it could never fail, and printing
# "Migration order is correct" from it was a success claim for work not done.
# Order is not a property of the directory anyway: sortMigrationFiles reorders
# every run (numeric-then-suffix, so bare 082 precedes 082_a).
#
# The invariant that does matter, and that nothing else here checks, is that
# every filename has the NNN[_x]_description.sql shape migrationPrefix expects.
check_filename_shape() {
    echo -e "\n${BLUE}Checking migration filename shape...${NC}"

    cd "$MIGRATION_DIR"

    local failed=0
    for migration in *.sql; do
        if [[ ! $migration =~ ^[0-9]{3}(_[a-z])?_[A-Za-z0-9_]+\.sql$ ]]; then
            echo -e "${RED}❌ Unparseable migration filename: $migration${NC}"
            failed=1
        fi
    done

    if [ "$failed" -ne 0 ]; then
        echo "   Expected NNN_description.sql or NNN_x_description.sql."
        return 1
    fi

    echo -e "${GREEN}✅ All migration filenames are parseable${NC}"
}

# Function to check for required Batch 2 migrations
check_batch2_migrations() {
    echo -e "\n${BLUE}Checking for required Batch 2 migrations...${NC}"
    
    cd "$MIGRATION_DIR"
    
    REQUIRED_BATCH2=(
        "100_team_tenancy.sql"
        "100_z_app_platform_applications.sql"
        "101_a_multi_node_replicas.sql"
        "102_a_uncloud_service_model.sql"
        "103_a_deployment_steps.sql"
        "103_b_procedures.sql"
        "104_a_backup_system.sql"
        "130_app_platform_applications.sql"
        "131_reconcile_plans.sql"
        "132_backup_schedules_orchestration.sql"
    )
    
    MISSING=()
    for MIGRATION in "${REQUIRED_BATCH2[@]}"; do
        if [ ! -f "$MIGRATION" ]; then
            MISSING+=("$MIGRATION")
        fi
    done
    
    if [ ${#MISSING[@]} -gt 0 ]; then
        echo -e "${RED}❌ Missing required Batch 2 migrations:${NC}"
        for MIGRATION in "${MISSING[@]}"; do
            echo "  - $MIGRATION"
        done
        return 1
    else
        echo -e "${GREEN}✅ All required Batch 2 migrations present${NC}"
    fi
}

# Function to check foreign key references
check_foreign_keys() {
    echo -e "\n${BLUE}Checking foreign key references...${NC}"
    
    cd "$MIGRATION_DIR"
    
    # Extract all table names from CREATE TABLE statements
    TABLES=$(grep -h -o 'CREATE TABLE IF NOT EXISTS [^ (]*' ./*.sql | sed 's/CREATE TABLE IF NOT EXISTS //' | sort -u)
    
    # Extract all referenced tables from foreign key constraints
    REFERENCES=$(grep -h -o 'REFERENCES [^ (]*' ./*.sql | sed 's/REFERENCES //' | sort -u)
    
    ISSUES=()
    for REF in $REFERENCES; do
        if ! printf '%s\n' "$TABLES" | grep -Fxq "$REF"; then
            ISSUES+=("$REF")
        fi
    done
    
    if [ ${#ISSUES[@]} -gt 0 ]; then
        echo -e "${YELLOW}⚠️  Foreign key references to potentially missing tables:${NC}"
        for ISSUE in "${ISSUES[@]}"; do
            echo "  - $ISSUE"
        done
        echo -e "${YELLOW}Note: These might be created in later migrations or in different files${NC}"
    else
        echo -e "${GREEN}✅ All foreign key references appear valid${NC}"
    fi
}

# Function to check for tenancy ownership columns
check_tenancy_columns() {
    echo -e "\n${BLUE}Checking tenancy ownership columns...${NC}"
    
    cd "$MIGRATION_DIR"
    
    # Check for org_id columns in key tables
    TABLES_WITH_TENANCY=("servers" "deployments" "backups" "applications" "app_services" "replica_applications" "instances")
    
    MISSING_TENANCY=()
    for TABLE in "${TABLES_WITH_TENANCY[@]}"; do
        if ! grep -h -E "(ALTER TABLE ${TABLE}.*org_id|CREATE TABLE.*${TABLE})" ./*.sql >/dev/null ||
           ! grep -h "org_id" ./*.sql >/dev/null; then
            MISSING_TENANCY+=("$TABLE")
        fi
    done
    
    if [ ${#MISSING_TENANCY[@]} -gt 0 ]; then
        echo -e "${YELLOW}⚠️  Tables without explicit org_id columns:${NC}"
        for TABLE in "${MISSING_TENANCY[@]}"; do
            echo "  - $TABLE"
        done
        echo -e "${YELLOW}Note: These might have org_id added in later migrations${NC}"
    else
        echo -e "${GREEN}✅ Tenancy ownership columns found in key tables${NC}"
    fi
}

# Function to check for required Batch 2 entities
check_batch2_entities() {
    echo -e "\n${BLUE}Checking Batch 2 entities...${NC}"
    
    cd "$MIGRATION_DIR"
    
    # List of Batch 2 entities and their tables
    BATCH2_ENTITIES=(
        "organizations:Team-based tenancy"
        "projects:Project grouping"
        "environments:Deployment environments"
        "team_members:Organization membership"
        "applications:Unified workload identity"
        "app_services:Service definitions"
        "replica_applications:Service definitions"
        "instances:Replicas"
        "placement_decisions:Placement constraints"
        "placement_reservations:Resource reservations"
        "reconcile_plans:Drift records"
        "reconcile_events:Reconciliation events"
        "service_endpoints:Service discovery endpoints"
        "procedures:Procedure definitions"
        "procedure_steps:Procedure steps"
        "procedure_executions:Procedure execution tracking"
        "deployment_steps:Deployment strategy and rollout state"
        "backup_policies:Backup policies"
        "backup_manifests:Backup manifests"
        "backup_storage_receipts:Storage receipts"
        "database_backups:Database backup records"
        "volume_backups:Volume backup records"
    )
    
    echo "  Batch 2 Entities:"
    for ENTITY in "${BATCH2_ENTITIES[@]}"; do
        TABLE=${ENTITY%%:*}
        DESCRIPTION=${ENTITY#*:}
        
        # Check if table is created in any migration
        if grep -h -E "CREATE TABLE( IF NOT EXISTS)? ${TABLE}[ (]" ./*.sql >/dev/null; then
            echo -e "  ${GREEN}✅${NC} $TABLE ($DESCRIPTION)"
        else
            echo -e "  ${YELLOW}⚠️${NC} $TABLE ($DESCRIPTION) - not found in CREATE TABLE statements"
        fi
    done
}

# Function to check for cascading behavior
check_cascading_behavior() {
    echo -e "\n${BLUE}Checking cascading behavior...${NC}"
    
    cd "$MIGRATION_DIR"
    
    # Look for ON DELETE CASCADE clauses
    CASCADE_COUNT=$(grep -h -c "ON DELETE CASCADE" ./*.sql | awk '{sum += $1} END {print sum + 0}')
    RESTRICT_COUNT=$(grep -h -c -E "ON DELETE RESTRICT|ON DELETE NO ACTION" ./*.sql | awk '{sum += $1} END {print sum + 0}')
    
    echo "  ON DELETE CASCADE: $CASCADE_COUNT occurrences"
    echo "  ON DELETE RESTRICT/NO ACTION: $RESTRICT_COUNT occurrences"
    
    if [ $CASCADE_COUNT -gt 0 ]; then
        echo -e "  ${YELLOW}⚠️  CASCADE deletes found - verify these are intentional${NC}"
        grep -n "ON DELETE CASCADE" *.sql | head -5 || true
    fi
}

# Function to check for unique constraints
check_unique_constraints() {
    echo -e "\n${BLUE}Checking unique constraints...${NC}"
    
    cd "$MIGRATION_DIR"
    
    UNIQUE_COUNT=$(grep -h -c "UNIQUE\|PRIMARY KEY" ./*.sql | awk '{sum += $1} END {print sum + 0}')
    echo "  Unique constraints: $UNIQUE_COUNT occurrences"
    
    # Look for specific important unique constraints
    IMPORTANT_UNIQUES=(
        "UNIQUE (org_id, slug)"
        "UNIQUE (project_id, name)"
        "UNIQUE (app_id, idx)"
    )
    
    for CONSTRAINT in "${IMPORTANT_UNIQUES[@]}"; do
        if grep -q "$CONSTRAINT" *.sql; then
            echo -e "  ${GREEN}✅${NC} Found: $CONSTRAINT"
        else
            echo -e "  ${YELLOW}⚠️${NC} Not found: $CONSTRAINT"
        fi
    done
}

# Function to check for nullable vs required fields
check_nullable_fields() {
    echo -e "\n${BLUE}Checking nullable vs required fields...${NC}"
    
    cd "$MIGRATION_DIR"
    
    NOT_NULL_COUNT=$(grep -h -c "NOT NULL" ./*.sql | awk '{sum += $1} END {print sum + 0}')
    NULL_COUNT=$(grep -h -c "NULL\|DEFAULT NULL" ./*.sql | awk '{sum += $1} END {print sum + 0}')
    
    echo "  NOT NULL constraints: $NOT_NULL_COUNT occurrences"
    echo "  NULL/DDEFAULT NULL: $NULL_COUNT occurrences"
    
    # Check for important required fields
    IMPORTANT_REQUIRED=(
        "id UUID PRIMARY KEY"
        "name TEXT NOT NULL"
        "created_at TIMESTAMPTZ NOT NULL"
    )
    
    for FIELD in "${IMPORTANT_REQUIRED[@]}"; do
        if grep -q "$FIELD" *.sql; then
            echo -e "  ${GREEN}✅${NC} Found required field pattern: $FIELD"
        fi
    done
}

# Function to check for indexes
check_indexes() {
    echo -e "\n${BLUE}Checking indexes...${NC}"
    
    cd "$MIGRATION_DIR"
    
    INDEX_COUNT=$(grep -h -c "CREATE INDEX\|CREATE UNIQUE INDEX" ./*.sql | awk '{sum += $1} END {print sum + 0}')
    echo "  Indexes: $INDEX_COUNT occurrences"
    
    # Look for important indexes
    IMPORTANT_INDEXES=(
        "idx_applications_org_id"
        "servers_org_id_idx"
        "deployments_org_id_idx"
        "idx_replica_applications_name"
        "idx_instances_app"
    )
    
    for INDEX in "${IMPORTANT_INDEXES[@]}"; do
        if grep -q "$INDEX" *.sql; then
            echo -e "  ${GREEN}✅${NC} Found index: $INDEX"
        else
            echo -e "  ${YELLOW}⚠️${NC} Not found: $INDEX"
        fi
    done
}

# Function to generate summary report
generate_summary() {
    echo -e "\n${BLUE}=== Migration Validation Summary ===${NC}"
    
    cd "$MIGRATION_DIR"
    
    TOTAL_MIGRATIONS=$(ls *.sql | wc -l)
    BATCH1_MIGRATIONS=$(printf '%s\n' ./*.sql | awk -F/ '{name=$NF; number=substr(name,1,3)+0; if (number < 100) count++} END {print count+0}')
    BATCH2_MIGRATIONS=$(printf '%s\n' ./*.sql | awk -F/ '{name=$NF; number=substr(name,1,3)+0; if (number >= 100) count++} END {print count+0}')
    
    echo "Total migrations: $TOTAL_MIGRATIONS"
    echo "Batch 1 migrations (001-099): $BATCH1_MIGRATIONS"
    echo "Batch 2 migrations (100+): $BATCH2_MIGRATIONS"
}

# Run all checks.
#
# set -e is disabled around them deliberately: every check must run so the log
# shows the full picture, rather than stopping at the first failure. Failures
# are accumulated and reported in the exit status, because this script is a CI
# gate — a guard that always exits 0 is not a guard. The summary reports what
# failed instead of unconditionally claiming that "all critical checks passed",
# which is what it used to print even on the run that produced this rewrite.
set +e
FAILED_CHECKS=()
run_check() {
    "$1" || FAILED_CHECKS+=("$1")
}

run_check check_duplicates
run_check check_filename_shape
run_check check_batch2_migrations
run_check check_foreign_keys
run_check check_tenancy_columns
run_check check_batch2_entities
run_check check_cascading_behavior
run_check check_unique_constraints
run_check check_nullable_fields
run_check check_indexes
generate_summary

if [ ${#FAILED_CHECKS[@]} -ne 0 ]; then
    echo -e "\n${RED}❌ Migration validation failed: ${#FAILED_CHECKS[@]} check(s)${NC}"
    for check in "${FAILED_CHECKS[@]}"; do
        echo "   - $check"
    done
    echo "Checks that emit ⚠️ warnings above do not fail the run; only ❌ does."
    exit 1
fi

echo -e "\n🎉 Migration validation passed!"
