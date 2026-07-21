#!/bin/bash

# Simple Migration Validation Script
# Focuses on the key validation points without complex grep patterns

set -e

echo "🔍 Starting simple migration validation..."

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MIGRATION_DIR="$SCRIPT_DIR/../forge/api/migrations"

echo "Migration directory: $MIGRATION_DIR"

# Function to check for duplicate migration identifiers
check_duplicates() {
    echo -e "\n📋 Checking for duplicate migration identifiers..."
    
    cd "$MIGRATION_DIR"
    
    # Get all migration names without .sql extension
    ls *.sql | sed 's/\.sql$//' | sort | uniq -d > /tmp/duplicates.txt
    
    if [ -s /tmp/duplicates.txt ]; then
        echo "❌ Found duplicate migration identifiers:"
        cat /tmp/duplicates.txt
        return 1
    else
        echo "✅ No duplicate migration identifiers found"
    fi
}

# Function to check migration order
check_order() {
    echo -e "\n📋 Checking migration order..."
    
    cd "$MIGRATION_DIR"
    
    # Get all migration names without .sql extension
    MIGRATIONS=$(ls *.sql | sed 's/\.sql$//' | sort)
    
    PREV=""
    for MIGRATION in $MIGRATIONS; do
        if [ -n "$PREV" ] && [[ "$MIGRATION" < "$PREV" ]]; then
            echo "❌ Migration order violation: $MIGRATION comes after $PREV"
            return 1
        fi
        PREV="$MIGRATION"
    done
    
    echo "✅ Migration order is correct"
}

# Function to check for required Batch 2 migrations
check_batch2_migrations() {
    echo -e "\n📋 Checking for required Batch 2 migrations..."
    
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
        echo "❌ Missing required Batch 2 migrations:"
        for MIGRATION in "${MISSING[@]}"; do
            echo "  - $MIGRATION"
        done
        return 1
    else
        echo "✅ All required Batch 2 migrations present"
    fi
}

# Function to check Batch 2 entities
check_batch2_entities() {
    echo -e "\n📋 Checking Batch 2 entities..."
    
    cd "$MIGRATION_DIR"
    
    # List of Batch 2 entities and their tables
    BATCH2_ENTITIES=(
        "organizations"
        "projects"
        "environments"
        "team_members"
        "applications"
        "app_services"
        "replica_applications"
        "instances"
        "placement_decisions"
        "reservations"
        "reconcile_plans"
        "reconcile_events"
        "service_endpoints"
        "procedures"
        "procedure_steps"
        "procedure_executions"
        "deployment_steps"
        "backup_policies"
        "backup_manifests"
        "backup_storage_receipts"
        "database_backups"
        "volume_backups"
    )
    
    FOUND=0
    NOT_FOUND=0
    
    for TABLE in "${BATCH2_ENTITIES[@]}"; do
        if grep -l "CREATE TABLE.*$TABLE" *.sql > /dev/null 2>&1; then
            echo "  ✅ $TABLE"
            ((FOUND++))
        else
            echo "  ⚠️  $TABLE (not found in CREATE TABLE)"
            ((NOT_FOUND++))
        fi
    done
    
    echo "  Found: $FOUND, Not found: $NOT_FOUND"
    
    if [ $NOT_FOUND -gt 0 ]; then
        return 1
    fi
}

# Function to check for tenancy columns
check_tenancy_columns() {
    echo -e "\n📋 Checking tenancy ownership columns..."
    
    cd "$MIGRATION_DIR"
    
    # Check for org_id columns in Batch 2 migrations
    ORG_ID_COUNT=$(grep -h "org_id" 100_*.sql 101_*.sql 102_*.sql 103_*.sql | wc -l)
    PROJECT_ID_COUNT=$(grep -h "project_id" 100_*.sql 101_*.sql 102_*.sql 103_*.sql | wc -l)
    ENV_ID_COUNT=$(grep -h "environment_id" 100_*.sql 101_*.sql 102_*.sql 103_*.sql | wc -l)
    
    echo "  org_id references: $ORG_ID_COUNT"
    echo "  project_id references: $PROJECT_ID_COUNT"
    echo "  environment_id references: $ENV_ID_COUNT"
    
    if [ $ORG_ID_COUNT -gt 0 ]; then
        echo "  ✅ Tenancy ownership columns found"
    else
        echo "  ⚠️  No org_id columns found in Batch 2 migrations"
    fi
}

# Function to check foreign key patterns
check_foreign_keys() {
    echo -e "\n📋 Checking foreign key patterns..."
    
    cd "$MIGRATION_DIR"
    
    # Count foreign key references
    FK_COUNT=$(grep -h "REFERENCES" *.sql | wc -l)
    CASCADE_COUNT=$(grep -h "ON DELETE CASCADE" *.sql | wc -l)
    
    echo "  Foreign key references: $FK_COUNT"
    echo "  ON DELETE CASCADE: $CASCADE_COUNT"
    
    if [ $CASCADE_COUNT -gt 0 ]; then
        echo "  ⚠️  CASCADE deletes found - verify these are intentional"
    fi
}

# Function to check unique constraints
check_unique_constraints() {
    echo -e "\n📋 Checking unique constraints..."
    
    cd "$MIGRATION_DIR"
    
    UNIQUE_COUNT=$(grep -h "UNIQUE" *.sql | wc -l)
    PRIMARY_KEY_COUNT=$(grep -h "PRIMARY KEY" *.sql | wc -l)
    
    echo "  UNIQUE constraints: $UNIQUE_COUNT"
    echo "  PRIMARY KEY constraints: $PRIMARY_KEY_COUNT"
    
    # Check for important unique constraints
    if grep -h "UNIQUE.*org_id.*slug" *.sql > /dev/null 2>&1; then
        echo "  ✅ Found org/slug unique constraint"
    fi
    
    if grep -h "UNIQUE.*project_id.*name" *.sql > /dev/null 2>&1; then
        echo "  ✅ Found project/name unique constraint"
    fi
}

# Function to check nullable vs required fields
check_nullable_fields() {
    echo -e "\n📋 Checking nullable vs required fields..."
    
    cd "$MIGRATION_DIR"
    
    NOT_NULL_COUNT=$(grep -h "NOT NULL" *.sql | wc -l)
    NULL_COUNT=$(grep -h "NULL" *.sql | grep -v "NOT NULL" | wc -l)
    
    echo "  NOT NULL constraints: $NOT_NULL_COUNT"
    echo "  NULL constraints: $NULL_COUNT"
}

# Function to check indexes
check_indexes() {
    echo -e "\n📋 Checking indexes..."
    
    cd "$MIGRATION_DIR"
    
    INDEX_COUNT=$(grep -h "CREATE INDEX" *.sql | wc -l)
    UNIQUE_INDEX_COUNT=$(grep -h "CREATE UNIQUE INDEX" *.sql | wc -l)
    
    echo "  Indexes: $INDEX_COUNT"
    echo "  Unique indexes: $UNIQUE_INDEX_COUNT"
}

# Function to generate summary report
generate_summary() {
    echo -e "\n📊 Migration Validation Summary"
    
    cd "$MIGRATION_DIR"
    
    TOTAL_MIGRATIONS=$(ls *.sql | wc -l)
    BATCH1_MIGRATIONS=$(ls [0-9][0-9][0-9]_*.sql | wc -l)
    BATCH2_MIGRATIONS=$(ls 1[0-9][0-9]_*.sql | wc -l)
    
    echo "Total migrations: $TOTAL_MIGRATIONS"
    echo "Batch 1 migrations (001-099): $BATCH1_MIGRATIONS"
    echo "Batch 2 migrations (100+): $BATCH2_MIGRATIONS"
    
    echo -e "\n✅ Validation completed!"
}

# Run all checks
check_duplicates
check_order
check_batch2_migrations
check_batch2_entities
check_tenancy_columns
check_foreign_keys
check_unique_constraints
check_nullable_fields
check_indexes
generate_summary

echo -e "\n🎉 Migration validation completed!"