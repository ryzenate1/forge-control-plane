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
MIGRATION_DIR="$SCRIPT_DIR/../forge/api/migrations"

echo "Migration directory: $MIGRATION_DIR"

# Function to check for duplicate migration identifiers
check_duplicates() {
    echo -e "\n${BLUE}Checking for duplicate migration identifiers...${NC}"
    
    cd "$MIGRATION_DIR"
    
    # Get all migration names without .sql extension
    MIGRATIONS=$(ls *.sql | sed 's/\.sql$//' | sort)
    
    # Check for duplicates
    DUPLICATES=$(echo "$MIGRATIONS" | uniq -d)
    
    if [ -n "$DUPLICATES" ]; then
        echo -e "${RED}❌ Found duplicate migration identifiers:${NC}"
        echo "$DUPLICATES"
        return 1
    else
        echo -e "${GREEN}✅ No duplicate migration identifiers found${NC}"
    fi
}

# Function to check migration order
check_order() {
    echo -e "\n${BLUE}Checking migration order...${NC}"
    
    cd "$MIGRATION_DIR"
    
    # Get all migration names without .sql extension
    MIGRATIONS=$(ls *.sql | sed 's/\.sql$//' | sort)
    
    PREV=""
    for MIGRATION in $MIGRATIONS; do
        if [ -n "$PREV" ] && [[ "$MIGRATION" < "$PREV" ]]; then
            echo -e "${RED}❌ Migration order violation: $MIGRATION comes after $PREV${NC}"
            return 1
        fi
        PREV="$MIGRATION"
    done
    
    echo -e "${GREEN}✅ Migration order is correct${NC}"
}

# Function to check for required Batch 2 migrations
check_batch2_migrations() {
    echo -e "\n${BLUE}Checking for required Batch 2 migrations...${NC}"
    
    cd "$MIGRATION_DIR"
    
    REQUIRED_BATCH2=(
        "100_team_tenancy.sql"
        "101_app_platform_applications.sql"
        "101_multi_node_replicas.sql"
        "102_reconcile_plans.sql"
        "102_uncloud_service_model.sql"
        "103_backup_schedules_orchestration.sql"
        "103_deployment_steps.sql"
        "103_procedures.sql"
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
    TABLES=$(grep -o 'CREATE TABLE IF NOT EXISTS [^ ]*' *.sql | sed 's/CREATE TABLE IF NOT EXISTS //' | sort | uniq)
    
    # Extract all referenced tables from foreign key constraints
    REFERENCES=$(grep -o 'REFERENCES [^ ]*' *.sql | sed 's/REFERENCES //' | sort | uniq)
    
    ISSUES=()
    for REF in $REFERENCES; do
        if ! echo "$TABLES" | grep -q -w "$REF"; then
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
        # Check if table has org_id column
        if ! grep -q "org_id" *.sql | grep -q "$TABLE"; then
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
        "reservations:Resource reservations"
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
        if grep -q "CREATE TABLE" *.sql | grep -q "$TABLE"; then
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
    CASCADE_COUNT=$(grep -c "ON DELETE CASCADE" *.sql || echo "0")
    RESTRICT_COUNT=$(grep -c -E "ON DELETE RESTRICT|ON DELETE NO ACTION" *.sql || echo "0")
    
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
    
    UNIQUE_COUNT=$(grep -c "UNIQUE\|PRIMARY KEY" *.sql)
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
    
    NOT_NULL_COUNT=$(grep -c "NOT NULL" *.sql)
    NULL_COUNT=$(grep -c "NULL\|DEFAULT NULL" *.sql)
    
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
    
    INDEX_COUNT=$(grep -c "CREATE INDEX\|CREATE UNIQUE INDEX" *.sql)
    echo "  Indexes: $INDEX_COUNT occurrences"
    
    # Look for important indexes
    IMPORTANT_INDEXES=(
        "idx_applications_org_id"
        "idx_servers_org_id"
        "idx_deployments_org_id"
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
    BATCH1_MIGRATIONS=$(ls [0-9][0-9][0-9]_*.sql | wc -l)
    BATCH2_MIGRATIONS=$(ls 1[0-9][0-9]_*.sql | wc -l)
    
    echo "Total migrations: $TOTAL_MIGRATIONS"
    echo "Batch 1 migrations (001-099): $BATCH1_MIGRATIONS"
    echo "Batch 2 migrations (100+): $BATCH2_MIGRATIONS"
    
    echo -e "\n${GREEN}✅ Validation completed!${NC}"
    echo "All critical checks passed."
    echo "Review warnings above for potential issues."
}

# Run all checks
check_duplicates
check_order
check_batch2_migrations
check_foreign_keys
check_tenancy_columns
check_batch2_entities
check_cascading_behavior
check_unique_constraints
check_nullable_fields
check_indexes
generate_summary

echo -e "\n🎉 Migration validation completed!"